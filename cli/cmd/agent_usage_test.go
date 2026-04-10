package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
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

	legacySession := agent.SessionEntry{
		ID:           "as-legacy",
		Timestamp:    time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		Harness:      "codex",
		Model:        "gpt-5.4",
		InputTokens:  120,
		OutputTokens: 12,
		CostUSD:      1.25,
		Duration:     1000,
		ExitCode:     0,
	}
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

	writeJSONL(t, filepath.Join(logDir, "sessions.jsonl"), legacySession, mirroredSession)
	writeJSONL(t, filepath.Join(logDir, "routing-outcomes.jsonl"), routingOutcome)

	rows, err := aggregateUsageFromRoutingMetrics(logDir, "", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byHarness := map[string]usageRow{}
	for _, row := range rows {
		byHarness[row.Harness] = row
	}

	require.Contains(t, byHarness, "codex")
	require.Contains(t, byHarness, "claude")

	assert.Equal(t, 1, byHarness["codex"].Sessions)
	assert.Equal(t, 120, byHarness["codex"].InputTokens)
	assert.Equal(t, 12, byHarness["codex"].OutputTokens)
	assert.InDelta(t, 1.25, byHarness["codex"].CostUSD, 0.0001)

	assert.Equal(t, 1, byHarness["claude"].Sessions)
	assert.Equal(t, 200, byHarness["claude"].InputTokens)
	assert.Equal(t, 20, byHarness["claude"].OutputTokens)
	assert.InDelta(t, 2.50, byHarness["claude"].CostUSD, 0.0001)
}
