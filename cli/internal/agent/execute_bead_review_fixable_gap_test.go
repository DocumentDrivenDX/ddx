package agent

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreCloseReviewFixableGap_PreventsCloseAndSchedulesRepair verifies that a
// non-terminal BLOCK verdict is classified as review_fixable_gap:
//   - CloseWithEvidence is not called (bead remains open and not closed)
//   - A review-fixable-gap event is recorded with the correct result_rev body
//   - PostMergeReviewOutput.RepairContext is non-nil with the expected fields
//   - Status is ExecuteBeadStatusReviewFixableGap
//
// It also covers the one-cycle scheduling limit (AC4): a second call to
// RunPostMergeReview for the same result_rev must not schedule another repair
// cycle; it must fall through to the regular BLOCK path (status = review_block,
// RepairContext = nil).
func TestPreCloseReviewFixableGap_PreventsCloseAndSchedulesRepair(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	const resultRev = "fixable01"
	const blockRationale = "error handling missing in pkg/foo.go:42"
	const implPower = 70

	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       blockRationale,
			RawOutput:       "BLOCK: " + blockRationale,
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			ResultRev:       resultRev,
			Findings: []Finding{
				{Severity: "block", Summary: blockRationale, Location: "pkg/foo.go:42"},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	report := ExecuteBeadReport{
		BeadID:      first.ID,
		Status:      ExecuteBeadStatusSuccess,
		SessionID:   "sess-fixable",
		ResultRev:   resultRev,
		BaseRev:     "fixable00",
		ActualPower: implPower,
	}

	// First review: must yield review_fixable_gap with a repair context.
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead:     *first,
		Report:   report,
		Reviewer: reviewer,
		Store:    store,
		Rcfg:     rcfg,
		Assignee: "worker",
	})

	assert.False(t, out.Approved, "review_fixable_gap must not be approved")
	assert.Equal(t, ExecuteBeadStatusReviewFixableGap, out.Report.Status)
	assert.Nil(t, out.StoreErr)

	// RepairContext must be populated.
	require.NotNil(t, out.RepairContext, "review_fixable_gap must set RepairContext")
	assert.Equal(t, resultRev, out.RepairContext.ResultRev)
	assert.Equal(t, blockRationale, out.RepairContext.ReviewRationale)
	assert.Equal(t, implPower, out.RepairContext.ImplementerActualPower)

	// review-fixable-gap event must be recorded with result_rev in the body.
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var fixableEv *bead.BeadEvent
	for i := range events {
		if events[i].Kind == ReviewFixableGapEventKind {
			fixableEv = &events[i]
		}
	}
	require.NotNil(t, fixableEv, "review-fixable-gap event must be recorded")
	assert.Equal(t, "review_fixable_gap", fixableEv.Summary)
	assert.Contains(t, fixableEv.Body, "result_rev="+resultRev)

	// Bead must remain open.
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status, "bead must not be closed on review_fixable_gap")
}

// TestRetryPolicy_ReviewFixableGapStopsAfterOneRepairCycle verifies that a
// second BLOCK verdict on the same result_rev does NOT schedule a second
// repair cycle. Once a review-fixable-gap event exists for a result_rev, the
// repair path is bypassed and RunPostMergeReview returns status=review_block
// with RepairContext=nil so the normal BLOCK triage fires instead.
func TestRetryPolicy_ReviewFixableGapStopsAfterOneRepairCycle(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	const resultRev = "fixable02"
	const blockRationale = "still missing error handling"

	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       blockRationale,
			RawOutput:       "BLOCK: " + blockRationale,
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			ResultRev:       resultRev,
			Findings: []Finding{
				{Severity: "block", Summary: blockRationale, Location: "pkg/foo.go:42"},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	report := ExecuteBeadReport{
		BeadID:      first.ID,
		Status:      ExecuteBeadStatusSuccess,
		SessionID:   "sess-onecycle",
		ResultRev:   resultRev,
		BaseRev:     "fixable01",
		ActualPower: 70,
	}

	input := PostMergeReviewInput{
		Bead:     *first,
		Report:   report,
		Reviewer: reviewer,
		Store:    store,
		Rcfg:     rcfg,
		Assignee: "worker",
	}

	// First call: schedules repair cycle.
	out1 := RunPostMergeReview(context.Background(), input)
	require.NotNil(t, out1.RepairContext, "first review must schedule a repair cycle")
	assert.Equal(t, ExecuteBeadStatusReviewFixableGap, out1.Report.Status)

	// Second call for the same result_rev: must NOT schedule another repair.
	out2 := RunPostMergeReview(context.Background(), input)
	assert.Nil(t, out2.RepairContext, "second review on same result_rev must not schedule another repair")
	assert.Equal(t, ExecuteBeadStatusReviewBlock, out2.Report.Status,
		"second BLOCK on same result_rev must fall through to regular review_block")
	assert.False(t, out2.Approved)
}
