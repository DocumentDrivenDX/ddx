package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadWorkerSuccessClosesBead(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-1",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.Equal(t, "sess-1", got.Extra["session_id"])
	assert.Equal(t, "deadbeef", got.Extra["closing_commit_sha"])

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "success", events[0].Summary)
}

func TestExecuteBeadWorkerLabelFilterSkipsNonMatchingReadyBeads(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	require.NoError(t, store.Update(second.ID, func(b *bead.Bead) {
		b.Labels = []string{"ui"}
	}))

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-filter",
				ResultRev: "cafe",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		LabelFilter: "ui",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{second.ID}, executed)
	assert.Equal(t, 1, result.Attempts)

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
}

func TestExecuteBeadWorkerPreservedFailureStaysOpenAndContinues(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	executed := []string{}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:      beadID,
					Status:      ExecuteBeadStatusExecutionFailed,
					Detail:      "agent execution failed",
					PreserveRef: "refs/ddx/iterations/" + beadID + "/attempt-1",
					ResultRev:   "badc0de",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-2",
				ResultRev: "c0ffee",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.LastFailureStatus)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, firstGot.Status)
	assert.Empty(t, firstGot.Owner)

	secondGot, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, secondGot.Status)
	assert.Equal(t, "sess-2", secondGot.Extra["session_id"])
}

func TestExecuteBeadWorkerLandConflictStaysOpenAndContinues(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	executed := []string{}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:      beadID,
					Status:      ExecuteBeadStatusLandConflict,
					Detail:      "ff-merge not possible",
					PreserveRef: "refs/ddx/iterations/" + beadID + "/attempt-1",
					ResultRev:   "feedface",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-3",
				ResultRev: "f00dbabe",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusLandConflict, result.LastFailureStatus)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, firstGot.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, ExecuteBeadStatusLandConflict, events[0].Summary)
	assert.Contains(t, events[0].Body, "preserve_ref=")
}

func TestExecuteBeadWorkerNoChangesStaysOpenAndContinues(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	executed := []string{}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusNoChanges,
					Detail:    "agent made no commits",
					BaseRev:   "aaaa1111",
					ResultRev: "aaaa1111",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-4",
				ResultRev: "facefeed",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.LastFailureStatus)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, firstGot.Status)
	assert.Empty(t, firstGot.Owner)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, ExecuteBeadStatusNoChanges, events[0].Summary)
	assert.Contains(t, events[0].Body, "agent made no commits")
}

func TestExecuteBeadWorkerNoChangesSuppressesImmediateRetryAcrossRuns(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	callCount := 0
	now := time.Now().UTC().Truncate(time.Second)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			callCount++
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusNoChanges,
					Detail:    "agent made no commits",
					BaseRev:   "aaaa1111",
					ResultRev: "aaaa1111",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-5",
				ResultRev: "fadedcab",
			}, nil
		}),
		Now: func() time.Time {
			return now
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	firstRun, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, firstRun)
	assert.Equal(t, 1, firstRun.Attempts)
	assert.Equal(t, ExecuteBeadStatusNoChanges, firstRun.LastFailureStatus)
	require.Len(t, firstRun.Results, 1)
	assert.NotEmpty(t, firstRun.Results[0].RetryAfter)

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Equal(t, ExecuteBeadStatusNoChanges, gotFirst.Extra["execute-loop-last-status"])
	assert.Equal(t, "agent made no commits", gotFirst.Extra["execute-loop-last-detail"])
	assert.NotEmpty(t, gotFirst.Extra["execute-loop-retry-after"])

	secondRun, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, secondRun)
	assert.Equal(t, 1, secondRun.Attempts)
	require.Len(t, secondRun.Results, 1)
	assert.Equal(t, second.ID, secondRun.Results[0].BeadID)

	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotSecond.Status)
	assert.Equal(t, 2, callCount)
}

