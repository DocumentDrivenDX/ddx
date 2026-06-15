package cmd

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkProviderModelUnavailableFallsBackWhenUnpinned (AC3, ddx-ef9df563):
// when the first route returns a model-not-found error, an unpinned worker must
// record the failed route as typed evidence (provider_model_unavailable) and
// retry an alternate route without operator intervention.
func TestWorkProviderModelUnavailableFallsBackWhenUnpinned(t *testing.T) {
	requested := make([]int, 0, 2)
	captured := make([]agent.ExecuteBeadReport, 0, 2)
	reports := []agent.ExecuteBeadReport{
		{
			BeadID:      "ddx-model-1",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "model claude-4 not found on this harness",
			Error:       "model claude-4 not found on this harness",
			Harness:     "claude",
			Provider:    "anthropic",
			Model:       "claude-4",
			ActualPower: 5,
		},
		{
			BeadID:      "ddx-model-1",
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
			got := reports[idx]
			if idx < len(reports)-1 {
				idx++
			}
			return got, nil
		},
		func(r agent.ExecuteBeadReport) { captured = append(captured, r) },
		nil,
		true, // unpinned: infrastructure retry allowed
		agent.ProviderPin{},
	)
	require.NoError(t, err)

	// Fell back to an alternate route rather than parking on the first failure.
	require.GreaterOrEqual(t, len(requested), 2, "unpinned worker must retry an alternate route")
	assert.Equal(t, agent.ExecuteBeadStatusSuccess, report.Status)

	// First attempt must be typed as provider_model_unavailable.
	require.GreaterOrEqual(t, len(captured), 1)
	first := captured[0]
	assert.Equal(t, agent.FailureModeProviderModelUnavailable, first.OutcomeReason,
		"model-not-found must classify as provider_model_unavailable, not a generic bucket")
	assert.True(t, first.Disrupted)

	// Route-health evidence must record the fallback decision.
	require.NotNil(t, first.ProviderFailureEvidence, "failed route must produce durable evidence")
	assert.Equal(t, agent.FailureModeProviderModelUnavailable, first.ProviderFailureEvidence.TypedFailure)
	assert.True(t, first.ProviderFailureEvidence.Retryable)
	assert.True(t, first.ProviderFailureEvidence.FallbackAttempted,
		"unpinned worker must record fallback_attempted=true")
	assert.Empty(t, first.ProviderFailureEvidence.FallbackStopReason,
		"fallback was not stopped; the worker continued to an alternate route")
}

// TestWorkProviderFailureHonorsHardPins (AC4, ddx-ef9df563):
// when a worker is pinned (--harness/--provider/--model) and the pinned route
// hits a provider failure, the pin must not be silently widened. The worker
// stops immediately with fallback_stop_reason=hard_pin_exhausted and the report
// Detail must name the pin so the operator can take targeted action.
func TestWorkProviderFailureHonorsHardPins(t *testing.T) {
	requested := make([]int, 0, 2)
	reports := []agent.ExecuteBeadReport{
		{
			BeadID:      "ddx-pin-1",
			Status:      agent.ExecuteBeadStatusExecutionFailed,
			Detail:      "executable file not found in $PATH: claude-code",
			Error:       "executable file not found in $PATH: claude-code",
			Harness:     "claude",
			Provider:    "anthropic",
			Model:       "claude-opus",
			ActualPower: 5,
		},
		{
			// Must never be reached: a pinned worker must not widen/retry.
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
			got := reports[idx]
			if idx < len(reports)-1 {
				idx++
			}
			return got, nil
		},
		nil,
		nil,
		false, // pinned: infrastructure retry NOT allowed
		pin,
	)
	require.NoError(t, err)

	// No widening: exactly one attempt.
	require.Equal(t, []int{5}, requested,
		"pinned worker must not widen its pin or attempt alternate routes")

	// Typed failure preserved on the report.
	assert.Equal(t, agent.FailureModeProviderHarnessUnavailable, report.OutcomeReason,
		"harness-unavailable must surface as typed reason, not generic execution_failed")

	// Operator-actionable output names the exhausted pin.
	assert.Contains(t, report.Detail, "hard pin exhausted",
		"operator output must describe the hard-pin exhaustion")
	assert.Contains(t, report.Detail, "--harness claude")
	assert.Contains(t, report.Detail, "--provider anthropic")
	assert.Contains(t, report.Detail, "--model claude-opus")

	// Evidence records that fallback was intentionally not attempted.
	require.NotNil(t, report.ProviderFailureEvidence)
	assert.Equal(t, agent.FallbackStopHardPinExhausted, report.ProviderFailureEvidence.FallbackStopReason)
	assert.False(t, report.ProviderFailureEvidence.FallbackAttempted,
		"pinned worker must record fallback_attempted=false")
	assert.Equal(t, agent.FailureModeProviderHarnessUnavailable, report.ProviderFailureEvidence.TypedFailure)
}

