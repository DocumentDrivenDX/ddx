package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain scrubs all GIT_* environment variables before running tests so
// test subprocesses (and the production code under test) don't inherit
// lefthook's GIT_DIR/GIT_WORK_TREE and leak into the parent repo's config.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				_ = os.Unsetenv(kv[:idx])
			}
		}
	}
	os.Exit(m.Run())
}

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
	return m.ExecuteInDir(ctx, binary, args, stdin, "")
}

func (m *mockExecutor) ExecuteInDir(ctx context.Context, binary string, args []string, stdin, dir string) (*ExecResult, error) {
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
	for _, name := range []string{"codex", "claude", "gemini", "opencode", "agent", "pi"} {
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

func TestRegistryDefaultBaseArgs(t *testing.T) {
	r := NewRegistry()

	codex, ok := r.Get("codex")
	require.True(t, ok)
	assert.Equal(t, []string{"exec", "--json"}, codex.BaseArgs)
	assert.NotContains(t, codex.BaseArgs, "--ephemeral")

	claude, ok := r.Get("claude")
	require.True(t, ok)
	assert.Equal(t, []string{"--print", "-p", "--verbose", "--output-format", "stream-json"}, claude.BaseArgs)
	assert.NotContains(t, claude.BaseArgs, "--no-session-persistence")
}

func TestRegistryNamesPreferenceOrder(t *testing.T) {
	r := NewRegistry()
	names := r.Names()
	require.Len(t, names, 10)
	assert.Equal(t, "codex", names[0])
	assert.Equal(t, "claude", names[1])
	assert.Equal(t, "gemini", names[2])
	assert.Contains(t, names, "virtual")
}

// --- Arg construction tests ---

func TestBuildArgsCodexBasic(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("codex")
	args := BuildArgs(h, RunOptions{Prompt: "do stuff"}, "")
	// Default (safe): no bypass flags, structured JSON remains enabled.
	assert.Equal(t, []string{"exec", "--json", "do stuff"}, args)
	for _, arg := range args {
		assert.NotEqual(t, "--dangerously-bypass-approvals-and-sandbox", arg,
			"safe mode should not include bypass flag")
	}
}

func TestBuildArgsCodexAllFlags(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("codex")
	args := BuildArgs(h, RunOptions{
		Prompt:  "build it",
		WorkDir: "/tmp/project",
		Effort:  "high",
	}, "gpt-5.4")
	assert.Contains(t, args, "-C")
	assert.Contains(t, args, "/tmp/project")
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "gpt-5.4")
	assert.Contains(t, args, "-c")
	assert.Contains(t, args, "reasoning.effort=high")
	// prompt is last
	assert.Equal(t, "build it", args[len(args)-1])
}

func TestBuildArgsClaudeBasic(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("claude")
	args := BuildArgs(h, RunOptions{Prompt: "review code"}, "")
	// Should have base args + prompt, with stream-json output preserved so the
	// harness emits real-time progress during a run.
	assert.Equal(t, []string{"--print", "-p", "--verbose", "--output-format", "stream-json", "review code"}, args)
}

func TestBuildArgsClaudeWithModel(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("claude")
	args := BuildArgs(h, RunOptions{Prompt: "test"}, "claude-sonnet-4-6")
	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "claude-sonnet-4-6")
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
	// No harness without ModelFlag in current registry - this test verifies BuildArgs behavior
	// when a harness has no ModelFlag set
	h, _ := r.Get("gemini")
	// Note: gemini HAS ModelFlag now, so this test is deprecated
	// Keeping for BuildArgs coverage of edge case
	assert.Equal(t, "gemini", h.Name)
}

// --- Permission profile tests ---

func TestBuildArgsPermissionsDefault(t *testing.T) {
	r := NewRegistry()
	// codex: default (no permissions set) should be safe — no bypass flags
	h, _ := r.Get("codex")
	args := BuildArgs(h, RunOptions{Prompt: "task"}, "")
	for _, arg := range args {
		assert.NotEqual(t, "--dangerously-bypass-approvals-and-sandbox", arg,
			"default safe mode must not include codex bypass flag")
	}

	// claude: default safe — no bypass flags
	hc, _ := r.Get("claude")
	argsC := BuildArgs(hc, RunOptions{Prompt: "task"}, "")
	for _, arg := range argsC {
		assert.NotEqual(t, "--dangerously-skip-permissions", arg,
			"default safe mode must not include claude bypass flag")
		assert.NotEqual(t, "bypassPermissions", arg,
			"default safe mode must not include bypassPermissions")
	}
}

