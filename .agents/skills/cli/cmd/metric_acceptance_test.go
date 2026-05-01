package cmd

import (
	"context"
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

func writeMetricFixture(t *testing.T, workingDir string) {
	t.Helper()
	artifactPath := filepath.Join(workingDir, "docs", "metrics", "MET-001.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("---\nddx:\n  id: MET-001\n---\n# Metric Startup Time\n"), 0o644))

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

	validateOut, err := executeCommand(rootCmd, "metric", "validate", "MET-001")
	require.NoError(t, err)
	assert.Contains(t, validateOut, "validated")

	store := ddxexec.NewStore(workingDir)
	_, err = store.Run(context.Background(), "exec-metric-startup-time@baseline")
	require.NoError(t, err)

	runOut, err := executeCommand(rootCmd, "metric", "run", "MET-001", "--json")
	require.NoError(t, err)

	var run metric.HistoryRecord
	require.NoError(t, json.Unmarshal([]byte(runOut), &run))
	assert.Equal(t, "MET-001", run.MetricID)
	assert.Equal(t, metric.StatusPass, run.Status)

	historyOut, err := executeCommand(rootCmd, "metric", "history", "MET-001", "--json")
	require.NoError(t, err)
	var history []metric.HistoryRecord
	require.NoError(t, json.Unmarshal([]byte(historyOut), &history))
	require.Len(t, history, 2)

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

func mustMetricTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}
