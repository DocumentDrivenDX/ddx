package escalation

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

// --- Local mirrors of agent.ExecuteBeadStatus* so these tests do not import
// the agent package. Their string values must stay in sync with
// cli/internal/agent/execute_bead_status.go; the agent package's
// TestEscalatableStatusesMatchAgentVocab guards against drift for the
// statuses that drive escalation.
const (
	statusExecutionFailed            = "execution_failed"
	statusNoChanges                  = "no_changes"
	statusPostRunCheckFailed         = "post_run_check_failed"
	statusLandConflict               = "land_conflict"
	statusStructuralValidationFailed = "structural_validation_failed"
	statusAlreadySatisfied           = "already_satisfied"
	statusSuccess                    = "success"
)

// testResolver mimics agent.ResolveModelTier for the claude harness, which is
// the only harness the AdaptiveMinTier tests exercise. Unknown (harness, tier)
// pairs resolve to "" so ad-hoc model pins are correctly classified as
// non-tier attempts.
func testResolver(harness string, tier ModelTier) string {
	if harness != "claude" {
		return ""
	}
	switch tier {
	case TierCheap:
		return "claude-haiku-4-5"
	case TierStandard:
		return "claude-sonnet-4-6"
	case TierSmart:
		return "claude-opus-4-6"
	}
	return ""
}

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
		{Tier: "cheap", Harness: "agent", Model: "cheap-model", Status: statusExecutionFailed, CostUSD: 0.02, DurationMS: 1200},
		{Tier: "standard", Harness: "codex", Model: "standard-model", Status: statusNoChanges, CostUSD: 0.15, DurationMS: 3400},
		{Tier: "smart", Harness: "claude", Model: "smart-model", Status: statusSuccess, CostUSD: 0.80, DurationMS: 9000},
	}

	summary := BuildEscalationSummary(attempts, "smart")
	require.Equal(t, "smart", summary.WinningTier)
	require.Len(t, summary.TiersAttempted, 3)
	assert.InDelta(t, 0.97, summary.TotalCostUSD, 1e-9)
	assert.InDelta(t, 0.17, summary.WastedCostUSD, 1e-9, "cheap (0.02) + standard (0.15) wasted, smart succeeded")
	assert.Equal(t, "cheap", summary.TiersAttempted[0].Tier)
	assert.Equal(t, statusExecutionFailed, summary.TiersAttempted[0].Status)
	assert.Equal(t, statusNoChanges, summary.TiersAttempted[1].Status)
	assert.Equal(t, statusSuccess, summary.TiersAttempted[2].Status)

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
	assert.Equal(t, statusExecutionFailed, decoded.TiersAttempted[0].Status)
	assert.InDelta(t, 0.02, decoded.TiersAttempted[0].CostUSD, 1e-9)
	assert.Equal(t, int64(1200), decoded.TiersAttempted[0].DurationMS)
	assert.InDelta(t, 0.97, decoded.TotalCostUSD, 1e-9)
	assert.InDelta(t, 0.17, decoded.WastedCostUSD, 1e-9)
}