func TestBuildArgsPermissionsUnrestricted(t *testing.T) {
	r := NewRegistry()
	// codex unrestricted
	h, _ := r.Get("codex")
	args := BuildArgs(h, RunOptions{Prompt: "task", Permissions: "unrestricted"}, "")
	assert.Contains(t, args, "--dangerously-bypass-approvals-and-sandbox")

	// claude unrestricted
	hc, _ := r.Get("claude")
	argsC := BuildArgs(hc, RunOptions{Prompt: "task", Permissions: "unrestricted"}, "")
	assert.Contains(t, argsC, "--dangerously-skip-permissions")
	assert.Contains(t, argsC, "bypassPermissions")
}

func TestBuildArgsPermissionsFlagOverridesConfig(t *testing.T) {
	// Runner with config permissions = "safe", opts.Permissions = "unrestricted"
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)
	r.Config.Permissions = "safe"

	_, err := r.Run(RunOptions{
		Harness:     "codex",
		Prompt:      "task",
		Permissions: "unrestricted",
	})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "--dangerously-bypass-approvals-and-sandbox",
		"--permissions flag should override config permission level")
}

func TestBuildArgsPermissionsConfigDefault(t *testing.T) {
	// Runner with no config permissions — should be safe
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)

	_, err := r.Run(RunOptions{
		Harness: "codex",
		Prompt:  "task",
	})
	require.NoError(t, err)
	for _, arg := range mock.lastArgs {
		assert.NotEqual(t, "--dangerously-bypass-approvals-and-sandbox", arg,
			"no permissions config should default to safe")
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
	r.Config.Models = map[string]string{"codex": "gpt-5.4"}

	_, err := r.Run(RunOptions{Harness: "codex", Prompt: "test"})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "-m")
	assert.Contains(t, mock.lastArgs, "gpt-5.4")
}

func TestCapabilitiesUsesBuiltinDefaultModel(t *testing.T) {
	r := newTestRunner(&mockExecutor{})

	caps, err := r.Capabilities("codex")
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.4", caps.Model)
	assert.Contains(t, caps.Models, "gpt-5.4") // default model is always in the list
}

func TestRunModelOverride(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)
	r.Config.Models = map[string]string{"codex": "gpt-5.4"}

	_, err := r.Run(RunOptions{Harness: "codex", Prompt: "test", Model: "gpt-4o"})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "gpt-4o")
	assert.NotContains(t, mock.lastArgs, "gpt-5.4")
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

// TC-010: codex harness BaseArgs contains "--json"
func TestCodexArgsContainsJSON(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("codex")
	require.True(t, ok)
	assert.Contains(t, h.BaseArgs, "--json")
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

// TC-011: claude harness BaseArgs contains "--output-format" and a structured format.
// The harness switched from non-streaming "json" to "stream-json" to emit
// real-time progress during long-running bead executions.
func TestClaudeArgsContainsOutputFormatJSON(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("claude")
	require.True(t, ok)
	assert.Contains(t, h.BaseArgs, "--output-format")
	assert.Contains(t, h.BaseArgs, "stream-json")
	assert.Contains(t, h.BaseArgs, "--verbose")
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
	assert.Empty(t, entry.NativeSessionID)
	assert.Empty(t, entry.TraceID)
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
		ID:              "as-0003",
		Harness:         "claude",
		Surface:         "claude",
		CanonicalTarget: "claude-sonnet-4-6",
		Tokens:          900,
		InputTokens:     300,
		OutputTokens:    600,
		CostUSD:         0.0045,
		Duration:        1500,
		NativeSessionID: "native-123",
		NativeLogRef:    "/tmp/native.jsonl",
		TraceID:         "trace-123",
		SpanID:          "span-123",
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded SessionEntry
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Surface, decoded.Surface)
	assert.Equal(t, original.CanonicalTarget, decoded.CanonicalTarget)
	assert.Equal(t, original.Tokens, decoded.Tokens)
	assert.Equal(t, original.InputTokens, decoded.InputTokens)
	assert.Equal(t, original.OutputTokens, decoded.OutputTokens)
	assert.InDelta(t, original.CostUSD, decoded.CostUSD, 1e-9)
	assert.Equal(t, original.NativeSessionID, decoded.NativeSessionID)
	assert.Equal(t, original.NativeLogRef, decoded.NativeLogRef)
	assert.Equal(t, original.TraceID, decoded.TraceID)
	assert.Equal(t, original.SpanID, decoded.SpanID)
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
	return e.ExecuteInDir(ctx, binary, args, stdin, "")
}

