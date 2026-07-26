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
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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

	gotFirst, err := inner.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, "open", gotFirst.Status)
	assert.Empty(t, gotFirst.Owner)

	gotSecond, err := inner.Get(context.Background(), second.ID)
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

	got, err := inner.Get(context.Background(), first.ID)
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
			filepath.Join(projectRoot, ddxroot.DirName, "executions"),
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

	got, err := inner.Get(context.Background(), first.ID)
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
			filepath.Join(projectRoot, ddxroot.DirName, "executions"),
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

	got, err := inner.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, "open", got.Status)
	assert.Empty(t, got.Owner)
}

func TestWorkResourcePreflight_ReportsBeforeAfterCapacity(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(projectRoot, ddxroot.DirName, "tmp")
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	checkResult := ExecutionResourceCheckResult{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ddxroot.DirName, "executions"),
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

func runResourceExhaustedLoopEvent(t *testing.T, checkResult ExecutionResourceCheckResult, checkErr error) (ExecuteBeadReport, map[string]any, string) {
	t.Helper()
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	checker := &staticLoopResourceChecker{result: checkResult, err: checkErr}

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
		ProjectRoot:     t.TempDir(),
		ResourceChecker: checker,
		SessionID:       "sess-resource-fd",
		WorkerID:        "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	var exhausted map[string]any
	for _, line := range strings.Split(strings.TrimSpace(eventSink.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] == "resource.exhausted" {
			data, _ := entry["data"].(map[string]any)
			exhausted = data
		}
	}
	require.NotNil(t, exhausted, "resource.exhausted event must be emitted")

	return result.Results[0], exhausted, logSink.String()
}

// TestWorkLoopResourceExhaustedEMFILEReportsRestartableWorkerFailure proves
// that when the resource preflight fails because the worker process hit its
// open-file-descriptor limit (EMFILE/ENFILE), the work loop reports a
// structured fd_exhaustion diagnosis marked worker-local/restartable instead
// of the generic "resource_exhausted after cleanup" message.
func TestWorkLoopResourceExhaustedEMFILEReportsRestartableWorkerFailure(t *testing.T) {
	checkResult := ExecutionResourceCheckResult{
		ProjectRoot: "/project/root",
		TempRoot:    "/tmp/ddx-exec",
		RootChecks: []ExecutionResourceRootCheck{
			{
				Path:           "/tmp/ddx-exec",
				Writable:       false,
				WritableReason: "writability check failed: too many open files",
				FDExhausted:    true,
				FDCount:        1024,
				FDSoftLimit:    1024,
				FDHardLimit:    4096,
			},
		},
	}
	checkErr := &ResourceExhaustedError{Detail: "temp root fd exhausted", Result: checkResult}

	report, exhausted, logOutput := runResourceExhaustedLoopEvent(t, checkResult, checkErr)

	assert.Equal(t, ExecuteBeadStatusResourceExhausted, report.Status)
	assert.Equal(t, FDExhaustionStopMessage, report.Detail)
	assert.Equal(t, ResourceExhaustionDiagnosisFD, report.ResourceExhaustionDiagnosis)
	assert.True(t, report.ResourceExhaustionRestartable, "fd exhaustion must be reported as restartable")

	assert.Equal(t, ResourceExhaustionDiagnosisFD, exhausted["diagnosis"])
	assert.Equal(t, true, exhausted["restartable"])
	assert.Equal(t, true, exhausted["worker_local"])
	assert.Equal(t, FDExhaustionStopMessage, exhausted["detail"])

	assert.Contains(t, logOutput, FDExhaustionStopMessage)
	assert.NotContains(t, logOutput, ResourceExhaustedStopMessage)
}

// TestWorkLoopResourceExhaustedDiskPressureRemainsNonRestartableRootFailure
// proves that ordinary byte/inode or unwritable-root failures (no fd
// exhaustion involved) keep the existing generic, non-restartable
// resource_exhausted behavior.
func TestWorkLoopResourceExhaustedDiskPressureRemainsNonRestartableRootFailure(t *testing.T) {
	checkResult := ExecutionResourceCheckResult{
		ProjectRoot: "/project/root",
		TempRoot:    "/tmp/ddx-exec",
		RootChecks: []ExecutionResourceRootCheck{
			{
				Path:           "/tmp/ddx-exec",
				Writable:       false,
				WritableReason: "no space left on device",
			},
		},
	}
	checkErr := &ResourceExhaustedError{Detail: "temp root is full", Result: checkResult}

	report, exhausted, logOutput := runResourceExhaustedLoopEvent(t, checkResult, checkErr)

	assert.Equal(t, ExecuteBeadStatusResourceExhausted, report.Status)
	assert.Equal(t, ResourceExhaustedStopMessage, report.Detail)
	assert.Empty(t, report.ResourceExhaustionDiagnosis)
	assert.False(t, report.ResourceExhaustionRestartable)

	assert.Empty(t, exhausted["diagnosis"])
	assert.Equal(t, false, exhausted["restartable"])
	assert.Equal(t, ResourceExhaustedStopMessage, exhausted["detail"])

	assert.Contains(t, logOutput, ResourceExhaustedStopMessage)
	assert.NotContains(t, logOutput, FDExhaustionStopMessage)
}

// inodeDiagnosticFixture builds a resource check result with a populated
// top-inode-consumer diagnostic suitable for report/event pass-through tests.
func inodeDiagnosticFixture(tempRoot, heavyPath string) ExecutionResourceCheckResult {
	consumer := ExecutionTopInodeConsumer{
		Path:             heavyPath,
		EntryCount:       5,
		Bytes:            52,
		ModTime:          time.Unix(1_700_000_000, 0).UTC(),
		AgeSeconds:       10800,
		CleanupPrefix:    "ddx-home-",
		MatchesCleanup:   true,
		EntriesTruncated: true,
	}
	before := ExecutionResourceRootCheck{
		Path:                       tempRoot,
		Writable:                   true,
		BytesFree:                  1,
		InodesFree:                 2,
		TopInodeConsumers:          []ExecutionTopInodeConsumer{consumer},
		TopInodeConsumersTruncated: true,
	}
	after := ExecutionResourceRootCheck{
		Path:                       tempRoot,
		Writable:                   true,
		BytesFree:                  3,
		InodesFree:                 4,
		Notes:                      []string{"free inodes 4 < required 10"},
		TopInodeConsumers:          []ExecutionTopInodeConsumer{consumer},
		TopInodeConsumersTruncated: true,
	}
	return ExecutionResourceCheckResult{
		ProjectRoot:      "/project/root",
		TempRoot:         tempRoot,
		EvidenceRoots:    []string{"/project/root/.ddx/executions"},
		BeforeRootChecks: []ExecutionResourceRootCheck{before},
		RootChecks:       []ExecutionResourceRootCheck{after},
		CleanupSummary: ExecutionCleanupSummary{
			ProjectRoot:     "/project/root",
			TempRoot:        tempRoot,
			BytesReclaimed:  5,
			InodesReclaimed: 6,
		},
	}
}

// assertJSONTopInodeConsumer proves a decoded event/root-check consumer map
// carries path, entry count, size, age, cleanup-prefix match, and truncation.
func assertJSONTopInodeConsumer(t *testing.T, consumer map[string]any, wantPath string) {
	t.Helper()
	assert.Equal(t, wantPath, consumer["path"])
	assert.Equal(t, float64(5), consumer["entry_count"])
	assert.Equal(t, float64(52), consumer["bytes"])
	assert.Equal(t, float64(10800), consumer["age_seconds"])
	assert.Equal(t, "ddx-home-", consumer["cleanup_prefix"])
	assert.Equal(t, true, consumer["matches_cleanup"])
	assert.Equal(t, true, consumer["entries_truncated"])
}

func rootCheckMapsFromEvent(t *testing.T, data map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := data[key].([]any)
	require.True(t, ok, "event field %q must be a JSON array", key)
	require.NotEmpty(t, raw, "event field %q must not be empty", key)
	out := make([]map[string]any, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		require.True(t, ok, "%s[%d] must decode as object", key, i)
		out = append(out, m)
	}
	return out
}

func topInodeConsumersFromRootCheck(t *testing.T, check map[string]any) []map[string]any {
	t.Helper()
	raw, ok := check["top_inode_consumers"].([]any)
	require.True(t, ok, "top_inode_consumers must survive JSON output")
	require.NotEmpty(t, raw)
	out := make([]map[string]any, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		require.True(t, ok, "top_inode_consumers[%d] must decode as object", i)
		out = append(out, m)
	}
	return out
}

// TestResourceExhaustedPayloadIncludesInodeDiagnostics proves the structured
// resource_exhausted event/report carries top-consumer diagnostics through
// existing JSON output, including path, entry count, size, age, cleanup-prefix
// match, and truncation metadata when present.
func TestResourceExhaustedPayloadIncludesInodeDiagnostics(t *testing.T) {
	const (
		tempRoot  = "/tmp/ddx-exec-inodes"
		heavyPath = "/tmp/ddx-exec-inodes/ddx-home-heavy"
	)
	checkResult := inodeDiagnosticFixture(tempRoot, heavyPath)
	checkErr := &ResourceExhaustedError{Detail: "temp root free inodes below hard floor", Result: checkResult}

	report, exhausted, _ := runResourceExhaustedLoopEvent(t, checkResult, checkErr)
	require.Equal(t, ExecuteBeadStatusResourceExhausted, report.Status)

	// Report payload remains JSON-marshalable with full diagnostic fields.
	reportJSON, err := json.Marshal(report.ResourceExhausted)
	require.NoError(t, err)
	assert.Contains(t, string(reportJSON), "top_inode_consumers")
	assert.Contains(t, string(reportJSON), heavyPath)
	assert.Contains(t, string(reportJSON), `"entry_count":5`)
	assert.Contains(t, string(reportJSON), `"bytes":52`)
	assert.Contains(t, string(reportJSON), `"age_seconds":10800`)
	assert.Contains(t, string(reportJSON), `"cleanup_prefix":"ddx-home-"`)
	assert.Contains(t, string(reportJSON), `"matches_cleanup":true`)
	assert.Contains(t, string(reportJSON), `"entries_truncated":true`)
	assert.Contains(t, string(reportJSON), `"top_inode_consumers_truncated":true`)

	// resource.exhausted event preserves the same structured fields.
	for _, key := range []string{"root_checks_after", "root_checks"} {
		checks := rootCheckMapsFromEvent(t, exhausted, key)
		assert.Equal(t, true, checks[0]["top_inode_consumers_truncated"])
		consumers := topInodeConsumersFromRootCheck(t, checks[0])
		assertJSONTopInodeConsumer(t, consumers[0], heavyPath)
	}
	before := rootCheckMapsFromEvent(t, exhausted, "root_checks_before")
	assert.Equal(t, true, before[0]["top_inode_consumers_truncated"])
	assertJSONTopInodeConsumer(t, topInodeConsumersFromRootCheck(t, before[0])[0], heavyPath)

	// Nested body string (bead-event payload) also retains the diagnostic.
	body, _ := exhausted["body"].(string)
	require.NotEmpty(t, body, "resource.exhausted body must carry marshaled detail")
	assert.Contains(t, body, "top_inode_consumers")
	assert.Contains(t, body, heavyPath)
	assert.Contains(t, body, "ddx-home-")
}

// TestResourcePreflightEventIncludesInodeDiagnostics proves resource.preflight
// events preserve top-consumer diagnostics in root_checks_before and
// root_checks_after.
func TestResourcePreflightEventIncludesInodeDiagnostics(t *testing.T) {
	const (
		tempRoot  = "/tmp/ddx-exec-preflight-inodes"
		heavyPath = "/tmp/ddx-exec-preflight-inodes/ddx-home-heavy"
	)
	checkResult := inodeDiagnosticFixture(tempRoot, heavyPath)
	checker := &staticLoopResourceChecker{
		result: checkResult,
		err: &ResourceExhaustedError{
			Detail: "temp root free inodes below hard floor",
			Result: checkResult,
		},
	}

	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run after pre-claim resource exhaustion: %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	var eventSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		EventSink:       &eventSink,
		Once:            false,
		ProjectRoot:     t.TempDir(),
		ResourceChecker: checker,
		SessionID:       "sess-resource-preflight-inodes",
		WorkerID:        "worker-resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var preflight map[string]any
	for _, line := range strings.Split(strings.TrimSpace(eventSink.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] == "resource.preflight" {
			data, _ := entry["data"].(map[string]any)
			preflight = data
		}
	}
	require.NotNil(t, preflight, "resource.preflight event must be emitted")
	assert.Equal(t, ExecuteBeadStatusResourceExhausted, preflight["status"])

	before := rootCheckMapsFromEvent(t, preflight, "root_checks_before")
	after := rootCheckMapsFromEvent(t, preflight, "root_checks_after")
	assert.Equal(t, true, before[0]["top_inode_consumers_truncated"])
	assert.Equal(t, true, after[0]["top_inode_consumers_truncated"])
	assertJSONTopInodeConsumer(t, topInodeConsumersFromRootCheck(t, before[0])[0], heavyPath)
	assertJSONTopInodeConsumer(t, topInodeConsumersFromRootCheck(t, after[0])[0], heavyPath)
}

// TestResourceExhaustedTextSummaryIncludesBoundedInodeDiagnostic proves the
// text/log summary includes only a compact bounded diagnostic (path + counts)
// and never sensitive file contents.
func TestResourceExhaustedTextSummaryIncludesBoundedInodeDiagnostic(t *testing.T) {
	const secret = "SUPER_SECRET_PAYLOAD_must_not_appear_in_logs_99"
	const (
		tempRoot  = "/tmp/ddx-exec-text-inodes"
		heavyPath = "/tmp/ddx-exec-text-inodes/ddx-home-heavy"
	)

	// Four consumers so the text renderer must bound to maxTextTopInodeConsumers.
	consumers := make([]ExecutionTopInodeConsumer, 0, 4)
	for i := 0; i < 4; i++ {
		consumers = append(consumers, ExecutionTopInodeConsumer{
			Path:           filepath.Join(tempRoot, "ddx-home-"+string(rune('a'+i))),
			EntryCount:     int64(10 - i),
			Bytes:          int64(100 * (i + 1)),
			AgeSeconds:     3600,
			CleanupPrefix:  "ddx-home-",
			MatchesCleanup: true,
		})
	}
	// First consumer uses the canonical heavy path for log substring checks.
	consumers[0].Path = heavyPath
	consumers[0].EntryCount = 5
	consumers[0].Bytes = 52
	consumers[0].AgeSeconds = 10800
	consumers[0].EntriesTruncated = true

	checkResult := ExecutionResourceCheckResult{
		ProjectRoot: "/project/root",
		TempRoot:    tempRoot,
		BeforeRootChecks: []ExecutionResourceRootCheck{
			{
				Path:                       tempRoot,
				Writable:                   true,
				BytesFree:                  1,
				InodesFree:                 2,
				TopInodeConsumers:          consumers,
				TopInodeConsumersTruncated: true,
			},
		},
		RootChecks: []ExecutionResourceRootCheck{
			{
				Path:                       tempRoot,
				Writable:                   true,
				BytesFree:                  3,
				InodesFree:                 4,
				TopInodeConsumers:          consumers,
				TopInodeConsumersTruncated: true,
			},
		},
		CleanupSummary: ExecutionCleanupSummary{
			BytesReclaimed:  5,
			InodesReclaimed: 6,
		},
	}
	checkErr := &ResourceExhaustedError{Detail: "temp root free inodes below hard floor", Result: checkResult}

	_, _, logOutput := runResourceExhaustedLoopEvent(t, checkResult, checkErr)

	// Compact path/count summary is present in the preflight text log.
	assert.Contains(t, logOutput, "top_inode_consumers=[")
	assert.Contains(t, logOutput, heavyPath+":entries=5")
	assert.Contains(t, logOutput, "bytes=52")
	assert.Contains(t, logOutput, "age_s=10800")
	assert.Contains(t, logOutput, "cleanup=ddx-home-")
	assert.Contains(t, logOutput, "entries_truncated")
	assert.Contains(t, logOutput, ",truncated")

	// Bound: at most maxTextTopInodeConsumers consumers listed in text.
	// Fourth consumer path must not appear; first three may.
	assert.NotContains(t, logOutput, filepath.Join(tempRoot, "ddx-home-d"))
	assert.Contains(t, logOutput, filepath.Join(tempRoot, "ddx-home-b"))
	assert.Contains(t, logOutput, filepath.Join(tempRoot, "ddx-home-c"))

	// Sensitive contents must never appear in the text summary.
	assert.NotContains(t, logOutput, secret)
	// Direct unit check of the compact formatter also omits secrets.
	compact := formatTopInodeConsumersCompact(checkResult.RootChecks[0])
	assert.NotContains(t, compact, secret)
	assert.Contains(t, compact, "top_inode_consumers=[")
	// Exactly three path tokens before the list closes (bounded).
	assert.Equal(t, maxTextTopInodeConsumers, strings.Count(compact, ":entries="))
}
