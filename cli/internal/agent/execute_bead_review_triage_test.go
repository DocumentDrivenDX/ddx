package agent

import (
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedBlocks appends `n` review-BLOCK events spaced 1 minute apart, starting
// at base. Returns the timestamp of the last BLOCK so test code can layer
// pairing-degraded events around it.
func seedBlocks(t *testing.T, store *bead.Store, beadID string, base time.Time, n int) time.Time {
	t.Helper()
	last := base
	for i := 0; i < n; i++ {
		last = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "review",
			Summary:   "BLOCK",
			Body:      "rationale",
			Actor:     "reviewer",
			Source:    "test",
			CreatedAt: last,
		}))
	}
	return last
}

// findEvent returns the last event of kind `kind` for bead, or nil.
func findEvent(t *testing.T, store *bead.Store, beadID, kind string) *bead.BeadEvent {
	t.Helper()
	events, err := store.Events(beadID)
	require.NoError(t, err)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == kind {
			ev := events[i]
			return &ev
		}
	}
	return nil
}

func newTriageTestStore(t *testing.T) (*bead.Store, *bead.Bead) {
	t.Helper()
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	b := &bead.Bead{ID: "ddx-triage-1", Title: "triage decision target"}
	require.NoError(t, store.Create(b))
	return store, b
}

// TestApplyReviewTriageDecision_FirstBlockReAttempts verifies that the first
// BLOCK on a bead chooses re_attempt_with_context: no powerClass hint is set, the
// bead remains worker-runnable, and a triage-decision event records the action.
func TestApplyReviewTriageDecision_FirstBlockReAttempts(t *testing.T) {
	store, b := newTriageTestStore(t)
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	seedBlocks(t, store, b.ID, now, 1)

	require.NoError(t, applyReviewTriageDecision(store, b.ID, "ddx", now.Add(time.Second), ""))

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Labels, TriageNeedsHumanLabel)
	if got.Extra != nil {
		_, hasHint := got.Extra[TriagePowerHintKey]
		assert.False(t, hasHint, "first BLOCK must not set powerClass hint")
	}
	ev := findEvent(t, store, b.ID, "triage-decision")
	require.NotNil(t, ev)
	assert.Contains(t, ev.Summary, "re_attempt_with_context")
}

// TestApplyReviewTriageDecision_SecondBlockEscalates verifies that the second
// BLOCK chooses escalate_power: a powerClass-pin hint is written into bead.Extra
// and a triage-decision event records the chosen action.
func TestApplyReviewTriageDecision_SecondBlockEscalates(t *testing.T) {
	store, b := newTriageTestStore(t)
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	seedBlocks(t, store, b.ID, now, 2)

	require.NoError(t, applyReviewTriageDecision(store, b.ID, "ddx", now.Add(time.Hour), string(escalation.PowerStandard)))

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Labels, TriageNeedsHumanLabel)
	require.NotNil(t, got.Extra)
	assert.Equal(t, string(escalation.PowerSmart), got.Extra[TriagePowerHintKey])
	ev := findEvent(t, store, b.ID, "triage-decision")
	require.NotNil(t, ev)
	assert.Contains(t, ev.Summary, "escalate_power")
	assert.Contains(t, ev.Body, "smart")
}

// TestApplyReviewTriageDecision_ThirdBlockOperatorRequired verifies that the
// third BLOCK chooses operator_required: the bead moves to status=proposed
// without an active needs_human label and a triage-decision event records the
// action.
func TestApplyReviewTriageDecision_ThirdBlockOperatorRequired(t *testing.T) {
	store, b := newTriageTestStore(t)
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Claim(b.ID, "worker"))
	seedBlocks(t, store, b.ID, now, 3)

	require.NoError(t, applyReviewTriageDecision(store, b.ID, "ddx", now.Add(time.Hour), string(escalation.PowerSmart)))

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	assert.Empty(t, got.Owner)
	assert.NotContains(t, got.Labels, TriageNeedsHumanLabel)
	assert.NotContains(t, got.Labels, bead.LabelNeedsHuman)
	assert.NotContains(t, got.Extra, "claimed-at")
	meta := bead.GetNeedsHumanMeta(*got)
	assert.Contains(t, meta.Reason, "operator-required")
	assert.Equal(t, "ddx work", meta.Source)
	ev := findEvent(t, store, b.ID, "triage-decision")
	require.NotNil(t, ev)
	assert.Contains(t, ev.Summary, "operator_required")
}

// TestApplyReviewTriageDecision_PairingDegradedBiasesToReAttempt verifies that
// when the latest BLOCK was paired with a kind:review-pairing-degraded event
// from the same attempt window, the policy's escalate_power rung is overridden
// to re_attempt_with_context so a freshly-paired reviewer gets another chance.
func TestApplyReviewTriageDecision_PairingDegradedBiasesToReAttempt(t *testing.T) {
	store, b := newTriageTestStore(t)
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)

	// First BLOCK at t0.
	seedBlocks(t, store, b.ID, now, 1)
	// Pairing-degraded event between BLOCK 1 and BLOCK 2.
	require.NoError(t, store.AppendEvent(b.ID, bead.BeadEvent{
		Kind:      ReviewPairingDegradedEventKind,
		Summary:   "reviewer pinned to same provider",
		Body:      "{}",
		Source:    "test",
		CreatedAt: now.Add(30 * time.Second),
	}))
	// Second BLOCK after pairing-degraded — same attempt window.
	require.NoError(t, store.AppendEvent(b.ID, bead.BeadEvent{
		Kind:      "review",
		Summary:   "BLOCK",
		Body:      "rationale",
		Actor:     "reviewer",
		Source:    "test",
		CreatedAt: now.Add(time.Minute),
	}))

	require.NoError(t, applyReviewTriageDecision(store, b.ID, "ddx", now.Add(time.Hour), string(escalation.PowerStandard)))

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	if got.Extra != nil {
		_, hasHint := got.Extra[TriagePowerHintKey]
		assert.False(t, hasHint, "pairing-degraded must override escalate_power; no powerClass hint expected")
	}
	assert.NotContains(t, got.Labels, TriageNeedsHumanLabel)
	ev := findEvent(t, store, b.ID, "triage-decision")
	require.NotNil(t, ev)
	assert.Contains(t, ev.Summary, "re_attempt_with_context")
	assert.Contains(t, ev.Body, "pairing_degraded")
}
