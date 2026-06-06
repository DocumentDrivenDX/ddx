package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewBlock_EscalatesImplementerWithoutBeadRetryFloorMetadata asserts that when
// repair-cycle-exhausted is returned and the escalation ladder has a higher
// powerClass available, the bead remains open without persisting a bead retry
// floor.
func TestReviewBlock_EscalatesImplementerWithoutBeadRetryFloorMetadata(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce01", Title: "Repair cycle exhausted escalation"}
	require.NoError(t, store.Create(context.Background(), b))

	worker := &ExecuteBeadWorker{
		Store: store,
		EscalationNextFloor: func(actualPower int) (int, error) {
			if actualPower < 70 {
				return 70, nil
			}
			return 0, fmt.Errorf("ladder exhausted")
		},
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusRepairCycleExhausted,
				ActualPower: 50,
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must remain open after escalation")
	assert.NotContains(t, got.Extra, legacyRetryFloorKey)
}

// TestReviewBlock_StillFailsAtTopPowerClass_ParkProposed asserts that when
// repair-cycle-exhausted occurs at the top powerClass (EscalationNextFloor errors),
// the bead is parked to proposed for operator review.
func TestReviewBlock_StillFailsAtTopPowerClass_ParkProposed(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce02", Title: "Repair cycle exhausted at top powerClass"}
	require.NoError(t, store.Create(context.Background(), b))

	worker := &ExecuteBeadWorker{
		Store: store,
		EscalationNextFloor: func(actualPower int) (int, error) {
			return 0, fmt.Errorf("ladder exhausted")
		},
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusRepairCycleExhausted,
				ActualPower: 90,
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "bead must be parked to proposed when ladder exhausted")
	assert.Empty(t, got.Owner, "proposed bead must not be owned")
}

// TestReviewBlock_NonFixableClassification_DoesNotEscalate asserts that
// review_terminal_block (non-fixable: spec gap, too_large, unsafe) is NOT in
// EscalatableStatuses — a smarter model cannot fix a structural spec problem.
func TestReviewBlock_NonFixableClassification_DoesNotEscalate(t *testing.T) {
	assert.False(t, escalation.EscalatableStatuses[ExecuteBeadStatusReviewTerminalBlock],
		"review_terminal_block must not trigger escalation — spec gap / too_large requires operator decision")
}
