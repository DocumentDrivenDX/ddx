package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/require"
)

func TestAgentDoctorRoutingIncludesSignalMetadata(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "library"), 0o755))

	config := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  session_log_dir: ".ddx/agent-logs"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0o644))

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	codexPath := filepath.Join(dir, "codex-session.jsonl")
	require.NoError(t, os.WriteFile(codexPath, []byte(
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":10}}`+"\n"+
			`{"type":"session.updated","token_count":{"rate_limits":{"primary":{"used_percent":97,"window_minutes":300,"resets_at":"April 12"}}}}`+"\n",
	), 0o644))
	t.Setenv("DDX_CODEX_NATIVE_SESSION_JSONL", codexPath)

	home := filepath.Join(dir, "home")
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	claudePath := filepath.Join(claudeDir, "stats-cache.json")
	require.NoError(t, os.WriteFile(claudePath, []byte(`{
		"dailyActivity": [{"date":"2026-04-09","sessions":2,"usage":{"input_tokens":100,"cached_input_tokens":5,"output_tokens":20}}],
		"modelUsage": {"claude-sonnet-4-6": {"inputTokens":500,"outputTokens":70,"cachedInputTokens":12,"sessionCount":3}}
	}`), 0o644))
	t.Setenv("HOME", home)
	t.Setenv("DDX_CLAUDE_STATS_CACHE", claudePath)

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "doctor", "--routing", "--json")
	require.NoError(t, err)

	type doctorSignal struct {
		Provider string `json:"provider"`
		Source   struct {
			Freshness string `json:"freshness"`
			Kind      string `json:"kind"`
		} `json:"source"`
		HistoricalUsage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"historical_usage"`
	}
	type doctorState struct {
		QuotaState    string        `json:"quota_state"`
		RoutingSignal *doctorSignal `json:"routing_signal"`
	}
	type doctorEntry struct {
		Name  string      `json:"name"`
		State doctorState `json:"state"`
	}

	var entries []doctorEntry
	require.NoError(t, json.Unmarshal([]byte(output), &entries))

	var codex, claude *doctorEntry
	for i := range entries {
		switch entries[i].Name {
		case "codex":
			codex = &entries[i]
		case "claude":
			claude = &entries[i]
		}
	}
	require.NotNil(t, codex)
	require.NotNil(t, claude)
	require.NotNil(t, codex.State.RoutingSignal)
	require.Equal(t, "blocked", codex.State.QuotaState)
	require.Equal(t, "fresh", codex.State.RoutingSignal.Source.Freshness)
	require.Equal(t, "codex", codex.State.RoutingSignal.Provider)
	require.Equal(t, 135, codex.State.RoutingSignal.HistoricalUsage.TotalTokens)
	require.NotNil(t, claude.State.RoutingSignal)
	require.Equal(t, "unknown", claude.State.QuotaState)
	require.Equal(t, "cached", claude.State.RoutingSignal.Source.Freshness)
	require.Equal(t, "claude", claude.State.RoutingSignal.Provider)
	require.Equal(t, 582, claude.State.RoutingSignal.HistoricalUsage.TotalTokens)
}

func TestAgentUsageAnnotatesRowsWithRoutingSignals(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	logDir := filepath.Join(ddxDir, "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "library"), 0o755))

	config := `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  session_log_dir: ".ddx/agent-logs"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(config), 0o644))

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	codexPath := filepath.Join(dir, "codex-session.jsonl")
	require.NoError(t, os.WriteFile(codexPath, []byte(
		`{"type":"turn.completed","usage":{"input_tokens":50,"cached_input_tokens":10,"output_tokens":5}}`+"\n"+
			`{"type":"session.updated","token_count":{"rate_limits":{"primary":{"used_percent":97,"window_minutes":300,"resets_at":"April 12"}}}}`+"\n",
	), 0o644))
	t.Setenv("DDX_CODEX_NATIVE_SESSION_JSONL", codexPath)

	home := filepath.Join(dir, "home")
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	claudePath := filepath.Join(claudeDir, "stats-cache.json")
	require.NoError(t, os.WriteFile(claudePath, []byte(`{
		"dailyActivity": [{"date":"2026-04-09","sessions":2,"usage":{"input_tokens":100,"cached_input_tokens":5,"output_tokens":20}}],
		"modelUsage": {"claude-sonnet-4-6": {"inputTokens":500,"outputTokens":70,"cachedInputTokens":12,"sessionCount":3}}
	}`), 0o644))
	t.Setenv("HOME", home)
	t.Setenv("DDX_CLAUDE_STATS_CACHE", claudePath)

	writeJSONL(t, filepath.Join(logDir, "sessions.jsonl"),
		agent.SessionEntry{
			ID:           "s1",
			Timestamp:    time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC),
			Harness:      "codex",
			Model:        "gpt-5.4",
			InputTokens:  100,
			OutputTokens: 10,
			CostUSD:      1.0,
			Duration:     1000,
			ExitCode:     0,
		},
		agent.SessionEntry{
			ID:           "s2",
			Timestamp:    time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC),
			Harness:      "claude",
			Model:        "claude-sonnet-4-6",
			InputTokens:  200,
			OutputTokens: 20,
			CostUSD:      2.0,
			Duration:     2000,
			ExitCode:     0,
		},
	)
	writeJSONL(t, filepath.Join(logDir, "routing-outcomes.jsonl"),
		agent.RoutingOutcome{
			Harness:         "codex",
			Surface:         "codex",
			CanonicalTarget: "gpt-5.4",
			Model:           "gpt-5.4",
			ObservedAt:      time.Date(2026, 4, 9, 10, 0, 1, 0, time.UTC),
			Success:         true,
			LatencyMS:       1000,
			InputTokens:     100,
			OutputTokens:    10,
			CostUSD:         1.0,
		},
		agent.RoutingOutcome{
			Harness:         "claude",
			Surface:         "claude",
			CanonicalTarget: "claude-sonnet-4-6",
			Model:           "claude-sonnet-4-6",
			ObservedAt:      time.Date(2026, 4, 9, 11, 0, 1, 0, time.UTC),
			Success:         true,
			LatencyMS:       2000,
			InputTokens:     200,
			OutputTokens:    20,
			CostUSD:         2.0,
		},
	)

	rootCmd := NewCommandFactory(dir).NewRootCommand()
	output, err := executeCommand(rootCmd, "agent", "usage", "--format", "json", "--since", "2026-04-01")
	require.NoError(t, err)

	type usageSignalRow struct {
		Harness                string `json:"harness"`
		QuotaState             string `json:"quota_state"`
		SignalProvider         string `json:"signal_provider"`
		SignalFreshness        string `json:"signal_freshness"`
		NativeTotalTokens      int    `json:"native_total_tokens"`
		NativeQuotaUsedPercent int    `json:"native_quota_used_percent"`
	}
	var rows []usageSignalRow
	require.NoError(t, json.Unmarshal([]byte(output), &rows))
	require.Len(t, rows, 2)

	var codex, claude *usageSignalRow
	for i := range rows {
		switch rows[i].Harness {
		case "codex":
			codex = &rows[i]
		case "claude":
			claude = &rows[i]
		}
	}
	require.NotNil(t, codex)
	require.NotNil(t, claude)
	require.Equal(t, "blocked", codex.QuotaState)
	require.Equal(t, "codex", codex.SignalProvider)
	require.Equal(t, "fresh", codex.SignalFreshness)
	require.Equal(t, 65, codex.NativeTotalTokens)
	require.Equal(t, 97, codex.NativeQuotaUsedPercent)
	require.Equal(t, "unknown", claude.QuotaState)
	require.Equal(t, "claude", claude.SignalProvider)
	require.Equal(t, "cached", claude.SignalFreshness)
	require.Equal(t, 582, claude.NativeTotalTokens)
}
