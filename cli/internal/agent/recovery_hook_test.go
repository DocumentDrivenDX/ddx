package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsecutiveLadderExhaustionsCounter verifies that the
// consecutive_ladder_exhaustions counter increments on per-bead-budget-exhausted
// and that the PostLadderExhaustionHook fires when the counter reaches threshold 2.
func TestConsecutiveLadderExhaustionsCounter(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-hook-threshold", Title: "hook threshold test"}
	require.NoError(t, store.Create(context.

		// Pre-seed counter to 1 so the next budget exhaustion hits the threshold.
		Background(), b))

	require.NoError(t, incrementConsecutiveLadderExhaustions(store, b.ID))
	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, got.Extra[consecutiveLadderExhaustionsKey])

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var hookCalls int32
	var hookBeadID string
	var hookClass RecoveryFailureClass

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    escalation.PerBeadBudgetExhaustedReason + ": $1.00 billed >= $0.50 per-bead budget",
				SessionID: "sess-hook-test",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		atomic.AddInt32(&hookCalls, 1)
		hookBeadID = beadID
		hookClass = class
		cancel()
		return &PostLadderExhaustionResult{Attempted: false}, nil
	})

	_, _ = worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:                     executeloop.ModeDrain,
		PostLadderExhaustionHook: hook,
		ProjectRoot:              t.TempDir(),
		SessionID:                "sess-hook-test",
		WorkerID:                 "worker-hook-test",
	})

	assert.EqualValues(t, 1, atomic.LoadInt32(&hookCalls), "hook must fire exactly once at threshold 2")
	assert.Equal(t, b.ID, hookBeadID)
	assert.Equal(t, PersistentExecutionFailed, hookClass)

	updated, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, consecutiveLadderExhaustionsValue(updated.Extra[consecutiveLadderExhaustionsKey]), 2,
		"counter must be >= 2 after threshold reached")
}

// TestRecoveryManualLabel_SkipsAutoRecovery verifies that a bead with the
// "recovery:manual" label parks to status=proposed without invoking the
// PostLadderExhaustionHook when the exhaustion threshold is reached.
func TestRecoveryManualLabel_SkipsAutoRecovery(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:     "ddx-manual-recovery",
		Title:  "manual recovery test",
		Labels: []string{"recovery:manual"},
	}
	require.NoError(t, store.Create(context.

		// Pre-seed counter to 1 so the next budget exhaustion hits the threshold.
		Background(), b))

	require.NoError(t, incrementConsecutiveLadderExhaustions(store, b.ID))

	var hookCalled bool
	hook := PostLadderExhaustionHook(func(_ context.Context, _ string, _ RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		hookCalled = true
		return &PostLadderExhaustionResult{Attempted: false}, nil
	})

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    escalation.PerBeadBudgetExhaustedReason + ": $1.00 billed >= $0.50 per-bead budget",
				SessionID: "sess-manual-test",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode:                     executeloop.ModeDrain,
		PostLadderExhaustionHook: hook,
		ProjectRoot:              t.TempDir(),
		SessionID:                "sess-manual-test",
		WorkerID:                 "worker-manual-test",
	})
	require.NoError(t, err)

	assert.False(t, hookCalled, "hook must not be invoked when recovery:manual label is set")

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "bead must be parked to proposed")
}
