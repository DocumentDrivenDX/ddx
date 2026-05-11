package agent

import (
	"context"
	"strings"
)

// RecoveryFailureClass categorises the persistent failure mode that exhausted
// the escalation ladder, so the PostLadderExhaustionHook can choose the
// appropriate recovery action.
type RecoveryFailureClass string

const (
	// SpecGap is set when the last attempt was blocked by a spec-gap or
	// missing-acceptance review verdict.
	SpecGap RecoveryFailureClass = "spec_gap"
	// TooLarge is set when the last attempt was blocked by a too-large review
	// verdict.
	TooLarge RecoveryFailureClass = "too_large"
	// PersistentExecutionFailed is the default class for all other persistent
	// failures.
	PersistentExecutionFailed RecoveryFailureClass = "persistent_execution_failed"
)

// RecoveryPath indicates which automated recovery action a
// PostLadderExhaustionHook took.
type RecoveryPath string

const (
	Reframe   RecoveryPath = "reframe"
	Decompose RecoveryPath = "decompose"
)

// PostLadderExhaustionResult is the outcome returned by a
// PostLadderExhaustionHook invocation.
type PostLadderExhaustionResult struct {
	Attempted bool
	Succeeded bool
	Path      RecoveryPath
	CostUSD   float64
}

// PostLadderExhaustionHook is called when the consecutive_ladder_exhaustions
// counter for a bead reaches the auto-recovery threshold (>= 2). It should
// attempt automated recovery and return the outcome. A nil hook or a result
// with Attempted=false causes the caller to fall through to the existing loop
// path unchanged.
type PostLadderExhaustionHook func(ctx context.Context, beadID string, failureClass RecoveryFailureClass) (*PostLadderExhaustionResult, error)

// deriveRecoveryFailureClass maps the last-attempt report to a
// RecoveryFailureClass for use by the PostLadderExhaustionHook.
func deriveRecoveryFailureClass(report ExecuteBeadReport) RecoveryFailureClass {
	switch {
	case strings.Contains(report.Status, ReviewTerminalClassSpecGap),
		strings.Contains(report.Status, ReviewTerminalClassMissingAcceptance):
		return SpecGap
	case strings.Contains(report.Status, ReviewTerminalClassTooLarge):
		return TooLarge
	default:
		return PersistentExecutionFailed
	}
}
