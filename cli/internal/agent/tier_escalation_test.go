package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TiersInRange ---

func TestTiersInRangeDefaults(t *testing.T) {
	got := TiersInRange("", "")
	require.Equal(t, []ModelTier{TierCheap, TierStandard, TierSmart}, got)
}

func TestTiersInRangeMinOnly(t *testing.T) {
	got := TiersInRange(TierStandard, "")
	require.Equal(t, []ModelTier{TierStandard, TierSmart}, got)
}

func TestTiersInRangeMaxOnly(t *testing.T) {
	got := TiersInRange("", TierStandard)
	require.Equal(t, []ModelTier{TierCheap, TierStandard}, got)
}

func TestTiersInRangeSingleTier(t *testing.T) {
	got := TiersInRange(TierSmart, TierSmart)
	require.Equal(t, []ModelTier{TierSmart}, got)
}

func TestTiersInRangeInvertedIsEmpty(t *testing.T) {
	got := TiersInRange(TierSmart, TierCheap)
	require.Empty(t, got)
}

func TestTiersInRangeDoesNotMutateTierOrder(t *testing.T) {
	before := make([]ModelTier, len(TierOrder))
	copy(before, TierOrder)
	got := TiersInRange("", "")
	require.Equal(t, before, TierOrder, "TierOrder must not be mutated")
	got[0] = "mutated"
	assert.Equal(t, TierCheap, TierOrder[0], "modifying result must not affect TierOrder")
}

// --- ShouldEscalate ---

func TestShouldEscalateExecutionFailed(t *testing.T) {
	assert.True(t, ShouldEscalate(ExecuteBeadStatusExecutionFailed))
}

func TestShouldEscalateNoChanges(t *testing.T) {
	assert.True(t, ShouldEscalate(ExecuteBeadStatusNoChanges))
}

func TestShouldEscalatePostRunCheckFailed(t *testing.T) {
	assert.True(t, ShouldEscalate(ExecuteBeadStatusPostRunCheckFailed))
}

func TestShouldEscalateLandConflict(t *testing.T) {
	assert.True(t, ShouldEscalate(ExecuteBeadStatusLandConflict))
}

func TestShouldEscalateSuccessIsFalse(t *testing.T) {
	assert.False(t, ShouldEscalate(ExecuteBeadStatusSuccess))
}

func TestShouldEscalateStructuralValidationIsFalse(t *testing.T) {
	assert.False(t, ShouldEscalate(ExecuteBeadStatusStructuralValidationFailed))
}

func TestShouldEscalateAlreadySatisfiedIsFalse(t *testing.T) {
	assert.False(t, ShouldEscalate(ExecuteBeadStatusAlreadySatisfied))
}

// --- ProviderHealthTracker ---

func TestProviderHealthTrackerIsHealthyByDefault(t *testing.T) {
	h := NewProviderHealthTracker()
	assert.True(t, h.IsHealthy("claude"))
	assert.True(t, h.IsHealthy("codex"))
}

func TestProviderHealthTrackerMarkMakesUnhealthy(t *testing.T) {
	h := NewProviderHealthTracker()
	h.Mark("bragi", time.Now().Add(5*time.Minute))
	assert.False(t, h.IsHealthy("bragi"))
	assert.True(t, h.IsHealthy("claude"), "unrelated harness must remain healthy")
}

func TestProviderHealthTrackerExpiredCooldownRestoresHealth(t *testing.T) {
	h := NewProviderHealthTracker()
	// Mark as unhealthy in the past (already expired).
	h.Mark("vidar", time.Now().Add(-1*time.Second))
	assert.True(t, h.IsHealthy("vidar"), "expired cooldown must be treated as healthy")
}

func TestProviderHealthTrackerSnapshotExcludesExpired(t *testing.T) {
	h := NewProviderHealthTracker()
	h.Mark("live", time.Now().Add(5*time.Minute))
	h.Mark("dead", time.Now().Add(-1*time.Second))

	snap := h.Snapshot()
	assert.Contains(t, snap, "live")
	assert.NotContains(t, snap, "dead", "expired entries must be excluded from snapshot")
}

func TestProviderHealthTrackerSnapshotIsCopy(t *testing.T) {
	h := NewProviderHealthTracker()
	h.Mark("claude", time.Now().Add(5*time.Minute))

	snap := h.Snapshot()
	delete(snap, "claude")

	// Tracker must be unaffected by external map mutations.
	assert.False(t, h.IsHealthy("claude"), "internal state must be independent of snapshot copy")
}

// --- FormatTierAttemptBody ---

func TestFormatTierAttemptBodyWithAllFields(t *testing.T) {
	body := FormatTierAttemptBody("cheap", "claude", "claude-haiku-4-5", "ok", "execution failed")
	assert.Contains(t, body, "tier=cheap")
	assert.Contains(t, body, "harness=claude")
	assert.Contains(t, body, "model=claude-haiku-4-5")
	assert.Contains(t, body, "probe=ok")
	assert.Contains(t, body, "execution failed")
}

func TestFormatTierAttemptBodyNoProbeNoDetail(t *testing.T) {
	body := FormatTierAttemptBody("standard", "codex", "gpt-5.4", "", "")
	assert.Contains(t, body, "tier=standard")
	assert.NotContains(t, body, "probe=")
}
