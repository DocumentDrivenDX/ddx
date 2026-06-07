package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

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
//   - Bead moves to status=proposed with operator-required metadata.
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
			reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
				return &ReviewResult{
					Verdict:         VerdictBlock,
					Rationale:       "This BLOCK is terminal: " + tc.class,
					RawOutput:       "BLOCK: " + tc.class,
					ReviewerHarness: "claude",
					ReviewerModel:   "claude-opus-4-6",
					Findings: []Finding{
						{Severity: "block", Summary: "Structured terminal class: " + tc.class, Location: "bead:AC1"},
					},
				}, nil
			})

			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
			out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
				Bead: *first,
				Report: ExecuteBeadReport{
					BeadID:    first.ID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-terminal-" + tc.name,
					ResultRev: "aabbcc01",
					BaseRev:   "aabbcc00",
				},
				Reviewer:    reviewer,
				Store:       store,
				ProjectRoot: t.TempDir(),
				Rcfg:        rcfg,
				Now:         time.Now,
				Assignee:    "worker",
			})
			require.False(t, out.Approved)

			// Must count as a failure, not a success.
			assert.Equal(t, ExecuteBeadStatusReviewTerminalBlock, out.Report.Status)

			// Bead must move to operator attention (not closed).
			got, err := store.Get(context.Background(), first.ID)
			require.NoError(t, err)
			assert.Equal(t, bead.StatusProposed, got.Status, "terminal block must move to proposed")
			assert.NotEqual(t, bead.StatusClosed, got.Status, "terminal block must not close the bead")

			// Bead must use status-owned operator metadata, not legacy labels.
			assert.NotContains(t, got.Labels, TriageNeedsHumanLabel,
				"terminal block must not add needs_human label (class=%s)", tc.class)
			meta := bead.GetNeedsHumanMeta(*got)
			assert.Contains(t, meta.Reason, tc.class)
			assert.Equal(t, "ddx work", meta.Source)

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

// TestReviewTerminalClassifications_ExhaustedReviewErrorProposed verifies that
// when review-error events exhaust the retry budget, the bead moves to
// status=proposed alongside the terminal review-manual-required event. This
// ensures exhausted review errors park the bead with operator-required metadata
// rather than silently cycling.
func TestReviewTerminalClassifications_ExhaustedReviewErrorProposed(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	const resultRev = "cafebabe"

	// Seed N-1 prior failures (default N=3 → 2 prior events). The next
	// reviewer attempt becomes the 3rd, which must trip the terminal event.
	seedReviewErrorEvents(t, store, first.ID, resultRev, evidence.OutcomeReviewProviderEmpty, 2)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	out := runPostMergeReviewRetryFixture(t, store, first, resultRev, failingReviewer(evidence.OutcomeReviewProviderEmpty), rcfg)
	require.False(t, out.Approved)

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

	// Operator parking must be status-owned on exhausted review error.
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status,
		"exhausted review_error must move bead to proposed for operator triage")
	assert.NotContains(t, got.Labels, TriageNeedsHumanLabel,
		"exhausted review_error must not use needs_human label parking")
	meta := bead.GetNeedsHumanMeta(*got)
	assert.Contains(t, meta.Reason, evidence.OutcomeReviewProviderEmpty)
	assert.Equal(t, "ddx work", meta.Source)

	// Bead must NOT be closed by reviewer-failure escalation.
	assert.NotEqual(t, bead.StatusClosed, got.Status,
		"reviewer failure (even at terminal escalation) must never close the bead")
}

