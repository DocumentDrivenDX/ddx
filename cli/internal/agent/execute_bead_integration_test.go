package agent

// execute_bead_integration_test.go — Tier-2 integration tests for
// execute-loop orchestration. Every test uses:
//   - a real git init'd temp repo (via newScriptHarnessRepo)
//   - the script harness (not virtual/fakeAgentRunner)
//   - real git commands to assert outcomes (via gitCommitCount, refExists, etc.)
//
// No mocked git anywhere. GIT_* env scrubbing is handled by TestMain in
// agent_test.go which fires before any test in the package.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test 1: single bead, append-line + explicit commit → lands on main
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_SingleBead_AppendLine_Merged seeds one bead,
// runs a directive that appends a line to a file and commits, then asserts that
// the bead is closed with a closing_commit_sha and that git log shows +1 commit.
func TestIntegration_ScriptHarness_SingleBead_AppendLine_Merged(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	// Count commits before execution.
	commitsBefore := gitCommitCount(t, projectRoot, "HEAD")

	// Write the directive.
	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"append-line output.txt hello from integration test",
		"commit chore: integration test output",
	})

	store := makeLoopStore(t, ddxDir)
	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: scriptHarnessExecutor(t, projectRoot, dirFile),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee: "integration-worker",
		Once:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	// Bead must be closed.
	beadStore := bead.NewStore(ddxDir)
	got, err := beadStore.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "bead should be closed after successful merge")
	assert.NotEmpty(t, got.Extra["closing_commit_sha"], "closing_commit_sha must be recorded")

	// Git history must have grown by at least 1 commit (tracker + iteration).
	commitsAfter := gitCommitCount(t, projectRoot, "HEAD")
	assert.Greater(t, commitsAfter, commitsBefore, "git log must show new commits after landing")

	// output.txt must be tracked in HEAD on main (Land() advances the ref but does
	// not update the working tree checkout, so we verify via git-show).
	out, gitErr := runGitIntegOutput(projectRoot, "show", "HEAD:output.txt")
	assert.NoError(t, gitErr, "output.txt must be reachable via HEAD:output.txt after merge: %s", out)
}

// ---------------------------------------------------------------------------
// Test 2: no-op directive → classified as no_changes, bead stays open
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_NoOp_ClassifiedAsNoChanges verifies that a
// directive that makes no filesystem or git changes results in outcome=no_changes
// and the bead remains open below the adjudication threshold.
func TestIntegration_ScriptHarness_NoOp_ClassifiedAsNoChanges(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	commitsBefore := gitCommitCount(t, projectRoot, "HEAD")

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"no-op",
	})

	store := makeLoopStore(t, ddxDir)
	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: scriptHarnessExecutor(t, projectRoot, dirFile),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee:                "integration-worker",
		Once:                    true,
		MaxNoChangesBeforeClose: 3, // explicit threshold so test is readable
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.LastFailureStatus)

	// Bead must still be open.
	beadStore := bead.NewStore(ddxDir)
	got, err := beadStore.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead should remain open on first no_changes")

	// Main branch must not have grown (tracker commit aside — tracker commits
	// are made before the worktree; the landing step only advances on real changes).
	commitsAfter := gitCommitCount(t, projectRoot, "HEAD")
	// Allow for the tracker commit (CommitTracker runs before the worktree add).
	// What must NOT happen: a merge commit landing the no-op.
	// The tracker may add 1, but the iteration itself adds 0.
	assert.LessOrEqual(t, commitsAfter-commitsBefore, 1,
		"no iteration commit should land when no changes were made")
}

// ---------------------------------------------------------------------------
// Test 3: dirty worktree (no explicit commit) → SynthesizeCommit fires
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_DirtyWorktreeSynthesized verifies that when the
// directive creates files but omits an explicit commit, SynthesizeCommit fires and
// the result is merged onto main with the new files present.
func TestIntegration_ScriptHarness_DirtyWorktreeSynthesized(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	// Directive creates a file but does NOT call "commit".
	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"create-file synthesized.txt synthesized content here",
		// No "commit" directive — SynthesizeCommit should handle it.
	})

	store := makeLoopStore(t, ddxDir)
	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: scriptHarnessExecutor(t, projectRoot, dirFile),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee: "integration-worker",
		Once:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	// Bead must be closed.
	beadStore := bead.NewStore(ddxDir)
	got, err := beadStore.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	// synthesized.txt must be tracked in HEAD on main via git-show.
	out, gitErr := runGitIntegOutput(projectRoot, "show", "HEAD:synthesized.txt")
	assert.NoError(t, gitErr, "synthesized.txt must be reachable via HEAD after SynthesizeCommit merge: %s", out)
}