// TestEscalationSummaryExhausted verifies the exhausted case (no tier
// succeeded): winning_tier is "exhausted" and all costs are wasted.
func TestEscalationSummaryExhausted(t *testing.T) {
	attempts := []TierAttemptRecord{
		{Tier: "cheap", Harness: "agent", Model: "c", Status: statusExecutionFailed, CostUSD: 0.01, DurationMS: 900},
		{Tier: "smart", Harness: "claude", Model: "s", Status: statusExecutionFailed, CostUSD: 0.50, DurationMS: 7000},
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
		{Tier: "cheap", Status: statusSuccess, CostUSD: 0.05},
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
	assert.True(t, ShouldEscalate(statusExecutionFailed))
}

func TestShouldEscalateNoChangesIsFalse(t *testing.T) {
	assert.False(t, ShouldEscalate(statusNoChanges))
}

func TestShouldEscalatePostRunCheckFailed(t *testing.T) {
	assert.True(t, ShouldEscalate(statusPostRunCheckFailed))
}

func TestShouldEscalateLandConflict(t *testing.T) {
	assert.True(t, ShouldEscalate(statusLandConflict))
}

func TestShouldEscalateSuccessIsFalse(t *testing.T) {
	assert.False(t, ShouldEscalate(statusSuccess))
}

func TestShouldEscalateStructuralValidationIsTrue(t *testing.T) {
	assert.True(t, ShouldEscalate(statusStructuralValidationFailed))
}

func TestShouldEscalateAlreadySatisfiedIsFalse(t *testing.T) {
	assert.False(t, ShouldEscalate(statusAlreadySatisfied))
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

// adaptiveFixture is the JSON shape AdaptiveMinTier reads from
// .ddx/executions/<ts>/result.json. It is a subset of agent.ExecuteBeadResult
// — escalation's taskResultLite reads Harness/Model/Outcome only — but the
// test writes BeadID too so result.json files remain recognizable to
// ad-hoc inspection.
type adaptiveFixture struct {
	BeadID  string `json:"bead_id"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Outcome string `json:"outcome"`
}

// writeAdaptiveFixture writes a result.json for a synthetic attempt under
// workingDir/.ddx/executions/<ts>/result.json. The `seq` parameter seeds the
// timestamp so the lexicographic ordering matches chronological order.
func writeAdaptiveFixture(t *testing.T, workingDir string, seq int, harness, model, outcome string) {
	t.Helper()
	ts := fmt.Sprintf("20260101T%06d-%08x", seq, seq)
	dir := filepath.Join(workingDir, ".ddx", "executions", ts)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	res := adaptiveFixture{
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
	got := AdaptiveMinTier(dir, 50, testResolver)
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

	got := AdaptiveMinTier(dir, 50, testResolver)
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

	got := AdaptiveMinTier(dir, 50, testResolver)
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

	got := AdaptiveMinTier(dir, 50, testResolver)
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

	got := AdaptiveMinTier(dir, 50, testResolver)
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
	got := AdaptiveMinTier(dir, 10, testResolver)
	assert.Equal(t, TierStandard, got.Tier)
	assert.True(t, got.Skipped)
	assert.Equal(t, 10, got.CheapAttempts)

	// Window of 20 sees the whole span — cheap-tier rate is 50%, stays cheap.
	got = AdaptiveMinTier(dir, 20, testResolver)
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

	got := AdaptiveMinTier(dir, 50, testResolver)
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

	got := AdaptiveMinTier(dir, 50, testResolver)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.Equal(t, 0, got.CheapAttempts, "attempts with non-catalog models must not count")
}

// --- Infra failure exclusion (AC3) ---

// adaptiveFixtureWithMode is the full shape written by the new tests that
// exercise failure_mode exclusion. It extends adaptiveFixture with FailureMode.
type adaptiveFixtureWithMode struct {
	BeadID      string `json:"bead_id"`
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Outcome     string `json:"outcome"`
	FailureMode string `json:"failure_mode,omitempty"`
}

// writeAdaptiveFixtureWithMode writes a result.json with an explicit
// failure_mode under workingDir/.ddx/executions/<ts>/result.json.
func writeAdaptiveFixtureWithMode(t *testing.T, workingDir string, seq int, harness, model, outcome, failureMode string) {
	t.Helper()
	ts := fmt.Sprintf("20260101T%06d-%08x", seq, seq)
	dir := filepath.Join(workingDir, ".ddx", "executions", ts)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	res := adaptiveFixtureWithMode{
		BeadID:      fmt.Sprintf("bead-%d", seq),
		Harness:     harness,
		Model:       model,
		Outcome:     outcome,
		FailureMode: failureMode,
	}
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), raw, 0o644))
}

// TestAdaptiveMinTierInfraFailuresExcluded verifies that cheap-tier attempts
// with an infra failure_mode (no_viable_provider, harness_not_installed) do
// not count toward the success-rate computation and are reported as
// CheapInfraSkipped in the diagnostics.
func TestAdaptiveMinTierInfraFailuresExcluded(t *testing.T) {
	dir := t.TempDir()
	// 5 infra failures (no_viable_provider) — should not trigger suppression.
	for i := 0; i < 5; i++ {
		writeAdaptiveFixtureWithMode(t, dir, i, "claude", "claude-haiku-4-5", "task_failed", "no_viable_provider")
	}
	// 1 real failure — below min-samples, no suppression.
	writeAdaptiveFixtureWithMode(t, dir, 5, "claude", "claude-haiku-4-5", "task_failed", "")

	got := AdaptiveMinTier(dir, 50, testResolver)
	assert.Equal(t, TierCheap, got.Tier, "infra failures must not trigger cheap-tier suppression")
	assert.False(t, got.Skipped)
	assert.Equal(t, 1, got.CheapAttempts, "only real failures count toward cheap attempts")

	diag := ComputeAdaptiveMinTierDiagnostics(dir, 50, testResolver)
	assert.Equal(t, 5, diag.CheapInfraSkipped, "infra skip count must be reported")
	assert.Equal(t, 1, diag.CheapAttempts)
	assert.False(t, diag.IsSkippingCheap)
}

// TestAdaptiveMinTierHarnessNotInstalledExcluded mirrors the infra test for
// the harness_not_installed failure mode.
func TestAdaptiveMinTierHarnessNotInstalledExcluded(t *testing.T) {
	dir := t.TempDir()
	// 9 harness_not_installed failures — should NOT suppress cheap tier.
	for i := 0; i < 9; i++ {
		writeAdaptiveFixtureWithMode(t, dir, i, "claude", "claude-haiku-4-5", "task_failed", "harness_not_installed")
	}

	got := AdaptiveMinTier(dir, 50, testResolver)
	assert.Equal(t, TierCheap, got.Tier)
	assert.False(t, got.Skipped)
	assert.Equal(t, 0, got.CheapAttempts)

	diag := ComputeAdaptiveMinTierDiagnostics(dir, 50, testResolver)
	assert.Equal(t, 9, diag.CheapInfraSkipped)
}

// TestAdaptiveMinTierMixedRealAndInfraFailures verifies that real failures
// below threshold still suppress cheap tier even when infra failures exist.
func TestAdaptiveMinTierMixedRealAndInfraFailures(t *testing.T) {
	dir := t.TempDir()
	// 5 infra failures (excluded) + 9 real failures + 1 success = 10% real rate.
	for i := 0; i < 5; i++ {
		writeAdaptiveFixtureWithMode(t, dir, i, "claude", "claude-haiku-4-5", "task_failed", "no_viable_provider")
	}
	for i := 5; i < 14; i++ {
		writeAdaptiveFixtureWithMode(t, dir, i, "claude", "claude-haiku-4-5", "task_failed", "")
	}
	writeAdaptiveFixtureWithMode(t, dir, 14, "claude", "claude-haiku-4-5", "task_succeeded", "")

	got := AdaptiveMinTier(dir, 50, testResolver)
	assert.Equal(t, TierStandard, got.Tier, "real failures below threshold should still suppress cheap tier")
	assert.True(t, got.Skipped)
	assert.Equal(t, 10, got.CheapAttempts)
	assert.InDelta(t, 0.10, got.CheapSuccessRate, 0.001)
}

// --- Reset marker (AC1, AC5) ---

func TestWriteAndReadAdaptiveResetMarker(t *testing.T) {
	dir := t.TempDir()
	before := time.Now().UTC().Truncate(time.Second)
	path, err := WriteAdaptiveResetMarker(dir)
	require.NoError(t, err)
	assert.Contains(t, path, adaptiveResetFileName)

	m := ReadAdaptiveResetMarker(dir)
	require.NotNil(t, m, "marker must be readable after write")
	assert.False(t, m.ResetAt.IsZero())
	assert.True(t, !m.ResetAt.Before(before), "reset_at must not predate the write call")
}

func TestReadAdaptiveResetMarkerMissing(t *testing.T) {
	dir := t.TempDir()
	assert.Nil(t, ReadAdaptiveResetMarker(dir), "missing marker must return nil")
}

func TestAdaptiveMinTierResetMarkerIgnoresOldExecutions(t *testing.T) {
	dir := t.TempDir()
	// Write 9 cheap failures with old timestamps (seq 0..8 → 20260101T000000-...
	// through 20260101T000008-...).
	for i := 0; i < 9; i++ {
		writeAdaptiveFixture(t, dir, i, "claude", "claude-haiku-4-5", "task_failed")
	}

	// Before reset: cheap tier is suppressed.
	got := AdaptiveMinTier(dir, 50, testResolver)
	assert.Equal(t, TierStandard, got.Tier, "pre-reset: cheap tier should be suppressed")
	assert.True(t, got.Skipped)

	// Write the reset marker with a timestamp that predates our fixtures'
	// directory names but is still "after" them in our fictional time. Since
	// fixture timestamps are in 2026-01-01 and the reset marker uses time.Now()
	// (2026-04-29), the reset marker is AFTER all fixtures → all are ignored.
	path, err := WriteAdaptiveResetMarker(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	// After reset: no executions in window → cheap tier is eligible.
	got = AdaptiveMinTier(dir, 50, testResolver)
	assert.Equal(t, TierCheap, got.Tier, "post-reset: cheap tier must be eligible")
	assert.False(t, got.Skipped)
	assert.Equal(t, 0, got.CheapAttempts, "all pre-reset executions must be excluded")
}

// --- parseDirTimestamp ---

func TestParseDirTimestamp(t *testing.T) {
	cases := []struct {
		name  string
		wantY int
		wantM time.Month
		wantD int
		ok    bool
	}{
		{"20260429T162702-2c963978", 2026, time.April, 29, true},
		{"20260101T000000-00000000", 2026, time.January, 1, true},
		{"", 0, 0, 0, false},
		{"short", 0, 0, 0, false},
		{"not-a-timestamp-at-all-x", 0, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts, ok := parseDirTimestamp(c.name)
			assert.Equal(t, c.ok, ok)
			if c.ok {
				assert.Equal(t, c.wantY, ts.Year())
				assert.Equal(t, c.wantM, ts.Month())
				assert.Equal(t, c.wantD, ts.Day())
				assert.Equal(t, time.UTC, ts.Location())
			}
		})
	}
}
