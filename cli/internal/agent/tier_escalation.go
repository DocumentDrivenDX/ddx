package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
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

// AdaptiveMinTier inspects the most recent `window` entries under
// workingDir/.ddx/executions/*/result.json, computes the cheap-tier trailing
// success rate, and returns a recommendation:
//
//   - When cheap-tier success rate < AdaptiveMinTierThreshold (and at least
//     AdaptiveMinTierMinSamples cheap attempts were observed), returns
//     TierStandard with Skipped=true.
//   - Otherwise returns TierCheap with Skipped=false.
//
// A tier is identified by resolving (harness, model) back through
// ResolveModelTier — attempts whose harness/model do not match any known
// tier mapping are ignored for the purpose of this calculation. When
// workingDir has no executions directory (e.g. a fresh project), the cheap
// tier is kept in-range so a first run is not artificially restricted.
func AdaptiveMinTier(workingDir string, window int) AdaptiveMinTierResult {
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
		var res ExecuteBeadResult
		if err := json.Unmarshal(raw, &res); err != nil {
			continue
		}
		if res.Harness == "" {
			continue
		}
		tier := classifyAttemptTier(res.Harness, res.Model)
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
// model) pair by reverse-lookup against ResolveModelTier. Returns "" when
// no tier in TierOrder matches — e.g. an ad-hoc model pin not present in
// the catalog, which should not contribute to tier-level analytics.
func classifyAttemptTier(harness, model string) ModelTier {
	if harness == "" {
		return ""
	}
	for _, tier := range TierOrder {
		if ResolveModelTier(harness, tier) == model {
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
const ProviderCooldownDuration = 5 * time.Minute

// ProviderHealthTracker tracks process-local provider health with per-harness
// cooldowns. When a harness probe fails (network error, 502, auth rejected),
// the harness is marked unhealthy for ProviderCooldownDuration. Subsequent
// routing attempts skip unhealthy harnesses so the queue can continue with
// working providers.
type ProviderHealthTracker struct {
	mu        sync.Mutex
	unhealthy map[string]time.Time // harness name → unhealthy until
}

// NewProviderHealthTracker creates an empty tracker.
func NewProviderHealthTracker() *ProviderHealthTracker {
	return &ProviderHealthTracker{unhealthy: make(map[string]time.Time)}
}

// GlobalProviderHealth is the process-local singleton shared across workers
// and execute-loop iterations. One worker's probe result benefits all others
// in the same process.
var GlobalProviderHealth = NewProviderHealthTracker()

// Mark marks harness as unhealthy until the given time.
func (h *ProviderHealthTracker) Mark(harness string, until time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.unhealthy[harness] = until
}

// IsHealthy returns true when harness has no active cooldown.
func (h *ProviderHealthTracker) IsHealthy(harness string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	until, ok := h.unhealthy[harness]
	if !ok {
		return true
	}
	if time.Now().After(until) {
		delete(h.unhealthy, harness)
		return true
	}
	return false
}

// Snapshot returns a copy of active (not yet expired) cooldowns.
func (h *ProviderHealthTracker) Snapshot() map[string]time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	out := make(map[string]time.Time, len(h.unhealthy))
	for k, v := range h.unhealthy {
		if v.After(now) {
			out[k] = v
		}
	}
	return out
}

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
	switch status {
	case ExecuteBeadStatusExecutionFailed,
		ExecuteBeadStatusNoChanges,
		ExecuteBeadStatusPostRunCheckFailed,
		ExecuteBeadStatusLandConflict:
		return true
	}
	return false
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
