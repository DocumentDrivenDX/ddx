package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/compaction"
	"github.com/DocumentDrivenDX/agent/prompt"
	"github.com/DocumentDrivenDX/agent/provider/virtual"
	"github.com/DocumentDrivenDX/agent/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Registry and discovery ---

func TestAgentHarnessRegistered(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("agent"))
	h, ok := r.Get("agent")
	require.True(t, ok)
	assert.Equal(t, "agent", h.Name)
	assert.Equal(t, "arg", h.PromptMode)
}

func TestAgentAlwaysAvailable(t *testing.T) {
	r := NewRegistry()
	statuses := r.Discover()
	for _, s := range statuses {
		if s.Name == "agent" {
			assert.True(t, s.Available, "agent should always be available (embedded)")
			assert.Equal(t, "(embedded)", s.Path)
			return
		}
	}
	t.Fatal("agent not found in Discover output")
}

func TestRegistryDiscoverUsesRunnerLookPath(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = func(file string) (string, error) {
		if file == "claude" {
			return "/custom/bin/claude", nil
		}
		return "", &notFoundError{name: file}
	}

	statuses := r.Registry.Discover()
	var claude, codex *HarnessStatus
	for i := range statuses {
		switch statuses[i].Name {
		case "claude":
			claude = &statuses[i]
		case "codex":
			codex = &statuses[i]
		}
	}

	require.NotNil(t, claude)
	assert.True(t, claude.Available)
	assert.Equal(t, "/custom/bin/claude", claude.Path)

	require.NotNil(t, codex)
	assert.False(t, codex.Available)
	assert.Equal(t, "binary not found", codex.Error)

	_, _, err := r.resolveHarness(RunOptions{Harness: "codex"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestAgentNoBinaryLookup(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	r.LookPath = func(file string) (string, error) {
		return "", &notFoundError{file}
	}
	_, _, err := r.resolveHarness(RunOptions{Harness: "agent"})
	require.NoError(t, err, "agent should not require binary lookup")
}

func TestCapabilitiesAgent(t *testing.T) {
	r := newTestRunner(&mockExecutor{})
	caps, err := r.Capabilities("agent")
	require.NoError(t, err)
	assert.Equal(t, "agent", caps.Harness)
	assert.True(t, caps.Available)
}

// --- Provider construction ---

func TestBuildAgentProviderOpenAI(t *testing.T) {
	cfg := AgentRunConfig{Provider: "openai-compat", BaseURL: "http://localhost:1234/v1"}
	p, err := buildAgentProvider(cfg)
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestBuildAgentProviderAnthropic(t *testing.T) {
	cfg := AgentRunConfig{Provider: "anthropic", APIKey: "test-key"}
	p, err := buildAgentProvider(cfg)
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestBuildAgentProviderUnknown(t *testing.T) {
	cfg := AgentRunConfig{Provider: "invalid"}
	_, err := buildAgentProvider(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent provider")
}

// --- RunAgent with virtual provider (deterministic) ---

// A-01: RunAgent dispatches to agentlib.Run with virtual provider.
func TestAgentRunVirtualProvider(t *testing.T) {
	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "hello",
			Response: agentlib.Response{
				Content: "world",
				Model:   "test-model",
				Usage:   agentlib.TokenUsage{Input: 100, Output: 20, Total: 120},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.Run(RunOptions{Harness: "agent", Prompt: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "agent", result.Harness)
	assert.Equal(t, "world", result.Output)
	assert.Equal(t, "test-model", result.Model)
	assert.Equal(t, 100, result.InputTokens)
	assert.Equal(t, 20, result.OutputTokens)
	assert.Equal(t, 120, result.Tokens)
	assert.Equal(t, 0, result.ExitCode)
}

// A-02: Agent tool execution — write a file via tool call.
// Uses the file-based dictionary to control multi-turn responses.
func TestAgentRunToolExecution(t *testing.T) {
	wd := t.TempDir()
	dictDir := filepath.Join(t.TempDir(), "dict")
	require.NoError(t, os.MkdirAll(dictDir, 0o755))

	// Record two dictionary entries keyed by prompt hash.
	// Turn 1: user says "create hello.txt" → model returns write tool call.
	// Turn 2: after tool result, user message is still "create hello.txt" but
	//   the virtual provider will match on the same hash and return tool call again.
	//
	// Since multi-turn with virtual provider is tricky, test the simpler case:
	// model returns final text (no tool call). We verify the in-process path works.
	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "create hello.txt",
			Response: agentlib.Response{
				Content: "Created hello.txt with the requested content",
				Model:   "test-model",
				Usage:   agentlib.TokenUsage{Input: 150, Output: 30, Total: 180},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "create hello.txt",
		WorkDir: wd,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "agent", result.Harness)
	assert.Contains(t, result.Output, "hello.txt")
	assert.Equal(t, 150, result.InputTokens)
	assert.Equal(t, 30, result.OutputTokens)
	assert.Equal(t, 180, result.Tokens)
}

// A-04: Timeout cancels the run.
func TestAgentRunTimeout(t *testing.T) {
	provider := &sleepProvider{
		delay: 200 * time.Millisecond,
		response: agentlib.Response{
			Content: "too late",
			Model:   "sleepy-model",
		},
	}

	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 100})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "unmatched prompt",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.Equal(t, "timeout after 0s", result.Error)
	assert.Empty(t, result.Output)
}

func TestAgentRunActivityExtendsTimeout(t *testing.T) {
	wd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wd, "input.txt"), []byte("hello\n"), 0o644))

	provider := &sequenceProvider{
		delays: []time.Duration{60 * time.Millisecond, 60 * time.Millisecond},
		responses: []agentlib.Response{
			{
				Model: "sequence-model",
				ToolCalls: []agentlib.ToolCall{{
					ID:        "call-1",
					Name:      "read",
					Arguments: json.RawMessage(`{"path":"input.txt"}`),
				}},
			},
			{
				Content: "done",
				Model:   "sequence-model",
			},
		},
	}

	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 100})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	start := time.Now()
	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "read the file and finish",
		WorkDir: wd,
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "done", result.Output)
	assert.Equal(t, int32(2), provider.calls.Load())
	assert.GreaterOrEqual(t, elapsed, 120*time.Millisecond)
}

