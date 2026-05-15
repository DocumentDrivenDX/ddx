package agent

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errLadderExhaustedTest = fmt.Errorf("ladder exhausted")

// TestApplyProviderConnectivityRouteExclusion_LadderExhaustedEmitsEvent asserts
// that when nextFloorFn returns an error (ladder exhausted), a
// kind=execution-escalation-aborted event is appended whose summary names the
// provider, model, and actualPower.
func TestApplyProviderConnectivityRouteExclusion_LadderExhaustedEmitsEvent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-eabc01", Title: "connectivity ladder exhausted event"}
	require.NoError(t, store.Create(b))

	report := ExecuteBeadReport{
		Provider:    "lm-studio",
		Model:       "qwen2.5-7b",
		ActualPower: 20,
	}
	exhaustedFn := func(int) (int, error) { return 0, errLadderExhaustedTest }
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	err := applyProviderConnectivityRouteExclusion(store, b.ID, "test-actor", report, false, exhaustedFn, at)
	require.NoError(t, err)

	events, err := store.Events(b.ID)
	require.NoError(t, err)

	var aborted *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execution-escalation-aborted" {
			aborted = &events[i]
			break
		}
	}
	require.NotNil(t, aborted, "execution-escalation-aborted event must be appended when ladder is exhausted")
	assert.Contains(t, aborted.Summary, "lm-studio", "summary must name provider")
	assert.Contains(t, aborted.Summary, "qwen2.5-7b", "summary must name model")
	assert.Contains(t, aborted.Summary, "20", "summary must name actualPower")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(aborted.Body), &body))
	assert.Equal(t, "lm-studio", body["provider"])
	assert.Equal(t, "qwen2.5-7b", body["model"])
	assert.Equal(t, float64(20), body["actual_power"])
}

// TestApplyProviderConnectivityRouteExclusion_LadderExhaustedDoesNotWriteNumericPowerHint
// asserts that ladder exhaustion still emits evidence without persisting a
// numeric retry floor on the bead.
func TestApplyProviderConnectivityRouteExclusion_LadderExhaustedDoesNotWriteNumericPowerHint(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-eabc02", Title: "connectivity sentinel hint on exhaustion"}
	require.NoError(t, store.Create(b))

	const actualPower = 30
	report := ExecuteBeadReport{
		Provider:    "bragi",
		Model:       "llama-3.1-8b",
		ActualPower: actualPower,
	}
	exhaustedFn := func(int) (int, error) { return 0, errLadderExhaustedTest }

	err := applyProviderConnectivityRouteExclusion(store, b.ID, "test-actor", report, false, exhaustedFn, time.Now().UTC())
	require.NoError(t, err)

	got, err := store.Get(b.ID)
	require.NoError(t, err)

	assert.NotContains(t, got.Extra, TriagePowerHintKey)
}

// TestProviderConnectivityRepeatedFailure_PromotesToOperatorRequired asserts that
// after 2 consecutive provider_connectivity failures against the same
// (provider, model), the bead is promoted to proposed (operator_required) with a
// message naming the unreachable provider.
func TestProviderConnectivityRepeatedFailure_PromotesToOperatorRequired(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-eabc03", Title: "repeated connectivity failure escalation"}
	require.NoError(t, store.Create(b))

	report := ExecuteBeadReport{
		Provider:    "grendel",
		Model:       "mistral-7b",
		ActualPower: 15,
	}
	// nextFloorFn succeeds on first call so the bead stays open and the route is recorded.
	nextFloorFn := func(int) (int, error) { return 50, nil }
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// First failure: bead stays open, route recorded.
	err := applyProviderConnectivityRouteExclusion(store, b.ID, "test-actor", report, false, nextFloorFn, at)
	require.NoError(t, err)

	first, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, first.Status, "bead must remain open after first failure")

	// Second failure: same (provider, model) — must escalate to proposed.
	err = applyProviderConnectivityRouteExclusion(store, b.ID, "test-actor", report, false, nextFloorFn, at)
	require.NoError(t, err)

	second, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, second.Status, "bead must be proposed (operator_required) after 2nd failure")

	meta := bead.GetNeedsHumanMeta(*second)
	assert.Contains(t, meta.Reason, "grendel", "operator_required reason must name the unreachable provider")
}

// TestApplyNoChangesSmartRetry_LadderExhaustedEmitsEvent asserts that when the
// escalation ladder is exhausted, applyNoChangesSmartRetry emits a
// kind=execution-escalation-aborted event before parking the bead.
func TestApplyNoChangesSmartRetry_LadderExhaustedEmitsEvent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-eabc04", Title: "no-changes smart retry ladder exhausted event"}
	require.NoError(t, store.Create(b))

	noChanges := &agenttry.NoChangesOutcome{
		Reason:          "agent cannot proceed further",
		SuggestedAction: "operator review needed",
	}
	exhaustedFn := func(int) (int, error) { return 0, errLadderExhaustedTest }

	err := applyNoChangesSmartRetry(store, b.ID, "test-actor", noChanges, 40, exhaustedFn)
	require.NoError(t, err)

	events, err := store.Events(b.ID)
	require.NoError(t, err)

	var aborted *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execution-escalation-aborted" {
			aborted = &events[i]
			break
		}
	}
	require.NotNil(t, aborted, "execution-escalation-aborted event must be emitted when no-changes ladder is exhausted")
	assert.Contains(t, aborted.Summary, "40", "summary must reference actualPower")
}

// TestApplyRepairCycleExhaustedEscalation_LadderExhaustedEmitsEvent asserts that
// when the escalation ladder is exhausted, applyRepairCycleExhaustedEscalation
// emits a kind=execution-escalation-aborted event before parking the bead.
func TestApplyRepairCycleExhaustedEscalation_LadderExhaustedEmitsEvent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-eabc05", Title: "repair cycle exhausted event"}
	require.NoError(t, store.Create(b))

	exhaustedFn := func(int) (int, error) { return 0, errLadderExhaustedTest }
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	err := applyRepairCycleExhaustedEscalation(store, b.ID, "test-actor", 60, at, exhaustedFn)
	require.NoError(t, err)

	events, err := store.Events(b.ID)
	require.NoError(t, err)

	var aborted *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execution-escalation-aborted" {
			aborted = &events[i]
			break
		}
	}
	require.NotNil(t, aborted, "execution-escalation-aborted event must be emitted when repair-cycle ladder is exhausted")
	assert.Contains(t, aborted.Summary, "60", "summary must reference actualPower")
}
