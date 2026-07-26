package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDefaultBranchCmdFixture clones a bare origin seeded with one commit on
// defaultBranch. The work dir tracks origin/<defaultBranch> and has
// refs/remotes/origin/HEAD -> refs/remotes/origin/<defaultBranch>.
func setupDefaultBranchCmdFixture(t *testing.T, defaultBranch string) (workDir, originDir string) {
	t.Helper()
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	gitEnv := gitpkg.CleanEnv()
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = gitEnv
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	runOut := func(dir string, args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = gitEnv
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
		return string(out)
	}

	originDir = t.TempDir()
	run(originDir, "init", "--bare", "-b", defaultBranch)

	seedDir := t.TempDir()
	run(seedDir, "clone", originDir, ".")
	run(seedDir, "config", "user.email", "test@ddx.test")
	run(seedDir, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(seedDir, "seed.txt"), []byte("seed\n"), 0o644))
	run(seedDir, "add", "seed.txt")
	run(seedDir, "commit", "-m", "chore: initial seed")
	run(seedDir, "push", "-u", "origin", defaultBranch)

	workDir = t.TempDir()
	run(workDir, "clone", originDir, ".")
	run(workDir, "config", "user.email", "test@ddx.test")
	run(workDir, "config", "user.name", "DDx Test")

	// Ensure origin/HEAD is a symbolic ref even on older git clones.
	headCmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	headCmd.Dir = workDir
	headCmd.Env = gitEnv
	if out, err := headCmd.CombinedOutput(); err != nil || len(out) == 0 {
		run(workDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+defaultBranch)
	}
	require.Contains(t, runOut(workDir, "symbolic-ref", "refs/remotes/origin/HEAD"), "refs/remotes/origin/"+defaultBranch)

	return workDir, originDir
}

func seedOpenBeadForDefaultBranch(t *testing.T, workDir, beadID string) {
	t.Helper()
	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        beadID,
		Title:     "default-branch preflight bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})
}

func assertBeadUnclaimed(t *testing.T, workDir, beadID string) {
	t.Helper()
	store := bead.NewStore(filepath.Join(workDir, ddxroot.DirName))
	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must remain open (no claim)")
	assert.Empty(t, got.Owner, "bead must not have an owner after preflight rejection")
}

// TestWork_DefaultBranchPreflightRejectsMismatchedUpstreamBeforeClaim proves
// ddx work exits non-zero before any store claim and names both the current
// upstream and origin/HEAD default branch.
func TestWork_DefaultBranchPreflightRejectsMismatchedUpstreamBeforeClaim(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "master")
	// Incident-shaped misconfiguration: local branch master tracks main while
	// origin/HEAD still points at master.
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = workDir
		c.Env = gitpkg.CleanEnv()
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("config", "branch.master.merge", "refs/heads/main")
	run("config", "branch.master.remote", "origin")

	beadID := "ddx-dbp-work-mismatch"
	seedOpenBeadForDefaultBranch(t, workDir, beadID)

	var executed int
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		executed++
		t.Fatalf("executor must not run when default-branch preflight fails")
		return agent.ExecuteBeadReport{}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"work", "--once", "--project", workDir,
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.Error(t, err, "mismatched upstream must fail: %s", out)
	assert.Contains(t, out, "refs/heads/main", "must name the configured upstream merge ref: %s", out)
	assert.Contains(t, out, "refs/remotes/origin/master", "must name origin/HEAD: %s", out)
	assert.Zero(t, executed, "executor must not run")
	assertBeadUnclaimed(t, workDir, beadID)
}

// TestTry_DefaultBranchPreflightRejectsMismatchedUpstreamBeforeClaim proves
// ddx try applies the same pre-claim gate before claiming the targeted bead.
func TestTry_DefaultBranchPreflightRejectsMismatchedUpstreamBeforeClaim(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "master")
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = workDir
		c.Env = gitpkg.CleanEnv()
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("config", "branch.master.merge", "refs/heads/main")
	run("config", "branch.master.remote", "origin")

	beadID := "ddx-dbp-try-mismatch"
	seedOpenBeadForDefaultBranch(t, workDir, beadID)

	var executed int
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		executed++
		t.Fatalf("executor must not run when default-branch preflight fails")
		return agent.ExecuteBeadReport{}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"try", beadID, "--project", workDir,
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.Error(t, err, "mismatched upstream must fail: %s", out)
	assert.Contains(t, out, "refs/heads/main", "must name the configured upstream merge ref: %s", out)
	assert.Contains(t, out, "refs/remotes/origin/master", "must name origin/HEAD: %s", out)
	assert.Zero(t, executed, "executor must not run")
	assertBeadUnclaimed(t, workDir, beadID)
}

// TestWork_AllowNonDefaultBranchBypassesGateWithWarning proves the override
// permits execution setup and emits a warning naming both refs.
func TestWork_AllowNonDefaultBranchBypassesGateWithWarning(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "master")
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = workDir
		c.Env = gitpkg.CleanEnv()
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("config", "branch.master.merge", "refs/heads/main")
	run("config", "branch.master.remote", "origin")

	beadID := "ddx-dbp-work-allow"
	seedOpenBeadForDefaultBranch(t, workDir, beadID)

	var executed int
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		executed++
		return agent.ExecuteBeadReport{
			BeadID:    id,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "rev-" + id,
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"work", "--once", "--project", workDir,
		"--allow-non-default-branch",
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "override must permit drain: %s", out)
	assert.Contains(t, out, "--allow-non-default-branch", "must emit override warning: %s", out)
	assert.Contains(t, out, "refs/heads/main", "warning must name upstream: %s", out)
	assert.Contains(t, out, "refs/remotes/origin/master", "warning must name origin/HEAD: %s", out)
	assert.Equal(t, 1, executed, "executor must run once under override")
}

