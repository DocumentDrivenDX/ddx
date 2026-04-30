package agent

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// orchTestGitOps is a minimal OrchestratorGitOps mock for orchestrator unit tests.
// After the land-coordinator redesign, OrchestratorGitOps only needs UpdateRef.
type orchTestGitOps struct {
	mu          sync.Mutex
	preserveRef string
	preserveSHA string
}

func (m *orchTestGitOps) UpdateRef(dir, ref, sha string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preserveRef = ref
	m.preserveSHA = sha
	return nil
}

// fakeLandingAdvancer returns a LandingAdvancer callback that records each
// invocation and returns the given result/err. Used by orchestrator tests to
// simulate the coordinator-pattern Land() call without spinning up a real
// coordinator goroutine.
type fakeLandingAdvancer struct {
	mu         sync.Mutex
	calls      int
	lastResult *ExecuteBeadResult
	returnLand *LandResult
	returnErr  error
}

func (f *fakeLandingAdvancer) advance(res *ExecuteBeadResult) (*LandResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastResult = res
	return f.returnLand, f.returnErr
}

func makeWorkerResult(beadID, baseRev, resultRev string, exitCode int) *ExecuteBeadResult {
	outcome := ExecuteBeadOutcomeTaskSucceeded
	if exitCode != 0 {
		outcome = ExecuteBeadOutcomeTaskFailed
	}
	if resultRev == baseRev {
		outcome = ExecuteBeadOutcomeTaskNoChanges
	}
	return &ExecuteBeadResult{
		BeadID:    beadID,
		BaseRev:   baseRev,
		ResultRev: resultRev,
		ExitCode:  exitCode,
		Outcome:   outcome,
	}
}

// TestLandBeadResult_Merge verifies the default path: agent succeeded with
// commits → LandBeadResult invokes the LandingAdvancer (coordinator Land()
// callback) and classifies the outcome as merged.
func TestLandBeadResult_Merge(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-orch-01", "aaa0001", "bbb0001", 0)
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{
		returnLand: &LandResult{Status: "landed", NewTip: "bbb0001"},
	}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "merged" {
		t.Errorf("expected outcome=merged, got %q", res.Outcome)
	}
	if advancer.calls != 1 {
		t.Errorf("expected 1 advancer call, got %d", advancer.calls)
	}
	if res.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected status=success, got %q", res.Status)
	}
}

// TestLandBeadResult_NoChanges verifies that when resultRev == baseRev the
// landing outcome is "no-changes" and the advancer is not invoked.
func TestLandBeadResult_NoChanges(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-orch-02", "aaa0002", "aaa0002", 0)
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "no-changes" {
		t.Errorf("expected outcome=no-changes, got %q", res.Outcome)
	}
	if advancer.calls != 0 {
		t.Errorf("expected 0 advancer calls, got %d", advancer.calls)
	}
	if res.Status != ExecuteBeadStatusNoChanges {
		t.Errorf("expected status=no_changes, got %q", res.Status)
	}
}

func TestLandBeadResult_NoEvidenceProduced(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-orch-noev", "aaa0010", "aaa0010", 0)
	res.Outcome = ExecuteBeadOutcomeTaskNoEvidence
	res.FailureMode = FailureModeNoEvidenceProduced
	res.Reason = "agent exited without a commit or no_changes_rationale.txt"
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "no-evidence" {
		t.Errorf("expected outcome=no-evidence, got %q", res.Outcome)
	}
	if advancer.calls != 0 {
		t.Errorf("expected 0 advancer calls, got %d", advancer.calls)
	}
	if res.Status != ExecuteBeadStatusNoEvidenceProduced {
		t.Errorf("expected status=no_evidence_produced, got %q", res.Status)
	}
	if res.FailureMode != FailureModeNoEvidenceProduced {
		t.Errorf("expected failure_mode=no_evidence_produced, got %q", res.FailureMode)
	}
}

// TestLandBeadResult_AgentFailedNoCommits verifies that when exitCode != 0 and
// resultRev == baseRev (no commits), the outcome is "error".
func TestLandBeadResult_AgentFailedNoCommits(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-orch-03", "aaa0003", "aaa0003", 1)
	res.Error = "agent crashed"
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "error" {
		t.Errorf("expected outcome=error, got %q", res.Outcome)
	}
	if advancer.calls != 0 {
		t.Errorf("expected 0 advancer calls for error outcome, got %d", advancer.calls)
	}
	if res.Status != ExecuteBeadStatusExecutionFailed {
		t.Errorf("expected status=execution_failed, got %q", res.Status)
	}
}

// TestLandBeadResult_AgentFailedWithCommits verifies that when exitCode != 0 but
// commits were produced, the result is preserved rather than landed or discarded.
func TestLandBeadResult_AgentFailedWithCommits(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-orch-04", "aaa0004", "bbb0004", 1)
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "preserved" {
		t.Errorf("expected outcome=preserved, got %q", res.Outcome)
	}
	if advancer.calls != 0 {
		t.Errorf("expected 0 advancer calls when agent failed, got %d", advancer.calls)
	}
	if orch.preserveRef == "" {
		t.Error("expected a preserve ref when agent failed with commits")
	}
	if res.Status != ExecuteBeadStatusExecutionFailed {
		t.Errorf("expected status=execution_failed, got %q", res.Status)
	}
}

