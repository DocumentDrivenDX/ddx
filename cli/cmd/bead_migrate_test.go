package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateCommand(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)

	// Seed two closed beads (one with inline events, one without) plus an
	// open bead, then run `ddx bead migrate`.
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))
	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-c1","title":"closed with events","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"deadbeef","events":[{"kind":"review","summary":"APPROVE","body":"ok","created_at":"` + old + `"}]}`,
		`{"id":"ddx-c2","title":"closed no events","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"deadbeef"}`,
		`{"id":"ddx-open","title":"open","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ".ddx", "beads.jsonl"), []byte(rows), 0o644))

	beforeStatus, err := executeCommand(factory.NewRootCommand(), "bead", "status", "--json")
	require.NoError(t, err)

	out, err := executeCommand(factory.NewRootCommand(), "bead", "migrate", "--json")
	require.NoError(t, err)
	var stats struct {
		EventsExternalized int `json:"EventsExternalized"`
		Archived           int `json:"Archived"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &stats))
	assert.Equal(t, 1, stats.EventsExternalized)
	assert.Equal(t, 2, stats.Archived)

	// AC2: archive + attachments populated.
	assert.FileExists(t, filepath.Join(workingDir, ".ddx", "beads-archive.jsonl"))
	assert.FileExists(t, filepath.Join(workingDir, ".ddx", "attachments", "ddx-c1", "events.jsonl"))

	// AC3: status totals identical pre/post.
	afterStatus, err := executeCommand(factory.NewRootCommand(), "bead", "status", "--json")
	require.NoError(t, err)
	assert.Equal(t, beforeStatus, afterStatus)

	// AC4: show works for archived bead.
	showOut, err := executeCommand(factory.NewRootCommand(), "bead", "show", "ddx-c1", "--json")
	require.NoError(t, err)
	var shown map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &shown))
	assert.Equal(t, "ddx-c1", shown["id"])

	// AC4: list (default) still includes the archived bead.
	listOut, err := executeCommand(factory.NewRootCommand(), "bead", "list", "--json")
	require.NoError(t, err)
	var listed []map[string]any
	require.NoError(t, json.Unmarshal([]byte(listOut), &listed))
	ids := map[string]bool{}
	for _, b := range listed {
		ids[b["id"].(string)] = true
	}
	assert.True(t, ids["ddx-c1"], "archived bead should appear in default list")
	assert.True(t, ids["ddx-c2"], "archived bead should appear in default list")
	assert.True(t, ids["ddx-open"], "open bead should still appear")

	// AC6: idempotency — second pass no-ops.
	out2, err := executeCommand(factory.NewRootCommand(), "bead", "migrate", "--json")
	require.NoError(t, err)
	var stats2 struct {
		EventsExternalized int `json:"EventsExternalized"`
		Archived           int `json:"Archived"`
	}
	require.NoError(t, json.Unmarshal([]byte(out2), &stats2))
	assert.Equal(t, 0, stats2.EventsExternalized)
	assert.Equal(t, 0, stats2.Archived)
}

