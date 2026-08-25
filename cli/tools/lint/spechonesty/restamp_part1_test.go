// Regression tests for the Phase 2 WB-2 restamp part 1 bead
// (phase2-doc-truth-plan-2026-07-13.md WB-2). This bead owns FEAT-011,
// FEAT-022, FEAT-023, and FEAT-027 — the four Complete/Implemented docs
// that sibling restamp beads (part 2, part 3) do not cover. Both tests
// guard the restamp itself: neither test may pass by construction if a
// future edit re-stamps one of these documents Complete/Implemented
// without also building the WB-1 Verification evidence that status
// requires.
package spechonesty

import (
	"os"
	"path/filepath"
	"testing"
)

// restampPart1Docs are the documents this bead is responsible for restamping
// (docs/helix/06-iterate/phase2-doc-truth-plan-2026-07-13.md WB-2, the
// FEAT-011/FEAT-022/FEAT-023/FEAT-027 rows the parent bead's part-2 restamp
// list omits).
var restampPart1Docs = []string{
	filepath.Join("01-frame", "features", "FEAT-011-skills.md"),
	filepath.Join("01-frame", "features", "FEAT-022-prompt-evidence-assembly.md"),
	filepath.Join("01-frame", "features", "FEAT-023-sync.md"),
	filepath.Join("01-frame", "features", "FEAT-027-prose-quality-support.md"),
}

// helixDocsRoot resolves the docs/helix directory from this test package's
// location under cli/tools/lint/spechonesty.
func helixDocsRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "docs", "helix"))
	if err != nil {
		t.Fatalf("resolve docs/helix root: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("docs/helix root not found at %s: %v", root, err)
	}
	return root
}

// TestCompleteRequiresFullCurrentVerification asserts that every document
// this bead governs which remains Complete/Implemented after the restamp
// carries a complete, currently-resolvable Verification mapping: no
// zero-evidence, coverage-cardinality, missing-static-check, missing-
// command-allowlist, missing-test-symbol, or missing-runtime-artifact
// finding. A document with real code behind it does not get a free pass —
// it must meet the same bar as a doc with a full REQ-* Verification table,
// or it must not claim Complete/Implemented.
func TestCompleteRequiresFullCurrentVerification(t *testing.T) {
	docsRoot := helixDocsRoot(t)
	repoRoot := filepath.Dir(filepath.Dir(docsRoot)) // .../docs/helix -> .../docs -> repo root

	for _, rel := range restampPart1Docs {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			path := filepath.Join(docsRoot, rel)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(data)

			statusRes := ParseDocumentStatusMarkdown(path, content)
			if !IsCompleteStatus(statusRes.Status) {
				// Downgraded documents carry a short evidence-gap
				// Verification block instead of a full mapping; that is
				// the WB-2-sanctioned alternative to full verification.
				return
			}

			if findings := CheckDocumentZeroEvidence(path, content); len(findings) != 0 {
				t.Errorf("%s: stamped %s but zero_evidence findings present: %+v", path, statusRes.Status, findings)
			}
			if findings := CheckDocumentCoverageCardinality(path, content); len(findings) != 0 {
				t.Errorf("%s: stamped %s but coverage-cardinality findings present: %+v", path, statusRes.Status, findings)
			}
			if findings := CheckDocumentStaticChecks(path, content); len(findings) != 0 {
				t.Errorf("%s: stamped %s but missing-static-check findings present: %+v", path, statusRes.Status, findings)
			}
			if findings := CheckDocumentCommandAllowlist(path, content); len(findings) != 0 {
				t.Errorf("%s: stamped %s but command-allowlist findings present: %+v", path, statusRes.Status, findings)
			}
			if findings := CheckDocumentTestSymbols(path, content, repoRoot); len(findings) != 0 {
				t.Errorf("%s: stamped %s but missing-test-symbol findings present: %+v", path, statusRes.Status, findings)
			}
			if findings := CheckDocumentRuntimeArtifacts(path, content, repoRoot); len(findings) != 0 {
				t.Errorf("%s: stamped %s but missing-runtime-artifact findings present: %+v", path, statusRes.Status, findings)
			}
		})
	}
}

// TestRestampedCompleteDocsRequireObservedVerification asserts that a
// document this bead governs which remains Complete/Implemented cannot
// satisfy observation freshness on the strength of its Verification table
// alone — a current-revision, exit-zero observation must exist for every
// row. Since this bead's restamp leaves none of the four documents
// Complete/Implemented, the meaningful assertion is the restamp itself:
// none may silently regain a Complete/Implemented stamp without also
// wiring real observations, which CheckObservationFreshness must reject
// when none are supplied.
func TestRestampedCompleteDocsRequireObservedVerification(t *testing.T) {
	docsRoot := helixDocsRoot(t)
	const fixtureRevision = "current-rev-under-test"

	for _, rel := range restampPart1Docs {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			path := filepath.Join(docsRoot, rel)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(data)

			statusRes := ParseDocumentStatusMarkdown(path, content)
			model := ParseVerificationMarkdown(path, content)

			// No per-row observations are wired into these documents yet
			// (that is exactly the evidence gap the restamp records).
			findings := CheckObservationFreshness(FreshnessInput{
				CurrentRevision: fixtureRevision,
				Status:          statusRes.Status,
				Path:            path,
				Rows:            model.Rows,
				Observations:    nil,
			})

			if IsCompleteStatus(statusRes.Status) {
				if len(model.Rows) == 0 {
					t.Fatalf("%s: stamped %s with zero Verification rows — restamp must downgrade or add rows before this document can be Complete/Implemented", path, statusRes.Status)
				}
				if len(findings) == 0 {
					t.Fatalf("%s: stamped %s but produced no observation-freshness findings with zero recorded observations; every row needs a current-revision, exit-zero observation", path, statusRes.Status)
				}
				return
			}

			// Non-Complete statuses never produce freshness findings —
			// this is the sanctioned downgrade path this bead used.
			if len(findings) != 0 {
				t.Fatalf("%s: stamped %s (non-Complete) but observation-freshness findings present: %+v", path, statusRes.Status, findings)
			}
		})
	}
}
