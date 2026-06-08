package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/gitrepohealth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTestRepo creates a git repo at dir with a clean initial commit.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	env := gitpkg.CleanEnv()

	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = env
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644))
	run("add", "README")
	run("commit", "-m", "init")
}

func hasIssueType(issues []DiagnosticIssue, t string) bool {
	for _, i := range issues {
		if i.Type == t {
			return true
		}
	}
	return false
}

// TestCheckGitRepoHealthDetectsCoreBare verifies that core.bare=true on a repo
// with a working tree is detected, and that --fix removes it.
func TestCheckGitRepoHealthDetectsCoreBare(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Corrupt the repo — set core.bare=true on a repo that clearly has a work tree.
	corrupt := exec.Command("git", "config", "core.bare", "true")
	corrupt.Dir = dir
	corrupt.Env = gitpkg.CleanEnv()
	require.NoError(t, corrupt.Run())

	// Without --fix: detection only.
	issues := checkGitRepoHealth(dir, false)
	assert.True(t, hasIssueType(issues, "git_core_bare_corruption"),
		"expected git_core_bare_corruption issue, got: %+v", issues)

	// Confirm core.bare still present.
	getBare := exec.Command("git", "config", "--local", "--get", "core.bare")
	getBare.Dir = dir
	getBare.Env = gitpkg.CleanEnv()
	out, _ := getBare.Output()
	assert.Contains(t, string(out), "true", "core.bare should still be set before --fix")

	// With --fix: remediate.
	issues = checkGitRepoHealth(dir, true)
	assert.True(t, hasIssueType(issues, "git_core_bare_corruption"),
		"issue should still be reported under --fix (as fixed)")

	getBare2 := exec.Command("git", "config", "--local", "--get", "core.bare")
	getBare2.Dir = dir
	getBare2.Env = gitpkg.CleanEnv()
	if err := getBare2.Run(); err == nil {
		t.Fatalf("core.bare should be unset after --fix")
	}
}

// TestCheckGitRepoHealthDetectsStrayCoreWorktree verifies stray core.worktree detection.
func TestCheckGitRepoHealthDetectsStrayCoreWorktree(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	bogus := filepath.Join(t.TempDir(), "not-the-real-worktree")
	set := exec.Command("git", "config", "core.worktree", bogus)
	set.Dir = dir
	set.Env = gitpkg.CleanEnv()
	require.NoError(t, set.Run())

	issues := checkGitRepoHealth(dir, false)
	assert.True(t, hasIssueType(issues, "git_stray_core_worktree"),
		"expected git_stray_core_worktree issue, got: %+v", issues)

	// --fix should remove it.
	_ = checkGitRepoHealth(dir, true)
	get := exec.Command("git", "config", "--local", "--get", "core.worktree")
	get.Dir = dir
	get.Env = gitpkg.CleanEnv()
	if err := get.Run(); err == nil {
		t.Fatalf("core.worktree should be unset after --fix")
	}
}

// TestCheckGitRepoHealthDetectsPrimaryCheckoutCoreWorktreeRedirect verifies
// the incident shape where the primary checkout's .git/config redirects Git
// commands to a different worktree path.
func TestCheckGitRepoHealthDetectsPrimaryCheckoutCoreWorktreeRedirect(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	redirect := filepath.Join(t.TempDir(), "redirected-worktree")
	add := exec.Command("git", "worktree", "add", "--detach", redirect, "HEAD")
	add.Dir = dir
	add.Env = gitpkg.CleanEnv()
	require.NoError(t, add.Run())

	set := exec.Command("git", "config", "core.worktree", redirect)
	set.Dir = dir
	set.Env = gitpkg.CleanEnv()
	require.NoError(t, set.Run())

	issues := checkGitRepoHealth(dir, false)
	assert.True(t, hasIssueType(issues, "git_stray_core_worktree"),
		"expected git_stray_core_worktree issue, got: %+v", issues)
}

