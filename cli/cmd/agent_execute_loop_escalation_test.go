package cmd

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedFizeauProviderFailureIngressDoesNotReadStderr(t *testing.T) {
	first := agent.ExecuteBeadReport{
		Status:        agent.ExecuteBeadStatusExecutionFailed,
		OutcomeReason: agent.FailureModeProviderRateLimit,
		Detail:        "model unavailable: this text must be ignored",
		Error:         "harness unavailable: this text must be ignored",
		Stderr:        "connection refused: this text must be ignored",
	}
	second := agent.ExecuteBeadReport{
		Status:        agent.ExecuteBeadStatusExecutionFailed,
		OutcomeReason: agent.FailureModeProviderRateLimit,
		Detail:        "different ignored detail",
		Error:         "different ignored error",
		Stderr:        "different ignored stderr",
	}

	pf1, ok1 := normalizeProviderFailureReport(&first)
	require.True(t, ok1)
	pf2, ok2 := normalizeProviderFailureReport(&second)
	require.True(t, ok2)

	assert.Equal(t, agent.FailureModeProviderRateLimit, pf1.Reason)
	assert.Equal(t, agent.FailureModeProviderRateLimit, pf2.Reason)
	assert.Equal(t, first.OutcomeReason, second.OutcomeReason)
	assert.Equal(t, agent.FailureModeProviderRateLimit, first.OutcomeReason)
	assert.Equal(t, agent.FailureModeProviderRateLimit, second.OutcomeReason)
	assert.Equal(t, first.DisruptionReason, second.DisruptionReason)
	assert.Equal(t, first.Disrupted, second.Disrupted)
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
			wantRequested: []int{5, 6},
		},
		{
			name: "semantic_review_block_escalates",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusReviewBlock,
				Detail:        "review blocked on missing AC coverage",
				OutcomeReason: agent.ExecuteBeadStatusReviewBlock,
				ActualPower:   5,
			},
			wantRequested: []int{5, 6},
		},
		{
			name: "semantic_review_request_changes_escalates",
			first: agent.ExecuteBeadReport{
				Status:        agent.ExecuteBeadStatusReviewRequestChanges,
				Detail:        "review requested changes for acceptance gaps",
				OutcomeReason: agent.ExecuteBeadStatusReviewRequestChanges,
				ActualPower:   5,
			},
			wantRequested: []int{5, 6},
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
				0,
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
				agent.ProviderPin{},
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
			OutcomeReason: agent.FailureModeProviderConnectivity,
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
		0,
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
		agent.ProviderPin{},
	)

	require.NoError(t, err)
	require.Equal(t, []int{5, 6}, requested)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
}

func TestWorkRetryEscalation_MaxPowerStopsBeforeInvalidSecondExecute(t *testing.T) {
	requested := make([]int, 0, 2)
	report, err := runEscalatingPowerAttempts(
		context.Background(),
		10,
		11,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			requested = append(requested, requestedMinPower)
			return agent.ExecuteBeadReport{
				Status:      agent.ExecuteBeadStatusExecutionFailed,
				Detail:      "build failed",
				ActualPower: 10,
			}, nil
		},
		nil,
		nil,
		true,
		agent.ProviderPin{},
	)

	require.NoError(t, err)
	assert.Equal(t, []int{10}, requested, "MinPower must remain strictly below MaxPower")
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, report.Status)
}

func TestWorkRetryEscalation_MaxPowerAllowsValidStrongerFloor(t *testing.T) {
	requested := make([]int, 0, 2)
	report, err := runEscalatingPowerAttempts(
		context.Background(),
		9,
		11,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			requested = append(requested, requestedMinPower)
			if len(requested) == 2 {
				return agent.ExecuteBeadReport{Status: agent.ExecuteBeadStatusSuccess}, nil
			}
			return agent.ExecuteBeadReport{
				Status:      agent.ExecuteBeadStatusExecutionFailed,
				Detail:      "build failed",
				ActualPower: 9,
			}, nil
		},
		nil,
		nil,
		true,
		agent.ProviderPin{},
	)

	require.NoError(t, err)
	assert.Equal(t, []int{9, 10}, requested)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
}