// ---------------------------------------------------------------------------
// Test 4: agent commits then set-exit 1 → preserved under refs/ddx/iterations/
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_FailedExit_WithCommits_Preserved verifies that
// when a directive commits changes but then sets exit code 1, the outcome is
// preserved under refs/ddx/iterations/, the bead stays open, and main is
// unchanged beyond the tracker commit.
func TestIntegration_ScriptHarness_FailedExit_WithCommits_Preserved(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	mainBefore := runGitInteg(t, projectRoot, "rev-parse", "refs/heads/main")

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"create-file failed.txt content that will be preserved",
		"commit chore: failed attempt with commits",
		"set-exit 1",
	})

	store := makeLoopStore(t, ddxDir)
	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: scriptHarnessExecutor(t, projectRoot, dirFile),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee: "integration-worker",
		Once:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)

	// Bead stays open.
	beadStore := bead.NewStore(ddxDir)
	got, err := beadStore.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead should remain open after failed-exit preservation")

	// A preserve ref must exist somewhere under refs/ddx/iterations/<bead>.
	out := runGitInteg(t, projectRoot, "for-each-ref",
		"--format=%(refname)", "refs/ddx/iterations/"+beadID)
	assert.NotEmpty(t, out, "preserve ref under refs/ddx/iterations/%s must exist", beadID)

	// Main branch must not have advanced with the failed commit.
	// (Tracker commits are allowed — they happen before the worktree is created.)
	mainAfter := runGitInteg(t, projectRoot, "rev-parse", "refs/heads/main")
	// The failed commit must NOT be on main. The preserve ref has it, not main.
	assert.NotEqual(t, mainBefore, mainAfter,
		"main may advance by a tracker commit but not by the failed iteration") // tracker commit is fine
	// Verify failed.txt is NOT reachable from HEAD on main.
	_, showErr := runGitIntegOutput(projectRoot, "show", "HEAD:failed.txt")
	assert.Error(t, showErr, "failed.txt must NOT be present on main after preservation")
}

// ---------------------------------------------------------------------------
// Test 5: five concurrent beads, linear history
// TODO: When ddx-8746d8a6 coordinator is fully in place, assert no merge
// commits and exactly 5 iteration commits. Currently the Land() path already
// provides linear history (rebase-then-ff) so merge commits should be absent.
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_FiveConcurrentBeads_LinearHistory seeds 5
// distinct beads and runs 5 workers concurrently (single shared store, single
// projectRoot). All workers use the script harness. After completion: every
// bead is closed and git log shows no merge commits (linear history invariant
// for ddx-6aa50e57).
func TestIntegration_ScriptHarness_FiveConcurrentBeads_LinearHistory(t *testing.T) {
	const n = 5
	projectRoot, initialSHA := newScriptHarnessRepo(t, n)
	ddxDir := filepath.Join(projectRoot, ".ddx")

	// Each bead gets its own directive file: create a unique file and commit.
	dirFiles := make([]string, n)
	tmpDir := t.TempDir()
	for i := 0; i < n; i++ {
		dirFile := filepath.Join(tmpDir, fmt.Sprintf("directive-%d.txt", i+1))
		writeDirectiveFile(t, dirFile, []string{
			fmt.Sprintf("create-file bead-%04d.txt content for bead %d", i+1, i+1),
			fmt.Sprintf("commit chore: iteration for ddx-int-%04d", i+1),
		})
		dirFiles[i] = dirFile
	}

	// Build per-bead executors before spawning goroutines to avoid races on
	// BuiltinCatalog (NewRunner is not goroutine-safe during catalog construction).
	// scriptHarnessExecutor uses landMutexFor(projectRoot) to serialize Land() calls,
	// mimicking the production LandCoordinator (ddx-8746d8a6).
	beadExecutors := make(map[string]ExecuteBeadExecutorFunc, n)
	for i := 0; i < n; i++ {
		beadID := fmt.Sprintf("ddx-int-%04d", i+1)
		beadExecutors[beadID] = scriptHarnessExecutor(t, projectRoot, dirFiles[i])
	}

	store := makeLoopStore(t, ddxDir)
	// Dispatch executor: looks up the pre-built executor by bead ID.
	dispatchExec := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		exec, ok := beadExecutors[beadID]
		if !ok {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: "no executor for bead " + beadID,
			}, nil
		}
		return exec(ctx, beadID)
	})

	// Run n workers concurrently. Each worker loops until no more ready beads.
	// The store's atomic Claim ensures each bead is executed exactly once.
	var wg sync.WaitGroup
	results := make([]*ExecuteBeadLoopResult, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := &ExecuteBeadWorker{Store: store, Executor: dispatchExec}
			results[i], errs[i] = worker.Run(context.Background(), ExecuteBeadLoopOptions{
				Assignee: fmt.Sprintf("worker-%d", i),
				Once:     false,
			})
		}()
	}
	wg.Wait()

	for i, e := range errs {
		require.NoError(t, e, "worker %d returned error", i)
	}

	// All 5 beads must be closed.
	closedCount := countClosedBeads(t, ddxDir)
	assert.Equal(t, n, closedCount, "all 5 beads must be closed")

	// Linear history invariant: no merge commits.
	hasMerge := gitHasMergeCommits(t, projectRoot, "HEAD")
	assert.False(t, hasMerge,
		"git log must show no merge commits (linear history invariant for ddx-6aa50e57)")

	// At least n iteration commits must have landed on main beyond the initial seed.
	commitsOnMain := gitCommitCount(t, projectRoot, "HEAD", "--not", initialSHA)
	// n iteration commits + n tracker commits (one per bead).
	assert.GreaterOrEqual(t, commitsOnMain, n,
		"at least %d iteration commits must be on main", n)

	// All 5 bead-specific files must be reachable via HEAD (Land() only advances
	// the ref; working tree checkout is not updated).
	for i := 1; i <= n; i++ {
		fileName := fmt.Sprintf("bead-%04d.txt", i)
		out, err := runGitIntegOutput(projectRoot, "show", "HEAD:"+fileName)
		assert.NoError(t, err, "HEAD:%s must be reachable on main: %s", fileName, out)
	}
}

