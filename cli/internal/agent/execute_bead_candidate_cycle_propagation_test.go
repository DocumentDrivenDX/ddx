package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyWorkerCandidateCycle_ReviewMalfunctionOverridesSuccess proves that
// applyWorkerCandidateCycle projects the candidate-cycle report's outcome
// fields back onto the execute result instead of leaving the worker's
// provisional "success" status in place when the pre-land reviewer returns an
// empty/unparseable result (ddx-0c7d976c).
func TestApplyWorkerCandidateCycle_ReviewMalfunctionOverridesSuccess(t *testing.T) {
	projectRoot, rev := initTestGitRepo(t)
	wtPath := t.TempDir()

	res := &ExecuteBeadResult{
		BeadID:    "ddx-cycle-test",
		AttemptID: "attempt-cycle-001",
		BaseRev:   rev,
		ResultRev: rev,
		Status:    ExecuteBeadStatusSuccess,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		ExitCode:  0,
	}

	runtime := ExecuteBeadRuntime{
		Reviewer: candidateReviewerFunc(func(context.Context, string, CandidateResult) (CandidateReviewResult, error) {
			return CandidateReviewResult{}, errors.New("reviewer returned zero bytes")
		}),
	}

	err := applyWorkerCandidateCycle(context.Background(), projectRoot, wtPath, runtime, res)
	require.NoError(t, err)

	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, res.Status,
		"a review malfunction must overwrite the worker's provisional success status")
	assert.Contains(t, res.Detail, "pre-land review")
	require.Len(t, res.CycleTrace, 1, "the malfunctioning cycle must still be recorded")
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, res.CycleTrace[0].FinalDecision)
	require.NotEmpty(t, res.CandidateRef, "the candidate ref must be retained when review malfunctions")

	got, err := gitRevParse(t, projectRoot, res.CandidateRef)
	require.NoError(t, err)
	assert.Equal(t, rev, got, "the candidate ref must resolve to the unreviewed candidate commit")
}

// reviewMalfunctionReport drives the real applyWorkerCandidateCycle path for
// beadID with a reviewer that returns an empty/unparseable result, then
// converts the resulting ExecuteBeadResult into the loop-facing report shape
// exactly as ExecuteBeadWithConfig does in production.
func reviewMalfunctionReport(t *testing.T, beadID string) ExecuteBeadReport {
	t.Helper()
	projectRoot, rev := initTestGitRepo(t)
	res := &ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "attempt-worker-malfunction",
		BaseRev:   rev,
		ResultRev: rev,
		Status:    ExecuteBeadStatusSuccess,
		SessionID: "sess-malfunction",
	}
	runtime := ExecuteBeadRuntime{
		Reviewer: candidateReviewerFunc(func(context.Context, string, CandidateResult) (CandidateReviewResult, error) {
			return CandidateReviewResult{}, errors.New("reviewer returned zero bytes")
		}),
	}
	require.NoError(t, applyWorkerCandidateCycle(context.Background(), projectRoot, t.TempDir(), runtime, res))
	return ReportFromExecuteBeadResult(res, "")
}

// approvedCandidateReport drives the real applyWorkerCandidateCycle path for
// beadID with a reviewer that approves the candidate, mirroring a healthy
// pre-land review pass.
func approvedCandidateReport(t *testing.T, beadID string) ExecuteBeadReport {
	t.Helper()
	projectRoot, rev := initTestGitRepo(t)
	res := &ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "attempt-worker-approve",
		BaseRev:   rev,
		ResultRev: rev,
		Status:    ExecuteBeadStatusSuccess,
		SessionID: "sess-approve",
	}
	runtime := ExecuteBeadRuntime{
		Reviewer: candidateReviewerFunc(func(context.Context, string, CandidateResult) (CandidateReviewResult, error) {
			return CandidateReviewResult{
				Verdict:          "APPROVE",
				Rationale:        "looks good",
				ReviewGroupID:    "rg-approve-1",
				ReviewerIndices:  []int{0},
				ReviewerVerdicts: []string{"APPROVE"},
			}, nil
		}),
	}
	require.NoError(t, applyWorkerCandidateCycle(context.Background(), projectRoot, t.TempDir(), runtime, res))
	return ReportFromExecuteBeadResult(res, "")
}

// findBodyLine returns the first line of body starting with prefix, failing
// the test if none is found.
func findBodyLine(t *testing.T, body, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("no line with prefix %q in body: %s", prefix, body)
	return ""
}

// TestExecuteBeadWorker_EmptyCandidateReviewDoesNotLandOrClose exercises the
// real ExecuteBeadWorker loop with a candidate-cycle report produced by an
// empty/unparseable reviewer result, proving the malfunctioning candidate is
// never closed and never recorded as landed evidence (ddx-0c7d976c).
func TestExecuteBeadWorker_EmptyCandidateReviewDoesNotLandOrClose(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return reviewMalfunctionReport(t, beadID), nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Successes, "a malfunctioning review must not count as a success")
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, result.LastFailureStatus)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"the bead must not be closed when the candidate review malfunctioned")
	assert.Empty(t, got.Extra["closing_commit_sha"],
		"the unreviewed candidate commit must never be recorded as landed evidence")
}

// TestExecuteBeadWorker_ReviewMalfunctionAuditIsConsistent proves that the
// durable execute-bead event's cycle_trace and decision_audit agree: both
// report the malfunction/retry/not_landed disposition, never a contradictory
// close/landed tuple (ddx-0c7d976c).
func TestExecuteBeadWorker_ReviewMalfunctionAuditIsConsistent(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return reviewMalfunctionReport(t, beadID), nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	events, err := store.Events(first.ID)
	require.NoError(t, err)

	var executeEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execute-bead" {
			executeEvent = &events[i]
		}
	}
	require.NotNil(t, executeEvent, "execute-bead event must be recorded")
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, executeEvent.Summary)

	cycleTraceLine := findBodyLine(t, executeEvent.Body, "cycle_trace=")
	var trace []ExecutionCycleTrace
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(cycleTraceLine, "cycle_trace=")), &trace))
	require.Len(t, trace, 1)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, trace[0].FinalDecision)
	assert.Equal(t, "not_landed", trace[0].LandStatus)
	assert.Equal(t, "retry", trace[0].RetryAction)

	auditLine := findBodyLine(t, executeEvent.Body, "decision_audit=")
	var audit map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(auditLine, "decision_audit=")), &audit))
	assert.Equal(t, "not_landed", audit["land_status"],
		"the final decision audit must not contradict the cycle trace by claiming the candidate landed")
	assert.Equal(t, "retry", audit["retry_action"])
}

// TestExecuteBeadWorker_ApprovedCandidateStillLandsAndCloses proves that an
// explicit APPROVE verdict from the pre-land candidate-cycle reviewer still
// closes the bead normally, so fixing the malfunction-propagation bug does
// not regress the existing successful-landing path (ddx-0c7d976c).
func TestExecuteBeadWorker_ApprovedCandidateStillLandsAndCloses(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return approvedCandidateReport(t, beadID), nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "an approved candidate must still close normally")
	assert.Equal(t, "sess-approve", got.Extra["session_id"])
}
