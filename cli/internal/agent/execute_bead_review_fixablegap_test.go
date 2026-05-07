package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreCloseReviewFixableGap_PreventsCloseAndSchedulesRepair verifies that a
// review_fixable_gap verdict (REQUEST_CHANGES or non-terminal BLOCK):
//   - does not close the bead
//   - appends a review-fixable-gap event carrying result_rev, impl_actual_power,
//     and rationale
//   - sets RepairContext on PostMergeReviewOutput
//   - enforces a one-cycle limit: a second call with the same result_rev does
//     not schedule another repair (RepairContext is nil)
func TestPreCloseReviewFixableGap_PreventsCloseAndSchedulesRepair(t *testing.T) {
	type tc struct {
		name    string
		verdict Verdict
	}
	cases := []tc{
		{"request_changes", VerdictRequestChanges},
		{"non_terminal_block", VerdictBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, first, _ := newExecuteLoopTestStore(t)
			const resultRev = "fixrev001a"
			const implActualPower = 50
			fixableRationale := "assertion at foo_test.go:42 fails"

			makeReviewer := func() beadReviewerFunc {
				var rationale string
				if tc.verdict == VerdictBlock {
					// non-terminal block: rationale must not contain terminal class labels
					rationale = "fixable_issue: " + fixableRationale
				} else {
					rationale = fixableRationale
				}
				return beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
					return &ReviewResult{
						Verdict:         tc.verdict,
						Rationale:       rationale,
						ReviewerHarness: "claude",
						ReviewerModel:   "claude-opus-4-6",
					}, nil
				})
			}

			makeIn := func() PostMergeReviewInput {
				cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
				rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
				return PostMergeReviewInput{
					Bead: *first,
					Report: ExecuteBeadReport{
						BeadID:      first.ID,
						Status:      ExecuteBeadStatusSuccess,
						SessionID:   "sess-fixable-" + tc.name,
						ResultRev:   resultRev,
						ActualPower: implActualPower,
					},
					Reviewer:    makeReviewer(),
					Store:       store,
					ProjectRoot: t.TempDir(),
					Rcfg:        rcfg,
					Now:         time.Now,
					Assignee:    "worker",
				}
			}

			// --- First call: repair must be scheduled ---
			out := RunPostMergeReview(context.Background(), makeIn())
			require.False(t, out.Approved, "fixable gap must not approve the bead")

			// Bead must not be closed.
			got, err := store.Get(first.ID)
			require.NoError(t, err)
			assert.NotEqual(t, bead.StatusClosed, got.Status, "fixable gap must not close the bead")

			// review-fixable-gap event must be present.
			events, err := store.Events(first.ID)
			require.NoError(t, err)
			var fixableEv *bead.BeadEvent
			for i := range events {
				if events[i].Kind == ReviewFixableGapEventKind {
					fixableEv = &events[i]
				}
			}
			require.NotNil(t, fixableEv, "review-fixable-gap event must be appended")
			assert.Equal(t, "review_fixable_gap", fixableEv.Summary)
			assert.Contains(t, fixableEv.Body, "result_rev="+resultRev,
				"event body must carry result_rev")
			assert.Contains(t, fixableEv.Body, fmt.Sprintf("impl_actual_power=%d", implActualPower),
				"event body must carry impl_actual_power")
			assert.Contains(t, fixableEv.Body, "rationale=",
				"event body must carry reviewer rationale")

			// RepairContext must be populated.
			require.NotNil(t, out.RepairContext,
				"PostMergeReviewOutput.RepairContext must be set on review_fixable_gap")
			assert.Equal(t, resultRev, out.RepairContext.ResultRev,
				"RepairContext.ResultRev must match the attempt's result_rev")
			assert.Equal(t, implActualPower, out.RepairContext.ImplActualPower,
				"RepairContext.ImplActualPower must match the implementer's actual power")

			// --- One-cycle limit: second call with same result_rev must not schedule repair ---
			out2 := RunPostMergeReview(context.Background(), makeIn())
			require.False(t, out2.Approved, "second verdict is still not approved")
			assert.Nil(t, out2.RepairContext,
				"second review_fixable_gap for the same result_rev must not schedule another repair (one-cycle limit)")
		})
	}
}

// TestRetryPolicy_ReviewFixableGapStopsAfterOneRepairCycle verifies that when a
// prior review-fixable-gap event already exists for the same result_rev,
// RunPostMergeReview does not schedule a second repair cycle regardless of the
// review verdict.
func TestRetryPolicy_ReviewFixableGapStopsAfterOneRepairCycle(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "stoprev001"

	// Pre-seed a prior review-fixable-gap event for this result_rev to simulate
	// a repair cycle that was already dispatched.
	require.NoError(t, store.AppendEvent(first.ID, bead.BeadEvent{
		Kind:      ReviewFixableGapEventKind,
		Summary:   "review_fixable_gap",
		Body:      "result_rev=" + resultRev + "\nimpl_actual_power=50",
		Actor:     "worker",
		Source:    "ddx agent execute-loop",
		CreatedAt: time.Now().UTC(),
	}))

	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictRequestChanges,
			Rationale:       "still failing on second pass",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:      first.ID,
			Status:      ExecuteBeadStatusSuccess,
			SessionID:   "sess-stop-1",
			ResultRev:   resultRev,
			ActualPower: 50,
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})

	assert.False(t, out.Approved, "still not approved after the repair cycle limit")
	assert.Nil(t, out.RepairContext,
		"must not schedule a second repair cycle for the same review_group_id/result_rev")
}
