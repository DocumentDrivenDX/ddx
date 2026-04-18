package agent

import (
	"os"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/provider/virtual"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUseNewAgentPathFlag verifies that the dispatch helper honors both
// the Runner field and the env var, defaulting to off.
func TestUseNewAgentPathFlag(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: t.TempDir()})

	// Default: off.
	assert.False(t, r.useNewAgentPath())

	// Field on: enabled.
	r.UseNewAgentPath = true
	assert.True(t, r.useNewAgentPath())

	// Field off + env var enables.
	r.UseNewAgentPath = false
	t.Setenv("DDX_USE_NEW_AGENT_PATH", "1")
	assert.True(t, r.useNewAgentPath())

	t.Setenv("DDX_USE_NEW_AGENT_PATH", "0")
	assert.False(t, r.useNewAgentPath())

	t.Setenv("DDX_USE_NEW_AGENT_PATH", "false")
	assert.False(t, r.useNewAgentPath())
}

// TestRunAgentNewPathDispatchesToService verifies that with the new path
// enabled, RunAgent drives the request through the agentlib.DdxAgent
// service surface and produces a Result with the expected output and
// usage drained from the service event stream.
func TestRunAgentNewPathDispatchesToService(t *testing.T) {
	isolateNativeAgentHome(t)

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "service path test",
			Response: agentlib.Response{
				Content: "service-routed",
				Model:   "service-model",
				Usage:   agentlib.TokenUsage{Input: 75, Output: 12, Total: 87},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider
	r.UseNewAgentPath = true

	wd := t.TempDir()
	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "service path test",
		WorkDir: wd,
	})
	require.NoError(t, err)
	assert.Equal(t, "agent", result.Harness)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, 75, result.InputTokens)
	assert.Equal(t, 12, result.OutputTokens)
	assert.Equal(t, 87, result.Tokens)
}

// TestRunAgentNewPathHonorsEnvVar confirms an opt-in flip via env var
// also routes RunAgent through the new service path. This guards against
// the dispatch shim ignoring the env var.
func TestRunAgentNewPathHonorsEnvVar(t *testing.T) {
	isolateNativeAgentHome(t)
	t.Setenv("DDX_USE_NEW_AGENT_PATH", "1")

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "env flip test",
			Response: agentlib.Response{
				Content: "via-env",
				Model:   "env-model",
				Usage:   agentlib.TokenUsage{Input: 42, Output: 7, Total: 49},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider
	// UseNewAgentPath field stays default (false) — env var must flip it.
	require.False(t, r.UseNewAgentPath)

	wd := t.TempDir()
	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "env flip test",
		WorkDir: wd,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, 42, result.InputTokens)
	assert.Equal(t, 7, result.OutputTokens)
}

// TestRunAgentLegacyPathStillDefault asserts the default dispatch path
// is unchanged (i.e. the legacy in-package loop runs when neither the
// env var nor the field is set). This is the production-safety guard.
func TestRunAgentLegacyPathStillDefault(t *testing.T) {
	// Belt-and-braces: ensure no env var leaks from the test runner.
	require.NoError(t, os.Unsetenv("DDX_USE_NEW_AGENT_PATH"))

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "legacy default",
			Response: agentlib.Response{
				Content: "legacy",
				Model:   "legacy-model",
				Usage:   agentlib.TokenUsage{Input: 5, Output: 1, Total: 6},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider
	require.False(t, r.UseNewAgentPath)
	require.False(t, r.useNewAgentPath())

	result, err := r.RunAgent(RunOptions{Harness: "agent", Prompt: "legacy default"})
	require.NoError(t, err)
	assert.Equal(t, "legacy", result.Output)
	assert.Equal(t, 5, result.InputTokens)
}
