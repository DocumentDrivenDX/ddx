package executeloop

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testFloorFinder []int

func (f testFloorFinder) Next(actualPower int) (int, error) {
	for _, floor := range f {
		if floor > actualPower {
			return floor, nil
		}
	}
	return 0, errors.New("exhausted")
}

func TestTryLoopStateMachineProviderConnectivityRetriesWithExactHigherFloor(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "provider request failed: dial tcp 100.70.199.113:1235: connect: connection refused",
		CurrentMinPower:          0,
		ActualPower:              5,
		AllowInfrastructureRetry: true,
	}, testFloorFinder{5, 7, 9})

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
	}, testFloorFinder{5, 7, 9})

	assert.Equal(t, TryLoopActionStop, transition.Action)
	assert.Equal(t, "infrastructure_no_retry_route", transition.Reason)
}

func TestTryLoopStateMachineSemanticFailureUsesLadder(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "build failed",
		CurrentMinPower:          0,
		ActualPower:              5,
		AllowInfrastructureRetry: true,
	}, testFloorFinder{5, 7, 9})

	assert.Equal(t, TryLoopActionRetryPower, transition.Action)
	assert.Equal(t, 7, transition.NextMinPower)
	assert.Equal(t, "semantic_retry_with_higher_min_power", transition.Reason)
}

func TestTryLoopStateMachineNoRouteEvidenceStopsInfrastructure(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "ResolveRoute: no viable routing candidate",
		CurrentMinPower:          0,
		AllowInfrastructureRetry: true,
	}, testFloorFinder{5, 7, 9})

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
	}, testFloorFinder{5, 7, 9})

	assert.Equal(t, TryLoopActionStop, transition.Action)
	assert.Equal(t, "infrastructure_no_retry_route", transition.Reason)
}
