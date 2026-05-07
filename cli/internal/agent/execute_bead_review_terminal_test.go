package agent

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewTerminalClassifications_BlockWithoutNoProgress verifies that each
// terminal review class (spec_gap, missing_acceptance, too_large,
// unsafe_or_out_of_scope) is correctly classified as a terminal operator-
// required outcome. Key assertions:
//   - Report status is ExecuteBeadStatusReviewTerminalBlock.
//   - Bead carries the needs_human label.
//   - A review-terminal-block event is appended with the correct class.
//   - isValidImplementationAttempt returns false → no no-progress budget consumed.
//   - Bead is NOT closed.
func TestReviewTerminalClassifications_BlockWithoutNoProgress(t *testing.T) {
	classes := []struct {
		name  string
		class string
	}{
		{"spec_gap", ReviewTerminalClassSpecGap},
		{"missing_acceptance", ReviewTerminalClassMissingAcceptance},
		{"too_large", ReviewTerminalClassTooLarge},
		{"unsafe_or_out_of_scope", ReviewTerminalClassUnsafeOrOutScope},
	}
	for _, tc := range classes {
		t.Run(tc.name, func(t *testing.T) {
			store, first, _ := newExecuteLoopTestStore(t)
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
					return ExecuteBeadReport{
						BeadID:    beadID,
						Status:    ExecuteBeadStatusSuccess,
						SessionID: "sess-terminal-" + tc.name,
						ResultRev: "aabbcc01",
						BaseRev:   "aabbcc00",
					}, nil
				}),
				Reviewer: beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
					return &ReviewResult{
						Verdict:         VerdictBlock,
						Rationale:       "This BLOCK is terminal: " + tc.class,
						RawOutput:       "BLOCK: " + tc.class,
						ReviewerHarness: "claude",
						ReviewerModel:   "claude-opus-4-6",
					}, nil
				}),
			}

			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
			result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
			require.NoError(t, err)

			// Must count as a failure, not a success.
			assert.Equal(t, 0, result.Successes)
			assert.Equal(t, 1, result.Failures)
			assert.Equal(t, ExecuteBeadStatusReviewTerminalBlock, result.LastFailureStatus)

			// Bead must remain open (not closed).
			got, err := store.Get(first.ID)
			require.NoError(t, err)
			assert.Equal(t, bead.StatusInProgress, got.Status, "terminal block must not close the bead")

			// Bead must carry the needs_human label.
			assert.True(t, HasBeadLabel(got.Labels, TriageNeedsHumanLabel),
				"terminal block must add needs_human label (class=%s)", tc.class)

			// review-terminal-block event must be appended with the correct class.
			events, err := store.Events(first.ID)
			require.NoError(t, err)
			foundTerminal := false
			for _, ev := range events {
				if ev.Kind == ReviewTerminalBlockEventKind && ev.Summary == tc.class {
					foundTerminal = true
					assert.Contains(t, ev.Body, "terminal_class="+tc.class)
					assert.Contains(t, ev.Body, "result_rev=aabbcc01")
				}
			}
			assert.True(t, foundTerminal,
				"expected review-terminal-block event with class %s", tc.class)

			// isValidImplementationAttempt must return false → no no-progress budget.
			assert.False(t, isValidImplementationAttempt(ExecuteBeadReport{
				Status:    ExecuteBeadStatusReviewTerminalBlock,
				BaseRev:   "aabbcc00",
				ResultRev: "aabbcc01",
			}), "review_terminal_block must not consume the no-progress retry budget")
		})
	}
}

// TestReviewTerminalClassifications_ExhaustedReviewErrorNeedsHuman verifies that
// when review-error events exhaust the retry budget, the bead receives the
// needs_human label alongside the terminal review-manual-required event. This
// ensures exhausted review errors park the bead with operator-required metadata
// rather than silently cycling.
func TestReviewTerminalClassifications_ExhaustedReviewErrorNeedsHuman(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "cafebabe"

	// Seed N-1 prior failures (default N=3 → 2 prior events). The next
	// reviewer attempt becomes the 3rd, which must trip the terminal event.
	seedReviewErrorEvents(t, store, first.ID, resultRev, evidence.OutcomeReviewProviderEmpty, 2)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-exhausted-terminal",
				ResultRev: resultRev,
			}, nil
		}),
		Reviewer: failingReviewer(evidence.OutcomeReviewProviderEmpty),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	// review-manual-required event must be present with the correct class.
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var manual *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review-manual-required" {
			manual = &events[i]
		}
	}
	require.NotNil(t, manual, "exhausted review_error must emit a review-manual-required event")
	assert.Contains(t, manual.Body, "failure_class="+evidence.OutcomeReviewProviderEmpty,
		"review-manual-required body must carry failure_class")
	assert.Contains(t, manual.Body, "attempt_count=3",
		"review-manual-required body must carry attempt_count")
	assert.Contains(t, manual.Body, "result_rev="+resultRev,
		"review-manual-required body must carry result_rev")

	// needs_human label must be added on exhausted review error.
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.True(t, HasBeadLabel(got.Labels, TriageNeedsHumanLabel),
		"exhausted review_error must add needs_human label so the bead is surfaced for operator triage")

	// Bead must NOT be closed by reviewer-failure escalation.
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"reviewer failure (even at terminal escalation) must never close the bead")
}