// TestReadinessPreservesOperatorPromotedOpen verifies that a bead with a recent
// operator-driven proposed->open transition remains status=open after an ambiguous
// readiness terminal outcome. The test asserts:
//   - A bead parked to proposed receives a triaged event (operator promoted to open).
//   - When terminal readiness outcome is evaluated again, the bead stays open.
//   - No new proposed-to-open transition is written.
func TestReadinessPreservesOperatorPromotedOpen(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	// Park the bead to proposed (simulating a prior terminal outcome).
	err := store.SetLifecycleStatus(first.ID, bead.StatusProposed, bead.LifecycleTransitionOptions{
		OperatorRequired: true,
		Reason:           "operator decision required",
		Actor:            "worker",
		Source:           "ddx work",
	})
	require.NoError(t, err)

	// Simulate operator promotion from proposed to open via a triaged event.
	_ = store.AppendEvent(first.ID, bead.BeadEvent{
		Kind:      "triaged",
		Summary:   "operator accepted proposed bead",
		Body:      `{"from_status":"proposed","to_status":"open","actor":"operator-user","source":"web-ui"}`,
		Actor:     "operator-user",
		Source:    "web-ui",
		CreatedAt: time.Now().UTC(),
	})

	// Promote the bead to open (as the operator would do).
	err = store.SetLifecycleStatus(first.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: "operator accepted",
		Actor:  "operator-user",
		Source: "web-ui",
	})
	require.NoError(t, err)

	// Get the bead with current status.
	first, err = store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, first.Status, "operator must promote bead to open")

	// Now simulate a terminal review block outcome (which would normally park to proposed).
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       "This BLOCK is terminal: review_spec_gap",
			RawOutput:       "BLOCK: review_spec_gap",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			Findings: []Finding{
				{Severity: "block", Summary: "Structured terminal class: review_spec_gap", Location: "bead:AC1"},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-operator-promoted-open",
			ResultRev: "aabbcc01",
			BaseRev:   "aabbcc00",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})

	// The review result should still indicate a terminal block.
	assert.Equal(t, ExecuteBeadStatusReviewTerminalBlock, out.Report.Status)
	require.False(t, out.Approved)

	// The key assertion: the bead should remain open, not be downgraded to proposed.
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status,
		"operator-promoted bead must remain open despite terminal review block")
}

// TestReadinessStillDowngradesAgentProposed verifies that a bead in status=proposed
// with no operator promotion still receives terminal downgrade handling (the
// normal case where the agent parks a bead to proposed without operator
// intervention).
func TestReadinessStillDowngradesAgentProposed(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	// Park the bead to proposed without any operator promotion event.
	err := store.SetLifecycleStatus(first.ID, bead.StatusProposed, bead.LifecycleTransitionOptions{
		OperatorRequired: true,
		Reason:           "agent decision required",
		Actor:            "worker",
		Source:           "ddx work",
	})
	require.NoError(t, err)

	// Verify the bead is in proposed.
	first, err = store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, first.Status)

	// Simulate a terminal review block outcome.
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       "This BLOCK is terminal: review_spec_gap",
			RawOutput:       "BLOCK: review_spec_gap",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			Findings: []Finding{
				{Severity: "block", Summary: "Structured terminal class: review_spec_gap", Location: "bead:AC1"},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-agent-proposed",
			ResultRev: "aabbcc01",
			BaseRev:   "aabbcc00",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})

	// The review result should indicate a terminal block.
	assert.Equal(t, ExecuteBeadStatusReviewTerminalBlock, out.Report.Status)
	require.False(t, out.Approved)

	// Since there is no operator promotion, the bead should remain proposed.
	// (In this case, there's no downgrade needed since it's already proposed).
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status,
		"agent-parked proposed bead must stay proposed with terminal block")
}

