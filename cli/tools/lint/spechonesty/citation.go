// Citation-granularity predicate for Verification mapping rows
// (phase2-doc-truth-plan WB-1 steps 3-4: a test-file path without the
// specific Test* it proves is not coverage; exact static checks and
// artifact targets do count as covering).
//
// This file supplies the exclusion predicate used by coverage-cardinality:
// file-only test citations and bare file paths that do not name an
// artifact target do not count as covering; rows that name an exact
// Test* symbol, deterministic static check (check:…), or inspectable
// runtime artifact do. Existence of named targets on disk is
// intentionally not resolved here.
//
// Coverage cardinality diagnostics, zero-evidence, waivers, and command
// allowlists are out of scope. Read-only: pure over the row fields.
package spechonesty

// IsCoveringCitation reports whether a Verification mapping row names
// evidence at sufficient citation granularity to count as covering a
// requirement for the coverage-cardinality exclusion predicate.
//
// Rules:
//   - exact Test* symbol (bare or scoped path/package:Test*) → covering
//   - evidence that is only a Go test file path with no Test* → non-covering
//   - exact static check (check:<name>) → covering
//   - exact runtime-artifact target (path classified as runtime_artifact)
//     → covering
//   - bare file path / free text that is not a Test*, static check, or
//     artifact target → non-covering
//
// Does not resolve whether named targets exist on disk. Pure and
// read-only: no filesystem or network access.
func IsCoveringCitation(row VerificationRow) bool {
	covering, recognized := testCitationCovering(row)
	if recognized {
		return covering
	}
	if isStaticCheckTarget(row.EvidenceTarget) {
		return true
	}
	if isArtifactTarget(row.EvidenceTarget) {
		return true
	}
	// Bare file paths, free text, empty cells: not covering.
	return false
}

// testCitationCovering classifies test-shaped evidence for citation
// granularity. recognized is true when the evidence is a file-only test
// path or an exact Test* symbol (bare or scoped).
func testCitationCovering(row VerificationRow) (covering bool, recognized bool) {
	parsed := parseTestEvidenceTarget(row.EvidenceTarget, row.Command)
	switch parsed.kind {
	case evidenceKindTestSymbol:
		// Exact Test* named — covering at citation granularity.
		// Disk existence is a separate resolver pass.
		return true, true
	case evidenceKindFileOnly:
		// Test file path without a mapped Test* — not coverage.
		return false, true
	default:
		return false, false
	}
}

// isStaticCheckTarget reports whether evidence names an exact deterministic
// static check of the form check:<name> (non-empty name after the prefix).
// Existence of the check on disk is not resolved.
func isStaticCheckTarget(evidenceTarget string) bool {
	_, ok := parseStaticCheckTarget(evidenceTarget)
	return ok
}

// isArtifactTarget reports whether evidence names an exact inspectable
// runtime-artifact target. Delegates to ClassifyRuntimeArtifactTarget so
// citation granularity and the runtime-artifact resolver agree: bare Go
// file paths, Test* symbols, static checks, free text, and empty cells
// are not artifact targets.
func isArtifactTarget(evidenceTarget string) bool {
	return ClassifyRuntimeArtifactTarget(evidenceTarget).Kind == RuntimeArtifactClassRuntime
}
