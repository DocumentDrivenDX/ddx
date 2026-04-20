package agent

import (
	"fmt"
	"sort"
	"time"
)

// costClassRank maps cost class to a numeric rank for cheap-preference sorting.
// Lower is cheaper.
var costClassRank = map[string]int{
	"local":     0,
	"cheap":     1,
	"medium":    2,
	"expensive": 3,
	"":          2, // unknown = medium
}

// BuildCandidatePlans evaluates all harnesses for a given RouteRequest and
// returns one CandidatePlan per harness. Plans that cannot satisfy the
// request are marked with RejectReason and Viable=false.
//
// stateOverride, when non-nil, supplies pre-probed state for each harness
// (used in tests and cached-state scenarios). If nil, state is derived from
// binary availability only (installed = binary found; reachable/auth/quota
// are optimistically assumed true for installed harnesses).
func (r *Runner) BuildCandidatePlans(req RouteRequest, stateOverride map[string]HarnessState) []CandidatePlan {
	var plans []CandidatePlan

	successRates := r.loadHistoricalSuccessRates()

	names := r.Registry.Names()
	for _, name := range names {
		harness, ok := r.Registry.Get(name)
		if !ok {
			continue
		}
		// TestOnly harnesses (script, virtual) are sentinels for test infrastructure
		// and must never be selected by production tier routing. They remain reachable
		// via explicit --harness <name> override so existing script tests continue to
		// work. See ddx-869848ec: script leaked into the standard-tier fallback chain
		// because it registered as IsLocal=true and scored a +25/+40 bonus in
		// scoreCandidate, with no filter excluding it from profile-based selection.
		if harness.TestOnly && req.HarnessOverride != name {
			continue
		}
		plan := r.evaluateCandidate(name, harness, req, stateOverride)
		if rate, ok := successRates[name]; ok {
			plan.HistoricalSuccessRate = rate
		} else {
			plan.HistoricalSuccessRate = -1
		}
		plans = append(plans, plan)
	}

	return plans
}

// loadHistoricalSuccessRates reads recorded routing outcomes and returns the
// observed success rate per harness. Harnesses with fewer than 3 samples are
// omitted (insufficient data). Returns an empty map when the metrics store is
// not configured or unreadable.
func (r *Runner) loadHistoricalSuccessRates() map[string]float64 {
	rates := make(map[string]float64)
	if r == nil || r.Config.SessionLogDir == "" {
		return rates
	}
	outcomes, err := NewRoutingMetricsStore(r.Config.SessionLogDir).ReadOutcomes()
	if err != nil || len(outcomes) == 0 {
		return rates
	}
	type counts struct {
		total   int
		success int
	}
	tally := make(map[string]*counts)
	for _, o := range outcomes {
		c, ok := tally[o.Harness]
		if !ok {
			c = &counts{}
			tally[o.Harness] = c
		}
		c.total++
		if o.Success {
			c.success++
		}
	}
	for harness, c := range tally {
		if c.total < 3 {
			continue
		}
		rates[harness] = float64(c.success) / float64(c.total)
	}
	return rates
}

// catalog returns the catalog to use for routing. Defaults to BuiltinCatalog.
func (r *Runner) catalog() *Catalog {
	if r.Catalog != nil {
		return r.Catalog
	}
	return BuiltinCatalog
}

