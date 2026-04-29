package agent

import (
	"strings"
	"sync/atomic"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

const DefaultRoutingProfile = "default"

var profileTierRank = map[string]int{
	"cheap":    0,
	"fast":     1,
	"standard": 2,
	"smart":    3,
}

// Test seams: tests assert that ResolveProfileLadder and ResolveTierModelRef
// are NOT in the call graph on the default routing path (ddx-755f5881 AC #3).
// Production code never reads these counters; they are exported only for tests.
var (
	resolveProfileLadderCalls atomic.Int64
	resolveTierModelRefCalls  atomic.Int64
)

// ResolveProfileLadderCallCount returns the current value of the call counter.
// Tests use it together with ResetRoutingCallCounters to verify call-graph claims.
func ResolveProfileLadderCallCount() int64 { return resolveProfileLadderCalls.Load() }

// ResolveTierModelRefCallCount returns the current value of the call counter.
func ResolveTierModelRefCallCount() int64 { return resolveTierModelRefCalls.Load() }

// ResetRoutingCallCounters zeros the test-seam counters.
func ResetRoutingCallCounters() {
	resolveProfileLadderCalls.Store(0)
	resolveTierModelRefCalls.Store(0)
}

// ResolveProfileLadder returns the ordered tiers to try for profile after
// applying explicit --min-tier / --max-tier caps.
func ResolveProfileLadder(routing *config.RoutingConfig, profile, minTier, maxTier string) []escalation.ModelTier {
	resolveProfileLadderCalls.Add(1)
	profile = NormalizeRoutingProfile(profile)
	ladder := routing.ResolvedLadder(profile)
	out := make([]escalation.ModelTier, 0, len(ladder))
	for _, raw := range ladder {
		tier := strings.TrimSpace(raw)
		if tier == "" || !tierWithinBounds(tier, minTier, maxTier) {
			continue
		}
		out = append(out, escalation.ModelTier(tier))
	}
	return out
}

func NormalizeRoutingProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return DefaultRoutingProfile
	}
	return profile
}

// ResolveTierModelRef applies agent.routing.model_overrides for a ladder tier.
func ResolveTierModelRef(routing *config.RoutingConfig, tier escalation.ModelTier) string {
	resolveTierModelRefCalls.Add(1)
	tierRef := string(tier)
	if routing != nil && routing.ModelOverrides != nil {
		if override := strings.TrimSpace(routing.ModelOverrides[tierRef]); override != "" {
			return override
		}
	}
	return tierRef
}

// TierCandidate is one (harness, model) pair to try for a ladder tier.
// It is produced by ResolveTierCandidates and consumed by the escalation
// loop in execute-loop. Source carries the provenance (override|catalog) so
// diagnostics can distinguish caller-provided overrides from defaults.
type TierCandidate struct {
	Surface string // catalog surface (empty for override candidates)
	Harness string // harness name (empty for override candidates — upstream picks)
	Model   string // concrete model string to pass to ResolveRoute
	Source  string // "override" or "catalog"
}

// catalogSurfaceOrder is the deterministic order surfaces are tried within a
// tier. Walking surfaces in a stable order makes tier-attempt diagnostics
// reproducible across runs.
var catalogSurfaceOrder = []string{"codex", "claude", "embedded-openai"}

// surfaceToHarnessName returns the harness name that owns a catalog surface.
// Inverse of harnessToSurface.
func surfaceToHarnessName(surface string) string {
	for h, s := range harnessToSurface {
		if s == surface {
			return h
		}
	}
	return ""
}

// ResolveTierCandidates returns ordered (harness, model) candidates for a
// ladder tier. agent.routing.model_overrides wins when set (an override has
// no associated harness — upstream picks); catalog surfaces in
// catalogSurfaceOrder follow as defaults so projects without
// model_overrides still resolve a concrete model per tier via the catalog.
//
// Healthy-harness and provider-reachability checks are NOT performed here:
// callers should iterate the returned candidates and probe each via
// svc.ResolveRoute / svc.RouteStatus, recording per-candidate rejections in
// tier-attempt diagnostics. ResolveTierCandidates is purely the "what could
// be tried" enumeration.
func ResolveTierCandidates(routing *config.RoutingConfig, cat *Catalog, tier escalation.ModelTier) []TierCandidate {
	var out []TierCandidate
	tierStr := string(tier)

	if routing != nil && routing.ModelOverrides != nil {
		if override := strings.TrimSpace(routing.ModelOverrides[tierStr]); override != "" {
			out = append(out, TierCandidate{Model: override, Source: "override"})
		}
	}

	if cat == nil {
		cat = BuiltinCatalog
	}
	if entry, ok := cat.Entry(tierStr); ok {
		for _, surface := range catalogSurfaceOrder {
			model, ok := entry.Surfaces[surface]
			if !ok || strings.TrimSpace(model) == "" {
				continue
			}
			if cat.IsBlockedModelID(model) {
				continue
			}
			harness := surfaceToHarnessName(surface)
			if harness == "" {
				continue
			}
			out = append(out, TierCandidate{
				Surface: surface,
				Harness: harness,
				Model:   model,
				Source:  "catalog",
			})
		}
	}
	return out
}

func tierWithinBounds(tier, minTier, maxTier string) bool {
	rank, ok := profileTierRank[tier]
	if !ok {
		return minTier == "" && maxTier == ""
	}
	if minTier != "" {
		if minRank, ok := profileTierRank[minTier]; ok && rank < minRank {
			return false
		}
	}
	if maxTier != "" {
		if maxRank, ok := profileTierRank[maxTier]; ok && rank > maxRank {
			return false
		}
	}
	return true
}
