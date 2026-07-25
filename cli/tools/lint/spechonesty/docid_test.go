package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDoc(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func fixtureDoc(id, status, extra string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("ddx:\n")
	b.WriteString("  id: ")
	b.WriteString(id)
	b.WriteString("\n")
	if extra != "" {
		b.WriteString(extra)
		if !strings.HasSuffix(extra, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("---\n")
	b.WriteString("# Fixture ")
	b.WriteString(id)
	b.WriteString("\n\n")
	if status != "" {
		b.WriteString("**Status:** ")
		b.WriteString(status)
		b.WriteString("\n\n")
	}
	b.WriteString("Body.\n")
	return b.String()
}

// TestDuplicateDocumentIDFails: two in-scope documents sharing a document id
// (TD-040 class) produce a failure naming both documents.
func TestDuplicateDocumentIDFails(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "02-design", "technical-designs", "TD-040-leftover.md")
	b := filepath.Join(root, "02-design", "technical-designs", "TD-040-cross-repo.md")
	// Distinct full frontmatter ids that both canonicalize to TD-040.
	writeDoc(t, a, fixtureDoc("TD-040", "Proposed", ""))
	writeDoc(t, b, fixtureDoc("TD-040-cross-repo-blocker-recheck", "Proposed", ""))

	findings, err := FindDuplicateDocumentIDs(root)
	if err != nil {
		t.Fatalf("FindDuplicateDocumentIDs: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1; findings=%v", len(findings), findings)
	}
	f := findings[0]
	if f.Kind != FindingDuplicateID {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingDuplicateID)
	}
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if !strings.Contains(f.Message, "TD-040") {
		t.Fatalf("message should name id TD-040, got %q", f.Message)
	}
	if !strings.Contains(f.Message, a) {
		t.Fatalf("message should name first document %s, got %q", a, f.Message)
	}
	if !strings.Contains(f.Message, b) {
		t.Fatalf("message should name second document %s, got %q", b, f.Message)
	}
}

// TestDuplicateDocumentIDIgnoresWaiver: a duplicate document-id failure
// remains SeverityError even when a syntactically valid reasoned waiver
// is present and the status is non-Complete.
func TestDuplicateDocumentIDIgnoresWaiver(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "01-frame", "features", "FEAT-900-a.md")
	b := filepath.Join(root, "01-frame", "features", "FEAT-900-b.md")
	waiverExtra := "spec:verification-waiver: \"REQ-001 evidence not yet landed; temporary exception\"\n"
	writeDoc(t, a, fixtureDoc("FEAT-900", "Proposed", waiverExtra))
	writeDoc(t, b, fixtureDoc("FEAT-900", "Proposed", waiverExtra))

	findings, err := FindDuplicateDocumentIDs(root)
	if err != nil {
		t.Fatalf("FindDuplicateDocumentIDs: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Kind != FindingDuplicateID {
		t.Fatalf("Kind = %q, want %q", findings[0].Kind, FindingDuplicateID)
	}

	waiver, err := ParseVerificationWaiverFile(a)
	if err != nil {
		t.Fatalf("ParseVerificationWaiverFile: %v", err)
	}
	if waiver == nil || !waiver.Present || !waiver.Reasoned {
		t.Fatalf("expected reasoned waiver on fixture, got %+v", waiver)
	}

	got := ApplyWaiverPolicy(StatusProposed, waiver, findings)
	if len(got) != 1 {
		t.Fatalf("len(after waiver) = %d, want 1", len(got))
	}
	if got[0].Severity != SeverityError {
		t.Fatalf("duplicate_id severity after waiver = %q, want %q (non-waivable)", got[0].Severity, SeverityError)
	}
	if got[0].Kind != FindingDuplicateID {
		t.Fatalf("Kind = %q, want %q", got[0].Kind, FindingDuplicateID)
	}
}

// TestDuplicateDocumentIDIgnoresOutOfScopePaths: a same-id document outside
// HelixLintRelativeDirs does not trigger a failure when only one in-scope
// document carries that id.
func TestDuplicateDocumentIDIgnoresOutOfScopePaths(t *testing.T) {
	root := t.TempDir()
	inScope := filepath.Join(root, "02-design", "technical-designs", "TD-901-in-scope.md")
	// Outside the five 02-design subdirs and features/.
	outOfScope := filepath.Join(root, "06-iterate", "notes", "TD-901-out-of-scope.md")
	// Also outside: a sibling under 02-design root (not one of the five).
	outOfScope2 := filepath.Join(root, "02-design", "plans", "TD-901-plan.md")

	writeDoc(t, inScope, fixtureDoc("TD-901", "Proposed", ""))
	writeDoc(t, outOfScope, fixtureDoc("TD-901", "Proposed", ""))
	writeDoc(t, outOfScope2, fixtureDoc("TD-901", "Proposed", ""))

	findings, err := FindDuplicateDocumentIDs(root)
	if err != nil {
		t.Fatalf("FindDuplicateDocumentIDs: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no duplicate findings when only one in-scope doc has the id; got %v", findings)
	}

	// Sanity: two in-scope copies still fail.
	inScope2 := filepath.Join(root, "02-design", "adr", "TD-901-also.md")
	writeDoc(t, inScope2, fixtureDoc("TD-901", "Accepted", ""))
	findings, err = FindDuplicateDocumentIDs(root)
	if err != nil {
		t.Fatalf("FindDuplicateDocumentIDs (2nd): %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1 after second in-scope copy", len(findings))
	}
	if !strings.Contains(findings[0].Message, inScope) || !strings.Contains(findings[0].Message, inScope2) {
		t.Fatalf("message should name both in-scope paths, got %q", findings[0].Message)
	}
	if strings.Contains(findings[0].Message, outOfScope) || strings.Contains(findings[0].Message, outOfScope2) {
		t.Fatalf("message must not list out-of-scope paths, got %q", findings[0].Message)
	}
}

func TestCanonicalDocumentID(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"TD-040", "TD-040"},
		{"TD-040-cross-repo-blocker-recheck", "TD-040"},
		{"feat-001", "FEAT-001"},
		{"  \"SD-013\"  ", "SD-013"},
		{"CONCEPT-bounded-context-execution", "CONCEPT-bounded-context-execution"},
	}
	for _, tc := range cases {
		if got := CanonicalDocumentID(tc.raw); got != tc.want {
			t.Errorf("CanonicalDocumentID(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestWalkHelixLintDocsSkipsMissingAndOutOfScope(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "01-frame", "features", "FEAT-001.md")
	out := filepath.Join(root, "06-iterate", "plan.md")
	writeDoc(t, in, "# a\n")
	writeDoc(t, out, "# b\n")

	var seen []string
	err := WalkHelixLintDocs(root, func(path string) error {
		seen = append(seen, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkHelixLintDocs: %v", err)
	}
	if len(seen) != 1 || seen[0] != in {
		t.Fatalf("walked %v, want only %s", seen, in)
	}
}
