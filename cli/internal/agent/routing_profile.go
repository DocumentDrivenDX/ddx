package agent

import "strings"

const DefaultRoutingProfile = "default"

// NormalizeRoutingProfile returns "default" for empty/whitespace-only strings,
// and the trimmed input otherwise.
func NormalizeRoutingProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return DefaultRoutingProfile
	}
	return profile
}
