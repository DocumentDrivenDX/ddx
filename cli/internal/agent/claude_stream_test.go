package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseClaudeStream feeds synthetic stream-json events through the
// parser and asserts:
//
//  1. Progress lines (session.start, llm.response, tool.call) are emitted
//     for relevant events so the existing TailSessionLogs pipeline can
//     render them.
//  2. The final aggregated result captures the text, tokens, cost, and
//     session id from the authoritative "result" event.
//  3. The parser stays lenient in the face of garbage lines.
//
// The table-driven form exercises multiple event shapes so regressions in
// one case don't hide breakage in another.
func TestParseClaudeStream(t *testing.T) {
	cases := []struct {
		name              string
		input             string
		wantTurnCount     int
		wantToolCalls     int
		wantInputTokens   int
		wantOutputTokens  int
		wantCostUSD       float64
		wantFinalText     string
		wantSessionID     string
		wantModel         string
		wantProgressTypes []string
	}{
		{
			name: "full stream with tool use and result",
			input: strings.Join([]string{
				`{"type":"system","subtype":"init","session_id":"sess-abc","model":"claude-sonnet-4-6","tools":["Bash","Read"]}`,
				`{"type":"assistant","message":{"id":"m-1","model":"claude-sonnet-4-6","content":[{"type":"text","text":"Starting"},{"type":"tool_use","id":"tu-1","name":"Bash","input":{"command":"ls"}}],"usage":{"input_tokens":120,"output_tokens":42}}}`,
				`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu-1","content":"README.md\nfoo.go"}]}}`,
				`{"type":"assistant","message":{"id":"m-2","model":"claude-sonnet-4-6","content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":260,"output_tokens":88}}}`,
				`{"type":"result","subtype":"success","is_error":false,"duration_ms":1200,"result":"All done.","usage":{"input_tokens":260,"output_tokens":88},"total_cost_usd":0.0123,"session_id":"sess-abc"}`,
			}, "\n"),
			wantTurnCount:     2,
			wantToolCalls:     1,
			wantInputTokens:   260,
			wantOutputTokens:  88,
			wantCostUSD:       0.0123,
			wantFinalText:     "All done.",
			wantSessionID:     "sess-abc",
			wantModel:         "claude-sonnet-4-6",
			wantProgressTypes: []string{"session.start", "llm.response", "tool.call", "llm.response"},
		},
		{
			name: "garbage lines are skipped",
			input: strings.Join([]string{
				`not json`,
				`{"type":"system","subtype":"init","session_id":"sess-xyz","model":"claude-sonnet-4-6"}`,
				`{garbage`,
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu-2","name":"Read","input":{"path":"/tmp/x"}}],"usage":{"input_tokens":10,"output_tokens":5}}}`,
				`{"type":"result","subtype":"success","result":"ok","usage":{"input_tokens":10,"output_tokens":5},"total_cost_usd":0.001,"session_id":"sess-xyz"}`,
			}, "\n"),
			wantTurnCount:     1,
			wantToolCalls:     1,
			wantInputTokens:   10,
			wantOutputTokens:  5,
			wantCostUSD:       0.001,
			wantFinalText:     "ok",
			wantSessionID:     "sess-xyz",
			wantModel:         "claude-sonnet-4-6",
			wantProgressTypes: []string{"session.start", "llm.response", "tool.call"},
		},
		{
			name: "text-only assistant without tool_use still emits progress",
			input: strings.Join([]string{
				`{"type":"system","subtype":"init","session_id":"sess-t","model":"claude-sonnet-4-6"}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":3,"output_tokens":2}}}`,
				`{"type":"result","subtype":"success","result":"hello","usage":{"input_tokens":3,"output_tokens":2},"total_cost_usd":0.0001,"session_id":"sess-t"}`,
			}, "\n"),
			wantTurnCount:     1,
			wantToolCalls:     0,
			wantInputTokens:   3,
			wantOutputTokens:  2,
			wantCostUSD:       0.0001,
			wantFinalText:     "hello",
			wantSessionID:     "sess-t",
			wantModel:         "claude-sonnet-4-6",
			wantProgressTypes: []string{"session.start", "llm.response"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var progressBuf bytes.Buffer
			start := time.Now().Add(-3 * time.Second) // exercise elapsed_ms > 0
			res, err := parseClaudeStream(strings.NewReader(tc.input), &progressBuf, "sess-test", "ddx-12345678", start)
			require.NoError(t, err)
			require.NotNil(t, res)

			assert.Equal(t, tc.wantTurnCount, res.TurnCount, "turn count")
			assert.Equal(t, tc.wantToolCalls, res.ToolCalls, "tool call count")
			assert.Equal(t, tc.wantInputTokens, res.InputTokens, "input tokens")
			assert.Equal(t, tc.wantOutputTokens, res.OutputTokens, "output tokens")
			assert.InDelta(t, tc.wantCostUSD, res.CostUSD, 1e-9, "cost usd")
			assert.Equal(t, tc.wantFinalText, res.FinalText, "final text")
			assert.Equal(t, tc.wantSessionID, res.SessionID, "session id")
			assert.Equal(t, tc.wantModel, res.Model, "model")
			assert.False(t, res.IsError)

			// Walk the emitted progress lines and confirm the expected types
			// appear in order.
			var gotTypes []string
			for _, line := range strings.Split(strings.TrimSpace(progressBuf.String()), "\n") {
				if line == "" {
					continue
				}
				var entry map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &entry), "progress line must be valid JSON: %s", line)
				t, _ := entry["type"].(string)
				gotTypes = append(gotTypes, t)
			}
			assert.Equal(t, tc.wantProgressTypes, gotTypes, "progress event types (in order)")

			// Spot-check that at least one tool.call line carries the bead_id
			// so execute-bead operators can grep the log stream.
			if tc.wantToolCalls > 0 {
				assert.Contains(t, progressBuf.String(), `"bead_id":"ddx-12345678"`)
				assert.Contains(t, progressBuf.String(), `"tool.call"`)
			}
		})
	}
}