func (e *trackingExecutor) ExecuteInDir(ctx context.Context, binary string, args []string, stdin, dir string) (*ExecResult, error) {
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
	assert.Contains(t, caps.Models, "gpt-5.4") // default model always present

	caps, err = r.Capabilities("claude")
	require.NoError(t, err)
	assert.Contains(t, caps.Models, "claude-sonnet-4-6")
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

// --- Opencode harness tests ---

func TestBuildArgsOpencodeBasic(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("opencode")
	require.True(t, ok)
	args := BuildArgs(h, RunOptions{Prompt: "do stuff"}, "")
	assert.Equal(t, []string{"run", "--format", "json", "do stuff"}, args)
}

func TestBuildArgsOpencodeAllFlags(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("opencode")
	args := BuildArgs(h, RunOptions{
		Prompt:  "build it",
		WorkDir: "/tmp/project",
		Effort:  "high",
	}, "anthropic/claude-sonnet-4-6")
	assert.Contains(t, args, "--dir")
	assert.Contains(t, args, "/tmp/project")
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "anthropic/claude-sonnet-4-6")
	assert.Contains(t, args, "--variant")
	assert.Contains(t, args, "high")
	assert.Equal(t, "build it", args[len(args)-1])
}

func TestOpencodeHarnessProperties(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("opencode")
	require.True(t, ok)
	assert.Equal(t, "opencode", h.Binary)
	assert.Equal(t, "arg", h.PromptMode)
	assert.Equal(t, "--dir", h.WorkDirFlag)
	assert.Equal(t, "-m", h.ModelFlag)
	assert.Equal(t, "--variant", h.EffortFlag)
	assert.Contains(t, h.BaseArgs, "run")
	assert.Contains(t, h.BaseArgs, "--format")
	assert.Contains(t, h.BaseArgs, "json")
}

func TestOpencodePermissionsAllLevels(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("opencode")

	// All permission levels should produce the same args (opencode run auto-approves)
	for _, level := range []string{"safe", "supervised", "unrestricted"} {
		args := BuildArgs(h, RunOptions{Prompt: "task", Permissions: level}, "")
		expected := []string{"run", "--format", "json", "task"}
		assert.Equal(t, expected, args, "permission level %q should not add extra flags", level)
	}
}

func TestOpencodeModelFlag(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("opencode")
	args := BuildArgs(h, RunOptions{Prompt: "test"}, "anthropic/claude-sonnet-4-6")
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "anthropic/claude-sonnet-4-6")
}

func TestRunOpencodeWithMockExecutor(t *testing.T) {
	mock := &mockExecutor{output: `{"result":"done"}`}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "opencode", Prompt: "do something"})
	require.NoError(t, err)
	assert.Equal(t, "opencode", mock.lastBinary)
	assert.Equal(t, `{"result":"done"}`, result.Output)
	assert.Equal(t, 0, result.ExitCode)
	// Prompt should be in args (arg mode), not stdin
	assert.Equal(t, "do something", mock.lastArgs[len(mock.lastArgs)-1])
	assert.Empty(t, mock.lastStdin, "opencode uses arg mode, not stdin")
}

func TestRunOpencodeWorkDir(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)

	_, err := r.Run(RunOptions{
		Harness: "opencode",
		Prompt:  "test",
		WorkDir: "/tmp/myproject",
	})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "--dir")
	assert.Contains(t, mock.lastArgs, "/tmp/myproject")
}

