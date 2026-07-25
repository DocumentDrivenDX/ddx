// Zero-evidence pass for Complete/Implemented documents (WB-1 step 5).
//
// Consumes the status parser (ParseDocumentStatus*) and Verification
// mapping model (ParseVerification*) and emits exactly one diagnostic
// when a Complete or Implemented document has an empty row set.
// Per-requirement cardinality, citation granularity, waivers, and
// evidence-target resolution are sibling children. Read-only.
package spechonesty

import "fmt"

// ZeroEvidenceInput is the document-level input for the zero-evidence pass.
// Callers supply status from the status parser and rows from the
// Verification mapping parser; this pass does not re-parse markdown.
type ZeroEvidenceInput struct {
	// Path is the document path recorded on the diagnostic.
	Path string
	// Status is the normalized base document status.
	Status DocStatus
	// StatusLine is the 1-based line of the status stamp (0 → 1).
	StatusLine int
	// Rows is the well-formed Verification mapping row set for the document.
	Rows []VerificationRow
}

// CheckZeroEvidence emits exactly one diagnostic when a document stamped
// Complete or Implemented has zero Verification mapping rows.
//
// Rules:
//   - Complete/Implemented + empty rows → one FindingZeroEvidence naming
//     the document path and status.
//   - Complete/Implemented + any non-empty row set → no findings from
//     this pass (per-requirement cardinality is a sibling child).
//   - Any other status → no findings, even with empty rows.
//
// Pure and read-only: no filesystem or network access.
func CheckZeroEvidence(in ZeroEvidenceInput) []CoverageFinding {
	if !IsCompleteStatus(in.Status) {
		return nil
	}
	if len(in.Rows) > 0 {
		return nil
	}
	line := in.StatusLine
	if line <= 0 {
		line = 1
	}
	return []CoverageFinding{{
		Path:     in.Path,
		Line:     line,
		Kind:     FindingZeroEvidence,
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s: document stamped %s has zero Verification mapping rows",
			in.Path, in.Status,
		),
	}}
}

// CheckDocumentZeroEvidence parses status and Verification rows for content
// and runs the zero-evidence pass. Convenience for fixture tests; pure
// over the supplied content (path is diagnostic metadata only).
func CheckDocumentZeroEvidence(path, content string) []CoverageFinding {
	status := ParseDocumentStatusMarkdown(path, content)
	model := ParseVerificationMarkdown(path, content)
	return CheckZeroEvidence(ZeroEvidenceInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
	})
}