// TestParseClaudeStreamEmpty verifies the parser tolerates an empty stream
// (e.g. claude crashed before producing any events) and returns an empty but
// non-nil result rather than panicking.
func TestParseClaudeStreamEmpty(t *testing.T) {
	res, err := parseClaudeStream(strings.NewReader(""), nil, "sess-empty", "ddx-0", time.Now())
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 0, res.TurnCount)
	assert.Equal(t, 0, res.InputTokens)
	assert.Empty(t, res.FinalText)
}

// TestClaudeStreamArgsUnsupported ensures the stderr-detection helper that
// drives fallback-to-legacy-args recognises the phrases we care about.
func TestClaudeStreamArgsUnsupported(t *testing.T) {
	cases := []struct {
		stderr string
		want   bool
	}{
		{"error: unknown option '--output-format'", true},
		{"Error: unrecognized option --verbose", true},
		{"error: Invalid value for --output-format: stream-json", true},
		{"Usage: claude [options]\n\nerror: unknown argument", true},
		{"error: unknown flag: --output-format", true},
		{"rate limit exceeded", false},
		{"", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, claudeStreamArgsUnsupported(tc.stderr), tc.stderr)
	}
}

// TestExtractUsageClaudeStreamJSON confirms the legacy extractor now also
// handles stream-json output (the final "type":"result" line).
func TestExtractUsageClaudeStreamJSON(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"result","subtype":"success","result":"hi","usage":{"input_tokens":7,"output_tokens":9},"total_cost_usd":0.0042}`,
	}, "\n")
	usage := ExtractUsage("claude", stream)
	assert.Equal(t, 7, usage.InputTokens)
	assert.Equal(t, 9, usage.OutputTokens)
	assert.InDelta(t, 0.0042, usage.CostUSD, 1e-9)

	// And the text extractor pulls the final result text.
	assert.Equal(t, "hi", ExtractOutput("claude", stream))
}

// TestResolveClaudeProgressLogDir documents the precedence rule that fixes
// the claude/execute-bead parity gap: the per-run override wins over the
// runner-wide config so execute-bead can redirect the JSONL trace into the
// execution bundle's embedded/ dir.
func TestResolveClaudeProgressLogDir(t *testing.T) {
	cases := []struct {
		name string
		opts RunOptions
		cfg  Config
		want string
	}{
		{
			name: "opts override wins over config",
			opts: RunOptions{SessionLogDir: "/bundle/embedded"},
			cfg:  Config{SessionLogDir: "/runner/default"},
			want: "/bundle/embedded",
		},
		{
			name: "falls back to config when opts empty",
			opts: RunOptions{},
			cfg:  Config{SessionLogDir: "/runner/default"},
			want: "/runner/default",
		},
		{
			name: "empty when both unset",
			opts: RunOptions{},
			cfg:  Config{},
			want: "",
		},
		{
			name: "opts override wins even when config empty",
			opts: RunOptions{SessionLogDir: "/bundle/embedded"},
			cfg:  Config{},
			want: "/bundle/embedded",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveClaudeProgressLogDir(tc.opts, tc.cfg))
		})
	}
}

// writeFakeClaudeBinary creates a shell script that mimics the claude CLI's
// stream-json output. The script ignores all arguments and prints a minimal
// but complete sequence of stream events so runClaudeStreaming has real
// bytes to parse and the progress log file ends up with content.
func writeFakeClaudeBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-claude")
	script := `#!/bin/sh
