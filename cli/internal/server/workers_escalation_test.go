package server

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscalationLadder_WiredIntoExecutor(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 10, Available: true, AutoRoutable: true},
		{Power: 20, Available: true, AutoRoutable: true},
		{Power: 30, Available: true, AutoRoutable: true},
	})

	var requestedMinPowers []int
	attempts := []agent.ExecuteBeadReport{
		{
			BeadID:      "ddx-bead-1",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "build failed",
			ActualPower: 10,
		},
		{
			BeadID:      "ddx-bead-1",
			Status:      agent.ExecuteBeadStatusSuccess,
			Detail:      "merged",
			ActualPower: 20,
		},
	}

	report, err := runEscalatingSingleTierAttempts(context.Background(), 10, ladder, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		requestedMinPowers = append(requestedMinPowers, requestedMinPower)
		if len(requestedMinPowers) > len(attempts) {
			t.Fatalf("attempt helper was called too many times: %v", requestedMinPowers)
		}
		return attempts[len(requestedMinPowers)-1], nil
	})

	require.NoError(t, err)
	require.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
	assert.Equal(t, []int{10, 20}, requestedMinPowers)
}

func TestEscalationLadder_StopsAtFinalRung(t *testing.T) {
	ladder := tierescalation.NewLadder([]agentlib.ModelInfo{
		{Power: 10, Available: true, AutoRoutable: true},
		{Power: 20, Available: true, AutoRoutable: true},
	})

	var requestedMinPowers []int
	report, err := runEscalatingSingleTierAttempts(context.Background(), 10, ladder, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		requestedMinPowers = append(requestedMinPowers, requestedMinPower)
		return agent.ExecuteBeadReport{
			BeadID:      "ddx-bead-2",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "build failed",
			ActualPower: requestedMinPower,
		}, nil
	})

	require.NoError(t, err)
	require.Equal(t, agent.ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Equal(t, []int{10, 20}, requestedMinPowers)
}