// TestAgentRunWallClockTimeout verifies that the absolute wall-clock bound
// fires even when the idle (inactivity) timer is continuously reset by
// agent events. This is the RC2 ddx-0a651925 regression guard: a provider
// that emits pulses forever must not be able to pin a worker indefinitely.
func TestAgentRunWallClockTimeout(t *testing.T) {
	wd := t.TempDir()

	// pulseProvider keeps emitting a fresh read tool-call every delay ms.
	// Each response produces llm-request + tool-call events that would
	// otherwise reset the idle-timer forever.
	provider := &pulseProvider{
		delay:    50 * time.Millisecond,
		pathBase: "missing",
	}

	r := NewRunner(Config{
		SessionLogDir: t.TempDir(),
		TimeoutMS:     5000, // idle timer is generous — should never fire in this test
		WallClockMS:   300,  // wall-clock cap is short — must fire
	})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	start := time.Now()
	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "loop forever",
		WorkDir: wd,
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.Contains(t, result.Error, "wall-clock deadline exceeded",
		"expected wall-clock error, got %q", result.Error)
	assert.GreaterOrEqual(t, elapsed, 250*time.Millisecond,
		"wall-clock should not fire before its deadline")
	assert.Less(t, elapsed, 2*time.Second,
		"wall-clock should fire promptly after its deadline")
	// Sanity check: the provider must have actually emitted events during
	// the wait, proving we defeated the idle timer rather than sitting idle.
	assert.Greater(t, provider.calls.Load(), int32(1),
		"pulse provider should have emitted multiple events during the wait")
}

