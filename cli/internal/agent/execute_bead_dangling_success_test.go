package agent

// execute_bead_dangling_success_test.go — regression tests for ddx-2b2d114e.
//
// AC1: success-outcome propagation is recoverable on retry; not silent no_evidence.
// AC2: regression test simulates a finalize failure and asserts the bead lands
//      cleanly on retry.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// writeResultJSON writes a minimal result.json for a given bead/attempt.
func writeResultJSON(t *testing.T, projectRoot, attemptID string, res ExecuteBeadResult) {
	t.Helper()
	dir := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID)
	require.NoError(t, os.MkdirAll(dir, 0755))
	data, err := json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), data, 0644))
}

// mustGitRevParse runs git rev-parse <ref> in dir and fatals on error.
func mustGitRevParse(t *testing.T, dir, ref string) string {
	t.Helper()
	sha, err := gitRevParse(t, dir, ref)
	require.NoError(t, err, "git rev-parse %s in %s", ref, dir)
	return sha
}

// gitCommitFile creates a file, stages it, and commits, returning the new SHA.
func gitCommitFile(t *testing.T, dir, filename, content, message string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644))
	runGitInteg(t, dir, "add", filename)
	runGitInteg(t, dir, "commit", "-m", message)
	return mustGitRevParse(t, dir, "HEAD")
}

// gitDetachedBranchCommit creates a commit on a side branch and checks out
// main again so the commit remains present but unreachable from HEAD.
func gitDetachedBranchCommit(t *testing.T, dir, branch, filename, content, message string) (baseSHA, resultSHA string) {
	t.Helper()
	baseSHA = mustGitRevParse(t, dir, "HEAD")
	runGitInteg(t, dir, "checkout", "-b", branch)
	resultSHA = gitCommitFile(t, dir, filename, content, message)
	runGitInteg(t, dir, "checkout", "main")
	return baseSHA, resultSHA
}

// ---------------------------------------------------------------------------
// Unit tests for latestTaskSucceededResult
// ---------------------------------------------------------------------------

func TestLatestTaskSucceededResult_ReturnsNilWhenNoExecutions(t *testing.T) {
	root := t.TempDir()
	result := latestTaskSucceededResult(root, "ddx-test")
	assert.Nil(t, result, "should return nil when executions dir does not exist")
}

func TestLatestTaskSucceededResult_ReturnsNilWhenNoMatch(t *testing.T) {
	root := t.TempDir()
	// Write a result.json for a DIFFERENT bead.
	writeResultJSON(t, root, "20260508T000001-aabbccdd", ExecuteBeadResult{
		BeadID:    "ddx-other",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: "abc123",
		BaseRev:   "def456",
	})
	result := latestTaskSucceededResult(root, "ddx-test")
	assert.Nil(t, result)
}

func TestLatestTaskSucceededResult_ReturnsNilWhenOutcomeIsNotSucceeded(t *testing.T) {
	root := t.TempDir()
	writeResultJSON(t, root, "20260508T000001-aabbccdd", ExecuteBeadResult{
		BeadID:    "ddx-test",
		Outcome:   ExecuteBeadOutcomeTaskNoChanges,
		ResultRev: "abc123",
		BaseRev:   "def456",
	})
	result := latestTaskSucceededResult(root, "ddx-test")
	assert.Nil(t, result)
}

func TestLatestTaskSucceededResult_ReturnsNilWhenResultRevEqualsBase(t *testing.T) {
	root := t.TempDir()
	writeResultJSON(t, root, "20260508T000001-aabbccdd", ExecuteBeadResult{
		BeadID:    "ddx-test",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: "abc123",
		BaseRev:   "abc123",
	})
	result := latestTaskSucceededResult(root, "ddx-test")
	assert.Nil(t, result)
}

func TestLatestTaskSucceededResult_ReturnsMatchingResult(t *testing.T) {
	root := t.TempDir()
	writeResultJSON(t, root, "20260508T100001-aabbccdd", ExecuteBeadResult{
		BeadID:    "ddx-test",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: "merged123",
		BaseRev:   "base000",
		SessionID: "sess-abc",
	})
	result := latestTaskSucceededResult(root, "ddx-test")
	require.NotNil(t, result)
	assert.Equal(t, "ddx-test", result.BeadID)
	assert.Equal(t, "merged123", result.ResultRev)
	assert.Equal(t, "sess-abc", result.SessionID)
}

