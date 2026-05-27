package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSameBeadTwiceWedgeGuardStopsClaiming verifies AC #1: when the same bead
// wedges on consecutive claims (default threshold 2), the worker stops claiming
// it, parks it to an operator-attention/proposed state, surfaces it via an
// event, and the queue keeps draining its sibling beads. Two consecutive wedges
// are injected through the live progress-watchdog release primitive.
func TestSameBeadTwiceWedgeGuardStopsClaiming(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	beadID := first.ID
	frozen := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

	// First wedge: claim, then the progress watchdog releases the lease and
	// records the consecutive-wedge marker (count 1). One wedge must stay below
	// the threshold so a transient single wedge does not sideline the bead.
	require.NoError(t, store.Claim(beadID, "worker-a"))
	flagWedgedForOperatorAttention(store, beadID, "worker-a", "attempt-1", string(work.PhaseRunning), frozen, time.Minute, frozen)

	afterFirst, err := store.Get(beadID)
	require.NoError(t, err)
	require.Equal(t, bead.StatusOpen, afterFirst.Status, "a single wedge releases the bead back to open (re-claimable)")
	require.Empty(t, afterFirst.Owner, "the wedged lease owner must be cleared")
	marker1 := readWedgeMarker(afterFirst.Extra)
	assert.Equal(t, 1, marker1.Count)
	assert.Equal(t, FailureModeProgressWatchdog, marker1.LastReason)
	assert.False(t, consecutiveWedgeGuardTrips(marker1, DefaultConsecutiveWedgeThreshold), "one wedge must stay below the default threshold")

	// Second consecutive wedge: re-claim and wedge again (count 2).
	require.NoError(t, store.Claim(beadID, "worker-a"))
	flagWedgedForOperatorAttention(store, beadID, "worker-a", "attempt-2", string(work.PhaseRunning), frozen, time.Minute, frozen)

	afterSecond, err := store.Get(beadID)
	require.NoError(t, err)
	marker2 := readWedgeMarker(afterSecond.Extra)
	assert.Equal(t, 2, marker2.Count)
	require.True(t, consecutiveWedgeGuardTrips(marker2, DefaultConsecutiveWedgeThreshold), "two consecutive wedges must trip the guard")

	// The guard's terminal action: stop re-claiming, park to proposed, emit a
	// consecutive_wedge operator-attention event.
	require.NoError(t, flagConsecutiveWedgeForOperator(store, beadID, "worker-a", marker2, DefaultConsecutiveWedgeThreshold, frozen))

	parked, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, parked.Status, "a twice-wedged bead must be parked to proposed for operator attention")
	assert.Empty(t, parked.Owner, "the parked bead must not be claimed")

	events, err := store.Events(beadID)
	require.NoError(t, err)
	var attention *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "operator_attention" && events[i].Summary == FailureModeConsecutiveWedge {
			attention = &events[i]
			break
		}
	}
	require.NotNil(t, attention, "a consecutive_wedge operator_attention event must be surfaced")
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(attention.Body), &body))
	assert.Equal(t, beadID, body["bead_id"])
	assert.Equal(t, float64(2), body["count"])
	assert.Equal(t, float64(DefaultConsecutiveWedgeThreshold), body["threshold"])

	// The queue keeps draining: the parked bead drops out of the execution-ready
	// view while the sibling bead is still ready for the loop to continue to.
	ready, err := store.ReadyExecution()
	require.NoError(t, err)
	var ids []string
	for _, b := range ready {
		ids = append(ids, b.ID)
	}
	assert.NotContains(t, ids, beadID, "the parked bead must drop out of the ready queue")
	assert.Contains(t, ids, second.ID, "the loop must continue to the next ready bead")
}

// TestWedgeMarkerResetsOnProgress verifies AC #2: the consecutive-wedge marker
// resets when the bead next makes real progress, so a bead that wedges once then
// makes progress is not sidelined when it later wedges again.
func TestWedgeMarkerResetsOnProgress(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	beadID := first.ID
	frozen := time.Date(2026, 5, 26, 13, 0, 0, 0, time.UTC)

	// One wedge: marker count 1, below the threshold.
	require.NoError(t, store.Claim(beadID, "worker-a"))
	flagWedgedForOperatorAttention(store, beadID, "worker-a", "attempt-1", string(work.PhaseRunning), frozen, time.Minute, frozen)

	afterWedge, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, 1, readWedgeMarker(afterWedge.Extra).Count)

	// Real progress (a resolved route / LLM call / commit) clears the marker.
	clearConsecutiveWedge(store, beadID)

	afterProgress, err := store.Get(beadID)
	require.NoError(t, err)
	cleared := readWedgeMarker(afterProgress.Extra)
	assert.Equal(t, 0, cleared.Count, "real progress must reset the consecutive-wedge marker")
	assert.False(t, consecutiveWedgeGuardTrips(cleared, DefaultConsecutiveWedgeThreshold))

	// A subsequent wedge restarts the count at 1 — not 2 — so the bead is not
	// sidelined after a single wedge that was followed by progress.
	require.NoError(t, store.Claim(beadID, "worker-a"))
	flagWedgedForOperatorAttention(store, beadID, "worker-a", "attempt-2", string(work.PhaseRunning), frozen, time.Minute, frozen)

	afterSecond, err := store.Get(beadID)
	require.NoError(t, err)
	marker := readWedgeMarker(afterSecond.Extra)
	assert.Equal(t, 1, marker.Count, "the post-progress wedge must restart the streak at one")
	assert.False(t, consecutiveWedgeGuardTrips(marker, DefaultConsecutiveWedgeThreshold), "a wedge after real progress must not trip the guard")
	assert.Equal(t, bead.StatusOpen, afterSecond.Status, "the bead must remain re-claimable, not parked")
}