// evaluateCandidate produces a CandidatePlan for one harness against a RouteRequest.
func (r *Runner) evaluateCandidate(name string, harness Harness, req RouteRequest, stateOverride map[string]HarnessState) CandidatePlan {
	plan := CandidatePlan{
		Harness:             name,
		Surface:             harness.Surface,
		CostClass:           harness.CostClass,
		IsSubscription:      harness.IsSubscription,
		SupportsEffort:      harness.EffortFlag != "",
		SupportsPermissions: len(harness.PermissionArgs) > 0,
	}

	// Populate state.
	if stateOverride != nil {
		if s, ok := stateOverride[name]; ok {
			plan.State = s
		} else {
			// Not in override map — treat as not installed.
			plan.State = HarnessState{
				Installed:   false,
				LastChecked: time.Now(),
			}
		}
	} else {
		// Derive state from binary availability.
		plan.State = r.fastHarnessState(name, harness)
	}

	// Apply rejection rules in order — first failing rule wins.
	if reason := rejectIfNotViable(plan, req); reason != "" {
		plan.RejectReason = reason
		plan.Viable = false
		return plan
	}

	cat := r.catalog()

	// Set requested ref and canonical target for diagnostics.
	if req.HarnessOverride != "" {
		plan.RequestedRef = "harness-override:" + req.HarnessOverride
		// HarnessOverride only constrains harness selection; model resolution must
		// still happen via the remaining request fields.
		if req.ModelPin != "" {
			if !harness.ExactPinSupport {
				plan.RejectReason = fmt.Sprintf("harness %s does not support exact model pins", name)
				plan.Viable = false
				return plan
			}
			plan.CanonicalTarget = req.ModelPin
			plan.ConcreteModel = req.ModelPin
			if dp, deprecated := cat.CheckDeprecatedPin(req.ModelPin, harness.Surface); deprecated {
				plan.DeprecationWarning = fmt.Sprintf("model %q is deprecated; use %q instead", req.ModelPin, dp.ReplacedBy)
			}
		} else if req.ModelRef != "" {
			concreteModel, ok := cat.Resolve(req.ModelRef, harness.Surface)
			if !ok {
				plan.RejectReason = fmt.Sprintf("model ref %q not available on surface %q", req.ModelRef, harness.Surface)
				plan.Viable = false
				return plan
			}
			plan.CanonicalTarget = req.ModelRef
			plan.ConcreteModel = concreteModel
			if entry, ok := cat.Entry(req.ModelRef); ok && entry.Deprecated {
				plan.DeprecationWarning = fmt.Sprintf("model ref %q is deprecated; use %q instead", req.ModelRef, entry.ReplacedBy)
			}
		} else if req.Profile != "" {
			concreteModel, ok := cat.Resolve(req.Profile, harness.Surface)
			if ok {
				plan.CanonicalTarget = req.Profile
				plan.ConcreteModel = concreteModel
			} else {
				plan.ConcreteModel = r.resolveModel(RunOptions{}, name)
				if plan.ConcreteModel == "" {
					plan.ConcreteModel = harness.DefaultModel
				}
			}
		} else {
			plan.ConcreteModel = r.resolveModel(RunOptions{}, name)
			if plan.ConcreteModel == "" {
				plan.ConcreteModel = harness.DefaultModel
			}
		}
	} else if req.ModelPin != "" {
		// Exact pin: only harnesses with ExactPinSupport can serve this.
		if !harness.ExactPinSupport {
			plan.RejectReason = fmt.Sprintf("harness %s does not support exact model pins", name)
			plan.Viable = false
			return plan
		}
		resolvedPin := req.ModelPin
		plan.RequestedRef = "pin:" + req.ModelPin
		plan.CanonicalTarget = resolvedPin
		plan.ConcreteModel = resolvedPin
		if dp, deprecated := cat.CheckDeprecatedPin(req.ModelPin, harness.Surface); deprecated {
			plan.DeprecationWarning = fmt.Sprintf("model %q is deprecated; use %q instead", req.ModelPin, dp.ReplacedBy)
		}
	} else if req.ModelRef != "" {
		// Logical ref: resolve through catalog for this harness's surface.
		concreteModel, ok := cat.Resolve(req.ModelRef, harness.Surface)
		if !ok {
			plan.RejectReason = fmt.Sprintf("model ref %q not available on surface %q", req.ModelRef, harness.Surface)
			plan.Viable = false
			return plan
		}
		plan.RequestedRef = "ref:" + req.ModelRef
		plan.CanonicalTarget = req.ModelRef
		plan.ConcreteModel = concreteModel
		// Surface deprecation warning if the ref is deprecated.
		if entry, ok := cat.Entry(req.ModelRef); ok && entry.Deprecated {
			plan.DeprecationWarning = fmt.Sprintf("model ref %q is deprecated; use %q instead", req.ModelRef, entry.ReplacedBy)
		}
	} else if req.Profile != "" {
		// Profile: resolve through catalog for this surface.
		concreteModel, ok := cat.Resolve(req.Profile, harness.Surface)
		if ok {
			plan.RequestedRef = "profile:" + req.Profile
			plan.CanonicalTarget = req.Profile
			plan.ConcreteModel = concreteModel
		} else {
			// Profile not in catalog for this surface — fall back to DefaultModelTiers.
			plan.RequestedRef = "profile:" + req.Profile
			plan.ConcreteModel = r.resolveModel(RunOptions{}, name)
			if plan.ConcreteModel == "" {
				plan.ConcreteModel = harness.DefaultModel
			}
		}
	} else {
		// No selector: use configured or harness default model.
		plan.ConcreteModel = r.resolveModel(RunOptions{}, name)
		if plan.ConcreteModel == "" {
			plan.ConcreteModel = harness.DefaultModel
		}
	}

	plan.Viable = true
	return plan
}

