package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeHarnessStateUsesCodexNativeRoutingSignal(t *testing.T) {
	r := newTestRunnerForRouting()

	dir := t.TempDir()
	path := filepath.Join(dir, "codex-session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":10}}`+"\n"+
			`{"type":"session.updated","token_count":{"rate_limits":{"primary":{"used_percent":97,"window_minutes":300,"resets_at":"April 12"}}}}`+"\n",
	), 0o644))
	t.Setenv(codexNativeSessionEnv, path)

	state := r.ProbeHarnessState("codex", 2*time.Second)
	require.NotNil(t, state.RoutingSignal)

	assert.Equal(t, "blocked", state.QuotaState)
	assert.False(t, state.QuotaOK)
	assert.Equal(t, 97, state.Quota.PercentUsed)
	assert.Equal(t, "codex", state.RoutingSignal.Provider)
	assert.Equal(t, codexNativeSessionSourceKind, state.RoutingSignal.Source.Kind)
	assert.Equal(t, "fresh", state.RoutingSignal.Source.Freshness)
	assert.Equal(t, 135, state.RoutingSignal.HistoricalUsage.TotalTokens)
}

func TestProbeHarnessStateAutoDiscoversCodexNativeRoutingSignal(t *testing.T) {
	r := newTestRunnerForRouting()

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	sessionDir := filepath.Join(home, ".codex", "sessions", "2026", "04", "11")
	require.NoError(t, os.MkdirAll(sessionDir, 0o755))
	path := filepath.Join(sessionDir, "rollout-2026-04-11T05-00-00-test.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":10}}`+"\n"+
			`{"type":"session.updated","token_count":{"rate_limits":{"primary":{"used_percent":97,"window_minutes":300,"resets_at":"April 12"}}}}`+"\n",
	), 0o644))
	t.Setenv("HOME", home)
	t.Setenv(codexNativeSessionEnv, "")

	state := r.ProbeHarnessState("codex", 2*time.Second)
	require.NotNil(t, state.RoutingSignal)

	assert.Equal(t, "blocked", state.QuotaState)
	assert.False(t, state.QuotaOK)
	assert.Equal(t, path, state.RoutingSignal.Source.Path)
	assert.Equal(t, "codex", state.RoutingSignal.Provider)
	assert.Equal(t, codexNativeSessionSourceKind, state.RoutingSignal.Source.Kind)
	assert.Equal(t, 135, state.RoutingSignal.HistoricalUsage.TotalTokens)
}