// TestCheckGitRepoHealthFixUnsetsRedirectedCoreWorktree verifies --fix removes
// the redirect and a subsequent health check no longer reports the corruption.
func TestCheckGitRepoHealthFixUnsetsRedirectedCoreWorktree(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	redirect := filepath.Join(t.TempDir(), "redirected-worktree")
	add := exec.Command("git", "worktree", "add", "--detach", redirect, "HEAD")
	add.Dir = dir
	add.Env = gitpkg.CleanEnv()
	require.NoError(t, add.Run())

	set := exec.Command("git", "config", "core.worktree", redirect)
	set.Dir = dir
	set.Env = gitpkg.CleanEnv()
	require.NoError(t, set.Run())

	issues := checkGitRepoHealth(dir, true)
	assert.True(t, hasIssueType(issues, "git_stray_core_worktree"),
		"expected git_stray_core_worktree issue, got: %+v", issues)

	issues = checkGitRepoHealth(dir, false)
	assert.False(t, hasIssueType(issues, "git_stray_core_worktree"),
		"core.worktree redirect should be gone after --fix, got: %+v", issues)

	get := exec.Command("git", "config", "--local", "--get", "core.worktree")
	get.Dir = dir
	get.Env = gitpkg.CleanEnv()
	if err := get.Run(); err == nil {
		t.Fatalf("core.worktree should be unset after --fix")
	}
}

// TestCheckGitRepoHealthDetectsLocalHooksPath verifies stale local
// core.hooksPath detection and --fix remediation. A local hooksPath causes
// lefthook to print "Skipping hook sync" and can leave stale hooks active.
func TestCheckGitRepoHealthDetectsLocalHooksPath(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	set := exec.Command("git", "config", "core.hooksPath", ".git/hooks")
	set.Dir = dir
	set.Env = gitpkg.CleanEnv()
	require.NoError(t, set.Run())

	issues := checkGitRepoHealth(dir, false)
	assert.True(t, hasIssueType(issues, "git_local_hooks_path"),
		"expected git_local_hooks_path issue, got: %+v", issues)

	issues = checkGitRepoHealth(dir, true)
	assert.True(t, hasIssueType(issues, "git_local_hooks_path"),
		"issue should still be reported under --fix (as fixed)")

	get := exec.Command("git", "config", "--local", "--get", "core.hooksPath")
	get.Dir = dir
	get.Env = gitpkg.CleanEnv()
	if err := get.Run(); err == nil {
		t.Fatalf("core.hooksPath should be unset after --fix")
	}
}

func TestCheckGitRepoHealthFixUsesSharedHelperForSafeConfigKeys(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	bogus := filepath.Join(t.TempDir(), "not-the-real-worktree")
	for _, args := range [][]string{
		{"config", "core.worktree", bogus},
		{"config", "core.hooksPath", ".git/hooks"},
		{"config", "core.bare", "true"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = gitpkg.CleanEnv()
		require.NoError(t, cmd.Run(), "git %v", args)
	}

	issues := checkGitRepoHealth(dir, true)
	assert.True(t, hasIssueType(issues, gitrepohealth.IssueCoreBareCorruption),
		"expected core.bare issue, got: %+v", issues)
	assert.True(t, hasIssueType(issues, gitrepohealth.IssueStrayCoreWorktree),
		"expected core.worktree issue, got: %+v", issues)
	assert.True(t, hasIssueType(issues, gitrepohealth.IssueLocalHooksPath),
		"expected hooksPath issue, got: %+v", issues)

	for _, key := range []string{"core.bare", "core.worktree", "core.hooksPath"} {
		cmd := exec.Command("git", "config", "--local", "--get", key)
		cmd.Dir = dir
		cmd.Env = gitpkg.CleanEnv()
		assert.Error(t, cmd.Run(), "%s should be unset after --fix", key)
	}
	cmd := exec.Command("git", "config", "--local", "--get", "extensions.worktreeConfig")
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	assert.Error(t, cmd.Run(), "doctor --fix must leave extensions.worktreeConfig warning-only")
}

// TestPreCommitDDXValidateFailsOnCoreWorktreeRedirect verifies the pre-commit
// guard used by lefthook fails when local config points the primary checkout
// at a different worktree.
func TestPreCommitDDXValidateFailsOnCoreWorktreeRedirect(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	redirect := filepath.Join(t.TempDir(), "redirected-worktree")
	add := exec.Command("git", "worktree", "add", "--detach", redirect, "HEAD")
	add.Dir = dir
	add.Env = gitpkg.CleanEnv()
	require.NoError(t, add.Run())

	set := exec.Command("git", "config", "core.worktree", redirect)
	set.Dir = dir
	set.Env = gitpkg.CleanEnv()
	require.NoError(t, set.Run())

	script, absErr := filepath.Abs(filepath.Join("..", "..", "scripts", "git-config-health.sh"))
	require.NoError(t, absErr)
	cmd := exec.Command("sh", script)
	cmd.Dir = dir
	cmd.Env = append(gitpkg.CleanEnv(), "DDX_GIT_CONFIG_HEALTH_ROOT="+dir)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "hook guard should fail on redirected core.worktree")
	assert.Contains(t, string(out), "Invalid local git config: core.worktree=")
	assert.Contains(t, string(out), "git config --unset core.worktree")

	lefthook, readErr := os.ReadFile(filepath.Join("..", "..", "lefthook.yml"))
	require.NoError(t, readErr)
	assert.Contains(t, string(lefthook), "git-config-health:")
	assert.Contains(t, string(lefthook), "sh scripts/git-config-health.sh")
}

