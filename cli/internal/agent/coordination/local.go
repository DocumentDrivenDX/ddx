package coordination

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// ClaimBackend is the bead-store surface the local claim path requires.
// Production LocalCoordinator must be wired to a real *bead.Store (or another
// Backend that invokes the same claim primitive). Call-recording fakes are not
// a substitute for the contention contract test.
//
// SetLifecycleStatus is required so tracker-transition mutations share the
// same production store path as claim (ddx-0ce3c378).
type ClaimBackend interface {
	Claim(id, assignee string) error
	Get(ctx context.Context, id string) (*bead.Bead, error)
	SetLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions) error
}

// Compile-time: *bead.Store is the production claim/transition backend.
var _ ClaimBackend = (*bead.Store)(nil)

// Compile-time: LocalCoordinator satisfies Coordinator.
var _ Coordinator = (*LocalCoordinator)(nil)

// LocalCoordinator implements Coordinator against a project bead store.
// It is the offline/local path: claim and tracker-transition mutations call
// existing bead-store APIs. Durable offline journal and HTTP transport are
// out of scope.
type LocalCoordinator struct {
	store ClaimBackend

	mu sync.Mutex
	// claimByKey / transitionByKey are process-local idempotency memory for
	// this coordinator instance. Durable journaling is a sibling bead
	// (ADR-022 offline journal).
	claimByKey      map[string]ClaimResult
	transitionByKey map[string]TransitionResult
}

// NewLocalCoordinator returns a Coordinator backed by store. store must be
// non-nil; production callers pass bead.NewStore(...) for the project root.
// try/work bootstrap wiring is ddx-2e49980d (out of scope for the claim/
// transition contract beads).
func NewLocalCoordinator(store ClaimBackend) *LocalCoordinator {
	return &LocalCoordinator{
		store:           store,
		claimByKey:      make(map[string]ClaimResult),
		transitionByKey: make(map[string]TransitionResult),
	}
}

// Claim applies a claim mutation via the bead-store Claim API and maps the
// result to the transport-neutral claim contract outcomes.
func (c *LocalCoordinator) Claim(ctx context.Context, req ClaimRequest) (ClaimResult, error) {
	if c == nil {
		return ClaimResult{}, fmt.Errorf("coordination: nil local coordinator")
	}
	if c.store == nil {
		return ClaimResult{}, fmt.Errorf("coordination: nil claim backend")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ClaimResult{}, err
	}

	beadID := strings.TrimSpace(req.BeadID)
	assignee := strings.TrimSpace(req.Assignee)
	key := strings.TrimSpace(req.IdempotencyKey)
	if beadID == "" {
		return ClaimResult{}, fmt.Errorf("coordination: claim requires bead_id")
	}
	if assignee == "" {
		return ClaimResult{}, fmt.Errorf("coordination: claim requires assignee")
	}
	if key == "" {
		return ClaimResult{}, fmt.Errorf("coordination: claim requires idempotency_key")
	}

	c.mu.Lock()
	if prev, ok := c.claimByKey[key]; ok {
		c.mu.Unlock()
		// Replay of a key this coordinator already observed: return the prior
		// outcome as already_applied without re-invoking the store.
		out := prev
		out.Code = OutcomeAlreadyApplied
		out.IdempotencyKey = key
		return out, nil
	}
	c.mu.Unlock()

	err := c.store.Claim(beadID, assignee)
	if err != nil {
		if result, handled := mapClaimContention(err, beadID, key); handled {
			// Best-effort owner for conflict evidence.
			if owner := c.lookupOwner(ctx, beadID); owner != "" {
				result.Owner = owner
			}
			return result, nil
		}
		return ClaimResult{}, err
	}

	owner := assignee
	if got := c.lookupOwner(ctx, beadID); got != "" {
		owner = got
	}
	result := ClaimResult{
		Code:           OutcomeApplied,
		BeadID:         beadID,
		Owner:          owner,
		IdempotencyKey: key,
	}

	c.mu.Lock()
	c.claimByKey[key] = result
	c.mu.Unlock()

	return result, nil
}

