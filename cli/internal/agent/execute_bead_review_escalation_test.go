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

// escalationTestService returns review output while exposing catalog methods as
// tripwires: abstract reviewer escalation must never consult them.
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
// the result_rev field used to scope retry escalation.
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

// TestReviewerEscalation_OnThirdErrorRaisesAbstractFloor verifies that prior
// review errors increase MinPower without selecting a Fizeau policy.
func TestReviewerEscalation_OnThirdErrorRaisesAbstractFloor(t *testing.T) {
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

	assert.Equal(t, 73, svc.lastReq.MinPower,
		"third error must dispatch reviewer at a stronger min_power")
	assert.Empty(t, svc.lastReq.Policy)
	assert.False(t, svc.listPoliciesCalled)
	assert.False(t, svc.listModelsCalled)

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

// TestReviewerEscalation_LegacyPairingEventDoesNotRaiseMinPower verifies that
// historical concrete-route comparison telemetry is audit-only.
func TestReviewerEscalation_LegacyPairingEventDoesNotRaiseMinPower(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	svc := escalationTestService(escalationApproveEvents())
	events := &stubBeadEventAppender{}

	// Seed a legacy review-pairing-degraded event scoped to this result_rev.
	require.NoError(t, store.AppendEvent("ddx-pairing", bead.BeadEvent{
		Kind:      legacyReviewPairingDegradedEventKind,
		Summary:   "reviewer pinned to same provider as implementer (openai)",
		Body:      "impl_provider=openai\nreviewer_provider=openai\nresult_rev=" + head + "\n",
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

	assert.Equal(t, 71, svc.lastReq.MinPower,
		"legacy pairing telemetry must not change the reviewer request")

	for _, ev := range events.events {
		assert.NotEqual(t, ReviewerEscalatedEventKind, ev.Event.Kind,
			"legacy pairing telemetry must not emit reviewer escalation")
	}
}
