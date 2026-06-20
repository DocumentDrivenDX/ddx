package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDocListByType(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/adr-001.md", "---\nddx:\n  id: ADR-001\n  title: Use Go for CLI\n---\n# ADR-001\n")
	writeTestFile(t, dir, "docs/adr-002.md", "---\nddx:\n  id: ADR-002\n  title: Use Cobra\n---\n# ADR-002\n")
	writeTestFile(t, dir, "docs/feat-001.md", "---\nddx:\n  id: FEAT-001\n  title: Some Feature\n---\n# FEAT-001\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "list", "--type", "ADR", "--json"})

	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err != nil {
		t.Fatalf("doc list --type ADR --json should succeed, got: %v", err)
	}

	var entries []struct {
		ID        string `json:"id"`
		Path      string `json:"path"`
		Title     string `json:"title"`
		MediaType string `json:"media_type"`
	}
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", out.String(), err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 ADR entries, got %d: %v", len(entries), entries)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.ID, "ADR-") {
			t.Errorf("expected ADR- prefix, got %q", e.ID)
		}
		if e.MediaType != "text/markdown" {
			t.Errorf("expected text/markdown media_type, got %q", e.MediaType)
		}
	}
}

func TestDocListNoTypeReturnsAll(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/adr-001.md", "---\nddx:\n  id: ADR-001\n---\n# ADR-001\n")
	writeTestFile(t, dir, "docs/feat-001.md", "---\nddx:\n  id: FEAT-001\n---\n# FEAT-001\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "list", "--json"})

	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err != nil {
		t.Fatalf("doc list --json should succeed, got: %v", err)
	}

	var entries []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", out.String(), err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestAdrListLegacyResolvesToDocListTypeADR(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/adr-001.md", "---\nddx:\n  id: ADR-001\n  title: Use Go\n---\n# ADR-001\n")
	writeTestFile(t, dir, "docs/feat-001.md", "---\nddx:\n  id: FEAT-001\n---\n# FEAT-001\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"adr", "list", "--json"})

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("adr list --json should succeed, got: %v", err)
	}

	if !strings.Contains(errOut.String(), "deprecated") {
		t.Errorf("expected deprecation warning on stderr, got: %q", errOut.String())
	}

	var entries []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", out.String(), err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 ADR entry, got %d", len(entries))
	}
	if entries[0].ID != "ADR-001" {
		t.Errorf("expected ADR-001, got %q", entries[0].ID)
	}
}
