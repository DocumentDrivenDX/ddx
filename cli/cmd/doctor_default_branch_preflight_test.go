package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoctor_DefaultBranchPreflightReportsMismatchedUpstream proves ddx doctor
// maps a mismatched upstream vs origin/HEAD into a failed DiagnosticIssue that
// names both refs and includes remediation (switch / retarget / override).
func TestDoctor_DefaultBranchPreflightReportsMismatchedUpstream(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "master")
	gitEnv := gitpkg.CleanEnv()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = workDir
		c.Env = gitEnv
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Incident-shaped: local master tracks main while origin/HEAD is master.
	run("config", "branch.master.merge", "refs/heads/main")
	run("config", "branch.master.remote", "origin")

	issues := checkDefaultBranchPreflight(workDir)
	require.NotEmpty(t, issues, "mismatched upstream must produce a doctor issue")
	issue := issues[0]
	assert.Equal(t, "default_branch_hard_fail", issue.Type,
		"mismatch must be a failed (hard-fail) check, got: %+v", issue)
	assert.Contains(t, issue.Description, "refs/heads/main",
		"must name current upstream merge ref: %s", issue.Description)
	assert.Contains(t, issue.Description, "refs/remotes/origin/master",
		"must name origin/HEAD: %s", issue.Description)

	remediation := strings.Join(issue.Remediation, "\n")
	assert.NotEmpty(t, remediation, "must include remediation text")
	assert.True(t,
		strings.Contains(remediation, "git switch") ||
			strings.Contains(remediation, "check out") ||
			strings.Contains(remediation, "checkout"),
		"remediation must cover switching to the default branch, got:\n%s", remediation)
	assert.True(t,
		strings.Contains(remediation, "git branch -u") ||
			strings.Contains(remediation, "branch.master.merge") ||
			strings.Contains(remediation, "upstream"),
		"remediation must cover updating upstream tracking, got:\n%s", remediation)
	assert.Contains(t, remediation, "--allow-non-default-branch",
		"remediation must mention deliberate feature-branch override, got:\n%s", remediation)

	// Full doctor path surfaces the failed check (❌), not only the advisory path.
	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err, "doctor must remain non-fatal; output:\n%s", output)
	assert.Contains(t, output, "Default Branch Tracking", "doctor must run the check: %s", output)
	assert.Contains(t, output, "❌", "hard-fail must mark doctor as failed: %s", output)
	assert.True(t,
		strings.Contains(output, "refs/heads/main") &&
			strings.Contains(output, "refs/remotes/origin/master"),
		"doctor output must name both upstream and origin/HEAD:\n%s", output)
	assert.Contains(t, output, "--allow-non-default-branch",
		"doctor output must include override remediation:\n%s", output)
}

// TestDoctor_DefaultBranchPreflightReportsDetachedHEAD proves ddx doctor
// reports detached HEAD as a failed check with remediation-oriented text.
func TestDoctor_DefaultBranchPreflightReportsDetachedHEAD(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "main")
	gitEnv := gitpkg.CleanEnv()

	shaCmd := exec.Command("git", "rev-parse", "HEAD")
	shaCmd.Dir = workDir
	shaCmd.Env = gitEnv
	shaOut, err := shaCmd.CombinedOutput()
	require.NoError(t, err, "rev-parse HEAD: %s", shaOut)
	sha := strings.TrimSpace(string(shaOut))

	detach := exec.Command("git", "checkout", "--detach", sha)
	detach.Dir = workDir
	detach.Env = gitEnv
	out, err := detach.CombinedOutput()
	require.NoError(t, err, "checkout --detach: %s", out)

	issues := checkDefaultBranchPreflight(workDir)
	require.NotEmpty(t, issues, "detached HEAD must produce a doctor issue")
	issue := issues[0]
	assert.Equal(t, "default_branch_hard_fail", issue.Type,
		"detached HEAD must be a failed (hard-fail) check, got: %+v", issue)
	assert.Contains(t, strings.ToLower(issue.Description), "detached",
		"description must mention detached HEAD: %s", issue.Description)
	remediation := strings.Join(issue.Remediation, "\n")
	assert.True(t,
		strings.Contains(remediation, "git switch") ||
			strings.Contains(remediation, "check out") ||
			strings.Contains(remediation, "checkout"),
		"remediation must tell the operator how to attach HEAD, got:\n%s", remediation)

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err, "doctor must remain non-fatal; output:\n%s", output)
	assert.Contains(t, output, "Default Branch Tracking")
	assert.Contains(t, output, "❌", "detached HEAD must mark doctor as failed: %s", output)
	assert.Contains(t, strings.ToLower(output), "detached",
		"doctor output must mention detached HEAD:\n%s", output)
}

