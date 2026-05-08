package agent

import "strings"

// NormalizeRoutingProfile trims operator input while preserving an empty value.
// Empty means unconstrained: the agent service chooses without profile-derived
// power bounds.
func NormalizeRoutingProfile(profile string) string {
	return strings.TrimSpace(profile)
}
