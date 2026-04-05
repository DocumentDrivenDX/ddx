package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock executor ---

type mockExecutor struct {
	lastBinary string
	lastArgs   []string
	lastStdin  string
	output     string
	exitCode   int
	err        error
}

func (m *mockExecutor) Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error) {
	m.lastBinary = binary
	m.lastArgs = args
	m.lastStdin = stdin
	if m.err != nil {
		return &ExecResult{ExitCode: m.exitCode}, m.err
	}
	return &ExecResult{Stdout: m.output, ExitCode: m.exitCode}, nil
}

func mockLookPath(file string) (string, error) {
	return "/usr/bin/" + file, nil
}

func newTestRunner(exec *mockExecutor) *Runner {
	r := NewRunner(Config{SessionLogDir: ""}) // disable logging
	r.Executor = exec
	r.LookPath = mockLookPath
	return r
}

// --- Registry tests ---

func TestRegistryBuiltinHarnesses(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"codex", "claude", "gemini", "opencode", "pi", "cursor"} {
		assert.True(t, r.Has(name), "should have builtin harness: %s", name)
	}
	assert.False(t, r.Has("nonexistent"))
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("codex")
	require.True(t, ok)
	assert.Equal(t, "codex", h.Name)
	assert.Equal(t, "codex", h.Binary)
	assert.Equal(t, "arg", h.PromptMode)
	assert.Equal(t, "-m", h.ModelFlag)
	assert.Equal(t, "-C", h.WorkDirFlag)
}

func TestRegistryNamesPreferenceOrder(t *testing.T) {
	r := NewRegistry()
	names := r.Names()
	require.Len(t, names, 6)
	assert.Equal(t, "codex", names[0])
	assert.Equal(t, "claude", names[1])
	assert.Equal(t, "gemini", names[2])
}

// --- Arg construction tests ---

func TestBuildArgsCodexBasic(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("codex")
	args := BuildArgs(h, RunOptions{Prompt: "do stuff"}, "")
	assert.Equal(t, []string{
		"--dangerously-bypass-approvals-and-sandbox", "exec", "--ephemeral", "--json",
		"do stuff",
	}, args)
}

func TestBuildArgsCodexAllFlags(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("codex")
	args := BuildArgs(h, RunOptions{
		Prompt:  "build it",
		WorkDir: "/tmp/project",
		Effort:  "high",
	}, "o3-mini")
	assert.Contains(t, args, "-C")
	assert.Contains(t, args, "/tmp/project")
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "o3-mini")
	assert.Contains(t, args, "-c")
	assert.Contains(t, args, "reasoning.effort=high")
	// prompt is last
	assert.Equal(t, "build it", args[len(args)-1])
}

func TestBuildArgsClaudeBasic(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("claude")
	args := BuildArgs(h, RunOptions{Prompt: "review code"}, "")
	// Should have base args + prompt
	assert.Contains(t, args, "--no-session-persistence")
	assert.Contains(t, args, "--print")
	assert.Contains(t, args, "-p")
	assert.Equal(t, "review code", args[len(args)-1])
}

func TestBuildArgsClaudeWithModel(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("claude")
	args := BuildArgs(h, RunOptions{Prompt: "test"}, "claude-sonnet-4-20250514")
	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "claude-sonnet-4-20250514")
}

func TestBuildArgsGeminiStdin(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("gemini")
	args := BuildArgs(h, RunOptions{Prompt: "hello"}, "")
	// stdin mode: prompt should NOT be in args
	for _, arg := range args {
		assert.NotEqual(t, "hello", arg, "stdin harness should not have prompt in args")
	}
}

func TestBuildArgsNoModelFlagWhenEmpty(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("gemini")
	args := BuildArgs(h, RunOptions{Prompt: "test"}, "some-model")
	// gemini has no ModelFlag, so model should not appear
	for _, arg := range args {
		assert.NotEqual(t, "some-model", arg, "harness without ModelFlag should not include model")
	}
}

// --- Runner with mock executor ---

func TestRunWithMockExecutor(t *testing.T) {
	mock := &mockExecutor{output: "agent output here\n"}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "codex", Prompt: "do stuff"})
	require.NoError(t, err)
	assert.Equal(t, "codex", mock.lastBinary)
	assert.Equal(t, "agent output here\n", result.Output)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunStdinMode(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "gemini", Prompt: "hello via stdin"})
	require.NoError(t, err)
	assert.Equal(t, "gemini", mock.lastBinary)
	assert.Equal(t, "hello via stdin", mock.lastStdin)
	assert.Equal(t, "ok", result.Output)
}

func TestRunPromptFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "prompt.txt")
	os.WriteFile(tmpFile, []byte("prompt from file"), 0644)

	mock := &mockExecutor{output: "done"}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "codex", PromptFile: tmpFile})
	require.NoError(t, err)
	assert.Equal(t, "done", result.Output)
	// The prompt text should be in the args (codex is arg mode)
	assert.Equal(t, "prompt from file", mock.lastArgs[len(mock.lastArgs)-1])
}

func TestRunModelResolution(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)
	r.Config.Models = map[string]string{"codex": "o3-mini"}

	_, err := r.Run(RunOptions{Harness: "codex", Prompt: "test"})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "-m")
	assert.Contains(t, mock.lastArgs, "o3-mini")
}

func TestCapabilitiesUsesBuiltinDefaultModel(t *testing.T) {
	r := newTestRunner(&mockExecutor{})

	caps, err := r.Capabilities("codex")
	require.NoError(t, err)
	assert.Equal(t, "o3-mini", caps.Model)
	assert.Contains(t, caps.Models, "o3-mini") // default model is always in the list
}

func TestRunModelOverride(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)
	r.Config.Models = map[string]string{"codex": "o3-mini"}

	_, err := r.Run(RunOptions{Harness: "codex", Prompt: "test", Model: "gpt-4o"})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "gpt-4o")
	assert.NotContains(t, mock.lastArgs, "o3-mini")
}

func TestRunNonZeroExit(t *testing.T) {
	mock := &mockExecutor{output: "partial output", exitCode: 1}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "codex", Prompt: "fail"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.Equal(t, "partial output", result.Output)
}

