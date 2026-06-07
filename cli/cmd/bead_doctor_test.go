package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadDoctorDetectsParentAncestorBackEdge(t *testing.T) {
	workDir := t.TempDir()
	ddxDir := filepath.Join(workDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	path := filepath.Join(ddxDir, "beads.jsonl")

	fixedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	tstamp := fixedAt.Format(time.RFC3339)
	lines := []map[string]any{
		{
			"id":             "ddx-root",
			"title":          "root",
			"issue_type":     "task",
			"status":         "open",
			"priority":       2,
			"schema_version": 1,
			"created_at":     tstamp,
			"updated_at":     tstamp,
		},
		{
			"id":             "ddx-parent",
			"title":          "parent",
			"issue_type":     "task",
			"status":         "open",
			"priority":       2,
			"schema_version": 1,
			"parent":         "ddx-root",
			"created_at":     tstamp,
			"updated_at":     tstamp,
		},
		{
			"id":             "ddx-independent",
			"title":          "independent",
			"issue_type":     "task",
			"status":         "open",
			"priority":       2,
			"schema_version": 1,
			"created_at":     tstamp,
			"updated_at":     tstamp,
		},
		{
			"id":             "ddx-child",
			"title":          "child",
			"issue_type":     "task",
			"status":         "open",
			"priority":       2,
			"schema_version": 1,
			"parent":         "ddx-parent",
			"dependencies": []any{
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-parent", "type": "blocks"},
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-root", "type": "blocks"},
				map[string]any{"issue_id": "ddx-child", "depends_on_id": "ddx-independent", "type": "blocks"},
			},
			"created_at": tstamp,
			"updated_at": tstamp,
		},
	}
	var file bytes.Buffer
	for _, line := range lines {
		encoded, err := json.Marshal(line)
		require.NoError(t, err)
		file.Write(encoded)
		file.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(path, file.Bytes(), 0o644))

	factory := NewCommandFactory(workDir)

	type finding struct {
		Kind         string   `json:"kind"`
		BeadID       string   `json:"bead_id"`
		FieldPath    string   `json:"field_path"`
		TargetID     string   `json:"target_id"`
		DependencyID string   `json:"dependency_id"`
		AncestorID   string   `json:"ancestor_id"`
		ParentChain  []string `json:"parent_chain"`
	}
	type report struct {
		Path     string    `json:"path"`
		Fixed    bool      `json:"fixed"`
		Clean    bool      `json:"clean"`
		Findings []finding `json:"findings"`
	}

	out, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "bead", "doctor", "--json")
	require.NoError(t, err)
	var first report
	require.NoError(t, json.Unmarshal([]byte(out), &first))
	assert.False(t, first.Clean)
	assert.False(t, first.Fixed)
	require.Len(t, first.Findings, 2)
	assert.Equal(t, "parent_ancestor_in_deps", first.Findings[0].Kind)
	assert.Equal(t, "ddx-child", first.Findings[0].BeadID)
	assert.Equal(t, "ddx-parent", first.Findings[0].TargetID)
	assert.Equal(t, "ddx-parent", first.Findings[0].DependencyID)
	assert.Equal(t, "ddx-parent", first.Findings[0].AncestorID)
	assert.Equal(t, []string{"ddx-parent", "ddx-root"}, first.Findings[0].ParentChain)
	assert.Equal(t, "ddx-root", first.Findings[1].TargetID)
	assert.Equal(t, "ddx-root", first.Findings[1].DependencyID)
	assert.Equal(t, "ddx-root", first.Findings[1].AncestorID)
	assert.Equal(t, []string{"ddx-parent", "ddx-root"}, first.Findings[1].ParentChain)

	fixOut, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "bead", "doctor", "--fix", "--json")
	require.NoError(t, err)
	var fixed report
	require.NoError(t, json.Unmarshal([]byte(fixOut), &fixed))
	assert.False(t, fixed.Clean)
	assert.True(t, fixed.Fixed)
	require.Len(t, fixed.Findings, 2)

	postOut, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "bead", "doctor", "--json")
	require.NoError(t, err)
	var post report
	require.NoError(t, json.Unmarshal([]byte(postOut), &post))
	assert.True(t, post.Clean)
	assert.Empty(t, post.Findings)
}