func TestExecuteBeadWorkerNoReadyWork(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("unexpected execution for %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork)
	assert.Equal(t, 0, result.Attempts)
}

// TestExecuteBeadWorkerNoReadyWork_EpicOnlyQueue covers ddx-672cf2bf:
// when the dependency-ready queue contains only epics, the loop must not
// silently exit — it must surface a breakdown identifying the skipped epics
// so the operator knows why nothing ran.
func TestExecuteBeadWorkerNoReadyWork_EpicOnlyQueue(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	epic := &bead.Bead{ID: "ddx-epic-001", Title: "Epic container", IssueType: "epic", Priority: 1}
	require.NoError(t, store.Create(epic))

	cooldownTask := &bead.Bead{ID: "ddx-task-002", Title: "Cooldown task", IssueType: "task", Priority: 1,
		Extra: map[string]any{
			"execute-loop-retry-after": time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	require.NoError(t, store.Create(cooldownTask))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("unexpected execution for %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork, "NoReadyWork must fire when execution queue is empty but dep-ready queue isn't")
	assert.Equal(t, 0, result.Attempts)

	breakdown := result.NoReadyWorkDetail
	assert.Equal(t, []string{"ddx-epic-001"}, breakdown.SkippedEpics,
		"ready epic must surface in the breakdown with its id so the operator knows to decompose it")
	assert.Equal(t, []string{"ddx-task-002"}, breakdown.SkippedOnCooldown,
		"cooldown bead must surface separately so the operator sees retry-after, not just absence of work")
	assert.NotEmpty(t, breakdown.NextRetryAfter,
		"NextRetryAfter must identify the soonest cooldown release so the operator knows when to poll again")
}

func TestExecuteBeadWorkerConcurrentWorkersDoNotDoubleExecuteSameBead(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	only := &bead.Bead{ID: "ddx-race-1", Title: "Only ready bead", Priority: 0}
	require.NoError(t, store.Create(only))

	var execCalls atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		execCalls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    ExecuteBeadStatusSuccess,
			Detail:    "merged cleanly",
			SessionID: "sess-race",
			ResultRev: "deadbeef",
		}, nil
	})

	workerA := &ExecuteBeadWorker{Store: store, Executor: executor}
	workerB := &ExecuteBeadWorker{Store: store, Executor: executor}

	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]*ExecuteBeadLoopResult, 2)
	errs := make([]error, 2)

	cfgOptsA := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfgA := config.NewTestConfigForLoop(cfgOptsA).Resolve(config.TestLoopOverrides(cfgOptsA))
	cfgOptsB := config.TestLoopConfigOpts{Assignee: "worker-b"}
	rcfgB := config.NewTestConfigForLoop(cfgOptsB).Resolve(config.TestLoopOverrides(cfgOptsB))
	go func() {
		defer wg.Done()
		results[0], errs[0] = workerA.RunWithConfig(context.Background(), rcfgA, ExecuteBeadLoopRuntime{Once: true})
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = workerB.RunWithConfig(context.Background(), rcfgB, ExecuteBeadLoopRuntime{Once: true})
	}()

	<-started
	close(release)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, int32(1), execCalls.Load(), "only one worker should execute the bead")

	totalAttempts := 0
	totalSuccesses := 0
	for _, result := range results {
		require.NotNil(t, result)
		totalAttempts += result.Attempts
		totalSuccesses += result.Successes
	}
	assert.Equal(t, 1, totalAttempts)
	assert.Equal(t, 1, totalSuccesses)

	got, err := store.Get(only.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.Equal(t, "sess-race", got.Extra["session_id"])
	assert.Equal(t, "deadbeef", got.Extra["closing_commit_sha"])
}

