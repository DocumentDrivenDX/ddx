package graphql

import (
	"context"
	"fmt"
	"time"
)

// BeadLifecycle is the resolver for the beadLifecycle subscription.
// It streams lifecycle events (created, status_changed, updated) for all
// beads in the project at projectID whenever beads.jsonl changes on disk.
func (r *subscriptionResolver) BeadLifecycle(ctx context.Context, projectID string) (<-chan *BeadEvent, error) {
	if r.BeadBus == nil {
		return nil, fmt.Errorf("subscription not available: bead watcher not configured")
	}

	src, unsub := r.BeadBus.SubscribeLifecycle(projectID)
	out := make(chan *BeadEvent, 16)

	go func() {
		defer unsub()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-src:
				if !ok {
					return
				}
				ts := evt.Timestamp.Format(time.RFC3339)
				be := &BeadEvent{
					EventID:   evt.EventID,
					BeadID:    evt.BeadID,
					Kind:      evt.Kind,
					Timestamp: ts,
				}
				if evt.Summary != "" {
					s := evt.Summary
					be.Summary = &s
				}
				if evt.Body != "" {
					b := evt.Body
					be.Body = &b
				}
				if evt.Actor != "" {
					a := evt.Actor
					be.Actor = &a
				}
				select {
				case out <- be:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}
