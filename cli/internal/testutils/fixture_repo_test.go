package testutils

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewFixtureRepo_StandardProfile is a smoke test for NewFixtureRepo:
// the standard profile must produce a project with a config and 5 seeded beads.
func TestNewFixtureRepo_StandardProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short: builds ddx binary")
	}
	dest := NewFixtureRepo(t, "standard")

	if _, err := os.Stat(filepath.Join(dest, ".ddx", "config.yaml")); err != nil {
		t.Fatalf("expected .ddx/config.yaml: %v", err)
	}
	beads, err := os.ReadFile(filepath.Join(dest, ".ddx", "beads.jsonl"))
	if err != nil {
		t.Fatalf("read beads.jsonl: %v", err)
	}
	if n := countLines(beads); n != 5 {
		t.Fatalf("standard profile: want 5 beads, got %d", n)
	}
}

// TestNewFixtureRepo_MultiProject verifies multi-project layout: the helper
// returns the parent dir, with proj-a (seeded) and proj-b (empty) underneath.
func TestNewFixtureRepo_MultiProject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short: builds ddx binary")
	}
	dest := NewFixtureRepo(t, "multi-project")
	for _, sub := range []string{"proj-a", "proj-b"} {
		if _, err := os.Stat(filepath.Join(dest, sub, ".ddx", "config.yaml")); err != nil {
			t.Fatalf("multi-project: missing %s/.ddx/config.yaml: %v", sub, err)
		}
	}
}

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}