func TestLatestTaskSucceededResult_ReturnsMostRecent(t *testing.T) {
	root := t.TempDir()
	// Older attempt.
	writeResultJSON(t, root, "20260508T090000-older", ExecuteBeadResult{
		BeadID:    "ddx-test",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: "old-rev",
		BaseRev:   "base000",
		SessionID: "sess-old",
	})
	// Newer attempt (lexicographically greater directory name).
	writeResultJSON(t, root, "20260508T100000-newer", ExecuteBeadResult{
		BeadID:    "ddx-test",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: "new-rev",
		BaseRev:   "base001",
		SessionID: "sess-new",
	})
	result := latestTaskSucceededResult(root, "ddx-test")
	require.NotNil(t, result)
	assert.Equal(t, "new-rev", result.ResultRev, "should return newest entry")
}

// ---------------------------------------------------------------------------
// Unit tests for recoverDanglingSuccess
// ---------------------------------------------------------------------------

// TestRecoverDanglingSuccess_NopWhenNoPriorSuccess verifies that a claimed
// bead with no prior task_succeeded result is not recovered.
func TestRecoverDanglingSuccess_NopWhenNoPriorSuccess(t *testing.T) {
	root := t.TempDir()
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-test", Title: "test bead", IssueType: "task"}))
	require.NoError(t, store.Claim("ddx-test", "worker"))

	// No result.json at all.
	recovery, err := recoverDanglingSuccess(store, root, "ddx-test", "worker", nil, nil)
	require.NoError(t, err)
	assert.Nil(t, recovery)

	// Bead should still be in_progress.
	b, _ := store.Get("ddx-test")
	assert.Equal(t, bead.StatusInProgress, b.Status)
}

// TestRecoverDanglingSuccess_EmitsEventWhenResultRevUnreachable verifies that
// a missing successful result_rev becomes an operator-visible terminal state
// instead of a silent retry.
func TestRecoverDanglingSuccess_EmitsEventWhenResultRevUnreachable(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Claim(beadID, "worker"))

	// Write a result.json whose result_rev is a fake SHA (not in this repo).
	writeResultJSON(t, projectRoot, "20260508T100000-fake", ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "20260508T100000-fake",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: "deadbeefdeadbeefdeadbeef",
		BaseRev:   "base000",
		SessionID: "sess-fake",
	})

	var eventKind string
	var payload map[string]any
	emit := func(kind string, data map[string]any) {
		eventKind = kind
		payload = data
	}

	recovery, err := recoverDanglingSuccess(store, projectRoot, beadID, "worker", nil, emit)
	require.NoError(t, err)
	require.NotNil(t, recovery)
	assert.Equal(t, danglingSuccessOutcomeOperatorRequired, recovery.Outcome)
	assert.Equal(t, "bead.dangling_success_operator_required", eventKind)
	assert.Equal(t, beadID, payload["bead_id"])
	assert.Equal(t, "20260508T100000-fake", payload["attempt_id"])
	assert.Equal(t, "deadbeefdeadbeefdeadbeef", payload["result_rev"])
	assert.Contains(t, payload["failure_reason"], "not present in the local git object database")

	b, _ := store.Get(beadID)
	assert.Equal(t, bead.StatusProposed, b.Status, "missing successful commit must park the bead instead of retrying")
	meta := bead.GetNeedsHumanMeta(*b)
	assert.Equal(t, "successful result commit could not be recovered automatically", meta.Reason)
	assert.Contains(t, meta.SuggestedAction, "recover or reconstruct result_rev")
}

// ---------------------------------------------------------------------------
// AC2: integration regression test — simulates finalize failure and asserts
// bead closes cleanly on retry.
// ---------------------------------------------------------------------------

