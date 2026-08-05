package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must remain open after escalation")
	assert.NotContains(t, got.Extra, legacyRetryFloorKey)
}

func TestRepairExhaustedAfterBlockEntersAutoRecovery(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce03", Title: "Repair cycle exhausted after BLOCK"}
	require.NoError(t, store.Create(context.Background(), b))

	var hookCalls int
	var observedClass RecoveryFailureClass
	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		hookCalls++
		assert.Equal(t, b.ID, beadID)
		observedClass = class
		return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Reframe}, nil
	})

	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          50,
		ReviewVerdict:        string(VerdictBlock),
		ReviewRationale:      "missing regression test",
		ReviewGroupID:        "rg-block",
		ReviewClassification: ReviewTerminalClassSpecGap,
		ReviewPerAC: []ReviewAC{{
			Number:   1,
			Item:     "Add regression test",
			Grade:    "BLOCK",
			Evidence: "pkg/foo_test.go:42",
		}},
		ReviewFindings: []Finding{{
			Severity: "block",
			Summary:  "missing regression test",
			Location: "pkg/foo_test.go:42",
		}},
	}

	err := applyRepairCycleExhaustedEscalation(context.Background(), store, b.ID, "worker", report, report.ActualPower, time.Now().UTC(), func(int) (int, error) {
		return 0, fmt.Errorf("ladder exhausted")
	}, hook)
	require.NoError(t, err)

	assert.Equal(t, 1, hookCalls, "repair exhaustion must invoke TD-031 auto-recovery")
	assert.Equal(t, SpecGap, observedClass, "evidenced BLOCK must recover using the spec-gap class")

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "recovery hook should short-circuit the old park-to-proposed fallback")

	event := findRepairCycleExhaustedEvent(t, store, b.ID)
	var body repairCycleExhaustedEventBody
	require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
	assert.Equal(t, "rg-block", body.ReviewGroupID)
	require.Len(t, body.ReviewFindings, 1)
	assert.Equal(t, "missing regression test", body.ReviewFindings[0].Summary)
}

func TestRepairExhaustedRecoveryLinksReviewGroup(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce04", Title: "Repair cycle exhausted links review group"}
	require.NoError(t, store.Create(context.Background(), b))

	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          65,
		ReviewVerdict:        string(VerdictBlock),
		ReviewRationale:      "spec gap needs a rewrite",
		ReviewGroupID:        "rg-link",
		ReviewClassification: ReviewTerminalClassSpecGap,
		ReviewPerAC: []ReviewAC{{
			Number:   2,
			Item:     "Document edge case",
			Grade:    "BLOCK",
			Evidence: "docs/notes.md:12",
		}},
		ReviewFindings: []Finding{{
			Severity: "block",
			Summary:  "spec gap needs a rewrite",
			Location: "docs/notes.md:12",
		}},
	}

	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		assert.Equal(t, b.ID, beadID)
		assert.Equal(t, SpecGap, class)
		return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Reframe}, nil
	})

	err := applyRepairCycleExhaustedEscalation(context.Background(), store, b.ID, "worker", report, report.ActualPower, time.Now().UTC(), func(int) (int, error) {
		return 0, fmt.Errorf("ladder exhausted")
	}, hook)
	require.NoError(t, err)

	event := findRepairCycleExhaustedEvent(t, store, b.ID)
	var body repairCycleExhaustedEventBody
	require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
	assert.Equal(t, "rg-link", body.ReviewGroupID)
	assert.Equal(t, ReviewTerminalClassSpecGap, body.ReviewClassification)
	require.Len(t, body.ReviewFindings, 1)
	assert.Equal(t, "spec gap needs a rewrite", body.ReviewFindings[0].Summary)
}

func TestRepairExhaustedFixableGapEntersAutoRecovery(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce05", Title: "Repair cycle exhausted after fixable gap"}
	require.NoError(t, store.Create(context.Background(), b))

	var hookCalls int
	var observedClass RecoveryFailureClass
	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		hookCalls++
		assert.Equal(t, b.ID, beadID)
		observedClass = class
		return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Decompose}, nil
	})

	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          40,
		ReviewVerdict:        string(VerdictRequestChanges),
		ReviewRationale:      "small fixable gap",
		ReviewGroupID:        "rg-gap",
		ReviewClassification: ReviewFindingClassFixableGap,
		ReviewPerAC: []ReviewAC{{
			Number:   3,
			Item:     "Add missing assertion",
			Grade:    "REQUEST_CHANGES",
			Evidence: "pkg/foo_test.go:84",
		}},
		ReviewFindings: []Finding{{
			Severity: "warn",
			Summary:  "small fixable gap",
			Location: "pkg/foo_test.go:84",
		}},
	}

	err := applyRepairCycleExhaustedEscalation(context.Background(), store, b.ID, "worker", report, report.ActualPower, time.Now().UTC(), func(int) (int, error) {
		return 0, fmt.Errorf("ladder exhausted")
	}, hook)
	require.NoError(t, err)

	assert.Equal(t, 1, hookCalls, "fixable-gap exhaustion must invoke TD-031 auto-recovery")
	assert.Equal(t, PersistentExecutionFailed, observedClass, "fixable-gap recovery should reuse the persistent execution failure path")

	event := findRepairCycleExhaustedEvent(t, store, b.ID)
	var body repairCycleExhaustedEventBody
	require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
	assert.Equal(t, "rg-gap", body.ReviewGroupID)
	assert.Equal(t, ReviewFindingClassFixableGap, body.ReviewClassification)
}

func findRepairCycleExhaustedEvent(t *testing.T, store *bead.Store, beadID string) bead.BeadEvent {
	t.Helper()
	events, err := store.Events(beadID)
	require.NoError(t, err)
	for _, ev := range events {
		if ev.Kind == ExecuteBeadStatusRepairCycleExhausted {
			return ev
		}
	}
	t.Fatalf("repair-cycle-exhausted event not found")
	return bead.BeadEvent{}
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

	got, err := store.Get(context.Background(), b.ID)
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
