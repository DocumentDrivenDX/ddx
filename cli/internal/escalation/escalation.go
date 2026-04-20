package escalation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// AdaptiveMinTierThreshold is the cheap-tier trailing success rate below which
// AdaptiveMinTier recommends skipping the cheap tier. At 0.20, four out of
// five cheap-tier attempts must fail before the cheap tier is suppressed.
const AdaptiveMinTierThreshold = 0.20

// AdaptiveMinTierMinSamples is the minimum number of cheap-tier attempts
// required in the window before the cheap-tier success rate is considered
// statistically meaningful. Below this the cheap tier is kept in-range so
// we do not starve it on insufficient evidence.
const AdaptiveMinTierMinSamples = 3

// SuccessStatus mirrors agent.ExecuteBeadStatusSuccess. The agent package's
// TestEscalatableStatusesMatchAgentVocab guard catches drift if either side
// renames. Defined locally so escalation does not import agent.
const SuccessStatus = "success"

// EscalatableStatuses is the set of executor status strings that warrant
// retrying with a higher tier. Mirrors a subset of agent.ExecuteBeadStatus*
// strings; the agent package has a TestEscalatableStatusesMatchAgentVocab
// guard (see agent/tier_escalation_alignment_test.go) to catch drift.
var EscalatableStatuses = map[string]bool{
	"execution_failed":      true,
	"no_changes":            true,
	"post_run_check_failed": true,
	"land_conflict":         true,
}

// AdaptiveMinTierResult carries the recommendation from AdaptiveMinTier along
// with the observed cheap-tier sample count and success rate so callers can
// emit a log line explaining the decision.
type AdaptiveMinTierResult struct {
	// Tier is the recommended minimum tier: TierCheap when the cheap tier is
	// viable, TierStandard when cheap-tier success is below the threshold.
	Tier ModelTier
	// CheapAttempts is the number of cheap-tier attempts observed in the
	// window. Zero when no cheap-tier history was found.
	CheapAttempts int
	// CheapSuccessRate is CheapSuccesses / CheapAttempts, or 0 when
	// CheapAttempts is 0.
	CheapSuccessRate float64
	// Skipped is true when the recommendation is to skip the cheap tier.
	Skipped bool
}

// taskResultLite is a minimal projection of agent.ExecuteBeadResult that
// captures only the fields AdaptiveMinTier needs. Defined locally so the
// escalation package does not depend on the agent package.
type taskResultLite struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Outcome string `json:"outcome"`
}

// TierResolver resolves a (harness, tier) pair to a concrete model name. This
// is normally agent.ResolveModelTier; injected as a parameter so the
// escalation package does not import agent.
type TierResolver func(harness string, tier ModelTier) string

