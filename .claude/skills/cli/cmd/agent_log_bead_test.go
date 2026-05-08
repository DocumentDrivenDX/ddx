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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSessionsJSONL(t *testing.T, dir string, entries []agent.SessionEntry) {
	t.Helper()
	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))
	for _, e := range entries {
		require.NoError(t, agent.AppendSessionIndex(logDir, agent.SessionIndexEntryFromLegacy(dir, e), e.Timestamp))
	}
}

func writeResultJSON(t *testing.T, dir, attemptID, outcome string) {
	t.Helper()
	execDir := filepath.Join(dir, ".ddx", "executions", attemptID)
	require.NoError(t, os.MkdirAll(execDir, 0755))
	result := agent.ExecuteBeadResult{
		BeadID:    "test-bead",
		AttemptID: attemptID,
		Outcome:   outcome,
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "result.json"), data, 0644))
}

func runAgentLogCmd(t *testing.T, workDir string, args ...string) (string, error) {
	t.Helper()
	factory := NewCommandFactory(workDir)
	root := factory.NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"agent", "log"}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestAgentLog_BeadFilter_NoSessions(t *testing.T) {
	dir := t.TempDir()
	entries := []agent.SessionEntry{
		{
			ID:          "sess-other",
			Timestamp:   time.Now().Add(-5 * time.Minute),
			Harness:     "claude",
			Correlation: map[string]string{"bead_id": "other-bead"},
		},
	}
	writeSessionsJSONL(t, dir, entries)

	out, err := runAgentLogCmd(t, dir, "--bead", "missing-bead")
	require.NoError(t, err)
	assert.Contains(t, out, "no sessions found for bead missing-bead")
}

func TestAgentLog_BeadFilter_TableOutput(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	entries := []agent.SessionEntry{
		{
			ID:          "sess-aaa11111",
			Timestamp:   now.Add(-10 * time.Minute),
			Harness:     "claude",
			Model:       "claude-3",
			Duration:    5000,
			Tokens:      100,
			CostUSD:     0.0042,
			ExitCode:    0,
			Correlation: map[string]string{"bead_id": "axon-abc123", "attempt_id": "attempt-1"},
		},
		{
			ID:          "sess-bbb22222",
			Timestamp:   now.Add(-5 * time.Minute),
			Harness:     "claude",
			Model:       "claude-3",
			Duration:    7000,
			Tokens:      200,
			CostUSD:     0,
			ExitCode:    1,
			Correlation: map[string]string{"bead_id": "axon-abc123", "attempt_id": "attempt-2"},
		},
		// different bead — should be excluded
		{
			ID:          "sess-ccc33333",
			Timestamp:   now.Add(-2 * time.Minute),
			Harness:     "claude",
			Correlation: map[string]string{"bead_id": "other-bead"},
		},
	}
	writeSessionsJSONL(t, dir, entries)
	writeResultJSON(t, dir, "attempt-1", "merged")
	writeResultJSON(t, dir, "attempt-2", "error")

	out, err := runAgentLogCmd(t, dir, "--bead", "axon-abc123")
	require.NoError(t, err)

	// Header row
	assert.Contains(t, out, "ATTEMPT")
	assert.Contains(t, out, "STARTED")
	assert.Contains(t, out, "HARNESS")
	assert.Contains(t, out, "OUTCOME")
	assert.Contains(t, out, "SESSION")

	// Outcomes
	assert.Contains(t, out, "merged")
	assert.Contains(t, out, "error")

	// Cost formatting
	assert.Contains(t, out, "$0.0042")
	assert.Contains(t, out, "local")

	// Session ID truncated to 8 chars (sess-aaa11111[:8] = "sess-aaa", sess-bbb22222[:8] = "sess-bbb")
	assert.Contains(t, out, "sess-aaa")
	assert.Contains(t, out, "sess-bbb")

	// Summary line
	assert.Contains(t, out, "axon-abc123")
	assert.Contains(t, out, "2 attempts")
	assert.Contains(t, out, "1 merged")
	assert.Contains(t, out, "1 errors")
}