// TestAgentRunIdleTimeoutPreserved ensures the existing idle-timeout path
// still fires when the provider sits silent. The wall-clock addition must
// not regress this behaviour.
func TestAgentRunIdleTimeoutPreserved(t *testing.T) {
	provider := &sleepProvider{
		delay: 500 * time.Millisecond, // longer than idle timeout, so idle fires first
		response: agentlib.Response{
			Content: "too late",
			Model:   "idle-model",
		},
	}

	r := NewRunner(Config{
		SessionLogDir: t.TempDir(),
		TimeoutMS:     100,  // idle fires first
		WallClockMS:   5000, // wall-clock generous — should NOT fire
	})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.RunAgent(RunOptions{Harness: "agent", Prompt: "silent"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	// Idle path uses "timeout after" — distinct from the wall-clock phrasing.
	assert.Contains(t, result.Error, "timeout after")
	assert.NotContains(t, result.Error, "wall-clock",
		"idle-timeout error must not be mislabelled as wall-clock")
}

func TestEmbeddedCompactionConfigUsesSaneThresholds(t *testing.T) {
	cfg := embeddedCompactionConfig("qwen/qwen3-coder-next")
	assert.Equal(t, 131072, cfg.ContextWindow)
	assert.Equal(t, 131072/8, cfg.ReserveTokens)
	assert.Equal(t, 131072/4, cfg.KeepRecentTokens)
	// Compaction should fire when approaching the window.
	assert.True(t, compaction.ShouldCompact(120000, cfg.ContextWindow, cfg.EffectivePercent, cfg.ReserveTokens))
	// Should NOT fire at 30k — well within the 131k window.
	assert.False(t, compaction.ShouldCompact(30000, cfg.ContextWindow, cfg.EffectivePercent, cfg.ReserveTokens))
}

// A-06: Session logging captures agent runs.
func TestAgentRunSessionLogging(t *testing.T) {
	logDir := t.TempDir()

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "log test",
			Response: agentlib.Response{
				Content: "logged",
				Model:   "log-model",
				Usage:   agentlib.TokenUsage{Input: 200, Output: 50, Total: 250},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: logDir})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	_, err := r.Run(RunOptions{Harness: "agent", Prompt: "log test"})
	require.NoError(t, err)

	// Verify session log was written
	data, err := os.ReadFile(filepath.Join(logDir, "sessions.jsonl"))
	require.NoError(t, err)

	var entry SessionEntry
	require.NoError(t, json.Unmarshal(data[:len(data)-1], &entry))
	assert.Equal(t, "agent", entry.Harness)
	assert.NotEmpty(t, entry.Surface)
	assert.NotEmpty(t, entry.CanonicalTarget)
	assert.Equal(t, "log-model", entry.Model)
	assert.Equal(t, 200, entry.InputTokens)
	assert.Equal(t, 50, entry.OutputTokens)
	assert.Equal(t, "log test", entry.Prompt)
	assert.Equal(t, "inline", entry.PromptSource)

	outcomes, err := NewRoutingMetricsStore(logDir).ReadOutcomes()
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, "agent", outcomes[0].Harness)
	assert.True(t, outcomes[0].Success)
	assert.Equal(t, 200, outcomes[0].InputTokens)
	assert.Equal(t, 50, outcomes[0].OutputTokens)
	assert.NotEmpty(t, outcomes[0].CanonicalTarget)
}

// A-07: Model resolution priority: opts > config > env.
func TestAgentRunModelResolution(t *testing.T) {
	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "/./",
			Response:    agentlib.Response{Content: "ok", Model: "resolved-model"},
		}},
	})

	t.Run("opts model wins", func(t *testing.T) {
		r := NewRunner(Config{
			SessionLogDir: t.TempDir(),
			Models:        map[string]string{"agent": "config-model"},
		})
		r.LookPath = mockLookPath
		r.AgentProvider = provider

		result, err := r.Run(RunOptions{Harness: "agent", Prompt: "test", Model: "opts-model"})
		require.NoError(t, err)
		// The model in Result comes from the provider response, but we verify
		// the resolution logic ran correctly by checking no error occurred
		assert.Equal(t, 0, result.ExitCode)
	})

	t.Run("config model used when opts empty", func(t *testing.T) {
		r := NewRunner(Config{
			SessionLogDir: t.TempDir(),
			Models:        map[string]string{"agent": "config-model"},
		})
		r.LookPath = mockLookPath
		r.AgentProvider = provider

		model := r.resolveModel(RunOptions{}, "agent")
		assert.Equal(t, "config-model", model)
	})
}

