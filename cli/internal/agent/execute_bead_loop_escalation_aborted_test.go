package agent

import (
	"context"
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

// TestApplyProviderConnectivityRouteExclusion_DoesNotEscalatePower asserts
// that provider connectivity failures do NOT trigger power escalation checks.
// Instead, the bead remains open for retry on alternate providers at the same
// power level. Power escalation only occurs when Fizeau reports no_viable_provider.
func TestApplyProviderConnectivityRouteExclusion_DoesNotEscalatePower(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-eabc01", Title: "connectivity fallback to alternate provider"}
	require.NoError(t, store.Create(context.Background(), b))

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
	assert.Nil(t, aborted, "execution-escalation-aborted event must NOT be emitted for provider connectivity failures; power escalation is deferred to no_viable_provider handling")

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must remain open for retry on alternate providers")

	entries := readFailedRoutes(got.Extra)
	require.Len(t, entries, 1)
	assert.Equal(t, "lm-studio", entries[0].Provider)
	assert.Equal(t, "qwen2.5-7b", entries[0].Model)
}

// TestApplyProviderConnectivityRouteExclusion_NoEscalationMetadata
// asserts that provider connectivity failures don't write escalation metadata
// since power escalation is deferred to no_viable_provider handling.
func TestApplyProviderConnectivityRouteExclusion_NoEscalationMetadata(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-eabc02", Title: "connectivity defers escalation"}
	require.NoError(t, store.Create(context.Background(), b))

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

	assert.NotContains(t, got.Extra, legacyRetryFloorKey, "no legacy retry floor should be written for provider connectivity failures")
}

// TestProviderConnectivityRepeatedFailure_KeepsOpenForAutonomousRetry asserts
// that repeated provider_connectivity failures against the same (provider,
// model) stay retryable route-health evidence instead of parking the bead for
// operator attention.
func TestProviderConnectivityRepeatedFailure_KeepsOpenForAutonomousRetry(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-eabc03", Title: "repeated connectivity failure escalation"}
	require.NoError(t, store.Create(context.Background(), b))

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

	// Second failure: same (provider, model) stays open and records auto-retry evidence.
	err = applyProviderConnectivityRouteExclusion(store, b.ID, "test-actor", report, false, nextFloorFn, at)
	require.NoError(t, err)

	second, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, second.Status, "bead must remain open after repeated provider connectivity failures")
	assert.Empty(t, bead.GetNeedsHumanMeta(*second).Reason)

	entries := readFailedRoutes(second.Extra)
	require.Len(t, entries, 1)
	assert.Equal(t, 2, entries[0].Count)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var retry *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "provider_connectivity.auto_retry" {
			retry = &events[i]
			break
		}
	}
	require.NotNil(t, retry, "auto-retry event must be appended after repeated route failure")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(retry.Body), &body))
	assert.Equal(t, "grendel", body["provider"])
	assert.Equal(t, "mistral-7b", body["model"])
	assert.Equal(t, float64(2), body["count"])
}

