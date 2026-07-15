package executeloop

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTryLoopStateMachineProviderConnectivityRetriesWithExactHigherFloor(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "provider request failed: dial tcp 100.70.199.113:1235: connect: connection refused",
		CurrentMinPower:          0,
		ActualPower:              5,
		AllowInfrastructureRetry: true,
	})

	assert.Equal(t, TryLoopStateRetryPower, transition.State)
	assert.Equal(t, TryLoopActionRetryPower, transition.Action)
	assert.Equal(t, 6, transition.NextMinPower)
	assert.Equal(t, "infrastructure_retry_with_higher_min_power", transition.Reason)
}

func TestTryLoopStateMachinePinnedProviderConnectivityStops(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "provider request failed: dial tcp 100.70.199.113:1235: connect: connection refused",
		CurrentMinPower:          0,
		ActualPower:              5,
		AllowInfrastructureRetry: false,
	})

	assert.Equal(t, TryLoopActionStop, transition.Action)
	assert.Equal(t, "infrastructure_no_retry_route", transition.Reason)
}

func TestTryLoopStateMachineSemanticFailureRaisesAbstractFloor(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "build failed",
		CurrentMinPower:          0,
		ActualPower:              5,
		AllowInfrastructureRetry: true,
	})

	assert.Equal(t, TryLoopActionRetryPower, transition.Action)
	assert.Equal(t, 6, transition.NextMinPower)
	assert.Equal(t, "semantic_retry_with_higher_min_power", transition.Reason)
}

func TestTryLoopStateMachineSemanticFailureRaisesAboveCurrentAndActual(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:          "execution_failed",
		Detail:          "build failed",
		CurrentMinPower: 8,
		ActualPower:     5,
	})

	assert.Equal(t, TryLoopActionRetryPower, transition.Action)
	assert.Equal(t, 9, transition.NextMinPower)
}

func TestTryLoopStateMachineStopsWhenNextFloorReachesMaxPower(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:          "execution_failed",
		Detail:          "build failed",
		CurrentMinPower: 10,
		ActualPower:     10,
		MaxPower:        11,
	})

	assert.Equal(t, TryLoopActionStop, transition.Action)
	assert.Equal(t, "max_power_exhausted", transition.Reason)
	assert.Zero(t, transition.NextMinPower)
}

func TestTryLoopStateMachineRetriesWhenNextFloorRemainsBelowMaxPower(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:          "execution_failed",
		Detail:          "build failed",
		CurrentMinPower: 9,
		ActualPower:     9,
		MaxPower:        11,
	})

	assert.Equal(t, TryLoopActionRetryPower, transition.Action)
	assert.Equal(t, 10, transition.NextMinPower)
}

func TestTryLoopStateMachineNoRouteEvidenceStopsInfrastructure(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "ResolveRoute: no viable routing candidate",
		CurrentMinPower:          0,
		AllowInfrastructureRetry: true,
	})

	assert.Equal(t, TryLoopActionStop, transition.Action)
	assert.Equal(t, "infrastructure_no_retry_route", transition.Reason)
}

func TestTryLoopStateMachineQuotaInfrastructureStops(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "429 rate limit exceeded",
		CurrentMinPower:          0,
		ActualPower:              5,
		AllowInfrastructureRetry: true,
	})

	assert.Equal(t, TryLoopActionStop, transition.Action)
	assert.Equal(t, "infrastructure_no_retry_route", transition.Reason)
}