func TestAgentLog_BeadFilter_OutcomeFallback(t *testing.T) {
	dir := t.TempDir()
	entries := []agent.SessionEntry{
		{
			ID:          "sess-ok000000",
			Timestamp:   time.Now().Add(-3 * time.Minute),
			Harness:     "claude",
			ExitCode:    0,
			Correlation: map[string]string{"bead_id": "bead-x"},
		},
		{
			ID:          "sess-fail0000",
			Timestamp:   time.Now().Add(-1 * time.Minute),
			Harness:     "claude",
			ExitCode:    1,
			Correlation: map[string]string{"bead_id": "bead-x"},
		},
	}
	writeSessionsJSONL(t, dir, entries)
	// No result.json written — should fall back to exit code

	out, err := runAgentLogCmd(t, dir, "--bead", "bead-x")
	require.NoError(t, err)
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "error")
}

func TestAgentLog_BeadFilter_JSON(t *testing.T) {
	dir := t.TempDir()
	entries := []agent.SessionEntry{
		{
			ID:          "sess-json1234",
			Timestamp:   time.Now().Add(-2 * time.Minute),
			Harness:     "claude",
			ExitCode:    0,
			Correlation: map[string]string{"bead_id": "bead-j", "attempt_id": "att-j"},
		},
	}
	writeSessionsJSONL(t, dir, entries)
	writeResultJSON(t, dir, "att-j", "preserved")

	out, err := runAgentLogCmd(t, dir, "--bead", "bead-j", "--json")
	require.NoError(t, err)

	var results []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "preserved", results[0]["outcome"])
	assert.Equal(t, "sess-json1234", results[0]["id"])
}

func TestAgentLog_DefaultBehavior_Unchanged(t *testing.T) {
	dir := t.TempDir()
	entries := []agent.SessionEntry{
		{
			ID:        "sess-default1",
			Timestamp: time.Now().Add(-1 * time.Minute),
			Harness:   "claude",
			Duration:  1000,
			Tokens:    50,
			ExitCode:  0,
		},
	}
	writeSessionsJSONL(t, dir, entries)

	out, err := runAgentLogCmd(t, dir)
	require.NoError(t, err)
	assert.Contains(t, out, "sess-defa")
	// No table header — default mode
	assert.NotContains(t, out, "ATTEMPT")
}

func TestAgentLogReindexMigratesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	entries := []agent.SessionEntry{
		{ID: "sess-jan", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Harness: "agent"},
		{ID: "sess-feb", Timestamp: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Harness: "agent"},
	}
	f, err := os.Create(filepath.Join(logDir, "sessions.jsonl"))
	require.NoError(t, err)
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		require.NoError(t, enc.Encode(entry))
	}
	require.NoError(t, f.Close())

	out, err := runAgentLogCmd(t, dir, "reindex")
	require.NoError(t, err)
	assert.Contains(t, out, "indexed 2 legacy sessions")
	_, err = os.Stat(filepath.Join(logDir, "sessions.jsonl.legacy"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(logDir, "sessions", "sessions-2026-01.jsonl"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(logDir, "sessions", "sessions-2026-02.jsonl"))
	require.NoError(t, err)

	out, err = runAgentLogCmd(t, dir, "reindex")
	require.NoError(t, err)
	assert.Contains(t, out, "indexed 0 legacy sessions")
}

func TestAgentLogFormatElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m"},
		{90 * time.Minute, "1h"},
		{25 * time.Hour, "1d"},
		{0, "0s"},
		{-5 * time.Second, "0s"},
	}
	for _, tc := range cases {
		got := agentLogFormatElapsed(tc.d)
		assert.Equal(t, tc.want, got, "duration=%v", tc.d)
	}
}
