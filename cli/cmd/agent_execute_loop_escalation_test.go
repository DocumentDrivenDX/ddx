package cmd

import (
	"context"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	policyescalation "github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
)

type ladderResponse struct {
	next int
	err  error
}

type ladderSpy struct {
	mu        sync.Mutex
	calls     []int
	responses []ladderResponse
}

func (s *ladderSpy) Next(actualPower int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, actualPower)
	idx := len(s.calls) - 1
	if idx >= len(s.responses) {
		return 0, tierescalation.ErrLadderExhausted
	}
	resp := s.responses[idx]
	return resp.next, resp.err
}

func (s *ladderSpy) CalledWith() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, len(s.calls))
	copy(out, s.calls)
	return out
}

func TestEscalationLadder_WiredIntoExecutor(t *testing.T) {
	ladder := &ladderSpy{responses: []ladderResponse{{next: 70}}}
	var requested []int
	attempts := 0

	report, err := runEscalatingSingleTierAttempts(
		context.Background(),
		0,
		ladder,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			requested = append(requested, requestedMinPower)
			attempts++
			switch attempts {
			case 1:
				return agent.ExecuteBeadReport{
					BeadID:      "ddx-escalation-001",
					ActualPower: 50,
					Status:      agent.ExecuteBeadStatusExecutionFailed,
					Detail:      "TestFoo failed: assertion mismatch",
				}, nil
			case 2:
				return agent.ExecuteBeadReport{
					BeadID:      "ddx-escalation-001",
					ActualPower: 70,
					Status:      agent.ExecuteBeadStatusSuccess,
					Detail:      "ok",
				}, nil
			default:
				t.Fatalf("unexpected attempt %d", attempts)
				return agent.ExecuteBeadReport{}, nil
			}
		},
		nil,
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
	assert.Equal(t, []int{0, 70}, requested, "executor should run once per rung it reaches")
	assert.Equal(t, []int{50}, ladder.CalledWith(), "ladder.Next must consume the failed attempt's actual power")
}

func TestEscalationLadder_StopsAtFinalRung(t *testing.T) {
	ladder := &ladderSpy{responses: []ladderResponse{
		{next: 70},
		{err: tierescalation.ErrLadderExhausted},
	}}
	var requested []int
	attempts := 0

	report, err := runEscalatingSingleTierAttempts(
		context.Background(),
		0,
		ladder,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			attempts++
			requested = append(requested, requestedMinPower)
			switch attempts {
			case 1:
				return agent.ExecuteBeadReport{
					BeadID:      "ddx-escalation-002",
					ActualPower: 50,
					Status:      agent.ExecuteBeadStatusExecutionFailed,
					Detail:      "TestFoo failed: assertion mismatch",
				}, nil
			case 2:
				return agent.ExecuteBeadReport{
					BeadID:      "ddx-escalation-002",
					ActualPower: 70,
					Status:      agent.ExecuteBeadStatusExecutionFailed,
					Detail:      "TestFoo failed: assertion mismatch",
				}, nil
			default:
				t.Fatalf("unexpected attempt %d", attempts)
				return agent.ExecuteBeadReport{}, nil
			}
		},
		nil,
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Equal(t, []int{0, 70}, requested, "helper must stop after the final rung exhausts")
	assert.Equal(t, []int{50, 70}, ladder.CalledWith(), "ladder.Next should be called once per substantive failure")
}

func TestZeroConfigEscalationIntegration_RealLadderAdvancesMinPower(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 50, Available: true, AutoRoutable: true},
		{Power: 70, Available: true, AutoRoutable: true},
		{Power: 90, Available: true, AutoRoutable: true},
	})

	var requested []int
	attempts := []agent.ExecuteBeadReport{
		{
			BeadID:      "ddx-escalation-003",
			ActualPower: 50,
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "TestFoo failed: assertion mismatch",
		},
		{
			BeadID:      "ddx-escalation-003",
			ActualPower: 70,
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "TestFoo failed: assertion mismatch",
		},
		{
			BeadID:      "ddx-escalation-003",
			ActualPower: 90,
			Status:      agent.ExecuteBeadStatusSuccess,
			Detail:      "ok",
		},
	}
	idx := 0

	report, err := runEscalatingSingleTierAttempts(
		context.Background(),
		0,
		ladder,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			requested = append(requested, requestedMinPower)
			r := attempts[idx]
			idx++
			return r, nil
		},
		nil,
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
	assert.Equal(t, []int{0, 70, 90}, requested, "the helper should invoke the attempt once per rung")
}

