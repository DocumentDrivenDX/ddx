package spechonesty

import (
	"path/filepath"
	"strings"
	"testing"
)

func fixtureFeatureWithStories(id, status string, stories []string, extra string) string {
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
	b.WriteString("## User Stories\n\n")
	for _, us := range stories {
		b.WriteString("### ")
		b.WriteString(us)
		b.WriteString(": story title\n\n")
		b.WriteString("Acceptance criteria for ")
		b.WriteString(us)
		b.WriteString(".\n\n")
	}
	return b.String()
}

// TestDuplicateUserStoryIDFails: the same US-id appearing in two feature
// fixtures (US-087/US-088 class) produces a failure naming both feature documents.
func TestDuplicateUserStoryIDFails(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "01-frame", "features", "FEAT-008-web-ui.md")
	b := filepath.Join(root, "01-frame", "features", "FEAT-020-server-node-state.md")
	writeDoc(t, a, fixtureFeatureWithStories("FEAT-008", "Proposed", []string{"US-087", "US-088"}, ""))
	writeDoc(t, b, fixtureFeatureWithStories("FEAT-020", "Proposed", []string{"US-087", "US-088"}, ""))

	findings, err := FindDuplicateUserStoryIDs(root)
	if err != nil {
		t.Fatalf("FindDuplicateUserStoryIDs: %v", err)
	}
	// Two collisions: US-087 and US-088 each appear in both files.
	if len(findings) != 2 {
		t.Fatalf("len(findings) = %d, want 2; findings=%v", len(findings), findings)
	}
	seen := map[string]bool{}
	for _, f := range findings {
		if f.Kind != FindingDuplicateUSID {
			t.Fatalf("Kind = %q, want %q", f.Kind, FindingDuplicateUSID)
		}
		if f.Severity != SeverityError {
			t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
		}
		if !strings.Contains(f.Message, a) {
			t.Fatalf("message should name first feature %s, got %q", a, f.Message)
		}
		if !strings.Contains(f.Message, b) {
			t.Fatalf("message should name second feature %s, got %q", b, f.Message)
		}
		if strings.Contains(f.Message, "US-087") {
			seen["US-087"] = true
		}
		if strings.Contains(f.Message, "US-088") {
			seen["US-088"] = true
		}
	}
	if !seen["US-087"] || !seen["US-088"] {
		t.Fatalf("expected findings for both US-087 and US-088, seen=%v findings=%v", seen, findings)
	}
}

// TestDuplicateUserStoryIDIgnoresWaiver: a duplicate US-id failure remains
// SeverityError even when a fixture includes a syntactically valid reasoned
// waiver on a non-Complete status.
func TestDuplicateUserStoryIDIgnoresWaiver(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "01-frame", "features", "FEAT-900-a.md")
	b := filepath.Join(root, "01-frame", "features", "FEAT-900-b.md")
	waiverExtra := "spec:verification-waiver: \"REQ-001 evidence not yet landed; temporary exception\"\n"
	writeDoc(t, a, fixtureFeatureWithStories("FEAT-900", "Proposed", []string{"US-999"}, waiverExtra))
	writeDoc(t, b, fixtureFeatureWithStories("FEAT-901", "Proposed", []string{"US-999"}, waiverExtra))

	findings, err := FindDuplicateUserStoryIDs(root)
	if err != nil {
		t.Fatalf("FindDuplicateUserStoryIDs: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1; findings=%v", len(findings), findings)
	}
	if findings[0].Kind != FindingDuplicateUSID {
		t.Fatalf("Kind = %q, want %q", findings[0].Kind, FindingDuplicateUSID)
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
		t.Fatalf("duplicate_us_id severity after waiver = %q, want %q (non-waivable)", got[0].Severity, SeverityError)
	}
	if got[0].Kind != FindingDuplicateUSID {
		t.Fatalf("Kind = %q, want %q", got[0].Kind, FindingDuplicateUSID)
	}
}

func TestCanonicalUserStoryID(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"US-087", "US-087"},
		{"us-087", "US-087"},
		{"US-082b", "US-082b"},
		{"US-082B", "US-082b"},
		{"  US-088  ", "US-088"},
	}
	for _, tc := range cases {
		if got := CanonicalUserStoryID(tc.raw); got != tc.want {
			t.Errorf("CanonicalUserStoryID(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestDuplicateUserStoryIDIgnoresOutOfScopePaths(t *testing.T) {
	root := t.TempDir()
	inScope := filepath.Join(root, "01-frame", "features", "FEAT-910.md")
	// Outside features/: design doc mentioning a US heading must not count.
	outOfScope := filepath.Join(root, "02-design", "technical-designs", "TD-910.md")
	writeDoc(t, inScope, fixtureFeatureWithStories("FEAT-910", "Proposed", []string{"US-910"}, ""))
	writeDoc(t, outOfScope, "# TD-910\n\n**Status:** Proposed\n\n### US-910: should not collide\n")

	findings, err := FindDuplicateUserStoryIDs(root)
	if err != nil {
		t.Fatalf("FindDuplicateUserStoryIDs: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings when only one feature has the US-id; got %v", findings)
	}
}

func TestExtractUserStoryIDsFromHeadings(t *testing.T) {
	content := "# FEAT\n\n### US-087: One\n\nText mentioning US-088 inline only.\n\n### US-088: Two\n"
	refs := ExtractUserStoryIDs("x.md", content)
	if len(refs) != 2 {
		t.Fatalf("len(refs) = %d, want 2; refs=%v", len(refs), refs)
	}
	if refs[0].ID != "US-087" || refs[0].Line != 3 {
		t.Fatalf("refs[0] = %+v, want US-087 line 3", refs[0])
	}
	if refs[1].ID != "US-088" || refs[1].Line != 7 {
		t.Fatalf("refs[1] = %+v, want US-088 line 7", refs[1])
	}
}
