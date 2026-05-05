package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

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
	assert.Contains(t, got, "  thinking complete 8 tok in 5s, 1.6 tok/s\n")
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
	assert.Contains(t, got, "  ← llm response (42 tokens, 2.5s, 16.8 tok/s, text=17B think=2.0KB tool_in=32B) claude-sonnet-4-6 → Bash\n")
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
			"phase":          "tool",
			"state":          "complete",
			"tool_name":      "shell",
			"command":        "ls -al /tmp/project/with/a/very/long/path/that/should/be/truncated/because/operators/do/not/need/full/output",
			"output_summary": "out=57B 3 lines \"README.md\"",
			"duration_ms":    3000,
			"total_tokens":   100,
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  > ls -al\n")
	assert.Contains(t, got, "  ok ls -al …/output")
	assert.Contains(t, got, " < out=57B 3 lines \"README.md\"")
	assert.Contains(t, got, " 3s 100tok, 33.3 tok/s\n")
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
	assert.Contains(t, got, "  ok test ./cmd -run TestAgentRouteStatus -count=1 3.2s\n")
	assert.Contains(t, got, "  ok search \"pre-execute\" in . 40ms\n")
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
			"tool":           "Bash",
			"output_bytes":   4096,
			"output_excerpt": "2 lines \"ok package\"",
			"duration_ms":    1200,
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "  🔧 Bash go test ./... in=128B\n")
	assert.NotContains(t, got, "/bin/zsh -lc")
	assert.Contains(t, got, "  < Bash out=4.0KB 2 lines \"ok package\" 1.2s\n")
}

func TestFormatSessionLogLines_ToolOutputCorpusStaysBounded(t *testing.T) {
	longSecretTail := "FULL_OUTPUT_SHOULD_NOT_APPEAR_" + strings.Repeat("x", 200)
	longCommandTail := "FULL_COMMAND_SHOULD_NOT_APPEAR_" + strings.Repeat("y", 200)
	cases := []struct {
		name        string
		line        string
		want        []string
		notWant     []string
		maxLineLen  int
		expectLines int
	}{
		{
			name: "progress complete with long command and output summary",
			line: mustSessionLogLine(t, "progress", map[string]any{
				"phase":          "tool",
				"state":          "complete",
				"tool_name":      "bash",
				"command":        "sed -n '240,320p' service_execute.go && " + longCommandTail,
				"output_summary": "out=8.3KB 81 lines \"func runExecute(ctx context.Context, req ServiceExecuteRequest, decision RouteDecision)\"",
				"duration_ms":    1450,
			}),
			want:       []string{"ok inspect 240,320p in service_execute.go", "< out=8.3KB 81 lines", "1.45s"},
			notWant:    []string{longCommandTail, "FULL_COMMAND_SHOULD_NOT_APPEAR"},
			maxLineLen: 122,
		},
		{
			name: "tool result with explicit excerpt",
			line: mustSessionLogLine(t, "tool.result", map[string]any{
				"tool":           "Bash",
				"output_bytes":   987654,
				"output_excerpt": "120 lines \"--- FAIL: TestWorkerLogContext (0.03s)\"",
				"duration_ms":    2300,
			}),
			want:       []string{"< Bash out=964.5KB", "120 lines", "TestWorkerLogContext", "2.3s"},
			maxLineLen: 122,
		},
		{
			name: "verbose raw output is summarized without leaking tail",
			line: mustSessionLogLine(t, "tool.result", map[string]any{
				"tool":         "Bash",
				"output":       "first useful line\nsecond line\n" + longSecretTail,
				"output_bytes": 4096,
				"duration_ms":  10,
			}),
			want:       []string{"< Bash out=4.0KB", "3 lines", "\"first useful line\"", "10ms"},
			notWant:    []string{longSecretTail, "FULL_OUTPUT_SHOULD_NOT_APPEAR", "second line"},
			maxLineLen: 122,
		},
		{
			name: "empty output keeps byte context",
			line: mustSessionLogLine(t, "tool.result", map[string]any{
				"tool":         "read",
				"output_bytes": 0,
				"duration_ms":  1,
			}),
			want:       []string{"< read out=0B 1ms"},
			maxLineLen: 122,
		},
		{
			name: "failed result preserves failure without full error body",
			line: mustSessionLogLine(t, "tool.result", map[string]any{
				"tool":           "Bash",
				"output_bytes":   24,
				"output_excerpt": "1 line \"permission denied\"",
				"duration_ms":    5,
				"error":          "permission denied: " + longSecretTail,
			}),
			want:       []string{"< Bash out=24B", "permission denied", "5ms failed"},
			notWant:    []string{longSecretTail, "FULL_OUTPUT_SHOULD_NOT_APPEAR"},
			maxLineLen: 122,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatSessionLogLines([]string{tc.line})
			require.NotEmpty(t, got)
			for _, want := range tc.want {
				assert.Contains(t, got, want)
			}
			for _, notWant := range tc.notWant {
				assert.NotContains(t, got, notWant)
			}
			assertFormattedLinesBounded(t, got, tc.maxLineLen)
		})
	}
}

