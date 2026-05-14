package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingReviewer always returns the given class as a typed review-error.
func failingReviewer(class string) beadReviewerFunc {
	return beadReviewerFunc(func(_ context.Context, _, resultRev string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
				Verdict:   VerdictBlock,
				Error:     class,
				ResultRev: resultRev,
			}, fmt.Errorf("reviewer: %s: %w", class,
				errors.New("simulated failure"))
	})
}

// seedReviewErrorEvents pre-populates a bead with N existing review-error
// events scoped to the given result_rev, simulating prior failed reviewer
// attempts. Used to drive the retry counter to its threshold without running
// the loop N-1 extra times.
func seedReviewErrorEvents(t *testing.T, store *bead.Store, beadID, resultRev, class string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		require.NoError(t, store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "review-error",
			Summary:   class,
			Body:      ReviewErrorEventBody(class, i+1, resultRev, "prior failure"),
			Actor:     "worker",
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		}))
	}
}

func runPostMergeReviewRetryFixture(t *testing.T, store *bead.Store, first *bead.Bead, resultRev string, reviewer BeadReviewer, rcfg config.ResolvedConfig) PostMergeReviewOutput {
	t.Helper()
	return RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-review-retry",
			ResultRev: resultRev,
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})
}

// TestBoundedReviewRetry_NthFailureEmitsManualRequired covers FEAT-022 §14
// case (a) for the retained legacy/manual helper: after N (default 3)
// review-error events on the same result_rev, the helper emits a terminal
// `review-manual-required` event and parks the bead. work no longer
// invokes this helper after a candidate has landed.
func TestBoundedReviewRetry_NthFailureEmitsManualRequired(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "deadbeef"

	// Seed N-1 prior failures (default N=3 → 2 prior events). The loop's
	// reviewer attempt becomes the 3rd, which must trip the terminal event.
	seedReviewErrorEvents(t, store, first.ID, resultRev, evidence.OutcomeReviewProviderEmpty, 2)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := runPostMergeReviewRetryFixture(t, store, first, resultRev, failingReviewer(evidence.OutcomeReviewProviderEmpty), rcfg)
	require.False(t, out.Approved)

	events, err := store.Events(first.ID)
	require.NoError(t, err)

	var manual *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review-manual-required" {
			manual = &events[i]
			break
		}
	}
	require.NotNil(t, manual, "Nth review failure must emit a terminal review-manual-required event")
	assert.Contains(t, manual.Body, "failure_class="+evidence.OutcomeReviewProviderEmpty,
		"manual-required body must include failure_class for triage")
	assert.Contains(t, manual.Body, "attempt_count=3",
		"manual-required body must include attempt_count for triage")
	assert.Contains(t, manual.Body, "result_rev="+resultRev,
		"manual-required body must include the blocked result_rev for triage")

	// §13 invariant: bead must NOT be closed by reviewer-failure escalation.
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"reviewer failure (even at terminal escalation) must never close the bead")
}

// TestBoundedReviewRetry_FreshResultRevResetsCounter covers FEAT-022 §14
// case (b): a new result_rev is a fresh review scope; the counter starts
// from zero even when the prior result_rev exhausted its budget.
func TestBoundedReviewRetry_FreshResultRevResetsCounter(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const oldRev = "0000aaaa"
	const newRev = "1111bbbb"

	// The previous result_rev exhausted its budget — N review-error events
	// plus a review-manual-required event are present.
	seedReviewErrorEvents(t, store, first.ID, oldRev, evidence.OutcomeReviewTransport, 3)
	require.NoError(t, store.AppendEvent(first.ID, bead.BeadEvent{
		Kind:    "review-manual-required",
		Summary: evidence.OutcomeReviewTransport,
		Body:    ReviewErrorEventBody(evidence.OutcomeReviewTransport, 3, oldRev, "exhausted"),
		Actor:   "worker",
		Source:  "ddx work",
	}))

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := runPostMergeReviewRetryFixture(t, store, first, newRev, failingReviewer(evidence.OutcomeReviewTransport), rcfg)
	require.False(t, out.Approved)

	events, err := store.Events(first.ID)
	require.NoError(t, err)

	// New scope: a single review-error event for the new result_rev with
	// attempt_count=1. NO additional review-manual-required event (the
	// original belongs to the old scope and must not be retriggered).
	var freshErr *bead.BeadEvent
	manualForNewRev := 0
	for i := range events {
		ev := events[i]
		if ev.Kind == "review-error" && strings.Contains(ev.Body, "result_rev="+newRev) {
			freshErr = &events[i]
		}
		if ev.Kind == "review-manual-required" && strings.Contains(ev.Body, "result_rev="+newRev) {
			manualForNewRev++
		}
	}
	require.NotNil(t, freshErr, "expected a fresh review-error event scoped to the new result_rev")
	assert.Contains(t, freshErr.Body, "attempt_count=1",
		"a new result_rev must reset the retry counter to 1")
	assert.Equal(t, 0, manualForNewRev,
		"a single failure under a fresh result_rev must not emit review-manual-required")
}

