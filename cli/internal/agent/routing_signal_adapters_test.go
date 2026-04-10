package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCodexNativeSignals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codex-session.jsonl")
	now := time.Date(2026, 4, 10, 15, 0, 0, 0, time.UTC)
	mtime := time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC)

	content := "" +
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":10}}` + "\n" +
		`{"type":"session.updated","token_count":{"rate_limits":{"primary":{"used_percent":83,"window_minutes":300,"resets_at":"April 12"}}}}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":75,"output_tokens":5}}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	require.NoError(t, os.Chtimes(path, mtime, mtime))

	signals, err := ReadCodexNativeSignals(path, now)
	require.NoError(t, err)

	assert.Equal(t, "codex", signals.CurrentQuota.Source.Provider)
	assert.Equal(t, codexNativeSessionSourceKind, signals.CurrentQuota.Source.Kind)
	assert.Equal(t, path, signals.CurrentQuota.Source.Path)
	assert.Equal(t, "fresh", signals.CurrentQuota.Source.Freshness)
	assert.Equal(t, mtime.UTC(), signals.CurrentQuota.Source.ObservedAt)
	assert.Equal(t, int64(30*60), signals.CurrentQuota.Source.AgeSeconds)
	assert.Equal(t, "native session jsonl", signals.CurrentQuota.Source.Basis)

	assert.Equal(t, "ok", signals.CurrentQuota.State)
	assert.Equal(t, 83, signals.CurrentQuota.UsedPercent)
	assert.Equal(t, 300, signals.CurrentQuota.WindowMinutes)
	assert.Equal(t, "April 12", signals.CurrentQuota.ResetsAt)

	assert.Equal(t, 175, signals.RecentUsage.InputTokens)
	assert.Equal(t, 25, signals.RecentUsage.CachedInputTokens)
	assert.Equal(t, 15, signals.RecentUsage.OutputTokens)
	assert.Equal(t, 215, signals.RecentUsage.TotalTokens)
	assert.Equal(t, 2, signals.RecentUsage.SessionCount)
}

func TestReadClaudeNativeSignalsHistoricalUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats-cache.json")
	now := time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
	mtime := time.Date(2026, 4, 10, 16, 0, 0, 0, time.UTC)

	content := `{
		"dailyActivity": [
			{"date":"2026-04-09","sessions":2,"usage":{"input_tokens":100,"cached_input_tokens":5,"output_tokens":20}},
			{"date":"2026-04-10","sessions":1,"usage":{"inputTokens":30,"outputTokens":10}}
		],
		"modelUsage": {
			"claude-sonnet-4-6": {"inputTokens":500,"outputTokens":70,"cachedInputTokens":12,"sessionCount":3},
			"claude-opus-4": {"usage":{"input_tokens":250,"output_tokens":40},"sessionCount":1}
		}
	}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	require.NoError(t, os.Chtimes(path, mtime, mtime))

	signals, err := ReadClaudeNativeSignals(path, now)
	require.NoError(t, err)

	assert.Equal(t, "claude", signals.HistoricalUsage.Source.Provider)
	assert.Equal(t, claudeStatsCacheSourceKind, signals.HistoricalUsage.Source.Kind)
	assert.Equal(t, path, signals.HistoricalUsage.Source.Path)
	assert.Equal(t, "cached", signals.HistoricalUsage.Source.Freshness)
	assert.Equal(t, mtime.UTC(), signals.HistoricalUsage.Source.ObservedAt)
	assert.Equal(t, int64(2*60*60), signals.HistoricalUsage.Source.AgeSeconds)
	assert.Equal(t, "stats-cache.json", signals.HistoricalUsage.Source.Basis)

	assert.Equal(t, "unknown", signals.CurrentQuota.State)
	assert.Equal(t, claudeUnknownQuotaSourceKind, signals.CurrentQuota.Source.Kind)
	assert.Equal(t, "unknown", signals.CurrentQuota.Source.Freshness)
	assert.Contains(t, signals.CurrentQuota.Source.Basis, "no stable non-PTY")

	require.Len(t, signals.ByModel, 2)
	require.Len(t, signals.ByDay, 2)

	assert.Equal(t, 750, signals.HistoricalUsage.InputTokens)
	assert.Equal(t, 12, signals.HistoricalUsage.CachedInputTokens)
	assert.Equal(t, 110, signals.HistoricalUsage.OutputTokens)
	assert.Equal(t, 872, signals.HistoricalUsage.TotalTokens)
	assert.Equal(t, 4, signals.HistoricalUsage.SessionCount)
}

func TestReadClaudeNativeSignalsUnknownQuotaOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats-cache.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o644))

	signals, err := ReadClaudeNativeSignals(path, time.Now().UTC())
	require.NoError(t, err)

	assert.Equal(t, "unknown", signals.CurrentQuota.State)
	assert.Equal(t, claudeUnknownQuotaSourceKind, signals.CurrentQuota.Source.Kind)
	assert.Equal(t, "unknown", signals.CurrentQuota.Source.Freshness)
	assert.Equal(t, "current quota/headroom remains unknown", signals.CurrentQuota.Source.Notes)
}