func TestFormatSessionLogLines_MixedSessionCorpusKeepsContextAndTrim(t *testing.T) {
	hugeOutput := strings.Join([]string{
		"package agent",
		"func TestFormatSessionLogLines_MixedSessionCorpusKeepsContextAndTrim(t *testing.T) {",
		strings.Repeat("raw-log-body-", 80),
	}, "\n")
	lines := []string{
		mustSessionLogLine(t, "session.start", map[string]any{"model": "claude-sonnet-4-6"}),
		mustSessionLogLine(t, "llm.request", map[string]any{
			"attempt_index": 2,
			"messages": []map[string]any{
				{"role": "user", "content": "Please inspect the worker log formatter and make output lines useful."},
			},
		}),
		mustSessionLogLine(t, "llm.response", map[string]any{
			"model":            "claude-sonnet-4-6",
			"latency_ms":       4100,
			"response_bytes":   71,
			"thinking_bytes":   2048,
			"tool_input_bytes": 512,
			"attempt": map[string]any{
				"cost": map[string]any{"raw": map[string]any{"total_tokens": 1234}},
			},
			"tool_calls": []map[string]any{{"name": "Bash"}},
		}),
		mustSessionLogLine(t, "tool.call", map[string]any{
			"tool":        "Bash",
			"input":       map[string]any{"command": `/bin/zsh -lc "sed -n '240,320p' cli/internal/agent/session_log_format.go"`},
			"input_bytes": 95,
		}),
		mustSessionLogLine(t, "tool.result", map[string]any{
			"tool":         "Bash",
			"output":       hugeOutput,
			"output_bytes": len([]byte(hugeOutput)),
			"duration_ms":  7,
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":           "context",
			"state":           "update",
			"session_summary": "read formatter, added compact output hints, verified focused tests",
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "▶ session started (model: claude-sonnet-4-6)")
	assert.Contains(t, got, "llm request (attempt 2")
	assert.Contains(t, got, "sed -n '240,320p' …/session_log_format.go")
	assert.Contains(t, got, "3 lines \"package agent\"")
	assert.Contains(t, got, "context summary: read formatter")
	assert.NotContains(t, got, strings.Repeat("raw-log-body-", 10))

	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.Contains(line, "  < ") || strings.Contains(line, "  ok ") || strings.Contains(line, "  > ") {
			assert.LessOrEqual(t, utf8.RuneCountInString(line), 122, "tool line should stay compact: %q", line)
		}
	}
}

func TestFormatSessionLogLines_SourceShapeSamplesStayUseful(t *testing.T) {
	longTail := strings.Repeat("verbose-output-", 80)
	cases := []struct {
		name    string
		lines   []string
		want    []string
		notWant []string
	}{
		{
			name: "claude stream tool call and result",
			lines: []string{
				mustSessionLogLine(t, "session.start", map[string]any{
					"model":   "claude-sonnet-4-6",
					"harness": "claude",
				}),
				mustSessionLogLine(t, "tool.call", map[string]any{
					"tool":        "Bash",
					"input":       map[string]any{"command": `/bin/zsh -lc "sed -n '240,320p' cli/internal/agent/session_log_format.go"`},
					"input_bytes": 95,
				}),
				mustSessionLogLine(t, "tool.result", map[string]any{
					"tool":           "Bash",
					"output_bytes":   6400,
					"output_excerpt": "74 lines \"func formatToolProgressLine(state string, data map[string]any) string\"",
					"duration_ms":    92,
				}),
			},
			want: []string{
				"▶ session started (model: claude-sonnet-4-6)",
				"🔧 Bash sed -n '240,320p' …/session_log_format.go",
				"< Bash out=6.2KB 74 lines",
				"92ms",
			},
		},
		{
			name: "codex progress has task round action and target",
			lines: []string{
				mustSessionLogLine(t, "progress", map[string]any{
					"phase":          "tool",
					"state":          "complete",
					"task_id":        "ddx-1234",
					"turn_index":     22,
					"tool_name":      "apply_patch",
					"action":         "add test implementation",
					"target":         "cli/internal/file.go",
					"command":        "apply_patch",
					"output_bytes":   312,
					"output_lines":   12,
					"output_excerpt": "Success. Updated the following files:",
					"duration_ms":    35,
					"tok_per_sec":    18.4,
				}),
			},
			want: []string{
				"ok ddx-1234 22 add test implementation to cli/internal/file.go",
				"< out=312B 12 lines",
				"35ms",
				"18.4 tok/s",
			},
		},
		{
			name: "native agent raw progress summarizes output and keeps filename",
			lines: []string{
				mustSessionLogLine(t, "progress", map[string]any{
					"phase":          "tool",
					"state":          "complete",
					"task_id":        "ddx-native",
					"turn_index":     3,
					"tool_name":      "bash",
					"command":        "rg -n \"FormatSessionLogLines\" cli/internal/agent/session_log_format_test.go",
					"output_summary": "out=1.5KB 18 lines \"13:func TestFormatSessionLogLines_ProgressThinking(t *testing.T)\"",
					"duration_ms":    11,
				}),
			},
			want: []string{
				"ok ddx-native 3 search \"FormatSessionLogLines\" in …/session_log_format_test.go",
				"< out=1.5KB 18 lines",
				"11ms",
			},
		},
		{
			name: "native tool result with verbose raw output trims body",
			lines: []string{
				mustSessionLogLine(t, "tool.result", map[string]any{
					"tool":         "bash",
					"output":       "ok github.com/DocumentDrivenDX/ddx/internal/agent 0.014s\n" + longTail,
					"output_bytes": 4096,
					"duration_ms":  1400,
				}),
			},
			want: []string{
				"< bash out=4.0KB 2 lines \"ok github.com/DocumentDrivenDX/ddx/internal/age…\"",
				"1.4s",
			},
			notWant: []string{strings.Repeat("verbose-output-", 10)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatSessionLogLines(tc.lines)
			require.NotEmpty(t, got)
			for _, want := range tc.want {
				assert.Contains(t, got, want)
			}
			for _, notWant := range tc.notWant {
				assert.NotContains(t, got, notWant)
			}
			assertFormattedLinesBounded(t, got, 122)
		})
	}
}

func TestFormatSessionLogLines_ProgressTurnsCountUp(t *testing.T) {
	lines := []string{
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":      "tool",
			"state":      "complete",
			"task_id":    "ddx-1234",
			"turn_index": 21,
			"tool_name":  "bash",
			"command":    "sed -n '1,80p' cli/internal/agent/session_log_format.go",
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":          "tool",
			"state":          "complete",
			"task_id":        "ddx-1234",
			"turn_index":     22,
			"tool_name":      "apply_patch",
			"action":         "add test implementation",
			"target":         "cli/internal/agent/session_log_format_test.go",
			"output_summary": "out=288B 9 lines \"Success. Updated the following files:\"",
		}),
		mustSessionLogLine(t, "progress", map[string]any{
			"phase":      "tool",
			"state":      "complete",
			"task_id":    "ddx-1234",
			"turn_index": 23,
			"tool_name":  "bash",
			"command":    "go test ./internal/agent -run TestFormatSessionLogLines",
		}),
	}

	got := FormatSessionLogLines(lines)
	assert.Contains(t, got, "ok ddx-1234 21 inspect 1,80p in …/session_log_format.go")
	assert.Contains(t, got, "ok ddx-1234 22 add test implementation to …/session_log_format_test.go")
	assert.Contains(t, got, "ok ddx-1234 23 test ./internal/agent -run TestFormatSessionLogLines")
	assert.Less(t, strings.Index(got, "ddx-1234 21"), strings.Index(got, "ddx-1234 22"))
	assert.Less(t, strings.Index(got, "ddx-1234 22"), strings.Index(got, "ddx-1234 23"))
	assertFormattedLinesBounded(t, got, 122)
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
	assert.Contains(t, got, "  response complete 100 tok in 4.2s, 23.8 tok/s\n")
	assert.Contains(t, got, "  context summary: edited routing output and verified tests\n")
	assert.Contains(t, got, "  compaction complete 4800 -> 1200 tokens: trimmed prompt history and preserved tool outputs\n")
}

func assertFormattedLinesBounded(t *testing.T, got string, maxLen int) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		assert.LessOrEqual(t, utf8.RuneCountInString(line), maxLen, "formatted line too long: %q", line)
	}
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