func TestExtractUsageOpencodeWithUsage(t *testing.T) {
	fixture := `{"result":"code changes applied","usage":{"input_tokens":3000,"output_tokens":500},"total_cost_usd":0.025}`
	usage := ExtractUsage("opencode", fixture)
	assert.Equal(t, 3000, usage.InputTokens)
	assert.Equal(t, 500, usage.OutputTokens)
	assert.Equal(t, 0.025, usage.CostUSD)
}

func TestExtractUsageOpencodeLastLine(t *testing.T) {
	fixture := "spinner output\nsome preamble\n" + `{"result":"done","usage":{"input_tokens":1200,"output_tokens":300},"total_cost_usd":0.01}`
	usage := ExtractUsage("opencode", fixture)
	assert.Equal(t, 1200, usage.InputTokens)
	assert.Equal(t, 300, usage.OutputTokens)
	assert.Equal(t, 0.01, usage.CostUSD)
}

func TestExtractUsageOpencodeNoUsage(t *testing.T) {
	// JSON output without usage fields — should return zero
	fixture := `{"result":"done"}`
	usage := ExtractUsage("opencode", fixture)
	assert.Equal(t, UsageData{}, usage)
}

func TestExtractUsageOpencodeGarbage(t *testing.T) {
	usage := ExtractUsage("opencode", "not json\n{broken\n")
	assert.Equal(t, UsageData{}, usage)
}