func TestRunUnknownHarness(t *testing.T) {
	r := NewRunner(Config{})
	_, err := r.Run(RunOptions{Harness: "nonexistent", Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown harness")
}

func TestRunEmptyPrompt(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	_, err := r.Run(RunOptions{Harness: "codex"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}

func TestCapabilitiesUsesDefaultsAndConfigOverrides(t *testing.T) {
	mock := &mockExecutor{}
	r := newTestRunner(mock)
	r.Config.Models = map[string]string{"codex": "gpt-4o"}
	r.Config.ReasoningLevels = map[string][]string{
		"codex": []string{"concise", "balanced", "deep"},
	}

	caps, err := r.Capabilities("codex")
	require.NoError(t, err)
	assert.Equal(t, "codex", caps.Harness)
	assert.Equal(t, "gpt-4o", caps.Model)
	assert.Contains(t, caps.Models, "gpt-4o") // configured model is in the list
	assert.Equal(t, []string{"concise", "balanced", "deep"}, caps.ReasoningLevels)
	assert.NotEmpty(t, caps.Path)
}

func TestCapabilitiesUnknownHarness(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	_, err := r.Capabilities("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown harness")
}

// --- Token extraction ---

func TestExtractTokensCodex(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("codex")
	// codex uses ExtractUsage (JSON path); plain-text output returns 0
	assert.Equal(t, 0, ExtractTokens("some output\ntokens used\n1,234\n", h))
	// JSON output is parsed via ExtractUsage
	jsonOutput := `{"type":"turn.completed","usage":{"input_tokens":1000,"output_tokens":234}}` + "\n"
	assert.Equal(t, 1234, ExtractTokens(jsonOutput, h))
}

func TestExtractTokensNoPattern(t *testing.T) {
	h := Harness{TokenPattern: ""}
	assert.Equal(t, 0, ExtractTokens("tokens: 500", h))
}

func TestExtractTokensNoMatch(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("codex")
	assert.Equal(t, 0, ExtractTokens("no token info", h))
}

// TC-010: codex harness Args contains "--json"
func TestCodexArgsContainsJSON(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("codex")
	require.True(t, ok)
	assert.Contains(t, h.Args, "--json")
}

// TC-001: ExtractUsage with fixture codex JSONL output containing turn.completed returns correct tokens
func TestExtractUsageCodexTurnCompleted(t *testing.T) {
	fixture := `{"type":"turn.started","session_id":"s-abc123"}
{"type":"message","role":"assistant","content":"Working on it..."}
{"type":"turn.completed","usage":{"input_tokens":17337,"cached_input_tokens":16768,"output_tokens":37}}
`
	usage := ExtractUsage("codex", fixture)
	assert.Equal(t, 17337, usage.InputTokens)
	assert.Equal(t, 37, usage.OutputTokens)
	assert.Equal(t, 0.0, usage.CostUSD)
}

// TC-002: ExtractUsage with fixture claude JSON returns correct tokens and cost
func TestExtractUsageClaudeJSON(t *testing.T) {
	fixture := `{"usage":{"input_tokens":5000,"output_tokens":800,"cache_creation_input_tokens":0,"cache_read_input_tokens":4200},"total_cost_usd":0.045,"result":"the agent's text output..."}`
	usage := ExtractUsage("claude", fixture)
	assert.Equal(t, 5000, usage.InputTokens)
	assert.Equal(t, 800, usage.OutputTokens)
	assert.Equal(t, 0.045, usage.CostUSD)
}

// TC-002b: ExtractUsage with claude JSON as last line (preceded by other output) returns correct tokens and cost
func TestExtractUsageClaudeJSONLastLine(t *testing.T) {
	fixture := "some preamble output\nanother line\n" + `{"usage":{"input_tokens":5000,"output_tokens":800,"cache_creation_input_tokens":0,"cache_read_input_tokens":4200},"total_cost_usd":0.045,"result":"the agent's text output..."}`
	usage := ExtractUsage("claude", fixture)
	assert.Equal(t, 5000, usage.InputTokens)
	assert.Equal(t, 800, usage.OutputTokens)
	assert.Equal(t, 0.045, usage.CostUSD)
}

// TC-003: ExtractUsage with garbage input returns zero UsageData (no panic)
func TestExtractUsageCodexGarbageInput(t *testing.T) {
	usage := ExtractUsage("codex", "not json at all\n{broken\n")
	assert.Equal(t, UsageData{}, usage)
}

// TC-011: claude harness Args contains "--output-format" and "json"
func TestClaudeArgsContainsOutputFormatJSON(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("claude")
	require.True(t, ok)
	assert.Contains(t, h.Args, "--output-format")
	assert.Contains(t, h.Args, "json")
}

// --- Session logging ---

func TestSessionLogging(t *testing.T) {
	logDir := t.TempDir()
	jsonOutput := `{"type":"turn.completed","usage":{"input_tokens":12,"output_tokens":30}}` + "\n"
	mock := &mockExecutor{output: jsonOutput}
	r := newTestRunner(mock)
	r.Config.SessionLogDir = logDir

	_, err := r.Run(RunOptions{Harness: "codex", Prompt: "test prompt"})
	require.NoError(t, err)

	// Verify session log was written
	data, err := os.ReadFile(filepath.Join(logDir, "sessions.jsonl"))
	require.NoError(t, err)

	var entry SessionEntry
	require.NoError(t, json.Unmarshal(data[:len(data)-1], &entry)) // strip trailing newline
	assert.Equal(t, "codex", entry.Harness)
	assert.Equal(t, 42, entry.Tokens)
	assert.Equal(t, 11, entry.PromptLen) // len("test prompt")
	assert.Equal(t, "test prompt", entry.Prompt)
	assert.Equal(t, jsonOutput, entry.Response)
	assert.Equal(t, "inline", entry.PromptSource)
	assert.True(t, strings.HasPrefix(entry.ID, "as-"))
}

func TestSessionEntryLegacyRowCompatibility(t *testing.T) {
	row := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"tokens":500,"duration_ms":2000,"exit_code":0}`
	var entry SessionEntry
	require.NoError(t, json.Unmarshal([]byte(row), &entry))
	assert.Equal(t, "as-0001", entry.ID)
	assert.Equal(t, 100, entry.PromptLen)
	assert.Empty(t, entry.Prompt)
	assert.Empty(t, entry.Response)
}

// TC-004: old-format JSON (tokens only) parses without error; new fields default to zero.
func TestSessionEntryTC004_OldFormatNewFieldsZero(t *testing.T) {
	row := `{"id":"as-0002","timestamp":"2026-01-01T10:00:00Z","harness":"codex","prompt_len":50,"tokens":1200,"duration_ms":1000,"exit_code":0}`
	var entry SessionEntry
	require.NoError(t, json.Unmarshal([]byte(row), &entry))
	assert.Equal(t, 1200, entry.Tokens)
	assert.Equal(t, 0, entry.InputTokens)
	assert.Equal(t, 0, entry.OutputTokens)
	assert.Equal(t, 0.0, entry.CostUSD)
}

// TC-005: new fields round-trip through JSON correctly.
func TestSessionEntryTC005_NewFieldsRoundTrip(t *testing.T) {
	original := SessionEntry{
		ID:           "as-0003",
		Harness:      "claude",
		Tokens:       900,
		InputTokens:  300,
		OutputTokens: 600,
		CostUSD:      0.0045,
		Duration:     1500,
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded SessionEntry
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Tokens, decoded.Tokens)
	assert.Equal(t, original.InputTokens, decoded.InputTokens)
	assert.Equal(t, original.OutputTokens, decoded.OutputTokens)
	assert.InDelta(t, original.CostUSD, decoded.CostUSD, 1e-9)
}

// --- Quorum ---

func TestEffectiveThreshold(t *testing.T) {
	tests := []struct {
		strategy  string
		threshold int
		total     int
		expected  int
	}{
		{"any", 0, 3, 1},
		{"majority", 0, 3, 2},
		{"majority", 0, 5, 3},
		{"unanimous", 0, 3, 3},
		{"", 2, 3, 2},
		{"", 0, 3, 1},
	}
	for _, tt := range tests {
		got := effectiveThreshold(tt.strategy, tt.threshold, tt.total)
		assert.Equal(t, tt.expected, got)
	}
}

func TestQuorumMet(t *testing.T) {
	pass := &Result{ExitCode: 0}
	fail := &Result{ExitCode: 1}

	assert.True(t, QuorumMet("any", 0, []*Result{pass, fail, fail}))
	assert.False(t, QuorumMet("any", 0, []*Result{fail, fail, fail}))
	assert.True(t, QuorumMet("majority", 0, []*Result{pass, pass, fail}))
	assert.False(t, QuorumMet("majority", 0, []*Result{pass, fail, fail}))
	assert.True(t, QuorumMet("unanimous", 0, []*Result{pass, pass, pass}))
	assert.False(t, QuorumMet("unanimous", 0, []*Result{pass, nil, pass}))
}

func TestQuorumRunsAllHarnesses(t *testing.T) {
	calls := make(map[string]bool)
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)
	// Override executor to track calls
	r.Executor = &trackingExecutor{calls: calls, output: "ok"}

	results, err := r.RunQuorum(QuorumOptions{
		RunOptions: RunOptions{Prompt: "test"},
		Harnesses:  []string{"codex", "claude"},
		Strategy:   "unanimous",
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.True(t, calls["codex"])
	assert.True(t, calls["claude"])
}

type trackingExecutor struct {
	mu     sync.Mutex
	calls  map[string]bool
	output string
}

func (e *trackingExecutor) Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error) {
	e.mu.Lock()
	e.calls[binary] = true
	e.mu.Unlock()
	return &ExecResult{Stdout: e.output}, nil
}

func TestRunWithUnknownModelWarns(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "codex", Prompt: "test", Model: "gpt-99-turbo"})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "ok", result.Output)
}

func TestRunWithUnknownEffortWarns(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "codex", Prompt: "test", Effort: "turbo"})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "ok", result.Output)
}

func TestCapabilitiesIncludesDefaultModel(t *testing.T) {
	r := newTestRunner(&mockExecutor{})

	caps, err := r.Capabilities("codex")
	require.NoError(t, err)
	assert.Contains(t, caps.Models, "o3-mini") // default model always present

	caps, err = r.Capabilities("claude")
	require.NoError(t, err)
	assert.Contains(t, caps.Models, "claude-sonnet-4-20250514")
}

// --- Integration tests (require real harnesses) ---

func TestIntegration_CodexEcho(t *testing.T) {
	if _, err := DefaultLookPath("codex"); err != nil {
		t.Skip("codex not available")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 30000})
	result, err := r.Run(RunOptions{
		Harness: "codex",
		Prompt:  `print("hello from codex integration test")`,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

func TestIntegration_ClaudeEcho(t *testing.T) {
	if _, err := DefaultLookPath("claude"); err != nil {
		t.Skip("claude not available")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 60000})
	result, err := r.Run(RunOptions{
		Harness: "claude",
		Prompt:  "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}