// Transition applies a tracker lifecycle mutation via the bead-store
// SetLifecycleStatus API (TransitionLifecycle) and maps the result to the
// transport-neutral transition contract outcomes.
func (c *LocalCoordinator) Transition(ctx context.Context, req TransitionRequest) (TransitionResult, error) {
	if c == nil {
		return TransitionResult{}, fmt.Errorf("coordination: nil local coordinator")
	}
	if c.store == nil {
		return TransitionResult{}, fmt.Errorf("coordination: nil claim backend")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return TransitionResult{}, err
	}

	beadID := strings.TrimSpace(req.BeadID)
	toStatus := strings.TrimSpace(req.ToStatus)
	key := strings.TrimSpace(req.IdempotencyKey)
	if beadID == "" {
		return TransitionResult{}, fmt.Errorf("coordination: transition requires bead_id")
	}
	if toStatus == "" {
		return TransitionResult{}, fmt.Errorf("coordination: transition requires to_status")
	}
	if key == "" {
		return TransitionResult{}, fmt.Errorf("coordination: transition requires idempotency_key")
	}

	c.mu.Lock()
	if prev, ok := c.transitionByKey[key]; ok {
		c.mu.Unlock()
		out := prev
		out.Code = OutcomeAlreadyApplied
		out.IdempotencyKey = key
		return out, nil
	}
	c.mu.Unlock()

	fromStatus := c.lookupStatus(ctx, beadID)

	opts := bead.LifecycleTransitionOptions{
		OperatorRequired:      req.OperatorRequired,
		ExternalBlockerReason: strings.TrimSpace(req.ExternalBlockerReason),
		ManualClose:           req.ManualClose,
		ManualReopen:          req.ManualReopen,
		Reason:                strings.TrimSpace(req.Reason),
		Actor:                 strings.TrimSpace(req.Actor),
		Source:                strings.TrimSpace(req.Source),
	}
	if opts.Source == "" {
		opts.Source = "coordination.local"
	}

	err := c.store.SetLifecycleStatus(beadID, toStatus, opts)
	if err != nil {
		if result, handled := mapTransitionRejection(err, beadID, fromStatus, toStatus, key); handled {
			return result, nil
		}
		return TransitionResult{}, err
	}

	// Durable status after apply (may equal from when store no-ops same-status).
	after := c.lookupStatus(ctx, beadID)
	if after == "" {
		after = toStatus
	}
	result := TransitionResult{
		Code:           OutcomeApplied,
		BeadID:         beadID,
		FromStatus:     fromStatus,
		ToStatus:       after,
		IdempotencyKey: key,
	}

	c.mu.Lock()
	c.transitionByKey[key] = result
	c.mu.Unlock()

	return result, nil
}

func (c *LocalCoordinator) lookupOwner(ctx context.Context, beadID string) string {
	b, err := c.store.Get(ctx, beadID)
	if err != nil || b == nil {
		return ""
	}
	return strings.TrimSpace(b.Owner)
}

func (c *LocalCoordinator) lookupStatus(ctx context.Context, beadID string) string {
	b, err := c.store.Get(ctx, beadID)
	if err != nil || b == nil {
		return ""
	}
	return strings.TrimSpace(b.Status)
}

// mapClaimContention translates bead-store claim rejections into the
// contract-defined conflict/already_claimed outcome. Returns handled=false
// for errors that are not contention (not found, IO, invalid state, etc.).
func mapClaimContention(err error, beadID, key string) (ClaimResult, bool) {
	if err == nil {
		return ClaimResult{}, false
	}
	if errors.Is(err, bead.ErrAlreadyClaimed) {
		return ClaimResult{
			Code:           OutcomeConflict,
			BeadID:         beadID,
			Reason:         ReasonAlreadyClaimed,
			IdempotencyKey: key,
		}, true
	}
	// Store.Claim currently formats contention as:
	//   "bead: cannot claim <id> from status <status>"
	// when a fresh lease blocks the claim (open or in_progress).
	msg := err.Error()
	if strings.Contains(msg, "cannot claim") {
		return ClaimResult{
			Code:           OutcomeConflict,
			BeadID:         beadID,
			Reason:         ReasonAlreadyClaimed,
			IdempotencyKey: key,
		}, true
	}
	return ClaimResult{}, false
}

// mapTransitionRejection translates bead-store lifecycle transition rejections
// into OutcomeConflict / ReasonTransitionRejected. Returns handled=false for
// errors that are not transition-matrix rejections (not found, IO, etc.).
func mapTransitionRejection(err error, beadID, fromStatus, toStatus, key string) (TransitionResult, bool) {
	if err == nil {
		return TransitionResult{}, false
	}
	msg := err.Error()
	// Store wraps ValidateLifecycleTransition as:
	//   "bead: lifecycle transition <from> -> <to> rejected: ..."
	if strings.Contains(msg, "lifecycle transition") && strings.Contains(msg, "rejected") {
		return TransitionResult{
			Code:           OutcomeConflict,
			BeadID:         beadID,
			FromStatus:     fromStatus,
			ToStatus:       toStatus,
			Reason:         ReasonTransitionRejected,
			IdempotencyKey: key,
		}, true
	}
	return TransitionResult{}, false
}
