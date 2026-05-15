package agent

import (
	"context"
	"strings"
)

// PreClaimIntakeOutcome classifies the pre-claim intake decision for a bead.
// Only actionable_atomic proceeds directly to Claim; intake_error fail-opens.
type PreClaimIntakeOutcome string

const (
	PreClaimIntakeActionableAtomic       PreClaimIntakeOutcome = "actionable_atomic"
	PreClaimIntakeActionableButRewritten PreClaimIntakeOutcome = "actionable_but_rewritten"
	PreClaimIntakeTooLargeDecomposed     PreClaimIntakeOutcome = "too_large_decomposed"
	PreClaimIntakeOperatorRequired       PreClaimIntakeOutcome = "operator_required"
	PreClaimIntakeError                  PreClaimIntakeOutcome = "intake_error"
)

// PreClaimIntakeResult is the typed decision returned by the intake hook.
// Detail carries the human-readable rationale or infra failure context.
type PreClaimIntakeResult struct {
	Outcome      PreClaimIntakeOutcome `json:"outcome,omitempty"`
	Reason       string                `json:"reason,omitempty"`
	SystemReason string                `json:"system_reason,omitempty"`
	Detail       string                `json:"detail,omitempty"`
	// EstimatedDifficulty is the readiness model's transient easy/medium/hard
	// assessment. It is not persisted to bead metadata.
	EstimatedDifficulty string                `json:"estimated_difficulty,omitempty"`
	Rewrite             PreClaimIntakeRewrite `json:"rewrite,omitempty"`
	// Decomposition is populated when Outcome == too_large_decomposed and the
	// hook produced a concrete split with children and AC map.
	Decomposition *PreClaimDecomposition `json:"decomposition,omitempty"`
}

// PreClaimDecompositionChild is one proposed child bead in a splitter result.
type PreClaimDecompositionChild struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Acceptance  string   `json:"acceptance"`
	Labels      []string `json:"labels,omitempty"`
}

// ACMapEntry maps one parent acceptance criterion to child coverage.
// Coverage must be non-empty; use "operator_required" or "non_scope" for
// ACs that cannot be expressed as a child bead.
type ACMapEntry struct {
	ParentAC string `json:"parent_ac"` // text of the parent AC
	Coverage string `json:"coverage"`  // child AC text | "operator_required" | "non_scope"
}

// PreClaimDecomposition is the splitter output when outcome is too_large_decomposed.
// Every parent AC must have a corresponding ACMap entry with non-empty Coverage;
// a split with missing or empty Coverage entries is lossy and blocks for operator.
type PreClaimDecomposition struct {
	Children  []PreClaimDecompositionChild `json:"children"`
	ACMap     []ACMapEntry                 `json:"ac_map"`
	Rationale string                       `json:"rationale"`
}

// isDecompositionLossy returns true if any ACMapEntry has empty Coverage,
// indicating the split does not fully cover all parent acceptance criteria.
func isDecompositionLossy(acMap []ACMapEntry) bool {
	for _, entry := range acMap {
		if strings.TrimSpace(entry.Coverage) == "" {
			return true
		}
	}
	return false
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
		PreClaimIntakeOperatorRequired, PreClaimIntakeError:
		return r.Outcome
	default:
		return PreClaimIntakeError
	}
}