// TestMigrateCommandToAxon covers AC §1 (the --to axon flag exists and is
// documented), AC §3 (idempotent across runs), and the source-files-intact
// contract from the bead description.
func TestMigrateCommandToAxon(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)

	helpOut, err := executeCommand(factory.NewRootCommand(), "bead", "migrate", "--help")
	require.NoError(t, err)
	assert.Contains(t, helpOut, "--to", "--help must advertise the --to flag")
	assert.Contains(t, helpOut, "axon", "--help must advertise the axon target")

	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))
	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	activeRows := strings.Join([]string{
		`{"id":"ddx-mta-1","title":"open","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
		`{"id":"ddx-mta-2","title":"in_progress","status":"in_progress","priority":3,"issue_type":"task","owner":"alice","created_at":"` + old + `","updated_at":"` + old + `","events":[{"kind":"claimed","summary":"alice claimed","actor":"alice","created_at":"` + old + `"}]}`,
	}, "\n") + "\n"
	archiveRows := strings.Join([]string{
		`{"id":"ddx-mta-3","title":"closed and archived","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"abc1230000","archived_at":"` + old + `"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ".ddx", "beads.jsonl"), []byte(activeRows), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ".ddx", "beads-archive.jsonl"), []byte(archiveRows), 0o644))

	out, err := executeCommand(factory.NewRootCommand(), "bead", "migrate", "--to", "axon", "--json")
	require.NoError(t, err)
	var view struct {
		BeadsMigrated  int    `json:"BeadsMigrated"`
		EventsMigrated int    `json:"EventsMigrated"`
		To             string `json:"To"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &view))
	assert.Equal(t, 3, view.BeadsMigrated)
	assert.Equal(t, 1, view.EventsMigrated)
	assert.Equal(t, "axon", view.To)

	// Source files left intact (operator removes after verification).
	assert.FileExists(t, filepath.Join(workingDir, ".ddx", "beads.jsonl"))
	assert.FileExists(t, filepath.Join(workingDir, ".ddx", "beads-archive.jsonl"))

	// Axon collection files written.
	assert.FileExists(t, filepath.Join(workingDir, ".ddx", "axon", "ddx_beads.jsonl"))
	assert.FileExists(t, filepath.Join(workingDir, ".ddx", "axon", "ddx_bead_events.jsonl"))

	// AC §3: a second invocation reports the same counts and produces
	// byte-identical files (no duplicates).
	beadsBefore, err := os.ReadFile(filepath.Join(workingDir, ".ddx", "axon", "ddx_beads.jsonl"))
	require.NoError(t, err)
	eventsBefore, err := os.ReadFile(filepath.Join(workingDir, ".ddx", "axon", "ddx_bead_events.jsonl"))
	require.NoError(t, err)

	out2, err := executeCommand(factory.NewRootCommand(), "bead", "migrate", "--to", "axon", "--json")
	require.NoError(t, err)
	var view2 struct {
		BeadsMigrated  int `json:"BeadsMigrated"`
		EventsMigrated int `json:"EventsMigrated"`
	}
	require.NoError(t, json.Unmarshal([]byte(out2), &view2))
	assert.Equal(t, 3, view2.BeadsMigrated)
	assert.Equal(t, 1, view2.EventsMigrated)

	beadsAfter, err := os.ReadFile(filepath.Join(workingDir, ".ddx", "axon", "ddx_beads.jsonl"))
	require.NoError(t, err)
	eventsAfter, err := os.ReadFile(filepath.Join(workingDir, ".ddx", "axon", "ddx_bead_events.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, string(beadsBefore), string(beadsAfter))
	assert.Equal(t, string(eventsBefore), string(eventsAfter))
}

func TestMigrateCommandDryRun(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)

	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))
	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-c1","title":"closed with events","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"deadbeef","events":[{"kind":"review","summary":"APPROVE","body":"ok","created_at":"` + old + `"}]}`,
		`{"id":"ddx-c2","title":"closed no events","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `","closing_commit_sha":"deadbeef"}`,
		`{"id":"ddx-open","title":"open","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
	}, "\n") + "\n"
	beadsPath := filepath.Join(workingDir, ".ddx", "beads.jsonl")
	require.NoError(t, os.WriteFile(beadsPath, []byte(rows), 0o644))
	before, err := os.ReadFile(beadsPath)
	require.NoError(t, err)

	out, err := executeCommand(factory.NewRootCommand(), "bead", "migrate", "--dry-run", "--json")
	require.NoError(t, err)
	var stats struct {
		EventsExternalized int  `json:"EventsExternalized"`
		Archived           int  `json:"Archived"`
		DryRun             bool `json:"DryRun"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &stats))
	assert.Equal(t, 1, stats.EventsExternalized)
	assert.Equal(t, 2, stats.Archived)
	assert.True(t, stats.DryRun)

	// Active file must not change, and archive must not be created.
	after, err := os.ReadFile(beadsPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	_, statErr := os.Stat(filepath.Join(workingDir, ".ddx", "beads-archive.jsonl"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not create archive file")
	_, statErr = os.Stat(filepath.Join(workingDir, ".ddx", "attachments", "ddx-c1", "events.jsonl"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not create attachments")
}
