// Citation-granularity predicate for Verification mapping rows
// (phase2-doc-truth-plan WB-1 steps 3-4: a test-file path without the
// specific Test* it proves is not coverage).
//
// This file supplies the test-evidence branch of the exclusion predicate
// used by coverage-cardinality: file-only test citations do not count as
// covering; rows that name an exact Test* symbol do. Existence of the
// named symbol on disk is intentionally not resolved here.
//
// Static-check and artifact-target classification are sibling territory.
// Coverage cardinality diagnostics, zero-evidence, waivers, and command
// allowlists are out of scope. Read-only: pure over the row fields.
package spechonesty

// IsCoveringCitation reports whether a Verification mapping row names
// evidence at sufficient citation granularity to count as covering a
// requirement for the coverage-cardinality exclusion predicate.
//
// Test-evidence rules:
//   - exact Test* symbol (bare or scoped path/package:Test*) → covering
//   - evidence that is only a Go test file path with no Test* → non-covering
//
// Does not resolve whether named Test* symbols exist on disk. Non-test
// evidence kinds (static checks, artifacts) are classified by sibling
// passes; until those branches land they are treated as non-covering by
// this predicate so file-only citations cannot silently count.
//
// Pure and read-only: no filesystem or network access.
func IsCoveringCitation(row VerificationRow) bool {
	covering, recognized := testCitationCovering(row)
	if recognized {
		return covering
	}
	// Sibling: static checks and artifact targets extend this predicate.
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
