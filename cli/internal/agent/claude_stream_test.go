package agent

import (
	"bytes"
	"encoding/json"
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
