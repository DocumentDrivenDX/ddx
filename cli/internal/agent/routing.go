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

	names := r.Registry.Names()
	for _, name := range names {
		harness, ok := r.Registry.Get(name)
		if !ok {
			continue
		}
		plan := r.evaluateCandidate(name, harness, req, stateOverride)
		plans = append(plans, plan)
	}

	return plans
}

// evaluateCandidate produces a CandidatePlan for one harness against a RouteRequest.
func (r *Runner) evaluateCandidate(name string, harness Harness, req RouteRequest, stateOverride map[string]HarnessState) CandidatePlan {
	plan := CandidatePlan{
		Harness:             name,
		Surface:             harness.Surface,
		CostClass:           harness.CostClass,
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

	// Resolve the concrete model for this candidate.
	plan.ConcreteModel = r.resolveModel(RunOptions{Model: req.ModelPin}, name)
	if plan.ConcreteModel == "" {
		plan.ConcreteModel = harness.DefaultModel
	}

	// Set requested ref and canonical target for diagnostics.
	if req.HarnessOverride != "" {
		plan.RequestedRef = "harness-override:" + req.HarnessOverride
	} else if req.ModelPin != "" {
		plan.RequestedRef = "pin:" + req.ModelPin
		plan.CanonicalTarget = req.ModelPin
	} else if req.ModelRef != "" {
		plan.RequestedRef = "ref:" + req.ModelRef
	} else if req.Profile != "" {
		plan.RequestedRef = "profile:" + req.Profile
	}

	plan.Viable = true
	return plan
}

// fastHarnessState derives a HarnessState from binary availability (no I/O).
func (r *Runner) fastHarnessState(name string, harness Harness) HarnessState {
	state := HarnessState{
		PolicyOK:    true,
		LastChecked: time.Now(),
	}
	if harness.IsLocal || name == "virtual" || name == "forge" {
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
	if !s.QuotaOK {
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
func scoreCandidate(profile string, plan CandidatePlan) float64 {
	base := 100.0

	cr := costClassRank[plan.CostClass] // 0=local,1=cheap,2=medium,3=expensive

	switch profile {
	case "cheap":
		// Prefer lowest cost. Local = best, then cheap, medium, expensive.
		base -= float64(cr) * 30
		// Extra bonus for local (no API cost at all).
		if plan.CostClass == "local" {
			base += 5
		}
	case "fast":
		// Prefer fast response: local first (no network), then cheap cloud.
		base -= float64(cr) * 15
		if plan.CostClass == "local" {
			base += 5
		}
	case "smart":
		// Prefer quality: expensive > medium > cheap > local.
		// Higher cost rank = higher quality.
		base += float64(cr) * 20
	default:
		// No profile: neutral ranking; local preferred over cloud when equal.
		base -= float64(cr) * 10
		if plan.CostClass == "local" {
			base += 5
		}
	}

	// Penalize near-limit quota (>= 80% used).
	if plan.State.Quota != nil && plan.State.Quota.PercentUsed >= 80 {
		base -= float64(plan.State.Quota.PercentUsed-80) * 2
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

// ProbeAndBuildCandidatePlans probes live harness state before building plans.
// This is the full routing path for actual dispatch.
func (r *Runner) ProbeAndBuildCandidatePlans(req RouteRequest, timeout time.Duration) []CandidatePlan {
	states := make(map[string]HarnessState)
	for _, name := range r.Registry.Names() {
		states[name] = r.ProbeHarnessState(name, timeout)
	}
	return r.BuildCandidatePlans(req, states)
}
