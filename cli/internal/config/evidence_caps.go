package config

import "github.com/DocumentDrivenDX/ddx/internal/evidence"

// ResolveEvidenceCaps returns the effective evidence caps for the given
// harness, applying defaults < project-wide < per-harness precedence
// (FEAT-022 §1a).
func (c *NewConfig) ResolveEvidenceCaps(harness string) evidence.Caps {
	if c == nil || c.EvidenceCaps == nil {
		return evidence.DefaultCaps()
	}
	project := evidence.CapsOverride{
		MaxPromptBytes:       c.EvidenceCaps.MaxPromptBytes,
		MaxInlinedFileBytes:  c.EvidenceCaps.MaxInlinedFileBytes,
		MaxDiffBytes:         c.EvidenceCaps.MaxDiffBytes,
		MaxGoverningDocBytes: c.EvidenceCaps.MaxGoverningDocBytes,
	}
	perHarness := make(map[string]evidence.CapsOverride, len(c.EvidenceCaps.PerHarness))
	for name, h := range c.EvidenceCaps.PerHarness {
		if h == nil {
			continue
		}
		perHarness[name] = evidence.CapsOverride{
			MaxPromptBytes:       h.MaxPromptBytes,
			MaxInlinedFileBytes:  h.MaxInlinedFileBytes,
			MaxDiffBytes:         h.MaxDiffBytes,
			MaxGoverningDocBytes: h.MaxGoverningDocBytes,
		}
	}
	return evidence.ResolveCaps(project, perHarness, harness)
}
