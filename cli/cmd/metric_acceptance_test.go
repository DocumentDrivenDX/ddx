package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	"github.com/DocumentDrivenDX/ddx/internal/metric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMetricTestRoot(t *testing.T, workingDir string) *CommandFactory {
	t.Helper()
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	return NewCommandFactory(workingDir)
}

func writeMetricArtifact(t *testing.T, workingDir, id, title string) {
	t.Helper()
	artifactPath := filepath.Join(workingDir, "docs", "metrics", id+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	if title == "" {
		title = id
	}
	require.NoError(t, os.WriteFile(artifactPath, []byte("---\nddx:\n  id: "+id+"\n---\n# "+title+"\n"), 0o644))
}

func writeMetricFixture(t *testing.T, workingDir string) {
	t.Helper()
	writeMetricArtifact(t, workingDir, "MET-001", "Metric Startup Time")

	store := ddxexec.NewStore(workingDir)
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          "exec-metric-startup-time@baseline",
		ArtifactIDs: []string{"MET-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '20ms\\n'"},
		},
		Result: ddxexec.ResultSpec{Metric: &ddxexec.MetricResultSpec{Unit: "ms"}},
		Evaluation: ddxexec.Evaluation{
			Comparison: metric.ComparisonLowerIsBetter,
			Thresholds: ddxexec.Thresholds{WarnMS: 20, RatchetMS: 30},
		},
		Active:    true,
		CreatedAt: mustMetricTime(t, "2026-04-04T15:00:00Z"),
	}))
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          "exec-metric-startup-time@current",
		ArtifactIDs: []string{"MET-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '14.6ms\\n'"},
		},
		Result: ddxexec.ResultSpec{Metric: &ddxexec.MetricResultSpec{Unit: "ms"}},
		Evaluation: ddxexec.Evaluation{
			Comparison: metric.ComparisonLowerIsBetter,
			Thresholds: ddxexec.Thresholds{WarnMS: 20, RatchetMS: 30},
		},
		Active:    true,
		CreatedAt: mustMetricTime(t, "2026-04-04T15:01:00Z"),
	}))
}

