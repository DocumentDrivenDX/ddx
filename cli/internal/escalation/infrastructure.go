package escalation

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// InfrastructureFailurePatterns are detail substrings that indicate a
// transient infrastructure-level failure (provider 502, network unreachable,
// command not found, auth/quota exhausted) rather than a model-capability
// failure (test failed, build failed, no changes after attempt).
//
// Infrastructure failures should not consume escalation budget — the bead
// should be deferred with a retry-after, not retried at the next powerClass with
// a more expensive model. The model wasn't given a fair chance.
//
// Patterns are matched case-insensitively as substrings of the failure
// detail string. Any new pattern added here is automatically picked up by
// IsInfrastructureFailure callers.
var InfrastructureFailurePatterns = []string{
	// HTTP-level provider unavailability
	"502", "503", "504",
	"bad gateway",
	"service unavailable",
	"gateway timeout",
	// Network-level unreachability
	"connection refused",
	"no such host",
	"no route to host",
	"network is unreachable",
	"i/o timeout",
	"context deadline exceeded",
	// Auth/quota exhaustion (operator-fixable, not a model fault)
	"401", "429",
	"unauthorized",
	"rate limit",
	"ratelimit",
	"quota exceeded",
	"insufficient quota",
	"insufficient_quota",
	// Subprocess harness binary missing or unstartable
	"command not found",
	"executable file not found",
	"no such file or directory",
	// Upstream routing/service selection failures
	"resolveroute:",
	"no viable routing candidate",
	"no live provider supports",
	"no candidate satisfying local endpoint",
}

// IsInfrastructureFailure reports whether the given failure status + detail
// indicates a transient infrastructure problem the model could not have
// fixed. Only execution_failed can be infrastructure; other escalatable
// statuses are semantic outcomes that should proceed through the powerClass ladder.
// Returns false for statuses whose detail does not match any known
// infrastructure pattern.
//
// Used by the work to decide whether to (a) defer the bead with a
// retry-after and try the same powerClass later, or (b) burn through to the next
// powerClass as the standard escalation policy.
func IsInfrastructureFailure(status, detail string) bool {
	if status != "execution_failed" {
		return false
	}
	if detail == "" {
		return false
	}
	lower := strings.ToLower(detail)
	for _, p := range InfrastructureFailurePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// DefaultMaxCostUSD is the per-work dollar cap on accumulated billed
// spend. Exceeding the cap halts further bead claiming so an work
// run cannot silently burn through arbitrary openrouter / pay-per-token
// credits. Local LLMs and subscription-bundled providers do not count
// toward the cap (see CountsTowardCostCap).
//
// $100 is large enough not to interrupt normal subscription-driven work
// (where billed cost stays at $0) and small enough to surface a clear stop
// signal before runaway openrouter usage.
const DefaultMaxCostUSD = 100.0

// DefaultMaxBeadCostUSD is the default per-bead budget. When a single bead
// exhausts this amount across all escalation powerClasses, escalation stops and the
// bead is returned to open without a cooldown. Operators can override
// per-bead with a budget:<USD> label (e.g. budget:25.0).
//
// $5 is a conservative default — enough to attempt two or three smart-powerClass
// runs before halting, but low enough to protect against runaway escalation
// on a pathologically difficult bead.
const DefaultMaxBeadCostUSD = 5.0

// DefaultMaxRecoveryCostUSD is the per-bead recovery budget for automated
// reframe/decompose attempts after repeated ladder exhaustion.
const DefaultMaxRecoveryCostUSD = 2.0

// PerBeadBudgetExhaustedReason is the detail-string prefix written into a
// report when the per-bead budget is exceeded. The work detects this
// marker to handle the outcome correctly (unclaim, event, counter increment,
// no cooldown).
const PerBeadBudgetExhaustedReason = "per-bead budget exhausted"

// CostClassBilled is the HarnessInfo.CostClass value the agent uses for
// pay-per-token providers. Free local providers report "free" and
// subscription-bundled providers report "subscription".
const CostClassBilled = "expensive"

// CountsTowardCostCap returns true when an attempt's reported CostUSD
// should accumulate against the loop's MaxCostUSD. Callers pass the
// HarnessInfo of the harness that ran the attempt; when info is nil the
// safe default is to count (treat unknown as billed).
func CountsTowardCostCap(isLocal, isSubscription bool, costClass string) bool {
	if isLocal {
		return false
	}
	if isSubscription {
		return false
	}
	// CostClass "free" or "subscription" never bill; only "expensive" does.
	if costClass != "" && costClass != CostClassBilled {
		return false
	}
	return true
}

// HarnessBilledLookup reports whether a harness's reported CostUSD should
// be accumulated against a CostCapTracker. Implementations typically
// consult a service.ListHarnesses snapshot. A nil lookup is treated as
// "count by default" (the safe option for unknown harnesses).
type HarnessBilledLookup func(harnessName string) bool

// CostCapTracker accumulates billed cost for a single work run
// and reports when accumulated spend has reached the configured cap.
// Safe for concurrent use across goroutines (workers may call Add and
// Tripped concurrently).
//
// MaxUSD == 0 disables the cap; Tripped always returns (_, false). The
// Lookup callback decides which harnesses contribute to the running
// total — local and subscription-bundled providers should be excluded
// (see CountsTowardCostCap).
type CostCapTracker struct {
	MaxUSD float64
	Lookup HarnessBilledLookup

	mu    sync.Mutex
	spent float64
	// cache memoizes Lookup results so we do not rebuild a service for
	// every reported attempt. Protected by mu.
	cache map[string]bool
}

// NewCostCapTracker constructs a tracker with the given dollar cap and
// lookup. Pass maxUSD <= 0 to disable the cap. A nil lookup is treated
// as "count by default" — every harness contributes.
func NewCostCapTracker(maxUSD float64, lookup HarnessBilledLookup) *CostCapTracker {
	return &CostCapTracker{
		MaxUSD: maxUSD,
		Lookup: lookup,
		cache:  map[string]bool{},
	}
}

// counts reports whether harnessName's CostUSD should accumulate. The
// result is memoized; callers that want fresh resolution should
// construct a new tracker. Lookups for empty harness names default to
// true (count) so accidentally-missing harness metadata never silently
// bypasses the cap.
func (t *CostCapTracker) counts(harnessName string) bool {
	if harnessName == "" {
		return true
	}
	t.mu.Lock()
	if v, ok := t.cache[harnessName]; ok {
		t.mu.Unlock()
		return v
	}
	t.mu.Unlock()
	result := true
	if t.Lookup != nil {
		result = t.Lookup(harnessName)
	}
	t.mu.Lock()
	t.cache[harnessName] = result
	t.mu.Unlock()
	return result
}

// Add accumulates costUSD against the running total when harnessName's
// billing class counts toward the cap. Non-positive cost is ignored.
func (t *CostCapTracker) Add(harnessName string, costUSD float64) {
	if costUSD <= 0 {
		return
	}
	if !t.counts(harnessName) {
		return
	}
	t.mu.Lock()
	t.spent += costUSD
	t.mu.Unlock()
}

// Spent returns the current accumulated billed total.
func (t *CostCapTracker) Spent() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.spent
}

