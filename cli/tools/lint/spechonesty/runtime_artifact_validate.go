// Runtime-artifact validation pass for Complete/Implemented Verification
// mapping rows (phase2-doc-truth-plan WB-1 steps 3-4).
//
// Consumes the status parser, Verification mapping model, and the
// runtime-artifact target resolver. For each Complete/Implemented row
// classified as an inspectable runtime artifact target, resolves the
// path under the repository root. Existing (resolved) artifacts emit
// no diagnostic — the positive path this file owns.
//
// Emission of missing-artifact diagnostics for unresolved targets is a
// sibling child; this pass only guarantees that current-revision
// artifacts do not false-positive. Read-only: never mutates documents
// or fixtures.
package spechonesty

// RuntimeArtifactFindingKind classifies a runtime-artifact validation
// diagnostic. The missing-artifact kind is reserved for the sibling
// that emits unresolved-target diagnostics; this pass currently
// returns no findings on the positive (resolved) path.
type RuntimeArtifactFindingKind string

const (
	// FindingMissingRuntimeArtifact is emitted when a mapped inspectable
	// runtime artifact path does not resolve under the repository root.
	// Emission is owned by the missing-artifact sibling; the constant
	// exists so the positive-path pass and tests can name the diagnostic
	// kind without inventing a second vocabulary.
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
	// ArtifactPath is the normalized/offending path string from the
	// mapping row (resolver Path). Empty when not a runtime artifact.
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
//   - Runtime-artifact target that does not resolve → no emission from
//     this pass; the missing-artifact sibling owns that diagnostic body.
//
// Resolution is independent per row. Read-only: delegates existence
// checks to ResolveRuntimeArtifactRow; never writes.
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
		// Unresolved runtime artifact: missing-artifact diagnostic body
		// is intentionally not emitted here (sibling child). Keep the
		// branch explicit so resolved rows never fall through into a
		// future diagnostic by accident.
		_ = res
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
