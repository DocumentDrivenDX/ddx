package graphql

import (
	"context"
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// RunRequeueEventKind is the BeadEvent.Kind appended to the originating bead's
// audit log every time runRequeue successfully re-queues that bead. The body
// captures operator identity, the run id under attempt, the optional layer
// override, and the idempotency key so duplicate-key collapses are traceable.
const RunRequeueEventKind = "run_requeue"

// RunRequeue implements the runRequeue mutation: given a runId, look up the
// originating bead, atomically dedupe by idempotencyKey, reopen the bead so
// the execute-loop can claim it again, and append an audit event capturing
// operator identity. Concurrent calls with the same idempotencyKey collapse
// to a single re-queue (the second caller receives deduplicated=true and the
// same bead).
func (r *mutationResolver) RunRequeue(ctx context.Context, input RunRequeueInput) (*RunRequeueResult, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(input.RunID) == "" {
		return nil, fmt.Errorf("runRequeue: runId is required")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, fmt.Errorf("runRequeue: idempotencyKey is required")
	}

	cache := r.RunRequeueIdempotency
	if cache == nil {
		return nil, fmt.Errorf("runRequeue: idempotency cache not configured")
	}

	provider, ok := r.State.(RunsStateProvider)
	if !ok {
		return nil, fmt.Errorf("runRequeue: runs state provider not configured")
	}
	run, ok := provider.GetRunGraphQL(input.RunID)
	if !ok || run == nil {
		return nil, fmt.Errorf("runRequeue: run %s not found", input.RunID)
	}
	if run.BeadID == nil || *run.BeadID == "" {
		return nil, fmt.Errorf("runRequeue: run %s has no originating bead", input.RunID)
	}
	originatingBeadID := *run.BeadID

	store := r.beadStore(ctx)

	// Atomic claim block: serialize lookup+reserve+store so concurrent calls
	// with the same idempotencyKey collapse to a single re-queue. The store's
	// per-bead Update is itself atomic; this mutex covers the cache plus the
	// audit event so duplicate keys never produce two reopen+event pairs.
	r.runRequeueMu.Lock()
	defer r.runRequeueMu.Unlock()

	if cachedID, hit := cache.Lookup(input.IdempotencyKey); hit {
		// A prior call under this key already requeued a bead. Return that
		// bead unchanged regardless of its current lifecycle status —
		// "duplicate-key returns existing requeued bead, not error".
		if existing, err := store.Get(cachedID); err == nil {
			return &RunRequeueResult{
				Bead:         beadModelFromBead(existing),
				Deduplicated: true,
			}, nil
		}
		// The cached id points at a missing bead — fall through and replay
		// the requeue against the run's current originating bead.
	}

	if err := store.Update(originatingBeadID, func(b *bead.Bead) {
		b.Status = bead.StatusOpen
		b.Owner = ""
	}); err != nil {
		return nil, fmt.Errorf("runRequeue: reopen bead %s: %w", originatingBeadID, err)
	}

	identity := operatorPromptIdentityInfo{kind: "unknown", actor: "anonymous"}
	if httpReq := httpRequestFromContext(ctx); httpReq != nil {
		identity = operatorPromptIdentity(httpReq)
	}
	layerOverride := ""
	if input.Layer != nil {
		layerOverride = *input.Layer
	}
	auditBody := fmt.Sprintf(
		"identity=%s actor=%s run_id=%s idempotency_key=%s layer_override=%s",
		identity.kind,
		identity.actor,
		input.RunID,
		input.IdempotencyKey,
		layerOverride,
	)
	if err := store.AppendEvent(originatingBeadID, bead.BeadEvent{
		Kind:    RunRequeueEventKind,
		Summary: "run requeued",
		Body:    auditBody,
		Actor:   identity.actor,
		Source:  "graphql:runRequeue",
	}); err != nil {
		return nil, fmt.Errorf("runRequeue: append audit event: %w", err)
	}

	cache.Store(input.IdempotencyKey, originatingBeadID)

	persisted, err := store.Get(originatingBeadID)
	if err != nil {
		return nil, err
	}
	return &RunRequeueResult{
		Bead:         beadModelFromBead(persisted),
		Deduplicated: false,
	}, nil
}
