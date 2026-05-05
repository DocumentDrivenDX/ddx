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

func TestFormatSessionLogLines_LLMPayloadSizes(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "llm.request", map[string]any{
			"attempt_index": 0,
			"messages": []map[string]any{
				{"role": "user", "content": "inspect routing logs"},
			},
		}),
		mustSessionLogLine(t, "llm.response", map[string]any{
			"model":            "claude-sonnet-4-6",
			"latency_ms":       2500,
			"response_bytes":   17,
			"thinking_bytes":   2048,
			"tool_input_bytes": 32,
			"attempt": map[string]any{
				"cost": map[string]any{
					"raw": map[string]any{"total_tokens": 42},
				},
			},
			"tool_calls": []map[string]any{{"name": "Bash"}},
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  → llm request (attempt 0, req=")
	assert.Contains(t, got, "[inspect routing logs]\n")
	assert.Contains(t, got, "  ← llm response (42 tokens, 2.5s, text=17B think=2.0KB tool_in=32B) claude-sonnet-4-6 → Bash\n")
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
	assert.Contains(t, got, "  > ls -al\n")
	assert.Contains(t, got, "  ok ls -al /tmp/project/with/a/very/long/path/that/should/be/truncated/")
	assert.Contains(t, got, " 3s 100tok\n")
	assert.NotContains(t, got, "operators/do/not/need/full/output")
}

func TestFormatSessionLogLines_ProgressToolStripsShellWrapper(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":       "tool",
			"state":       "complete",
			"tool_name":   "bash",
			"command":     `/bin/zsh -lc "go test ./cmd -run TestAgentRouteStatus -count=1"`,
			"duration_ms": 3200,
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":       "tool",
			"state":       "complete",
			"tool_name":   "bash",
			"command":     `{command="/bin/bash -lc \"rg -n \\\"pre-execute\\\" .\""}`,
			"duration_ms": 40,
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  ok go test ./cmd -run TestAgentRouteStatus -count=1 3.2s\n")
	assert.Contains(t, got, "  ok rg -n \"pre-execute\" . 40ms\n")
	assert.NotContains(t, got, "/bin/zsh -lc")
	assert.NotContains(t, got, "/bin/bash -lc")
	assert.NotContains(t, got, "{command=")
}

func TestFormatSessionLogLines_ToolPayloadSizes(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "tool.call", map[string]any{
			"tool":        "Bash",
			"input":       map[string]any{"command": `/bin/zsh -lc "go test ./..."`},
			"input_bytes": 128,
		}),
		mustSessionLogLine(t, "tool.result", map[string]any{
			"tool":         "Bash",
			"output_bytes": 4096,
			"duration_ms":  1200,
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  🔧 Bash go test ./... in=128B\n")
	assert.NotContains(t, got, "/bin/zsh -lc")
	assert.Contains(t, got, "  < Bash out=4.0KB 1.2s\n")
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