cat <<'EOF'
{"type":"system","subtype":"init","session_id":"sess-fake","model":"claude-sonnet-4-6"}
{"type":"assistant","message":{"id":"m-1","model":"claude-sonnet-4-6","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":5,"output_tokens":2}}}
{"type":"result","subtype":"success","is_error":false,"duration_ms":10,"result":"hello","usage":{"input_tokens":5,"output_tokens":2},"total_cost_usd":0.0001,"session_id":"sess-fake"}
EOF
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// findClaudeProgressLog locates the agent-*.jsonl trace file that
// runClaudeStreaming writes. Returns the path to the single matching file.
func findClaudeProgressLog(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "agent-") && strings.HasSuffix(name, ".jsonl") {
			found = append(found, filepath.Join(dir, name))
		}
	}
	require.Len(t, found, 1, "expected exactly one agent-*.jsonl in %s, got %v", dir, found)
	return found[0]
}

// TestRunClaudeStreaming_OptsSessionLogDirOverridesConfig exercises the
// claude streaming path end-to-end and confirms the per-run override
// (opts.SessionLogDir) wins over the runner-wide Config.SessionLogDir.
// This is the execute-bead parity behavior: the JSONL progress trace must
// land in the DDx-owned bundle dir, not the runner's default log dir.
func TestRunClaudeStreaming_OptsSessionLogDirOverridesConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake claude binary relies on POSIX shell")
	}
	tmp := t.TempDir()
	binPath := writeFakeClaudeBinary(t, tmp)
	runnerDefaultDir := filepath.Join(tmp, "runner-default")
	bundleEmbeddedDir := filepath.Join(tmp, "bundle-embedded")

	r := NewRunner(Config{SessionLogDir: runnerDefaultDir})
	harness := Harness{Name: "claude", Binary: binPath, PromptMode: "arg"}
	opts := RunOptions{
		Harness:       "claude",
		Prompt:        "hi",
		SessionLogDir: bundleEmbeddedDir,
	}

	result, err := r.runClaudeStreaming(context.Background(), harness, "claude", "", opts, "hi", "", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Progress trace must land in the opts-provided dir, not the config dir.
	assert.NoDirExists(t, runnerDefaultDir, "runner default dir must stay empty when opts override is set")
	logPath := findClaudeProgressLog(t, bundleEmbeddedDir)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	// At minimum the session.start and llm.response events should be present.
	assert.Contains(t, string(data), `"session.start"`)
	assert.Contains(t, string(data), `"llm.response"`)
}

// TestRunClaudeStreaming_FallsBackToConfigLogDir is the regression guard:
// when opts.SessionLogDir is empty, the trace must still go to the runner's
// configured SessionLogDir (the pre-fix default behavior).
func TestRunClaudeStreaming_FallsBackToConfigLogDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake claude binary relies on POSIX shell")
	}
	tmp := t.TempDir()
	binPath := writeFakeClaudeBinary(t, tmp)
	runnerDefaultDir := filepath.Join(tmp, "runner-default")

	r := NewRunner(Config{SessionLogDir: runnerDefaultDir})
	harness := Harness{Name: "claude", Binary: binPath, PromptMode: "arg"}
	opts := RunOptions{
		Harness: "claude",
		Prompt:  "hi",
		// SessionLogDir deliberately left empty — must fall back to config.
	}

	result, err := r.runClaudeStreaming(context.Background(), harness, "claude", "", opts, "hi", "", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)

	logPath := findClaudeProgressLog(t, runnerDefaultDir)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"session.start"`)
}
