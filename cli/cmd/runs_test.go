package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunsCommandHelp(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	dir := testutils.NewFixtureRepo(t, "minimal")

	factory := NewCommandFactory(dir)
	rootCmd := factory.NewRootCommand()

	out, err := executeCommand(rootCmd, "runs", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "log", "runs --help should list log subcommand")
	assert.Contains(t, out, "history", "runs --help should list history subcommand")
}

func TestRunsCommandLogHistory(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	t.Run("log_reads_from_evidence_store", func(t *testing.T) {
		dir := testutils.NewFixtureRepo(t, "minimal")
		factory := NewCommandFactory(dir)
		rootCmd := factory.NewRootCommand()

		out, err := executeCommand(rootCmd, "runs", "log")
		require.NoError(t, err)
		assert.NotEmpty(t, out)
	})

	t.Run("history_empty_returns_null_or_empty", func(t *testing.T) {
		dir := testutils.NewFixtureRepo(t, "minimal")
		factory := NewCommandFactory(dir)
		rootCmd := factory.NewRootCommand()

		out, err := executeCommand(rootCmd, "runs", "history", "--json")
		require.NoError(t, err)
		trimmed := strings.TrimSpace(out)
		assert.True(t, trimmed == "null" || trimmed == "[]", "empty history should be null or [], got: %s", trimmed)
	})

	t.Run("history_surfaces_exec_runs", func(t *testing.T) {
		workDir := t.TempDir()

		artifactPath := filepath.Join(workDir, "docs", "metrics", "MET-RUNS.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
		require.NoError(t, os.WriteFile(artifactPath, []byte("---\nddx:\n  id: MET-RUNS\n---\n# Metric\n"), 0o644))

		store := ddxexec.NewStore(workDir)
		require.NoError(t, store.SaveDefinition(ddxexec.Definition{
			ID:          "runs-hist-def@1",
			ArtifactIDs: []string{"MET-RUNS"},
			Executor: ddxexec.ExecutorSpec{
				Kind:    ddxexec.ExecutorKindCommand,
				Command: []string{"sh", "-c", "echo ok"},
			},
			Active: true,
		}))

		factory := NewCommandFactory(workDir)
		rootCmd := factory.NewRootCommand()

		_, err := executeCommand(rootCmd, "exec", "run", "runs-hist-def@1")
		require.NoError(t, err)

		histOut, err := executeCommand(rootCmd, "runs", "history", "--json")
		require.NoError(t, err)

		var records []ddxexec.RunRecord
		require.NoError(t, json.Unmarshal([]byte(histOut), &records))
		require.Len(t, records, 1)
		assert.Equal(t, "runs-hist-def@1", records[0].DefinitionID)
	})
}

func TestRunsCommandMetrics(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := testutils.NewFixtureRepo(t, "minimal")
	execRoot := filepath.Join(dir, ddxroot.DirName, "executions")

	writeExecResult(t, execRoot, "20260601T100000-met0001", map[string]any{
		"bead_id":     "ddx-m1",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_succeeded",
		"duration_ms": 120000,
		"tokens":      800,
		"cost_usd":    0.4,
	})
	writeExecResult(t, execRoot, "20260601T110000-met0002", map[string]any{
		"bead_id":     "ddx-m1",
		"harness":     "claude",
		"model":       "sonnet",
		"outcome":     "task_failed",
		"duration_ms": 60000,
		"tokens":      400,
		"cost_usd":    0.2,
	})
	writeExecResult(t, execRoot, "20260601T120000-met0003", map[string]any{
		"bead_id":     "ddx-m2",
		"harness":     "claude",
		"model":       "opus",
		"outcome":     "task_succeeded",
		"duration_ms": 200000,
		"tokens":      1000,
		"cost_usd":    0.8,
	})

	// Seed beads.jsonl with titles for the fixture beads.
	beadsPath := filepath.Join(dir, ddxroot.DirName, "beads.jsonl")
	beadLines := `{"id":"ddx-m1","title":"Metrics bead one","status":"closed","priority":2,"issue_type":"task","created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-01T00:00:00Z"}` + "\n" +
		`{"id":"ddx-m2","title":"Metrics bead two","status":"closed","priority":2,"issue_type":"task","created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-01T00:00:00Z"}` + "\n"
	existing, _ := os.ReadFile(beadsPath)
	require.NoError(t, os.WriteFile(beadsPath, append(existing, []byte(beadLines)...), 0o644))

	t.Run("help_exits_zero", func(t *testing.T) {
		rootCmd := NewCommandFactory(dir).NewRootCommand()
		out, err := executeCommand(rootCmd, "runs", "metrics", "--help")
		require.NoError(t, err)
		assert.Contains(t, out, "metrics")
	})

	t.Run("json_output_aggregates_by_bead", func(t *testing.T) {
		rootCmd := NewCommandFactory(dir).NewRootCommand()
		out, err := executeCommand(rootCmd, "runs", "metrics", "--json")
		require.NoError(t, err)

		var rows []beadMetricsRow
		require.NoError(t, json.Unmarshal([]byte(out), &rows))

		byID := map[string]beadMetricsRow{}
		for _, r := range rows {
			byID[r.BeadID] = r
		}

		require.Contains(t, byID, "ddx-m1")
		assert.Equal(t, 2, byID["ddx-m1"].AttemptCount)
		assert.Equal(t, 1200, byID["ddx-m1"].TotalTokens)
		assert.InDelta(t, 0.6, byID["ddx-m1"].TotalCostUSD, 0.0001)
		assert.Equal(t, "Metrics bead one", byID["ddx-m1"].Title)

		require.Contains(t, byID, "ddx-m2")
		assert.Equal(t, 1, byID["ddx-m2"].AttemptCount)
		assert.Equal(t, 1000, byID["ddx-m2"].TotalTokens)
		assert.InDelta(t, 0.8, byID["ddx-m2"].TotalCostUSD, 0.0001)
		assert.Equal(t, "Metrics bead two", byID["ddx-m2"].Title)
	})

	t.Run("table_output_has_header_columns", func(t *testing.T) {
		rootCmd := NewCommandFactory(dir).NewRootCommand()
		out, err := executeCommand(rootCmd, "runs", "metrics")
		require.NoError(t, err)
		header := strings.SplitN(out, "\n", 2)[0]
		for _, col := range []string{"BEAD_ID", "ATTEMPTS", "TOTAL_TOKENS", "TOTAL_COST_USD"} {
			assert.Contains(t, header, col)
		}
	})
}