func TestInvestigationRetry_RequestsSmartRoute(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 50, Available: true, AutoRoutable: true},
		{Power: 70, Available: true, AutoRoutable: false},
		{Power: 90, Available: true, AutoRoutable: true},
	})
	target := &bead.Bead{
		ID: "ddx-smart-retry",
		Extra: map[string]any{
			agent.TriageTierHintKey: string(policyescalation.TierSmart),
		},
	}

	initialMinPower, unavailableReport, unavailable := investigationRetryInitialMinPower(target, 0, 0, ladder)
	require.False(t, unavailable, "a viable smart retry route should not produce a routing-unavailable report")
	assert.Empty(t, unavailableReport.Status)
	assert.Equal(t, 90, initialMinPower, "smart investigation retry should start at the strongest viable abstract power floor")

	var requested []int
	report, err := runEscalatingSingleTierAttempts(
		context.Background(),
		initialMinPower,
		ladder,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			requested = append(requested, requestedMinPower)
			return agent.ExecuteBeadReport{
				BeadID:      target.ID,
				ActualPower: requestedMinPower,
				Status:      agent.ExecuteBeadStatusSuccess,
				Detail:      "ok",
			}, nil
		},
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
	assert.Equal(t, []int{90}, requested, "the next attempt should receive only the raised MinPower floor")
}

// TestPerBeadCostTracker_StopsEscalationAtLimit asserts that when the
// accumulated per-bead billed cost reaches the configured budget, escalation
// stops and the returned report signals per-bead budget exhaustion.
func TestPerBeadCostTracker_StopsEscalationAtLimit(t *testing.T) {
	ladder := &ladderSpy{responses: []ladderResponse{
		{next: 70},
		{next: 90},
	}}
	attempts := 0

	// Budget is $3. First attempt costs $2 (below), second costs $2 (total $4 ≥ $3).
	tracker := policyescalation.NewPerBeadCostTracker(3.0, nil)

	report, err := runEscalatingSingleTierAttempts(
		context.Background(),
		0,
		ladder,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			attempts++
			switch attempts {
			case 1:
				return agent.ExecuteBeadReport{
					BeadID:      "ddx-per-bead-001",
					ActualPower: 50,
					Harness:     "openrouter",
					CostUSD:     2.0,
					Status:      agent.ExecuteBeadStatusExecutionFailed,
					Detail:      "TestFoo failed",
				}, nil
			case 2:
				return agent.ExecuteBeadReport{
					BeadID:      "ddx-per-bead-001",
					ActualPower: 70,
					Harness:     "openrouter",
					CostUSD:     2.0,
					Status:      agent.ExecuteBeadStatusExecutionFailed,
					Detail:      "TestFoo failed again",
				}, nil
			default:
				t.Fatalf("unexpected attempt %d", attempts)
				return agent.ExecuteBeadReport{}, nil
			}
		},
		nil,
		tracker,
	)

	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "should stop after two attempts (second trips the budget)")
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, report.Status)
	assert.True(t, strings.Contains(report.Detail, policyescalation.PerBeadBudgetExhaustedReason),
		"detail must contain per-bead budget exhausted marker, got: %q", report.Detail)
	assert.Equal(t, 4.0, report.CostUSD, "CostUSD on the report should be the total bead spend")
}