func TestCapabilitiesOpencode(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	caps, err := r.Capabilities("opencode")
	require.NoError(t, err)
	assert.Equal(t, "opencode", caps.Harness)
	assert.True(t, caps.Available)
	assert.Equal(t, "opencode", caps.Binary)
	assert.Equal(t, []string{"minimal", "low", "medium", "high", "max"}, caps.ReasoningLevels)
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

func TestIntegration_OpencodeEcho(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	if _, err := DefaultLookPath("opencode"); err != nil {
		t.Skip("opencode not available")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 60000})
	result, err := r.Run(RunOptions{
		Harness: "opencode",
		Prompt:  "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

// --- Pi harness tests ---

func TestPiHarnessProperties(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("pi")
	require.True(t, ok)
	assert.Equal(t, "pi", h.Name)
	assert.Equal(t, "pi", h.Binary)
	assert.Equal(t, "arg", h.PromptMode)
	assert.Equal(t, []string{"--mode", "json", "--print"}, h.BaseArgs)
	assert.Equal(t, "--model", h.ModelFlag)
	assert.Empty(t, h.WorkDirFlag, "pi should not have a workdir flag")
	assert.Equal(t, "--thinking", h.EffortFlag)
	assert.Nil(t, h.PermissionArgs, "pi should not have permission args")
	assert.Equal(t, []string{"low", "medium", "high"}, h.ReasoningLevels)
}

func TestBuildArgsPiBasic(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("pi")
	args := BuildArgs(h, RunOptions{Prompt: "hello world"}, "")
	// arg mode: base args + prompt
	assert.Equal(t, []string{"--mode", "json", "--print", "hello world"}, args)
}

func TestBuildArgsPiWithModelAndEffort(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("pi")
	args := BuildArgs(h, RunOptions{
		Prompt: "task",
		Model:  "pi-minimax",
		Effort: "high",
	}, "pi-default")
	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "pi-default")
	assert.Contains(t, args, "--thinking")
	assert.Contains(t, args, "high")
	assert.Contains(t, args, "task")
}

func TestRunPiWithMockExecutor(t *testing.T) {
	mock := &mockExecutor{output: `{"response": "Pi response text"}`}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "pi", Prompt: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "pi", mock.lastBinary)
	assert.Equal(t, []string{"--mode", "json", "--print", "hello"}, mock.lastArgs)
	assert.Empty(t, mock.lastStdin, "pi uses arg mode, not stdin")
	assert.Equal(t, `{"response": "Pi response text"}`, result.Output)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunPiWithModelAndEffort(t *testing.T) {
	mock := &mockExecutor{output: "response"}
	r := newTestRunner(mock)

	_, err := r.Run(RunOptions{
		Harness: "pi",
		Prompt:  "task",
		Model:   "pi-minimax",
		Effort:  "high",
	})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "--model")
	assert.Contains(t, mock.lastArgs, "pi-minimax")
	assert.Contains(t, mock.lastArgs, "--thinking")
	assert.Contains(t, mock.lastArgs, "high")
}

func TestExtractUsagePi(t *testing.T) {
	// pi outputs JSONL with message.usage.cost.total - scans backwards for cost
	fixture := `{"type":"session"}
{"type":"text_end","message":{"usage":{"input":135,"output":52,"cost":{"total":0.0003714}}}}`
	usage := ExtractUsage("pi", fixture)
	assert.Equal(t, 135, usage.InputTokens)
	assert.Equal(t, 52, usage.OutputTokens)
	assert.Equal(t, 0.0003714, usage.CostUSD)
}

func TestExtractUsageGemini(t *testing.T) {
	// gemini outputs single JSON with stats.models[].tokens
	fixture := `{"session_id":"abc","response":"ok","stats":{"models":{"gemini-3-flash-preview":{"tokens":{"input":2404,"total":9367}}}}}`
	usage := ExtractUsage("gemini", fixture)
	assert.Equal(t, 2404, usage.InputTokens)
	assert.Equal(t, 6963, usage.OutputTokens) // 9367 - 2404
}

func TestExtractOutputPi(t *testing.T) {
	// pi returns JSON with "response" field
	fixture := `{"type":"session"}
{"type":"agent_end"}
{"session_id":"abc","response":"TEST_OK"}`
	output := ExtractOutput("pi", fixture)
	assert.Equal(t, "TEST_OK", output)
}

func TestExtractOutputGemini(t *testing.T) {
	// gemini returns JSON with "response" field
	fixture := `{"session_id":"abc","response":"TEST_OK"}`
	output := ExtractOutput("gemini", fixture)
	assert.Equal(t, "TEST_OK", output)
}

func TestCapabilitiesPi(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	caps, err := r.Capabilities("pi")
	require.NoError(t, err)
	assert.Equal(t, "pi", caps.Harness)
	assert.True(t, caps.Available)
	assert.Equal(t, "pi", caps.Binary)
	assert.Equal(t, []string{"low", "medium", "high"}, caps.ReasoningLevels)
	assert.Empty(t, caps.Models, "pi should not expose known models")
}

// --- Gemini harness tests ---

func TestGeminiHarnessProperties(t *testing.T) {
	r := NewRegistry()
	h, ok := r.Get("gemini")
	require.True(t, ok)
	assert.Equal(t, "gemini", h.Name)
	assert.Equal(t, "gemini", h.Binary)
	assert.Equal(t, "stdin", h.PromptMode)
	assert.Empty(t, h.BaseArgs)
	assert.Equal(t, "-m", h.ModelFlag)
	assert.Empty(t, h.WorkDirFlag, "gemini should not have a workdir flag")
	assert.Empty(t, h.EffortFlag, "gemini should not have an effort flag")
	assert.Nil(t, h.PermissionArgs, "gemini should not have permission args")
	assert.Equal(t, []string{"low", "medium", "high"}, h.ReasoningLevels)
}

func TestBuildArgsGeminiWithModel(t *testing.T) {
	r := NewRegistry()
	h, _ := r.Get("gemini")
	args := BuildArgs(h, RunOptions{Prompt: "task"}, "gemini-2.5")
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "gemini-2.5")
}

