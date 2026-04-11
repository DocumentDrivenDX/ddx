package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
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

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
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

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker"})
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

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker"})
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

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker"})
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

	firstRun, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
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

	secondRun, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker", Once: true})
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

	result, err := worker.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork)
	assert.Equal(t, 0, result.Attempts)
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

	go func() {
		defer wg.Done()
		results[0], errs[0] = workerA.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker-a", Once: true})
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = workerB.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker-b", Once: true})
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

	go func() {
		defer wg.Done()
		results[0], errs[0] = workerA.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker-a", Once: true})
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = workerB.Run(context.Background(), ExecuteBeadLoopOptions{Assignee: "worker-b", Once: true})
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
