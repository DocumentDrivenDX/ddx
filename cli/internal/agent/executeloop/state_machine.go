package executeloop

import (
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

// FloorFinder returns the next viable MinPower floor above the supplied power.
type FloorFinder interface {
	Next(actualPower int) (int, error)
}

// AttemptTransitionInput is the complete input the try-loop state machine needs
// to decide whether another implementation attempt is useful.
type AttemptTransitionInput struct {
	Status          string
	Detail          string
	CurrentMinPower int
	ActualPower     int

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
// up the model-power ladder, while retryable provider-connectivity failures can
// be retried immediately with MinPower set just above the failed route's actual
// power. Infrastructure failures without concrete route evidence still stop the
// current attempt; a smarter model cannot fix an absent service with no
// alternate power signal.
func DecideAttemptTransition(input AttemptTransitionInput, ladder FloorFinder) AttemptTransition {
	if input.Disrupted {
		return stopTransition("disrupted")
	}
	if input.BudgetExhausted {
		return stopTransition("budget_exhausted")
	}
	if !escalation.ShouldEscalate(input.Status) {
		return stopTransition("terminal_status")
	}

	if escalation.IsInfrastructureFailure(input.Status, input.Detail) {
		if !input.AllowInfrastructureRetry || input.ActualPower <= 0 || !isProviderConnectivityDetail(input.Detail) {
			return stopTransition("infrastructure_no_retry_route")
		}
		next := input.ActualPower + 1
		if next <= input.CurrentMinPower {
			next = input.CurrentMinPower + 1
		}
		return AttemptTransition{
			State:        TryLoopStateRetryPower,
			Action:       TryLoopActionRetryPower,
			NextMinPower: next,
			Reason:       "infrastructure_retry_with_higher_min_power",
		}
	}

	basis := input.CurrentMinPower
	if input.ActualPower > 0 {
		basis = input.ActualPower
	}
	next, err := nextLadderFloor(ladder, basis)
	if err != nil {
		return stopTransition("power_ladder_exhausted")
	}
	return AttemptTransition{
		State:        TryLoopStateRetryPower,
		Action:       TryLoopActionRetryPower,
		NextMinPower: next,
		Reason:       "semantic_retry_with_higher_min_power",
	}
}

func stopTransition(reason string) AttemptTransition {
	return AttemptTransition{
		State:  TryLoopStateStopAttempt,
		Action: TryLoopActionStop,
		Reason: reason,
	}
}

func nextLadderFloor(ladder FloorFinder, basis int) (int, error) {
	if ladder == nil {
		return 0, errNoFloorFinder{}
	}
	return ladder.Next(basis)
}

type errNoFloorFinder struct{}

func (errNoFloorFinder) Error() string { return "no floor finder configured" }

func isProviderConnectivityDetail(detail string) bool {
	lower := strings.ToLower(detail)
	for _, marker := range []string{"provider_connectivity", "connection refused", "no route to host", "network is unreachable", "i/o timeout", "connection reset", "dial tcp"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
