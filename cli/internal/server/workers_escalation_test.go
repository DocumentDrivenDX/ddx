package server

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscalationUsesAbstractMinPower(t *testing.T) {
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

	report, err := runEscalatingPowerAttempts(context.Background(), 10, 0, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
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
	assert.Equal(t, []int{10, 11}, requestedMinPowers)
	require.Len(t, recordedAttempts, 2)
	assert.Equal(t, []string{"cheap", "standard"}, []string{recordedAttempts[0].PowerClass, recordedAttempts[1].PowerClass})
	assert.Equal(t, []string{agent.ExecuteBeadStatusExecutionFailed, agent.ExecuteBeadStatusSuccess}, []string{recordedAttempts[0].Status, recordedAttempts[1].Status})
}

func TestEscalationRaisesAboveCurrentWhenActualPowerIsLower(t *testing.T) {
	var requestedMinPowers []int
	attempt := 0
	report, err := runEscalatingPowerAttempts(context.Background(), 10, 0, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		requestedMinPowers = append(requestedMinPowers, requestedMinPower)
		attempt++
		if attempt == 2 {
			return agent.ExecuteBeadReport{Status: agent.ExecuteBeadStatusSuccess}, nil
		}
		return agent.ExecuteBeadReport{
			BeadID:      "ddx-bead-2",
			PowerClass:  "cheap",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "build failed",
			ActualPower: 5,
		}, nil
	}, nil, true)

	require.NoError(t, err)
	require.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
	assert.Equal(t, []int{10, 11}, requestedMinPowers)
}

func TestEscalationDoesNotNeedRouteCatalog(t *testing.T) {
	report, err := runEscalatingPowerAttempts(context.Background(), 10, 0, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			Status:      agent.ExecuteBeadStatusSuccess,
			ActualPower: requestedMinPower,
		}, nil
	}, nil, true)

	require.NoError(t, err)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
}

func TestEscalationMaxPowerStopsBeforeInvalidSecondExecute(t *testing.T) {
	requested := make([]int, 0, 2)
	report, err := runEscalatingPowerAttempts(context.Background(), 10, 11, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		requested = append(requested, requestedMinPower)
		return agent.ExecuteBeadReport{
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "build failed",
			ActualPower: 10,
		}, nil
	}, nil, true)

	require.NoError(t, err)
	assert.Equal(t, []int{10}, requested, "MinPower must remain strictly below MaxPower")
	assert.Equal(t, agent.ExecuteBeadStatusExecutionFailed, report.Status)
}

func TestEscalationMaxPowerAllowsValidStrongerFloor(t *testing.T) {
	requested := make([]int, 0, 2)
	report, err := runEscalatingPowerAttempts(context.Background(), 9, 11, func(_ context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
		requested = append(requested, requestedMinPower)
		if len(requested) == 2 {
			return agent.ExecuteBeadReport{Status: agent.ExecuteBeadStatusSuccess}, nil
		}
		return agent.ExecuteBeadReport{
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "build failed",
			ActualPower: 9,
		}, nil
	}, nil, true)

	require.NoError(t, err)
	assert.Equal(t, []int{9, 10}, requested)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)
}
