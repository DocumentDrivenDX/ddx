package spechonesty

import (
	"path/filepath"
	"testing"
)

func TestParseDocumentStatusBodyAndFrontmatter(t *testing.T) {
	t.Run("body_with_qualifier", func(t *testing.T) {
		res := ParseDocumentStatusMarkdown("SD-001-body.md",
			"# Title\n\n**Status:** Proposed (Accepted after review)\n\nBody.\n")
		if res.Source != StatusSourceBody {
			t.Fatalf("Source = %q, want %q", res.Source, StatusSourceBody)
		}
		if res.Status != StatusProposed {
			t.Fatalf("Status = %q, want %q", res.Status, StatusProposed)
		}
		if res.MissingDesignStatus {
			t.Fatal("unexpected MissingDesignStatus")
		}
	})

	t.Run("frontmatter", func(t *testing.T) {
		res := ParseDocumentStatusMarkdown("TD-001-fm.md",
			"---\nstatus: Complete\n---\n# Title\n")
		if res.Source != StatusSourceFrontmatter {
			t.Fatalf("Source = %q, want %q", res.Source, StatusSourceFrontmatter)
		}
		if res.Status != StatusComplete {
			t.Fatalf("Status = %q, want %q", res.Status, StatusComplete)
		}
	})

	t.Run("missing_design", func(t *testing.T) {
		res := ParseDocumentStatusMarkdown(filepath.Join("solution-designs", "SD-013-unstamped.md"),
			"---\nddx:\n  id: SD-013\n---\n# No status\n")
		if !res.IsDesign {
			t.Fatal("expected IsDesign")
		}
		if !res.MissingDesignStatus {
			t.Fatal("expected MissingDesignStatus")
		}
		if res.Source != StatusSourceNone {
			t.Fatalf("Source = %q, want %q", res.Source, StatusSourceNone)
		}
	})

	t.Run("non_design_without_status_ok", func(t *testing.T) {
		res := ParseDocumentStatusMarkdown("notes/README.md", "# Note\n\nNo status.\n")
		if res.IsDesign {
			t.Fatal("README should not be design")
		}
		if res.MissingDesignStatus {
			t.Fatal("non-design must not flag missing status")
		}
	})
}