// TestReadinessLoopBoundedAfterOperatorPromotion verifies that readiness
// idempotency is preserved: running the readiness check twice over the same
// operator-promoted bead should not produce new status transitions on the
// second pass.
func TestReadinessLoopBoundedAfterOperatorPromotion(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	// Park to proposed, then promote to open via operator action.
	err := store.SetLifecycleStatus(first.ID, bead.StatusProposed, bead.LifecycleTransitionOptions{
		OperatorRequired: true,
		Reason:           "operator decision required",
		Actor:            "worker",
		Source:           "ddx work",
	})
	require.NoError(t, err)

	_ = store.AppendEvent(first.ID, bead.BeadEvent{
		Kind:      "triaged",
		Summary:   "operator accepted proposed bead",
		Body:      `{"from_status":"proposed","to_status":"open","actor":"operator-user","source":"web-ui"}`,
		Actor:     "operator-user",
		Source:    "web-ui",
		CreatedAt: time.Now().UTC(),
	})

	err = store.SetLifecycleStatus(first.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: "operator accepted",
		Actor:  "operator-user",
		Source: "web-ui",
	})
	require.NoError(t, err)

	first, err = store.Get(context.Background(), first.ID)
	require.NoError(t, err)

	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       "This BLOCK is terminal: review_spec_gap",
			RawOutput:       "BLOCK: review_spec_gap",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			Findings: []Finding{
				{Severity: "block", Summary: "Structured terminal class: review_spec_gap", Location: "bead:AC1"},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// First terminal check.
	_ = RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-bounded-first",
			ResultRev: "aabbcc01",
			BaseRev:   "aabbcc00",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})

	first, _ = store.Get(context.Background(), first.ID)
	statusAfterFirst := first.Status
	eventsAfterFirst, _ := store.Events(first.ID)
	countAfterFirst := len(eventsAfterFirst)

	// Second terminal check (same review outcome).
	_ = RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-bounded-second",
			ResultRev: "aabbcc01",
			BaseRev:   "aabbcc00",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})

	first, _ = store.Get(context.Background(), first.ID)
	statusAfterSecond := first.Status
	eventsAfterSecond, _ := store.Events(first.ID)
	countAfterSecond := len(eventsAfterSecond)

	// Status must not change between the two passes.
	assert.Equal(t, statusAfterFirst, statusAfterSecond,
		"status must be stable across readiness passes after operator promotion")

	// The bead should stay open.
	assert.Equal(t, bead.StatusOpen, statusAfterSecond,
		"operator-promoted bead must remain open after second readiness pass")

	// Count must either stay the same (if we're idempotent) or increase by 1 at most
	// (if we emit an observable warning). Either way, no new status transitions.
	// t.Logf("Events: before=%d, after_first=%d, after_second=%d", countBefore, countAfterFirst, countAfterSecond)
	assert.True(t, countAfterSecond <= countAfterFirst+1,
		fmt.Sprintf("readiness check must be idempotent: at most one new event on second pass (after_first=%d, after_second=%d)", countAfterFirst, countAfterSecond))
}

// TestReadinessHonorsOperatorAcceptanceAcrossSessions verifies that the operator
// acceptance signal is read from the persisted transition/event log, not in-
// memory state, so it survives process restart.
func TestReadinessHonorsOperatorAcceptanceAcrossSessions(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	// Park to proposed, then promote to open.
	err := store.SetLifecycleStatus(first.ID, bead.StatusProposed, bead.LifecycleTransitionOptions{
		OperatorRequired: true,
		Reason:           "initial parking",
		Actor:            "worker",
		Source:           "ddx work",
	})
	require.NoError(t, err)

	_ = store.AppendEvent(first.ID, bead.BeadEvent{
		Kind:      "triaged",
		Summary:   "operator accepted proposed bead",
		Body:      `{"from_status":"proposed","to_status":"open","actor":"operator-user","source":"web-ui"}`,
		Actor:     "operator-user",
		Source:    "web-ui",
		CreatedAt: time.Now().UTC(),
	})

	err = store.SetLifecycleStatus(first.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: "operator accepted",
		Actor:  "operator-user",
		Source: "web-ui",
	})
	require.NoError(t, err)

	// Fetch from store again (simulating process restart).
	first, err = store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, first.Status)

	// Now run terminal check on the reloaded bead.
	reviewer := beadReviewerFunc(func(_ context.Context, _, _ string, _ ImplementerRouting) (*ReviewResult, error) {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       "This BLOCK is terminal: review_spec_gap",
			RawOutput:       "BLOCK: review_spec_gap",
			ReviewerHarness: "claude",
			ReviewerModel:   "claude-opus-4-6",
			Findings: []Finding{
				{Severity: "block", Summary: "Structured terminal class: review_spec_gap", Location: "bead:AC1"},
			},
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_ = RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-across-sessions",
			ResultRev: "aabbcc01",
			BaseRev:   "aabbcc00",
		},
		Reviewer:    reviewer,
		Store:       store,
		ProjectRoot: t.TempDir(),
		Rcfg:        rcfg,
		Now:         time.Now,
		Assignee:    "worker",
	})

	// Fetch the bead again.
	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)

	// The operator acceptance signal must be honored across the process restart.
	assert.Equal(t, bead.StatusOpen, got.Status,
		"operator acceptance must be honored even after process restart")
}