// TestTry_AllowNonDefaultBranchBypassesGateWithWarning proves the override for
// ddx try permits execution setup and emits a warning naming both refs.
func TestTry_AllowNonDefaultBranchBypassesGateWithWarning(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "master")
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = workDir
		c.Env = gitpkg.CleanEnv()
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("config", "branch.master.merge", "refs/heads/main")
	run("config", "branch.master.remote", "origin")

	beadID := "ddx-dbp-try-allow"
	seedOpenBeadForDefaultBranch(t, workDir, beadID)

	var executed int
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		executed++
		return agent.ExecuteBeadReport{
			BeadID:    id,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "rev-" + id,
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"try", beadID, "--project", workDir,
		"--allow-non-default-branch",
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "override must permit try: %s", out)
	assert.Contains(t, out, "--allow-non-default-branch", "must emit override warning: %s", out)
	assert.Contains(t, out, "refs/heads/main", "warning must name upstream: %s", out)
	assert.Contains(t, out, "refs/remotes/origin/master", "warning must name origin/HEAD: %s", out)
	assert.Equal(t, 1, executed, "executor must run once under override")
}

// TestWork_DefaultBranchPreflightDetachedHEADFailsBeforeClaim proves detached
// HEAD fails with a clear message before any claim.
func TestWork_DefaultBranchPreflightDetachedHEADFailsBeforeClaim(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "main")
	detachToHEAD(t, workDir)

	beadID := "ddx-dbp-work-detach"
	seedOpenBeadForDefaultBranch(t, workDir, beadID)

	var executed int
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		executed++
		t.Fatalf("executor must not run on detached HEAD")
		return agent.ExecuteBeadReport{}, nil
	})

	cmdOut, err := executeCommand(
		factory.NewRootCommand(),
		"work", "--once", "--project", workDir,
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.Error(t, err, "detached HEAD must fail: %s", cmdOut)
	assert.Contains(t, cmdOut, "detached", "message must mention detached HEAD: %s", cmdOut)
	assert.Zero(t, executed)
	assertBeadUnclaimed(t, workDir, beadID)
}

// TestTry_DefaultBranchPreflightDetachedHEADFailsBeforeClaim proves detached
// HEAD fails for ddx try with a clear message before any claim.
func TestTry_DefaultBranchPreflightDetachedHEADFailsBeforeClaim(t *testing.T) {
	workDir, _ := setupDefaultBranchCmdFixture(t, "main")
	detachToHEAD(t, workDir)

	beadID := "ddx-dbp-try-detach"
	seedOpenBeadForDefaultBranch(t, workDir, beadID)

	var executed int
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		executed++
		t.Fatalf("executor must not run on detached HEAD")
		return agent.ExecuteBeadReport{}, nil
	})

	cmdOut, err := executeCommand(
		factory.NewRootCommand(),
		"try", beadID, "--project", workDir,
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.Error(t, err, "detached HEAD must fail: %s", cmdOut)
	assert.Contains(t, cmdOut, "detached", "message must mention detached HEAD: %s", cmdOut)
	assert.Zero(t, executed)
	assertBeadUnclaimed(t, workDir, beadID)
}

func detachToHEAD(t *testing.T, workDir string) {
	t.Helper()
	c := exec.Command("git", "rev-parse", "HEAD")
	c.Dir = workDir
	c.Env = gitpkg.CleanEnv()
	shaOut, err := c.CombinedOutput()
	require.NoError(t, err, "rev-parse HEAD: %s", shaOut)
	sha := string(shaOut)
	if len(sha) > 0 && sha[len(sha)-1] == '\n' {
		sha = sha[:len(sha)-1]
	}
	detach := exec.Command("git", "checkout", "--detach", sha)
	detach.Dir = workDir
	detach.Env = gitpkg.CleanEnv()
	out, err := detach.CombinedOutput()
	require.NoError(t, err, "checkout --detach: %s", out)
}

// TestWork_DefaultBranchPreflightBehindUpstreamWarnsAtDrainStart proves
// local-behind-upstream is surfaced as a warning at drain start without
// blocking a matching default-branch checkout.
func TestWork_DefaultBranchPreflightBehindUpstreamWarnsAtDrainStart(t *testing.T) {
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

	// Advance origin via a second clone, then fetch so the local remote-tracking
	// tip is ahead of the local branch tip (behind upstream).
	second := t.TempDir()
	run(second, "clone", originDir, ".")
	run(second, "config", "user.email", "test@ddx.test")
	run(second, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(second, "remote.txt"), []byte("remote\n"), 0o644))
	run(second, "add", "remote.txt")
	run(second, "commit", "-m", "feat: remote advance")
	run(second, "push", "origin", "main")
	run(workDir, "fetch", "origin", "main")

	beadID := "ddx-dbp-work-behind"
	seedOpenBeadForDefaultBranch(t, workDir, beadID)

	var executed int
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (agent.ExecuteBeadReport, error) {
		executed++
		return agent.ExecuteBeadReport{
			BeadID:    id,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "rev-" + id,
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"work", "--once", "--project", workDir,
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "behind upstream must not block drain: %s", out)
	assert.Contains(t, out, "behind", "drain start must warn about behind upstream: %s", out)
	assert.Equal(t, 1, executed, "ready bead must still be claimed and executed")
}
