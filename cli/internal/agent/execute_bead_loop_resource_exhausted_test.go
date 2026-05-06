package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resourceExhaustedTestReport(beadID string) ExecuteBeadReport {
	return ExecuteBeadReport{
		BeadID:    beadID,
		AttemptID: "attempt-resource",
		WorkerID:  "worker-resource",
		Status:    ExecuteBeadStatusResourceExhausted,
		Detail:    ResourceExhaustedStopMessage,
		SessionID: "sess-resource",
		ResourceExhausted: &ExecutionResourceCheckResult{
			ProjectRoot:   "/project/root",
			TempRoot:      "/tmp/ddx-exec",
			EvidenceRoots: []string{"/project/root/.ddx/executions"},
			RootChecks: []ExecutionResourceRootCheck{
				{
					Path:           "/tmp/ddx-exec",
					Writable:       false,
					WritableReason: "no space left on device",
					Notes:          []string{"cleanup completed", "recheck still failed"},
				},
			},
			CleanupSummary: ExecutionCleanupSummary{
				ProjectRoot:                 "/project/root",
				TempRoot:                    "/tmp/ddx-exec",
				ScannedTempDirs:             1,
				ScannedEvidenceDirs:         1,
				CompleteEvidenceDirs:        0,
				RemovedUnregisteredTempDirs: 1,
				RemovedRunStateFiles:        1,
				BytesReclaimed:              1024,
				InodesReclaimed:             4,
			},
		},
	}
}

func TestExecuteBeadWorkerResourceExhaustedStopsLoop(t *testing.T) {
	inner, first, second := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			n := atomic.AddInt32(&execCalls, 1)
			if n > 1 {
				t.Fatalf("executor unexpectedly called for %s after resource exhaustion", beadID)
			}
			return resourceExhaustedTestReport(beadID), nil
		}),
	}

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Log:          &logBuf,
		Once:         false,
		PollInterval: 10 * time.Millisecond,
		ProjectRoot:  t.TempDir(),
		SessionID:    "sess-resource-loop",
		WorkerID:     "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls))
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, result.LastFailureStatus)
	assert.Equal(t, first.ID, result.Results[0].BeadID)
	assert.Contains(t, logBuf.String(), ResourceExhaustedStopMessage)

	gotFirst, err := inner.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, "open", gotFirst.Status)
	assert.Empty(t, gotFirst.Owner)

	gotSecond, err := inner.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, "open", gotSecond.Status)
	assert.Empty(t, gotSecond.Owner)
}

func TestExecuteBeadWorkerResourceExhaustedUnclaimsAndNoCooldown(t *testing.T) {
	inner, first, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return resourceExhaustedTestReport(beadID), nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        false,
		ProjectRoot: t.TempDir(),
		SessionID:   "sess-resource-loop",
		WorkerID:    "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	got, err := inner.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, "open", got.Status)
	assert.Empty(t, got.Owner)
	if got.Extra != nil {
		_, hasRetry := got.Extra["execute-loop-retry-after"]
		assert.False(t, hasRetry, "resource exhaustion must not write execute-loop-retry-after")
	}
}

func TestExecuteBeadWorkerResourceExhaustedLoopEndEvent(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	var eventSink bytes.Buffer
	var logSink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return resourceExhaustedTestReport(beadID), nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Log:         &logSink,
		EventSink:   &eventSink,
		Once:        false,
		ProjectRoot: t.TempDir(),
		SessionID:   "sess-resource-loop",
		WorkerID:    "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	lines := strings.Split(strings.TrimSpace(eventSink.String()), "\n")
	var byType = map[string][]map[string]any{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		typ, _ := entry["type"].(string)
		data, _ := entry["data"].(map[string]any)
		byType[typ] = append(byType[typ], data)
	}

	require.Len(t, byType["loop.end"], 1)
	assert.Equal(t, "resource_exhausted", byType["loop.end"][0]["exit_reason"])

	require.NotEmpty(t, byType["resource.exhausted"], "resource.exhausted loop event must be emitted")
	data := byType["resource.exhausted"][0]
	assert.Equal(t, first.ID, data["bead_id"])
	assert.NotEmpty(t, data["cleanup_summary"], "cleanup summary must be included in resource.exhausted event")
	assert.Equal(t, ResourceExhaustedStopMessage, result.Results[0].Detail)
}
