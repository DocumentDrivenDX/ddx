package bead

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// LifecycleEvent is emitted by a WatcherHub when a bead is created or updated.
type LifecycleEvent struct {
	EventID   string
	BeadID    string
	Kind      string // "created", "status_changed", "updated"
	Summary   string
	Body      string
	Actor     string
	Timestamp time.Time
}

// WatcherHub manages per-project bead file watchers by polling beads.jsonl
// for changes. It satisfies the BeadLifecycleSubscriber interface used by
// the GraphQL subscription resolver.
type WatcherHub struct {
	mu       sync.Mutex
	factory  StoreFactory
	watchers map[string]*projectWatcher
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

type StoreFactory func(ctx context.Context, projectID string) (BeadReader, error)

var _ LifecycleSubscriber = (*WatcherHub)(nil)

// NewWatcherHub creates a hub that polls each watched project at interval.
func NewWatcherHub(factory StoreFactory, interval time.Duration) *WatcherHub {
	ctx, cancel := context.WithCancel(context.Background())
	return &WatcherHub{
		factory:  factory,
		watchers: make(map[string]*projectWatcher),
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Close stops all background watchers.
func (h *WatcherHub) Close() {
	h.cancel()
}

// SubscribeLifecycle registers for lifecycle events from the project at
// projectID (the project root directory). A new per-project watcher is
// started on first Subscribe call. The returned func unsubscribes.
func (h *WatcherHub) SubscribeLifecycle(ctx context.Context, projectID string) (<-chan LifecycleEvent, func(), error) {
	if h.factory == nil {
		return nil, nil, fmt.Errorf("bead watcher store factory is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	pw, ok := h.watchers[projectID]
	if !ok {
		store, err := h.factory(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		pw = newProjectWatcher(store, h.interval)
		h.watchers[projectID] = pw
		go pw.run(h.ctx)
	}
	ch, unsub := pw.subscribe()
	return ch, unsub, nil
}

// beadState captures the fields we compare across polls to detect changes.
type beadState struct {
	status string
	owner  string
	title  string
}

// projectWatcher polls a single bead store and broadcasts lifecycle events.
type projectWatcher struct {
	store    BeadReader
	interval time.Duration

	mu       sync.Mutex
	subs     []chan LifecycleEvent
	snapshot map[string]beadState
}

func newProjectWatcher(store BeadReader, interval time.Duration) *projectWatcher {
	return &projectWatcher{
		store:    store,
		interval: interval,
		snapshot: make(map[string]beadState),
	}
}

func (pw *projectWatcher) subscribe() (<-chan LifecycleEvent, func()) {
	ch := make(chan LifecycleEvent, 16)
	pw.mu.Lock()
	pw.subs = append(pw.subs, ch)
	pw.mu.Unlock()
	unsub := func() {
		pw.mu.Lock()
		defer pw.mu.Unlock()
		for i, sub := range pw.subs {
			if sub == ch {
				pw.subs = append(pw.subs[:i], pw.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsub
}

func (pw *projectWatcher) broadcast(evt LifecycleEvent) {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	for _, ch := range pw.subs {
		select {
		case ch <- evt:
		default: // drop event if subscriber buffer is full
		}
	}
}

func (pw *projectWatcher) run(ctx context.Context) {
	ticker := time.NewTicker(pw.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pw.poll(ctx)
		}
	}
}

func (pw *projectWatcher) poll(ctx context.Context) {
	beads, err := pw.store.ReadAll(ctx)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	for _, b := range beads {
		prev, exists := pw.snapshot[b.ID]
		curr := beadState{status: b.Status, owner: b.Owner, title: b.Title}

		var evt *LifecycleEvent
		switch {
		case !exists:
			evt = &LifecycleEvent{
				BeadID:    b.ID,
				Kind:      "created",
				Summary:   fmt.Sprintf("bead %s created: %s", b.ID, b.Title),
				Timestamp: now,
			}
		case prev.status != curr.status:
			evt = &LifecycleEvent{
				BeadID:    b.ID,
				Kind:      "status_changed",
				Summary:   fmt.Sprintf("status changed from %s to %s", prev.status, curr.status),
				Timestamp: now,
			}
		case prev != curr:
			evt = &LifecycleEvent{
				BeadID:    b.ID,
				Kind:      "updated",
				Summary:   fmt.Sprintf("bead %s updated", b.ID),
				Timestamp: now,
			}
		}

		pw.snapshot[b.ID] = curr

		if evt != nil {
			evt.EventID = genLifecycleEventID(b.ID)
			pw.broadcast(*evt)
		}
	}
}

func genLifecycleEventID(beadID string) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return beadID + "-evt"
	}
	return fmt.Sprintf("%s-%s", beadID, hex.EncodeToString(b))
}
