// Package coordination defines the transport-neutral worker coordination
// domain contract (ADR-022 rev 6, FEAT-006 Worker Contract).
//
// Connected and offline workers share the same request/outcome types. This
// package owns claim, tracker-transition, and landing semantics on one
// Coordinator surface.
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
	// ReasonLandPreserved is returned when a land attempt completes as
	// preserved (e.g. merge conflict) rather than advancing the target tip.
	// Preserved is still OutcomeApplied for the first observation of a key;
	// the reason distinguishes it from a clean land.
	ReasonLandPreserved = "land_preserved"
)

// Land status values returned on LandResult.Status. Mirror agent.LandResult
// status strings so online and offline paths share vocabulary.
const (
	LandStatusLanded    = "landed"
	LandStatusPreserved = "preserved"
	LandStatusNoChanges = "no-changes"
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

// LandRequest is a transport-neutral landing mutation (ADR-022 land operation).
// Payload fields mirror the production agent.LandRequest inputs needed to
// invoke the real landing path; IdempotencyKey is the coordination contract
// addition for replay safety.
type LandRequest struct {
	// ProjectRoot is the project repository root used for locks and land
	// scratch (required).
	ProjectRoot string
	// WorktreeDir is the git working directory for land operations. When
	// empty, production adapters default to ProjectRoot.
	WorktreeDir string
	// BaseRev is the revision the worker branched from (required for a
	// non-trivial land).
	BaseRev string
	// ResultRev is the worker's final commit SHA (required for a non-trivial
	// land).
	ResultRev string
	// BeadID identifies the bead being landed (required).
	BeadID string
	// AttemptID uniquely identifies the land attempt for preserve-ref paths.
	AttemptID string
	// TargetBranch is the branch to advance. Empty resolves to HEAD branch.
	TargetBranch string
	// EvidenceDir is optional per-attempt evidence relative to ProjectRoot.
	EvidenceDir string
	// IdempotencyKey uniquely identifies this land attempt for replay safety
	// (required; ADR-022).
	IdempotencyKey string
}

// LandResult is the contract-defined landing outcome.
type LandResult struct {
	// Code is applied, already_applied, or conflict.
	Code OutcomeCode
	// BeadID echoes the requested bead.
	BeadID string
	// Status is landed, preserved, or no-changes (LandStatus* constants).
	Status string
	// NewTip is the new target tip when Status == landed.
	NewTip string
	// TargetBranch is the resolved branch advanced or attempted.
	TargetBranch string
	// Merged is true when the land took the merge-commit path.
	Merged bool
	// PreserveRef is set when Status == preserved.
	PreserveRef string
	// Reason is a human-readable or stable reason (e.g. merge conflict,
	// ReasonLandPreserved).
	Reason string
	// IdempotencyKey echoes the request key for journal/reconcile callers.
	IdempotencyKey string
}

// Coordinator is the transport-neutral coordination domain surface.
// LocalCoordinator implements it against the bead store and real land path;
// an HTTP client implements the same contract for connected workers (out of
// scope for the landing contract bead).
type Coordinator interface {
	Claim(ctx context.Context, req ClaimRequest) (ClaimResult, error)
	Transition(ctx context.Context, req TransitionRequest) (TransitionResult, error)
	Land(ctx context.Context, req LandRequest) (LandResult, error)
}