func TestExecuteBeadWorkerConcurrentWorkersDistributeDistinctReadyBeads(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)

	var (
		mu       sync.Mutex
		executed []string
	)
	barrier := make(chan struct{}, 2)
	release := make(chan struct{})
	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		mu.Lock()
		executed = append(executed, beadID)
		mu.Unlock()
		barrier <- struct{}{}
		<-release
		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    ExecuteBeadStatusSuccess,
			Detail:    "merged cleanly",
			SessionID: "sess-" + beadID,
			ResultRev: "rev-" + beadID,
		}, nil
	})

	workerA := &ExecuteBeadWorker{Store: store, Executor: executor}
	workerB := &ExecuteBeadWorker{Store: store, Executor: executor}

	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]*ExecuteBeadLoopResult, 2)
	errs := make([]error, 2)

	cfgOptsA := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfgA := config.NewTestConfigForLoop(cfgOptsA).Resolve(config.TestLoopOverrides(cfgOptsA))
	cfgOptsB := config.TestLoopConfigOpts{Assignee: "worker-b"}
	rcfgB := config.NewTestConfigForLoop(cfgOptsB).Resolve(config.TestLoopOverrides(cfgOptsB))
	go func() {
		defer wg.Done()
		results[0], errs[0] = workerA.RunWithConfig(context.Background(), rcfgA, ExecuteBeadLoopRuntime{Once: true})
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = workerB.RunWithConfig(context.Background(), rcfgB, ExecuteBeadLoopRuntime{Once: true})
	}()

	<-barrier
	<-barrier
	close(release)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)

	totalAttempts := 0
	totalSuccesses := 0
	for _, result := range results {
		require.NotNil(t, result)
		totalAttempts += result.Attempts
		totalSuccesses += result.Successes
	}
	assert.Equal(t, 2, totalAttempts)
	assert.Equal(t, 2, totalSuccesses)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	secondGot, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, firstGot.Status)
	assert.Equal(t, bead.StatusClosed, secondGot.Status)
	assert.Equal(t, "sess-"+first.ID, firstGot.Extra["session_id"])
	assert.Equal(t, "sess-"+second.ID, secondGot.Extra["session_id"])
}

func TestReadyExecutionExcludesEpics(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	task := &bead.Bead{ID: "ddx-task01", Title: "Task work", IssueType: "task", Priority: 1}
	epic := &bead.Bead{ID: "ddx-epic01", Title: "Epic container", IssueType: "epic", Priority: 0}
	require.NoError(t, store.Create(task))
	require.NoError(t, store.Create(epic))

	ready, err := store.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, task.ID, ready[0].ID)
}

func TestExecuteBeadWorkerEmitsStructuredProgressEvents(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					Detail:    "merged cleanly",
					SessionID: "sess-first",
					ResultRev: "feedbeef",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusExecutionFailed,
				Detail:      "agent execution failed",
				PreserveRef: "refs/ddx/iterations/" + beadID + "/attempt-1",
				ResultRev:   "baadf00d",
			}, nil
		}),
	}

	var sink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{
		Assignee: "worker",
		Harness:  "claude",
		Model:    "claude-3.5-sonnet",
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		EventSink:   &sink,
		WorkerID:    "worker-42",
		ProjectRoot: "/tmp/fake-project",
		SessionID:   "agent-loop-test",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Attempts)

	lines := strings.Split(strings.TrimRight(sink.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 6, "expected loop.start + 2*(bead.claimed+bead.result) + loop.end")

	parse := func(t *testing.T, line string) (string, map[string]any) {
		t.Helper()
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		typ, _ := entry["type"].(string)
		data, _ := entry["data"].(map[string]any)
		// Every entry must carry the envelope fields.
		assert.Equal(t, "agent-loop-test", entry["session_id"])
		assert.NotEmpty(t, entry["ts"])
		return typ, data
	}

	// First line must be loop.start with metadata.
	typ, data := parse(t, lines[0])
	assert.Equal(t, "loop.start", typ)
	assert.Equal(t, "worker-42", data["worker_id"])
	assert.Equal(t, "/tmp/fake-project", data["project_root"])
	assert.Equal(t, "claude", data["harness"])
	assert.Equal(t, "claude-3.5-sonnet", data["model"])

	// Collect the rest by type so ordering between beads isn't load-bearing.
	byType := map[string][]map[string]any{}
	for _, line := range lines {
		typ, data := parse(t, line)
		byType[typ] = append(byType[typ], data)
	}

	require.Len(t, byType["loop.start"], 1)
	require.Len(t, byType["loop.end"], 1)
	require.Len(t, byType["bead.claimed"], 2)
	require.Len(t, byType["bead.result"], 2)

	claimedIDs := []string{}
	for _, d := range byType["bead.claimed"] {
		id, _ := d["bead_id"].(string)
		claimedIDs = append(claimedIDs, id)
		assert.NotEmpty(t, d["title"], "bead.claimed should carry the title")
	}
	assert.ElementsMatch(t, []string{first.ID, second.ID}, claimedIDs)

	// bead.result must carry status + duration_ms for every attempt, and
	// success/result_rev for the successful bead.
	var sawSuccess, sawFailure bool
	for _, d := range byType["bead.result"] {
		beadID, _ := d["bead_id"].(string)
		status, _ := d["status"].(string)
		_, hasDuration := d["duration_ms"]
		assert.True(t, hasDuration, "bead.result must include duration_ms")
		if beadID == first.ID {
			sawSuccess = true
			assert.Equal(t, ExecuteBeadStatusSuccess, status)
			assert.Equal(t, "sess-first", d["session_id"])
			assert.Equal(t, "feedbeef", d["result_rev"])
		}
		if beadID == second.ID {
			sawFailure = true
			assert.Equal(t, ExecuteBeadStatusExecutionFailed, status)
			assert.Equal(t, "agent execution failed", d["detail"])
		}
	}
	assert.True(t, sawSuccess, "bead.result missing for successful bead")
	assert.True(t, sawFailure, "bead.result missing for failed bead")

	// loop.end must summarise attempts/successes/failures.
	endData := byType["loop.end"][0]
	assert.EqualValues(t, 2, endData["attempts"])
	assert.EqualValues(t, 1, endData["successes"])
	assert.EqualValues(t, 1, endData["failures"])
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, endData["last_failure_status"])
}

