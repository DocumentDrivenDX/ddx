package agent

import (
	"fmt"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// Retry policy logical outcome names. These are the policy's semantic
// vocabulary, distinct from execution status and failure-mode constants.
const (
	// RetryOutcomeSuccess marks a successful merge; no retry needed.
	RetryOutcomeSuccess = "success"

	// RetryOutcomeNoChangesAfterAttempt marks an attempt where the agent
	// produced no commits. Retryable with a higher MinPower.
	RetryOutcomeNoChangesAfterAttempt = "no_changes_after_attempt"

	// RetryOutcomeReviewBlockedCapability marks an attempt where the
	// post-merge reviewer returned BLOCK or REQUEST_CHANGES, indicating the
	// model's output did not meet the acceptance criteria. Retryable with a
	// higher MinPower.
	RetryOutcomeReviewBlockedCapability = "review_blocked_capability"

	// RetryOutcomePassthroughExhausted marks an attempt where the passthrough
	// envelope conflicts with the power bounds (blocked_by_passthrough_constraint
	// or agent_power_unsatisfied). Power escalation cannot resolve this;
	// the policy stops instead of mutating the passthrough.
	RetryOutcomePassthroughExhausted = "passthrough_exhausted"

	// RetryOutcomeSetupFailure marks a deterministic infrastructure failure that
	// power escalation cannot fix: harness not installed, no viable provider,
	// auth error, etc. The policy stops.
	RetryOutcomeSetupFailure = "deterministic_setup_failure"

	// RetryOutcomeMaxPowerNoProgress marks an attempt where MinPower cannot be
	// raised any further — either because the next threshold would exceed
	// MaxPower, or because the catalog has no higher power tier available.
	// The policy stops.
	RetryOutcomeMaxPowerNoProgress = "max_power_no_progress"

	// RetryOutcomeOther marks any outcome not eligible for power escalation.
	RetryOutcomeOther = "other"
)

// RetryDecision is the policy output for one bead attempt outcome.
// It never includes DDx-selected harness, provider, model, or model-ref pins;
// only a MinPower adjustment is returned.
type RetryDecision struct {
	// ShouldRetry indicates whether the bead should be retried with escalated
	// MinPower. false means the policy decided to stop.
	ShouldRetry bool

	// NextMinPower is the MinPower to use for the next attempt.
	// Only meaningful when ShouldRetry is true.
	NextMinPower int

	// Reason classifies why the policy decided to retry or stop.
	// Populated for both outcomes and used in audit records.
	Reason string

	// Evidence provides additional context for the audit record.
	Evidence string
}

// ClassifyRetryOutcome maps an attempt's status and failure_mode into a
// logical retry outcome that EvaluateRetryPolicy uses to make its decision.
//
// Passthrough exhaustion (blocked_by_passthrough_constraint or
// agent_power_unsatisfied) is detected first and always results in a stop,
// regardless of the status string. Deterministic setup failures (harness not
// installed, no viable provider, auth error) are detected next. All other
// classification is done from status alone.
func ClassifyRetryOutcome(status, failureMode string) string {
	// Passthrough exhaustion stops regardless of other signals; inspecting
	// the failure mode is sufficient and does not require reading string values
	// from the passthrough envelope itself.
	switch failureMode {
	case FailureModeBlockedByPassthroughConstraint, FailureModeAgentPowerUnsatisfied:
		return RetryOutcomePassthroughExhausted
	}

	// Deterministic setup failures that power escalation cannot fix.
	switch failureMode {
	case FailureModeHarnessNotInstalled, FailureModeNoViableProvider, FailureModeAuthError:
		return RetryOutcomeSetupFailure
	}

	switch status {
	case ExecuteBeadStatusSuccess, ExecuteBeadStatusAlreadySatisfied:
		return RetryOutcomeSuccess
	case ExecuteBeadStatusNoChanges:
		return RetryOutcomeNoChangesAfterAttempt
	case ExecuteBeadStatusReviewBlock, ExecuteBeadStatusReviewRequestChanges,
		ExecuteBeadStatusReviewMalfunction:
		return RetryOutcomeReviewBlockedCapability
	}

	return RetryOutcomeOther
}

// EvaluateRetryPolicy makes the retry/stop decision for one bead attempt.
//
// outcome must come from ClassifyRetryOutcome. actualPower is the power level
// used in the prior attempt (from routing evidence's actual_power field).
// currentMinPower and maxPower are the bounds currently in effect for the bead.
// models is the agent model catalog; when non-empty, top-1 (the strongest tier)
// is used as the escalation target.
//
// The returned RetryDecision never includes DDx-selected harness, provider,
// model, or model-ref pins — only a new MinPower bound. The passthrough
// envelope is neither read nor mutated.
func EvaluateRetryPolicy(
	outcome string,
	actualPower int,
	currentMinPower int,
	maxPower int,
	models []agentlib.ModelInfo,
) RetryDecision {
	switch outcome {
	case RetryOutcomeSuccess:
		return RetryDecision{
			ShouldRetry: false,
			Reason:      RetryOutcomeSuccess,
			Evidence:    "attempt succeeded; no retry needed",
		}

	case RetryOutcomePassthroughExhausted:
		return RetryDecision{
			ShouldRetry: false,
			Reason:      RetryOutcomePassthroughExhausted,
			Evidence:    "passthrough constraint or agent power bounds incompatible; retry would not resolve",
		}

	case RetryOutcomeSetupFailure:
		return RetryDecision{
			ShouldRetry: false,
			Reason:      RetryOutcomeSetupFailure,
			Evidence:    "deterministic setup failure; power escalation cannot fix",
		}

	case RetryOutcomeNoChangesAfterAttempt, RetryOutcomeReviewBlockedCapability:
		nextMinPower := computeNextMinPower(models, actualPower, currentMinPower)

		// MaxPower must be preserved across retries. If escalation would
		// produce a NextMinPower that violates the MaxPower bound, stop.
		if maxPower > 0 && nextMinPower > maxPower {
			return RetryDecision{
				ShouldRetry: false,
				Reason:      RetryOutcomeMaxPowerNoProgress,
				Evidence: fmt.Sprintf(
					"next_min_power=%d would exceed max_power=%d; cannot escalate within bounds",
					nextMinPower, maxPower),
			}
		}

		// If the computed next threshold is not higher than the power level
		// the prior attempt already used, escalating to it would not constrain
		// routing to a stronger model — the catalog has no higher tier.
		if actualPower > 0 && nextMinPower <= actualPower {
			return RetryDecision{
				ShouldRetry: false,
				Reason:      RetryOutcomeMaxPowerNoProgress,
				Evidence: fmt.Sprintf(
					"actual_power=%d; no higher power tier available in catalog",
					actualPower),
			}
		}

		return RetryDecision{
			ShouldRetry:  true,
			NextMinPower: nextMinPower,
			Reason:       outcome,
			Evidence: fmt.Sprintf(
				"actual_power=%d next_min_power=%d",
				actualPower, nextMinPower),
		}

	default:
		return RetryDecision{
			ShouldRetry: false,
			Reason:      RetryOutcomeOther,
			Evidence:    "outcome not eligible for power escalation: " + outcome,
		}
	}
}

// computeNextMinPower returns the MinPower for the next escalated attempt.
// When the models catalog is non-empty, returns TopNPowerThreshold(models, 1)
// — the minimum power required to reach only the strongest model tier.
// When the catalog is empty, bumps above the highest observed power by a
// fixed delta.
func computeNextMinPower(models []agentlib.ModelInfo, actualPower, currentMinPower int) int {
	top1 := TopNPowerThreshold(models, 1)
	if top1 > 0 {
		return top1
	}
	// No catalog: heuristic bump above the highest observed power.
	base := currentMinPower
	if actualPower > base {
		base = actualPower
	}
	if base == 0 {
		return 50
	}
	return base + 10
}
