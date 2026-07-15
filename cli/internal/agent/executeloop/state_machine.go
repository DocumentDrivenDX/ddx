package executeloop

import (
	"errors"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

// TryLoopState names the in-memory states used while ddx try/work evaluates a
// single bead attempt. Persisted bead lifecycle remains the six statuses owned
// by TD-027; these states are control-flow states only.
type TryLoopState string

const (
	TryLoopStateClassifyResult TryLoopState = "classify_result"
	TryLoopStateRetryPower     TryLoopState = "retry_power"
	TryLoopStateStopAttempt    TryLoopState = "stop_attempt"
)

// TryLoopAction is the action selected by the attempt state machine.
type TryLoopAction string

const (
	TryLoopActionStop       TryLoopAction = "stop"
	TryLoopActionRetryPower TryLoopAction = "retry_power"
)

var errMinPowerExhausted = errors.New("minimum power cannot be raised")

// AttemptTransitionInput is the complete input the try-loop state machine needs
// to decide whether another implementation attempt is useful.
type AttemptTransitionInput struct {
	Status          string
	Detail          string
	CurrentMinPower int
	MaxPower        int
	ActualPower     int
	// OutcomeReason carries the worker's typed failure classification. When
	// populated, only semantic acceptance/review reasons are allowed to
	// raise the abstract power floor; structural and infrastructure reasons stop
	// without escalating MinPower.
	OutcomeReason string

	Disrupted                bool
	BudgetExhausted          bool
	AllowInfrastructureRetry bool
}

// AttemptTransition is the state-machine decision for one completed attempt.
type AttemptTransition struct {
	State        TryLoopState
	Action       TryLoopAction
	NextMinPower int
	Reason       string
}

// DecideAttemptTransition implements the ddx try retry state machine for one
// attempt result. The default bias is forward progress: semantic failures move
// raise the abstract MinPower floor, while retryable provider-connectivity failures can
// be retried immediately with MinPower set above both the current request and
// the failed route's actual power. Infrastructure failures without concrete
// route evidence still stop the current attempt; a stronger agent cannot fix
// an absent service with no alternate power signal.
func DecideAttemptTransition(input AttemptTransitionInput) AttemptTransition {
	retryableProvider := isRetryableProviderReason(input.OutcomeReason)
	if input.Disrupted && !retryableProvider && !isProviderConnectivityDetail(input.Detail) {
		return stopTransition("disrupted")
	}
	if input.BudgetExhausted {
		return stopTransition("budget_exhausted")
	}
	if strings.TrimSpace(input.Status) == "land_conflict" {
		return stopTransition("land_conflict")
	}
	if reason := strings.TrimSpace(input.OutcomeReason); reason != "" && !isSemanticRetryOutcomeReason(reason) && !retryableProvider {
		return stopTransition("non_semantic_outcome_reason")
	}
	if !escalation.ShouldEscalate(input.Status) {
		return stopTransition("terminal_status")
	}

	if escalation.IsInfrastructureFailure(input.Status, input.Detail) {
		if !input.AllowInfrastructureRetry || input.ActualPower <= 0 || (!isProviderConnectivityDetail(input.Detail) && !retryableProvider) {
			return stopTransition("infrastructure_no_retry_route")
		}
		next, err := NextAbstractMinPower(input.CurrentMinPower, input.ActualPower)
		if err != nil {
			return stopTransition("min_power_exhausted")
		}
		if input.MaxPower > 0 && next >= input.MaxPower {
			return stopTransition("max_power_exhausted")
		}
		return AttemptTransition{
			State:        TryLoopStateRetryPower,
			Action:       TryLoopActionRetryPower,
			NextMinPower: next,
			Reason:       "infrastructure_retry_with_higher_min_power",
		}
	}

	next, err := NextAbstractMinPower(input.CurrentMinPower, input.ActualPower)
	if err != nil {
		return stopTransition("min_power_exhausted")
	}
	if input.MaxPower > 0 && next >= input.MaxPower {
		return stopTransition("max_power_exhausted")
	}
	return AttemptTransition{
		State:        TryLoopStateRetryPower,
		Action:       TryLoopActionRetryPower,
		NextMinPower: next,
		Reason:       "semantic_retry_with_higher_min_power",
	}
}

// NextAbstractMinPower returns the smallest integer floor strictly above both
// the current request and the power Fizeau reported for the completed attempt.
// DDx deliberately does not inspect model inventory or route viability here;
// Fizeau owns deciding whether and how to satisfy the stronger abstract floor.
func NextAbstractMinPower(currentMinPower, actualPower int) (int, error) {
	basis := currentMinPower
	if actualPower > basis {
		basis = actualPower
	}
	if basis == int(^uint(0)>>1) {
		return 0, errMinPowerExhausted
	}
	return basis + 1, nil
}

// isRetryableProviderReason reports whether reason is a typed provider-failure
// taxonomy value (ddx-3b721804) that an unpinned worker may fall back from by
// retrying another eligible route / higher floor. Mirrors the agent package's
// retryable subset; literals are used here (as with "provider_connectivity"
// above) so executeloop does not import agent. Non-retryable provider failures
// (provider_config_invalid, no_viable_provider, unknown_provider_failure) are
// intentionally excluded so they stop rather than spin.
func isRetryableProviderReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "provider_connectivity",
		"provider_auth",
		"provider_rate_limit",
		"provider_quota",
		"provider_model_unavailable",
		"provider_harness_unavailable":
		return true
	default:
		return false
	}
}

func isSemanticRetryOutcomeReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "tests_red",
		"test_failure",
		"build_failure",
		"review_block",
		"review_request_changes",
		"post_run_check_failed":
		return true
	default:
		return false
	}
}

func stopTransition(reason string) AttemptTransition {
	return AttemptTransition{
		State:  TryLoopStateStopAttempt,
		Action: TryLoopActionStop,
		Reason: reason,
	}
}

func isProviderConnectivityDetail(detail string) bool {
	lower := strings.ToLower(detail)
	for _, marker := range []string{"provider_connectivity", "connection refused", "no route to host", "network is unreachable", "i/o timeout", "connection reset", "dial tcp"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
