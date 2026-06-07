package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWatchdogFiresOnPhaseEmptyHeartbeats verifies AC #1: when phase-empty
// heartbeats (harness/model/route all empty) continue past the phase budget,
// the worker releases the held lease and emits an operator_attention event
// carrying bead-id, attempt-id, last_activity_at, and a diagnosis. The
// snapshot drives empty-field heartbeats while a small budget is exceeded.
func TestWatchdogFiresOnPhaseEmptyHeartbeats(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	beadID := first.ID
	require.NoError(t, store.Claim(beadID, "worker-a"))
	claimed, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	require.Equal(t, bead.StatusInProgress, claimed.Status)

	frozen := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	const attemptID = "20260526T100000-wedge001"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attemptCtx, attemptCancel := context.WithCancel(ctx)
	defer attemptCancel()

	done := make(chan struct{})
	go func() {
		work.RunProgressWatchdog(attemptCtx, 2*time.Millisecond, work.ProgressWatchdogConfig{
			Budgets: work.PhaseBudgets{Resolving: 20 * time.Millisecond, Running: 20 * time.Millisecond},
			Now:     time.Now,
			Snapshot: func() workerstatus.LivenessRecord {
				return workerstatus.LivenessRecord{
					CurrentBead:    beadID,
					AttemptID:      attemptID,
					Phase:          string(work.PhaseRunning),
					Harness:        "",
					Model:          "",
					Route:          "",
					LastActivityAt: frozen,
				}
			},
			OnWedged: func(rec workerstatus.LivenessRecord, budget time.Duration) {
				flagWedgedForOperatorAttention(store, beadID, "worker-a", rec.AttemptID, rec.Phase, rec.LastActivityAt, budget, frozen.UTC())
				attemptCancel()
				close(done)
			},
		})
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("progress watchdog did not fire on persistent phase-empty heartbeats")
	}

	// The lease was released atomically: status back to open, owner cleared.
	released, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, released.Status, "the wedged lease must be released to open")
	assert.Empty(t, released.Owner, "the claim owner must be cleared on release")

	// An operator-attention event was emitted with the required fields.
	events, err := store.Events(beadID)
	require.NoError(t, err)
	var attention *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "operator_attention" {
			attention = &events[i]
			break
		}
	}
	require.NotNil(t, attention, "an operator_attention event must be emitted when the watchdog fires")
	assert.Equal(t, FailureModeProgressWatchdog, attention.Summary)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(attention.Body), &body))
	assert.Equal(t, beadID, body["bead_id"])
	assert.Equal(t, attemptID, body["attempt_id"])
	assert.Equal(t, frozen.UTC().Format(time.RFC3339), body["last_activity_at"])
	assert.NotEmpty(t, body["diagnosis"])
	assert.Equal(t, "ddx work", attention.Source)
}

// TestAttemptTerminatesWhenBeadClosedExternally verifies AC #3: a running
// attempt whose bead is closed by another attempt terminates promptly and
// releases its lease without marking the attempt failed. The external-close
// watcher polls bead status; once a parallel attempt closes the bead it cancels
// the attempt and releases the lease, leaving the bead closed.
func TestAttemptTerminatesWhenBeadClosedExternally(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	beadID := first.ID
	require.NoError(t, store.Claim(beadID, "worker-a"))
	claimed, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	require.Equal(t, bead.StatusInProgress, claimed.Status)
	require.Equal(t, "worker-a", claimed.Owner)

	frozen := time.Date(2026, 5, 26, 11, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attemptCtx, attemptCancel := context.WithCancel(ctx)
	defer attemptCancel()

	done := make(chan struct{})
	go func() {
		work.RunExternalCloseWatcher(attemptCtx, 2*time.Millisecond, work.ExternalCloseWatcherConfig{
			IsClosed: func() (bool, error) { return beadClosedExternally(store, beadID) },
			OnClosed: func() {
				releaseClosedExternally(store, beadID, "worker-a", frozen.UTC())
				attemptCancel()
				close(done)
			},
		})
	}()

	// While the bead is still in_progress the watcher must not fire.
	select {
	case <-done:
		t.Fatal("external-close watcher fired before the bead was closed")
	case <-time.After(30 * time.Millisecond):
	}

	// A parallel attempt closes the same bead.
	require.NoError(t, store.Close(context.Background(), beadID))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("external-close watcher did not terminate the attempt after the bead closed")
	}

	// The attempt context was cancelled promptly.
	select {
	case <-attemptCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("attempt context was not cancelled on external close")
	}

	// The lease was released (owner cleared) and the bead stays closed — the
	// work is done, so the attempt is not reopened or marked failed.
	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "an externally-closed bead must stay closed")
	assert.Empty(t, got.Owner, "the lease owner must be cleared on release")

	// No operator_attention event was emitted: external close is not a failure
	// escalation.
	events, err := store.Events(beadID)
	require.NoError(t, err)
	for _, e := range events {
		assert.NotEqual(t, "operator_attention", e.Kind, "external close must not flag operator attention")
	}
}
