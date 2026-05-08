package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// ProgressSubscriber can subscribe to live progress events from a worker.
// WorkerManager satisfies this interface.
type ProgressSubscriber interface {
	SubscribeProgress(workerID string) (<-chan agent.ProgressEvent, func())
}

// WorkerProgress is the resolver for the workerProgress subscription.
// It wraps ProgressSubscriber.SubscribeProgress to stream FEAT-006 progress
// events for the given worker as GraphQL WorkerEvent values.
func (r *subscriptionResolver) WorkerProgress(ctx context.Context, workerID string) (<-chan *WorkerEvent, error) {
	if r.Workers == nil {
		return nil, fmt.Errorf("subscription not available: workers not configured")
	}

	src, unsub := r.Workers.SubscribeProgress(workerID)
	out := make(chan *WorkerEvent, 16)

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
				ts := evt.TS.Format(time.RFC3339)
				we := &WorkerEvent{
					EventID:   evt.EventID,
					WorkerID:  evt.WorkerID,
					Phase:     evt.Phase,
					Timestamp: ts,
				}
				if evt.Message != "" {
					we.LogLine = &evt.Message
				}
				if evt.BeadID != "" {
					beadID := evt.BeadID
					we.BeadID = &beadID
				}
				select {
				case out <- we:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}
