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
	t.Setenv("XDG_DATA_HOME", t.TempDir())
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

// TestExecuteBeadLoop_RepeatedDirtyRootEscalatesWithCooldown verifies that relaunching the
// worker against the same dirty path-set advances from the basic operator
// attention stop to the repeated/escalated stop with a cooldown field instead
// of immediately bouncing back out again.
func TestExecuteBeadLoop_RepeatedDirtyRootEscalatesWithCooldown(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	dirtyPaths := []string{"src/main.go", "README.md"}
	projectRoot := t.TempDir()
	var logBuf, eventBuf bytes.Buffer

	newWorker := func() *ExecuteBeadWorker {
		return &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatalf("executor must not be called when project root is dirty: %s", beadID)
				return ExecuteBeadReport{}, nil
			}),
		}
	}
	run := func() *ExecuteBeadLoopResult {
		result, err := newWorker().Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Log:         &logBuf,
			EventSink:   &eventBuf,
			Once:        false,
			ProjectRoot: projectRoot,
			SessionID:   "sess-dirty-root-relaunch",
			WorkerID:    "worker-dirty-root-relaunch",
			ProjectRootDirtyCheck: func(_ string) []string {
				return append([]string(nil), dirtyPaths...)
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		return result
	}

	first := run()
	require.NotNil(t, first.OperatorAttention)
	assert.Equal(t, "dirty_project_root", first.OperatorAttention.Reason)
	assert.Equal(t, []string{"README.md", "src/main.go"}, first.OperatorAttention.DirtyPaths)
	assert.Empty(t, first.OperatorAttention.RetryAfter)

	second := run()
	require.NotNil(t, second.OperatorAttention)
	assert.Equal(t, "dirty_project_root", second.OperatorAttention.Reason)
	assert.Equal(t, []string{"README.md", "src/main.go"}, second.OperatorAttention.DirtyPaths)
	assert.Empty(t, second.OperatorAttention.RetryAfter)

	third := run()
	require.NotNil(t, third.OperatorAttention)
	assert.Equal(t, "dirty_project_root_repeated", third.OperatorAttention.Reason)
	assert.Equal(t, []string{"README.md", "src/main.go"}, third.OperatorAttention.DirtyPaths)
	require.NotEmpty(t, third.OperatorAttention.RetryAfter)
	assert.Equal(t, "OperatorAttention", third.StopCondition)
	assert.Equal(t, "operator_attention", third.ExitReason)

	fourth := run()
	require.NotNil(t, fourth.OperatorAttention)
	assert.Equal(t, "dirty_project_root_repeated", fourth.OperatorAttention.Reason)
	assert.Equal(t, third.OperatorAttention.RetryAfter, fourth.OperatorAttention.RetryAfter,
		"relaunches during the cooldown should reuse the persisted backoff")
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls), "dirty root must never reach the claim path")

	// The escalated relaunch must emit operator-attention with retry_after set.
	var escalatedCount int
	for _, line := range strings.Split(strings.TrimSpace(eventBuf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.operator_attention" {
			continue
		}
		data, _ := entry["data"].(map[string]any)
		if data["reason"] == "dirty_project_root_repeated" && data["retry_after"] != "" {
			escalatedCount++
		}
	}
	assert.GreaterOrEqual(t, escalatedCount, 1, "expected at least one escalated dirty-root operator_attention event")
	assert.Contains(t, logBuf.String(), "relaunch suppressed")
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

	got, err := inner.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status)
}
