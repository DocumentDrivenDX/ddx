package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatSessionLogLines_ProgressThinking(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":   "thinking",
			"state":   "start",
			"message": "ddx-99419bc1 #1 thinking ...",
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":           "thinking",
			"state":           "update",
			"message":         "ddx-99419bc1 #1 thinking update: reviewing files",
			"session_summary": "reviewing files",
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":         "thinking",
			"state":         "complete",
			"message":       "ddx-99419bc1 #1 thought 5.0s",
			"output_tokens": 8,
			"duration_ms":   5000,
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  thinking ...\n")
	assert.Contains(t, got, "  thinking update: reviewing files\n")
	assert.Contains(t, got, "  thinking complete 8 tok in 5s\n")
}

func TestFormatSessionLogLines_ProgressTool(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":     "tool",
			"state":     "start",
			"tool_name": "shell",
			"command":   "ls -al",
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":        "tool",
			"state":        "complete",
			"tool_name":    "shell",
			"command":      "ls -al /tmp/project/with/a/very/long/path/that/should/be/truncated/because/operators/do/not/need/full/output",
			"duration_ms":  3000,
			"total_tokens": 100,
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  running tool call `ls -al` ...\n")
	assert.Contains(t, got, "  tool call `ls -al /tmp/project/with/a/very/long/path/that/should/be/truncated/")
	assert.Contains(t, got, " completed in 3s, 100 tok\n")
	assert.NotContains(t, got, "operators/do/not/need/full/output")
}

func TestFormatSessionLogLines_ProgressResponseAndContext(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":        "response",
			"state":        "complete",
			"message":      "ddx-99419bc1 #1 done 100tok",
			"total_tokens": 100,
			"duration_ms":  4200,
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":           "context",
			"state":           "update",
			"message":         "ddx-99419bc1 #1 context edited routing output and verified tests",
			"session_summary": "edited routing output and verified tests",
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":           "compaction",
			"state":           "complete",
			"tokens_before":   4800,
			"tokens_after":    1200,
			"session_summary": "trimmed prompt history and preserved tool outputs",
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  response complete 100 tok in 4.2s\n")
	assert.Contains(t, got, "  context summary: edited routing output and verified tests\n")
	assert.Contains(t, got, "  compaction complete 4800 -> 1200 tokens: trimmed prompt history and preserved tool outputs\n")
}

func mustSessionLogLine(t *testing.T, typ string, data map[string]any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"type": typ,
		"data": data,
	})
	require.NoError(t, err)
	return string(body)
}
