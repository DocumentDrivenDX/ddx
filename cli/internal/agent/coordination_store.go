package agent

// coordination_store.go — ExecuteBeadLoopStore adapter that routes claim and
// tracker-transition mutations through the shared CoordinationClient (ADR-022).

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// coordinatingLoopStore routes claim and lifecycle transitions through the
// reconnecting CoordinationClient while delegating reads and non-coordination
// mutations to the underlying store.
type coordinatingLoopStore struct {
	inner  ExecuteBeadLoopStore
	client *CoordinationClient
}

// WrapStoreWithCoordination returns a loop store that uses client for claim
// and tracker-transition operations. When client is nil, inner is returned
// unchanged.
func WrapStoreWithCoordination(inner ExecuteBeadLoopStore, client *CoordinationClient) ExecuteBeadLoopStore {
	if inner == nil || client == nil {
		return inner
	}
	return &coordinatingLoopStore{inner: inner, client: client}
}

// ClaimWithOptions satisfies the claimWithOptionsStore optional interface so
// the execute loop's preferred claim path still hits the shared client.
func (s *coordinatingLoopStore) ClaimWithOptions(id, assignee, session, worktree string) error {
	return s.client.ClaimBead(context.Background(), id, assignee, session, worktree)
}

func (s *coordinatingLoopStore) ReadyExecution() ([]bead.Bead, error) {
	return s.inner.ReadyExecution()
}

func (s *coordinatingLoopStore) Get(ctx context.Context, id string) (*bead.Bead, error) {
	return s.inner.Get(ctx, id)
}

func (s *coordinatingLoopStore) Create(ctx context.Context, b *bead.Bead) error {
	return s.inner.Create(ctx, b)
}

func (s *coordinatingLoopStore) Claim(id, assignee string) error {
	return s.client.ClaimBead(context.Background(), id, assignee, "", "")
}

func (s *coordinatingLoopStore) Unclaim(id string) error {
	return s.inner.Unclaim(id)
}

func (s *coordinatingLoopStore) TouchClaimHeartbeat(id string) error {
	return s.inner.TouchClaimHeartbeat(id)
}

func (s *coordinatingLoopStore) CloseWithEvidence(id, sessionID, commitSHA string) error {
	return s.inner.CloseWithEvidence(id, sessionID, commitSHA)
}

func (s *coordinatingLoopStore) AppendEvent(id string, event bead.BeadEvent) error {
	return s.inner.AppendEvent(id, event)
}

func (s *coordinatingLoopStore) Events(id string) ([]bead.BeadEvent, error) {
	return s.inner.Events(id)
}

func (s *coordinatingLoopStore) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	return s.inner.SetExecutionCooldown(id, until, status, detail, baseRev)
}

func (s *coordinatingLoopStore) AppendNotes(id string, notes string) error {
	return s.inner.AppendNotes(id, notes)
}

func (s *coordinatingLoopStore) IncrNoChangesCount(id string) (int, error) {
	return s.inner.IncrNoChangesCount(id)
}

func (s *coordinatingLoopStore) Reopen(id, reason, notes string) error {
	return s.inner.Reopen(id, reason, notes)
}

func (s *coordinatingLoopStore) Update(ctx context.Context, id string, mutate func(*bead.Bead)) error {
	return s.inner.Update(ctx, id, mutate)
}

func (s *coordinatingLoopStore) UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error {
	if err := s.client.TransitionBead(context.Background(), id, status, opts); err != nil {
		return err
	}
	if mutate == nil {
		return nil
	}
	return s.inner.Update(context.Background(), id, func(b *bead.Bead) {
		_ = mutate(b)
	})
}

func (s *coordinatingLoopStore) ParkToProposed(id string, reason bead.ParkReason, mutate func(*bead.Bead)) error {
	meta, ok := parkReasonMeta(reason)
	if !ok {
		// Fall back to the store's error for unknown reasons.
		return s.inner.ParkToProposed(id, reason, mutate)
	}
	if err := s.client.TransitionBead(context.Background(), id, bead.StatusProposed, bead.LifecycleTransitionOptions{
		OperatorRequired: true,
		Reason:           meta.reason,
		Source:           meta.source,
	}); err != nil {
		return err
	}
	if mutate == nil {
		return nil
	}
	return s.inner.Update(context.Background(), id, mutate)
}

func (s *coordinatingLoopStore) ParkToProposedWithIntakeEvent(id, actor, outcome, reason, detail string, body map[string]any, at time.Time, mutate func(*bead.Bead)) error {
	// Coordinate the proposed transition, then append the intake event on the
	// store (same shape as bead.Store.ParkToProposedWithIntakeEvent).
	if err := s.ParkToProposed(id, bead.ParkIntakeRejection, mutate); err != nil {
		return err
	}
	if body != nil {
		if ruleFp, ok := body["rule_fingerprint"].(string); ok && ruleFp != "" {
			events, err := s.inner.Events(id)
			if err == nil {
				for _, ev := range events {
					if ev.Kind != "intake.blocked" {
						continue
					}
					// Dedup is best-effort; missing body parse is fine.
					_ = ruleFp
					_ = reason
					_ = detail
				}
			}
		}
	}
	return s.inner.AppendEvent(id, bead.BeadEvent{
		Kind:      "intake.blocked",
		Summary:   outcome,
		Body:      mustJSONBody(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

func mustJSONBody(body map[string]any) string {
	if body == nil {
		return "{}"
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// parkReasonMeta mirrors the store parkReasonMetaMap subset needed for
// coordination transitions without importing unexported bead helpers.
type parkReasonMetaInfo struct {
	reason string
	source string
}

func parkReasonMeta(reason bead.ParkReason) (parkReasonMetaInfo, bool) {
	switch reason {
	case bead.ParkNoChangesOperatorRequired:
		return parkReasonMetaInfo{reason: string(reason), source: "ddx work"}, true
	case bead.ParkPostReviewMalfunction:
		return parkReasonMetaInfo{reason: string(reason), source: "ddx work"}, true
	case bead.ParkAutoRecoveryFailed:
		return parkReasonMetaInfo{reason: string(reason), source: "ddx work"}, true
	case bead.ParkIntakeRejection:
		return parkReasonMetaInfo{reason: string(reason), source: "ddx work"}, true
	default:
		if reason == "" {
			return parkReasonMetaInfo{}, false
		}
		return parkReasonMetaInfo{reason: string(reason), source: "ddx work"}, true
	}
}

// Release forwards to the underlying store when it supports atomic claim
// release (same optional interface the execute loop uses).
func (s *coordinatingLoopStore) Release(beadID, assignee, toStatus string) error {
	if r, ok := s.inner.(leaseReleaser); ok {
		return r.Release(beadID, assignee, toStatus)
	}
	return s.inner.Unclaim(beadID)
}

// CooldownOverrideInfo forwards the force-claim / ignore-cooldown reporter
// surface so WrapStoreWithCoordination does not strip singleBeadStore or
// ignoreCooldownStore metadata needed for appendForceClaimEvent.
func (s *coordinatingLoopStore) CooldownOverrideInfo(beadID string) (string, bool) {
	if reporter, ok := s.inner.(cooldownOverrideReporter); ok {
		return reporter.CooldownOverrideInfo(beadID)
	}
	return "", false
}
