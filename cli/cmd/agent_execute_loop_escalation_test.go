package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	policyescalation "github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEscalationLadder struct {
	floors []int
}

func (l testEscalationLadder) Next(actualPower int) (int, error) {
	for _, floor := range l.floors {
		if floor > actualPower {
			return floor, nil
		}
	}
	return 0, errTestLadderExhausted{}
}

type errTestLadderExhausted struct{}

func (errTestLadderExhausted) Error() string { return "test ladder exhausted" }

func TestRecentProviderConnectivityMinPowerFromEventsUsesRoutedFailure(t *testing.T) {
	now := time.Date(2026, 5, 14, 8, 40, 0, 0, time.UTC)
	events := []bead.BeadEvent{
		{
			Kind:      "routing",
			Body:      `{"resolved_provider":"bragi","resolved_model":"qwen3.5-27b","actual_power":5}`,
			CreatedAt: now.Add(-2 * time.Minute),
		},
		{
			Kind:      "execute-bead",
			Summary:   "execution_failed",
			Body:      `agent: provider error: dial tcp 100.127.38.115:1234: i/o timeout`,
			CreatedAt: now.Add(-2 * time.Minute),
		},
	}

	floor, ok := recentProviderConnectivityMinPowerFromEvents(events, now, 0, 0, testEscalationLadder{floors: []int{5, 70, 90}})

	assert.True(t, ok)
	assert.Equal(t, 70, floor)
}

func TestRecentProviderConnectivityMinPowerFromEventsIgnoresExpiredEvidence(t *testing.T) {
	now := time.Date(2026, 5, 14, 8, 40, 0, 0, time.UTC)
	events := []bead.BeadEvent{
		{
			Kind:      "routing",
			Body:      `{"resolved_provider":"bragi","resolved_model":"qwen3.5-27b","actual_power":5}`,
			CreatedAt: now.Add(-20 * time.Minute),
		},
		{
			Kind:      "execute-bead",
			Summary:   "execution_failed",
			Body:      `agent: provider error: dial tcp 100.127.38.115:1234: i/o timeout`,
			CreatedAt: now.Add(-20 * time.Minute),
		},
	}

	floor, ok := recentProviderConnectivityMinPowerFromEvents(events, now, 0, 0, testEscalationLadder{floors: []int{5, 70, 90}})

	assert.False(t, ok)
	assert.Equal(t, 0, floor)
}

func TestInvestigationRetryIgnoresNumericLegacyRetryFloor(t *testing.T) {
	b := &bead.Bead{
		ID:    "ddx-ignore-numeric-hint",
		Title: "Ignore numeric retry floor",
		Extra: map[string]any{
			"work-next-min-power": 90,
		},
	}

	floor, report, unavailable := investigationRetryInitialMinPower(b, 7, 8, nil)

	assert.False(t, unavailable)
	assert.Equal(t, 7, floor)
	assert.Equal(t, agent.ExecuteBeadReport{}, report)
}

// fixedFloorLadder always returns the same floor without erroring. Used to
// verify that highestViableEscalationFloor's monotonic-progress guard
// terminates on a ladder that violates the strict-monotonic contract.
type fixedFloorLadder struct{ floor int }

func (l fixedFloorLadder) Next(actualPower int) (int, error) { return l.floor, nil }

func TestResolvePowerFloorStandardEmptyLadderDegradesToCheap(t *testing.T) {
	floor, ok := resolvePowerFloor(policyescalation.PowerStandard, testEscalationLadder{floors: nil})

	assert.False(t, ok)
	assert.Equal(t, 0, floor)
}

func TestResolvePowerFloorStandardSingleFloorReturnsThatFloor(t *testing.T) {
	floor, ok := resolvePowerFloor(policyescalation.PowerStandard, testEscalationLadder{floors: []int{5}})

	assert.True(t, ok)
	assert.Equal(t, 5, floor)
}

func TestResolvePowerFloorStandardTwoFloorsReturnsSecond(t *testing.T) {
	floor, ok := resolvePowerFloor(policyescalation.PowerStandard, testEscalationLadder{floors: []int{5, 10}})

	assert.True(t, ok)
	assert.Equal(t, 10, floor)
}

func TestResolvePowerFloorSmartEmptyLadderDegradesToCheap(t *testing.T) {
	floor, ok := resolvePowerFloor(policyescalation.PowerSmart, testEscalationLadder{floors: nil})

	assert.False(t, ok)
	assert.Equal(t, 0, floor)
}

func TestResolvePowerFloorSmartSingleFloorDegradesToStandard(t *testing.T) {
	// Smart cannot find a "highest" floor with a single-element ladder
	// (highestViableEscalationFloor only returns a floor when a subsequent
	// Next call confirms exhaustion via the right sentinel). It degrades to
	// Standard, which finds the single floor.
	floor, ok := resolvePowerFloor(policyescalation.PowerSmart, testEscalationLadder{floors: []int{5}})

	assert.True(t, ok)
	assert.Equal(t, 5, floor)
}

