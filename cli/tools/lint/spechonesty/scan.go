// Docs-directory scan for the spechonesty CLI (WB-1 validation path).
//
// Walks a docs tree for Markdown files, runs the status parser and the
// Verification-based validation passes, and collects diagnostics.
// Read-only: never writes.
package spechonesty

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Diagnostic is one CLI-reported failure for a docs-directory scan.
type Diagnostic struct {
	// Path is the document path relative to the scan root when possible,
	// otherwise the absolute path passed to the parser.
	Path string
	// Line is the 1-based diagnostic line (0 when not applicable).
	Line int
	// Kind classifies the failure (e.g. missing_status, parse_error).
	Kind string
	// Message is a human-readable description.
	Message string
}

// ScanDocsDirectory walks root recursively for `.md` files, parses each
// document's status stamp, and returns diagnostics for:
//   - I/O or parse errors
//   - missing status on SD/TD/ADR design documents
//   - duplicate document ids within the helix lint scope (HelixLintRelativeDirs)
//   - duplicate user-story ids across feature documents (01-frame/features/)
//   - Complete/Implemented Verification rows without a current-revision,
//     exit-zero, evidenced observation (WB-1 step 4; empty report/revision)
//
// The tree is never modified. Non-design documents without a status stamp
// do not produce a missing-status diagnostic. Duplicate-id and
// duplicate-US-id failures are non-waivable (WB-1 step 5). Equivalent to
// ScanDocsDirectoryWithReport(root, nil, "") — no persisted observations,
// so every Complete/Implemented mapping row must fail as unobserved.
func ScanDocsDirectory(root string) ([]Diagnostic, error) {
	return ScanDocsDirectoryWithReport(root, nil, "")
}

// ScanDocsDirectoryWithReport is ScanDocsDirectory plus observation-report
// correlation: report rows (typically read via ReadObservationReport from a
// prior `spechonesty observe` run) are matched to each Complete/Implemented
// document by canonical document id and validated against revision via
// CheckDocumentObservationReport. A nil or empty report behaves exactly
// like ScanDocsDirectory: every Complete/Implemented mapping row fails as
// unobserved.
func ScanDocsDirectoryWithReport(root string, report []ObservationReportRow, revision string) ([]Diagnostic, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		diags, err := scanOne(root, filepath.Dir(root), report, revision)
		if err != nil {
			return nil, err
		}
		if len(diags) == 0 {
			return nil, nil
		}
		return diags, nil
	}

	var diags []Diagnostic
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		fileDiags, scanErr := scanOne(path, root, report, revision)
		if scanErr != nil {
			diags = append(diags, Diagnostic{
				Path:    path,
				Kind:    "parse_error",
				Message: scanErr.Error(),
			})
			return nil
		}
		diags = append(diags, fileDiags...)
		return nil
	})
	if err != nil {
		return diags, err
	}

	// Duplicate document-id scan is confined to HelixLintRelativeDirs under root.
	dupDiags, dupErr := ScanDuplicateDocumentIDs(root)
	if dupErr != nil {
		return diags, dupErr
	}
	diags = append(diags, dupDiags...)

	// Duplicate US-id scan is confined to 01-frame/features/ under root.
	usDiags, usErr := ScanDuplicateUserStoryIDs(root)
	if usErr != nil {
		return diags, usErr
	}
	diags = append(diags, usDiags...)
	return diags, nil
}

// scanOne parses a single markdown file and returns diagnostics for the
// status gate plus the Verification-based validation passes.
func scanOne(path, repoRoot string, report []ObservationReportRow, revision string) ([]Diagnostic, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	statusRes := ParseDocumentStatusMarkdown(path, content)
	var diags []Diagnostic
	if statusRes.MissingDesignStatus {
		diags = append(diags, Diagnostic{
			Path:    path,
			Line:    1,
			Kind:    string(FindingMissingStatus),
			Message: "missing status stamp: SD/TD/ADR documents require a body **Status:** line or frontmatter status: key",
		})
	}

	model := ParseVerificationMarkdown(path, content)
	for _, finding := range CheckZeroEvidence(ZeroEvidenceInput{
		Path:       path,
		Status:     statusRes.Status,
		StatusLine: statusRes.Line,
		Rows:       model.Rows,
	}) {
		diags = append(diags, Diagnostic{
			Path:    finding.Path,
			Line:    finding.Line,
			Kind:    string(finding.Kind),
			Message: finding.Message,
		})
	}
	for _, finding := range CheckDocumentStaticChecks(path, content) {
		diags = append(diags, Diagnostic{
			Path:    finding.Path,
			Line:    finding.Line,
			Kind:    string(finding.Kind),
			Message: finding.Message,
		})
	}
	for _, finding := range CheckDocumentRuntimeArtifacts(path, content, repoRoot) {
		diags = append(diags, Diagnostic{
			Path:    finding.Path,
			Line:    finding.Line,
			Kind:    string(finding.Kind),
			Message: finding.Message,
		})
	}
	for _, finding := range CheckDocumentObservationReport(path, content, report, revision) {
		diags = append(diags, Diagnostic{
			Path:    finding.Path,
			Line:    finding.Line,
			Kind:    string(finding.Kind),
			Message: finding.Message,
		})
	}
	return diags, nil
}