// TestDanglingSuccess_AC2_FinalizeFailureThenRetry is the AC2 regression test.
//
// Scenario:
//  1. An agent run completes successfully and is merged into HEAD.
//  2. A result.json with outcome=task_succeeded is written.
//  3. The process "crashes" before CloseWithEvidence runs (simulated by
//     putting the bead in a stale in_progress state without calling Close).
//  4. The next loop iteration picks up the bead (stale heartbeat).
//  5. The loop detects the prior success and closes the bead idempotently
//     WITHOUT re-running the executor.
func TestDanglingSuccess_AC2_FinalizeFailureThenRetry(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	// Step 1: simulate a successful merge — create a commit on main that
	// represents the merged agent result.
	mergedSHA := gitCommitFile(t, projectRoot, "agent-output.txt", "agent result\n",
		"chore: agent work for "+beadID)

	// Step 2: write a result.json reflecting that success.
	const attemptID = "20260508T100000-deadbeef"
	writeResultJSON(t, projectRoot, attemptID, ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: attemptID,
		SessionID: "sess-test",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: mergedSHA,
		BaseRev:   mustGitRevParse(t, projectRoot, mergedSHA+"^"),
		Status:    ExecuteBeadStatusSuccess,
	})

	// Step 3: put the bead in stale in_progress (simulating crash before Close).
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Claim(beadID, "worker"))
	// Remove the external liveness file AND backdate the tracker field so the
	// bead looks stale to both staleness checks.
	require.NoError(t, store.RemoveClaimHeartbeat(beadID))
	require.NoError(t, store.Update(beadID, func(b *bead.Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["work-heartbeat-at"] = time.Now().Add(-2 * bead.HeartbeatTTL).Format(time.RFC3339Nano)
	}))

	// Verify the bead is in_progress before recovery.
	beforeB, _ := store.Get(beadID)
	require.Equal(t, bead.StatusInProgress, beforeB.Status)

	// Step 4: run the loop with Once=true. The executor must NOT be called.
	var execCalled atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			execCalled.Add(1)
			return ExecuteBeadReport{
				BeadID: id,
				Status: ExecuteBeadStatusNoEvidenceProduced,
				Detail: "executor should not have been called",
			}, nil
		}),
	}

	opts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// AC2: bead must be closed.
	afterB, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, afterB.Status,
		"bead must be closed by dangling-success recovery, not re-executed")

	// AC2: executor must not have been called.
	assert.Equal(t, int32(0), execCalled.Load(),
		"executor must not run when prior merged success is detected")

	// AC2: loop must record a success.
	assert.Equal(t, 1, result.Successes, "dangling-success recovery counts as a success")
	assert.Equal(t, 0, result.Failures)
}

func TestExecuteBeadWorker_UnmergedTaskSucceededResultDoesNotRetry(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	baseSHA, resultSHA := gitDetachedBranchCommit(
		t,
		projectRoot,
		"worker-result",
		"worker-output.txt",
		"detached result\n",
		"chore: detached worker result for "+beadID,
	)

	const attemptID = "20260515T223000-unmerged"
	writeResultJSON(t, projectRoot, attemptID, ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: attemptID,
		SessionID: "sess-unmerged",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: resultSHA,
		BaseRev:   baseSHA,
		Status:    ExecuteBeadStatusSuccess,
	})

	frozen := time.Date(2026, 5, 15, 23, 59, 0, 0, time.UTC)
	prevNow := NowFunc
	NowFunc = func() time.Time { return frozen }
	t.Cleanup(func() { NowFunc = prevNow })

	store := bead.NewStore(ddxDir)
	var execCalled atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			execCalled.Add(1)
			return ExecuteBeadReport{
				BeadID: id,
				Status: ExecuteBeadStatusSuccess,
				Detail: "executor should not have been called",
			}, nil
		}),
	}

	opts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(0), execCalled.Load(), "prior successful result must be reconciled before any retry")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusPreservedNeedsReview, result.LastFailureStatus)

	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)

	preserveRef := PreserveRef(beadID, baseSHA)
	assert.Equal(t, resultSHA, mustGitRevParse(t, projectRoot, preserveRef))

	meta := bead.GetNeedsHumanMeta(*got)
	assert.Equal(t, "successful result commit was preserved for manual landing", meta.Reason)
	assert.Contains(t, meta.SuggestedAction, preserveRef)

	events, err := store.Events(beadID)
	require.NoError(t, err)
	var preservedEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "dangling-success-preserved" {
			preservedEvent = &events[i]
			break
		}
	}
	require.NotNil(t, preservedEvent, "preserved dangling success event must be recorded")
	assert.Contains(t, preservedEvent.Body, "attempt_id="+attemptID)
	assert.Contains(t, preservedEvent.Body, "result_rev="+resultSHA)
	assert.Contains(t, preservedEvent.Body, "preserve_ref="+preserveRef)
}

// ---------------------------------------------------------------------------
// AC1: a bead with no prior success re-executes normally
// ---------------------------------------------------------------------------