func TestResolvePowerFloorCheapReturnsNoPin(t *testing.T) {
	floor, ok := resolvePowerFloor(policyescalation.PowerCheap, testEscalationLadder{floors: []int{5, 10, 20}})

	assert.False(t, ok)
	assert.Equal(t, 0, floor)
}

func TestHighestViableEscalationFloorTerminatesOnNonMonotonicLadder(t *testing.T) {
	// fixedFloorLadder.Next always returns 5 without erroring. Without the
	// monotonic-progress guard, highestViableEscalationFloor would loop
	// forever. With the guard, it terminates and returns the floor it
	// observed.
	floor, err := highestViableEscalationFloor(fixedFloorLadder{floor: 5})

	assert.NoError(t, err)
	assert.Equal(t, 5, floor)
}

func TestZeroConfigInitialMinPowerStandardEmptyLadderDegradesWithNote(t *testing.T) {
	b := &bead.Bead{ID: "ddx-degrade-standard"}

	minPower, report, note, unavailable := zeroConfigInitialMinPower(b, policyescalation.PowerStandard, 0, 0, testEscalationLadder{floors: nil})

	assert.False(t, unavailable, "degradation must not surface as operator-attention")
	assert.Equal(t, agent.ExecuteBeadReport{}, report, "no routeUnavailableReport expected on degrade")
	assert.Equal(t, 0, minPower, "degraded class hands no MinPower pin to Fizeau")
	assert.NotEmpty(t, note, "degradation must produce a RoutingIntentNote")
	assert.True(t, strings.Contains(note, "standard"), "note should name the inferred class: %q", note)
	assert.True(t, strings.Contains(note, "cheap-equivalent"), "note should describe the fallback: %q", note)
}

func TestZeroConfigInitialMinPowerSmartEmptyLadderDegradesWithNote(t *testing.T) {
	b := &bead.Bead{ID: "ddx-degrade-smart"}

	minPower, report, note, unavailable := zeroConfigInitialMinPower(b, policyescalation.PowerSmart, 0, 0, testEscalationLadder{floors: nil})

	assert.False(t, unavailable)
	assert.Equal(t, agent.ExecuteBeadReport{}, report)
	assert.Equal(t, 0, minPower)
	assert.NotEmpty(t, note)
	assert.True(t, strings.Contains(note, "smart"), "note should name the inferred class: %q", note)
}

func TestZeroConfigInitialMinPowerCheapEmitsNoNote(t *testing.T) {
	b := &bead.Bead{ID: "ddx-cheap-no-note"}

	minPower, report, note, unavailable := zeroConfigInitialMinPower(b, policyescalation.PowerCheap, 0, 0, testEscalationLadder{floors: []int{5, 10}})

	assert.False(t, unavailable)
	assert.Equal(t, agent.ExecuteBeadReport{}, report)
	assert.Equal(t, 0, minPower)
	assert.Empty(t, note, "PowerCheap is not a degradation; no note expected")
}

func TestZeroConfigInitialMinPowerEmptyPowerClassPassthrough(t *testing.T) {
	b := &bead.Bead{ID: "ddx-empty-class"}

	minPower, report, note, unavailable := zeroConfigInitialMinPower(b, "", 7, 0, testEscalationLadder{floors: []int{5, 10}})

	assert.False(t, unavailable)
	assert.Equal(t, agent.ExecuteBeadReport{}, report)
	assert.Equal(t, 7, minPower, "empty class returns baseMinPower unchanged")
	assert.Empty(t, note)
}

func TestZeroConfigInitialMinPowerStandardLadderFloorPinsMinPower(t *testing.T) {
	b := &bead.Bead{ID: "ddx-standard-pin"}

	minPower, report, note, unavailable := zeroConfigInitialMinPower(b, policyescalation.PowerStandard, 0, 0, testEscalationLadder{floors: []int{5, 10}})

	assert.False(t, unavailable)
	assert.Equal(t, agent.ExecuteBeadReport{}, report)
	assert.Equal(t, 10, minPower, "standard with two floors should pin at the second floor")
	assert.Empty(t, note, "successful pin does not produce a degradation note")
}

func TestZeroConfigInitialMinPowerMaxPowerConflictStillUnavailable(t *testing.T) {
	// The maxPower conflict path (explicit user constraint) must continue to
	// surface as route-unavailable — that one is correct, not the bug being
	// fixed.
	b := &bead.Bead{ID: "ddx-max-power-conflict"}

	minPower, report, note, unavailable := zeroConfigInitialMinPower(b, policyescalation.PowerStandard, 0, 5, testEscalationLadder{floors: []int{5, 10}})

	assert.True(t, unavailable, "explicit MaxPower<floor conflict must still be operator-attention")
	assert.NotEqual(t, agent.ExecuteBeadReport{}, report, "routeUnavailableReport expected on MaxPower conflict")
	assert.Equal(t, 0, minPower)
	assert.Empty(t, note)
}