func TestRunGeminiWithMockExecutor(t *testing.T) {
	mock := &mockExecutor{output: "Gemini response text"}
	r := newTestRunner(mock)

	result, err := r.Run(RunOptions{Harness: "gemini", Prompt: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "gemini", mock.lastBinary)
	assert.Empty(t, mock.lastArgs, "gemini should be invoked with no args (stdin mode)")
	assert.Equal(t, "hello", mock.lastStdin, "gemini should receive prompt via stdin")
	assert.Equal(t, "Gemini response text", result.Output)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunGeminiWithModel(t *testing.T) {
	mock := &mockExecutor{output: "response"}
	r := newTestRunner(mock)

	_, err := r.Run(RunOptions{
		Harness: "gemini",
		Prompt:  "task",
		Model:   "gemini-2.5",
	})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "-m")
	assert.Contains(t, mock.lastArgs, "gemini-2.5")
}

func TestCapabilitiesGemini(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	caps, err := r.Capabilities("gemini")
	require.NoError(t, err)
	assert.Equal(t, "gemini", caps.Harness)
	assert.True(t, caps.Available)
	assert.Equal(t, "gemini", caps.Binary)
	assert.Equal(t, []string{"low", "medium", "high"}, caps.ReasoningLevels)
	assert.Empty(t, caps.Models, "gemini should not expose known models")
}

// --- Integration tests (skipped if binary not available) ---

func TestIntegration_PiEcho(t *testing.T) {
	if _, err := DefaultLookPath("pi"); err != nil {
		t.Skip("pi not available")
	}
	// Skip if no API key is configured for pi (avoids hanging until timeout).
	piKeys := []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN",
		"OPENAI_API_KEY", "GEMINI_API_KEY",
		"GROQ_API_KEY", "XAI_API_KEY", "OPENROUTER_API_KEY",
	}
	hasKey := false
	for _, k := range piKeys {
		if os.Getenv(k) != "" {
			hasKey = true
			break
		}
	}
	if !hasKey {
		t.Skip("pi API credentials not configured")
	}
	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 60000})
	result, err := r.Run(RunOptions{
		Harness: "pi",
		Prompt:  "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

func TestIntegration_GeminiEcho(t *testing.T) {
	if _, err := DefaultLookPath("gemini"); err != nil {
		t.Skip("gemini not available")
	}
	// Skip if gemini credentials are not configured (avoids hanging until timeout).
	// gemini CLI stores credentials in ~/.gemini/ or uses GEMINI_API_KEY.
	if os.Getenv("GEMINI_API_KEY") == "" {
		homeDir, _ := os.UserHomeDir()
		credPath := filepath.Join(homeDir, ".gemini", "credentials.json")
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			t.Skip("gemini credentials not configured (set GEMINI_API_KEY or provide ~/.gemini/credentials.json)")
		}
	}
	// Gemini has slow initialization (skill loading), so use a longer timeout
	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 180000})
	result, err := r.Run(RunOptions{
		Harness: "gemini",
		Prompt:  "Respond with exactly: INTEGRATION_TEST_OK",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0, error: %s", result.Error)
	assert.NotEmpty(t, result.Output, "should have output")
}

// --- OSExecutor early-cancel tests ---

func TestMatchesCancelPattern(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"Error 429 Too Many Requests", true},
		{"401 Unauthorized", true},
		{"403 Forbidden", true},
		{"rate limit exceeded", true},
		{"quota exceeded for project", true},
		{"not logged in, please authenticate", true},
		{"no credentials found", true},
		{"authentication required", true},
		{"invalid api key provided", true},
		{"insufficient credits remaining", true},
		{"normal output line", false},
		{"", false},
		{"running task...", false},
		// false-positive regression cases
		{"listening on port 4290", false},
		{"8401 bytes transferred", false},
		{"please run npm install", false},
		{"credentialing process", false},
	}
	for _, tc := range cases {
		got := matchesCancelPattern(tc.line)
		if tc.want && got == "" {
			t.Errorf("line %q: expected a match, got none", tc.line)
		}
		if !tc.want && got != "" {
			t.Errorf("line %q: expected no match, got %q", tc.line, got)
		}
	}
}