// TestDanglingSuccess_AC1_NormalRetryWhenNoPriorSuccess verifies that a stale
// in_progress bead WITHOUT a prior task_succeeded result is re-executed by the
// loop as normal — dangling-success recovery is a no-op in that case.
func TestDanglingSuccess_AC1_NormalRetryWhenNoPriorSuccess(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	store := bead.NewStore(ddxDir)
	// Directly claim the bead to simulate a stale in_progress state.
	require.NoError(t, store.Claim(beadID, "worker"))
	// Remove external liveness file and backdate tracker field to simulate staleness.
	require.NoError(t, store.RemoveClaimHeartbeat(beadID))
	require.NoError(t, store.Update(beadID, func(b *bead.Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["work-heartbeat-at"] = time.Now().Add(-2 * bead.HeartbeatTTL).Format(time.RFC3339Nano)
	}))

	// No result.json exists — executor MUST be called.
	var execCalled atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			execCalled.Add(1)
			return ExecuteBeadReport{
				BeadID: id,
				Status: ExecuteBeadStatusSuccess,
				Detail: "executor ran successfully",
			}, nil
		}),
	}

	opts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// AC1: executor was called (re-execution happened normally).
	assert.Equal(t, int32(1), execCalled.Load(), "executor must run for a stale in_progress bead with no prior success")
	assert.Equal(t, 1, result.Successes)
}

// ---------------------------------------------------------------------------
// AC3: DetectDanglingSuccessBeads unit test
// ---------------------------------------------------------------------------

// TestDetectDanglingSuccessBeads_FindsInProgressWithPriorSuccess verifies that
// DetectDanglingSuccessBeads correctly identifies beads with a prior
// task_succeeded result whose result_rev is reachable from HEAD.
func TestDetectDanglingSuccessBeads_FindsInProgressWithPriorSuccess(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	// Create a real commit on HEAD (represents the merged result).
	mergedSHA := gitCommitFile(t, projectRoot, "found.txt", "found\n",
		"chore: merged work for "+beadID)

	// Write a result.json reflecting that success.
	writeResultJSON(t, projectRoot, "20260508T100000-testdetect", ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "20260508T100000-testdetect",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: mergedSHA,
		BaseRev:   mustGitRevParse(t, projectRoot, mergedSHA+"^"),
	})

	// Put the bead in in_progress.
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Claim(beadID, "worker"))

	findings, err := DetectDanglingSuccessBeads(projectRoot)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, beadID, findings[0].BeadID)
	assert.Equal(t, mergedSHA, findings[0].ResultRev)
	assert.True(t, findings[0].Reachable,
		"result_rev is reachable from HEAD so Reachable must be true")
}

// TestDetectDanglingSuccessBeads_MarksDanglingUnreachable verifies that
// DetectDanglingSuccessBeads correctly flags a result_rev that is NOT
// reachable from HEAD (Instance A — dangling commit).
func TestDetectDanglingSuccessBeads_MarksDanglingUnreachable(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	// Write a result.json with a fake (unreachable) SHA.
	writeResultJSON(t, projectRoot, "20260508T100000-dangling", ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "20260508T100000-dangling",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: "deadbeefdeadbeefdeadbeef",
		BaseRev:   "base000",
	})

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Claim(beadID, "worker"))

	findings, err := DetectDanglingSuccessBeads(projectRoot)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, beadID, findings[0].BeadID)
	assert.False(t, findings[0].Reachable,
		"fake SHA must not be reachable — Reachable must be false")
}

// TestDetectDanglingSuccessBeads_IgnoresClosedBeads verifies that closed beads
// are not included in findings even when a prior task_succeeded result exists.
func TestDetectDanglingSuccessBeads_IgnoresClosedBeads(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	mergedSHA := gitCommitFile(t, projectRoot, "done.txt", "done\n",
		"chore: done "+beadID)

	writeResultJSON(t, projectRoot, "20260508T100000-closed", ExecuteBeadResult{
		BeadID:    beadID,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: mergedSHA,
		BaseRev:   mustGitRevParse(t, projectRoot, mergedSHA+"^"),
	})

	// Close the bead properly (not in_progress).
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Claim(beadID, "worker"))
	require.NoError(t, store.CloseWithEvidence(beadID, "sess", mergedSHA))

	findings, err := DetectDanglingSuccessBeads(projectRoot)
	require.NoError(t, err)
	assert.Empty(t, findings, "closed beads must not appear in dangling-success findings")
}

