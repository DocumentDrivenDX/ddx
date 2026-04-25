package evidence

import "strings"

// EvidenceAssemblySection is the per-section telemetry record produced by
// the evidence-assembly call sites and consumed by Stages C1, C2, and F
// (FEAT-022 §15). The shape is a contract — downstream stages decode this
// without re-deriving field names.
type EvidenceAssemblySection struct {
	Name             string   `json:"name"`
	BytesIncluded    int      `json:"bytes_included"`
	BytesOmitted     int      `json:"bytes_omitted"`
	TruncationReason string   `json:"truncation_reason"`
	SelectedItems    []string `json:"selected_items"`
	OmittedItems     []string `json:"omitted_items"`
}

// SectionInput is one candidate section handed to FitSections, in
// caller-determined priority order (highest priority first).
type SectionInput struct {
	Name    string
	Content string
	// MinFloor, when true, marks this section as part of the minimum
	// evidence floor (FEAT-022 §5 items 1-2). Floor sections are always
	// included; if even the floor exceeds budget FitSections still
	// emits them but reports the overall overflow via the returned
	// TotalBytes vs budget comparison left to the caller.
	MinFloor bool
	// PerSectionCap, if non-zero, additionally clamps this section to
	// at most this many bytes (using ClampOutput) before budgeting.
	PerSectionCap int
}

// FitResult is the outcome of FitSections.
type FitResult struct {
	Included   []SectionInput
	Omitted    []SectionInput
	TotalBytes int
	Sections   []EvidenceAssemblySection
}

// FitSections selects sections to include under a total byte budget,
// in priority order. Floor sections are always included. Non-floor
// sections are added in order while their content fits the remaining
// budget; sections that don't fit are recorded as omitted.
//
// Default behavior is line-based and type-agnostic (FEAT-022 §1):
// when a section's PerSectionCap is non-zero, content is clamped to
// that cap before budgeting. Markdown-aware extraction is handled by
// callers that need it; this primitive does not parse content.
func FitSections(sections []SectionInput, budget int) FitResult {
	if budget < 0 {
		budget = 0
	}
	res := FitResult{
		Sections: make([]EvidenceAssemblySection, 0, len(sections)),
	}
	used := 0

	// First pass: include all floor sections (apply per-section cap).
	for _, s := range sections {
		if !s.MinFloor {
			continue
		}
		content, reason := capContent(s)
		section := EvidenceAssemblySection{
			Name:          s.Name,
			BytesIncluded: len(content),
			BytesOmitted:  len(s.Content) - len(content),
			SelectedItems: []string{s.Name},
		}
		if reason != "" {
			section.TruncationReason = reason
		}
		used += len(content)
		res.Included = append(res.Included, SectionInput{
			Name: s.Name, Content: content, MinFloor: true,
		})
		res.Sections = append(res.Sections, section)
	}

	// Second pass: non-floor in priority order, fit under remaining budget.
	for _, s := range sections {
		if s.MinFloor {
			continue
		}
		content, reason := capContent(s)
		remaining := budget - used
		if len(content) <= remaining {
			used += len(content)
			res.Included = append(res.Included, SectionInput{
				Name: s.Name, Content: content,
			})
			section := EvidenceAssemblySection{
				Name:          s.Name,
				BytesIncluded: len(content),
				BytesOmitted:  len(s.Content) - len(content),
				SelectedItems: []string{s.Name},
			}
			if reason != "" {
				section.TruncationReason = reason
			}
			res.Sections = append(res.Sections, section)
			continue
		}
		// Doesn't fit even at PerSectionCap — try line-trim to fit.
		if remaining > 0 {
			trimmed := trimToLineBudget(content, remaining-len(TruncationMarker))
			if trimmed != "" {
				body := trimmed + TruncationMarker
				used += len(body)
				res.Included = append(res.Included, SectionInput{
					Name: s.Name, Content: body,
				})
				res.Sections = append(res.Sections, EvidenceAssemblySection{
					Name:             s.Name,
					BytesIncluded:    len(body),
					BytesOmitted:     len(s.Content) - len(body),
					TruncationReason: "budget",
					SelectedItems:    []string{s.Name},
				})
				continue
			}
		}
		// No room: omit entirely.
		res.Omitted = append(res.Omitted, s)
		res.Sections = append(res.Sections, EvidenceAssemblySection{
			Name:             s.Name,
			BytesIncluded:    0,
			BytesOmitted:     len(s.Content),
			TruncationReason: "budget",
			OmittedItems:     []string{s.Name},
		})
	}
	res.TotalBytes = used
	return res
}

func capContent(s SectionInput) (string, string) {
	if s.PerSectionCap <= 0 || len(s.Content) <= s.PerSectionCap {
		return s.Content, ""
	}
	clamped, truncated, _ := ClampOutput(s.Content, s.PerSectionCap)
	if truncated {
		return clamped, "per_section_cap"
	}
	return clamped, ""
}

// trimToLineBudget returns the longest prefix of s, ending on a line
// boundary, whose length is at most budget. Returns "" if budget <= 0
// or the first line alone exceeds budget.
func trimToLineBudget(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if len(s) <= budget {
		return s
	}
	cut := strings.LastIndexByte(s[:budget], '\n')
	if cut <= 0 {
		return ""
	}
	return s[:cut+1]
}
