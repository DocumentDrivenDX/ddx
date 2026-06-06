package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecompositionDepDirection verifies that instrStep0SizeCheck instructs the
// agent to add a parent→child dep (parent depends on children), not the reverse.
func TestDecompositionDepDirection(t *testing.T) {
	correct := "ddx bead dep add <parent-id> <child-id>"
	wrong := "ddx bead dep add <child-id> <parent-id>"

	assert.True(t,
		strings.Contains(instrStep0SizeCheck, correct),
		"instrStep0SizeCheck must instruct agent to run %q so the parent waits for children", correct,
	)
	assert.False(t,
		strings.Contains(instrStep0SizeCheck, wrong),
		"instrStep0SizeCheck must not contain %q (backwards dep direction that leaves parent execution-ready)", wrong,
	)
}

// TestMixedCommitCooldown verifies that the circuit-breaker parks a bead to
// proposed after 2 mixed_commit_and_no_changes_rationale outcomes within 24h.
func TestMixedCommitCooldown(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-mixed-cb", Title: "Mixed commit circuit breaker test"}
	require.NoError(t, store.Create(b))

	// Append a prior mixed_commit execute-bead event (the 1st occurrence).
	require.NoError(t, store.AppendEvent("ddx-mixed-cb", bead.BeadEvent{
		Kind:      "execute-bead",
		Summary:   ExecuteBeadStatusExecutionFailed,
		Body:      mixedCommitAndNoChangesRationaleReason,
		Actor:     "test-worker",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
	}))

	var callCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&callCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    mixedCommitAndNoChangesRationaleReason,
				BaseRev:   "abc1111",
				ResultRev: "def2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "test-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-mixed-cb",
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&callCount), "executor must run exactly once")

	got, err := store.Get("ddx-mixed-cb")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status,
		"bead must be parked to proposed after 2nd mixed_commit within 24h")
}

// TestMixedCommitCooldown_FirstOccurrenceDoesNotPark verifies that a single
// mixed_commit outcome (no prior events) does NOT park the bead.
func TestMixedCommitCooldown_FirstOccurrenceDoesNotPark(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-mixed-first", Title: "First mixed commit; must not park"}
	require.NoError(t, store.Create(b))

	// No prior events — this is the first occurrence.
	var callCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&callCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    mixedCommitAndNoChangesRationaleReason,
				BaseRev:   "abc1111",
				ResultRev: "def2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "test-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-mixed-first",
	})
	require.NoError(t, err)

	got, err := store.Get("ddx-mixed-first")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status,
		"bead must remain open after first mixed_commit (circuit-breaker needs 2 occurrences)")
}

// TestMixedCommitCooldown_OutsideWindowDoesNotPark verifies that a prior
// mixed_commit event older than 24h does not trigger the circuit-breaker.
func TestMixedCommitCooldown_OutsideWindowDoesNotPark(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-mixed-old", Title: "Old mixed commit; must not park"}
	require.NoError(t, store.Create(b))

	// Prior event is older than the 24h window.
	require.NoError(t, store.AppendEvent("ddx-mixed-old", bead.BeadEvent{
		Kind:      "execute-bead",
		Summary:   ExecuteBeadStatusExecutionFailed,
		Body:      mixedCommitAndNoChangesRationaleReason,
		Actor:     "test-worker",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC().Add(-25 * time.Hour),
	}))

	var callCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&callCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    mixedCommitAndNoChangesRationaleReason,
				BaseRev:   "abc1111",
				ResultRev: "def2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "test-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-mixed-old",
	})
	require.NoError(t, err)

	got, err := store.Get("ddx-mixed-old")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status,
		"bead must remain open when prior mixed_commit event is outside the 24h window")
}