func TestMetricCommandsValidateRunHistoryAndCompare(t *testing.T) {
	workingDir := t.TempDir()
	writeMetricFixture(t, workingDir)

	factory := newMetricTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	listOut, err := executeCommand(rootCmd, "metric", "list", "--json")
	require.NoError(t, err)
	var artifacts []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	require.NoError(t, json.Unmarshal([]byte(listOut), &artifacts))
	require.Len(t, artifacts, 1)
	assert.Equal(t, "MET-001", artifacts[0].ID)
	assert.Equal(t, "Metric Startup Time", artifacts[0].Title)

	validateOut, err := executeCommand(rootCmd, "metric", "validate", "MET-001")
	require.NoError(t, err)
	assert.Contains(t, validateOut, "validated")

	execRunOut, err := executeCommand(rootCmd, "exec", "run", "exec-metric-startup-time@baseline", "--json")
	require.NoError(t, err)
	var execRun ddxexec.RunRecord
	require.NoError(t, json.Unmarshal([]byte(execRunOut), &execRun))
	assert.Equal(t, "exec-metric-startup-time@baseline", execRun.DefinitionID)

	runOut, err := executeCommand(rootCmd, "metric", "run", "MET-001", "--json")
	require.NoError(t, err)

	var run metric.HistoryRecord
	require.NoError(t, json.Unmarshal([]byte(runOut), &run))
	assert.Equal(t, "MET-001", run.MetricID)
	assert.Equal(t, metric.StatusPass, run.Status)
	assert.Equal(t, "exec-metric-startup-time@current", run.DefinitionID)
	assert.InDelta(t, 14.6, run.Value, 0.01)

	showOut, err := executeCommand(rootCmd, "metric", "show", "MET-001", "--json")
	require.NoError(t, err)
	var show struct {
		Artifact struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"artifact"`
		Definition struct {
			DefinitionID string `json:"definition_id"`
		} `json:"definition"`
		RecentHistory []metric.HistoryRecord `json:"recent_history"`
	}
	require.NoError(t, json.Unmarshal([]byte(showOut), &show))
	assert.Equal(t, "MET-001", show.Artifact.ID)
	assert.Equal(t, "exec-metric-startup-time@current", show.Definition.DefinitionID)
	require.Len(t, show.RecentHistory, 2)
	assert.Equal(t, "exec-metric-startup-time@baseline", show.RecentHistory[0].DefinitionID)
	assert.Equal(t, "exec-metric-startup-time@current", show.RecentHistory[1].DefinitionID)

	historyOut, err := executeCommand(rootCmd, "metric", "history", "MET-001", "--json")
	require.NoError(t, err)
	var history []metric.HistoryGroup
	require.NoError(t, json.Unmarshal([]byte(historyOut), &history))
	require.Len(t, history, 1)
	assert.Equal(t, "ms", history[0].Unit)
	require.Len(t, history[0].Records, 2)

	compareOut, err := executeCommand(rootCmd, "metric", "compare", "MET-001", "--against", "baseline", "--json")
	require.NoError(t, err)
	assert.Contains(t, compareOut, "comparison")
	assert.Contains(t, strings.ToLower(compareOut), "baseline")

	trendOut, err := executeCommand(rootCmd, "metric", "trend", "MET-001", "--json")
	require.NoError(t, err)
	var trend metric.TrendSummary
	require.NoError(t, json.Unmarshal([]byte(trendOut), &trend))
	assert.Equal(t, 2, trend.Count)
	assert.Equal(t, "MET-001", trend.MetricID)
}

func TestMetricCommandsRefuseMixedUnitsAndGroupHistoryByUnit(t *testing.T) {
	workingDir := t.TempDir()
	writeMetricArtifact(t, workingDir, "MET-001", "")

	store := ddxexec.NewStore(workingDir)
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          "exec-metric-startup-time@baseline",
		ArtifactIDs: []string{"MET-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '20ms\\n'"},
		},
		Result: ddxexec.ResultSpec{Metric: &ddxexec.MetricResultSpec{Unit: "ms"}},
		Evaluation: ddxexec.Evaluation{
			Comparison: metric.ComparisonLowerIsBetter,
			Thresholds: ddxexec.Thresholds{WarnMS: 20, RatchetMS: 30},
		},
		Active:    true,
		CreatedAt: mustMetricTime(t, "2026-04-04T15:00:00Z"),
	}))
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          "exec-metric-startup-time@usd",
		ArtifactIDs: []string{"MET-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '10USD\\n'"},
		},
		Result: ddxexec.ResultSpec{Metric: &ddxexec.MetricResultSpec{Unit: "USD"}},
		Evaluation: ddxexec.Evaluation{
			Comparison: metric.ComparisonLowerIsBetter,
			Thresholds: ddxexec.Thresholds{WarnMS: 20, RatchetMS: 30},
		},
		Active:    true,
		CreatedAt: mustMetricTime(t, "2026-04-04T15:01:00Z"),
	}))

	factory := newMetricTestRoot(t, workingDir)
	rootCmd := factory.NewRootCommand()

	_, err := executeCommand(rootCmd, "exec", "run", "exec-metric-startup-time@baseline")
	require.NoError(t, err)
	_, err = executeCommand(rootCmd, "exec", "run", "exec-metric-startup-time@usd")
	require.NoError(t, err)

	_, err = executeCommand(rootCmd, "metric", "compare", "MET-001", "--against", "baseline")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "mixed units")

	_, err = executeCommand(rootCmd, "metric", "trend", "MET-001")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "mixed units")

	historyOut, err := executeCommand(rootCmd, "metric", "history", "MET-001", "--json")
	require.NoError(t, err)
	var grouped []metric.HistoryGroup
	require.NoError(t, json.Unmarshal([]byte(historyOut), &grouped))
	require.Len(t, grouped, 2)
	assert.Equal(t, "ms", grouped[0].Unit)
	assert.Equal(t, "USD", grouped[1].Unit)
	require.Len(t, grouped[0].Records, 1)
	require.Len(t, grouped[1].Records, 1)
}

func mustMetricTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}
