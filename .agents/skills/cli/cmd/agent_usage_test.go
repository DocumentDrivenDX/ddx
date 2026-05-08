package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeJSONL(t *testing.T, path string, values ...any) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, value := range values {
		require.NoError(t, enc.Encode(value))
	}
}

func writeSessionIndex(t *testing.T, projectRoot, logDir string, entries ...agent.SessionEntry) {
	t.Helper()
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	for _, entry := range entries {
		require.NoError(t, agent.AppendSessionIndex(logDir, agent.SessionIndexEntryFromLegacy(projectRoot, entry), entry.Timestamp))
	}
}

func TestAgentUsageIncludesLegacySessionsAndRoutingOutcomes(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	logDir := filepath.Join(ddxDir, "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	config := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  session_log_dir: ".ddx/agent-logs"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0o644))

	mirroredSession := agent.SessionEntry{
		ID:              "as-current",
		Timestamp:       time.Date(2026, 4, 9, 10, 0, 1, 0, time.UTC),
		Harness:         "claude",
		Model:           "claude-sonnet-4-6",
		NativeSessionID: "native-current",
		TraceID:         "trace-current",
		InputTokens:     200,
		OutputTokens:    20,
		CostUSD:         2.50,
		Duration:        2000,
		ExitCode:        0,
	}
	legacySession := agent.SessionEntry{
		ID:           "as-legacy",
		Timestamp:    time.Date(2026, 4, 9, 9, 55, 0, 0, time.UTC),
		Harness:      "claude",
		Model:        "claude-sonnet-4-6",
		InputTokens:  120,
		OutputTokens: 12,
		CostUSD:      1.25,
		Duration:     1000,
		ExitCode:     0,
	}
	routingOutcome := agent.RoutingOutcome{
		Harness:         "claude",
		Surface:         "claude",
		CanonicalTarget: "claude-sonnet-4-6",
		Model:           "claude-sonnet-4-6",
		ObservedAt:      time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC),
		Success:         true,
		LatencyMS:       2000,
		InputTokens:     200,
		OutputTokens:    20,
		CostUSD:         2.50,
		NativeSessionID: "native-current",
		TraceID:         "trace-current",
	}

	writeSessionIndex(t, dir, logDir, legacySession, mirroredSession)
	writeJSONL(t, filepath.Join(logDir, "routing-outcomes.jsonl"), routingOutcome)

	rows, err := aggregateUsageFromRoutingMetrics(logDir, "", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, rows, 1)

	byHarness := map[string]usageRow{}
	for _, row := range rows {
		byHarness[row.Harness] = row
	}

	require.Contains(t, byHarness, "claude")

	assert.Equal(t, 2, byHarness["claude"].Sessions)
	assert.Equal(t, 320, byHarness["claude"].InputTokens)
	assert.Equal(t, 32, byHarness["claude"].OutputTokens)
	assert.InDelta(t, 3.75, byHarness["claude"].CostUSD, 0.0001)
	assert.Equal(t, usageCostBasisEstimatedValue, byHarness["claude"].CostBasis)
	assert.InDelta(t, 1500.0, byHarness["claude"].AvgDurationMS, 0.0001)
}

func TestAgentUsageSkipsUnkeyedCurrentSessionsWrittenBeforeRoutingOutcome(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	logDir := filepath.Join(ddxDir, "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	config := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
agent:
  session_log_dir: ".ddx/agent-logs"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0o644))

	legacySession := agent.SessionEntry{
		ID:           "as-legacy",
		Timestamp:    time.Date(2026, 4, 9, 9, 55, 0, 0, time.UTC),
		Harness:      "codex",
		Model:        "gpt-5.4",
		InputTokens:  120,
		OutputTokens: 12,
		CostUSD:      1.25,
		Duration:     1000,
		ExitCode:     0,
	}
	currentSession := agent.SessionEntry{
		ID:           "as-current",
		Timestamp:    time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC),
		Harness:      "codex",
		Model:        "gpt-5.4",
		InputTokens:  200,
		OutputTokens: 20,
		CostUSD:      2.50,
		Duration:     2000,
		ExitCode:     0,
	}
	outcome := agent.RoutingOutcome{
		Harness:         "codex",
		Surface:         "codex",
		CanonicalTarget: "gpt-5.4",
		Model:           "gpt-5.4",
		ObservedAt:      time.Date(2026, 4, 9, 10, 0, 1, 0, time.UTC),
		Success:         true,
		LatencyMS:       2000,
		InputTokens:     200,
		OutputTokens:    20,
		CostUSD:         2.50,
	}

	writeSessionIndex(t, dir, logDir, legacySession, currentSession)
	writeJSONL(t, filepath.Join(logDir, "routing-outcomes.jsonl"), outcome)

	rows, err := aggregateUsageFromRoutingMetrics(logDir, "", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, "codex", row.Harness)
	assert.Equal(t, 2, row.Sessions)
	assert.Equal(t, 320, row.InputTokens)
	assert.Equal(t, 32, row.OutputTokens)
	assert.InDelta(t, 3.75, row.CostUSD, 0.0001)
	assert.Equal(t, usageCostBasisEstimatedValue, row.CostBasis)
	assert.InDelta(t, 1500.0, row.AvgDurationMS, 0.0001)
}

func TestUsageCostBasis(t *testing.T) {
	t.Run("subscription_harness_cost_is_estimated_value", func(t *testing.T) {
		row := usageRow{Harness: "claude", CostUSD: 1.23, CostBasis: usageCostBasisReported}

		applyUsageCostBasis(&row, true)

		assert.Equal(t, usageCostBasisEstimatedValue, row.CostBasis)
	})

	t.Run("non_subscription_reported_cost_keeps_reported_basis", func(t *testing.T) {
		row := usageRow{Harness: "openrouter", CostUSD: 1.23, CostBasis: usageCostBasisReported}

		applyUsageCostBasis(&row, false)

		assert.Equal(t, usageCostBasisReported, row.CostBasis)
	})
}

func TestRenderUsageOutputsCostBasis(t *testing.T) {
	rows := []usageRow{{
		Harness:       "codex",
		Sessions:      1,
		InputTokens:   10,
		OutputTokens:  5,
		CostUSD:       0.12,
		CostBasis:     usageCostBasisEstimatedValue,
		AvgDurationMS: 1000,
	}}

	t.Run("table", func(t *testing.T) {
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)

		require.NoError(t, renderUsageTable(cmd, rows))

		assert.Contains(t, out.String(), "COST BASIS")
		assert.Contains(t, out.String(), usageCostBasisEstimatedValue)
	})

	t.Run("json", func(t *testing.T) {
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)

		require.NoError(t, renderUsageJSON(cmd, rows))

		assert.Contains(t, out.String(), `"cost_basis": "estimated_value"`)
	})

	t.Run("csv", func(t *testing.T) {
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)

		require.NoError(t, renderUsageCSV(cmd, rows))

		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		require.Len(t, lines, 2)
		assert.Contains(t, lines[0], "cost_basis")
		assert.Contains(t, lines[1], usageCostBasisEstimatedValue)
	})
}
