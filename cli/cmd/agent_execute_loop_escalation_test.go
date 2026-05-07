package cmd

import (
	"context"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	)

	require.NoError(t, err)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
	assert.Equal(t, []int{0, 70, 90}, requested, "the helper should invoke the attempt once per rung")
}