// TestPreCommitGitConfigHealthFailsOnCoreHooksPath verifies the pre-commit
// guard fails before lefthook can silently continue with stale hook-path config.
func TestPreCommitGitConfigHealthFailsOnCoreHooksPath(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	set := exec.Command("git", "config", "core.hooksPath", ".git/hooks")
	set.Dir = dir
	set.Env = gitpkg.CleanEnv()
	require.NoError(t, set.Run())

	script, absErr := filepath.Abs(filepath.Join("..", "..", "scripts", "git-config-health.sh"))
	require.NoError(t, absErr)
	cmd := exec.Command("sh", script)
	cmd.Dir = dir
	cmd.Env = append(gitpkg.CleanEnv(), "DDX_GIT_CONFIG_HEALTH_ROOT="+dir)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "hook guard should fail on local core.hooksPath")
	assert.Contains(t, string(out), "Invalid local git config: core.hooksPath=.git/hooks")
	assert.Contains(t, string(out), "git config --unset-all --local core.hooksPath")
}

// TestCheckGitRepoHealthCleanRepo verifies a clean repo produces no corruption
// issues (only the optional worktreeConfig warning at most).
func TestCheckGitRepoHealthCleanRepo(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	issues := checkGitRepoHealth(dir, false)

	assert.False(t, hasIssueType(issues, "git_core_bare_corruption"),
		"clean repo should not report core.bare corruption")
	assert.False(t, hasIssueType(issues, "git_stray_core_worktree"),
		"clean repo should not report stray core.worktree")
}

// TestCheckGitRepoHealthWorktreeConfigWarning verifies that a missing
// extensions.worktreeConfig surfaces as a (non-fatal) warning issue.
func TestCheckGitRepoHealthWorktreeConfigWarning(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	issues := checkGitRepoHealth(dir, false)
	assert.True(t, hasIssueType(issues, "git_worktree_config_disabled"),
		"expected git_worktree_config_disabled warning on fresh repo")

	// Enable it, then the warning should disappear.
	en := exec.Command("git", "config", "extensions.worktreeConfig", "true")
	en.Dir = dir
	en.Env = gitpkg.CleanEnv()
	require.NoError(t, en.Run())

	issues = checkGitRepoHealth(dir, false)
	assert.False(t, hasIssueType(issues, "git_worktree_config_disabled"),
		"warning should clear once extensions.worktreeConfig=true")
}

// TestCheckGitRepoHealthNonGitDir verifies the check is a no-op outside a repo.
func TestCheckGitRepoHealthNonGitDir(t *testing.T) {
	dir := t.TempDir()
	issues := checkGitRepoHealth(dir, false)
	assert.Nil(t, issues, "non-git dir should return nil issues")
}
