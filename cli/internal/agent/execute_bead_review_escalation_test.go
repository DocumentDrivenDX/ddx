package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// escalationTestService builds a passthroughTestService with a two-profile
// setup: "frontier" (MinPower=71, MaxPower=75) and no stronger policy.
// This lets the tests observe:
//   - baseline (0 errors, floor=71): selects "frontier" with MinPower=71
//   - 1 prior error (floor=72): no qualifying profile → MinPower=72 (floor)
//   - 2+ prior errors (top-of-ladder): "frontier" selected via SelectStrongestProfile
//     with MinPower=max(71, escalatedFloor=73)=73
func escalationTestService(executeEvents []agentlib.ServiceEvent) *passthroughTestService {
	return &passthroughTestService{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "frontier", MinPower: 71, MaxPower: 75},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "frontier-model", Power: 72, Available: true, AutoRoutable: true},
		},
		executeEvents: executeEvents,
	}
}

// escalationApproveEvents returns a service event slice that produces a
// parseable APPROVE verdict so ReviewBead completes without parse errors.
func escalationApproveEvents() []agentlib.ServiceEvent {
	return []agentlib.ServiceEvent{
		{
			Type: "final",
			Data: []byte(`{"status":"success","final_text":"{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\",\"per_ac\":[{\"number\":1,\"item\":\"AC one\",\"grade\":\"pass\",\"evidence\":\"reviewed\"}]}"}`),
		},
	}
}

// seedReviewErrorEventsWithRev seeds N review-error events whose body contains
// the result_rev field (required for countPriorEscalationTriggers to count them).
func seedReviewErrorEventsWithRev(t *testing.T, store *bead.Store, beadID, resultRev, class string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		require.NoError(t, store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "review-error",
			Summary:   class,
			Body:      ReviewErrorEventBody(class, i+1, resultRev, "simulated transport failure"),
			Actor:     "worker",
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		}))
	}
}

// TestReviewerEscalation_OnSecondTransportError verifies that after one prior
// review-error event for the same result_rev, the reviewer's MinPower is
// bumped from impl.ActualPower+1 to impl.ActualPower+2, and a
// kind:reviewer-escalated event is emitted.
func TestReviewerEscalation_OnSecondTransportError(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	svc := escalationTestService(escalationApproveEvents())
	events := &stubBeadEventAppender{}

	// Seed 1 prior review-error event scoped to this result_rev.
	seedReviewErrorEventsWithRev(t, store, "ddx-pairing", head, evidence.OutcomeReviewTransport, 1)

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		EventReader: store,
		BeadEvents:  events,
		Service:     svc,
	}

	_, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
	})
	require.NoError(t, err)

	// MinPower should be impl.ActualPower+2 = 72 (bumped one step above baseline 71).
	// With only "frontier" (MinPower=71) available, floor=72 exceeds it so
	// profile selection returns no name but MinPower=72 from escalatedFloor.
	assert.Equal(t, 72, svc.lastReq.MinPower,
		"second transport error must bump reviewer MinPower to impl.ActualPower+2")

	// A reviewer-escalated event must have been emitted.
	var escalated *bead.BeadEvent
	for i := range events.events {
		ev := &events.events[i]
		if ev.Event.Kind == ReviewerEscalatedEventKind {
			escalated = &ev.Event
			break
		}
	}
	require.NotNil(t, escalated, "reviewer-escalated event must be emitted when MinPower is bumped")
	assert.Contains(t, escalated.Body, "error_count=1",
		"reviewer-escalated body must carry error_count for triage")
	assert.Contains(t, escalated.Body, "new_min_power=72",
		"reviewer-escalated body must carry new_min_power")
	assert.Contains(t, escalated.Body, "result_rev="+head,
		"reviewer-escalated body must be scoped to result_rev")
}