func TestExecuteBeadWorkerEmitsLoopEventsWithNoReadyWork(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("unexpected execution for %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	var sink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		EventSink: &sink,
		SessionID: "agent-loop-empty",
	})
	require.NoError(t, err)
	require.True(t, result.NoReadyWork)

	lines := strings.Split(strings.TrimRight(sink.String(), "\n"), "\n")
	require.Len(t, lines, 2, "empty queue should still emit loop.start and loop.end")

	var start, end map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &start))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &end))
	assert.Equal(t, "loop.start", start["type"])
	assert.Equal(t, "loop.end", end["type"])
	endData, _ := end["data"].(map[string]any)
	assert.EqualValues(t, 0, endData["attempts"])
}

// TestExecuteBeadWorkerNoChangesAutoClosesAtThreshold verifies that the default
// count-based adjudication closes a bead as already_satisfied once the
// no-changes count reaches the configured threshold.
func TestExecuteBeadWorkerNoChangesAutoClosesAtThreshold(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-nc01", Title: "Always no-changes"}
	require.NoError(t, store.Create(b))

	callCount := 0
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			callCount++
			// No BaseRev/ResultRev so shouldSuppressNoProgress returns false
			// and no cooldown is applied — bead stays immediately retryable.
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusNoChanges,
				Detail: "nothing to do",
			}, nil
		}),
	}

	const threshold = 2

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                "worker",
		MaxNoChangesBeforeClose: threshold,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// First pass: count=1 < threshold, bead stays open.
	r1, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, r1.Attempts)
	assert.Equal(t, 0, r1.Successes)
	assert.Equal(t, ExecuteBeadStatusNoChanges, r1.LastFailureStatus)
	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)

	// Second pass: count=2 == threshold, bead is closed as already_satisfied.
	r2, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, r2.Attempts)
	assert.Equal(t, 1, r2.Successes)
	require.Len(t, r2.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, r2.Results[0].Status)

	got, err = store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.Equal(t, 2, callCount)
}

// TestExecuteBeadWorkerCustomSatisfactionCheckerClosesBeadWhenSatisfied verifies
// that a custom SatisfactionChecker can close a bead immediately on the first
// no_changes result without waiting for the count threshold.
func TestExecuteBeadWorkerCustomSatisfactionCheckerClosesBeadWhenSatisfied(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-sat01", Title: "Already done"}
	require.NoError(t, store.Create(b))

	checkerCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusNoChanges,
				Detail: "agent found no work",
			}, nil
		}),
		SatisfactionChecker: SatisfactionCheckerFunc(func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
			checkerCalled = true
			assert.Equal(t, b.ID, beadID)
			assert.Equal(t, 1, noChangesCount)
			return true, "acceptance criteria already met", nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.True(t, checkerCalled, "SatisfactionChecker must be called")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status)
	assert.Equal(t, "acceptance criteria already met", result.Results[0].Detail)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, events[0].Summary)
}

