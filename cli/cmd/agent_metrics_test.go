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

// writeExecResult writes a minimal result.json fixture for tier-success tests.
func writeExecResult(t *testing.T, execRoot, attemptID string, res map[string]any) {
	t.Helper()
	dir := filepath.Join(execRoot, attemptID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), raw, 0o644))
}

func TestAgentMetricsTierSuccess(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	execRoot := filepath.Join(dir, ".ddx", "executions")

	// claude/sonnet: 2 attempts, 1 success.
	writeExecResult(t, execRoot, "20260401T100000-aaaa0001", map[string]any{
		"bead_id":     "ddx-1",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_succeeded",
		"duration_ms": 100000,
		"cost_usd":    1.0,
	})
	writeExecResult(t, execRoot, "20260401T110000-aaaa0002", map[string]any{
		"bead_id":     "ddx-2",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_failed",
		"duration_ms": 200000,
		"cost_usd":    2.0,
	})
	// claude/opus: 1 attempt, 1 success.
	writeExecResult(t, execRoot, "20260401T120000-aaaa0003", map[string]any{
		"bead_id":     "ddx-3",
		"harness":     "claude",
		"model":       "opus",
		"outcome":     "task_succeeded",
		"duration_ms": 300000,
		"cost_usd":    5.0,
	})
	// agent (no model): 1 attempt, 0 successes (error).
	writeExecResult(t, execRoot, "20260401T130000-aaaa0004", map[string]any{
		"bead_id":     "ddx-4",
		"harness":     "agent",
		"outcome":     "error",
		"duration_ms": 1000,
	})

	// Malformed result.json (should be skipped).
	badDir := filepath.Join(execRoot, "20260401T140000-aaaa0005")
	require.NoError(t, os.MkdirAll(badDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(badDir, "result.json"), []byte("not json"), 0o644))

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "metrics", "tier-success", "--json")
	require.NoError(t, err)

	var rows []tierSuccessRow
	require.NoError(t, json.Unmarshal([]byte(output), &rows))

	byTier := map[string]tierSuccessRow{}
	for _, r := range rows {
		byTier[r.Tier] = r
	}

	require.Contains(t, byTier, "claude/sonnet")
	assert.Equal(t, 2, byTier["claude/sonnet"].Attempts)
	assert.Equal(t, 1, byTier["claude/sonnet"].Successes)
	assert.InDelta(t, 0.5, byTier["claude/sonnet"].SuccessRate, 0.0001)
	assert.InDelta(t, 1.5, byTier["claude/sonnet"].AvgCostUSD, 0.0001)
	assert.InDelta(t, 150000.0, byTier["claude/sonnet"].AvgDurationMS, 0.0001)

	require.Contains(t, byTier, "claude/opus")
	assert.Equal(t, 1, byTier["claude/opus"].Attempts)
	assert.Equal(t, 1, byTier["claude/opus"].Successes)
	assert.InDelta(t, 1.0, byTier["claude/opus"].SuccessRate, 0.0001)

	require.Contains(t, byTier, "agent")
	assert.Equal(t, 1, byTier["agent"].Attempts)
	assert.Equal(t, 0, byTier["agent"].Successes)
	assert.InDelta(t, 0.0, byTier["agent"].SuccessRate, 0.0001)

	// Table output shape: has header columns for tier, attempts, successes,
	// success_rate, avg_cost_usd, avg_duration_ms.
	tableCmd := NewCommandFactory(dir).NewRootCommand()
	tableOut, err := executeCommand(tableCmd, "agent", "metrics", "tier-success")
	require.NoError(t, err)
	header := strings.SplitN(tableOut, "\n", 2)[0]
	for _, col := range []string{"TIER", "ATTEMPTS", "SUCCESSES", "SUCCESS_RATE", "AVG_COST_USD", "AVG_DURATION_MS"} {
		assert.Contains(t, header, col)
	}

	// --last 1 keeps only the most recent attempt (agent / error).
	lastCmd := NewCommandFactory(dir).NewRootCommand()
	lastOut, err := executeCommand(lastCmd, "agent", "metrics", "tier-success", "--last", "1", "--json")
	require.NoError(t, err)
	var lastRows []tierSuccessRow
	require.NoError(t, json.Unmarshal([]byte(lastOut), &lastRows))
	require.Len(t, lastRows, 1)
	assert.Equal(t, "agent", lastRows[0].Tier)
	assert.Equal(t, 1, lastRows[0].Attempts)
	assert.Equal(t, 0, lastRows[0].Successes)

	// Empty executions dir returns an empty list, not an error.
	empty := t.TempDir()
	emptyCmd := NewCommandFactory(empty).NewRootCommand()
	emptyOut, err := executeCommand(emptyCmd, "agent", "metrics", "tier-success", "--json")
	require.NoError(t, err)
	var emptyRows []tierSuccessRow
	require.NoError(t, json.Unmarshal([]byte(emptyOut), &emptyRows))
	assert.Empty(t, emptyRows)
}
