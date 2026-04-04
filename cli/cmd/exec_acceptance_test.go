package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/easel/ddx/internal/agent"
	ddxexec "github.com/easel/ddx/internal/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAgentRunnerCmd struct {
	result *agent.Result
}

func (m *mockAgentRunnerCmd) Run(opts agent.RunOptions) (*agent.Result, error) {
	return m.result, nil
}

func writeExecFixture(t *testing.T, workingDir string) {
	t.Helper()
	artifactPath := filepath.Join(workingDir, "docs", "metrics", "MET-001.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("---\nddx:\n  id: MET-001\n---\n# Metric Startup Time\n"), 0o644))

	store := ddxexec.NewStore(workingDir)
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          "exec-metric-startup-time@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '14.6ms\\n'"},
		},
		Result:    ddxexec.ResultSpec{Metric: &ddxexec.MetricResultSpec{Unit: "ms"}},
		Active:    true,
		CreatedAt: mustExecAcceptanceTime(t, "2026-04-04T15:00:00Z"),
	}))
}

func TestExecCommandsValidateRunHistoryAndResult(t *testing.T) {
	workingDir := t.TempDir()
	writeExecFixture(t, workingDir)

	factory := NewCommandFactory(workingDir)
	rootCmd := factory.NewRootCommand()

	validateOut, err := executeCommand(rootCmd, "exec", "validate", "exec-metric-startup-time@1")
	require.NoError(t, err)
	assert.Contains(t, validateOut, "validated")

	runOut, err := executeCommand(rootCmd, "exec", "run", "exec-metric-startup-time@1", "--json")
	require.NoError(t, err)
	var rec ddxexec.RunRecord
	require.NoError(t, json.Unmarshal([]byte(runOut), &rec))
	assert.Equal(t, ddxexec.StatusSuccess, rec.Status)
	require.NotNil(t, rec.Result.Metric)
	assert.Equal(t, "MET-001", rec.Result.Metric.ArtifactID)

	historyOut, err := executeCommand(rootCmd, "exec", "history", "--artifact", "MET-001", "--json")
	require.NoError(t, err)
	var history []ddxexec.RunRecord
	require.NoError(t, json.Unmarshal([]byte(historyOut), &history))
	require.Len(t, history, 1)

	resultOut, err := executeCommand(rootCmd, "exec", "result", rec.RunID)
	require.NoError(t, err)
	assert.Contains(t, resultOut, "metric")

	logOut, err := executeCommand(rootCmd, "exec", "log", rec.RunID)
	require.NoError(t, err)
	assert.Contains(t, logOut, "14.6")
}

func mustExecAcceptanceTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}

func TestExecAgentDelegation(t *testing.T) {
	workingDir := t.TempDir()

	// Write artifact
	artifactPath := filepath.Join(workingDir, "docs", "metrics", "MET-001.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("---\nddx:\n  id: MET-001\n---\n# Metric\n"), 0o644))

	// Write agent-kind exec definition
	store := ddxexec.NewStore(workingDir)
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          "exec-agent-task@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind: ddxexec.ExecutorKindAgent,
			Env: map[string]string{
				"DDX_AGENT_HARNESS": "codex",
				"DDX_AGENT_PROMPT":  "run the task",
			},
		},
		Active:    true,
		CreatedAt: mustExecAcceptanceTime(t, "2026-04-04T15:00:00Z"),
	}))

	factory := NewCommandFactory(workingDir)
	factory.AgentRunnerOverride = &mockAgentRunnerCmd{
		result: &agent.Result{
			Harness:  "codex",
			ExitCode: 0,
			Output:   "agent output here",
		},
	}
	rootCmd := factory.NewRootCommand()

	runOut, err := executeCommand(rootCmd, "exec", "run", "exec-agent-task@1", "--json")
	require.NoError(t, err)

	var rec ddxexec.RunRecord
	require.NoError(t, json.Unmarshal([]byte(runOut), &rec))
	assert.Equal(t, ddxexec.StatusSuccess, rec.Status)
	assert.Equal(t, 0, rec.ExitCode)
	assert.NotEmpty(t, rec.AgentSessionID)
	assert.Equal(t, "agent output here", rec.Result.Stdout)
	assert.Equal(t, "exec-agent-task@1", rec.DefinitionID)
}