// TestLandBeadResult_NoMerge verifies that --no-merge preserves unconditionally.
func TestLandBeadResult_NoMerge(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-orch-05", "aaa0005", "bbb0005", 0)
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		NoMerge:         true,
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "preserved" {
		t.Errorf("expected outcome=preserved with --no-merge, got %q", res.Outcome)
	}
	if advancer.calls != 0 {
		t.Errorf("expected 0 advancer calls with --no-merge, got %d", advancer.calls)
	}
	if res.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected status=success even when preserved via --no-merge, got %q", res.Status)
	}
}

// TestLandBeadResult_MergeConflictPreserves verifies that when the land
// advancer returns a preserved status (merge conflict) the result is
// preserved rather than landed.
func TestLandBeadResult_MergeConflictPreserves(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-orch-06", "aaa0006", "bbb0006", 0)
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{
		returnLand: &LandResult{
			Status:      "preserved",
			PreserveRef: "refs/ddx/iterations/ddx-orch-06/test-aaa0006",
			Reason:      "merge conflict",
		},
	}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "preserved" {
		t.Errorf("expected outcome=preserved after merge conflict, got %q", res.Outcome)
	}
	if res.Status != ExecuteBeadStatusLandConflict {
		t.Errorf("expected status=land_conflict, got %q", res.Status)
	}
	if res.PreserveRef == "" {
		t.Error("expected a preserve ref after merge conflict")
	}
}

// TestLandBeadResult_PreserveRefFormat verifies that when LandBeadResult
// falls back to preserving (no LandingAdvancer configured, e.g. for the
// interactive single-bead CLI), the generated preserve ref matches the
// documented pattern refs/ddx/iterations/<bead-id>/<ts>-<sha>.
func TestLandBeadResult_PreserveRefFormat(t *testing.T) {
	projectRoot := t.TempDir()
	const beadID = "ddx-orch-07"
	const baseRev = "deadbeef1234"

	oldNow := NowFunc
	NowFunc = func() time.Time { return time.Date(2026, 4, 14, 5, 36, 33, 0, time.UTC) }
	defer func() { NowFunc = oldNow }()

	res := makeWorkerResult(beadID, baseRev, "abcd1234abcd", 0)
	orch := &orchTestGitOps{}

	// No LandingAdvancer → LandBeadResult preserves as a safe fallback.
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}

	wantRef := "refs/ddx/iterations/ddx-orch-07/20260414T053633Z-deadbeef1234"
	if landing.PreserveRef != wantRef {
		t.Errorf("preserve ref = %q, want %q", landing.PreserveRef, wantRef)
	}
}

// TestConcurrentWorkersNoMergeRace is a regression test verifying that two
// concurrent workers each complete independently without producing merge races.
// After the land-coordinator redesign the actual land step is serialized by
// the coordinator goroutine, not by LandBeadResult itself; this test just
// asserts that LandBeadResult is safe to call concurrently with
// independent per-worker advancers.
func TestConcurrentWorkersNoMergeRace(t *testing.T) {
	projectRoot := t.TempDir()

	type call struct {
		rev string
		seq int
	}
	var mu sync.Mutex
	var landCalls []call
	callCount := 0

	const numWorkers = 2
	results := make([]*ExecuteBeadResult, numWorkers)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			beadID := fmt.Sprintf("ddx-concurrent-%02d", i)
			baseRev := fmt.Sprintf("base%08d", i)
			resultRev := fmt.Sprintf("result%06d", i)

			res := makeWorkerResult(beadID, baseRev, resultRev, 0)
			orch := &orchTestGitOps{}
			advancer := &fakeLandingAdvancer{
				returnLand: &LandResult{Status: "landed", NewTip: resultRev},
			}
			landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
				LandingAdvancer: advancer.advance,
			})
			if err != nil {
				t.Errorf("worker %d: LandBeadResult: %v", i, err)
				return
			}
			ApplyLandingToResult(res, landing)
			mu.Lock()
			results[i] = res
			landCalls = append(landCalls, call{rev: resultRev, seq: callCount})
			callCount++
			mu.Unlock()
		}()
	}

	wg.Wait()

	// All workers must complete with either merged or preserved (no panics, no deadlocks).
	for i, res := range results {
		if res == nil {
			t.Errorf("worker %d: result is nil", i)
			continue
		}
		if res.Outcome != "merged" && res.Outcome != "preserved" {
			t.Errorf("worker %d: unexpected outcome %q", i, res.Outcome)
		}
	}
	// Both workers must have landed.
	if len(landCalls) != numWorkers {
		t.Errorf("expected %d landing calls, got %d", numWorkers, len(landCalls))
	}
}
