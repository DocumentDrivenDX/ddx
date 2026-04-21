package agent

import (
	"strings"

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

// ResolveProfileLadder returns the ordered tiers to try for profile after
// applying explicit --min-tier / --max-tier caps.
func ResolveProfileLadder(routing *config.RoutingConfig, profile, minTier, maxTier string) []escalation.ModelTier {
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
	tierRef := string(tier)
	if routing != nil && routing.ModelOverrides != nil {
		if override := strings.TrimSpace(routing.ModelOverrides[tierRef]); override != "" {
			return override
		}
	}
	return tierRef
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
