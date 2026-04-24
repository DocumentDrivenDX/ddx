package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
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