// TestWorkStatusAndOperatorAttentionSurfaceTypedProviderFailure (AC5, ddx-ef9df563):
// after a typed provider/process failure the final report must carry a typed
// outcome_reason (not a generic execution_failed or underspecified-routing
// bucket), and the operator-attention Detail must provide actionable remediation
// so `ddx work status --json` and operator-attention surfaces show the typed
// failure instead of a vague worker failure.
func TestWorkStatusAndOperatorAttentionSurfaceTypedProviderFailure(t *testing.T) {
	// Simulate a pinned worker hitting a harness-unavailable failure so we can
	// verify both the typed OutcomeReason and the operator-attention Detail.
	failReport := agent.ExecuteBeadReport{
		BeadID:      "ddx-surface-1",
		Status:      agent.ExecuteBeadStatusExecutionFailed,
		Detail:      "executable file not found in $PATH: claude-code",
		Error:       "executable file not found in $PATH: claude-code",
		Harness:     "claude",
		Provider:    "anthropic",
		ActualPower: 5,
	}
	pin := agent.ProviderPin{Harness: "claude", Provider: "anthropic"}

	finalReport, err := runEscalatingPowerAttempts(
		context.Background(),
		5,
		testEscalationLadder{floors: []int{6, 10}},
		func(_ context.Context, _ int) (agent.ExecuteBeadReport, error) {
			return failReport, nil
		},
		nil,
		nil,
		false, // pinned: no infrastructure retry
		pin,
	)
	require.NoError(t, err)

	// ddx work status --json surfaces OutcomeReason as the typed provider failure,
	// not a generic bucket that leaves the operator without a diagnosis.
	assert.Equal(t, agent.FailureModeProviderHarnessUnavailable, finalReport.OutcomeReason,
		"work status JSON must surface the typed provider failure reason")
	assert.NotEqual(t, agent.ExecuteBeadStatusExecutionFailed, finalReport.OutcomeReason,
		"must not reuse the generic execution_failed status as the outcome reason")
	assert.True(t, agent.IsProviderFailureReason(finalReport.OutcomeReason),
		"outcome_reason must be a recognized provider-failure taxonomy value")

	// operator-attention: Detail must provide actionable remediation, not a vague
	// "underspecified worker failure" that gives the operator nothing to act on.
	assert.NotContains(t, finalReport.Detail, "under-specified",
		"operator-attention must not contain generic underspecified-routing message")
	assert.NotContains(t, finalReport.Detail, "worker failure",
		"operator-attention must not fall back to generic worker-failure message")
	assert.Contains(t, finalReport.Detail, "hard pin exhausted",
		"operator-attention must describe the actual cause of the stop")

	// Provider failure evidence must be present so work status JSON can embed the
	// full typed classification for downstream operator tools.
	require.NotNil(t, finalReport.ProviderFailureEvidence,
		"work status evidence must include ProviderFailureEvidence")
	assert.Equal(t, agent.FailureModeProviderHarnessUnavailable, finalReport.ProviderFailureEvidence.TypedFailure,
		"evidence typed_failure must match the report OutcomeReason")
	assert.NotEmpty(t, finalReport.ProviderFailureEvidence.FallbackStopReason,
		"evidence must record why fallback stopped so operator can diagnose without re-running")
}
