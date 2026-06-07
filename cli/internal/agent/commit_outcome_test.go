package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitOutcome_StoreErrorContinuesLoop(t *testing.T) {
	injectedErr := errors.New("transient storage failure")

	realStore := bead.NewStore(t.TempDir())
	require.NoError(t, realStore.Init(context.Background()))

	first := &bead.Bead{ID: "ddx-commit-first", Title: "First bead"}
	second := &bead.Bead{ID: "ddx-commit-second", Title: "Second bead"}
	require.NoError(t, realStore.Create(context.Background(), first))
	require.NoError(t, realStore.Create(context.Background(), second))

	store := &errorInjectingStore{ExecuteBeadLoopStore: realStore}
	store.onUnclaim = func(id string) error {
		if id == first.ID {
			return injectedErr
		}
		return nil
	}

	var executedIDs []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executedIDs = append(executedIDs, beadID)
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusNoChanges,
					SessionID: "sess-first",
					BaseRev:   "rev-first",
					ResultRev: "rev-first",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-second",
				ResultRev: "rev-second",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)

	assert.Contains(t, executedIDs, second.ID, "loop must continue to the next bead after the first store failure")
	assert.GreaterOrEqual(t, result.Failures, 1)
}

func TestCommitOutcome_StoreError_SchedulesCooldown_NotExit(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)
	result := &ExecuteBeadLoopResult{}

	err := commitOutcome(context.Background(), store, candidate.ID, func() error {
		return commitOutcomeError("CloseWithEvidence", "worker", result, errors.New("transient storage failure"))
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, "loop-error", result.LastFailureStatus)

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)
	assert.Equal(t, "loop-error", got.Extra["work-last-status"])
	_, hasRetry := got.Extra["work-retry-after"]
	assert.True(t, hasRetry, "cooldown timestamp must be persisted")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	loopError := events[len(events)-1]
	assert.Equal(t, "loop-error", loopError.Kind)
	assert.Contains(t, loopError.Summary, "CloseWithEvidence")
}
