package config

import "github.com/DocumentDrivenDX/ddx/internal/evidence"

// ResolveEvidenceCaps returns the project-wide evidence caps without applying
// a semantic-role override.
func (c *NewConfig) ResolveEvidenceCaps() evidence.Caps {
	return c.ResolveEvidenceCapsForRole("")
}

// ResolveEvidenceCapsForRole returns the effective caps for a DDx semantic
// prompt role, applying defaults < project-wide < per-role precedence. Route
// identity is deliberately absent from this API: Fizeau owns harness, model,
// provider, and route selection.
func (c *NewConfig) ResolveEvidenceCapsForRole(role string) evidence.Caps {
	if c == nil || c.EvidenceCaps == nil {
		return evidence.DefaultCaps()
	}
	project := evidence.CapsOverride{
		MaxPromptBytes:       c.EvidenceCaps.MaxPromptBytes,
		MaxInlinedFileBytes:  c.EvidenceCaps.MaxInlinedFileBytes,
		MaxDiffBytes:         c.EvidenceCaps.MaxDiffBytes,
		MaxGoverningDocBytes: c.EvidenceCaps.MaxGoverningDocBytes,
	}
	roleOverride := evidence.CapsOverride{}
	if isEvidenceRole(role) {
		r := c.EvidenceCaps.PerRole[role]
		if r != nil {
			roleOverride = evidence.CapsOverride{
				MaxPromptBytes:       r.MaxPromptBytes,
				MaxInlinedFileBytes:  r.MaxInlinedFileBytes,
				MaxDiffBytes:         r.MaxDiffBytes,
				MaxGoverningDocBytes: r.MaxGoverningDocBytes,
			}
		}
	}
	return evidence.ResolveCaps(project, roleOverride)
}

func isEvidenceRole(role string) bool {
	switch role {
	case EvidenceRoleImplementer, EvidenceRoleReviewer, EvidenceRoleLifecycle:
		return true
	default:
		return false
	}
}
