package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// --- AdaptiveMinTier ---

// writeAdaptiveFixture writes a result.json for a synthetic attempt under
// workingDir/.ddx/executions/<ts>/result.json. The `seq` parameter seeds the
// timestamp so the lexicographic ordering matches chronological order.
func writeAdaptiveFixture(t *testing.T, workingDir string, seq int, harness, model, outcome string) {
	t.Helper()
	ts := fmt.Sprintf("20260101T%06d-%08x", seq, seq)
	dir := filepath.Join(workingDir, ".ddx", "executions", ts)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	res := ExecuteBeadResult{
		BeadID:  fmt.Sprintf("bead-%d", seq),
		Harness: harness,
		Model:   model,
		Outcome: outcome,
	}
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), raw, 0o644))
}

func TestAdaptiveMinTierNoExecutionsReturnsCheap(t *testing.T) {
	dir := t.TempDir()
	got := AdaptiveMinTier(dir, 50)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.Equal(t, 0, got.CheapAttempts)
}

func TestAdaptiveMinTierCheapSuccessBelowThresholdPromotesToStandard(t *testing.T) {
	dir := t.TempDir()
	// 10 cheap-tier attempts, 1 success → 10% success rate (< 20% threshold).
	for i := 0; i < 9; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_failed")
	}
	writeAdaptiveFixture(t, dir, 9, "claude", "claude-haiku-4-5", "task_succeeded")

	got := AdaptiveMinTier(dir, 50)
	assert.Equal(t, TierStandard, got.Tier)
	assert.True(t, got.Skipped)
	assert.Equal(t, 10, got.CheapAttempts)
	assert.InDelta(t, 0.10, got.CheapSuccessRate, 0.001)
}

func TestAdaptiveMinTierCheapSuccessAboveThresholdStaysCheap(t *testing.T) {
	dir := t.TempDir()
	// 10 cheap-tier attempts, 5 successes → 50% success rate (>= 20%).
	for i := 0; i < 5; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_succeeded")
	}
	for i := 5; i < 10; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_failed")
	}

	got := AdaptiveMinTier(dir, 50)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.Equal(t, 10, got.CheapAttempts)
	assert.InDelta(t, 0.50, got.CheapSuccessRate, 0.001)
}

func TestAdaptiveMinTierAtThresholdStaysCheap(t *testing.T) {
	dir := t.TempDir()
	// 5 cheap-tier attempts, 1 success → exactly 20% success rate.
	// AC: ">= 0.20 returns cheap" — at the boundary we stay cheap.
	for i := 0; i < 4; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_failed")
	}
	writeAdaptiveFixture(t, dir, 4, "claude", "claude-haiku-4-5", "task_succeeded")

	got := AdaptiveMinTier(dir, 50)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.InDelta(t, 0.20, got.CheapSuccessRate, 0.001)
}

func TestAdaptiveMinTierInsufficientSamplesStaysCheap(t *testing.T) {
	dir := t.TempDir()
	// Only 2 cheap-tier attempts, both failed → rate is 0, but sample count is
	// below the min-samples safeguard so the cheap tier is not suppressed.
	writeAdaptiveFixture(t, dir, 0, "claude", "claude-haiku-4-5", "task_failed")
	writeAdaptiveFixture(t, dir, 1, "claude", "claude-haiku-4-5", "task_failed")

	got := AdaptiveMinTier(dir, 50)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.Equal(t, 2, got.CheapAttempts)
}

func TestAdaptiveMinTierWindowTruncatesToRecent(t *testing.T) {
	dir := t.TempDir()
	// First 10 attempts: all successes (old history).
	for i := 0; i < 10; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_succeeded")
	}
	// Next 10 attempts: all failures (recent history).
	for i := 10; i < 20; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_failed")
	}

	// Window of 10 sees only the recent failing batch — cheap-tier rate is 0.
	got := AdaptiveMinTier(dir, 10)
	assert.Equal(t, TierStandard, got.Tier)
	assert.True(t, got.Skipped)
	assert.Equal(t, 10, got.CheapAttempts)

	// Window of 20 sees the whole span — cheap-tier rate is 50%, stays cheap.
	got = AdaptiveMinTier(dir, 20)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.Equal(t, 20, got.CheapAttempts)
}

func TestAdaptiveMinTierIgnoresNonCheapAttempts(t *testing.T) {
	dir := t.TempDir()
	// 5 standard-tier successes and 5 smart-tier successes do not count.
	for i := 0; i < 5; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-sonnet-4-6", "task_succeeded")
	}
	for i := 5; i < 10; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-opus-4-6", "task_succeeded")
	}
	// 4 cheap attempts, 0 successes → 0% rate, above min-samples.
	for i := 10; i < 14; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_failed")
	}

	got := AdaptiveMinTier(dir, 50)
	assert.Equal(t, TierStandard, got.Tier)
	assert.True(t, got.Skipped)
	assert.Equal(t, 4, got.CheapAttempts)
	assert.InDelta(t, 0.0, got.CheapSuccessRate, 0.001)
}

func TestAdaptiveMinTierIgnoresUnknownHarnessModel(t *testing.T) {
	dir := t.TempDir()
	// Ad-hoc model pin not in the catalog — does not map to any tier.
	for i := 0; i < 10; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "some-custom-pin-v1", "task_failed")
	}

	got := AdaptiveMinTier(dir, 50)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.Equal(t, 0, got.CheapAttempts, "attempts with non-catalog models must not count")
}
