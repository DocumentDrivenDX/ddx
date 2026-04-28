package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunInternalDispatchesToExecutor verifies that the new private
// runInternal entry point reaches the harness executor with the
// expected binary/args/stdin — the same contract Run honours.
// SD-024 Stage 2 (B22 prereq): internal callers will dispatch through
// runInternal directly without manufacturing a ResolvedConfig.
func TestRunInternalDispatchesToExecutor(t *testing.T) {
	mock := &mockExecutor{output: "internal output\n"}
	r := newTestRunner(mock)

	result, err := r.runInternal(RunArgs{Harness: "codex", Prompt: "do stuff"})
	require.NoError(t, err)
	assert.Equal(t, "codex", mock.lastBinary)
	assert.Equal(t, "do stuff", mock.lastArgs[len(mock.lastArgs)-1])
	assert.Equal(t, "internal output\n", result.Output)
	assert.Equal(t, 0, result.ExitCode)
}

// TestRunInternalUnknownHarnessReturnsError checks that runInternal
// surfaces the same harness-resolution error Run does.
func TestRunInternalUnknownHarnessReturnsError(t *testing.T) {
	mock := &mockExecutor{output: "should not be called"}
	r := newTestRunner(mock)

	_, err := r.runInternal(RunArgs{Harness: "no-such-harness", Prompt: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown harness")
	assert.Equal(t, "", mock.lastBinary, "executor must not be invoked when harness resolution fails")
}

// TestRunInternalRequiresPrompt asserts the prompt-required guard
// fires through the runInternal path just like through Run.
func TestRunInternalRequiresPrompt(t *testing.T) {
	mock := &mockExecutor{output: "noop"}
	r := newTestRunner(mock)

	_, err := r.runInternal(RunArgs{Harness: "codex"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}

// TestRunInternalThreadsModelAndPermissions confirms the adapter
// carries the durable knobs (Model, Permissions) through to the
// resolved harness invocation. This is the contract subsequent B22
// migration beads rely on when swapping a caller from RunArgs to
// RunArgs.
func TestRunInternalThreadsModelAndPermissions(t *testing.T) {
	mock := &mockExecutor{output: "ok"}
	r := newTestRunner(mock)

	_, err := r.runInternal(RunArgs{
		Harness:     "codex",
		Prompt:      "task",
		Model:       "gpt-5.4",
		Permissions: "unrestricted",
	})
	require.NoError(t, err)
	assert.Contains(t, mock.lastArgs, "-m")
	assert.Contains(t, mock.lastArgs, "gpt-5.4")
	assert.Contains(t, mock.lastArgs, "--dangerously-bypass-approvals-and-sandbox",
		"unrestricted permissions should reach the codex bypass flag")
}

// TestRunArgsDeclared is a structural check that the RunArgs adapter
// type exists and carries the expected fields. Subsequent B22 beads
// migrate production callers onto this type.
func TestRunArgsDeclared(t *testing.T) {
	args := RunArgs{
		Harness:     "codex",
		Prompt:      "hello",
		Model:       "gpt-5.4",
		Provider:    "openai",
		Permissions: "safe",
	}
	assert.Equal(t, "codex", args.Harness)
	assert.Equal(t, "hello", args.Prompt)
	assert.Equal(t, "gpt-5.4", args.Model)
	assert.Equal(t, "openai", args.Provider)
	assert.Equal(t, "safe", args.Permissions)
}
