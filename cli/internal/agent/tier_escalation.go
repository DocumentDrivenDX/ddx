package agent

import (
	"fmt"
	"sync"
	"time"
)

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
