// Coverage-cardinality pass for Complete/Implemented documents
// (phase2-doc-truth-plan WB-1 steps 3-4: every inventory requirement or
// stable anchor covered exactly once by Verification mapping rows).
//
// Consumes status (ParseDocumentStatus*) and the Verification inventory
// + rows model (ParseVerification*). Emits one diagnostic per inventory
// requirement covered zero times and one per requirement covered more
// than once. Exactly-once coverage yields no diagnostic.
//
// At the requirement-to-evidence join, only rows that pass
// IsCoveringCitation count as coverage: file-only test citations and
// bare non-target evidence do not. Zero-evidence (empty row set),
// waivers, evidence-target resolution (disk existence), and command
// allowlists are sibling children. Read-only: pure over the supplied input.
package spechonesty

import (
	"fmt"
	"strings"
)

// CoverageCardinalityInput is the document-level input for the
// coverage-cardinality pass. Callers supply status from the status
// parser and inventory/rows from the Verification mapping parser; this
// pass does not re-parse markdown.
type CoverageCardinalityInput struct {
	// Path is the document path recorded on diagnostics.
	Path string
	// Status is the normalized base document status.
	Status DocStatus
	// StatusLine is the 1-based line of the status stamp (0 → 1).
	StatusLine int
	// Inventory is the ordered requirement IDs or section anchors.
	Inventory []string
	// Rows is the well-formed Verification mapping row set.
	Rows []VerificationRow
}

// CheckCoverageCardinality joins inventory requirements against mapping
// rows by requirement identifier for Complete/Implemented documents.
//
// Rules:
//   - Non-Complete/Implemented statuses → no findings.
//   - Only rows where IsCoveringCitation is true count toward coverage
//     (file-only test paths and bare non-target evidence are excluded).
//   - Inventory requirement with zero counting mapping rows → one
//     FindingUnmetVerification naming that requirement.
//   - Inventory requirement with two or more counting mapping rows → one
//     FindingDuplicateMapping naming that requirement.
//   - Inventory requirement with exactly one counting mapping row → no diagnostic.
//
// Pure and read-only: no filesystem or network access. Extra mapping
// rows whose RequirementRef is not in the inventory are ignored by this
// pass (they are not inventory coverage failures). Named target existence
// on disk is not resolved here.
func CheckCoverageCardinality(in CoverageCardinalityInput) []CoverageFinding {
	if !IsCompleteStatus(in.Status) {
		return nil
	}
	if len(in.Inventory) == 0 {
		return nil
	}

	counts := make(map[string]int, len(in.Rows))
	firstLine := make(map[string]int, len(in.Rows))
	for _, row := range in.Rows {
		ref := strings.TrimSpace(row.RequirementRef)
		if ref == "" {
			continue
		}
		// Citation-granularity exclusion at the coverage join: a mapping
		// row that does not name an exact Test*, static check, or
		// artifact target does not count as requirement coverage.
		if !IsCoveringCitation(row) {
			continue
		}
		counts[ref]++
		if _, ok := firstLine[ref]; !ok && row.Line > 0 {
			firstLine[ref] = row.Line
		}
	}

	statusLine := in.StatusLine
	if statusLine <= 0 {
		statusLine = 1
	}

	var findings []CoverageFinding
	for _, req := range in.Inventory {
		ref := strings.TrimSpace(req)
		if ref == "" {
			continue
		}
		n := counts[ref]
		switch {
		case n == 0:
			findings = append(findings, CoverageFinding{
				Path:     in.Path,
				Line:     statusLine,
				Kind:     FindingUnmetVerification,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s: Complete/Implemented requirement %q is uncovered (zero Verification mapping rows)",
					in.Path, ref,
				),
			})
		case n > 1:
			line := firstLine[ref]
			if line <= 0 {
				line = statusLine
			}
			findings = append(findings, CoverageFinding{
				Path:     in.Path,
				Line:     line,
				Kind:     FindingDuplicateMapping,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s: Complete/Implemented requirement %q has duplicate mapping (%d Verification rows; expected exactly one)",
					in.Path, ref, n,
				),
			})
		}
	}
	return findings
}

// CheckDocumentCoverageCardinality parses status and Verification
// inventory/rows for content and runs the coverage-cardinality pass.
// Convenience for fixture tests; pure over the supplied content (path is
// diagnostic metadata only).
func CheckDocumentCoverageCardinality(path, content string) []CoverageFinding {
	status := ParseDocumentStatusMarkdown(path, content)
	model := ParseVerificationMarkdown(path, content)
	return CheckCoverageCardinality(CoverageCardinalityInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Inventory:  model.Inventory,
		Rows:       model.Rows,
	})
}