// NormalizeRouteRequest builds a RouteRequest from CLI flags and config using
// the precedence rules from SD-015 and the routing plan:
//
//  1. If flags.Harness is set → HarnessOverride (constrains routing to one harness).
//  2. If flags.Model is set → model resolution:
//     - When HarnessOverride is set: always treat as an exact ModelPin so the
//     explicit harness is never rejected by catalog-surface mismatch. The
//     harness binary receives the model string as-is and can error clearly.
//     - Otherwise: try catalog resolution:
//     - If known in catalog: ModelRef.
//     - If not known: ModelPin (exact pin, bypasses catalog).
//  3. If flags.Profile is set → Profile.
//  4. Else fall back to cfg.Model (same catalog resolution) or cfg.Profile.
//  5. If nothing is set → cfg.Harness becomes HarnessOverride (legacy fallback).
func NormalizeRouteRequest(flags RouteFlags, cfg Config, catalog *Catalog) RouteRequest {
	if catalog == nil {
		catalog = BuiltinCatalog
	}

	req := RouteRequest{
		Effort:      flags.Effort,
		Permissions: flags.Permissions,
	}

	// 1. Harness override constrains routing to one harness.
	//    --provider that names a known harness (or alias like "local") also
	//    pins the harness so the routing engine never falls through to a
	//    different harness.
	if flags.Harness != "" {
		req.HarnessOverride = flags.Harness
	} else if flags.Provider != "" {
		resolved := resolveHarnessAlias(flags.Provider)
		// Only treat as harness override if it names a registered harness;
		// otherwise it's a ddx-agent provider name (e.g. "vidar") and
		// should not constrain harness selection.
		if _, ok := builtinHarnesses[resolved]; ok {
			req.HarnessOverride = resolved
		}
	} else if cfg.Harness != "" {
		req.HarnessOverride = cfg.Harness
	}

	// 2. Model flag: resolution depends on whether a harness is pinned.
	if flags.Model != "" {
		if req.HarnessOverride != "" {
			// Harness is explicit: always treat the model as an exact pin so
			// --harness <h> always wins. Passing it as ModelPin means the
			// harness is never rejected because of a catalog-surface mismatch;
			// the binary receives the string as-is and will error clearly if it
			// doesn't recognize the model.
			req.ModelPin = flags.Model
		} else {
			modelRef, modelPin := catalog.NormalizeModelRef(flags.Model)
			req.ModelRef = modelRef
			req.ModelPin = modelPin
		}
		return req
	}

	// 3. Profile flag.
	if flags.Profile != "" {
		req.Profile = flags.Profile
		return req
	}

	// 4. Config defaults: model first, then profile.
	if cfg.Model != "" {
		modelRef, modelPin := catalog.NormalizeModelRef(cfg.Model)
		req.ModelRef = modelRef
		req.ModelPin = modelPin
		return req
	}
	if cfg.Profile != "" {
		req.Profile = cfg.Profile
		return req
	}

	// 5. Nothing configured — request is underspecified; routing will use
	// harness default or first-available fallback.
	return req
}

// fastHarnessState derives a HarnessState from binary availability (no I/O).
func (r *Runner) fastHarnessState(name string, harness Harness) HarnessState {
	state := HarnessState{
		PolicyOK:    true,
		LastChecked: time.Now(),
	}
	if harness.IsLocal || name == "virtual" || name == "agent" {
		state.Installed = true
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
		return state
	}
	// HTTP-only providers (lmstudio, openrouter) have no binary to look up.
	// Optimistically mark as installed; full reachability is checked by
	// ProbeHarnessState when live probing is requested.
	if harness.IsHTTPProvider {
		state.Installed = true
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
		return state
	}
	if _, err := r.LookPath(harness.Binary); err == nil {
		state.Installed = true
		// Optimistically assume reachable/auth/quota for installed harnesses
		// when not performing a live probe.
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
	} else {
		state.Installed = false
		state.Error = fmt.Sprintf("%s not found in PATH", harness.Binary)
	}
	return state
}

// rejectIfNotViable returns a non-empty rejection reason if the candidate
// cannot satisfy the request, or "" if the candidate is viable.
func rejectIfNotViable(plan CandidatePlan, req RouteRequest) string {
	s := plan.State

	if !s.Installed {
		return "not installed"
	}
	if !s.Reachable {
		return "not reachable"
	}
	if !s.Authenticated {
		return "not authenticated"
	}
	if s.QuotaState == "blocked" || (s.QuotaState == "" && !s.QuotaOK) {
		return "quota exceeded"
	}
	if s.Degraded {
		return "degraded"
	}
	if !s.PolicyOK {
		return "policy restricted"
	}

	// If harness override is set, reject all other harnesses.
	if req.HarnessOverride != "" && plan.Harness != req.HarnessOverride {
		return fmt.Sprintf("harness override requires %s", req.HarnessOverride)
	}

	// Effort requested but harness doesn't support it.
	if req.Effort != "" && !plan.SupportsEffort {
		return fmt.Sprintf("effort %q not supported by harness %s", req.Effort, plan.Harness)
	}

	return ""
}

