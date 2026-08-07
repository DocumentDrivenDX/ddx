package failclass

import (
	"errors"

	agentlib "github.com/easel/fizeau"
)

// AttemptPolicyAction is the single DDx attempt decision selected from a typed
// Fizeau lifecycle result plus DDx-owned stage evidence.
type AttemptPolicyAction string

const (
	AttemptPolicyActionCurrentAttemptRepair      AttemptPolicyAction = "current_attempt_repair"
	AttemptPolicyActionNewAttemptRetry           AttemptPolicyAction = "new_attempt_retry"
	AttemptPolicyActionMinimumStrengthEscalation AttemptPolicyAction = "minimum_strength_escalation_request"
	AttemptPolicyActionPark                      AttemptPolicyAction = "park"
	AttemptPolicyActionLand                      AttemptPolicyAction = "land"
	AttemptPolicyActionClose                     AttemptPolicyAction = "close"
)

// AttemptPolicyEvidence carries DDx-owned evidence that is not part of the
// public Fizeau lifecycle contract.
type AttemptPolicyEvidence struct {
	LandReady                bool
	CurrentAttemptRepairable bool
	NewAttemptRetryAllowed   bool
	RequestMinimumStrength   bool
}

// AttemptPolicyInput is the typed Fizeau lifecycle result plus explicit DDx
// evidence consumed by the adapter.
type AttemptPolicyInput struct {
	Final        *agentlib.ServiceFinalData
	ImmediateErr error
	Evidence     AttemptPolicyEvidence
}

// AttemptPolicyDecision is the one DDx attempt decision selected for the
// typed lifecycle input.
type AttemptPolicyDecision struct {
	Action AttemptPolicyAction
	Reason string
}

const (
	attemptLifecycleCompleted      = "completed"
	attemptLifecycleRetryable      = "retryable"
	attemptLifecycleUnavailableNow = "unavailable_now"
	attemptLifecycleCancelled      = "cancelled"
	attemptLifecyclePermanent      = "permanent_failure"
	attemptLifecycleUnknown        = "unknown"
)

// DecideAttemptPolicy consumes typed Fizeau lifecycle fields and DDx-owned
// stage evidence exactly once, returning one DDx attempt decision without any
// provider-text parsing.
func DecideAttemptPolicy(input AttemptPolicyInput) AttemptPolicyDecision {
	lifecycle, reason := classifyAttemptLifecycle(input.Final, input.ImmediateErr)
	switch lifecycle {
	case attemptLifecycleCompleted:
		if input.Evidence.LandReady {
			return AttemptPolicyDecision{Action: AttemptPolicyActionLand, Reason: reason}
		}
		return AttemptPolicyDecision{Action: AttemptPolicyActionClose, Reason: reason}
	case attemptLifecycleRetryable:
		switch {
		case input.Evidence.CurrentAttemptRepairable:
			return AttemptPolicyDecision{Action: AttemptPolicyActionCurrentAttemptRepair, Reason: reason}
		case input.Evidence.NewAttemptRetryAllowed:
			return AttemptPolicyDecision{Action: AttemptPolicyActionNewAttemptRetry, Reason: reason}
		default:
			return AttemptPolicyDecision{Action: AttemptPolicyActionPark, Reason: reason}
		}
	case attemptLifecycleUnavailableNow:
		return AttemptPolicyDecision{Action: AttemptPolicyActionPark, Reason: reason}
	case attemptLifecycleCancelled:
		return AttemptPolicyDecision{Action: AttemptPolicyActionPark, Reason: reason}
	case attemptLifecyclePermanent:
		if input.Evidence.RequestMinimumStrength {
			return AttemptPolicyDecision{Action: AttemptPolicyActionMinimumStrengthEscalation, Reason: reason}
		}
		return AttemptPolicyDecision{Action: AttemptPolicyActionPark, Reason: reason}
	default:
		return AttemptPolicyDecision{Action: AttemptPolicyActionPark, Reason: reason}
	}
}

func classifyAttemptLifecycle(final *agentlib.ServiceFinalData, immediateErr error) (string, string) {
	if final != nil {
		switch final.Outcome {
		case agentlib.SessionOutcomeSuccess:
			return attemptLifecycleCompleted, "fizeau_outcome_success"
		case agentlib.SessionOutcomeCancelled:
			return attemptLifecycleCancelled, "fizeau_outcome_cancelled"
		}

		switch final.Cause {
		case agentlib.TerminalCauseCompleted:
			return attemptLifecycleCompleted, "fizeau_terminal_completed"
		case agentlib.TerminalCauseRouteUnavailable,
			agentlib.TerminalCauseBudgetHalted,
			agentlib.TerminalCauseContextCapacityExceeded:
			return attemptLifecycleUnavailableNow, "fizeau_terminal_unavailable_now"
		case agentlib.TerminalCauseContextCancelled,
			agentlib.TerminalCauseCallerDied:
			return attemptLifecycleCancelled, "fizeau_terminal_cancelled"
		case agentlib.TerminalCauseProviderFailed,
			agentlib.TerminalCauseHarnessFailed,
			agentlib.TerminalCauseToolLoopFailed,
			agentlib.TerminalCauseDeadlineExceeded:
			return attemptLifecycleRetryable, "fizeau_terminal_retryable"
		case agentlib.TerminalCauseSpawnFailed,
			agentlib.TerminalCauseCleanupFailed,
			agentlib.TerminalCauseInternalError,
			agentlib.TerminalCauseIterationLimit:
			return attemptLifecyclePermanent, "fizeau_terminal_permanent_failure"
		}
	}

	if immediateErr == nil {
		return attemptLifecycleUnknown, "fizeau_lifecycle_unknown"
	}

	var transient *agentlib.NoViableProviderForNow
	if errors.As(immediateErr, &transient) {
		return attemptLifecycleUnavailableNow, "fizeau_immediate_unavailable_now"
	}

	var modelIncompatible agentlib.ErrHarnessModelIncompatible
	if errors.As(immediateErr, &modelIncompatible) {
		return attemptLifecyclePermanent, "fizeau_immediate_model_incompatible"
	}

	var unsatPin agentlib.ErrUnsatisfiablePin
	if errors.As(immediateErr, &unsatPin) {
		return attemptLifecyclePermanent, "fizeau_immediate_unsatisfiable_pin"
	}

	var rejectedOverride *agentlib.ErrRejectedOverride
	if errors.As(immediateErr, &rejectedOverride) {
		return attemptLifecyclePermanent, "fizeau_immediate_rejected_override"
	}

	var noLive agentlib.ErrNoLiveProvider
	if errors.As(immediateErr, &noLive) {
		return attemptLifecycleRetryable, "fizeau_immediate_no_live_provider"
	}

	return attemptLifecyclePermanent, "fizeau_immediate_unclassified"
}
