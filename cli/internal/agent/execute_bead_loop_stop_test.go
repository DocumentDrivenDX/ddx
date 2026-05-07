package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadWorkerStopConditionWiredIntoRun(t *testing.T) {
	t.Run("drained explicit poll zero", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init())

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatalf("executor must not run for drained queue")
				return ExecuteBeadReport{}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
			PollInterval: 0,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.NoReadyWork)
		assert.Equal(t, "explicit_poll_zero", exitReason)
	})

	t.Run("once complete", func(t *testing.T) {
		store, _, _ := newExecuteLoopTestStore(t)
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-once-stop",
					ResultRev: "abc123",
				}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
			Once:         true,
			PollInterval: 30 * time.Second,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, "once_complete", exitReason)
	})

	t.Run("signal canceled", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatalf("executor must not run after cancellation")
				return ExecuteBeadReport{}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, ctx, ExecuteBeadLoopRuntime{
			PollInterval: 30 * time.Second,
		})
		require.ErrorIs(t, err, context.Canceled)
		require.NotNil(t, result)
		assert.Equal(t, "sigint", exitReason)
	})

	t.Run("budget stop before claim", func(t *testing.T) {
		inner, _, _ := newExecuteLoopTestStore(t)
		store := &claimCountingStore{Store: inner}
		var execCalls int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				atomic.AddInt32(&execCalls, 1)
				t.Fatalf("executor must not run after budget stop")
				return ExecuteBeadReport{}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
			BudgetStop: func() (ExecuteBeadReport, bool) {
				return ExecuteBeadReport{
					Status: ExecuteBeadStatusExecutionFailed,
					Detail: "cost cap reached",
				}, true
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "budget", exitReason)
		assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls))
		assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))
		require.Len(t, result.Results, 1)
		assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Results[0].Status)
		assert.Equal(t, "cost cap reached", result.Results[0].Detail)
	})
}

func runStopConditionCase(t *testing.T, worker *ExecuteBeadWorker, ctx context.Context, runtime ExecuteBeadLoopRuntime) (*ExecuteBeadLoopResult, string, error) {
	t.Helper()

	var sink bytes.Buffer
	runtime.EventSink = &sink
	runtime.SessionID = "sess-stop-condition"
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, runtime)
	exitReason := loopEndExitReason(t, sink.String())
	return result, exitReason, err
}

func loopEndExitReason(t *testing.T, raw string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.end" {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		require.True(t, ok, "loop.end data missing")
		reason, _ := data["exit_reason"].(string)
		return reason
	}
	require.Fail(t, "loop.end event not found", raw)
	return ""
}