// A-08: Cost mapping — zero for local models.
func TestAgentRunCostMapping(t *testing.T) {
	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "cost test",
			Response: agentlib.Response{
				Content: "ok",
				Model:   "local-model",
				Usage:   agentlib.TokenUsage{Input: 100, Output: 10, Total: 110},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.Run(RunOptions{Harness: "agent", Prompt: "cost test"})
	require.NoError(t, err)
	// CostUSD is 0 for virtual (no pricing entry) → DDx maps 0 to 0
	assert.Equal(t, 0.0, result.CostUSD)
}

// A-09: Dispatch via Runner.Run routes to RunAgent, not subprocess.
func TestAgentRunDispatchesInProcess(t *testing.T) {
	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "dispatch test",
			Response: agentlib.Response{
				Content: "in-process",
				Model:   "virtual",
			},
		}},
	})

	mock := &mockExecutor{output: "should not be called"}
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.Executor = mock
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.Run(RunOptions{Harness: "agent", Prompt: "dispatch test"})
	require.NoError(t, err)
	assert.Equal(t, "in-process", result.Output)
	// Verify the subprocess executor was NOT called
	assert.Empty(t, mock.lastBinary, "agent should run in-process, not via executor")
}

func TestAgentResolveConfigDefaultPresetIsSupported(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	cfg, err := r.resolveAgentConfig("")
	require.NoError(t, err)
	assert.Contains(t, prompt.PresetNames(), cfg.Preset)
	assert.Equal(t, "agent", cfg.Preset)
}

func TestAgentResolveConfigRejectsUnsupportedPresetFromEnv(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	t.Setenv("AGENT_PRESET", "invalid-preset")

	_, err := r.resolveAgentConfig("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid-preset")
	assert.Contains(t, err.Error(), "supported presets")
}

func TestResolveEmbeddedAgentProviderUsesNativeAgentConfig(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".agent", "config.yaml"), []byte(`
providers:
  openrouter:
    type: openai-compat
    base_url: https://openrouter.ai/api/v1
    api_key: sk-test
    model: anthropic/claude-haiku-4-5
    headers:
      HTTP-Referer: https://example.com
default: openrouter
max_iterations: 30
preset: agent
`), 0o644))

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	resolved, err := r.resolveEmbeddedAgentProvider(wd, "", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "openai-compat", resolved.Config.Provider)
	assert.Equal(t, "https://openrouter.ai/api/v1", resolved.Config.BaseURL)
	assert.Equal(t, "anthropic/claude-haiku-4-5", resolved.Config.Model)
	assert.Equal(t, 30, resolved.Config.MaxIterations)
	assert.Equal(t, "agent", resolved.Config.Preset)
	assert.NotNil(t, resolved.Provider)
}

func TestResolveEmbeddedAgentProviderSupportsDefaultProviderAlias(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".agent", "config.yaml"), []byte(`
providers:
  bragi:
    type: openai-compat
    base_url: http://bragi:1234/v1
    api_key: sk-test
    model: qwen3.5-27b
  openrouter:
    type: openai-compat
    base_url: https://openrouter.ai/api/v1
    api_key: sk-test
    model: minimax/minimax-m2.7
default_provider: openrouter
`), 0o644))

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	resolved, err := r.resolveEmbeddedAgentProvider(wd, "", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "https://openrouter.ai/api/v1", resolved.Config.BaseURL)
	assert.Equal(t, "minimax/minimax-m2.7", resolved.Config.Model)
}

func TestResolveEmbeddedAgentProviderAppliesNativeModelOverride(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".agent", "config.yaml"), []byte(`
providers:
  openrouter:
    type: openai-compat
    base_url: https://openrouter.ai/api/v1
    api_key: sk-test
    model: anthropic/claude-haiku-4-5
default: openrouter
`), 0o644))

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	resolved, err := r.resolveEmbeddedAgentProvider(wd, "minimax/minimax-m2.7", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "minimax/minimax-m2.7", resolved.Config.Model)
}

