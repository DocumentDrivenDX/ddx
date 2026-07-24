// Package coordination defines the transport-neutral worker coordination
// domain contract (ADR-022 rev 6, FEAT-006 Worker Contract).
//
// Connected and offline workers share the same request/outcome types. This
// package owns claim and tracker-transition semantics; landing contracts
// extend the same Coordinator surface in sibling beads.
//
// Outcomes match the coordination/mutations response vocabulary:
// applied, already_applied, and conflict.
package coordination

import (
	"context"
)

// OutcomeCode is the durable coordination mutation result shared by online
// and offline paths (ADR-022 POST .../coordination/mutations).
type OutcomeCode string

const (
	// OutcomeApplied means the mutation changed durable state.
	OutcomeApplied OutcomeCode = "applied"
	// OutcomeAlreadyApplied means the same idempotency key was already
	// observed and the prior outcome is returned without replaying.
	OutcomeAlreadyApplied OutcomeCode = "already_applied"
	// OutcomeConflict means the mutation is incompatible with current
	// durable state (e.g. claim contention).
	OutcomeConflict OutcomeCode = "conflict"
)

// Conflict reason codes for OutcomeConflict. Stable for journal reconcile
// and operator-attention surfaces.
const (
	// ReasonAlreadyClaimed is returned when a claim loses to an existing
	// fresh claim lease / in_progress owner (claim contention).
	ReasonAlreadyClaimed = "already_claimed"
	// ReasonTransitionRejected is returned when a tracker lifecycle
	// transition is incompatible with current durable state or options.
	ReasonTransitionRejected = "transition_rejected"
)

// ClaimRequest is a transport-neutral claim mutation.
type ClaimRequest struct {
	// BeadID is the bead to claim (required).
	BeadID string
	// Assignee is the claiming worker identity (required).
	Assignee string
	// IdempotencyKey uniquely identifies this claim attempt for replay
	// safety (required; ADR-022).
	IdempotencyKey string
	// Session is optional worker session metadata forwarded to the store.
	Session string
	// Worktree is optional worktree path metadata forwarded to the store.
	Worktree string
}

// ClaimResult is the contract-defined claim outcome.
type ClaimResult struct {
	// Code is applied, already_applied, or conflict.
	Code OutcomeCode
	// BeadID echoes the requested bead.
	BeadID string
	// Owner is the durable claim owner after the attempt when known
	// (winner on applied/conflict; prior owner on already_applied).
	Owner string
	// Reason is set for OutcomeConflict (e.g. ReasonAlreadyClaimed).
	Reason string
	// IdempotencyKey echoes the request key for journal/reconcile callers.
	IdempotencyKey string
}

// TransitionRequest is a transport-neutral tracker lifecycle transition.
// Maps to bead-store TransitionLifecycle / SetLifecycleStatus options.
type TransitionRequest struct {
	// BeadID is the bead to transition (required).
	BeadID string
	// ToStatus is the target lifecycle status (required).
	ToStatus string
	// IdempotencyKey uniquely identifies this transition attempt for replay
	// safety (required; ADR-022).
	IdempotencyKey string
	// OperatorRequired is required for transitions into proposed.
	OperatorRequired bool
	// ExternalBlockerReason is required for transitions into blocked.
	ExternalBlockerReason string
	// ManualClose allows closed from open/in_progress/blocked/proposed.
	ManualClose bool
	// ManualReopen allows closed -> open.
	ManualReopen bool
	// Reason is optional operator/worker reason metadata.
	Reason string
	// Actor is optional actor identity metadata.
	Actor string
	// Source is optional source metadata (e.g. "coordination.local").
	Source string
}

// TransitionResult is the contract-defined tracker-transition outcome.
type TransitionResult struct {
	// Code is applied, already_applied, or conflict.
	Code OutcomeCode
	// BeadID echoes the requested bead.
	BeadID string
	// FromStatus is the durable status observed before the attempt when known.
	FromStatus string
	// ToStatus is the durable status after a successful attempt, or the
	// requested target on conflict/already_applied.
	ToStatus string
	// Reason is set for OutcomeConflict (e.g. ReasonTransitionRejected).
	Reason string
	// IdempotencyKey echoes the request key for journal/reconcile callers.
	IdempotencyKey string
}

// Coordinator is the transport-neutral coordination domain surface.
// LocalCoordinator implements it against the bead store; an HTTP client
// implements the same contract for connected workers (out of scope here).
type Coordinator interface {
	Claim(ctx context.Context, req ClaimRequest) (ClaimResult, error)
	Transition(ctx context.Context, req TransitionRequest) (TransitionResult, error)
}
