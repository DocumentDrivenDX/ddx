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
type ClaimBackend interface {
	Claim(id, assignee string) error
	Get(ctx context.Context, id string) (*bead.Bead, error)
}

// Compile-time: *bead.Store is the production claim backend.
var _ ClaimBackend = (*bead.Store)(nil)

// Compile-time: LocalCoordinator satisfies Coordinator.
var _ Coordinator = (*LocalCoordinator)(nil)

// LocalCoordinator implements Coordinator against a project bead store.
// It is the offline/local path: claim mutations call the existing bead-store
// Claim API. Durable offline journal and HTTP transport are out of scope.
type LocalCoordinator struct {
	store ClaimBackend

	mu sync.Mutex
	// appliedByKey is process-local idempotency memory for this coordinator
	// instance. Durable journaling is a sibling bead (ADR-022 offline journal).
	appliedByKey map[string]ClaimResult
}

// NewLocalCoordinator returns a Coordinator backed by store. store must be
// non-nil; production callers pass bead.NewStore(...) for the project root.
// try/work bootstrap wiring is ddx-2e49980d (out of scope for the claim
// contract bead).
func NewLocalCoordinator(store ClaimBackend) *LocalCoordinator {
	return &LocalCoordinator{
		store:        store,
		appliedByKey: make(map[string]ClaimResult),
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
	if prev, ok := c.appliedByKey[key]; ok {
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
	c.appliedByKey[key] = result
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
