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

// writeSessionsJSONL writes SessionEntry values to a temp sessions.jsonl and returns its path.
func writeSessionsJSONL(t *testing.T, entries []SessionEntry) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		require.NoError(t, enc.Encode(e))
	}
	return path
}

// makeEntry is a helper to build a SessionEntry for tests.
func makeEntry(id, harness, model string, ts time.Time, inputTok, outputTok int, costUSD float64, durMS int) SessionEntry {
	return SessionEntry{
		ID:           id,
		Timestamp:    ts,
		Harness:      harness,
		Model:        model,
		InputTokens:  inputTok,
		OutputTokens: outputTok,
		CostUSD:      costUSD,
		Duration:     durMS,
	}
}

// aggregateFromFile is a thin wrapper that mirrors the cmd-layer aggregation so tests
// can live in the agent package.
func aggregateFromFile(logFile, harnessFilter string, since time.Time) (map[string]struct {
	Sessions     int
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}, error) {
	// Re-implement a minimal version here so the internal package can test without
	// importing cmd (which would create a cycle).
	type agg struct {
		sessions     int
		inputTokens  int
		outputTokens int
		costUSD      float64
	}

	f, err := os.Open(logFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := map[string]struct {
		Sessions     int
		InputTokens  int
		OutputTokens int
		CostUSD      float64
	}{}

	dec := json.NewDecoder(f)
	for dec.More() {
		var entry SessionEntry
		if err := dec.Decode(&entry); err != nil {
			continue
		}
		if entry.Timestamp.Before(since) {
			continue
		}
		if harnessFilter != "" && entry.Harness != harnessFilter {
			continue
		}
		a := result[entry.Harness]
		a.Sessions++
		a.InputTokens += entry.InputTokens
		a.OutputTokens += entry.OutputTokens
		if entry.CostUSD > 0 {
			a.CostUSD += entry.CostUSD
		} else if entry.Model != "" {
			if est := EstimateCost(entry.Model, entry.InputTokens, entry.OutputTokens); est >= 0 {
				a.CostUSD += est
			}
		}
		result[entry.Harness] = a
	}
	return result, nil
}

// TC-006: 3 codex and 2 claude sessions → correct per-harness counts and totals.
func TestTC006_UsageAggregation(t *testing.T) {
	now := time.Now().UTC()
	entries := []SessionEntry{
		makeEntry("s1", "codex", "o3-mini", now, 10000, 1000, 0, 30000),
		makeEntry("s2", "codex", "o3-mini", now, 20000, 2000, 0, 40000),
		makeEntry("s3", "codex", "o3-mini", now, 30000, 3000, 0, 50000),
		makeEntry("s4", "claude", "claude-sonnet-4-20250514", now, 50000, 4000, 0, 35000),
		makeEntry("s5", "claude", "claude-sonnet-4-20250514", now, 60000, 5000, 0, 45000),
	}
	path := writeSessionsJSONL(t, entries)
	since := now.Add(-time.Hour)

	agg, err := aggregateFromFile(path, "", since)
	require.NoError(t, err)

	require.Contains(t, agg, "codex")
	require.Contains(t, agg, "claude")

	assert.Equal(t, 3, agg["codex"].Sessions)
	assert.Equal(t, 60000, agg["codex"].InputTokens)
	assert.Equal(t, 6000, agg["codex"].OutputTokens)

	assert.Equal(t, 2, agg["claude"].Sessions)
	assert.Equal(t, 110000, agg["claude"].InputTokens)
	assert.Equal(t, 9000, agg["claude"].OutputTokens)

	// Total sessions across harnesses
	total := 0
	for _, a := range agg {
		total += a.Sessions
	}
	assert.Equal(t, 5, total)
}

// TC-007: Sessions spanning multiple days; filter with since=today returns only today's.
func TestTC007_SinceToday(t *testing.T) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.Add(-24 * time.Hour)

	entries := []SessionEntry{
		makeEntry("old1", "codex", "o3-mini", yesterday.Add(-time.Hour), 1000, 100, 0, 10000),
		makeEntry("old2", "codex", "o3-mini", yesterday, 2000, 200, 0, 10000),
		makeEntry("new1", "codex", "o3-mini", now, 3000, 300, 0, 10000),
		makeEntry("new2", "claude", "claude-sonnet-4-20250514", now.Add(-time.Minute), 4000, 400, 0, 10000),
	}
	path := writeSessionsJSONL(t, entries)

	agg, err := aggregateFromFile(path, "", today)
	require.NoError(t, err)

	total := 0
	for _, a := range agg {
		total += a.Sessions
	}
	assert.Equal(t, 2, total, "only today's sessions should be counted")
}

// TC-008: Filter by --harness claude returns only claude sessions.
func TestTC008_HarnessFilter(t *testing.T) {
	now := time.Now().UTC()
	entries := []SessionEntry{
		makeEntry("c1", "codex", "o3-mini", now, 5000, 500, 0, 10000),
		makeEntry("c2", "codex", "o3-mini", now, 5000, 500, 0, 10000),
		makeEntry("cl1", "claude", "claude-sonnet-4-20250514", now, 8000, 800, 0, 20000),
	}
	path := writeSessionsJSONL(t, entries)
	since := now.Add(-time.Hour)

	agg, err := aggregateFromFile(path, "claude", since)
	require.NoError(t, err)

	assert.NotContains(t, agg, "codex", "codex should be filtered out")
	require.Contains(t, agg, "claude")
	assert.Equal(t, 1, agg["claude"].Sessions)
}

// TC-009: Cost estimation from pricing table for a codex session with known model.
func TestTC009_CostEstimation(t *testing.T) {
	now := time.Now().UTC()
	// o3-mini: $1.10/1M input, $4.40/1M output
	// 1,000,000 input + 1,000,000 output = $5.50
	entries := []SessionEntry{
		makeEntry("x1", "codex", "o3-mini", now, 1_000_000, 1_000_000, 0, 30000),
	}
	path := writeSessionsJSONL(t, entries)
	since := now.Add(-time.Hour)

	agg, err := aggregateFromFile(path, "", since)
	require.NoError(t, err)

	require.Contains(t, agg, "codex")
	expected := 1.10 + 4.40 // $5.50
	assert.InDelta(t, expected, agg["codex"].CostUSD, 0.001)
}

// TestEstimateCostKnownModel verifies the pricing function directly.
func TestEstimateCostKnownModel(t *testing.T) {
	cost := EstimateCost("o3-mini", 1_000_000, 1_000_000)
	assert.InDelta(t, 5.50, cost, 0.001)
}

// TestEstimateCostUnknownModel returns -1 for unknown models.
func TestEstimateCostUnknownModel(t *testing.T) {
	assert.Equal(t, -1.0, EstimateCost("unknown-model-xyz", 1000, 1000))
}
