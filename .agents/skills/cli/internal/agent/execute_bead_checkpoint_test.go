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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	res, err := ExecuteBead(context.Background(), projectRoot, beadID, ExecuteBeadOptions{
		Harness:     "script",
		Model:       dirFile,
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
	_, err := ExecuteBead(context.Background(), projectRoot, beadID, ExecuteBeadOptions{
		Harness:     "script",
		Model:       dirFile,
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