func TestProbeHarnessStateUsesClaudeStatsCacheAndLeavesQuotaUnknown(t *testing.T) {
	r := newTestRunnerForRouting()

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	statsDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(statsDir, 0o755))
	path := filepath.Join(statsDir, "stats-cache.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"dailyActivity": [
			{"date":"2026-04-09","sessions":2,"usage":{"input_tokens":100,"cached_input_tokens":5,"output_tokens":20}}
		],
		"modelUsage": {
			"claude-sonnet-4-6": {"inputTokens":500,"outputTokens":70,"cachedInputTokens":12,"sessionCount":3}
		}
	}`), 0o644))
	t.Setenv("HOME", home)
	t.Setenv(claudeStatsCacheEnv, path)

	state := r.ProbeHarnessState("claude", 2*time.Second)
	require.NotNil(t, state.RoutingSignal)

	assert.Equal(t, "unknown", state.QuotaState)
	assert.True(t, state.QuotaOK)
	assert.Equal(t, "claude", state.RoutingSignal.Provider)
	assert.Equal(t, claudeStatsCacheSourceKind, state.RoutingSignal.Source.Kind)
	assert.Equal(t, "cached", state.RoutingSignal.Source.Freshness)
	assert.Equal(t, 582, state.RoutingSignal.HistoricalUsage.TotalTokens)
}

func TestProbeHarnessStateUsesClaudeQuotaSnapshotWhenAvailable(t *testing.T) {
	r := newTestRunnerForRouting()

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	statsDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(statsDir, 0o755))
	statsPath := filepath.Join(statsDir, "stats-cache.json")
	require.NoError(t, os.WriteFile(statsPath, []byte(`{
		"dailyActivity": [
			{"date":"2026-04-09","sessions":2,"usage":{"input_tokens":100,"cached_input_tokens":5,"output_tokens":20}}
		]
	}`), 0o644))
	snapshotPath := filepath.Join(home, ".ddx", "provider-state", "claude-quota.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o755))
	require.NoError(t, WriteClaudeQuotaSnapshot(snapshotPath, QuotaSignal{
		Source: SignalSourceMetadata{
			ObservedAt: time.Now().UTC().Add(-30 * time.Minute),
			Basis:      "async snapshot",
			Notes:      "captured in background",
		},
		State:         "ok",
		UsedPercent:   61,
		WindowMinutes: 300,
		ResetsAt:      "2026-04-12T04:00:00Z",
	}))

	t.Setenv("HOME", home)
	t.Setenv(claudeStatsCacheEnv, statsPath)
	t.Setenv(claudeQuotaSnapshotEnv, snapshotPath)

	state := r.ProbeHarnessState("claude", 2*time.Second)
	require.NotNil(t, state.RoutingSignal)

	assert.Equal(t, "ok", state.QuotaState)
	assert.True(t, state.QuotaOK)
	assert.Equal(t, claudeQuotaSnapshotSourceKind, state.RoutingSignal.CurrentQuota.Source.Kind)
	assert.Equal(t, 61, state.RoutingSignal.CurrentQuota.UsedPercent)
	assert.Equal(t, 125, state.RoutingSignal.HistoricalUsage.TotalTokens)
}

func TestProbeHarnessStateUsesRecentClaudeQuotaFailureOverlay(t *testing.T) {
	r := newTestRunnerForRouting()

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	statsDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(statsDir, 0o755))
	statsPath := filepath.Join(statsDir, "stats-cache.json")
	require.NoError(t, os.WriteFile(statsPath, []byte(`{
		"dailyActivity": [
			{"date":"2026-04-09","sessions":2,"usage":{"input_tokens":100,"cached_input_tokens":5,"output_tokens":20}}
		]
	}`), 0o644))
	t.Setenv("HOME", home)
	t.Setenv(claudeStatsCacheEnv, statsPath)

	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	r.Config.SessionLogDir = logDir

	entry := SessionEntry{
		ID:        "as-test-quota",
		Timestamp: time.Now().UTC(),
		Harness:   "claude",
		Response:  `You've hit your limit · resets 12am (America/New_York)`,
		ExitCode:  1,
		Error:     "agent execution failed",
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "sessions.jsonl"), append(data, '\n'), 0o644))

	state := r.ProbeHarnessState("claude", 2*time.Second)
	require.NotNil(t, state.RoutingSignal)

	assert.Equal(t, "blocked", state.QuotaState)
	assert.False(t, state.QuotaOK)
	assert.Equal(t, "claude", state.RoutingSignal.Provider)
	assert.Equal(t, sessionLogSourceKind, state.RoutingSignal.Source.Kind)
	assert.Equal(t, "recent session failure", state.RoutingSignal.Source.Basis)
	assert.NotEmpty(t, state.RoutingSignal.CurrentQuota.ResetsAt)
	assert.Equal(t, 125, state.RoutingSignal.HistoricalUsage.TotalTokens)

	snapshotPath := filepath.Join(home, ".ddx", "provider-state", "claude-quota.json")
	quota, err := ReadClaudeQuotaSnapshot(snapshotPath, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, "blocked", quota.State)
	assert.Equal(t, claudeQuotaSnapshotSourceKind, quota.Source.Kind)
}

func TestProbeAndBuildCandidatePlansUsesNativeQuotaSignal(t *testing.T) {
	r := newTestRunnerForRouting()

	dir := t.TempDir()
	path := filepath.Join(dir, "codex-session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`+"\n"+
			`{"type":"session.updated","token_count":{"rate_limits":{"primary":{"used_percent":97,"window_minutes":300,"resets_at":"April 12"}}}}`+"\n",
	), 0o644))
	t.Setenv(codexNativeSessionEnv, path)

	plans := r.ProbeAndBuildCandidatePlans(RouteRequest{Profile: "cheap"}, 2*time.Second)

	var codexPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "codex" {
			codexPlan = &plans[i]
			break
		}
	}
	require.NotNil(t, codexPlan)
	assert.False(t, codexPlan.Viable)
	assert.Equal(t, "quota exceeded", codexPlan.RejectReason)
	assert.Equal(t, "blocked", codexPlan.State.QuotaState)
	assert.NotNil(t, codexPlan.State.RoutingSignal)
}