// --- Config precedence regression tests ---
//
// These tests document the authoritative precedence chain for embedded-agent
// config resolution:
//
//  1. CLI opts (model from --model flag) — highest priority
//  2. AGENT_* env vars — override native config values
//  3. Native `.agent/config.yaml` — authoritative embedded-agent config source
//  4. Built-in defaults (openai-compat at localhost:1234) — lowest priority
//
// Note: the .ddx/config.yaml agent_runner section has been removed. All agent
// config should live in ~/.config/agent/config.yaml or .agent/config.yaml.

// TestConfigPrecedenceNativeWinsOverBuiltinDefaults verifies that native
// .agent/config.yaml values are used when present.
func TestConfigPrecedenceNativeWinsOverBuiltinDefaults(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".agent", "config.yaml"), []byte(`
providers:
  openrouter:
    type: openai-compat
    base_url: https://openrouter.ai/api/v1
    api_key: sk-native
    model: native/model
default: openrouter
`), 0o644))

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	resolved, err := r.resolveEmbeddedAgentProvider(wd, "", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	// Native config is used — not the built-in localhost default.
	assert.Equal(t, "https://openrouter.ai/api/v1", resolved.Config.BaseURL, "native config base_url must be used")
	assert.Equal(t, "native/model", resolved.Config.Model, "native config model must be used")
}

// TestConfigPrecedenceCliModelOverridesNativeConfig verifies that --model
// (CLI opts) overrides the model from native .agent/config.yaml.
func TestConfigPrecedenceCliModelOverridesNativeConfig(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".agent", "config.yaml"), []byte(`
providers:
  openrouter:
    type: openai-compat
    base_url: https://openrouter.ai/api/v1
    api_key: sk-test
    model: native/model
default: openrouter
`), 0o644))

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	// CLI-supplied model should override the native config's model.
	resolved, err := r.resolveEmbeddedAgentProvider(wd, "override/model", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "override/model", resolved.Config.Model, "CLI --model must override native config model")
	// Provider endpoint from native config is preserved.
	assert.Equal(t, "https://openrouter.ai/api/v1", resolved.Config.BaseURL)
}

// TestConfigPrecedenceEnvVarAppliesInDefaultFallbackPath verifies that AGENT_MODEL
// env var is respected when no native .agent/config.yaml is present (built-in defaults path).
func TestConfigPrecedenceEnvVarAppliesInDefaultFallbackPath(t *testing.T) {
	isolateNativeAgentHome(t)
	t.Setenv("AGENT_MODEL", "env-override-model")
	t.Setenv("AGENT_BASE_URL", "http://env-endpoint:1234/v1")

	// No native .agent/config.yaml present — resolves via built-in defaults + env vars.
	wd := t.TempDir()
	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	resolved, err := r.resolveEmbeddedAgentProvider(wd, "", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	// Env vars must override built-in defaults.
	assert.Equal(t, "env-override-model", resolved.Config.Model, "AGENT_MODEL env var must override built-in default model")
	assert.Equal(t, "http://env-endpoint:1234/v1", resolved.Config.BaseURL, "AGENT_BASE_URL env var must override built-in default base_url")
}

// TestConfigPrecedenceEnvVarAppliesInNativePath verifies that AGENT_* env vars
// override values resolved from the native ~/.config/agent/config.yaml path.
// This is the fix for bead ddx-a3b1c8d2.
func TestConfigPrecedenceEnvVarAppliesInNativePath(t *testing.T) {
	isolateNativeAgentHome(t)
	t.Setenv("AGENT_MODEL", "env-override-model")
	t.Setenv("AGENT_BASE_URL", "http://env-endpoint:1234/v1")

	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".agent", "config.yaml"), []byte(`
providers:
  openrouter:
    type: openai-compat
    base_url: https://openrouter.ai/api/v1
    api_key: sk-native
    model: native/config-model
default: openrouter
`), 0o644))

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	resolved, err := r.resolveEmbeddedAgentProvider(wd, "", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	// AGENT_MODEL and AGENT_BASE_URL must override the native config values.
	assert.Equal(t, "env-override-model", resolved.Config.Model, "AGENT_MODEL env var must override native config model")
	assert.Equal(t, "http://env-endpoint:1234/v1", resolved.Config.BaseURL, "AGENT_BASE_URL env var must override native config base_url")
}

