package bead

import (
	"context"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead/internal/lifecycle"
)

// NewStore is the public constructor surface for bead storage.
var NewStore = newStore

// NewStoreWithCollection constructs a store for a named logical collection.
var NewStoreWithCollection = newStoreWithCollection

// StoreFactory creates the bead reader used to watch one project.
type StoreFactory func(projectID string) (BeadReader, error)

// NewLifecycleSubscriber is the public lifecycle-watching entrypoint
// (TD-027 §21). It polls each watched project's store at interval and
// returns a LifecycleSubscriber; the concrete hub implementation lives in
// internal/bead/internal/lifecycle and is never named by callers. Callers
// that own the returned subscriber's lifetime (e.g. a server shutting down)
// can stop its background watchers via a Close() method on the returned
// value.
func NewLifecycleSubscriber(factory StoreFactory, interval time.Duration) LifecycleSubscriber {
	adapted := func(projectID string) (lifecycle.Reader, error) {
		reader, err := factory(projectID)
		if err != nil {
			return nil, err
		}
		if reader == nil {
			return nil, nil
		}
		return beadReaderAdapter{reader: reader}, nil
	}
	return &lifecycleSubscriber{hub: lifecycle.NewHub(adapted, interval)}
}

// beadReaderAdapter narrows a BeadReader down to the internal lifecycle
// package's Reader contract so that package need not depend on package bead
// (which would create an import cycle with this file).
type beadReaderAdapter struct {
	reader BeadReader
}

func (a beadReaderAdapter) ReadAll(ctx context.Context) ([]lifecycle.BeadSnapshot, error) {
	beads, err := a.reader.ReadAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]lifecycle.BeadSnapshot, 0, len(beads))
	for _, b := range beads {
		out = append(out, lifecycle.BeadSnapshot{ID: b.ID, Status: b.Status, Owner: b.Owner, Title: b.Title})
	}
	return out, nil
}

// lifecycleSubscriber adapts *lifecycle.Hub to the public LifecycleSubscriber
// surface. It also exposes Close so callers that own the hub's lifetime can
// stop the background watchers without naming the concrete hub type.
type lifecycleSubscriber struct {
	hub *lifecycle.Hub
}

var _ LifecycleSubscriber = (*lifecycleSubscriber)(nil)

func (l *lifecycleSubscriber) SubscribeLifecycle(ctx context.Context, projectID string) (<-chan LifecycleEvent, func(), error) {
	events, unsub, err := l.hub.SubscribeLifecycle(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	out := make(chan LifecycleEvent)
	go func() {
		defer close(out)
		for evt := range events {
			out <- LifecycleEvent{
				EventID:   evt.EventID,
				BeadID:    evt.BeadID,
				Kind:      evt.Kind,
				Summary:   evt.Summary,
				Body:      evt.Body,
				Actor:     evt.Actor,
				Timestamp: evt.Timestamp,
			}
		}
	}()
	return out, unsub, nil
}

// Close stops all background watchers owned by this subscriber.
func (l *lifecycleSubscriber) Close() {
	if l == nil || l.hub == nil {
		return
	}
	l.hub.Close()
}
