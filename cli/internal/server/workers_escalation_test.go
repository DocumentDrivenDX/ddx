package server

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	powerladder "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscalationLadder_WiredIntoExecutor(t *testing.T) {
	ladder := powerladder.NewLadder([]agentlib.ModelInfo{
		{Power: 10, Available: true, AutoRoutable: true},
		{Power: 20, Available: true, AutoRoutable: true},
		{Power: 30, Available: true, AutoRoutable: true},
	})

	var requestedMinPowers []int
	var recordedAttempts []escalation.PowerAttemptRecord
	attempts := []agent.ExecuteBeadReport{
		{
			BeadID:      "ddx-bead-1",
			PowerClass:  "cheap",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "build failed",
			ActualPower: 10,
		},
		{
			BeadID:      "ddx-bead-1",
			PowerClass:  "standard",
			Status:      agent.ExecuteBeadStatusSuccess,
			Detail:      "merged",
			ActualPower: 20,
		},
	}

	report, err := runEscalatingPowerAttempts(context.Background(), 10, ladder, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		requestedMinPowers = append(requestedMinPowers, requestedMinPower)
		if len(requestedMinPowers) > len(attempts) {
			t.Fatalf("attempt helper was called too many times: %v", requestedMinPowers)
		}
		return attempts[len(requestedMinPowers)-1], nil
	}, func(report agent.ExecuteBeadReport) {
		recordedAttempts = append(recordedAttempts, escalation.PowerAttemptRecord{
			PowerClass: report.PowerClass,
			Harness:    report.Harness,
			Model:      report.Model,
			Status:     report.Status,
			CostUSD:    report.CostUSD,
			DurationMS: report.DurationMS,
		})
	}, true)

	require.NoError(t, err)
	require.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
	assert.Equal(t, []int{10, 20}, requestedMinPowers)
	require.Len(t, recordedAttempts, 2)
	assert.Equal(t, []string{"cheap", "standard"}, []string{recordedAttempts[0].PowerClass, recordedAttempts[1].PowerClass})
	assert.Equal(t, []string{agent.ExecuteBeadStatusExecutionFailed, agent.ExecuteBeadStatusSuccess}, []string{recordedAttempts[0].Status, recordedAttempts[1].Status})
}

func TestEscalationLadder_StopsAtFinalRung(t *testing.T) {
	ladder := powerladder.NewLadder([]agentlib.ModelInfo{
		{Power: 10, Available: true, AutoRoutable: true},
		{Power: 20, Available: true, AutoRoutable: true},
	})

	var requestedMinPowers []int
	report, err := runEscalatingPowerAttempts(context.Background(), 10, ladder, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		requestedMinPowers = append(requestedMinPowers, requestedMinPower)
		return agent.ExecuteBeadReport{
			BeadID:      "ddx-bead-2",
			PowerClass:  "cheap",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "build failed",
			ActualPower: requestedMinPower,
		}, nil
	}, nil, true)

	require.NoError(t, err)
	require.Equal(t, agent.ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Equal(t, []int{10, 20}, requestedMinPowers)
}