// TestPerBeadCostTracker_NoLimit_RunsToLadderExhaustion asserts that a
// nil PerBeadCostTracker (unlimited) does not interrupt escalation — the loop
// runs all the way through ladder exhaustion.
func TestPerBeadCostTracker_NoLimit_RunsToLadderExhaustion(t *testing.T) {
	ladder := &ladderSpy{responses: []ladderResponse{
		{next: 70},
		// No more responses: second NextFloor call returns ErrLadderExhausted.
	}}
	attempts := 0

	report, err := runEscalatingSingleTierAttempts(
		context.Background(),
		0,
		ladder,
		func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
			attempts++
			return agent.ExecuteBeadReport{
				BeadID:      "ddx-per-bead-002",
				ActualPower: requestedMinPower,
				Harness:     "openrouter",
				CostUSD:     50.0, // high cost per attempt — should not stop with nil tracker
				Status:      agent.ExecuteBeadStatusExecutionFailed,
				Detail:      "TestFoo failed",
			}, nil
		},
		nil,
		nil, // no per-bead tracker = unlimited
	)

	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "should run through all available rungs (ladder exhaustion, not budget)")
	assert.False(t, strings.Contains(report.Detail, policyescalation.PerBeadBudgetExhaustedReason),
		"detail must not contain per-bead budget exhausted marker when tracker is nil")
}

// TestTierHint_StartsAtLabelFloor verifies that a bead with a tier:hint=<name>
// label starts at the resolved floor for that tier, not at the default zero.
func TestTierHint_StartsAtLabelFloor(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 50, Available: true, AutoRoutable: true},
		{Power: 70, Available: true, AutoRoutable: true},
		{Power: 90, Available: true, AutoRoutable: true},
	})

	tests := []struct {
		label     string
		wantFloor int
	}{
		// smart → highest viable floor
		{"tier:hint=smart", 90},
		// standard → second viable floor (skip cheapest)
		{"tier:hint=standard", 70},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			b := &bead.Bead{ID: "ddx-tier-hint-001", Labels: []string{tc.label}}
			floor, report, unavailable := investigationRetryInitialMinPower(b, 0, 0, ladder)
			require.False(t, unavailable, "valid tier hint must not produce unavailable report")
			assert.Empty(t, report.Status)
			assert.Equal(t, tc.wantFloor, floor, "initial floor must match the label-derived tier floor")
		})
	}
}

// TestTierHint_NoLabel_UsesDefault verifies that a bead without a tier:hint
// label uses the configured baseMinPower unchanged.
func TestTierHint_NoLabel_UsesDefault(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 50, Available: true, AutoRoutable: true},
		{Power: 90, Available: true, AutoRoutable: true},
	})
	b := &bead.Bead{ID: "ddx-tier-hint-002", Labels: []string{"phase:6", "area:agent"}}
	floor, report, unavailable := investigationRetryInitialMinPower(b, 0, 0, ladder)
	require.False(t, unavailable)
	assert.Empty(t, report.Status)
	assert.Equal(t, 0, floor, "no tier:hint label must leave the floor at the configured default")
}

// TestTierHint_InvalidLabel_FallsBackToDefault verifies that an unrecognized
// tier name in a tier:hint label falls back to the default floor without error.
func TestTierHint_InvalidLabel_FallsBackToDefault(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 50, Available: true, AutoRoutable: true},
		{Power: 90, Available: true, AutoRoutable: true},
	})
	b := &bead.Bead{ID: "ddx-tier-hint-003", Labels: []string{"tier:hint=nonexistent"}}
	floor, report, unavailable := investigationRetryInitialMinPower(b, 0, 0, ladder)
	require.False(t, unavailable, "invalid tier name must not produce unavailable report")
	assert.Empty(t, report.Status)
	assert.Equal(t, 0, floor, "unrecognized tier:hint must fall back to the default floor")
}

// TestTierHint_FlagRaisesFloorAboveLabel verifies that the --min-power flag
// (represented as baseMinPower) can raise the initial floor above the label-
// derived floor but cannot lower it below.
func TestTierHint_FlagRaisesFloorAboveLabel(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 50, Available: true, AutoRoutable: true},
		{Power: 70, Available: true, AutoRoutable: true},
		{Power: 90, Available: true, AutoRoutable: true},
	})
	// tier:hint=standard resolves to floor=70 (second tier); --min-power=90 raises above
	b := &bead.Bead{ID: "ddx-tier-hint-004", Labels: []string{"tier:hint=standard"}}
	floor, report, unavailable := investigationRetryInitialMinPower(b, 90, 0, ladder)
	require.False(t, unavailable)
	assert.Empty(t, report.Status)
	assert.Equal(t, 90, floor, "--min-power flag must raise the floor above the label-derived floor")
}
