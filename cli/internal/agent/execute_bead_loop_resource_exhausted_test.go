package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticLoopResourceChecker struct {
	calls  int32
	result ExecutionResourceCheckResult
	err    error
}

func (c *staticLoopResourceChecker) Check(ctx context.Context) (ExecutionResourceCheckResult, error) {
	_ = ctx
	atomic.AddInt32(&c.calls, 1)
	return c.result, c.err
}

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
		Mode:         executeloop.ModeWatch,
		IdleInterval: 10 * time.Millisecond,
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
		_, hasRetry := got.Extra["work-retry-after"]
		assert.False(t, hasRetry, "resource exhaustion must not write work-retry-after")
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
	assert.Equal(t, "ResourceExhausted", result.StopCondition)
	assert.Equal(t, "resource_exhausted", result.ExitReason)

	require.NotEmpty(t, byType["resource.exhausted"], "resource.exhausted loop event must be emitted")
	data := byType["resource.exhausted"][0]
	assert.Equal(t, first.ID, data["bead_id"])
	assert.NotEmpty(t, data["cleanup_summary"], "cleanup summary must be included in resource.exhausted event")
	assert.Equal(t, ResourceExhaustedStopMessage, result.Results[0].Detail)
}

func TestWorkResourcePreflight_ContinuesAfterCleanupRestoresBudget(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "ddx-exec-wt")
	inner, first, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	healthy := false
	runner := &fakeExecutionCleanupRunner{}
	checker := &ExecutionResourcePreflight{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ".ddx", "executions"),
		},
		SoftMinFreeBytes:  100,
		SoftMinFreeInodes: 100,
		HardMinFreeBytes:  10,
		HardMinFreeInodes: 10,
		CleanupRunner: &cleanupTogglingRunner{
			inner: runner,
			onCleanup: func() {
				healthy = true
			},
		},
		RootProbe: func(path string) (ExecutionResourceRootCheck, error) {
			check := ExecutionResourceRootCheck{
				Path:       path,
				Writable:   true,
				BytesFree:  50,
				InodesFree: 50,
			}
			if healthy {
				check.BytesFree = 150
				check.InodesFree = 150
			}
			return check, nil
		},
	}

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "sess-resource-ok", ResultRev: "abc123"}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:            true,
		ProjectRoot:     projectRoot,
		ResourceChecker: checker,
		SessionID:       "sess-resource-restored",
		WorkerID:        "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, runner.calls)
	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls))

	got, err := inner.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status)
}

func TestWorkResourcePreflight_StopsBelowHardFloorAfterCleanup(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "ddx-exec-wt")
	inner, first, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	runner := &fakeExecutionCleanupRunner{}
	checker := &ExecutionResourcePreflight{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ".ddx", "executions"),
		},
		SoftMinFreeBytes:  100,
		SoftMinFreeInodes: 100,
		HardMinFreeBytes:  10,
		HardMinFreeInodes: 10,
		CleanupRunner:     runner,
		RootProbe: func(path string) (ExecutionResourceRootCheck, error) {
			return ExecutionResourceRootCheck{
				Path:       path,
				Writable:   true,
				BytesFree:  1,
				InodesFree: 1,
			}, nil
		},
	}

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Fatalf("executor must not run when pre-claim resource preflight fails for %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Log:             &logBuf,
		Once:            false,
		ProjectRoot:     projectRoot,
		ResourceChecker: checker,
		SessionID:       "sess-resource-hard-stop",
		WorkerID:        "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	assert.Equal(t, 1, runner.calls)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, result.LastFailureStatus)
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, result.Results[0].Status)
	assert.Equal(t, ResourceExhaustedStopMessage, result.Results[0].Detail)
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls))
	assert.Contains(t, logBuf.String(), ResourceExhaustedStopMessage)

	got, err := inner.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, "open", got.Status)
	assert.Empty(t, got.Owner)
}

func TestWorkResourcePreflight_ReportsBeforeAfterCapacity(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(projectRoot, ".ddx", "tmp")
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	checkResult := ExecutionResourceCheckResult{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ".ddx", "executions"),
		},
		BeforeRootChecks: []ExecutionResourceRootCheck{
			{Path: tempRoot, Writable: true, BytesFree: 1, InodesFree: 2},
		},
		RootChecks: []ExecutionResourceRootCheck{
			{Path: tempRoot, Writable: true, BytesFree: 3, InodesFree: 4, Notes: []string{"free bytes 3 < required 10"}},
		},
		CleanupSummary: ExecutionCleanupSummary{
			ProjectRoot:                 projectRoot,
			TempRoot:                    tempRoot,
			RemovedUnregisteredTempDirs: 1,
			BytesReclaimed:              5,
			InodesReclaimed:             6,
		},
	}
	checker := &staticLoopResourceChecker{
		result: checkResult,
		err: &ResourceExhaustedError{
			Detail: "temp root still below hard floor",
			Result: checkResult,
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run after pre-claim resource exhaustion: %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	var eventSink bytes.Buffer
	var logSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Log:             &logSink,
		EventSink:       &eventSink,
		Once:            false,
		ProjectRoot:     projectRoot,
		ResourceChecker: checker,
		SessionID:       "sess-resource-capacity",
		WorkerID:        "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, logSink.String(), "before=["+tempRoot+" bytes_free=1 inodes_free=2")
	assert.Contains(t, logSink.String(), "after=["+tempRoot+" bytes_free=3 inodes_free=4")
	assert.Contains(t, logSink.String(), "cleanup_reclaimed_bytes=5")
	assert.Contains(t, logSink.String(), "cleanup_reclaimed_inodes=6")

	lines := strings.Split(strings.TrimSpace(eventSink.String()), "\n")
	var preflight, exhausted map[string]any
	for _, line := range lines {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		data, _ := entry["data"].(map[string]any)
		switch entry["type"] {
		case "resource.preflight":
			preflight = data
		case "resource.exhausted":
			exhausted = data
		}
	}

	require.NotNil(t, preflight)
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, preflight["status"])
	assert.Equal(t, float64(5), preflight["cleanup_bytes_reclaimed"])
	assert.Equal(t, float64(6), preflight["cleanup_inodes_reclaimed"])
	assert.NotEmpty(t, preflight["root_checks_before"])
	assert.NotEmpty(t, preflight["root_checks_after"])

	require.NotNil(t, exhausted)
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, exhausted["status"])
	assert.Equal(t, float64(5), exhausted["cleanup_bytes_reclaimed"])
	assert.Equal(t, float64(6), exhausted["cleanup_inodes_reclaimed"])
	assert.NotEmpty(t, exhausted["root_checks_before"])
	assert.NotEmpty(t, exhausted["root_checks_after"])
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls))
}
