package agent

// execute_bead_dirty_rescue_test.go — tests for ddx-3141b561: dirty
// no-evidence attempt rescue. When an agent exits 0 without committing or
// writing a rationale but leaves dirty files in the worktree, DDx must
// preserve a binary patch artifact instead of silently deleting the evidence.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noSynthesizeGitOps wraps RealGitOps but disables SynthesizeCommit, leaving
// any dirty files in the worktree without committing them.
type noSynthesizeGitOps struct {
	RealGitOps
}

func (g *noSynthesizeGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	return false, nil
}

// dirtyCodeRunner creates a source file in opts.WorkDir and exits 0 without
// committing, simulating an agent that modified files but failed to commit.
type dirtyCodeRunner struct{}

func (r *dirtyCodeRunner) Run(opts RunArgs) (*Result, error) {
	if opts.WorkDir != "" {
		_ = os.WriteFile(filepath.Join(opts.WorkDir, "dirty.go"), []byte("package main\n// dirty change\n"), 0o644)
	}
	return &Result{ExitCode: 0}, nil
}

// cleanExitRunner exits 0 without creating any files.
type cleanExitRunner struct{}

func (r *cleanExitRunner) Run(opts RunArgs) (*Result, error) {
	return &Result{ExitCode: 0}, nil
}

// TestExecuteBead_DirtyNoEvidencePreservesAttempt proves that when a runner
// leaves dirty files and exits 0 without committing or writing a rationale,
// ExecuteBeadWithConfig returns no_evidence_produced with a non-empty
// PreserveRef pointing to the rescue patch artifact.
func TestExecuteBead_DirtyNoEvidencePreservesAttempt(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	gitOps := &noSynthesizeGitOps{}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "virtual",
	}).Resolve(config.TestBeadOverrides(config.TestBeadConfigOpts{Harness: "virtual"}))

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner: &dirtyCodeRunner{},
	}, gitOps)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, ExecuteBeadOutcomeTaskNoEvidence, res.Outcome)
	assert.NotEmpty(t, res.PreserveRef, "dirty no-evidence attempt must have a non-empty preserve ref pointing to the rescue patch")
	assert.Contains(t, res.PreserveRef, "dirty_rescue.patch", "preserve ref must name the rescue patch file")
	assert.NotEmpty(t, res.NoEvidencePaths, "dirty paths must be recorded")

	// Verify the rescue patch was published to the project root (survived worktree cleanup).
	rescuePatch := filepath.Join(projectRoot, filepath.FromSlash(res.PreserveRef))
	assert.FileExists(t, rescuePatch, "rescue patch must exist in project root after worktree cleanup")
}

// TestExecuteBead_CleanNoEvidenceStillCleansWorktree proves that a clean
// no-evidence attempt (no dirty files) does not create a preserve artifact and
// cleans up normally.
func TestExecuteBead_CleanNoEvidenceStillCleansWorktree(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	gitOps := &noSynthesizeGitOps{}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "virtual",
	}).Resolve(config.TestBeadOverrides(config.TestBeadConfigOpts{Harness: "virtual"}))

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner: &cleanExitRunner{},
	}, gitOps)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, ExecuteBeadOutcomeTaskNoEvidence, res.Outcome)
	assert.Empty(t, res.PreserveRef, "clean no-evidence must not create a preserve artifact")

	// Verify no rescue patch was created for the clean attempt. Check both the
	// worktree (may be removed) and the project root (where evidence is published).
	if res.AttemptID != "" {
		rescuePath := filepath.Join(projectRoot, ExecuteBeadArtifactDir, res.AttemptID, "dirty_rescue.patch")
		assert.NoFileExists(t, rescuePath, "clean no-evidence must not create a dirty_rescue.patch")
	}
}

// TestWorkLoopNoEvidenceReportsPreserveLocation proves that when a
// no_evidence_produced report has a PreserveRef set, the bead event body
// written by the work loop includes the preserve location so operators can
// find and apply the rescue patch.
func TestWorkLoopNoEvidenceReportsPreserveLocation(t *testing.T) {
	report := ExecuteBeadReport{
		BeadID:      "ddx-test-rescue",
		AttemptID:   "attempt-rescue-001",
		Status:      ExecuteBeadStatusNoEvidenceProduced,
		PreserveRef: ".ddx/executions/attempt-rescue-001/dirty_rescue.patch",
		Detail:      "agent exited without a commit or no_changes_rationale.txt; dirty paths: dirty.go; rescue: .ddx/executions/attempt-rescue-001/dirty_rescue.patch",
		BaseRev:     "abc123",
	}

	event := executeBeadLoopEvent(report, "test-worker", time.Now().UTC())

	assert.Equal(t, "execute-bead", event.Kind)
	assert.Equal(t, ExecuteBeadStatusNoEvidenceProduced, event.Summary)
	assert.Contains(t, event.Body, "preserve_ref=.ddx/executions/attempt-rescue-001/dirty_rescue.patch",
		"loop bead event must include the preserve ref so operators can recover the rescue patch")
}

// TestRealGitOpsWorktreeRemoveReportsFailure proves that RealGitOps.WorktreeRemove
// returns an error when git worktree remove --force fails, instead of silently
// swallowing it.
func TestRealGitOpsWorktreeRemoveReportsFailure(t *testing.T) {
	root := t.TempDir()

	cmd := exec.Command("git", "-c", "init.defaultBranch=main", "init", root)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", out)

	gitCmd := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		o, e := c.CombinedOutput()
		require.NoError(t, e, "git %v: %s", args, o)
	}
	gitCmd("config", "user.email", "test@ddx.test")
	gitCmd("config", "user.name", "DDx Test")

	// The path "/nonexistent/not-a-registered-worktree" is not a registered git
	// worktree. git worktree remove --force on it must fail.
	ops := &RealGitOps{}
	err = ops.WorktreeRemove(root, "/nonexistent/not-a-registered-worktree")
	require.Error(t, err, "WorktreeRemove must return an error when git worktree remove fails")
	assert.Contains(t, strings.ToLower(err.Error()), "git worktree remove",
		"error message must identify the failing command")
}