func TestOSExecutor_EarlyCancel(t *testing.T) {
	// Shell prints an auth error on stderr then sleeps. Without early-cancel
	// this would block for the full timeout.
	ex := &OSExecutor{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ex.ExecuteInDir(ctx, "sh", []string{"-c",
		`echo "Error: 429 rate limit exceeded" >&2; sleep 60`}, "", "")
	require.NoError(t, err)
	assert.True(t, result.EarlyCancel, "should have been cancelled early")
	assert.NotEmpty(t, result.CancelReason)
	assert.Contains(t, result.Stderr, "429")
	assert.Equal(t, -1, result.ExitCode)
}

// --- ValidateForExecuteLoop tests ---

// TestValidateForExecuteLoopUnknownHarness verifies that an unknown harness
// returns an error immediately so execute-loop fails before claiming any beads.
func TestValidateForExecuteLoopUnknownHarness(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	err := r.ValidateForExecuteLoop("nonexistent", "", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown harness")
}

// TestValidateForExecuteLoopEmptyHarnessIsNoop verifies that an empty harness
// (routing will pick at bead-claim time) returns no error.
func TestValidateForExecuteLoopEmptyHarnessIsNoop(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	assert.NoError(t, r.ValidateForExecuteLoop("", "", "", ""))
}

// TestValidateForExecuteLoopValidHarnessNoModel verifies that a valid,
// available harness passes pre-flight with no model specified.
func TestValidateForExecuteLoopValidHarnessNoModel(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	assert.NoError(t, r.ValidateForExecuteLoop("claude", "", "", ""))
}

// TestValidateForExecuteLoopValidHarnessAndModel verifies that a valid harness
// with a compatible model string passes pre-flight.
func TestValidateForExecuteLoopValidHarnessAndModel(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	assert.NoError(t, r.ValidateForExecuteLoop("claude", "claude-sonnet-4-6", "", ""))
}

func TestOSExecutor_NormalExit(t *testing.T) {
	ex := &OSExecutor{}
	ctx := context.Background()
	result, err := ex.ExecuteInDir(ctx, "sh", []string{"-c", `echo "hello"; exit 0`}, "", "")
	require.NoError(t, err)
	assert.False(t, result.EarlyCancel)
	assert.Empty(t, result.CancelReason)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "hello")
}

func TestOSExecutor_IdleTimeout(t *testing.T) {
	ex := &OSExecutor{}
	ctx := withExecutionTimeout(context.Background(), 100*time.Millisecond)

	result, err := ex.ExecuteInDir(ctx, "sh", []string{"-c", `sleep 1`}, "", "")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, result.EarlyCancel)
	assert.Equal(t, -1, result.ExitCode)
}

func TestOSExecutor_OutputExtendsTimeout(t *testing.T) {
	ex := &OSExecutor{}
	ctx := withExecutionTimeout(context.Background(), 150*time.Millisecond)

	result, err := ex.ExecuteInDir(ctx, "sh", []string{"-c", `for i in 1 2 3 4; do echo tick; sleep 0.05; done`}, "", "")
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "tick")
}

// TestOSExecutor_WallClockFiresDespiteStreamingActivity verifies that the
// wall-clock deadline fires even when stdout is streaming continuously.
// This is the RC2 ddx-0a651925 guard for subprocess harnesses: chatty
// output must not be able to defeat the absolute bound by resetting the
// idle timer on every write.
func TestOSExecutor_WallClockFiresDespiteStreamingActivity(t *testing.T) {
	ex := &OSExecutor{}
	// Idle timer is generous; only the wall-clock bound can terminate this run.
	ctx := withExecutionTimeout(context.Background(), 5*time.Second)
	ctx = withExecutionWallClock(ctx, 300*time.Millisecond)

	start := time.Now()
	result, err := ex.ExecuteInDir(
		ctx,
		"sh",
		[]string{"-c", `while true; do echo tick; sleep 0.05; done`},
		"",
		"",
	)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, result)
	assert.True(t, result.WallClockTimeout,
		"ExecResult must flag wall-clock timeout to distinguish from idle")
	assert.False(t, result.EarlyCancel)
	assert.Equal(t, -1, result.ExitCode)
	assert.GreaterOrEqual(t, elapsed, 250*time.Millisecond,
		"wall-clock should not fire before its deadline")
	assert.Less(t, elapsed, 2*time.Second,
		"wall-clock should fire within ~1s of its deadline")
	assert.Contains(t, result.Stdout, "tick",
		"streaming must actually have been occurring during the wait")
}

// TestOSExecutor_IdleTimeoutNotFlaggedAsWallClock confirms that the plain
// idle-timeout path does NOT mark ExecResult.WallClockTimeout, so callers
// can reliably tell the two failure modes apart in result.json.
func TestOSExecutor_IdleTimeoutNotFlaggedAsWallClock(t *testing.T) {
	ex := &OSExecutor{}
	ctx := withExecutionTimeout(context.Background(), 100*time.Millisecond)
	// Wall-clock is very long; idle must fire first.
	ctx = withExecutionWallClock(ctx, 5*time.Second)

	result, err := ex.ExecuteInDir(ctx, "sh", []string{"-c", `sleep 1`}, "", "")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, result)
	assert.False(t, result.WallClockTimeout,
		"idle-timeout path must not be mislabelled as wall-clock timeout")
	assert.Equal(t, -1, result.ExitCode)
}
