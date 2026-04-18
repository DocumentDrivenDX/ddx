package agent

import (
	"os"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/provider/virtual"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUseNewAgentPathFlag verifies that the dispatch helper is on by default
// and can be disabled via the DDX_USE_NEW_AGENT_PATH env var.
func TestUseNewAgentPathFlag(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: t.TempDir()})

	// Default: on.
	assert.True(t, r.useNewAgentPath())

	// Env var disables.
	t.Setenv("DDX_USE_NEW_AGENT_PATH", "0")
	assert.False(t, r.useNewAgentPath())

	t.Setenv("DDX_USE_NEW_AGENT_PATH", "false")
	assert.False(t, r.useNewAgentPath())

	// Env var cleared: back to on.
	require.NoError(t, os.Unsetenv("DDX_USE_NEW_AGENT_PATH"))
	assert.True(t, r.useNewAgentPath())
}

// TestRunAgentNewPathDispatchesToService verifies that RunAgent drives the
// request through the agentlib.DdxAgent service surface and produces a Result
// with the expected output and usage drained from the service event stream.
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

// TestRunAgentNewPathHonorsEnvVarDisable confirms the env var DDX_USE_NEW_AGENT_PATH=0
// acts as an emergency escape hatch and still routes through the service path
// (since the legacy path was removed in ddx-d224671d, the fallback also calls
// runAgentViaService).
func TestRunAgentNewPathHonorsEnvVarDisable(t *testing.T) {
	isolateNativeAgentHome(t)
	t.Setenv("DDX_USE_NEW_AGENT_PATH", "0")

	provider := virtual.New(virtual.Config{
		InlineResponses: []virtual.InlineResponse{{
			PromptMatch: "env disable test",
			Response: agentlib.Response{
				Content: "via-service",
				Model:   "env-model",
				Usage:   agentlib.TokenUsage{Input: 42, Output: 7, Total: 49},
			},
		}},
	})

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	wd := t.TempDir()
	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "env disable test",
		WorkDir: wd,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, 42, result.InputTokens)
	assert.Equal(t, 7, result.OutputTokens)
}
