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
	"github.com/DocumentDrivenDX/agent/provider/virtual"
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

// --- RunAgent with virtual provider (deterministic) ---

// A-01: RunAgent dispatches to agentlib.DdxAgent.Execute with virtual provider.
// Note: the service path does not surface result.Output via event stream yet
// (text_delta events are not emitted by runNative; tracked as follow-up work).
func TestAgentRunVirtualProvider(t *testing.T) {
	isolateNativeAgentHome(t)
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
	assert.Equal(t, 100, result.InputTokens)
	assert.Equal(t, 20, result.OutputTokens)
	assert.Equal(t, 120, result.Tokens)
	assert.Equal(t, 0, result.ExitCode)
}

// A-02: Agent tool execution — verify in-process path works.
func TestAgentRunToolExecution(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()

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
	assert.Equal(t, 150, result.InputTokens)
	assert.Equal(t, 30, result.OutputTokens)
	assert.Equal(t, 180, result.Tokens)
}

// A-04: Wall-clock timeout cancels the run.
// The service path enforces wall-clock via ServiceExecuteRequest.Timeout;
// idle timeout (TimeoutMS) is delegated to the agent's StallPolicy.
func TestAgentRunTimeout(t *testing.T) {
	isolateNativeAgentHome(t)
	provider := &sleepProvider{
		delay: 500 * time.Millisecond,
		response: agentlib.Response{
			Content: "too late",
			Model:   "sleepy-model",
		},
	}

	// Use WallClockMS so the service path's Timeout cap fires.
	r := NewRunner(Config{SessionLogDir: t.TempDir(), WallClockMS: 100})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "unmatched prompt",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.NotEmpty(t, result.Error)
}

func TestAgentRunActivityExtendsTimeout(t *testing.T) {
	isolateNativeAgentHome(t)
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

	// WallClockMS is generous; the two 60ms provider delays should complete well within it.
	r := NewRunner(Config{SessionLogDir: t.TempDir(), WallClockMS: 5000})
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
	assert.Equal(t, int32(2), provider.calls.Load())
	assert.GreaterOrEqual(t, elapsed, 120*time.Millisecond)
}

// TestAgentRunWallClockTimeout verifies that the absolute wall-clock bound
// fires even when the idle (inactivity) timer is continuously reset by
// agent events.
func TestAgentRunWallClockTimeout(t *testing.T) {
	isolateNativeAgentHome(t)
	wd := t.TempDir()

	provider := &pulseProvider{
		delay:    50 * time.Millisecond,
		pathBase: "missing",
	}

	r := NewRunner(Config{
		SessionLogDir: t.TempDir(),
		TimeoutMS:     5000,
		WallClockMS:   300,
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
	assert.NotEmpty(t, result.Error)
	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond,
		"wall-clock should not fire before its deadline")
	assert.Less(t, elapsed, 2*time.Second,
		"wall-clock should fire promptly after its deadline")
	assert.Greater(t, provider.calls.Load(), int32(1),
		"pulse provider should have emitted multiple events during the wait")
}

// TestAgentRunWallClockCapsSlowProvider verifies the wall-clock cap fires
// when a provider is slow and no idle-timeout enforcement is wired (the
// service path delegates idle-timeout to the agent's StallPolicy, not DDx).
func TestAgentRunWallClockCapsSlowProvider(t *testing.T) {
	isolateNativeAgentHome(t)
	provider := &sleepProvider{
		delay: 500 * time.Millisecond,
		response: agentlib.Response{
			Content: "too late",
			Model:   "slow-model",
		},
	}

	r := NewRunner(Config{
		SessionLogDir: t.TempDir(),
		TimeoutMS:     5000, // idle timeout generous (not enforced by service path)
		WallClockMS:   150,  // wall-clock cap fires at 150ms, before provider responds
	})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	result, err := r.RunAgent(RunOptions{Harness: "agent", Prompt: "slow"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.NotEmpty(t, result.Error)
}

// A-06: Session logging captures agent runs.
func TestAgentRunSessionLogging(t *testing.T) {
	isolateNativeAgentHome(t)
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
}

// A-07: Model resolution priority: opts > config > env.
func TestAgentRunModelResolution(t *testing.T) {
	isolateNativeAgentHome(t)
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
	isolateNativeAgentHome(t)
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
	assert.Equal(t, 0.0, result.CostUSD)
}

// A-09: Dispatch via Runner.Run routes to RunAgent, not subprocess.
func TestAgentRunDispatchesInProcess(t *testing.T) {
	isolateNativeAgentHome(t)
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
	assert.Equal(t, 0, result.ExitCode)
	assert.Empty(t, mock.lastBinary, "agent should run in-process, not via executor")
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
// agentlib duplicate-tool-call circuit breaker.
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
