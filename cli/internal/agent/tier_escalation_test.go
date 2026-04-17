package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- EscalationSummary ---

// recordingAppender captures events appended by AppendEscalationSummaryEvent
// so tests can assert on the kind, summary, and JSON body.
type recordingAppender struct {
	events []struct {
		id    string
		event bead.BeadEvent
	}
}

func (r *recordingAppender) AppendEvent(id string, event bead.BeadEvent) error {
	r.events = append(r.events, struct {
		id    string
		event bead.BeadEvent
	}{id: id, event: event})
	return nil
}

// TestEscalationSummary exercises the 3-tier escalation scenario from the
// bead: cheap fails, standard fails, smart succeeds. The emitted
// kind:escalation-summary event body must contain all three tier records
// with correct statuses, costs, and a winning_tier/wasted_cost_usd roll-up.
func TestEscalationSummary(t *testing.T) {
	attempts := []TierAttemptRecord{
		{Tier: "cheap", Harness: "agent", Model: "cheap-model", Status: ExecuteBeadStatusExecutionFailed, CostUSD: 0.02, DurationMS: 1200},
		{Tier: "standard", Harness: "codex", Model: "standard-model", Status: ExecuteBeadStatusNoChanges, CostUSD: 0.15, DurationMS: 3400},
		{Tier: "smart", Harness: "claude", Model: "smart-model", Status: ExecuteBeadStatusSuccess, CostUSD: 0.80, DurationMS: 9000},
	}

	summary := BuildEscalationSummary(attempts, "smart")
	require.Equal(t, "smart", summary.WinningTier)
	require.Len(t, summary.TiersAttempted, 3)
	assert.InDelta(t, 0.97, summary.TotalCostUSD, 1e-9)
	assert.InDelta(t, 0.17, summary.WastedCostUSD, 1e-9, "cheap (0.02) + standard (0.15) wasted, smart succeeded")
	assert.Equal(t, "cheap", summary.TiersAttempted[0].Tier)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, summary.TiersAttempted[0].Status)
	assert.Equal(t, ExecuteBeadStatusNoChanges, summary.TiersAttempted[1].Status)
	assert.Equal(t, ExecuteBeadStatusSuccess, summary.TiersAttempted[2].Status)

	// Now wire through AppendEscalationSummaryEvent and verify the emitted
	// event has kind:escalation-summary, a summary line, and a JSON body
	// that round-trips to the same EscalationSummary.
	appender := &recordingAppender{}
	err := AppendEscalationSummaryEvent(appender, "ddx-test-1", "test-actor", attempts, "smart", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Len(t, appender.events, 1)

	ev := appender.events[0]
	assert.Equal(t, "ddx-test-1", ev.id)
	assert.Equal(t, "escalation-summary", ev.event.Kind)
	assert.Equal(t, "test-actor", ev.event.Actor)
	assert.Contains(t, ev.event.Summary, "winning_tier=smart")
	assert.Contains(t, ev.event.Summary, "attempts=3")

	var decoded EscalationSummary
	require.NoError(t, json.Unmarshal([]byte(ev.event.Body), &decoded))
	assert.Equal(t, "smart", decoded.WinningTier)
	require.Len(t, decoded.TiersAttempted, 3)
	assert.Equal(t, "cheap", decoded.TiersAttempted[0].Tier)
	assert.Equal(t, "agent", decoded.TiersAttempted[0].Harness)
	assert.Equal(t, "cheap-model", decoded.TiersAttempted[0].Model)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, decoded.TiersAttempted[0].Status)
	assert.InDelta(t, 0.02, decoded.TiersAttempted[0].CostUSD, 1e-9)
	assert.Equal(t, int64(1200), decoded.TiersAttempted[0].DurationMS)
	assert.InDelta(t, 0.97, decoded.TotalCostUSD, 1e-9)
	assert.InDelta(t, 0.17, decoded.WastedCostUSD, 1e-9)
}

// TestEscalationSummaryExhausted verifies the exhausted case (no tier
// succeeded): winning_tier is "exhausted" and all costs are wasted.
func TestEscalationSummaryExhausted(t *testing.T) {
	attempts := []TierAttemptRecord{
		{Tier: "cheap", Harness: "agent", Model: "c", Status: ExecuteBeadStatusExecutionFailed, CostUSD: 0.01, DurationMS: 900},
		{Tier: "smart", Harness: "claude", Model: "s", Status: ExecuteBeadStatusExecutionFailed, CostUSD: 0.50, DurationMS: 7000},
	}
	summary := BuildEscalationSummary(attempts, "")
	assert.Equal(t, EscalationWinningExhausted, summary.WinningTier)
	assert.InDelta(t, 0.51, summary.TotalCostUSD, 1e-9)
	assert.InDelta(t, 0.51, summary.WastedCostUSD, 1e-9, "every attempt wasted when no tier succeeded")
}

// TestEscalationSummaryBuildSourceSliceIndependent verifies the attempts
// slice in the returned summary is independent of the caller's slice —
// mutating the caller's slice must not change the summary.
func TestEscalationSummaryBuildSourceSliceIndependent(t *testing.T) {
	attempts := []TierAttemptRecord{
		{Tier: "cheap", Status: ExecuteBeadStatusSuccess, CostUSD: 0.05},
	}
	summary := BuildEscalationSummary(attempts, "cheap")
	attempts[0].Tier = "mutated"
	assert.Equal(t, "cheap", summary.TiersAttempted[0].Tier, "summary must hold an independent copy of attempts")
}

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
