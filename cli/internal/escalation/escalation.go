package escalation

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// SuccessStatus mirrors agent.ExecuteBeadStatusSuccess. The agent package's
// TestEscalatableStatusesMatchAgentVocab guard catches drift if either side
// renames. Defined locally so escalation does not import agent.
const SuccessStatus = "success"

// EscalatableStatuses is the set of executor status strings that warrant
// retrying with a higher tier. Mirrors a subset of agent.ExecuteBeadStatus*
// strings; the agent package has a TestEscalatableStatusesMatchAgentVocab
// guard (see agent/tier_escalation_alignment_test.go) to catch drift.
var EscalatableStatuses = map[string]bool{
	"execution_failed":             true,
	"post_run_check_failed":        true,
	"land_conflict":                true,
	"structural_validation_failed": true,
}

// ShouldEscalate reports whether status warrants escalating to the next tier.
// Structural failures (e.g. validation errors) do not escalate because a
// smarter model cannot fix a malformed prompt or corrupted bead state.
func ShouldEscalate(status string) bool {
	return EscalatableStatuses[status]
}

// ProviderCooldownDuration is how long an unhealthy harness is skipped before
// re-probing. Five minutes gives most transient errors time to resolve without
// locking out a provider for too long.
//
// Per-harness cooldown tracking moved to the upstream agent service in v0.8.0:
// callers record failures via svc.RecordRouteAttempt and read cooldown state
// via svc.RouteStatus's RouteCandidateStatus.Healthy (ddx-7bc0c8d5).
const ProviderCooldownDuration = 5 * time.Minute

// FormatTierAttemptBody formats the body of a tier-attempt bead event.
func FormatTierAttemptBody(tier, harness, model, probeResult, detail string) string {
	body := fmt.Sprintf("tier=%s harness=%s model=%s", tier, harness, model)
	if probeResult != "" {
		body += " probe=" + probeResult
	}
	if detail != "" {
		body += "\n" + detail
	}
	return body
}

// EscalationWinningExhausted is the sentinel value written into the
// winning_tier field of an escalation-summary body when the escalation loop
// ran through every eligible tier without producing a successful attempt.
const EscalationWinningExhausted = "exhausted"

// TierAttemptRecord is one row of an escalation trace. It captures the
// tier/harness/model that was tried, the status that attempt returned, and
// the cost/duration the harness reported for that attempt. Skipped tiers
// (no viable provider) are recorded with zero cost and zero duration.
type TierAttemptRecord struct {
	Tier       string  `json:"tier"`
	Harness    string  `json:"harness,omitempty"`
	Model      string  `json:"model,omitempty"`
	Status     string  `json:"status"`
	CostUSD    float64 `json:"cost_usd"`
	DurationMS int64   `json:"duration_ms"`
}

// EscalationSummary is the structured body of a kind:escalation-summary bead
// event. It captures the entire escalation trace so an operator can diagnose
// which tiers were tried, which one won (if any), and how much the path cost.
type EscalationSummary struct {
	TiersAttempted []TierAttemptRecord `json:"tiers_attempted"`
	WinningTier    string              `json:"winning_tier"`
	TotalCostUSD   float64             `json:"total_cost_usd"`
	WastedCostUSD  float64             `json:"wasted_cost_usd"`
}

// BuildEscalationSummary computes the summary body from the ordered list of
// attempts. winningTier is the string of the tier whose attempt succeeded;
// pass "" when the escalation was exhausted, in which case winning_tier is
// set to EscalationWinningExhausted. Total cost is the sum of all attempt
// costs; wasted cost is the sum of attempts whose status is not
// SuccessStatus.
func BuildEscalationSummary(attempts []TierAttemptRecord, winningTier string) EscalationSummary {
	out := EscalationSummary{
		TiersAttempted: append([]TierAttemptRecord{}, attempts...),
		WinningTier:    winningTier,
	}
	if out.WinningTier == "" {
		out.WinningTier = EscalationWinningExhausted
	}
	for _, a := range attempts {
		out.TotalCostUSD += a.CostUSD
		if a.Status != SuccessStatus {
			out.WastedCostUSD += a.CostUSD
		}
	}
	return out
}

// BeadEventAppender records append-only evidence events on a bead. Mirrors
// agent.BeadEventAppender so escalation can append events without importing
// agent. *bead.Store satisfies both interfaces.
type BeadEventAppender interface {
	AppendEvent(id string, event bead.BeadEvent) error
}

// AppendEscalationSummaryEvent writes a kind:escalation-summary event to the
// bead with a JSON body describing the tier escalation trace. It is a
// best-effort operation — errors from the underlying store are returned so
// callers can log them, but callers typically ignore the error so telemetry
// failures never abort the main execute-bead flow.
func AppendEscalationSummaryEvent(appender BeadEventAppender, beadID, actor string, attempts []TierAttemptRecord, winningTier string, createdAt time.Time) error {
	if appender == nil || beadID == "" {
		return nil
	}
	summary := BuildEscalationSummary(attempts, winningTier)
	body, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	return appender.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "escalation-summary",
		Summary:   fmt.Sprintf("winning_tier=%s attempts=%d total_cost_usd=%.4f wasted_cost_usd=%.4f", summary.WinningTier, len(attempts), summary.TotalCostUSD, summary.WastedCostUSD),
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: createdAt,
	})
}
