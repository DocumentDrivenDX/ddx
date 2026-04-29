package escalation

import (
	"encoding/json"
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
