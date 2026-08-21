package executeloop

import (
	"reflect"
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
		OutcomeReason:            "no_viable_provider",
		AllowInfrastructureRetry: true,
	})

	assert.Equal(t, TryLoopActionStop, transition.Action)
	assert.Equal(t, "non_semantic_outcome_reason", transition.Reason)
}

func TestTryLoopStateMachineQuotaInfrastructureStops(t *testing.T) {
	transition := DecideAttemptTransition(AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "429 rate limit exceeded",
		CurrentMinPower:          0,
		ActualPower:              5,
		OutcomeReason:            "provider_rate_limit",
		AllowInfrastructureRetry: true,
	})

	assert.Equal(t, TryLoopActionRetryPower, transition.Action)
	assert.Equal(t, "infrastructure_retry_with_higher_min_power", transition.Reason)
}

func TestAttemptPolicyEscalatesOnlyMinimumPower(t *testing.T) {
	input := AttemptTransitionInput{
		Status:                   "execution_failed",
		Detail:                   "build failed",
		CurrentMinPower:          5,
		ActualPower:              8,
		AllowInfrastructureRetry: true,
	}

	transition := DecideAttemptTransition(input)

	assert.Equal(t, TryLoopActionRetryPower, transition.Action)
	assert.Equal(t, 9, transition.NextMinPower)
	assert.Equal(t, "semantic_retry_with_higher_min_power", transition.Reason)

	// The attempt-policy surface is intentionally route-neutral: it can only
	// move the abstract minimum-power floor. It has no fields for harness,
	// provider, model, profile, or other concrete route pins to populate.
	transitionType := reflect.TypeOf(transition)
	for _, fieldName := range []string{
		"Harness",
		"Provider",
		"Model",
		"Profile",
		"RequestedProfile",
		"RequestedMinPower",
		"RequestedMaxPower",
		"ProviderPin",
	} {
		if _, ok := transitionType.FieldByName(fieldName); ok {
			t.Fatalf("attempt transition unexpectedly exposes route field %q", fieldName)
		}
	}

	inputType := reflect.TypeOf(input)
	for _, fieldName := range []string{
		"Harness",
		"Provider",
		"Model",
		"Profile",
		"RequestedProfile",
		"ProviderPin",
	} {
		if _, ok := inputType.FieldByName(fieldName); ok {
			t.Fatalf("attempt input unexpectedly exposes route field %q", fieldName)
		}
	}
}

func TestAttemptPolicyDrivesRetryCooldownParkAndNoViableProvider(t *testing.T) {
	cases := []struct {
		name       string
		input      AttemptTransitionInput
		wantAction TryLoopAction
		wantReason string
	}{
		{
			name: "semantic failure retries",
			input: AttemptTransitionInput{
				Status:          "execution_failed",
				Detail:          "build failed",
				CurrentMinPower: 5,
				ActualPower:     8,
			},
			wantAction: TryLoopActionRetryPower,
			wantReason: "semantic_retry_with_higher_min_power",
		},
		{
			name: "provider connectivity retries",
			input: AttemptTransitionInput{
				Status:                   "execution_failed",
				Detail:                   "provider request failed: dial tcp 100.70.199.113:1235: connect: connection refused",
				CurrentMinPower:          5,
				ActualPower:              8,
				OutcomeReason:            "provider_connectivity",
				AllowInfrastructureRetry: true,
			},
			wantAction: TryLoopActionRetryPower,
			wantReason: "infrastructure_retry_with_higher_min_power",
		},
		{
			name: "no viable provider stops",
			input: AttemptTransitionInput{
				Status:          "execution_failed",
				Detail:          "ResolveRoute: no viable routing candidate",
				CurrentMinPower: 5,
				ActualPower:     8,
				OutcomeReason:   "no_viable_provider",
			},
			wantAction: TryLoopActionStop,
			wantReason: "non_semantic_outcome_reason",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transition := DecideAttemptTransition(tc.input)
			assert.Equal(t, tc.wantAction, transition.Action)
			assert.Equal(t, tc.wantReason, transition.Reason)
		})
	}
}
