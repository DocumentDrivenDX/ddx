package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

// TestBeadDoctorDetectsAndRepairsBackEdge covers the CLI wiring for the bead
// doctor parent-ancestor dependency scan. The command must surface the stable
// finding code in JSON mode and `--fix` must remove only the offending edges.
func TestBeadDoctorDetectsAndRepairsBackEdge(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	ddxDir := filepath.Join(workingDir, ddxroot.DirName)
	path := filepath.Join(ddxDir, "beads.jsonl")

	corpus := []map[string]any{
		{
			"id":         "ddx-root",
			"title":      "root",
			"type":       "task",
			"status":     "open",
			"priority":   2,
			"labels":     []string{},
			"deps":       []string{},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
		{
			"id":         "ddx-parent",
			"title":      "parent",
			"type":       "task",
			"status":     "open",
			"priority":   2,
			"parent":     "ddx-root",
			"labels":     []string{},
			"deps":       []string{},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
		{
			"id":       "ddx-child",
			"title":    "child",
			"type":     "task",
			"status":   "open",
			"priority": 2,
			"parent":   "ddx-parent",
			"labels":   []string{},
			"dependencies": []any{
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-parent", "type": "blocks"},
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-root", "type": "blocks"},
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-allowed", "type": "blocks"},
			},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
		{
			"id":         "ddx-allowed",
			"title":      "allowed",
			"type":       "task",
			"status":     "open",
			"priority":   2,
			"labels":     []string{},
			"deps":       []string{},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
	}

	var lines [][]byte
	for _, row := range corpus {
		encoded, err := json.Marshal(row)
		require.NoError(t, err)
		lines = append(lines, encoded)
	}
	require.NoError(t, os.WriteFile(path, append(bytes.Join(lines, []byte{'\n'}), '\n'), 0o644))

	jsonOut, err := executeCommand(factory.NewRootCommand(), "bead", "doctor", "--json")
	require.NoError(t, err, "JSON scan mode should serialize findings without failing the command")

	var before struct {
		Clean    bool `json:"clean"`
		Findings []struct {
			Code      string `json:"code"`
			BeadID    string `json:"bead_id"`
			FieldPath string `json:"field_path"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &before))
	require.False(t, before.Clean)
	require.Len(t, before.Findings, 2)
	for _, finding := range before.Findings {
		require.Equal(t, "parent_ancestor_in_deps", finding.Code)
		require.Equal(t, "ddx-child", finding.BeadID)
	}

	_, err = executeCommand(factory.NewRootCommand(), "bead", "doctor", "--fix")
	require.NoError(t, err, "repair mode must succeed after removing the offending edges")

	afterOut, err := executeCommand(factory.NewRootCommand(), "bead", "doctor", "--json")
	require.NoError(t, err, "after repair the scan must be clean")

	var after struct {
		Clean    bool  `json:"clean"`
		Findings []any `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(afterOut), &after))
	require.True(t, after.Clean)
	require.Empty(t, after.Findings)

	repaired, err := os.ReadFile(path)
	require.NoError(t, err)
	repairedLines := strings.Split(strings.TrimSpace(string(repaired)), "\n")
	require.Len(t, repairedLines, 4)
	var child map[string]any
	require.NoError(t, json.Unmarshal([]byte(repairedLines[2]), &child))
	deps, ok := child["dependencies"].([]any)
	require.True(t, ok)
	require.Len(t, deps, 1)
	dep := deps[0].(map[string]any)
	require.Equal(t, "ddx-allowed", dep["depends_on_id"])
	require.True(t, strings.Contains(repairedLines[2], "\"kind\":\"repair\""))
}
