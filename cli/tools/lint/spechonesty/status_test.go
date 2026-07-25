package spechonesty

import (
	"path/filepath"
	"testing"
)

func statusFixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "status", name)
}

// TestStatusParserRecognizesBodyAndFrontmatter verifies both status
// encodings normalize free-text qualifiers to a base DocStatus.
func TestStatusParserRecognizesBodyAndFrontmatter(t *testing.T) {
	t.Run("body_with_qualifier", func(t *testing.T) {
		path := statusFixture(t, "body_with_qualifier.md")
		res, err := ParseDocumentStatus(path)
		if err != nil {
			t.Fatalf("ParseDocumentStatus: %v", err)
		}
		if res.Source != StatusSourceBody {
			t.Fatalf("Source = %q, want %q", res.Source, StatusSourceBody)
		}
		if res.Status != StatusProposed {
			t.Fatalf("Status = %q, want %q (raw %q)", res.Status, StatusProposed, res.Raw)
		}
		if res.MissingDesignStatus {
			t.Fatal("unexpected MissingDesignStatus for stamped document")
		}
	})

	t.Run("frontmatter_with_qualifier", func(t *testing.T) {
		path := statusFixture(t, "frontmatter_status.md")
		res, err := ParseDocumentStatus(path)
		if err != nil {
			t.Fatalf("ParseDocumentStatus: %v", err)
		}
		if res.Source != StatusSourceFrontmatter {
			t.Fatalf("Source = %q, want %q", res.Source, StatusSourceFrontmatter)
		}
		if res.Status != StatusComplete {
			t.Fatalf("Status = %q, want %q (raw %q)", res.Status, StatusComplete, res.Raw)
		}
		if res.MissingDesignStatus {
			t.Fatal("unexpected MissingDesignStatus for stamped document")
		}
	})

	// Inline free-text qualifiers cover the remaining base statuses.
	t.Run("normalize_bases", func(t *testing.T) {
		cases := []struct {
			raw  string
			want DocStatus
		}{
			{"Implemented (partial notes ok)", StatusImplemented},
			{"In Progress — remaining work", StatusInProgress},
			{"Deferred until Phase 3", StatusDeferred},
			{"Aspirational metric only", StatusAspirational},
		}
		for _, tc := range cases {
			got := NormalizeStatus(tc.raw)
			if got != tc.want {
				t.Errorf("NormalizeStatus(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		}
	})
}

// TestStatusParserFlagsMissingDesignStatus verifies SD/TD/ADR documents
// without a status marker fail, while non-design docs do not.
func TestStatusParserFlagsMissingDesignStatus(t *testing.T) {
	designFixtures := []string{
		"SD-013-unstamped.md",
		"TD-027-unstamped.md",
		"ADR-001-unstamped.md",
	}
	for _, name := range designFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			path := statusFixture(t, name)
			res, err := ParseDocumentStatus(path)
			if err != nil {
				t.Fatalf("ParseDocumentStatus: %v", err)
			}
			if !res.IsDesign {
				t.Fatalf("IsDesign = false for design fixture %s", name)
			}
			if !res.MissingDesignStatus {
				t.Fatalf("MissingDesignStatus = false, want true for unstamped design %s", name)
			}
			if res.Source != StatusSourceNone {
				t.Fatalf("Source = %q, want %q", res.Source, StatusSourceNone)
			}
		})
	}

	t.Run("non_design_without_status_ok", func(t *testing.T) {
		path := statusFixture(t, "non_design_note.md")
		res, err := ParseDocumentStatus(path)
		if err != nil {
			t.Fatalf("ParseDocumentStatus: %v", err)
		}
		if res.IsDesign {
			t.Fatal("non-design fixture must not be classified as SD/TD/ADR")
		}
		if res.MissingDesignStatus {
			t.Fatal("non-design must not flag missing status under the SD/TD/ADR rule")
		}
		if res.Source != StatusSourceNone {
			t.Fatalf("Source = %q, want %q", res.Source, StatusSourceNone)
		}
	})
}
