package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTailSessionLogs_ProgressEventsNotReplayed(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, DefaultLogDir)
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	logPath := filepath.Join(logDir, "agent-live.jsonl")
	require.NoError(t, os.WriteFile(logPath, []byte(mustSessionLogLine(t, "progress", map[string]any{
		"phase":   "thinking",
		"state":   "start",
		"message": "ddx-99419bc1 #1 thinking ...",
	})+"\n"), 0o644))

	states := map[string]*fileTrackState{}
	for _, file := range listSessionLogFiles(root, logDir) {
		info, err := os.Stat(file.path)
		require.NoError(t, err)
		states[file.path] = &fileTrackState{offset: info.Size()}
	}

	require.NoError(t, appendTestLogLine(logPath, mustSessionLogLine(t, "progress", map[string]any{
		"phase":         "thinking",
		"state":         "complete",
		"message":       "ddx-99419bc1 #1 thought 5.0s",
		"output_tokens": 8,
		"duration_ms":   5000,
	})+"\n"))

	var out bytes.Buffer
	readNewLogLines(root, logDir, states, &out)
	got := out.String()
	assert.NotContains(t, got, "thinking ...")
	assert.Contains(t, got, "ddx-99419bc1 #1 thought 5.0s")

	out.Reset()
	readNewLogLines(root, logDir, states, &out)
	assert.Empty(t, out.String(), "tailer must not replay lines already emitted")
}

func TestTailSessionLogs_ReadsEmbeddedServiceProgress(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, DefaultLogDir)
	embedded := filepath.Join(root, ".ddx", "executions", "attempt-1", "embedded")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	require.NoError(t, os.MkdirAll(embedded, 0o755))
	svcPath := filepath.Join(embedded, "svc-1.jsonl")
	require.NoError(t, os.WriteFile(svcPath, []byte(mustSessionLogLine(t, "progress", map[string]any{
		"phase":   "route",
		"state":   "start",
		"message": "route codex/gpt-5.4-mini power=8 speed=35.5 cost=catalog",
	})+"\n"), 0o644))

	var out bytes.Buffer
	readNewLogLines(root, logDir, map[string]*fileTrackState{}, &out)
	assert.Contains(t, out.String(), "route codex/gpt-5.4-mini power=8 speed=35.5 cost=catalog")
}

func appendTestLogLine(path, text string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	_, err = f.WriteString(text)
	return err
}