// TestReviewerEscalation_OnThirdError_TopOfLadder verifies that after two
// prior review-error events for the same result_rev, the reviewer is routed
// to the strongest available profile (top-of-ladder) regardless of floor
// restriction, and a reviewer-escalated event is emitted with error_count=2.
func TestReviewerEscalation_OnThirdError_TopOfLadder(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	svc := escalationTestService(escalationApproveEvents())
	events := &stubBeadEventAppender{}

	// Seed 2 prior review-error events scoped to this result_rev.
	seedReviewErrorEventsWithRev(t, store, "ddx-pairing", head, evidence.OutcomeReviewTransport, 2)

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		EventReader: store,
		BeadEvents:  events,
		Service:     svc,
	}

	_, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
	})
	require.NoError(t, err)

	// Top-of-ladder: SelectStrongestProfile selects "frontier" (only policy).
	// escalatedFloor = 71+2 = 73; max(71, 73) = 73 → MinPower=73.
	// This is higher than the 1-error case (72) and the baseline (71).
	assert.Equal(t, 73, svc.lastReq.MinPower,
		"third error must dispatch reviewer at top-of-ladder min_power")
	// Top-of-ladder uses SelectStrongestProfile which selects "frontier".
	assert.Equal(t, "frontier", svc.lastReq.Policy,
		"top-of-ladder must select the strongest available profile")

	// A reviewer-escalated event must be emitted with error_count=2.
	var escalated *bead.BeadEvent
	for i := range events.events {
		ev := &events.events[i]
		if ev.Event.Kind == ReviewerEscalatedEventKind {
			escalated = &ev.Event
			break
		}
	}
	require.NotNil(t, escalated, "reviewer-escalated event must be emitted for top-of-ladder dispatch")
	assert.Contains(t, escalated.Body, "error_count=2",
		"reviewer-escalated body must carry error_count=2 for triage")
	assert.Contains(t, escalated.Body, "new_min_power=73",
		"reviewer-escalated body must carry top-of-ladder min_power")
}

// TestReviewerEscalation_PairingDegraded_TriggersEscalation verifies that a
// prior kind:review-pairing-degraded event for the same result_rev counts as
// an escalation trigger: the next reviewer dispatch bumps MinPower by one
// ladder step above baseline, and a reviewer-escalated event is emitted.
func TestReviewerEscalation_PairingDegraded_TriggersEscalation(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	svc := escalationTestService(escalationApproveEvents())
	events := &stubBeadEventAppender{}

	// Seed 1 review-pairing-degraded event with result_rev in the body
	// (required for countPriorEscalationTriggers to count it).
	require.NoError(t, store.AppendEvent("ddx-pairing", bead.BeadEvent{
		Kind:    ReviewPairingDegradedEventKind,
		Summary: "reviewer pinned to same provider as implementer (openai)",
		Body: reviewPairingDegradedBody(
			ImplementerRouting{
				Harness:     "codex",
				Provider:    "openai",
				Model:       "gpt-5",
				ActualPower: 70,
			},
			"codex", "openai", "gpt-5", 70, head,
		),
		Actor:     "worker",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	}))

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		EventReader: store,
		BeadEvents:  events,
		Service:     svc,
	}

	_, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
	})
	require.NoError(t, err)

	// After 1 pairing-degraded trigger, MinPower bumps from 71 to 72
	// (same as after 1 review-error: impl.ActualPower+2).
	assert.Equal(t, 72, svc.lastReq.MinPower,
		"pairing-degraded event must trigger escalation: MinPower bumped to impl.ActualPower+2")

	// A reviewer-escalated event must have been emitted.
	var escalated *bead.BeadEvent
	for i := range events.events {
		ev := &events.events[i]
		if ev.Event.Kind == ReviewerEscalatedEventKind {
			escalated = &ev.Event
			break
		}
	}
	require.NotNil(t, escalated,
		"reviewer-escalated event must be emitted when pairing-degraded triggers escalation")
	assert.Contains(t, escalated.Body, "error_count=1",
		"reviewer-escalated body must reflect one escalation trigger")
	assert.Contains(t, escalated.Body, "result_rev="+head,
		"reviewer-escalated must be scoped to the same result_rev as the pairing-degraded event")
}