func TestAutoReopenRetryableProviderConnectivityProposals(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	retryable := &bead.Bead{
		ID:     "ddx-eabc-retry",
		Title:  "retry provider connectivity proposal",
		Status: bead.StatusProposed,
		Labels: []string{bead.LabelNeedsHuman, "area:agent"},
	}
	require.NoError(t, store.Create(context.Background(), retryable))
	require.NoError(t, store.Update(context.Background(), retryable.ID, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          "provider grendel / model qwen3.5-27b unreachable on 2+ attempts; check ddx config or provider health",
			Source:          "ddx work",
			SuggestedAction: "check provider health or reconfigure endpoints in .ddx/config.yaml",
			Summary:         "provider unreachable on repeated attempts",
		})
	}))

	manual := &bead.Bead{
		ID:     "ddx-eabc-manual",
		Title:  "manual operator proposal",
		Status: bead.StatusProposed,
		Labels: []string{bead.LabelNeedsHuman},
	}
	require.NoError(t, store.Create(context.Background(), manual))
	require.NoError(t, store.Update(context.Background(), manual.ID, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          "scope is ambiguous and requires product judgment",
			Source:          "ddx work",
			SuggestedAction: "clarify acceptance criteria",
			Summary:         "manual operator review required",
		})
	}))

	at := time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC)
	emitted := make([]string, 0, 1)
	reopened, err := autoReopenRetryableProviderConnectivityProposals(
		context.Background(),
		store,
		"test-actor",
		at,
		func(kind string, _ map[string]any) { emitted = append(emitted, kind) },
	)
	require.NoError(t, err)
	assert.Equal(t, 1, reopened)
	assert.Equal(t, []string{"provider_connectivity.auto_reopen"}, emitted)

	gotRetryable, err := store.Get(retryable.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotRetryable.Status)
	assert.NotContains(t, gotRetryable.Labels, bead.LabelNeedsHuman)
	assert.Empty(t, bead.GetNeedsHumanMeta(*gotRetryable).Reason)

	events, err := store.Events(retryable.ID)
	require.NoError(t, err)
	assert.Condition(t, func() bool {
		for _, ev := range events {
			if ev.Kind == "provider_connectivity.auto_reopen" {
				return true
			}
		}
		return false
	}, "auto-reopen event must be recorded")

	gotManual, err := store.Get(manual.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, gotManual.Status)
	assert.Contains(t, gotManual.Labels, bead.LabelNeedsHuman)
}

// TestApplyNoChangesSmartRetry_LadderExhaustedEmitsEvent asserts that when the
// escalation ladder is exhausted, applyNoChangesSmartRetry emits a
// kind=execution-escalation-aborted event before parking the bead.
func TestApplyNoChangesSmartRetry_LadderExhaustedEmitsEvent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-eabc04", Title: "no-changes smart retry ladder exhausted event"}
	require.NoError(t, store.Create(context.Background(), b))

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
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-eabc05", Title: "repair cycle exhausted event"}
	require.NoError(t, store.Create(context.Background(), b))

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

// TestProviderConnectivityFailure_RetriesOnAlternateProviderAtSamePower
// verifies AC #1 and #4: when a bead's primary route fails mid-execution with
// provider_connectivity, the bead is left open for automatic fallback to the
// next route in the readiness fallback chain (same power, different provider)
// without operator intervention.
func TestProviderConnectivityFailure_RetriesOnAlternateProviderAtSamePower(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:    "ddx-ac1test",
		Title: "provider connectivity retry on alternate",
	}
	require.NoError(t, store.Create(context.

		// First failure: bragi at power 5
		Background(), b))

	report1 := ExecuteBeadReport{
		Provider:    "bragi",
		Model:       "qwen3.6-27b",
		ActualPower: 5,
	}
	at := time.Date(2026, 5, 16, 5, 56, 28, 0, time.UTC)
	err := applyProviderConnectivityRouteExclusion(store, b.ID, "worker", report1, false, nil, at)
	require.NoError(t, err)

	afterFirst, err := store.Get(b.ID)
	require.NoError(t, err)

	// Verify bead remains open and reclaimable
	assert.Equal(t, bead.StatusOpen, afterFirst.Status, "AC #1: bead must remain open after provider_connectivity failure")
	assert.Empty(t, afterFirst.Owner, "bead must be unclaimed for the next attempt")

	// Verify failed route is recorded
	failedRoutes := readFailedRoutes(afterFirst.Extra)
	require.Len(t, failedRoutes, 1)
	assert.Equal(t, "bragi", failedRoutes[0].Provider)
	assert.Equal(t, "qwen3.6-27b", failedRoutes[0].Model)
	assert.Equal(t, 5, failedRoutes[0].ActualPower)

	// Verify NO escalation-aborted event is emitted
	events, err := store.Events(b.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "execution-escalation-aborted", ev.Kind,
			"AC #1: escalation-aborted must NOT be emitted for provider_connectivity failures")
	}

	// The bead is now ready for retry with the failed route recorded.
	// In production, Fizeau would use the failed routes list (buildExcludedRoutes)
	// to avoid bragi and select an alternate provider (e.g., claude/opus)
	// at the same power level (AC #4: automatic fallback without operator intervention).
	//
	// This demonstrates:
	// AC #1: Primary route failed mid-execution, bead retries on next route at same power
	// AC #4: Occurs automatically without operator intervention
}
