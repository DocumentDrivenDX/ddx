package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEpicClosureStore creates a store with an epic and one open child.
// The child's Parent field points to the epic. Call store.Close(child.ID) to
// make the epic a closure candidate.
func newEpicClosureStore(t *testing.T) (*bead.Store, *bead.Bead, *bead.Bead) {
	t.Helper()
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	epic := &bead.Bead{ID: "ddx-epic-1", Title: "Epic rollup", IssueType: "epic"}
	require.NoError(t, store.Create(context.Background(), epic))

	child := &bead.Bead{ID: "ddx-child-1", Title: "Child task", Parent: epic.ID}
	require.NoError(t, store.Create(context.Background(), child))

	return store, epic, child
}

// TestWorkLoop_IdlePassClosesEpicClosureCandidates: a fixture epic whose
// children are all closed is closed in one idle pass with an epic_auto_close
// event (FEAT-004 AC 1).
func TestWorkLoop_IdlePassClosesEpicClosureCandidates(t *testing.T) {
	store, epic, child := newEpicClosureStore(t)
	// Close the child so the epic becomes a closure candidate.
	require.NoError(t, store.Close(context.Background(), child.ID))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run: no execution-ready beads")
			return ExecuteBeadReport{}, nil
		}),
	}
	result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "drained", exitReason)

	got, err := store.Get(context.Background(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "epic must be closed by idle-path cascade")

	events, err := store.Events(epic.ID)
	require.NoError(t, err)
	var hasAutoClose bool
	for _, e := range events {
		if e.Kind == "epic_auto_close" {
			hasAutoClose = true
			break
		}
	}
	assert.True(t, hasAutoClose, "epic must have epic_auto_close event")
}

// TestWorkLoop_IdlePassEpicClosureIsIdempotent: a second idle pass closes zero
// additional epics and does not error (FEAT-004 AC 2).
func TestWorkLoop_IdlePassEpicClosureIsIdempotent(t *testing.T) {
	store, epic, child := newEpicClosureStore(t)
	require.NoError(t, store.Close(context.Background(), child.ID))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run")
			return ExecuteBeadReport{}, nil
		}),
	}

	// First drain: epic gets closed.
	_, firstExit, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	require.NoError(t, err)
	assert.Equal(t, "drained", firstExit)

	got, err := store.Get(context.Background(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	// Second drain: no new closure candidates; exits cleanly.
	result2, secondExit, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Equal(t, "drained", secondExit)
	assert.True(t, result2.NoReadyWork)
}

// TestWorkLoop_EpicWithOpenChildNotClosedByIdlePass: an epic with one open
// child is not closed by the idle-path cascade (FEAT-004 AC 3).
func TestWorkLoop_EpicWithOpenChildNotClosedByIdlePass(t *testing.T) {
	store, epic, child := newEpicClosureStore(t)
	// Put the child in cooldown so it is not execution-ready, but remains open
	// (non-terminal). The epic is therefore not a closure candidate.
	cooldown := time.Now().UTC().Add(24 * time.Hour)
	require.NoError(t, store.SetExecutionCooldown(child.ID, cooldown, "no_changes", "test hold", ""))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run: child is in cooldown")
			return ExecuteBeadReport{}, nil
		}),
	}
	result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "drained", exitReason)

	got, err := store.Get(context.Background(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "epic with open child must not be closed by idle-path")
}