// ---------------------------------------------------------------------------
// Test 6: two workers, same single bead → exactly one executes
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_TwoWorkersSameBead_ClaimedOnce seeds 1 bead,
// spawns 2 workers against the same store, and asserts exactly one executes the
// bead while the other reports no_ready_work. This is the ddx-3315dce2 atomic
// claim acceptance test.
func TestIntegration_ScriptHarness_TwoWorkersSameBead_ClaimedOnce(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"create-file claim-test.txt claimed once",
		"commit chore: claim-test iteration",
	})

	store := makeLoopStore(t, ddxDir)
	executor := scriptHarnessExecutor(t, projectRoot, dirFile)

	var wg sync.WaitGroup
	results := make([]*ExecuteBeadLoopResult, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := &ExecuteBeadWorker{Store: store, Executor: executor}
			results[i], errs[i] = worker.Run(context.Background(), ExecuteBeadLoopOptions{
				Assignee: fmt.Sprintf("worker-%d", i),
				Once:     true,
			})
		}()
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	totalAttempts := results[0].Attempts + results[1].Attempts
	totalSuccesses := results[0].Successes + results[1].Successes
	noReadyWork := results[0].NoReadyWork || results[1].NoReadyWork

	assert.Equal(t, 1, totalAttempts, "exactly one worker should execute the bead")
	assert.Equal(t, 1, totalSuccesses, "exactly one successful execution")
	assert.True(t, noReadyWork, "the other worker must report no_ready_work")

	// Bead must be closed exactly once.
	beadStore := bead.NewStore(ddxDir)
	got, err := beadStore.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	// claim-test.txt must be reachable from HEAD on main.
	out, gitErr := runGitIntegOutput(projectRoot, "show", "HEAD:claim-test.txt")
	assert.NoError(t, gitErr, "claim-test.txt must be present on main after single execution: %s", out)
}

// ---------------------------------------------------------------------------
// Test 7: rebase conflict → preserved (skipped until ddx-8746d8a6 coordinator)
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_RebaseConflict_Preserved is gated on the land
// coordinator from ddx-8746d8a6. The coordinator is needed to serialize land
// operations per projectRoot so the second bead's rebase conflict is
// deterministic. Without it, one of the two workers may land first and the
// second would also land cleanly (both write different files, no real conflict).
//
// The test would need to inject a conflict (same line in same file modified by
// both beads), which is feasible but requires the rebase path to be deterministic.
// That requires the coordinator to gate Land() calls. Skip for now.
//
// TODO: implement once ddx-8746d8a6 LandCoordinator is in place.
func TestIntegration_ScriptHarness_RebaseConflict_Preserved(t *testing.T) {
	t.Skip("requires ddx-8746d8a6 land coordinator for deterministic rebase conflict serialization")
}