// RankCandidates scores and sorts viable candidates according to the profile
// intent. Non-viable candidates are placed at the end in their original order.
// Returns a new slice; the input is not mutated.
func RankCandidates(profile string, plans []CandidatePlan) []CandidatePlan {
	ranked := make([]CandidatePlan, len(plans))
	copy(ranked, plans)

	// Assign scores to viable candidates.
	for i := range ranked {
		if !ranked[i].Viable {
			ranked[i].Score = -1
			continue
		}
		ranked[i].Score = scoreCandidate(profile, ranked[i])
	}

	// Stable sort: viable (score >= 0) before non-viable, then by descending score.
	sort.SliceStable(ranked, func(i, j int) bool {
		vi, vj := ranked[i].Viable, ranked[j].Viable
		if vi != vj {
			return vi // viable first
		}
		if !vi {
			return false // both non-viable: preserve order
		}
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		// Stable tie-breaker: prefer local, then alphabetical harness name.
		li, lj := ranked[i].State.Installed && costClassRank[ranked[i].CostClass] == 0,
			ranked[j].State.Installed && costClassRank[ranked[j].CostClass] == 0
		if li != lj {
			return li
		}
		return ranked[i].Harness < ranked[j].Harness
	})

	return ranked
}

// scoreCandidate returns a score for a viable candidate given the profile.
// Higher is better.
//
// Routing priority policy:
//   - cheap/standard: local (free, no quota) > subscription-within-quota > pay-per-token
//   - smart: cloud capability wins; local models are typically quantized and lower-quality
func scoreCandidate(profile string, plan CandidatePlan) float64 {
	base := 100.0

	cr := costClassRank[plan.CostClass] // 0=local,1=cheap,2=medium,3=expensive

	// withinQuota is true when a subscription harness has headroom (cost already sunk).
	withinQuota := plan.IsSubscription && (plan.State.QuotaOK || plan.State.QuotaState == "ok")

	switch profile {
	case "cheap":
		// Minimize cost: local >> subscription-within-quota >> pay-per-token.
		if plan.CostClass == "local" {
			base += 40
		} else if withinQuota {
			base += 20
		}
		base -= float64(cr) * 30

	case "standard":
		// Balanced: prefer local and subscription to avoid unnecessary spend.
		if plan.CostClass == "local" {
			base += 25
		} else if withinQuota {
			base += 15
		}
		base -= float64(cr) * 10

	case "smart":
		// Quality first; cost is secondary.
		// Higher cost rank approximates higher capability.
		// Local models are typically quantized — no local bonus here.
		base += float64(cr) * 20
		// Small bonus for subscription-within-quota at equal quality.
		if withinQuota {
			base += 5
		}

	default:
		// Treat unspecified as standard.
		if plan.CostClass == "local" {
			base += 25
		} else if withinQuota {
			base += 15
		}
		base -= float64(cr) * 10
	}

	// Penalize near-limit quota (>= 80% used).
	if plan.State.Quota != nil && plan.State.Quota.PercentUsed >= 80 {
		base -= float64(plan.State.Quota.PercentUsed-80) * 2
	}
	if plan.State.QuotaState == "unknown" {
		base -= 3
	}

	// Historical success-rate adjustment. Only applied when we have sufficient
	// data (>= 3 samples, encoded as HistoricalSuccessRate >= 0).
	if plan.HistoricalSuccessRate >= 0 {
		switch {
		case plan.HistoricalSuccessRate >= 0.8:
			base += 20
		case plan.HistoricalSuccessRate < 0.5:
			base -= 30
		}
	}

	return base
}

// SelectBestCandidate picks the first viable candidate from a ranked list.
// Returns an error if no viable candidate exists.
func SelectBestCandidate(plans []CandidatePlan) (*CandidatePlan, error) {
	for i := range plans {
		if plans[i].Viable {
			return &plans[i], nil
		}
	}
	return nil, fmt.Errorf("no viable harness candidate: all harnesses rejected")
}

// ProbeAndBuildCandidatePlans probes live harness state and runs provider
// discovery before building plans. This is the full routing path for actual dispatch.
func (r *Runner) ProbeAndBuildCandidatePlans(req RouteRequest, timeout time.Duration) []CandidatePlan {
	states := make(map[string]HarnessState)
	for _, name := range r.Registry.Names() {
		states[name] = r.ProbeHarnessState(name, timeout)
	}

	return r.BuildCandidatePlans(req, states)
}