func TestPreClaimWarnFingerprintThresholdDefaultsAndOverrides(t *testing.T) {
	assert.Equal(t, DefaultPreClaimWarnFingerprintThreshold, (ExecuteBeadLoopRuntime{}).effectivePreClaimWarnFingerprintThreshold())
	assert.Equal(t, 7, (ExecuteBeadLoopRuntime{PreClaimWarnFingerprintThreshold: 7}).effectivePreClaimWarnFingerprintThreshold())
}

// TestPreClaimWarn_EscalatesAfterConsecutiveIdenticalFingerprints verifies AC #1:
// the loop tolerates a transient warn, escalates once the same fingerprint has
// repeated N times, and surfaces the first example payload alongside the
// fingerprint.
func TestPreClaimWarn_EscalatesAfterConsecutiveIdenticalFingerprints(t *testing.T) {
	state := preClaimWarnFingerprintState{}
	threshold := DefaultPreClaimWarnFingerprintThreshold

	makePayload := func(beadID string, fingerprint string) map[string]any {
		return map[string]any{
			"bead_id":     beadID,
			"fingerprint": fingerprint,
			"reason":      "warn",
			"detail":      "same blocker",
		}
	}

	for i := 0; i < threshold-1; i++ {
		pending := makePayload(string(rune('a'+i)), "fp-same")
		assert.False(t, state.observe(pending, threshold), "a single warn must not escalate")
		assert.Equal(t, "fp-same", state.Fingerprint)
		assert.Equal(t, i+1, state.Count)
		assert.False(t, state.Escalated)
	}

	finalPayload := makePayload("z", "fp-same")
	require.True(t, state.observe(finalPayload, threshold), "the Nth identical warn must escalate")
	assert.True(t, state.Escalated)
	assert.Equal(t, threshold, state.Count)
	require.NotNil(t, state.ExamplePayload)
	assert.Equal(t, "a", state.ExamplePayload["bead_id"], "the first payload in the streak must be preserved as the example")

	body := preClaimWarnFingerprintEscalationBody("z", state, threshold)
	assert.Equal(t, preClaimWarnFingerprintEscalationReason, body["reason"])
	assert.Equal(t, "z", body["bead_id"])
	assert.Equal(t, "fp-same", body["fingerprint"])
	assert.Equal(t, threshold, body["count"])
	assert.Equal(t, threshold, body["threshold"])
	example, ok := body["example_payload"].(map[string]any)
	require.True(t, ok, "example_payload must be a nested payload map")
	assert.Equal(t, "a", example["bead_id"])
	assert.Equal(t, "fp-same", example["fingerprint"])
	assert.NotEmpty(t, body["detail"])

	single := preClaimWarnFingerprintState{}
	assert.False(t, single.observe(makePayload("single", "fp-single"), threshold), "a single warn must stay below the threshold")
	assert.False(t, single.Escalated)
}

// TestPreClaimWarnFingerprintStateResetsOnClaimAndFingerprintChange verifies AC #2:
// a successful claim clears the streak, and a changed fingerprint restarts the
// count from one.
func TestPreClaimWarnFingerprintStateResetsOnClaimAndFingerprintChange(t *testing.T) {
	state := preClaimWarnFingerprintState{}
	threshold := DefaultPreClaimWarnFingerprintThreshold

	first := map[string]any{
		"bead_id":     "ddx-a",
		"fingerprint": "fp-a",
		"reason":      "warn",
		"detail":      "same blocker",
	}
	require.False(t, state.observe(first, threshold))
	assert.Equal(t, 1, state.Count)

	state.recordSuccess()
	assert.Empty(t, state.Fingerprint)
	assert.Zero(t, state.Count)
	assert.False(t, state.Escalated)
	assert.Nil(t, state.ExamplePayload)

	afterClaim := map[string]any{
		"bead_id":     "ddx-b",
		"fingerprint": "fp-a",
		"reason":      "warn",
		"detail":      "same blocker",
	}
	require.False(t, state.observe(afterClaim, threshold))
	assert.Equal(t, 1, state.Count, "a successful claim must reset the streak before the next warn")
	assert.Equal(t, "fp-a", state.Fingerprint)

	changed := map[string]any{
		"bead_id":     "ddx-c",
		"fingerprint": "fp-b",
		"reason":      "warn",
		"detail":      "different blocker",
	}
	require.False(t, state.observe(changed, threshold))
	assert.Equal(t, 1, state.Count, "a changed fingerprint must restart the streak at one")
	assert.Equal(t, "fp-b", state.Fingerprint)
	assert.Equal(t, "ddx-c", state.ExamplePayload["bead_id"])
}