// TestExecuteBeadWorkerCustomSatisfactionCheckerLeavesBeadOpenWhenUnresolved
// verifies that when the SatisfactionChecker reports the bead is not yet
// satisfied, the bead remains open and retry suppression is applied.
func TestExecuteBeadWorkerCustomSatisfactionCheckerLeavesBeadOpenWhenUnresolved(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-unr01", Title: "Not yet done"}
	require.NoError(t, store.Create(b))

	now := time.Now().UTC().Truncate(time.Second)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusNoChanges,
				BaseRev:   "rev1",
				ResultRev: "rev1",
			}, nil
		}),
		SatisfactionChecker: SatisfactionCheckerFunc(func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
			return false, "", nil
		}),
		Now: func() time.Time { return now },
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:           "worker",
		NoProgressCooldown: 1 * time.Hour,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.LastFailureStatus)
	require.Len(t, result.Results, 1)
	assert.NotEmpty(t, result.Results[0].RetryAfter, "retry suppression must be recorded")

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.NotEmpty(t, got.Extra["execute-loop-retry-after"])
}

// TestExecuteBeadWorkerNoChangesDoesNotStarveQueue verifies that a bead with
// repeated no_changes results cannot prevent other ready beads from being
// executed across multiple queue passes. It also verifies the full adjudication
// lifecycle: other beads run unblocked while the no-changes bead is open, and
// the no-changes bead is eventually closed as already_satisfied at the threshold.
func TestExecuteBeadWorkerNoChangesDoesNotStarveQueue(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	// Three beads: one that always returns no_changes (no cooldown), two that succeed.
	// Not supplying BaseRev/ResultRev means shouldSuppressNoProgress returns false,
	// so no retry-after is written and the bead stays immediately retryable between
	// passes. This keeps the test deterministic without mocking time.
	ncBead := &bead.Bead{ID: "ddx-nc10", Title: "Always no-changes", Priority: 0}
	work1 := &bead.Bead{ID: "ddx-wk11", Title: "Work bead 1", Priority: 1}
	work2 := &bead.Bead{ID: "ddx-wk12", Title: "Work bead 2", Priority: 2}
	require.NoError(t, store.Create(ncBead))
	require.NoError(t, store.Create(work1))
	require.NoError(t, store.Create(work2))

	var mu sync.Mutex
	executed := []string{}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			mu.Lock()
			executed = append(executed, beadID)
			mu.Unlock()
			if beadID == ncBead.ID {
				// No BaseRev/ResultRev → shouldSuppressNoProgress returns false
				// → no cooldown written → bead is immediately retryable.
				return ExecuteBeadReport{
					BeadID: beadID,
					Status: ExecuteBeadStatusNoChanges,
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-" + beadID,
				ResultRev: "bbbb",
			}, nil
		}),
	}

	const threshold = 2
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                "worker",
		MaxNoChangesBeforeClose: threshold,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	runtime := ExecuteBeadLoopRuntime{}

	// Pass 1: all three beads are ready.
	// ncBead returns no_changes (count=1 < threshold) and stays open.
	// work1 and work2 succeed and are closed.
	// The `attempted` map inside Run prevents ncBead from being picked up
	// a second time within the same pass — the other beads run unblocked.
	result1, err := worker.RunWithConfig(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	assert.Equal(t, 3, result1.Attempts, "all three beads must be attempted in pass 1")
	assert.Equal(t, 2, result1.Successes)
	assert.Equal(t, 1, result1.Failures)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result1.LastFailureStatus)

	w1, _ := store.Get(work1.ID)
	w2, _ := store.Get(work2.ID)
	nc, _ := store.Get(ncBead.ID)
	assert.Equal(t, bead.StatusClosed, w1.Status)
	assert.Equal(t, bead.StatusClosed, w2.Status)
	assert.Equal(t, bead.StatusOpen, nc.Status, "ncBead must stay open after first no_changes")

	// Pass 2: only ncBead remains; count reaches threshold → closed as already_satisfied.
	result2, err := worker.RunWithConfig(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	assert.Equal(t, 1, result2.Attempts)
	assert.Equal(t, 1, result2.Successes, "ncBead must be closed as already_satisfied")
	require.Len(t, result2.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result2.Results[0].Status)

	nc, _ = store.Get(ncBead.ID)
	assert.Equal(t, bead.StatusClosed, nc.Status)

	// Pass 3: queue is empty.
	result3, err := worker.RunWithConfig(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	assert.True(t, result3.NoReadyWork)
	assert.Equal(t, 0, result3.Attempts)

	// ncBead was attempted exactly twice (once per pass), never a third time.
	ncExec := 0
	for _, id := range executed {
		if id == ncBead.ID {
			ncExec++
		}
	}
	assert.Equal(t, 2, ncExec, "ncBead must be executed exactly twice across all passes")
}

// TestRationaleIsSpecific verifies the heuristic that decides whether a
// no_changes rationale is specific enough to close the bead immediately.
func TestRationaleIsSpecific(t *testing.T) {
	cases := []struct {
		rationale string
		want      bool
	}{
		{"", false},
		{"nothing to do", false},
		{"agent found no work", false},
		// 7-hex commit SHA
		{"work already present in commit 1da6495 (store.go)", true},
		// 12-hex commit SHA
		{"see commit 0c60abf493c7 for details", true},
		// 40-hex commit SHA
		{"fully present since 0c60abf493c7117a9b5f7986c1412c1d513e2ef6", true},
		// Test function name
		{"TestReadyExecutionExcludesEpics already exists and passes", true},
		{"confirmed by TestEpicFilterSmoke", true},
		// Benchmark name
		{"BenchmarkStore already exists", true},
		// 6-char hex (too short to qualify as SHA)
		{"short ref abc123 is not a commit", false},
	}
	for _, tc := range cases {
		got := rationaleIsSpecific(tc.rationale)
		if got != tc.want {
			t.Errorf("rationaleIsSpecific(%q) = %v, want %v", tc.rationale, got, tc.want)
		}
	}
}

// TestExecuteBeadWorkerNoChangesWithCommitSHARationaleClosesImmediately verifies
// that when a no_changes report carries a rationale referencing a prior commit
// SHA, the loop closes the bead as already_satisfied on the first attempt without
// waiting for the count-based threshold.
func TestExecuteBeadWorkerNoChangesWithCommitSHARationaleClosesImmediately(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-sha01", Title: "Already in prior commit"}
	require.NoError(t, store.Create(b))

	const rationale = "Work already present in commit 1da6495 (cli/internal/bead/store.go). " +
		"TestReadyExecutionExcludesEpics confirms the epic filter passes."

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				Detail:             "agent made no commits",
				BaseRev:            "aaaa1111",
				ResultRev:          "aaaa1111",
				NoChangesRationale: rationale,
			}, nil
		}),
	}

	// MaxNoChangesBeforeClose=3 to confirm the heuristic fires before threshold.
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                "worker",
		MaxNoChangesBeforeClose: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes, "commit-SHA rationale must close bead on first attempt")
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status)
	assert.Equal(t, rationale, result.Results[0].Detail)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, events[0].Summary)
	assert.Contains(t, events[0].Body, "rationale:")
}

