// Package triage provides the post-attempt decision module that the
// drain loop consults after each terminal event. Given a bead's failure
// mode and the prior modes seen in this bead's attempt history, the
// policy returns the next action to take (re-attempt, escalate, file
// follow-up, etc.).
//
// This package is the standalone policy core; loop integration lands in
// a follow-up bead.
package triage

import "fmt"

// FailureMode classifies the terminal event of a single execute-bead attempt.
type FailureMode string

const (
	// FailureModeReviewBlock — reviewer returned BLOCK on the committed result.
	FailureModeReviewBlock FailureMode = "review_block"
	// FailureModeLockContention — claim/lock could not be acquired.
	FailureModeLockContention FailureMode = "lock_contention"
	// FailureModeExecutionFailed — runner crashed, harness errored, or
	// timed out without producing a usable commit.
	FailureModeExecutionFailed FailureMode = "execution_failed"
	// FailureModeNoChanges — attempt completed without producing a commit.
	FailureModeNoChanges FailureMode = "no_changes"
)

// Action is the decision the policy returns for an attempt.
type Action string

const (
	// ActionReAttemptWithContext — re-queue the bead with the prior
	// failure context threaded into the next prompt.
	ActionReAttemptWithContext Action = "re_attempt_with_context"
	// ActionEscalateTier — re-queue with a higher-capability model tier.
	ActionEscalateTier Action = "escalate_tier"
	// ActionRetryWithBackoff — retry after a delay (lock contention etc.).
	ActionRetryWithBackoff Action = "retry_with_backoff"
	// ActionFileFollowup — file a follow-up bead and close this one.
	ActionFileFollowup Action = "file_followup"
	// ActionNeedsHuman — park the bead for manual triage.
	ActionNeedsHuman Action = "needs_human"
)

// TriagePolicy maps a FailureMode to an ordered ladder of Actions. The
// nth time a given mode is seen in a bead's history, the nth rung of
// its ladder is returned. Past the end of the ladder, the final rung
// (typically ActionNeedsHuman) is returned.
type TriagePolicy struct {
	Ladders map[FailureMode][]Action
}

// DefaultPolicy returns the built-in policy. Mirrors the parent bead's
// AC#3: BLOCK → re_attempt_with_context, escalate_tier, needs_human.
func DefaultPolicy() TriagePolicy {
	return TriagePolicy{Ladders: map[FailureMode][]Action{
		FailureModeReviewBlock: {
			ActionReAttemptWithContext,
			ActionEscalateTier,
			ActionNeedsHuman,
		},
		FailureModeLockContention: {
			ActionRetryWithBackoff,
			ActionRetryWithBackoff,
			ActionNeedsHuman,
		},
		FailureModeExecutionFailed: {
			ActionReAttemptWithContext,
			ActionEscalateTier,
			ActionNeedsHuman,
		},
		FailureModeNoChanges: {
			ActionReAttemptWithContext,
			ActionFileFollowup,
			ActionNeedsHuman,
		},
	}}
}

// Decide returns the next Action for an attempt that ended in `mode`,
// given the ordered list of failure modes already seen for this bead
// (most recent last; the current attempt is NOT included).
//
// beadID is accepted for future per-bead policy customisation and to
// keep call sites stable; the default policy ignores it.
func (p TriagePolicy) Decide(beadID string, mode FailureMode, history []FailureMode) Action {
	ladder := p.Ladders[mode]
	if len(ladder) == 0 {
		return ActionNeedsHuman
	}
	n := 0
	for _, m := range history {
		if m == mode {
			n++
		}
	}
	if n >= len(ladder) {
		return ladder[len(ladder)-1]
	}
	return ladder[n]
}

// ParseAction validates and converts a string from config into an Action.
func ParseAction(s string) (Action, error) {
	switch Action(s) {
	case ActionReAttemptWithContext,
		ActionEscalateTier,
		ActionRetryWithBackoff,
		ActionFileFollowup,
		ActionNeedsHuman:
		return Action(s), nil
	default:
		return "", fmt.Errorf("triage: unknown action %q", s)
	}
}

// ParseFailureMode validates and converts a string from config into a FailureMode.
func ParseFailureMode(s string) (FailureMode, error) {
	switch FailureMode(s) {
	case FailureModeReviewBlock,
		FailureModeLockContention,
		FailureModeExecutionFailed,
		FailureModeNoChanges:
		return FailureMode(s), nil
	default:
		return "", fmt.Errorf("triage: unknown failure mode %q", s)
	}
}
