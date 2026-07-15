package agent

// NormalizeRoutingProfile preserves the operator-supplied Fizeau profile
// constraint byte-for-byte. Fizeau owns profile syntax and normalization.
func NormalizeRoutingProfile(profile string) string {
	return profile
}
