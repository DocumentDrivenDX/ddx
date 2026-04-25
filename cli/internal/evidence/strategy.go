package evidence

import (
	"fmt"
	"strings"
)

// RefOnlyEntry is one ref the agent should resolve via its own tools
// (the ref-only metadata strategy in FEAT-022 §2). Path and Title are
// emitted; content is intentionally not inlined.
type RefOnlyEntry struct {
	Path  string
	Title string
}

// AssembleRefOnly emits a metadata-only listing of refs. Suitable when
// the agent has filesystem access and the prompt does not need to
// guarantee the agent saw specific content (FEAT-022 §2).
func AssembleRefOnly(entries []RefOnlyEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if e.Title != "" {
			fmt.Fprintf(&b, "- %s — %s\n", e.Path, e.Title)
		} else {
			fmt.Fprintf(&b, "- %s\n", e.Path)
		}
	}
	return b.String()
}

// InlineAssembly is the result of AssembleInline.
type InlineAssembly struct {
	Prompt   string
	Sections []EvidenceAssemblySection
	// Overflow is true when, after all trimming, the assembled prompt
	// would still exceed total. Callers (reviewer/grader) MUST NOT
	// dispatch when Overflow is true (FEAT-022 §7) and should emit the
	// appropriate context_overflow outcome class instead.
	Overflow bool
}

// AssembleInline assembles a prompt by joining priority-ordered
// sections under a total byte budget (FEAT-022 §2 inline-with-cap
// strategy). Per-section caps and floor semantics are honored via the
// SectionInput fields. Sections are joined verbatim with a single
// newline separator between non-empty bodies.
func AssembleInline(sections []SectionInput, total int) InlineAssembly {
	fit := FitSections(sections, total)

	var b strings.Builder
	for i, s := range fit.Included {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s.Content)
	}
	out := b.String()

	overflow := len(out) > total
	return InlineAssembly{
		Prompt:   out,
		Sections: fit.Sections,
		Overflow: overflow,
	}
}
