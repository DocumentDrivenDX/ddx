// Runtime-artifact validation pass for Complete/Implemented Verification
// mapping rows (phase2-doc-truth-plan WB-1 steps 3-4).
//
// Consumes the status parser, Verification mapping model, and the
// runtime-artifact target resolver. For each Complete/Implemented row
// classified as an inspectable runtime artifact target, resolves the
// path under the repository root. Existing (resolved) artifacts emit
// no diagnostic. Unresolved inspectable targets emit one
// FindingMissingRuntimeArtifact diagnostic that carries the mapping
// row's requirement id and the source document path so the document
// owner can locate the exact Verification row.
//
// Read-only: never mutates documents or fixtures. Does not reimplement
// classification or path resolution — consumes ResolveRuntimeArtifactRow.
package spechonesty

import "fmt"

// RuntimeArtifactFindingKind classifies a runtime-artifact validation
// diagnostic.
type RuntimeArtifactFindingKind string

const (
	// FindingMissingRuntimeArtifact is emitted when a mapped inspectable
	// runtime artifact path does not resolve under the repository root.
	FindingMissingRuntimeArtifact RuntimeArtifactFindingKind = "missing_runtime_artifact"
)

// RuntimeArtifactFinding is one diagnostic from the runtime-artifact
// validation pass.
type RuntimeArtifactFinding struct {
	// Path is the document path recorded on the diagnostic.
	Path string
	// Line is the 1-based Verification mapping row line (0 when unknown).
	Line int
	// RequirementRef is the requirement/anchor the row covers.
	RequirementRef string
	// EvidenceTarget is the raw evidence cell from the mapping row.
	EvidenceTarget string
	// ArtifactPath is the mapped artifact path exactly as written on the
	// Verification row (raw EvidenceTarget text). It is never reconstructed
	// from resolver output, cleaned/normalized paths, or absolute
	// ResolvedPath values. Empty when not a runtime artifact.
	ArtifactPath string
	// Kind classifies the failure.
	Kind RuntimeArtifactFindingKind
	// Severity is SeverityError for Complete/Implemented target failures.
	Severity FindingSeverity
	// Message is a human-readable description.
	Message string
}

// RuntimeArtifactInput is the document-level input for runtime-artifact
// validation. Callers supply status and rows from the status and
// Verification parsers; RepoRoot is the filesystem root used for
// existence checks.
type RuntimeArtifactInput struct {
	// Path is the document path recorded on diagnostics.
	Path string
	// Status is the normalized base document status.
	Status DocStatus
	// StatusLine is the 1-based line of the status stamp (0 → 1).
	StatusLine int
	// Rows is the well-formed Verification mapping row set.
	Rows []VerificationRow
	// RepoRoot is the repository (or fixture) root against which
	// runtime-artifact paths are resolved.
	RepoRoot string
}

// CheckRuntimeArtifactResolution validates inspectable runtime-artifact
// evidence targets for Complete/Implemented documents against RepoRoot.
//
// Rules:
//   - Non-Complete/Implemented statuses → no findings.
//   - Empty row set → no findings (zero-evidence sibling owns that case).
//   - Out-of-band evidence (Test*, check:*, free text) → ignored here.
//   - Runtime-artifact target that resolves under RepoRoot → no diagnostic
//     for that row (positive path; prevents false positives).
//   - Runtime-artifact target that does not resolve → one
//     FindingMissingRuntimeArtifact diagnostic whose Path is the source
//     document, whose RequirementRef is the offending Verification
//     row's requirement id (never inferred from prose), and whose
//     ArtifactPath / message path fragment is the mapped artifact text
//     exactly as written on the row (not cleaned, absolute, resolved,
//     or inferred from the resolver).
//
// Resolution is independent per row. Read-only: delegates existence
// checks to ResolveRuntimeArtifactRow; never writes. The resolver may
// use a cleaned/absolute filesystem path internally for existence
// checks; that path is never copied into the diagnostic display fields.
func CheckRuntimeArtifactResolution(in RuntimeArtifactInput) []RuntimeArtifactFinding {
	if !IsCompleteStatus(in.Status) {
		return nil
	}
	if len(in.Rows) == 0 {
		return nil
	}

	var findings []RuntimeArtifactFinding
	for _, row := range in.Rows {
		res := ResolveRuntimeArtifactRow(in.RepoRoot, row)
		if res.Kind != RuntimeArtifactClassRuntime {
			// Test*, static checks, free text: other resolvers own these.
			continue
		}
		if res.Resolved {
			// Existing inspectable artifact: pass, no diagnostic.
			continue
		}
		// Unresolved inspectable runtime artifact: emit a diagnostic
		// that identifies the mapping-row requirement and source doc.
		line := row.Line
		if line <= 0 {
			line = 1
		}
		reqRef := row.RequirementRef
		if reqRef == "" {
			// Prefer resolver copy when the row field is empty; still
			// do not invent ids from prose.
			reqRef = res.RequirementRef
		}
		// Display path must be the raw mapped artifact text from the
		// Verification row. Do not use res.Path (cleaned/normalized),
		// res.ResolvedPath (absolute), or any inferred reconstruction.
		artifactPath := row.EvidenceTarget
		findings = append(findings, RuntimeArtifactFinding{
			Path:           in.Path,
			Line:           line,
			RequirementRef: reqRef,
			EvidenceTarget: row.EvidenceTarget,
			ArtifactPath:   artifactPath,
			Kind:           FindingMissingRuntimeArtifact,
			Severity:       SeverityError,
			Message: fmt.Sprintf(
				"%s: missing runtime artifact %q (requirement %q); mapped path does not exist under repository root",
				in.Path, artifactPath, reqRef,
			),
		})
	}
	return findings
}

// CheckDocumentRuntimeArtifacts parses status and Verification rows for
// content and runs runtime-artifact validation against repoRoot.
// Convenience for fixture tests; read-only over both the markdown
// content and the repository tree.
func CheckDocumentRuntimeArtifacts(path, content, repoRoot string) []RuntimeArtifactFinding {
	status := ParseDocumentStatusMarkdown(path, content)
	model := ParseVerificationMarkdown(path, content)
	return CheckRuntimeArtifactResolution(RuntimeArtifactInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   repoRoot,
	})
}
