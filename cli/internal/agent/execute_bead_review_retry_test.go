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
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingReviewer always returns the given class as a typed review-error.
func failingReviewer(class string) BeadReviewerFunc {
	return BeadReviewerFunc(func(_ context.Context, _, resultRev string, _ ImplementerRouting) (*ReviewResult, error) {
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
			Source:    "ddx agent execute-loop",
			CreatedAt: time.Now().UTC(),
		}))
	}
}

// TestBoundedReviewRetry_NthFailureEmitsManualRequired covers FEAT-022 §14
// case (a): after N (default 3) review-error events on the same result_rev,
// the loop emits a terminal `review-manual-required` event and parks the
// bead so a subsequent execute-loop iteration does NOT re-execute primary.
func TestBoundedReviewRetry_NthFailureEmitsManualRequired(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "deadbeef"

	// Seed N-1 prior failures (default N=3 → 2 prior events). The loop's
	// reviewer attempt becomes the 3rd, which must trip the terminal event.
	seedReviewErrorEvents(t, store, first.ID, resultRev, evidence.OutcomeReviewProviderEmpty, 2)

	var execCount atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			if beadID == first.ID {
				execCount.Add(1)
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-r1",
				ResultRev: resultRev,
			}, nil
		}),
		Reviewer: failingReviewer(evidence.OutcomeReviewProviderEmpty),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, int32(1), execCount.Load(), "primary should run exactly once on this iteration")

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

	// Subsequent execute-loop iteration must NOT re-execute primary. The
	// bead is parked via SetExecutionCooldown so ReadyExecution skips it.
	_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, int32(1), execCount.Load(),
		"after review-manual-required the executor must not be invoked again")
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
		Source:  "ddx agent execute-loop",
	}))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-fresh",
				ResultRev: newRev,
			}, nil
		}),
		Reviewer: failingReviewer(evidence.OutcomeReviewTransport),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

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
		reviewer := BeadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
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
		reviewer := BeadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
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

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-cfg",
				ResultRev: resultRev,
			}, nil
		}),
		Reviewer: failingReviewer(evidence.OutcomeReviewUnparseable),
	}

	// Override to 1: the very first failure should escalate to terminal.
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", ReviewMaxRetries: 1}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

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
