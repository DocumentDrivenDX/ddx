package agent

import "context"

// PreClaimIntakeOutcome classifies the pre-claim intake decision for a bead.
// Only actionable_atomic proceeds directly to Claim; intake_error fail-opens.
type PreClaimIntakeOutcome string

const (
	PreClaimIntakeActionableAtomic       PreClaimIntakeOutcome = "actionable_atomic"
	PreClaimIntakeActionableButRewritten PreClaimIntakeOutcome = "actionable_but_rewritten"
	PreClaimIntakeTooLargeDecomposed     PreClaimIntakeOutcome = "too_large_decomposed"
	PreClaimIntakeAmbiguousNeedsHuman    PreClaimIntakeOutcome = "ambiguous_needs_human"
	PreClaimIntakeError                  PreClaimIntakeOutcome = "intake_error"
)

// PreClaimIntakeResult is the typed decision returned by the intake hook.
// Detail carries the human-readable rationale or infra failure context.
type PreClaimIntakeResult struct {
	Outcome      PreClaimIntakeOutcome `json:"outcome,omitempty"`
	Reason       string                `json:"reason,omitempty"`
	SystemReason string                `json:"system_reason,omitempty"`
	Detail       string                `json:"detail,omitempty"`
	Rewrite      PreClaimIntakeRewrite `json:"rewrite,omitempty"`
}

// PreClaimIntakeRewrite carries intent-preserving bead edits returned by the
// intake hook when a bead is actionable but needs safe refinement before
// claim.
type PreClaimIntakeRewrite struct {
	Description   string   `json:"description,omitempty"`
	Acceptance    string   `json:"acceptance,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// PreClaimIntakeHook classifies a ready bead before Claim. The loop treats
// actionable_atomic as pass-through, skips Claim for non-atomic outcomes, and
// fail-opens when the hook returns an infrastructure error.
type PreClaimIntakeHook func(ctx context.Context, beadID string) (PreClaimIntakeResult, error)

func (r PreClaimIntakeResult) normalizedOutcome() PreClaimIntakeOutcome {
	switch r.Outcome {
	case "", PreClaimIntakeActionableAtomic:
		return PreClaimIntakeActionableAtomic
	case PreClaimIntakeActionableButRewritten, PreClaimIntakeTooLargeDecomposed,
		PreClaimIntakeAmbiguousNeedsHuman, PreClaimIntakeError:
		return r.Outcome
	default:
		return PreClaimIntakeError
	}
}
