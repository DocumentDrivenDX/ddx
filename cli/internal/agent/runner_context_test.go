package agent

import (
	"context"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAgentCancelledContextReturnsPromptly verifies that when the caller
// supplies an already-canceled context via RunOptions.Context, RunAgent
// returns within 500ms regardless of how long the provider would otherwise
// block. Regression anchor for ddx-0a651925 RC1: runner.Run and
// Runner.RunAgent previously discarded the caller's ctx by constructing
// their own context.WithCancel(context.Background()), so WorkerManager.Stop
// could not cancel the in-flight agent call.
func TestRunAgentCancelledContextReturnsPromptly(t *testing.T) {
	provider := &sleepProvider{
		delay: 30 * time.Second, // far larger than the 500ms AC deadline
		response: agentlib.Response{
			Content: "never-returned",
			Model:   "test-model",
		},
	}

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so the call returns on entry

	start := time.Now()
	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "test",
		Context: ctx,
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ExitCode, "canceled run should report non-zero exit")
	assert.NotEmpty(t, result.Error, "canceled run should carry error text")
	assert.Less(t, elapsed, 500*time.Millisecond,
		"canceled ctx must abort the provider call promptly; took %v", elapsed)
}

// TestRunAgentContextCancelledMidRunReturnsPromptly cancels the caller's
// context while the provider is blocked, then asserts that the runner
// unwinds within 500ms of cancellation. This exercises the propagation
// path (caller ctx → internal WithCancel(parent) → agentlib.Run → provider
// Chat) rather than the short-circuit pre-canceled case.
func TestRunAgentContextCancelledMidRunReturnsPromptly(t *testing.T) {
	provider := &sleepProvider{
		delay: 30 * time.Second,
		response: agentlib.Response{
			Content: "never-returned",
			Model:   "test-model",
		},
	}

	r := NewRunner(Config{SessionLogDir: t.TempDir()})
	r.LookPath = mockLookPath
	r.AgentProvider = provider

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a brief warmup so we measure the cancel→return latency.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	result, err := r.RunAgent(RunOptions{
		Harness: "agent",
		Prompt:  "test",
		Context: ctx,
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ExitCode)
	assert.Less(t, elapsed, 500*time.Millisecond,
		"caller cancel must propagate within 500ms; took %v", elapsed)
}

// TestRunCancelledContextReturnsPromptlySubprocess verifies the same
// cancellation contract for the subprocess dispatch path through Runner.Run.
// The OSExecutor honours ctx by sending SIGKILL on ctx.Done, so a canceled
// ctx must not leave the runner blocked on the child exit.
func TestRunCancelledContextReturnsPromptlySubprocess(t *testing.T) {
	// Use the mockExecutor (in-process fake) with a blocking delay. A
	// canceled context should short-circuit execution before any exec
	// happens, rather than waiting for the mock.
	exec := &blockingMockExecutor{block: 30 * time.Second}
	r := newTestRunner(&mockExecutor{output: "ok"})
	r.Executor = exec

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	result, err := r.Run(RunOptions{
		Harness: "codex",
		Prompt:  "hello",
		Context: ctx,
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Less(t, elapsed, 500*time.Millisecond,
		"canceled ctx must abort subprocess dispatch promptly; took %v", elapsed)
}

// blockingMockExecutor is an Executor whose ExecuteInDir respects ctx: it
// either blocks for `block` or returns ctx.Err() when ctx is canceled.
type blockingMockExecutor struct {
	block time.Duration
}

func (e *blockingMockExecutor) Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error) {
	return e.ExecuteInDir(ctx, binary, args, stdin, "")
}

func (e *blockingMockExecutor) ExecuteInDir(ctx context.Context, _ string, _ []string, _ string, _ string) (*ExecResult, error) {
	select {
	case <-time.After(e.block):
		return &ExecResult{Stdout: "", ExitCode: 0}, nil
	case <-ctx.Done():
		return &ExecResult{ExitCode: -1}, ctx.Err()
	}
}
