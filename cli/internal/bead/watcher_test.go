package bead

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWatcherReader struct {
	mu    sync.Mutex
	beads []Bead
}

func (f *fakeWatcherReader) ReadAll(ctx context.Context) ([]Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Bead, len(f.beads))
	copy(out, f.beads)
	return out, nil
}

func (f *fakeWatcherReader) ReadAllFiltered(ctx context.Context, pred func(Bead) bool) ([]Bead, error) {
	all, err := f.ReadAll(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]Bead, 0, len(all))
	for _, b := range all {
		if pred == nil || pred(b) {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}

func (f *fakeWatcherReader) Get(ctx context.Context, id string) (*Bead, error) {
	all, err := f.ReadAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, ErrNotFound
}

func TestWatcherHub_UsesProvidedFactory(t *testing.T) {
	reader := &fakeWatcherReader{beads: []Bead{{
		ID:     "ddx-factory",
		Title:  "factory bead",
		Status: StatusOpen,
	}}}
	var called int
	hub := NewWatcherHub(func(ctx context.Context, projectID string) (BeadReader, error) {
		called++
		assert.Equal(t, "project-a", projectID)
		return reader, nil
	}, time.Millisecond)
	defer hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	events, unsub, err := hub.SubscribeLifecycle(ctx, "project-a")
	require.NoError(t, err)
	defer unsub()
	require.Equal(t, 1, called)

	select {
	case evt := <-events:
		assert.Equal(t, "ddx-factory", evt.BeadID)
		assert.Equal(t, "created", evt.Kind)
		assert.Contains(t, evt.Summary, "factory bead")
	case <-ctx.Done():
		t.Fatalf("timed out waiting for watcher event: %v", ctx.Err())
	}
}

func TestWatcherHub_PropagatesFactoryErrors(t *testing.T) {
	wantErr := errors.New("backend unavailable")
	hub := NewWatcherHub(func(ctx context.Context, projectID string) (BeadReader, error) {
		return nil, wantErr
	}, time.Millisecond)
	defer hub.Close()

	events, unsub, err := hub.SubscribeLifecycle(context.Background(), "project-a")
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, events)
	assert.Nil(t, unsub)
}