// TestDoctor_DefaultBranchPreflightReportsBehindUpstreamWarning proves doctor
// warns when local is behind its upstream while that upstream still matches
// origin/HEAD (advisory, not a hard failure).
func TestDoctor_DefaultBranchPreflightReportsBehindUpstreamWarning(t *testing.T) {
	workDir, originDir := setupDefaultBranchCmdFixture(t, "main")
	gitEnv := gitpkg.CleanEnv()
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = gitEnv
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Advance origin via a second clone, fetch so the remote-tracking tip is
	// ahead of local, then leave origin unreachable so the check cannot fetch.
	second := t.TempDir()
	run(second, "clone", originDir, ".")
	run(second, "config", "user.email", "test@ddx.test")
	run(second, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(second, "remote.txt"), []byte("remote\n"), 0o644))
	run(second, "add", "remote.txt")
	run(second, "commit", "-m", "feat: remote advance")
	run(second, "push", "origin", "main")
	run(workDir, "fetch", "origin", "main")

	localSHACmd := exec.Command("git", "rev-parse", "HEAD")
	localSHACmd.Dir = workDir
	localSHACmd.Env = gitEnv
	localSHAOut, err := localSHACmd.CombinedOutput()
	require.NoError(t, err)
	localSHA := strings.TrimSpace(string(localSHAOut))

	originTipCmd := exec.Command("git", "rev-parse", "refs/remotes/origin/main")
	originTipCmd.Dir = workDir
	originTipCmd.Env = gitEnv
	originTipOut, err := originTipCmd.CombinedOutput()
	require.NoError(t, err)
	originTip := strings.TrimSpace(string(originTipOut))
	require.NotEqual(t, originTip, localSHA, "fixture must leave local behind upstream")

	// Make origin unreachable so any accidental fetch would fail (P9).
	run(workDir, "remote", "set-url", "origin", "file://"+filepath.Join(t.TempDir(), "gone"))
	require.NoError(t, os.RemoveAll(originDir))

	issues := checkDefaultBranchPreflight(workDir)
	require.NotEmpty(t, issues, "behind upstream must produce a doctor issue")
	issue := issues[0]
	assert.Equal(t, "default_branch_advisory", issue.Type,
		"behind upstream must be a warning/advisory, not hard-fail: %+v", issue)
	assert.Contains(t, strings.ToLower(issue.Description), "behind",
		"description must mention behind: %s", issue.Description)
	assert.Equal(t, "main", issue.SystemInfo["upstream_branch"])
	assert.Equal(t, "main", issue.SystemInfo["default_branch"])
	assert.Equal(t, "behind", issue.SystemInfo["kind"])

	// Local tip unchanged — doctor must not fetch or pull.
	afterCmd := exec.Command("git", "rev-parse", "HEAD")
	afterCmd.Dir = workDir
	afterCmd.Env = gitEnv
	afterOut, err := afterCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, localSHA, strings.TrimSpace(string(afterOut)),
		"doctor check must not advance local HEAD")

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err, "doctor must remain non-fatal; output:\n%s", output)
	assert.Contains(t, output, "Default Branch Tracking")
	assert.Contains(t, output, "Advisories",
		"behind upstream must be advisory, not a hard failure banner:\n%s", output)
	assert.Contains(t, strings.ToLower(output), "behind",
		"doctor output must warn about behind upstream:\n%s", output)
}

// TestDoctor_DefaultBranchPreflightMatchingUpstreamPasses proves a normal
// checkout whose upstream matches origin/HEAD reports a passing check and
// does not add a DiagnosticIssue.
func TestDoctor_DefaultBranchPreflightMatchingUpstreamPasses(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "master")

	issues := checkDefaultBranchPreflight(workDir)
	assert.Empty(t, issues,
		"matching upstream must not add a DiagnosticIssue, got: %+v", issues)

	// Shared diagnostic agrees (pass).
	res := gitpkg.CheckDefaultBranchPreflight(workDir)
	assert.True(t, res.Pass(), "matching upstream must pass: %+v", res)
	assert.Equal(t, gitpkg.DefaultBranchOK, res.Kind)

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err, "doctor must succeed; output:\n%s", output)
	assert.Contains(t, output, "✅ Default Branch Tracking",
		"doctor must report a passing default-branch check:\n%s", output)
	assert.NotContains(t, output, "Default Branch Tracking Issues",
		"passing checkout must not report hard-fail issues:\n%s", output)
	assert.NotContains(t, output, "Default Branch Tracking Advisories",
		"passing checkout must not report advisories:\n%s", output)
}