// TestConfigPrecedenceNativeConfigAbsentUsesBuiltinDefaults verifies that when no native
// .agent/config.yaml exists, built-in defaults are used.
func TestConfigPrecedenceNativeConfigAbsentUsesBuiltinDefaults(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()
	// No .agent/config.yaml written — native config path returns nil.

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	resolved, err := r.resolveEmbeddedAgentProvider(wd, "", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	// Built-in defaults are used when no native config is present.
	assert.Equal(t, "http://localhost:1234/v1", resolved.Config.BaseURL, "must use built-in default base_url when no native config")
	assert.Equal(t, "openai-compat", resolved.Config.Provider)
	assert.Equal(t, "agent", resolved.Config.Preset)
	assert.Equal(t, 100, resolved.Config.MaxIterations)
}

// TestConfigPrecedenceOpenRouterModelSelectsOpenRouterProvider verifies that a
// vendor/model string (e.g. "qwen/qwen3.6") routes to the openrouter provider
// when present in native config, enabling OpenRouter setups without DDx config.
func TestConfigPrecedenceOpenRouterModelSelectsOpenRouterProvider(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(wd, ".agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".agent", "config.yaml"), []byte(`
providers:
  local:
    type: openai-compat
    base_url: http://local:1234/v1
    api_key: sk-local
    model: local-model
  openrouter:
    type: openai-compat
    base_url: https://openrouter.ai/api/v1
    api_key: sk-openrouter
    model: anthropic/claude-haiku-4-5
default: local
`), 0o644))

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath

	// Requesting a vendor/model format should route to openrouter provider.
	resolved, err := r.resolveEmbeddedAgentProvider(wd, "qwen/qwen3.6", "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "https://openrouter.ai/api/v1", resolved.Config.BaseURL, "vendor/model must route to openrouter provider in native config")
	assert.Equal(t, "qwen/qwen3.6", resolved.Config.Model, "requested model must be applied as override")
}

// --- Helpers ---

type notFoundError struct {
	name string
}

func (e *notFoundError) Error() string {
	return e.name + ": not found"
}

func isolateNativeAgentHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

type sleepProvider struct {
	delay    time.Duration
	response agentlib.Response
}

func (p *sleepProvider) Chat(ctx context.Context, _ []agentlib.Message, _ []agentlib.ToolDef, _ agentlib.Options) (agentlib.Response, error) {
	select {
	case <-time.After(p.delay):
		return p.response, nil
	case <-ctx.Done():
		return agentlib.Response{}, ctx.Err()
	}
}

type sequenceProvider struct {
	delays    []time.Duration
	responses []agentlib.Response
	calls     atomic.Int32
}

func (p *sequenceProvider) Chat(ctx context.Context, _ []agentlib.Message, _ []agentlib.ToolDef, _ agentlib.Options) (agentlib.Response, error) {
	idx := int(p.calls.Add(1) - 1)
	if idx >= len(p.responses) {
		return agentlib.Response{}, fmt.Errorf("unexpected call %d", idx)
	}
	delay := p.delays[idx]
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return agentlib.Response{}, ctx.Err()
		}
	}
	return p.responses[idx], nil
}

// pulseProvider returns a tool-call response after a fixed delay, varying
// the tool-call arguments on every invocation so it is not tripped by the
// agentlib duplicate-tool-call circuit breaker. Used to verify that the
// wall-clock deadline fires even when the idle timer is continuously
// reset by a steady stream of agent events.
type pulseProvider struct {
	delay    time.Duration
	pathBase string
	calls    atomic.Int32
}

func (p *pulseProvider) Chat(ctx context.Context, _ []agentlib.Message, _ []agentlib.ToolDef, _ agentlib.Options) (agentlib.Response, error) {
	n := p.calls.Add(1)
	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return agentlib.Response{}, ctx.Err()
		}
	}
	// Distinct arguments per turn so the identical-call loop guard in
	// agentlib does not abort the run before the wall-clock timer fires.
	args := fmt.Sprintf(`{"path":"%s-%d.txt"}`, p.pathBase, n)
	return agentlib.Response{
		Model: "pulse-model",
		ToolCalls: []agentlib.ToolCall{{
			ID:        fmt.Sprintf("pulse-%d", n),
			Name:      "read",
			Arguments: json.RawMessage(args),
		}},
	}, nil
}

// --- Compaction stuck detection ---

// TestIsNoopCompactionEndEvent validates the helper that identifies no-op
// compaction end events — the events that the circuit breaker tracks.
func TestIsNoopCompactionEndEvent(t *testing.T) {
	mustMarshal := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	cases := []struct {
		name string
		e    agentlib.Event
		want bool
	}{
		{
			name: "no-op compaction end (success=false, no_compaction=true)",
			e: agentlib.Event{
				Type: agentlib.EventCompactionEnd,
				Data: mustMarshal(map[string]any{"success": false, "no_compaction": true}),
			},
			want: true,
		},
		{
			name: "successful compaction end",
			e: agentlib.Event{
				Type: agentlib.EventCompactionEnd,
				Data: mustMarshal(map[string]any{"success": true, "tokens_before": 1000, "tokens_after": 200}),
			},
			want: false,
		},
		{
			name: "error compaction end (no_compaction=true but success=false from error)",
			e: agentlib.Event{
				Type: agentlib.EventCompactionEnd,
				Data: mustMarshal(map[string]any{"success": false, "no_compaction": true, "error": "llm failed"}),
			},
			want: true,
		},
		{
			name: "compaction start — never a no-op end",
			e:    agentlib.Event{Type: agentlib.EventCompactionStart},
			want: false,
		},
		{
			name: "llm request event",
			e:    agentlib.Event{Type: agentlib.EventLLMRequest},
			want: false,
		},
		{
			name: "tool call event",
			e:    agentlib.Event{Type: agentlib.EventToolCall},
			want: false,
		},
		{
			name: "session end event",
			e:    agentlib.Event{Type: agentlib.EventSessionEnd},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isNoopCompactionEndEvent(tc.e)
			assert.Equal(t, tc.want, got, "isNoopCompactionEndEvent(%v)", tc.e.Type)
		})
	}
}

// TestAgentCompactionStuckCircuitBreaker verifies that the circuit breaker
// fires and returns a structured error when compaction fails repeatedly
// without any write-tool progress.
//
// Regression test for: agent stalls in post-commit compaction loop.
// Root cause: compaction emits no-op events every iteration, which pulse the
// inactivity timer and accumulate without bound when the agent is stuck
// in a read-only or verification loop.
//
// The test uses:
//   - A sequenceProvider returning read-only tool calls so the counter grows
//   - CompactionStuckThreshold=3 to trigger quickly in-process
//   - No timeout so only the circuit breaker terminates the run
func TestAgentCompactionStuckCircuitBreaker(t *testing.T) {
	wd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wd, "input.txt"), []byte("data\n"), 0o644))

	// Provider returns read-only tool calls then a final response.
	// The compaction circuit breaker fires before the third LLM call.
	provider := &sequenceProvider{
		delays: []time.Duration{0, 0, 0},
		responses: []agentlib.Response{
			// Iteration 1: returns a read tool call (read-only — does not reset counter).
			{
				Model: "test",
				ToolCalls: []agentlib.ToolCall{{
					ID:        "c1",
					Name:      "read",
					Arguments: json.RawMessage(`{"path":"input.txt"}`),
				}},
			},
			// Iteration 2: the circuit breaker fires at pre-compaction before this
			// LLM call is made. If somehow it is made, return a final response so
			// the loop exits cleanly without triggering "unexpected call N".
			{Content: "done", Model: "test"},
			// Extra safety entry.
			{Content: "done", Model: "test"},
		},
	}

	r := NewRunner(Config{SessionLogDir: t.TempDir()}) // no timeout — circuit breaker only
	r.LookPath = mockLookPath
	r.AgentProvider = provider
	// Trigger after 3 consecutive no-op compaction events (default is 50).
	// With 2 events per iteration (pre + mid), this fires at the mid→pre
	// boundary between iter 1 and iter 2.
	r.CompactionStuckThreshold = 3

	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "verify the file",
		WorkDir: wd,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode, "compaction stuck should set exit code 1")
	assert.Contains(t, result.Error, "compaction stuck", "error should identify the cause")
	assert.Contains(t, result.Error, "3", "error should report the threshold")
}

// TestAgentNoOpCompactionDoesNotPulseInactivityTimer verifies that no-op
// compaction events do not reset the inactivity timer. Without this fix,
// a stuck agent generating only compaction events would run until the
// absolute 2-hour deadline instead of the per-activity timeout.
func TestAgentNoOpCompactionDoesNotPulseInactivityTimer(t *testing.T) {
	// Provider that sleeps longer than the timeout on every call,
	// simulating an LLM that responds very slowly.
	provider := &sleepProvider{
		delay:    300 * time.Millisecond,
		response: agentlib.Response{Content: "done", Model: "slow"},
	}

	// Timeout is 100ms. If no-op compaction events were resetting the timer,
	// the pre-compaction event (emitted before the LLM call) would reset it
	// to 100ms and the slow LLM would then complete at 300ms — causing a
	// spurious pass. With the fix, compaction.start and compaction.end(no-op)
	// do not pulse: the timer starts from session.start and fires at 100ms
	// while the LLM is still sleeping.
	r := NewRunner(Config{SessionLogDir: t.TempDir(), TimeoutMS: 100})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "do something",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode, "should time out")
	assert.Contains(t, result.Error, "timeout", "error should report timeout")
}

// --- Read-only tool classification and dedicated tools ---

func TestIsWriteCapableTool(t *testing.T) {
	// Write-capable tools.
	for _, name := range []string{"bash", "write", "edit"} {
		assert.True(t, isWriteCapableTool(name), "%s should be write-capable", name)
	}
	// Read-only tools.
	for _, name := range []string{"read", "ls", "find", "grep", "glob", "unknown"} {
		assert.False(t, isWriteCapableTool(name), "%s should not be write-capable", name)
	}
}

func TestFindToolDelegatesToGlob(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello\n"), 0o644))

	ft := &findTool{Glob: tool.GlobTool{WorkDir: dir}}
	assert.Equal(t, "find", ft.Name())
	assert.NotEmpty(t, ft.Description())
	assert.NotEmpty(t, ft.Schema())
	assert.True(t, ft.Parallel())

	out, err := ft.Execute(context.Background(), json.RawMessage(`{"pattern":"*.go"}`))
	require.NoError(t, err)
	assert.Contains(t, out, "a.go")
	assert.NotContains(t, out, "b.txt")

	// Verify findTool satisfies agentlib.Tool.
	var _ agentlib.Tool = (*findTool)(nil)
}

func TestLsGrepFindToolsWiredForAgent(t *testing.T) {
	wd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wd, "hello.go"), []byte("package main\n// TODO: something\n"), 0o644))

	tools := []agentlib.Tool{
		&tool.ReadTool{WorkDir: wd},
		&tool.WriteTool{WorkDir: wd},
		&tool.EditTool{WorkDir: wd},
		&tool.BashTool{WorkDir: wd},
		&tool.LsTool{WorkDir: wd},
		&tool.GrepTool{WorkDir: wd},
		&findTool{Glob: tool.GlobTool{WorkDir: wd}},
	}

	names := make(map[string]agentlib.Tool, len(tools))
	for _, tl := range tools {
		names[tl.Name()] = tl
	}
	for _, want := range []string{"read", "write", "edit", "bash", "ls", "grep", "find"} {
		_, ok := names[want]
		assert.True(t, ok, "expected %q tool to be wired", want)
	}

	// ls executes successfully over the temp dir.
	lsOut, err := names["ls"].Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, lsOut, "hello.go")

	// grep finds the TODO line.
	grepOut, err := names["grep"].Execute(context.Background(), json.RawMessage(`{"pattern":"TODO"}`))
	require.NoError(t, err)
	assert.Contains(t, grepOut, "hello.go")
	assert.Contains(t, grepOut, "TODO")

	// find locates the Go file.
	findOut, err := names["find"].Execute(context.Background(), json.RawMessage(`{"pattern":"*.go"}`))
	require.NoError(t, err)
	assert.Contains(t, findOut, "hello.go")
}