// Tripped reports whether the accumulated spend has reached MaxUSD. When
// MaxUSD <= 0 the cap is disabled and Tripped returns (_, false).
// Returns the formatted operator-facing detail string when tripped so
// callers can populate a stop-the-loop ExecuteBeadReport.
func (t *CostCapTracker) Tripped() (string, bool) {
	if t.MaxUSD <= 0 {
		return "", false
	}
	t.mu.Lock()
	spent := t.spent
	t.mu.Unlock()
	if spent < t.MaxUSD {
		return "", false
	}
	return fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; raise the cap or set 0 to disable. Subscription and local providers do not count.", spent, t.MaxUSD), true
}

// PerBeadCostTracker accumulates billed cost for a single bead across all
// escalation powerClasses and reports when the per-bead budget is exceeded.
// When the budget is exceeded, the caller should stop escalating, unclaim
// the bead, and emit a per_bead_budget_exhausted event (see TD-031 §5).
//
// MaxUSD == 0 disables the cap (unlimited). A nil Lookup counts all harnesses.
// Use ParseBeadBudgetLabel to read a bead-level override from its label set.
type PerBeadCostTracker struct {
	inner *CostCapTracker
}

// NewPerBeadCostTracker constructs a per-bead tracker. Pass maxUSD <= 0 for
// unlimited. Lookup follows the same billing-class convention as CostCapTracker.
func NewPerBeadCostTracker(maxUSD float64, lookup HarnessBilledLookup) *PerBeadCostTracker {
	return &PerBeadCostTracker{inner: NewCostCapTracker(maxUSD, lookup)}
}

// Add accumulates costUSD when the harness counts toward the cap.
func (t *PerBeadCostTracker) Add(harnessName string, costUSD float64) {
	t.inner.Add(harnessName, costUSD)
}

// Spent returns the current accumulated billed total for this bead.
func (t *PerBeadCostTracker) Spent() float64 {
	return t.inner.Spent()
}

// Tripped reports whether the per-bead budget is exceeded. Returns a detail
// string containing PerBeadBudgetExhaustedReason when tripped so the execute-
// loop can detect and handle the outcome.
func (t *PerBeadCostTracker) Tripped() (string, bool) {
	if t.inner.MaxUSD <= 0 {
		return "", false
	}
	spent := t.inner.Spent()
	if spent < t.inner.MaxUSD {
		return "", false
	}
	return fmt.Sprintf("%s: $%.2f billed >= $%.2f per-bead budget; set --max-bead-cost=0 to disable or add a budget:<USD> label to override", PerBeadBudgetExhaustedReason, spent, t.inner.MaxUSD), true
}

// ParseBeadBudgetLabel scans a bead's label slice for a "budget:<float>" entry
// and returns the parsed value and true when found. An entry with an invalid
// numeric suffix is ignored (returns 0, false) so the caller falls back to the
// default per-bead budget rather than disabling the budget entirely.
func ParseBeadBudgetLabel(labels []string) (float64, bool) {
	const prefix = "budget:"
	for _, l := range labels {
		if !strings.HasPrefix(l, prefix) {
			continue
		}
		raw := strings.TrimPrefix(l, prefix)
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 {
			return 0, false
		}
		return v, true
	}
	return 0, false
}
