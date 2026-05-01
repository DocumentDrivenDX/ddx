package cmd

// Integration test for `ddx server state prune` (ddx-15f7ee0b Fix C).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestServerStatePruneDropsPhantomsKeepsRealProjects seeds a state file with
// 10 real project entries (non-/tmp paths) and 100 phantom test-dir entries,
// runs `ddx server state prune`, and asserts:
//   - 10 entries remain in the state file (all real)
//   - A backup file exists containing the original 110 entries
//   - Exit code is 0 and summary is printed
func TestServerStatePruneDropsPhantomsKeepsRealProjects(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "server-state.json")

	now := time.Now().UTC().Format(time.RFC3339)
	projects := make([]map[string]any, 0, 110)
	for i := 0; i < 10; i++ {
		projects = append(projects, map[string]any{
			"id":            "proj-real-" + strconv.Itoa(i),
			"name":          "real",
			"path":          "/home/user/projects/real-" + strconv.Itoa(i),
			"registered_at": now,
			"last_seen":     now,
		})
	}
	for i := 0; i < 100; i++ {
		projects = append(projects, map[string]any{
			"id":            "proj-fake-" + strconv.Itoa(i),
			"name":          "001",
			"path":          "/tmp/TestFooBar" + strconv.Itoa(i) + "/001",
			"registered_at": now,
			"last_seen":     now,
		})
	}
	state := map[string]any{
		"schema_version": "1",
		"node": map[string]any{
			"name": "test-node", "id": "node-test",
			"started_at": now, "last_seen": now,
		},
		"projects": projects,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Build and execute the CLI command.
	f := NewCommandFactory(tmp)
	root := f.newServerStateCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"prune", "--state", statePath})
	if err := root.Execute(); err != nil {
		t.Fatalf("prune returned error: %v; output: %s", err, out.String())
	}

	summary := out.String()
	if !strings.Contains(summary, "Pruned 100 of 110 entries (10 kept)") {
		t.Errorf("unexpected summary: %q", summary)
	}
	if !strings.Contains(summary, "Backup written to") {
		t.Errorf("expected backup file path in summary, got %q", summary)
	}

	// Verify the state file now has 10 entries.
	final, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var finalState struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(final, &finalState); err != nil {
		t.Fatal(err)
	}
	if len(finalState.Projects) != 10 {
		t.Errorf("expected 10 projects after prune, got %d", len(finalState.Projects))
	}
	for _, p := range finalState.Projects {
		path, _ := p["path"].(string)
		if strings.HasPrefix(path, "/tmp/") {
			t.Errorf("unexpected /tmp/ path survived prune: %s", path)
		}
	}

	// Verify a backup file exists with all 110 entries.
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	backupFound := false
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "server-state.json.bak-") {
			continue
		}
		backupFound = true
		backupData, err := os.ReadFile(filepath.Join(tmp, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var backup struct {
			Projects []map[string]any `json:"projects"`
		}
		if err := json.Unmarshal(backupData, &backup); err != nil {
			t.Fatal(err)
		}
		if len(backup.Projects) != 110 {
			t.Errorf("expected backup to contain 110 entries, got %d", len(backup.Projects))
		}
	}
	if !backupFound {
		t.Error("expected backup file (*.bak-*) to exist in state dir")
	}
}

// TestServerStatePruneDryRun verifies --dry-run prints the summary without
// writing any files.
func TestServerStatePruneDryRun(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "server-state.json")

	now := time.Now().UTC().Format(time.RFC3339)
	state := map[string]any{
		"schema_version": "1",
		"node": map[string]any{
			"name": "test-node", "id": "node-test",
			"started_at": now, "last_seen": now,
		},
		"projects": []map[string]any{
			{"id": "p-real", "path": "/home/user/real", "registered_at": now, "last_seen": now},
			{"id": "p-fake", "path": "/tmp/TestFoo123/001", "registered_at": now, "last_seen": now},
		},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Snapshot the mtime to verify the file was not rewritten.
	before, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}

	f := NewCommandFactory(tmp)
	root := f.newServerStateCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"prune", "--dry-run", "--state", statePath})
	if err := root.Execute(); err != nil {
		t.Fatalf("dry-run prune returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "DRY-RUN: Pruned 1 of 2 entries (1 kept)") {
		t.Errorf("unexpected summary: %q", got)
	}
	if strings.Contains(got, "Backup written to") {
		t.Errorf("dry-run must not write a backup; got %q", got)
	}

	// File should be byte-identical (no backup, no rewrite).
	after, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("state file was modified during dry-run")
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "server-state.json.bak-") {
			t.Errorf("dry-run must not leave a backup; found %s", e.Name())
		}
	}
}