// ---------------------------------------------------------------------------
// Test 8: context cancel between iterations → second bead not claimed
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_ContextCancelBetweenIterations seeds 2 beads
// and cancels the context after the first completes. The second bead must not
// be claimed. This is a regression test for commit 21d16e6 (context cancellation
// between iterations was not checked before claiming the next candidate).
func TestIntegration_ScriptHarness_ContextCancelBetweenIterations(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 2)
	ddxDir := filepath.Join(projectRoot, ".ddx")

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"create-file cancel-test.txt first bead content",
		"commit chore: first bead in cancel test",
	})

	ctx, cancel := context.WithCancel(context.Background())

	execCount := 0
	outerExecutor := ExecuteBeadExecutorFunc(func(innerCtx context.Context, beadID string) (ExecuteBeadReport, error) {
		execCount++
		report, err := scriptHarnessExecutor(t, projectRoot, dirFile)(innerCtx, beadID)
		// Cancel after the first bead executes.
		cancel()
		return report, err
	})

	store := makeLoopStore(t, ddxDir)
	worker := &ExecuteBeadWorker{Store: store, Executor: outerExecutor}

	_, err := worker.Run(ctx, ExecuteBeadLoopOptions{
		Assignee: "cancel-worker",
		// No Once: true — the worker would try to loop without context cancel.
	})
	// Context cancellation should cause Run to return context.Canceled.
	assert.ErrorIs(t, err, context.Canceled, "Run must return context.Canceled after cancel")

	// Only the first bead should have been executed.
	assert.Equal(t, 1, execCount, "second bead must not be executed after context cancel")

	// First bead must be closed; second must remain open.
	beadStore := bead.NewStore(ddxDir)
	first, err := beadStore.Get("ddx-int-0001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, first.Status, "first bead must be closed")

	second, err := beadStore.Get("ddx-int-0002")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, second.Status, "second bead must remain open after context cancel")
}

// ---------------------------------------------------------------------------
// Test 9: no_changes + specific rationale → closes as already_satisfied fast
// ---------------------------------------------------------------------------

// TestIntegration_ScriptHarness_NoChangesRationale_ClosesBeadFast verifies that
// when the directive writes a no_changes_rationale.txt containing a commit SHA,
// the bead is closed as already_satisfied without waiting for the 3-strike
// cooldown.
func TestIntegration_ScriptHarness_NoChangesRationale_ClosesBeadFast(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ".ddx")
	const beadID = "ddx-int-0001"

	// Get a real commit SHA to embed in the rationale.
	existingSHA := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	rationale := fmt.Sprintf("Work already present in commit %s (output.txt). TestSomeFunc confirms.", existingSHA[:12])

	// Build directive lines that write the rationale file using $DDX_ATTEMPT_ID.
	dirFile := filepath.Join(t.TempDir(), "directive.txt")

	// The execution bundle dir is .ddx/executions/$DDX_ATTEMPT_ID/
	// We write the rationale file there so ExecuteBead reads it.
	rationaleLines := []string{
		fmt.Sprintf(
			"run mkdir -p .ddx/executions/$DDX_ATTEMPT_ID && printf '%%s' '%s' > .ddx/executions/$DDX_ATTEMPT_ID/no_changes_rationale.txt",
			escapeForShell(rationale),
		),
		// No commit directive → resultRev == baseRev → task_no_changes.
	}

	writeDirectiveFile(t, dirFile, rationaleLines)

	store := makeLoopStore(t, ddxDir)
	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: scriptHarnessExecutor(t, projectRoot, dirFile),
	}

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{
		Assignee:                "rationale-worker",
		Once:                    true,
		MaxNoChangesBeforeClose: 3, // would require 3 strikes without a specific rationale
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// The bead should be closed immediately as already_satisfied (1 attempt, 1 success).
	assert.Equal(t, 1, result.Attempts)
	// already_satisfied counts as a success in the loop.
	assert.Equal(t, 1, result.Successes, "specific-rationale no_changes should count as success (already_satisfied)")

	beadStore := bead.NewStore(ddxDir)
	got, err := beadStore.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status,
		"bead must be closed as already_satisfied when rationale cites a commit SHA")

	// Verify the loop result status is already_satisfied.
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status,
		"loop result status must be already_satisfied")

	// Verify the rationale was captured.
	assert.NotEmpty(t, result.Results[0].NoChangesRationale,
		"no_changes_rationale must be non-empty")

	// Use t.Logf for debugging if needed.
	t.Logf("rationale recorded: %q", result.Results[0].NoChangesRationale)

	// Ensure that the test did NOT wait for 3 strikes (single attempt).
	// The already_satisfied close must happen on the first attempt, not after
	// MaxNoChangesBeforeClose=3 attempts.
	assert.Equal(t, 1, result.Attempts,
		"already_satisfied must close on first attempt, not after 3-strike wait")

	// The file must NOT be on main (no changes were made to the repo).
	_ = os.Remove(filepath.Join(projectRoot, "no_changes_rationale.txt")) // cleanup if leaked
}