func TestWorkRetryEscalation_NiflheimEvidence_OnlySemanticFailuresRaiseMinPower(t *testing.T) {
	cases := []struct {
		name          string
		first         agent.ExecuteBeadReport
		wantRequested []int
	}{
		{
			name: "semantic_test_failure_escalates",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusExecutionFailed,
				Detail:        "tests failed: assertion mismatch",
				OutcomeReason: "tests_red",
				ActualPower:   5,
			},
			wantRequested: []int{5, 10},
		},
		{
			name: "semantic_review_block_escalates",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusReviewBlock,
				Detail:        "review blocked on missing AC coverage",
				OutcomeReason: agent.ExecuteBeadStatusReviewBlock,
				ActualPower:   5,
			},
			wantRequested: []int{5, 10},
		},
		{
			name: "semantic_review_request_changes_escalates",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusReviewRequestChanges,
				Detail:        "review requested changes for acceptance gaps",
				OutcomeReason: agent.ExecuteBeadStatusReviewRequestChanges,
				ActualPower:   5,
			},
			wantRequested: []int{5, 10},
		},
		{
			name: "non_semantic_land_conflict_stops",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusLandConflict,
				Detail:        "merge conflict while landing",
				OutcomeReason: agent.FailureModeMergeConflict,
				ActualPower:   5,
			},
			wantRequested: []int{5},
		},
		{
			name: "non_semantic_preclaim_warning_stops",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusExecutionFailed,
				Detail:        "pre-claim hook reported system_unready",
				OutcomeReason: "operator_required",
				ActualPower:   5,
			},
			wantRequested: []int{5},
		},
		{
			name: "non_semantic_decomposed_parent_stops",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusDeclinedNeedsDecomposition,
				Detail:        "parent bead was already decomposed",
				OutcomeReason: "decomposed",
				ActualPower:   5,
			},
			wantRequested: []int{5},
		},
		{
			name: "non_semantic_dirty_evidence_stops",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusExecutionFailed,
				Detail:        "dirty generated DDx evidence: uncommitted tracked changes",
				OutcomeReason: agent.FailureModeAttemptIntegrity,
				ActualPower:   5,
			},
			wantRequested: []int{5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requested := make([]int, 0, 2)
			reports := []agent.ExecuteBeadReport{
				tc.first,
				{
					Status:        agent.ExecuteBeadStatusSuccess,
					OutcomeReason: "",
					ActualPower:   10,
				},
			}
			idx := 0
			report, err := runEscalatingPowerAttempts(
				context.Background(),
				5,
				testEscalationLadder{floors: []int{10, 20}},
				func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
					requested = append(requested, requestedMinPower)
					if idx >= len(reports) {
						return reports[len(reports)-1], nil
					}
					got := reports[idx]
					idx++
					return got, nil
				},
				nil,
				nil,
				true,
			)
			require.NoError(t, err)
			require.Equal(t, tc.wantRequested, requested)
			if len(tc.wantRequested) == 1 {
				assert.Equal(t, tc.first.Status, report.Status)
				assert.Equal(t, tc.first.OutcomeReason, report.OutcomeReason)
				return
			}
			assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
		})
	}
}

func TestWorkRetryEscalation_ProviderConnectivityRetriesAtExactHigherFloor(t *testing.T) {
	requested := make([]int, 0, 2)
	reports := []agent.ExecuteBeadReport{
		{
			Status:        agent.ExecuteBeadStatusExecutionFailed,
			Detail:        "provider request failed: dial tcp 100.70.199.113:1235: connect: connection refused",
			Error:         "provider request failed: dial tcp 100.70.199.113:1235: connect: connection refused",
			Provider:      "vidar",
			Model:         "qwen-local",
			ActualPower:   5,
			OutcomeReason: "timeout",
		},
		{
			Status:        agent.ExecuteBeadStatusSuccess,
			ActualPower:   10,
			OutcomeReason: "",
		},
	}
	idx := 0

	report, err := runEscalatingPowerAttempts(
		context.Background(),
		5,
		testEscalationLadder{floors: []int{6, 10}},
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			requested = append(requested, requestedMinPower)
			if idx >= len(reports) {
				return reports[len(reports)-1], nil
			}
			got := reports[idx]
			idx++
			return got, nil
		},
		nil,
		nil,
		true,
	)

	require.NoError(t, err)
	require.Equal(t, []int{5, 6}, requested)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
}
