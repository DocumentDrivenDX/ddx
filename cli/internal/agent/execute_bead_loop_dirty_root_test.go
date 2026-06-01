package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirtyRootPreventsClaimLoop verifies that a non-empty ProjectRootDirtyCheck
// result stops the worker with a single operator-attention event and zero claim
// calls, instead of repeatedly claiming and failing in a churn loop.
func TestDirtyRootPreventsClaimLoop(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Fatalf("executor must not be called when project root is dirty: %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	var logBuf, eventBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Log:         &logBuf,
		EventSink:   &eventBuf,
		Once:        false,
		ProjectRoot: t.TempDir(),
		SessionID:   "sess-dirty-root",
		WorkerID:    "worker-dirty-root",
		ProjectRootDirtyCheck: func(_ string) []string {
			return []string{"README.md", "src/main.go"}
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls), "executor must not be called on dirty root")
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls), "claim must not be called on dirty root")
	assert.Equal(t, 0, result.Attempts)

	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "dirty_project_root", result.OperatorAttention.Reason)
	assert.Equal(t, []string{"README.md", "src/main.go"}, result.OperatorAttention.DirtyPaths)
	assert.Equal(t, "OperatorAttention", result.StopCondition)
	assert.Equal(t, "operator_attention", result.ExitReason)

	assert.Contains(t, logBuf.String(), "operator attention")
	assert.Contains(t, logBuf.String(), "uncommitted tracked changes")

	// Exactly one loop.operator_attention event with reason=dirty_project_root.
	var attentionCount int
	for _, line := range strings.Split(strings.TrimSpace(eventBuf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] == "loop.operator_attention" {
			if data, _ := entry["data"].(map[string]any); data["reason"] == "dirty_project_root" {
				attentionCount++
			}
		}
	}
	assert.Equal(t, 1, attentionCount, "exactly one dirty_project_root operator_attention event")
}

// TestDirtyRootCleanRootProceedsNormally verifies that a nil ProjectRootDirtyCheck
// result does not stop the worker — the loop proceeds to claim and execute.
func TestDirtyRootCleanRootProceedsNormally(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-clean-root",
				ResultRev: "abc1234",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
		SessionID:   "sess-clean-root",
		WorkerID:    "worker-clean-root",
		ProjectRootDirtyCheck: func(_ string) []string {
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Nil(t, result.OperatorAttention, "clean root must not trigger operator attention")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "clean root must claim bead")
	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls), "executor must run on clean root")

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status)
}
