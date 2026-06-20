package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