// TestBoundedReviewRetry_NoResultRevDoesNotConsumeBudget covers FEAT-022 §14
// case (c): iterations that do NOT produce a committed result_rev — execution
// failures and --no-merge runs (PreserveRef set, no merged ResultRev) — do
// not invoke the reviewer and therefore do not consume the retry budget.
func TestBoundedReviewRetry_NoResultRevDoesNotConsumeBudget(t *testing.T) {
	t.Run("execution_failed", func(t *testing.T) {
		store, first, _ := newExecuteLoopTestStore(t)

		var reviewerCalls atomic.Int32
		reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalls.Add(1)
			return nil, errors.New("reviewer should not be called for execution_failed iterations")
		})

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID: beadID,
					Status: ExecuteBeadStatusExecutionFailed,
					Detail: "agent crashed",
				}, nil
			}),
			Reviewer: reviewer,
		}

		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
		require.NoError(t, err)
		assert.Equal(t, int32(0), reviewerCalls.Load(),
			"reviewer must not be invoked when primary execution failed")

		events, err := store.Events(first.ID)
		require.NoError(t, err)
		for _, ev := range events {
			assert.NotEqual(t, "review-error", ev.Kind,
				"execution_failed iterations must not append review-error events")
			assert.NotEqual(t, "review-manual-required", ev.Kind,
				"execution_failed iterations must not consume the retry budget")
		}
	})

	t.Run("no_merge_preserved", func(t *testing.T) {
		store, first, _ := newExecuteLoopTestStore(t)

		var reviewerCalls atomic.Int32
		reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
			reviewerCalls.Add(1)
			return nil, errors.New("reviewer should not be called for --no-merge iterations")
		})

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
				// --no-merge surrogate: a preserved iteration with no
				// committed result_rev. The loop must not invoke the
				// reviewer and therefore must not append review-error
				// events that would consume the per-result_rev budget.
				return ExecuteBeadReport{
					BeadID:      beadID,
					Status:      ExecuteBeadStatusExecutionFailed,
					PreserveRef: "refs/ddx/iterations/" + beadID + "/attempt-1",
					Detail:      "preserved (--no-merge)",
				}, nil
			}),
			Reviewer: reviewer,
		}

		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
		require.NoError(t, err)
		assert.Equal(t, int32(0), reviewerCalls.Load(),
			"reviewer must not be invoked for iterations without a committed result_rev")

		events, err := store.Events(first.ID)
		require.NoError(t, err)
		for _, ev := range events {
			assert.NotEqual(t, "review-error", ev.Kind,
				"--no-merge iterations must not append review-error events")
			assert.NotEqual(t, "review-manual-required", ev.Kind,
				"--no-merge iterations must not consume the retry budget")
		}
	})
}

// TestBoundedReviewRetry_RespectsConfiguredOverride asserts that the
// ReviewMaxRetries option (sourced from .ddx/config.yaml's review_max_retries)
// changes the threshold at which review-manual-required is emitted.
func TestBoundedReviewRetry_RespectsConfiguredOverride(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "c0ffee"

	// Override to 1: the very first failure should escalate to terminal.
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", ReviewMaxRetries: 1}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := runPostMergeReviewRetryFixture(t, store, first, resultRev, failingReviewer(evidence.OutcomeReviewUnparseable), rcfg)
	require.False(t, out.Approved)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	manual := 0
	for _, ev := range events {
		if ev.Kind == "review-manual-required" {
			manual++
		}
	}
	assert.Equal(t, 1, manual,
		"with ReviewMaxRetries=1, the first reviewer failure must escalate immediately")
}

