package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatSessionLogLines_RendersFizeauProgressEvents(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":   "thinking",
			"state":   "start",
			"message": "ddx-99419bc1 #1 thinking ...",
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":         "thinking",
			"state":         "complete",
			"message":       "ddx-99419bc1 #1 thought 5.0s",
			"output_tokens": 8,
			"duration_ms":   5000,
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":       "tool",
			"state":       "start",
			"message":     "ddx-99419bc1 #1 tool `ls -al` start",
			"tool_name":   "shell",
			"command":     "ls -al",
			"duration_ms": 0,
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":        "tool",
			"state":        "complete",
			"message":      "ddx-99419bc1 #1 tool `ls -al` done 3.0s",
			"command":      "ls -al",
			"duration_ms":  3000,
			"total_tokens": 100,
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":        "response",
			"state":        "complete",
			"message":      "ddx-99419bc1 #1 done 100tok",
			"total_tokens": 100,
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":           "context",
			"state":           "update",
			"session_summary": "edited routing output and verified tests",
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  ddx-99419bc1 #1 thinking ...\n")
	assert.Contains(t, got, "  ddx-99419bc1 #1 thought 5.0s\n")
	assert.Contains(t, got, "  ddx-99419bc1 #1 tool `ls -al` start\n")
	assert.Contains(t, got, "  ddx-99419bc1 #1 tool `ls -al` done 3.0s\n")
	assert.Contains(t, got, "  ddx-99419bc1 #1 done 100tok\n")
	assert.Contains(t, got, "  context: edited routing output and verified tests\n")
	assert.NotContains(t, got, "running tool call")
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
