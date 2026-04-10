package agent

import (
	"context"
	"testing"

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