// TestExecuteBeadWorkerNoChangesVagueRationaleUsesCountThreshold verifies that
// a vague rationale (no commit SHA, no test name) does not trigger early close
// and the count-based threshold still applies.
func TestExecuteBeadWorkerNoChangesVagueRationaleUsesCountThreshold(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-vague01", Title: "Vague rationale bead"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			// Non-empty but vague rationale — no commit SHA or test name.
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				Detail:             "nothing to do",
				NoChangesRationale: "the work seems done",
			}, nil
		}),
	}

	const threshold = 2

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                "worker",
		MaxNoChangesBeforeClose: threshold,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Pass 1: vague rationale → bead stays open.
	r1, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 0, r1.Successes)
	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)

	// Pass 2: count reaches threshold → closed as already_satisfied.
	r2, err := worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, r2.Successes)
	got, err = store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func newExecuteLoopTestStore(t *testing.T) (*bead.Store, *bead.Bead, *bead.Bead) {
	t.Helper()

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	first := &bead.Bead{ID: "ddx-0001", Title: "First ready", Priority: 0}
	second := &bead.Bead{ID: "ddx-0002", Title: "Second ready", Priority: 1}
	require.NoError(t, store.Create(first))
	require.NoError(t, store.Create(second))

	return store, first, second
}
