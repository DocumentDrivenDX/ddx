package agent

// execute_bead_checkpoint_test.go — Tier-2 integration tests for FEAT-012 §22 +
// US-126 AC#1: when ExecuteBead starts and the parent worktree has uncommitted
// changes, DDx must capture them as a real commit on the current branch
// (the "checkpoint commit") and use the resulting HEAD as the effective base
// revision for the worker worktree. Caller's edits are preserved as a normal
// commit they can `git reset HEAD~` to recover.
//
// Clean parent worktrees must NOT spawn a redundant checkpoint commit.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runNoopExecuteBeadForCheckpoint(t *testing.T, projectRoot, beadID string) *ExecuteBeadResult {
	t.Helper()

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})

	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	return res
}

// TestExecuteBead_DirtyParentTree_CheckpointCommitted seeds a repo, makes the
// parent worktree dirty (one tracked-modified file, one untracked file),
// runs ExecuteBead, and asserts that:
//   - HEAD advanced by at least one commit before the worker worktree was
//     created (the checkpoint commit captures the caller's dirt)
//   - both the modified content and the untracked file are reachable in HEAD
//     (changes survived as a real commit, not discarded)
//   - the worker worktree's BaseRev points at that new HEAD (or a descendant
//     such as a tracker commit), not at the original seed commit
func TestExecuteBead_DirtyParentTree_CheckpointCommitted(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	// Make the parent dirty: modify the tracked seed file and add an
	// untracked file. The checkpoint must capture both per IsDirty semantics.
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "seed.txt"),
		[]byte("seed\nlocal modification\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "untracked.txt"),
		[]byte("untracked content\n"), 0o644))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	commitsBefore := gitCommitCount(t, projectRoot, "HEAD")

	// Directive does nothing observable — we only care about the pre-execution
	// checkpoint behavior. An empty directive yields no_changes; that's fine.
	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})

	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)

	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	commitsAfter := gitCommitCount(t, projectRoot, "HEAD")

	assert.NotEqual(t, headBefore, headAfter,
		"HEAD must advance to capture the dirty caller worktree as a checkpoint commit")
	assert.GreaterOrEqual(t, commitsAfter-commitsBefore, 1,
		"at least one commit (the checkpoint) must land on the parent branch")

	// The modified seed and the untracked file must be reachable in HEAD —
	// the checkpoint preserved the caller's work, did not discard it.
	seedAtHead := runGitInteg(t, projectRoot, "show", "HEAD:seed.txt")
	assert.Contains(t, seedAtHead, "local modification",
		"tracked-modified content must be present in HEAD after checkpoint")
	untrackedAtHead := runGitInteg(t, projectRoot, "show", "HEAD:untracked.txt")
	assert.Contains(t, untrackedAtHead, "untracked content",
		"untracked file must be tracked in HEAD after checkpoint (git add -A)")

	// BaseRev recorded on the result must be the new HEAD, not the original seed.
	assert.NotEqual(t, headBefore, res.BaseRev,
		"BaseRev must be the post-checkpoint HEAD, not the pre-checkpoint HEAD")
}

// TestExecuteBead_CleanParentTree_NoSpuriousCheckpoint runs ExecuteBead against
// a clean parent worktree and asserts that no extra checkpoint commit is made
// beyond what other steps (CommitTracker) might add. The acceptance criterion
// is: clean parent trees do not create redundant checkpoint artifacts.
func TestExecuteBead_CleanParentTree_NoSpuriousCheckpoint(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	// Confirm we start clean.
	status := runGitInteg(t, projectRoot, "status", "--porcelain")
	require.Empty(t, status, "test setup invariant: parent must be clean")

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{})

	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	_, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)

	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	// CommitTracker may have added a tracker commit if .ddx/beads.jsonl
	// changed during the attempt (e.g. claim metadata). What must NOT happen:
	// a separate "checkpoint pre-execute-bead" commit when nothing was dirty.
	if headBefore != headAfter {
		// Inspect the diff: only beads.jsonl (or other tracker files) may
		// differ. No checkpoint commit message in the log between these refs.
		log := runGitInteg(t, projectRoot, "log", "--format=%s", headBefore+".."+headAfter)
		assert.NotContains(t, log, "checkpoint pre-execute-bead",
			"clean parent tree must not produce a checkpoint commit")
	}
}

func TestExecuteBead_PreDispatchCheckpointBypassesHooks(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "caller-dirt.txt"), []byte("dirty\n"), 0o644))

	markerPath := filepath.Join(projectRoot, "hook-invoked.txt")
	hookPath := filepath.Join(projectRoot, ".git", "hooks", "pre-commit")
	hook := "#!/bin/sh\nprintf invoked > " + markerPath + "\nexit 42\n"
	require.NoError(t, os.MkdirAll(filepath.Dir(hookPath), 0o755))
	require.NoError(t, os.WriteFile(hookPath, []byte(hook), 0o755))

	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	res := runNoopExecuteBeadForCheckpoint(t, projectRoot, beadID)
	require.NotEqual(t, headBefore, res.BaseRev, "dirty caller worktree should still become the worker base")

	_, statErr := os.Stat(markerPath)
	require.True(t, os.IsNotExist(statErr), "pre-dispatch checkpoint must not invoke parent pre-commit hook")
	assert.Contains(t, runGitInteg(t, projectRoot, "show", "HEAD:caller-dirt.txt"), "dirty")
}

func TestExecuteBead_PreDispatchCheckpointExcludesDDXBackups(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "caller-dirt.txt"), []byte("dirty\n"), 0o644))
	backupRel := filepath.Join(".ddx", "backups", "large.jsonl")
	backupPath := filepath.Join(projectRoot, backupRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(backupPath), 0o755))
	require.NoError(t, os.WriteFile(backupPath, []byte("backup\n"), 0o644))
	runGitInteg(t, projectRoot, "add", "-f", backupRel)

	runNoopExecuteBeadForCheckpoint(t, projectRoot, beadID)

	_, err := runGitIntegOutput(projectRoot, "show", "HEAD:.ddx/backups/large.jsonl")
	require.Error(t, err, "pre-dispatch checkpoint must not include DDx backup artifacts")
}

func TestExecuteBead_PreDispatchCheckpointExcludesExecutionEvidence(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "caller-dirt.txt"), []byte("dirty\n"), 0o644))
	evidenceRel := filepath.Join(".ddx", "executions", "manual-attempt", "manifest.json")
	evidencePath := filepath.Join(projectRoot, evidenceRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(evidencePath), 0o755))
	require.NoError(t, os.WriteFile(evidencePath, []byte(`{"attempt":"manual"}`+"\n"), 0o644))

	runNoopExecuteBeadForCheckpoint(t, projectRoot, beadID)

	_, err := runGitIntegOutput(projectRoot, "show", "HEAD:.ddx/executions/manual-attempt/manifest.json")
	require.Error(t, err, "pre-dispatch checkpoint must not include execution evidence artifacts")
}
