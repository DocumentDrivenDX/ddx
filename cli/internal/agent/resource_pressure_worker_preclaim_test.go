package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticResourcePressureChecker returns a fixed ResourcePressureReport so
// tests can drive the pre-claim pressure diagnostic without probing real
// process fd counts.
type staticResourcePressureChecker struct {
	report ResourcePressureReport
	err    error
}

func (c *staticResourcePressureChecker) Check(ctx context.Context) (ResourcePressureReport, error) {
	_ = ctx
	return c.report, c.err
}

func decodeLoopEventsByType(t *testing.T, eventSink *bytes.Buffer) map[string][]map[string]any {
	t.Helper()
	byType := map[string][]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(eventSink.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		typ, _ := entry["type"].(string)
		data, _ := entry["data"].(map[string]any)
		byType[typ] = append(byType[typ], data)
	}
	return byType
}

// TestResourcePressure_WorkerPreclaimEmitsWarnBeforeClaim verifies that FD
// pressure at or above the warn threshold (80%) is surfaced as a typed
// "loop.resource_pressure" diagnostic before claim, and that the worker
// continues to claim and execute the bead rather than stopping.
func TestResourcePressure_WorkerPreclaimEmitsWarnBeforeClaim(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	checker := &staticResourcePressureChecker{
		report: ResourcePressureReport{
			ResourcePressureCheck: CheckFDUsage(80, 100),
		},
	}

	var execCalls int
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			execCalls++
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "sess-pressure-warn", ResultRev: "abc123"}, nil
		}),
	}

	var eventSink bytes.Buffer
	var logSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Log:                     &logSink,
		EventSink:               &eventSink,
		Once:                    true,
		ProjectRoot:             t.TempDir(),
		ResourcePressureChecker: checker,
		SessionID:               "sess-pressure-warn",
		WorkerID:                "worker-pressure",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, execCalls, "worker must still claim and execute the bead")
	assert.Equal(t, 1, result.Successes)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status, "warn-level pressure must not block claim/close")

	byType := decodeLoopEventsByType(t, &eventSink)
	require.NotEmpty(t, byType["loop.resource_pressure"], "loop.resource_pressure event must be emitted")
	data := byType["loop.resource_pressure"][0]
	assert.Equal(t, "pre-claim", data["phase"])
	assert.Equal(t, string(ResourcePressureWarn), data["severity"])
	assert.EqualValues(t, 80, data["fd_used"])
	assert.EqualValues(t, 100, data["fd_limit"])

	assert.Empty(t, byType["loop.operator_attention"], "warn-level pressure must not escalate to operator_attention")
	assert.Contains(t, logSink.String(), "resource pressure")
}

// TestResourcePressure_WorkerPreclaimEmitsOperatorAttentionAtNinetyPercent
// verifies that FD pressure at or above the operator_attention threshold
// (90%) emits a typed operator_attention diagnostic carrying fd_used,
// fd_limit, worker subprocess count, temp worktree count, and stale
// execution dir count, without stopping the worker.
func TestResourcePressure_WorkerPreclaimEmitsOperatorAttentionAtNinetyPercent(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	checker := &staticResourcePressureChecker{
		report: ResourcePressureReport{
			ResourcePressureCheck:  CheckFDUsage(90, 100),
			WorkerSubprocessCount:  3,
			TempWorktreeCount:      2,
			StaleExecutionDirCount: 5,
		},
	}

	var execCalls int
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			execCalls++
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "sess-pressure-attn", ResultRev: "abc123"}, nil
		}),
	}

	var eventSink bytes.Buffer
	var logSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Log:                     &logSink,
		EventSink:               &eventSink,
		Once:                    true,
		ProjectRoot:             t.TempDir(),
		ResourcePressureChecker: checker,
		SessionID:               "sess-pressure-attn",
		WorkerID:                "worker-pressure",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, execCalls, "operator_attention pressure must not stop the worker")
	assert.Equal(t, 1, result.Successes)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status)

	byType := decodeLoopEventsByType(t, &eventSink)
	require.NotEmpty(t, byType["loop.operator_attention"], "loop.operator_attention event must be emitted")

	var data map[string]any
	for _, candidate := range byType["loop.operator_attention"] {
		if candidate["reason"] == "resource_pressure_fd" {
			data = candidate
			break
		}
	}
	require.NotNil(t, data, "operator_attention must include a resource_pressure_fd entry")
	assert.EqualValues(t, 90, data["fd_used"])
	assert.EqualValues(t, 100, data["fd_limit"])
	assert.EqualValues(t, 3, data["worker_subprocess_count"])
	assert.EqualValues(t, 2, data["temp_worktree_count"])
	assert.EqualValues(t, 5, data["stale_execution_dir_count"])
}
