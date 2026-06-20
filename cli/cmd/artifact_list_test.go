package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestArtifactListByMediaType(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/adr-001.md", "---\nddx:\n  id: ADR-001\n  title: ADR One\n---\n# ADR-001\n")
	writeTestFile(t, dir, "docs/feat-001.md", "---\nddx:\n  id: FEAT-001\n  title: Feature One\n  media_type: text/markdown\n---\n# FEAT-001\n")
	writeTestFile(t, dir, "docs/diagram.md", "---\nddx:\n  id: DIAG-001\n  title: A Diagram\n  media_type: image/png\n---\n# DIAG-001\n")

	t.Run("filter text/markdown returns markdown docs", func(t *testing.T) {
		root := NewCommandFactory(dir).NewRootCommand()
		root.SetArgs([]string{"artifact", "list", "--media-type", "text/markdown", "--json"})

		out := &bytes.Buffer{}
		root.SetOut(out)
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err != nil {
			t.Fatalf("artifact list --media-type text/markdown --json should succeed, got: %v", err)
		}

		var entries []struct {
			ID        string `json:"id"`
			MediaType string `json:"media_type"`
		}
		if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
			t.Fatalf("expected JSON array, got %q: %v", out.String(), err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 text/markdown entries, got %d: %v", len(entries), entries)
		}
		for _, e := range entries {
			if e.MediaType != "text/markdown" {
				t.Errorf("expected text/markdown, got %q for %s", e.MediaType, e.ID)
			}
		}
	})

	t.Run("filter image/png returns only image doc", func(t *testing.T) {
		root := NewCommandFactory(dir).NewRootCommand()
		root.SetArgs([]string{"artifact", "list", "--media-type", "image/png", "--json"})

		out := &bytes.Buffer{}
		root.SetOut(out)
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err != nil {
			t.Fatalf("artifact list --media-type image/png --json should succeed, got: %v", err)
		}

		var entries []struct {
			ID        string `json:"id"`
			MediaType string `json:"media_type"`
		}
		if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
			t.Fatalf("expected JSON array, got %q: %v", out.String(), err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 image/png entry, got %d", len(entries))
		}
		if entries[0].ID != "DIAG-001" {
			t.Errorf("expected DIAG-001, got %q", entries[0].ID)
		}
	})

	t.Run("no filter returns all", func(t *testing.T) {
		root := NewCommandFactory(dir).NewRootCommand()
		root.SetArgs([]string{"artifact", "list", "--json"})

		out := &bytes.Buffer{}
		root.SetOut(out)
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err != nil {
			t.Fatalf("artifact list --json should succeed, got: %v", err)
		}

		var entries []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
			t.Fatalf("expected JSON array, got %q: %v", out.String(), err)
		}
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
	})
}
