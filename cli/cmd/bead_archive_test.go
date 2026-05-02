package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// archiveCmdStats mirrors the JSON shape emitted by `ddx bead archive --json`.
type archiveCmdStats struct {
	EventsExternalized int   `json:"EventsExternalized"`
	Archived           int   `json:"Archived"`
	ActiveSizeBefore   int64 `json:"ActiveSizeBefore"`
	ActiveSizeAfter    int64 `json:"ActiveSizeAfter"`
	Threshold          int64 `json:"Threshold"`
	Skipped            bool  `json:"Skipped"`
}

// TestBeadArchiveBelowThresholdIsNoop verifies AC2: a small active beads.jsonl
// stays untouched under the default size trigger.
func TestBeadArchiveBelowThresholdIsNoop(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)

	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))
	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-c1","title":"closed","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
		`{"id":"ddx-open","title":"open","status":"open","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ".ddx", "beads.jsonl"), []byte(rows), 0o644))

	out, err := executeCommand(factory.NewRootCommand(), "bead", "archive", "--json")
	require.NoError(t, err)
	var stats archiveCmdStats
	require.NoError(t, json.Unmarshal([]byte(out), &stats))
	assert.True(t, stats.Skipped, "small active file must not trigger archival")
	assert.Equal(t, 0, stats.Archived)
	assert.NoFileExists(t, filepath.Join(workingDir, ".ddx", "beads-archive.jsonl"))
}

// TestBeadArchiveSizeTrigger covers AC7: synthetic 5MB beads.jsonl shrinks
// under the 4MB threshold post-archive.
func TestBeadArchiveSizeTrigger(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)

	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))

	// Build a synthetic active beads.jsonl just over 5MB. Each closed bead
	// carries ~4KB of inline padding so the file passes the 4MB default
	// threshold and a substantial number of rows is eligible for archive.
	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	padding := strings.Repeat("x", 4000)
	beadsPath := filepath.Join(workingDir, ".ddx", "beads.jsonl")
	f, err := os.Create(beadsPath)
	require.NoError(t, err)

	const closedCount = 1400
	for i := 0; i < closedCount; i++ {
		row := fmt.Sprintf(
			`{"id":"ddx-c%04d","title":"closed %d","status":"closed","priority":2,"issue_type":"task","created_at":"%s","updated_at":"%s","description":"%s"}`+"\n",
			i, i, old, old, padding,
		)
		_, err := f.WriteString(row)
		require.NoError(t, err)
	}
	// One open bead so the active collection isn't empty post-archive.
	openRow := fmt.Sprintf(
		`{"id":"ddx-open","title":"open","status":"open","priority":2,"issue_type":"task","created_at":"%s","updated_at":"%s"}`+"\n",
		old, old,
	)
	_, err = f.WriteString(openRow)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	info, err := os.Stat(beadsPath)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(5*1024*1024), "synthetic beads.jsonl must exceed 5MB to exercise the trigger")

	out, err := executeCommand(factory.NewRootCommand(), "bead", "archive", "--json")
	require.NoError(t, err)
	var stats archiveCmdStats
	require.NoError(t, json.Unmarshal([]byte(out), &stats))
	assert.False(t, stats.Skipped)
	assert.Equal(t, closedCount, stats.Archived, "all closed beads should move to the archive")
	assert.Greater(t, stats.ActiveSizeBefore, int64(5*1024*1024))
	assert.Less(t, stats.ActiveSizeAfter, int64(4*1024*1024), "active file must shrink under 4MB threshold")

	// AC4: archive grew, active shrank.
	archiveInfo, err := os.Stat(filepath.Join(workingDir, ".ddx", "beads-archive.jsonl"))
	require.NoError(t, err)
	assert.Greater(t, archiveInfo.Size(), int64(0))

	activeInfo, err := os.Stat(beadsPath)
	require.NoError(t, err)
	assert.Less(t, activeInfo.Size(), stats.ActiveSizeBefore)

	// AC6: ddx bead show works for an archived bead via read-through.
	showOut, err := executeCommand(factory.NewRootCommand(), "bead", "show", "ddx-c0000", "--json")
	require.NoError(t, err)
	var shown map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &shown))
	assert.Equal(t, "ddx-c0000", shown["id"])
}

// TestBeadArchiveFlagOverrides covers AC3: --max-size, --older-than, --max-count
// override the trigger.
func TestBeadArchiveFlagOverrides(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))

	old := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	rows := strings.Join([]string{
		`{"id":"ddx-old","title":"old","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
		`{"id":"ddx-recent","title":"recent","status":"closed","priority":2,"issue_type":"task","created_at":"` + recent + `","updated_at":"` + recent + `"}`,
		`{"id":"ddx-other","title":"other","status":"closed","priority":2,"issue_type":"task","created_at":"` + old + `","updated_at":"` + old + `"}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ".ddx", "beads.jsonl"), []byte(rows), 0o644))

	// --max-size=0 disables the size gate; --older-than 720h skips the
	// recently-closed bead; --max-count 1 caps the run at one bead.
	out, err := executeCommand(factory.NewRootCommand(),
		"bead", "archive",
		"--max-size", "0",
		"--older-than", "720h",
		"--max-count", "1",
		"--json",
	)
	require.NoError(t, err)
	var stats archiveCmdStats
	require.NoError(t, json.Unmarshal([]byte(out), &stats))
	assert.False(t, stats.Skipped)
	assert.Equal(t, 1, stats.Archived, "--max-count must cap the run")
}

// TestBeadArchiveHelp covers AC1: command exists with help text.
func TestBeadArchiveHelp(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)

	out, err := executeCommand(factory.NewRootCommand(), "bead", "archive", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "archive")
	assert.Contains(t, out, "--max-size")
	assert.Contains(t, out, "--older-than")
	assert.Contains(t, out, "--max-count")
}
