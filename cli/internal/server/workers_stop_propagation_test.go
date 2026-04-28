package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerManagerStopCancelsAgentExecution is the regression test for
// ddx-0a651925 RC1: WorkerManager.Stop(id) must cancel the in-flight
// executor (and transitively the agent provider call) via ctx propagation,
// not merely close the poll loop. Pre-fix, runner.Run / RunAgent discarded
// the caller ctx by using context.WithCancel(context.Background()), so
// cancelling the worker goroutine left the agent subprocess / HTTP call
// running until SIGKILL. With ctx threaded into RunArgs.Context, Stop's
// cancellation propagates into the executor within seconds.
//
// The fake executor here blocks on ctx (exactly like a real provider's
// Chat method), so a successful test proves cancellation reaches the
// provider boundary.
func TestWorkerManagerStopCancelsAgentExecution(t *testing.T) {
	root := t.TempDir()
	setupBeadStoreWithReadyBead(t, root)

	var executorStarted atomic.Bool
	var executorCtxErr atomic.Value // stores the ctx.Err() observed when the executor unblocks
	executorDone := make(chan struct{})

	m := NewWorkerManager(root)
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				executorStarted.Store(true)
				// Block on ctx — mirrors what a real agent provider HTTP call
				// does when ctx is threaded all the way down.
				select {
				case <-ctx.Done():
					executorCtxErr.Store(ctx.Err())
				case <-time.After(60 * time.Second):
					// If we get here, ctx cancellation never propagated.
				}
				close(executorDone)
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: "canceled",
				}, nil
			}),
		}
	}

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		PollInterval: 30 * time.Second,
	})
	require.NoError(t, err)

	// Wait until the executor has actually started running so we know the
	// cancellation we exercise is against an in-flight call.
	require.Eventually(t, executorStarted.Load, 3*time.Second, 20*time.Millisecond,
		"executor never entered the blocking Chat call")

	stopStart := time.Now()
	require.NoError(t, m.Stop(record.ID))

	select {
	case <-executorDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor was not canceled within 2s of WorkerManager.Stop")
	}
	elapsed := time.Since(stopStart)
	assert.Less(t, elapsed, 2*time.Second,
		"Stop should cancel the running agent call within 2s; took %v", elapsed)

	observedErr, _ := executorCtxErr.Load().(error)
	assert.ErrorIs(t, observedErr, context.Canceled,
		"executor should observe context.Canceled from the caller ctx; got %v", observedErr)

	// Ensure the worker goroutine also exits.
	_ = waitForWorkerExit(t, m, record.ID, 5*time.Second)
}
