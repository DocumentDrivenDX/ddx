package cmd

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnpinnedWorkerFallsBackAfterTypedProviderFailure (AC3, ddx-3b721804): a
// zero-config / unpinned worker that hits a typed provider failure must record
// failed-route evidence and ask for a different eligible route (higher floor)
// rather than parking the bead on the first provider failure.
func TestUnpinnedWorkerFallsBackAfterTypedProviderFailure(t *testing.T) {
	requested := make([]int, 0, 2)
	captured := make([]agent.ExecuteBeadReport, 0, 2)
	reports := []agent.ExecuteBeadReport{
		{
			BeadID:        "ddx-fallback-1",
			Status:        agent.ExecuteBeadStatusExecutionFailed,
			Detail:        "provider request failed: 429 Too Many Requests: rate limit reached",
			Error:         "provider request failed: 429 Too Many Requests: rate limit reached",
			Provider:      "openrouter",
			Model:         "some-model",
			ActualPower:   5,
			OutcomeReason: "timeout", // coarse pre-typed bucket, safe to refine
		},
		{
			BeadID:      "ddx-fallback-1",
			Status:      agent.ExecuteBeadStatusSuccess,
			ActualPower: 10,
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
		func(r agent.ExecuteBeadReport) { captured = append(captured, r) },
		nil,
		true, // unpinned: infrastructure retry allowed
		agent.ProviderPin{},
	)
	require.NoError(t, err)

	// Fell back to a higher floor rather than parking on the first failure.
	require.Equal(t, []int{5, 6}, requested)
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)

	// The first attempt was typed and recorded failed-route evidence.
	require.GreaterOrEqual(t, len(captured), 1)
	first := captured[0]
	assert.Equal(t, agent.FailureModeProviderRateLimit, first.OutcomeReason)
	assert.True(t, first.Disrupted)
	require.NotNil(t, first.ProviderFailureEvidence, "failed-route evidence must be recorded")
	assert.Equal(t, agent.FailureModeProviderRateLimit, first.ProviderFailureEvidence.TypedFailure)
	assert.True(t, first.ProviderFailureEvidence.Retryable)
	assert.True(t, first.ProviderFailureEvidence.FallbackAttempted)
	assert.Empty(t, first.ProviderFailureEvidence.FallbackStopReason)
}

// TestPinnedWorkerReportsHardPinExhaustedWithoutWidening (AC4, ddx-3b721804): a
// worker with explicit --harness/--provider/--model pins must not have its pin
// silently removed when the pinned route hits a typed provider failure. It
// stops without widening and the report names the typed failure plus operator
// remediation.
func TestPinnedWorkerReportsHardPinExhaustedWithoutWidening(t *testing.T) {
	requested := make([]int, 0, 2)
	reports := []agent.ExecuteBeadReport{
		{
			BeadID:      "ddx-pinned-1",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "provider request failed: 429 Too Many Requests: rate limit reached",
			Error:       "provider request failed: 429 Too Many Requests: rate limit reached",
			Harness:     "claude",
			Provider:    "anthropic",
			Model:       "claude-opus",
			ActualPower: 5,
		},
		{
			// Should never be reached: a pinned worker must not widen/retry.
			Status:      agent.ExecuteBeadStatusSuccess,
			ActualPower: 10,
		},
	}
	idx := 0
	pin := agent.ProviderPin{Harness: "claude", Provider: "anthropic", Model: "claude-opus"}

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
		false, // pinned: infrastructure retry NOT allowed
		pin,
	)
	require.NoError(t, err)

	// No widening: exactly one attempt, no escalation.
	require.Equal(t, []int{5}, requested)

	// Typed failure preserved and the pin is named in operator remediation.
	assert.Equal(t, agent.FailureModeProviderRateLimit, report.OutcomeReason)
	assert.Contains(t, report.Detail, "hard pin exhausted")
	assert.Contains(t, report.Detail, "--harness claude")
	assert.Contains(t, report.Detail, "--provider anthropic")
	assert.Contains(t, report.Detail, "--model claude-opus")

	// Evidence records that fallback was intentionally not attempted.
	require.NotNil(t, report.ProviderFailureEvidence)
	assert.Equal(t, agent.FallbackStopHardPinExhausted, report.ProviderFailureEvidence.FallbackStopReason)
	assert.False(t, report.ProviderFailureEvidence.FallbackAttempted)
	assert.Equal(t, agent.FailureModeProviderRateLimit, report.ProviderFailureEvidence.TypedFailure)
}
