package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadMetrics(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	execRoot := filepath.Join(dir, ".ddx", "executions")

	// ddx-1: 2 attempts (one succeeded, one failed). Total tokens 3000, cost 1.50.
	writeExecResult(t, execRoot, "20260401T100000-aaaa0001", map[string]any{
		"bead_id":     "ddx-1",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_succeeded",
		"duration_ms": 100000,
		"tokens":      1000,
		"cost_usd":    0.5,
	})
	writeExecResult(t, execRoot, "20260401T110000-aaaa0002", map[string]any{
		"bead_id":     "ddx-1",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_failed",
		"duration_ms": 200000,
		"tokens":      2000,
		"cost_usd":    1.0,
	})
	// ddx-2: 1 attempt, succeeded. 500 tokens, 0.25 cost.
	writeExecResult(t, execRoot, "20260401T120000-aaaa0003", map[string]any{
		"bead_id":     "ddx-2",
		"harness":     "claude",
		"model":       "opus",
		"outcome":     "task_succeeded",
		"duration_ms": 300000,
		"tokens":      500,
		"cost_usd":    0.25,
	})
	// Missing bead_id — should be skipped.
	writeExecResult(t, execRoot, "20260401T130000-aaaa0004", map[string]any{
		"harness":     "agent",
		"outcome":     "error",
		"duration_ms": 1000,
	})
	// Malformed result.json — should be skipped.
	badDir := filepath.Join(execRoot, "20260401T140000-aaaa0005")
	require.NoError(t, os.MkdirAll(badDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(badDir, "result.json"), []byte("not json"), 0o644))

	factory := NewCommandFactory(dir)
	rootCmd := factory.NewRootCommand()

	// Seed bead tracker so metrics can render titles.
	_, err := executeCommand(rootCmd, "bead", "create", "Fix auth bug", "--priority", "1")
	require.NoError(t, err)
	// Override ID via update by recreating — simpler: find generated ID and ignore.
	// For this test we create beads with explicit IDs via direct store write.
	// Use bead create + update to set custom IDs is not supported, so write beads.jsonl directly.

	// Direct seed of beads.jsonl with the IDs used above.
	beadsPath := filepath.Join(dir, ".ddx", "beads.jsonl")
	require.NoError(t, os.WriteFile(beadsPath, []byte(
		`{"id":"ddx-1","title":"First bead","status":"closed","priority":2,"issue_type":"task","created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z"}`+"\n"+
			`{"id":"ddx-2","title":"Second bead","status":"closed","priority":2,"issue_type":"task","created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z"}`+"\n"),
		0o644))

	// --json output
	jsonCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(jsonCmd, "bead", "metrics", "--json")
	require.NoError(t, err)

	var rows []beadMetricsRow
	require.NoError(t, json.Unmarshal([]byte(output), &rows))
	require.Len(t, rows, 2)

	byID := map[string]beadMetricsRow{}
	for _, r := range rows {
		byID[r.BeadID] = r
	}

	require.Contains(t, byID, "ddx-1")
	assert.Equal(t, 2, byID["ddx-1"].AttemptCount)
	assert.Equal(t, 3000, byID["ddx-1"].TotalTokens)
	assert.InDelta(t, 1.5, byID["ddx-1"].TotalCostUSD, 0.0001)
	assert.InDelta(t, 150000.0, byID["ddx-1"].AvgDurationMS, 0.0001)
	assert.Equal(t, "First bead", byID["ddx-1"].Title)

	require.Contains(t, byID, "ddx-2")
	assert.Equal(t, 1, byID["ddx-2"].AttemptCount)
	assert.Equal(t, 500, byID["ddx-2"].TotalTokens)
	assert.InDelta(t, 0.25, byID["ddx-2"].TotalCostUSD, 0.0001)
	assert.Equal(t, "Second bead", byID["ddx-2"].Title)

	// Table output shape: has header columns.
	tableCmd := NewCommandFactory(dir).NewRootCommand()
	tableOut, err := executeCommand(tableCmd, "bead", "metrics")
	require.NoError(t, err)
	header := strings.SplitN(tableOut, "\n", 2)[0]
	for _, col := range []string{"BEAD_ID", "ATTEMPTS", "TOTAL_TOKENS", "TOTAL_COST_USD", "AVG_DURATION_MS", "TITLE"} {
		assert.Contains(t, header, col)
	}

	// Empty executions dir returns an empty list, not an error.
	empty := t.TempDir()
	emptyCmd := NewCommandFactory(empty).NewRootCommand()
	emptyOut, err := executeCommand(emptyCmd, "bead", "metrics", "--json")
	require.NoError(t, err)
	var emptyRows []beadMetricsRow
	require.NoError(t, json.Unmarshal([]byte(emptyOut), &emptyRows))
	assert.Empty(t, emptyRows)
}

func TestBeadMetricsShowJSONIncludesMetrics(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	execRoot := filepath.Join(dir, ".ddx", "executions")

	writeExecResult(t, execRoot, "20260401T100000-aaaa0001", map[string]any{
		"bead_id":     "ddx-1",
		"harness":     "claude",
		"outcome":     "task_succeeded",
		"duration_ms": 100000,
		"tokens":      1000,
		"cost_usd":    0.5,
	})
	writeExecResult(t, execRoot, "20260401T110000-aaaa0002", map[string]any{
		"bead_id":     "ddx-1",
		"harness":     "claude",
		"outcome":     "task_failed",
		"duration_ms": 200000,
		"tokens":      2000,
		"cost_usd":    1.0,
	})

	beadsPath := filepath.Join(dir, ".ddx", "beads.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(beadsPath), 0o755))
	require.NoError(t, os.WriteFile(beadsPath, []byte(
		`{"id":"ddx-1","title":"First bead","status":"closed","priority":2,"issue_type":"task","created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z"}`+"\n"),
		0o644))

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "bead", "show", "ddx-1", "--json")
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &obj))

	metrics, ok := obj["metrics"].(map[string]any)
	require.True(t, ok, "metrics object missing from show --json output")
	assert.Equal(t, float64(2), metrics["attempt_count"])
	assert.Equal(t, float64(3000), metrics["total_tokens"])
	assert.InDelta(t, 1.5, metrics["total_cost_usd"], 0.0001)

	// Show for a bead with no execution evidence returns metrics with zero values.
	require.NoError(t, os.WriteFile(beadsPath, []byte(
		`{"id":"ddx-1","title":"First bead","status":"closed","priority":2,"issue_type":"task","created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z"}`+"\n"+
			`{"id":"ddx-no-evidence","title":"No evidence","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z"}`+"\n"),
		0o644))
	output2, err := executeCommand(NewCommandFactory(dir).NewRootCommand(), "bead", "show", "ddx-no-evidence", "--json")
	require.NoError(t, err)
	var obj2 map[string]any
	require.NoError(t, json.Unmarshal([]byte(output2), &obj2))
	metrics2, ok := obj2["metrics"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(0), metrics2["attempt_count"])
	assert.Equal(t, float64(0), metrics2["total_tokens"])
	assert.InDelta(t, 0.0, metrics2["total_cost_usd"], 0.0001)
}