// AdaptiveMinTier inspects the most recent `window` entries under
// workingDir/.ddx/executions/*/result.json, computes the cheap-tier trailing
// success rate, and returns a recommendation:
//
//   - When cheap-tier success rate < AdaptiveMinTierThreshold (and at least
//     AdaptiveMinTierMinSamples cheap attempts were observed), returns
//     TierStandard with Skipped=true.
//   - Otherwise returns TierCheap with Skipped=false.
//
// A tier is identified by resolving (harness, model) back through the
// supplied resolver — attempts whose harness/model do not match any known
// tier mapping are ignored for the purpose of this calculation. When
// workingDir has no executions directory (e.g. a fresh project), the cheap
// tier is kept in-range so a first run is not artificially restricted.
func AdaptiveMinTier(workingDir string, window int, resolver TierResolver) AdaptiveMinTierResult {
	execRoot := filepath.Join(workingDir, ".ddx", "executions")
	entries, err := os.ReadDir(execRoot)
	if err != nil {
		return AdaptiveMinTierResult{Tier: TierCheap}
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	// Directory names are sortable timestamps (YYYYMMDDTHHMMSS-<hash>), so
	// lexicographic sort is chronological.
	sort.Strings(names)

	// Collect usable attempts, then truncate to the most recent `window`.
	type attempt struct {
		tier    ModelTier
		success bool
	}
	collected := make([]attempt, 0, len(names))
	for _, name := range names {
		resultPath := filepath.Join(execRoot, name, "result.json")
		raw, err := os.ReadFile(resultPath)
		if err != nil {
			continue
		}
		var res taskResultLite
		if err := json.Unmarshal(raw, &res); err != nil {
			continue
		}
		if res.Harness == "" {
			continue
		}
		tier := classifyAttemptTier(res.Harness, res.Model, resolver)
		if tier == "" {
			continue
		}
		collected = append(collected, attempt{
			tier:    tier,
			success: res.Outcome == "task_succeeded",
		})
	}
	if window > 0 && len(collected) > window {
		collected = collected[len(collected)-window:]
	}

	var cheapAttempts, cheapSuccesses int
	for _, a := range collected {
		if a.tier == TierCheap {
			cheapAttempts++
			if a.success {
				cheapSuccesses++
			}
		}
	}

	result := AdaptiveMinTierResult{Tier: TierCheap, CheapAttempts: cheapAttempts}
	if cheapAttempts > 0 {
		result.CheapSuccessRate = float64(cheapSuccesses) / float64(cheapAttempts)
	}
	if cheapAttempts >= AdaptiveMinTierMinSamples && result.CheapSuccessRate < AdaptiveMinTierThreshold {
		result.Tier = TierStandard
		result.Skipped = true
	}
	return result
}

// classifyAttemptTier returns the ModelTier that corresponds to a (harness,
// model) pair by reverse-lookup against the supplied resolver. Returns ""
// when no tier in TierOrder matches — e.g. an ad-hoc model pin not present
// in the catalog, which should not contribute to tier-level analytics. A
// nil resolver is treated as no-match for every tier.
func classifyAttemptTier(harness, model string, resolver TierResolver) ModelTier {
	if harness == "" || resolver == nil {
		return ""
	}
	for _, tier := range TierOrder {
		if resolver(harness, tier) == model {
			return tier
		}
	}
	return ""
}

// TierOrder defines the escalation sequence from cheapest to most capable.
var TierOrder = []ModelTier{TierCheap, TierStandard, TierSmart}

// ProviderCooldownDuration is how long an unhealthy harness is skipped before
// re-probing. Five minutes gives most transient errors time to resolve without
// locking out a provider for too long.
//
// Per-harness cooldown tracking moved to the upstream agent service in v0.8.0:
// callers record failures via svc.RecordRouteAttempt and read cooldown state
// via svc.RouteStatus's RouteCandidateStatus.Healthy (ddx-7bc0c8d5).
const ProviderCooldownDuration = 5 * time.Minute

// tierIndex returns the position of t in TierOrder, or -1 if not found.
func tierIndex(t ModelTier) int {
	for i, tier := range TierOrder {
		if tier == t {
			return i
		}
	}
	return -1
}

// TiersInRange returns the subset of TierOrder from minTier to maxTier inclusive.
// Empty string defaults to the global extremes (cheap and smart).
// If minTier > maxTier in the order, an empty slice is returned.
func TiersInRange(minTier, maxTier ModelTier) []ModelTier {
	if minTier == "" {
		minTier = TierCheap
	}
	if maxTier == "" {
		maxTier = TierSmart
	}
	minIdx := tierIndex(minTier)
	maxIdx := tierIndex(maxTier)
	if minIdx < 0 {
		minIdx = 0
	}
	if maxIdx < 0 {
		maxIdx = len(TierOrder) - 1
	}
	if minIdx > maxIdx {
		return nil
	}
	// Return a copy so callers cannot mutate TierOrder.
	out := make([]ModelTier, maxIdx-minIdx+1)
	copy(out, TierOrder[minIdx:maxIdx+1])
	return out
}

// ShouldEscalate reports whether status warrants escalating to the next tier.
// Structural failures (e.g. validation errors) do not escalate because a
// smarter model cannot fix a malformed prompt or corrupted bead state.
func ShouldEscalate(status string) bool {
	return EscalatableStatuses[status]
}

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