func TestTwoSlotReview_PerSlotRetryBudget(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "slot-retry-rev"
	require.NoError(t, store.AppendEvent(first.ID, bead.BeadEvent{
		Kind:    "review-error",
		Summary: evidence.OutcomeReviewProviderEmpty,
		Body:    ReviewErrorEventBodyForSlot(evidence.OutcomeReviewProviderEmpty, 1, resultRev, 0, "prior slot 0 failure"),
		Actor:   "worker",
		Source:  "ddx work",
	}))

	reviewer := beadReviewGroupFunc(func(_ context.Context, beadID, gotRev string, _ ImplementerRouting) (*ReviewGroupResult, error) {
		slot0 := &ReviewResult{
			Verdict:         VerdictApprove,
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			ResultRev:       gotRev,
			PerAC: []ReviewAC{
				{Number: 1, Item: "AC#1", Evidence: "slot 0 approved"},
			},
		}
		slot1 := &ReviewResult{
			Verdict:         VerdictBlock,
			Error:           evidence.OutcomeReviewProviderEmpty,
			Rationale:       "empty reviewer output",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			ResultRev:       gotRev,
		}
		return &ReviewGroupResult{
			BeadID:    beadID,
			ResultRev: gotRev,
			Bundle:    ReviewGroupBundle{GroupID: "review-group-1"},
			Slots: []ReviewGroupSlotResult{
				{ReviewerIndex: 0, Result: slot0},
				{ReviewerIndex: 1, Result: slot1, Error: "provider_empty"},
			},
		}, fmt.Errorf("review-group slot 1: %s", evidence.OutcomeReviewProviderEmpty)
	})

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-slot-retry",
			ResultRev: resultRev,
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg: config.NewTestConfigForLoop(config.TestLoopConfigOpts{
			Assignee:         "worker",
			ReviewMaxRetries: 2,
		}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{
			Assignee:         "worker",
			ReviewMaxRetries: 2,
		})),
		Assignee: "worker",
		Now:      time.Now,
	})

	require.False(t, out.Approved)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, out.Report.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var slot1Error *bead.BeadEvent
	manualForSlot1 := 0
	for i := range events {
		ev := events[i]
		if ev.Kind == "review-error" && strings.Contains(ev.Body, "reviewer_index=1") {
			slot1Error = &events[i]
		}
		if ev.Kind == "review-manual-required" && strings.Contains(ev.Body, "reviewer_index=1") {
			manualForSlot1++
		}
	}
	require.NotNil(t, slot1Error, "slot 1 failure should emit its own review-error")
	assert.Contains(t, slot1Error.Body, "attempt_count=1")
	assert.Contains(t, slot1Error.Body, "reviewer_index=1")
	assert.Equal(t, 0, manualForSlot1, "slot 0 retry history must not exhaust slot 1")
}

// TestPreCloseReview_CostCapFeedsReviewError verifies AC 1 + AC 3: when the
// drain-level cost cap is already exhausted before the pre-close review is
// dispatched, RunPostMergeReview must:
//   - NOT close the bead (Approved=false)
//   - append a review-error event with failure_class=cost_cap_exceeded
//   - classify it through the review_max_retries retry/exhaustion path
func TestPreCloseReview_CostCapFeedsReviewError(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "costcap-rev"

	// Build a cost cap tracker already past its limit so Tripped() is true.
	capTracker := escalation.NewCostCapTracker(0.50, nil)
	capTracker.Add("claude", 1.00) // spent $1.00 > max $0.50 → tripped

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// The reviewer should NOT be called — the cap check must short-circuit.
	var reviewerCalled bool
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		reviewerCalled = true
		return nil, errors.New("reviewer must not be called when cap is tripped")
	})

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-costcap",
			ResultRev: resultRev,
		},
		Reviewer:      reviewer,
		Store:         store,
		ProjectRoot:   t.TempDir(),
		Rcfg:          rcfg,
		ReviewCostCap: capTracker,
		Assignee:      "worker",
		Now:           time.Now,
	})

	require.False(t, out.Approved, "bead must not be closed when cost cap is exceeded")
	assert.False(t, reviewerCalled, "reviewer must not be dispatched when cost cap is already tripped")

	events, err := store.Events(first.ID)
	require.NoError(t, err)

	var reviewErrEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review-error" {
			reviewErrEvent = &events[i]
			break
		}
	}
	require.NotNil(t, reviewErrEvent,
		"cost cap exceeded before dispatch must append a review-error event")
	assert.Contains(t, reviewErrEvent.Body, "failure_class="+evidence.OutcomeReviewCostCapExceeded,
		"review-error body must carry failure_class=cost_cap_exceeded")
	assert.Contains(t, reviewErrEvent.Body, "result_rev="+resultRev,
		"review-error body must carry the blocked result_rev")

	// AC 3: the bead must remain open (not closed by the cap refusal).
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"cost cap exceeded must not close the bead")
}
