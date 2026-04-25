package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentRunRuntimeDelegation verifies that Runner.RunWithConfig threads
// every run-path durable knob from a sealed ResolvedConfig through to the
// underlying Run path, and that runtime intent (Prompt, WorkDir,
// Correlation) flows from the AgentRunRuntime. SD-024 Stage 2 / Bead 17.
func TestAgentRunRuntimeDelegation(t *testing.T) {
	opts := config.TestRunConfigOpts{
		Harness:     "codex",
		Model:       "gpt-arrt",
		Permissions: "unrestricted",
		Timeout:     30 * time.Second,
	}
	cfg := config.NewTestConfigForRun(opts)
	rcfg := cfg.Resolve(config.CLIOverrides{Effort: "high"})

	mock := &mockExecutor{output: "agent output here\n"}
	r := newTestRunner(mock)

	runtime := AgentRunRuntime{
		Prompt:      "do stuff",
		WorkDir:     "/tmp/wd-arrt",
		Correlation: map[string]string{"trace_id": "abc"},
	}

	result, err := r.RunWithConfig(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "codex", result.Harness)
	assert.Equal(t, "gpt-arrt", result.Model)

	// Args derived from rcfg + runtime must reach the executor.
	joined := strings.Join(mock.lastArgs, " ")
	assert.Contains(t, joined, "gpt-arrt", "model from rcfg must reach harness args")
	assert.Contains(t, joined, "reasoning.effort=high", "effort from rcfg overrides must reach harness args")
	assert.Contains(t, joined, "/tmp/wd-arrt", "WorkDir from runtime must reach harness args")
	assert.Contains(t, joined, "do stuff", "Prompt from runtime must reach harness args")
	// Permissions=unrestricted resolves codex's --dangerously-bypass flag.
	assert.Contains(t, joined, "--dangerously-bypass-approvals-and-sandbox",
		"permissions from rcfg must reach harness args")
}

// TestAgentRunRuntimeDelegation_ZeroValueRcfgPanics confirms RunWithConfig
// refuses an unsealed ResolvedConfig — the SD-024 sealed-construction
// invariant flows through to the run path.
func TestAgentRunRuntimeDelegation_ZeroValueRcfgPanics(t *testing.T) {
	r := newTestRunner(&mockExecutor{output: "ok"})
	defer func() {
		rec := recover()
		require.NotNil(t, rec, "RunWithConfig with zero-value ResolvedConfig must panic via requireSealed")
	}()
	var rcfg config.ResolvedConfig
	_, _ = r.RunWithConfig(context.Background(), rcfg, AgentRunRuntime{Prompt: "x"})
}