func TestDanglingSuccess_DDX5baa6a15SuccessfulOpenBeadRecoveredBeforeRetry(t *testing.T) {
	// Incident model:
	//   - attempt 20260515T222727-7b672c7c produced worker commit dabf77e68
	//   - attempt 20260515T223843-6eb42169 produced worker commit 2776da59a
	//   - the operator later landed the second success with manual merge a14abb332
	//
	// This regression keeps the bead open/worker-ready and verifies the loop
	// closes it from the latest success evidence before any third dispatch.
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	olderBase, olderResult := gitDetachedBranchCommit(
		t,
		projectRoot,
		"incident-attempt-1",
		"attempt-1.txt",
		"first successful detached result\n",
		"chore: model detached worker result for dabf77e68",
	)
	mergedSHA := gitCommitFile(
		t,
		projectRoot,
		"attempt-2.txt",
		"second successful result merged to main\n",
		"chore: stand-in for manual merge a14abb332 of worker result 2776da59a",
	)

	writeResultJSON(t, projectRoot, "20260515T222727-7b672c7c", ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "20260515T222727-7b672c7c",
		SessionID: "sess-ddx-5baa6a15-a1",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: olderResult,
		BaseRev:   olderBase,
		Status:    ExecuteBeadStatusSuccess,
	})
	writeResultJSON(t, projectRoot, "20260515T223843-6eb42169", ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "20260515T223843-6eb42169",
		SessionID: "sess-ddx-5baa6a15-a2",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: mergedSHA,
		BaseRev:   mustGitRevParse(t, projectRoot, mergedSHA+"^"),
		Status:    ExecuteBeadStatusSuccess,
	})

	store := bead.NewStore(ddxDir)
	var execCalled atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			execCalled.Add(1)
			return ExecuteBeadReport{
				BeadID: id,
				Status: ExecuteBeadStatusSuccess,
				Detail: "executor should not have been called",
			}, nil
		}),
	}

	opts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(0), execCalled.Load(), "latest successful open-bead result must be reconciled before any retry")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(beadID)
	require.NoError(t, err)
	var recoveryEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "dangling-success-recovery" {
			recoveryEvent = &events[i]
			break
		}
	}
	require.NotNil(t, recoveryEvent, "merged latest success must emit dangling-success recovery evidence")
	assert.Contains(t, recoveryEvent.Body, "attempt_id=20260515T223843-6eb42169")
	assert.Contains(t, recoveryEvent.Body, "result_rev="+mergedSHA)
}

// ---------------------------------------------------------------------------
// Helper: newExecuteLoopProjectRoot creates a minimal git repo + bead store
// for pure loop tests that don't need the script harness.
// ---------------------------------------------------------------------------

func newExecuteLoopProjectRoot(t *testing.T) (projectRoot string, beadID string) {
	t.Helper()
	root := t.TempDir()
	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "test@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Test")

	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0644))
	runGitInteg(t, root, "add", ".")
	runGitInteg(t, root, "commit", "-m", "chore: seed")

	ddxDir := filepath.Join(root, ddxroot.DirName)
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))

	const id = "ddx-recovery-0001"
	require.NoError(t, store.Create(&bead.Bead{ID: id, Title: "recovery test bead", IssueType: "task"}))
	runGitInteg(t, root, "add", ".ddx/beads.jsonl")
	runGitInteg(t, root, "commit", "-m", "chore: seed beads")

	return root, id
}

// TestDanglingSuccess_RecoverFromResultRevAlreadyMerged is a focused unit test
// for the recoverDanglingSuccess path against a real git repo. It verifies that:
// - when result_rev is reachable from HEAD
// - and the bead is claimed as in_progress
// → recoverDanglingSuccess closes the bead and returns a terminal recovery.
func TestDanglingSuccess_RecoverFromResultRevAlreadyMerged(t *testing.T) {
	projectRoot, beadID := newExecuteLoopProjectRoot(t)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	// Create a commit to act as the merged result.
	mergedSHA := gitCommitFile(t, projectRoot, "result.txt", "work\n",
		fmt.Sprintf("chore: agent work for %s", beadID))

	// Write a result.json showing task_succeeded with that SHA.
	const attemptID = "20260508T120000-recover"
	writeResultJSON(t, projectRoot, attemptID, ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: attemptID,
		SessionID: "sess-recover",
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ResultRev: mergedSHA,
		BaseRev:   mustGitRevParse(t, projectRoot, mergedSHA+"^"),
		Status:    ExecuteBeadStatusSuccess,
	})

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Claim(beadID, "worker"))
	beforeB, _ := store.Get(beadID)
	require.Equal(t, bead.StatusInProgress, beforeB.Status)

	var events []string
	emit := func(kind string, _ map[string]any) { events = append(events, kind) }

	recovery, err := recoverDanglingSuccess(store, projectRoot, beadID, "worker", nil, emit)
	require.NoError(t, err)
	require.NotNil(t, recovery)
	assert.Equal(t, danglingSuccessOutcomeClosed, recovery.Outcome, "result_rev is reachable — must be recovered")

	afterB, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, afterB.Status, "bead must be closed after recovery")
	assert.Contains(t, events, "bead.dangling_success_recovery",
		"recovery event must be emitted")
}
