package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadDoctorDetectsAndRepairsBackEdge(t *testing.T) {
	workingDir := t.TempDir()
	factory := newBeadTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	ddxDir := filepath.Join(workingDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	path := filepath.Join(ddxDir, "beads.jsonl")

	rows := []map[string]any{
		{
			"id":         "ddx-root",
			"title":      "root",
			"issue_type": "task",
			"status":     "open",
			"priority":   2,
			"labels":     []string{},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
		{
			"id":         "ddx-parent",
			"title":      "parent",
			"issue_type": "task",
			"status":     "open",
			"priority":   2,
			"parent":     "ddx-root",
			"labels":     []string{},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
		{
			"id":         "ddx-helper",
			"title":      "helper",
			"issue_type": "task",
			"status":     "open",
			"priority":   2,
			"labels":     []string{},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
		{
			"id":         "ddx-child",
			"title":      "child",
			"issue_type": "task",
			"status":     "open",
			"priority":   2,
			"parent":     "ddx-parent",
			"labels":     []string{},
			"dependencies": []any{
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-parent", "type": "blocks"},
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-root", "type": "blocks"},
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-helper", "type": "blocks"},
			},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		},
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		encoded, err := json.Marshal(row)
		require.NoError(t, err)
		lines = append(lines, string(encoded))
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	out, err := executeCommand(rootCmd, "bead", "doctor", "--json")
	require.NoError(t, err, "JSON scan mode must return findings without failing")

	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Equal(t, false, report["clean"])
	findings, ok := report["findings"].([]any)
	require.True(t, ok)
	require.Len(t, findings, 2)
	first := findings[0].(map[string]any)
	second := findings[1].(map[string]any)
	assert.Equal(t, "parent_ancestor_in_deps", first["code"])
	assert.Equal(t, "dependencies[0].depends_on_id", first["field_path"])
	assert.Equal(t, "parent_ancestor_in_deps", second["code"])
	assert.Equal(t, "dependencies[1].depends_on_id", second["field_path"])

	fixOut, fixErr := executeCommand(rootCmd, "bead", "doctor", "--fix", "--json")
	require.NoError(t, fixErr)

	var fixReport map[string]any
	require.NoError(t, json.Unmarshal([]byte(fixOut), &fixReport))
	assert.Equal(t, true, fixReport["fixed"])
	assert.Equal(t, false, fixReport["clean"])

	repaired, err := os.ReadFile(path)
	require.NoError(t, err)
	var childRow map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(repaired)), "\n") {
		var row map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &row))
		if row["id"] == "ddx-child" {
			childRow = row
			break
		}
	}
	require.NotNil(t, childRow)
	deps, ok := childRow["dependencies"].([]any)
	require.True(t, ok)
	require.Len(t, deps, 1)
	dep := deps[0].(map[string]any)
	assert.Equal(t, "ddx-helper", dep["depends_on_id"])
	assert.Equal(t, "blocks", dep["type"])

	postOut, postErr := executeCommand(rootCmd, "bead", "doctor", "--json")
	require.NoError(t, postErr, "after repair the scan should be clean")
	assert.Contains(t, postOut, `"clean": true`)
	assert.NotContains(t, postOut, "parent_ancestor_in_deps")
}
