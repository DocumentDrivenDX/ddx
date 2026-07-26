// Deterministic static-check registry and resolver for Verification
// mapping rows (phase2-doc-truth-plan WB-1 steps 3-4).
//
// A mapped static check must name a check registered in the
// spechonesty verification model. Resolution is read-only and does not
// infer checks from prose, filenames, network state, or command text.
package spechonesty

import (
	"fmt"
	"strings"
)

// StaticCheckDefinition is one registered deterministic static check in
// the spechonesty verification model.
type StaticCheckDefinition struct {
	// Name is the canonical check name used after the `check:` prefix.
	Name string
	// Analyzer names the underlying analyzer or tool for documentation.
	Analyzer string
	// Command is the local command that drives the check.
	Command string
}

// StaticCheckModel is the deterministic registry of static checks that
// Verification rows may cite.
type StaticCheckModel struct {
	Checks []StaticCheckDefinition
}

var defaultStaticCheckDefinitions = []StaticCheckDefinition{
	{
		Name:     "static-delete",
		Analyzer: "deletecheck",
		Command:  "go run ./tools/lint/deletecheck",
	},
	{
		Name:     "static-list",
		Analyzer: "listcheck",
		Command:  "go run ./tools/lint/listcheck",
	},
	{
		Name:     "lockreentry",
		Analyzer: "lockreentrylint",
		Command:  "go run ./tools/lint/lockreentrylint/cmd/lockreentrylint",
	},
	{
		Name:     "spechonesty",
		Analyzer: "spechonesty",
		Command:  "go run ./tools/lint/spechonesty/cmd/spechonesty",
	},
}

// DefaultStaticCheckModel returns the built-in deterministic registry.
func DefaultStaticCheckModel() StaticCheckModel {
	checks := make([]StaticCheckDefinition, len(defaultStaticCheckDefinitions))
	copy(checks, defaultStaticCheckDefinitions)
	return StaticCheckModel{Checks: checks}
}

// Has reports whether the named static check is registered in the model.
func (m StaticCheckModel) Has(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, check := range m.Checks {
		if check.Name == name {
			return true
		}
	}
	return false
}

// Names returns the registered check names in deterministic order.
func (m StaticCheckModel) Names() []string {
	if len(m.Checks) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.Checks))
	for _, check := range m.Checks {
		out = append(out, check.Name)
	}
	return out
}

// StaticCheckInput is the document-level input for the static-check
// resolver.
type StaticCheckInput struct {
	// Path is the document path recorded on diagnostics.
	Path string
	// Status is the normalized base document status.
	Status DocStatus
	// StatusLine is the 1-based line of the status stamp (0 → 1).
	StatusLine int
	// Rows is the well-formed Verification mapping row set.
	Rows []VerificationRow
	// Model is the static-check registry to resolve against. Nil selects
	// the built-in deterministic model.
	Model *StaticCheckModel
}

// CheckStaticCheckResolution resolves mapped static-check targets for
// Complete/Implemented documents against a deterministic registry.
//
// Rules:
//   - Non-Complete/Implemented statuses → no findings.
//   - Empty row set → no findings.
//   - Rows that do not name an exact static check target → ignored here.
//   - Static-check targets whose names are not registered in the model →
//     one FindingMissingStaticCheck diagnostic naming the missing check.
//   - Registered static-check targets → no diagnostic.
//
// Resolution is independent per row and read-only. It does not infer
// checks from prose, filenames, network state, or command text.
func CheckStaticCheckResolution(in StaticCheckInput) []CoverageFinding {
	if !IsCompleteStatus(in.Status) {
		return nil
	}
	if len(in.Rows) == 0 {
		return nil
	}

	model := DefaultStaticCheckModel()
	if in.Model != nil {
		model = *in.Model
	}

	var findings []CoverageFinding
	for _, row := range in.Rows {
		name, ok := parseStaticCheckTarget(row.EvidenceTarget)
		if !ok {
			continue
		}
		if model.Has(name) {
			continue
		}
		line := row.Line
		if line <= 0 {
			line = 1
		}
		findings = append(findings, CoverageFinding{
			Path:     in.Path,
			Line:     line,
			Kind:     FindingMissingStaticCheck,
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s: missing static check %q (requirement %q); no registered check named %q in the spechonesty verification model",
				in.Path, name, row.RequirementRef, name,
			),
		})
	}
	return findings
}

// CheckDocumentStaticChecks parses status and Verification rows for
// content and runs the static-check resolver. Convenience for fixture
// tests; read-only over the supplied content.
func CheckDocumentStaticChecks(path, content string) []CoverageFinding {
	status := ParseDocumentStatusMarkdown(path, content)
	model := ParseVerificationMarkdown(path, content)
	return CheckStaticCheckResolution(StaticCheckInput{
		Path:   path,
		Status: status.Status,
		Rows:   model.Rows,
	})
}

// parseStaticCheckTarget returns the canonical static-check name for an
// exact check:<name> target. The check prefix is matched
// case-insensitively; the registered name itself is matched exactly.
func parseStaticCheckTarget(target string) (name string, ok bool) {
	target = stripEvidenceMarkup(target)
	if target == "" {
		return "", false
	}
	if !strings.HasPrefix(strings.ToLower(target), "check:") {
		return "", false
	}
	name = strings.TrimSpace(target[len("check:"):])
	if name == "" {
		return "", false
	}
	return name, true
}
