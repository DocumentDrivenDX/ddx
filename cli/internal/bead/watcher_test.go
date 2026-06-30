package bead

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedBeadReader struct {
	calls   atomic.Int32
	batches [][]Bead
}

func (r *scriptedBeadReader) ReadAll(ctx context.Context) ([]Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(r.batches) == 0 {
		return nil, nil
	}
	idx := int(r.calls.Add(1) - 1)
	if idx >= len(r.batches) {
		idx = len(r.batches) - 1
	}
	return append([]Bead(nil), r.batches[idx]...), nil
}

func (r *scriptedBeadReader) ReadAllFiltered(ctx context.Context, pred func(Bead) bool) ([]Bead, error) {
	beads, err := r.ReadAll(ctx)
	if err != nil || pred == nil {
		return beads, err
	}
	out := make([]Bead, 0, len(beads))
	for _, b := range beads {
		if pred(b) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *scriptedBeadReader) Get(ctx context.Context, id string) (*Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(r.batches) == 0 {
		return nil, nil
	}
	idx := len(r.batches) - 1
	for i := len(r.batches[idx]) - 1; i >= 0; i-- {
		if r.batches[idx][i].ID == id {
			bead := r.batches[idx][i]
			return &bead, nil
		}
	}
	return nil, nil
}

func waitForLifecycleEvent(t *testing.T, ch <-chan LifecycleEvent) LifecycleEvent {
	t.Helper()
	select {
	case evt, ok := <-ch:
		if !ok {
			t.Fatal("lifecycle event channel closed unexpectedly")
		}
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle event")
		return LifecycleEvent{}
	}
}

func TestWatcherHub_UsesProvidedFactory(t *testing.T) {
	reader := &scriptedBeadReader{
		batches: [][]Bead{
			{
				{ID: "bx-1", Title: "First bead", Status: StatusOpen},
			},
			{
				{ID: "bx-1", Title: "First bead", Status: StatusClosed},
			},
		},
	}
	factoryCalls := make(chan string, 1)
	hub := NewWatcherHub(func(projectID string) (BeadReader, error) {
		select {
		case factoryCalls <- projectID:
		default:
		}
		return reader, nil
	}, 5*time.Millisecond)
	t.Cleanup(hub.Close)

	events, unsub, err := hub.SubscribeLifecycle(context.Background(), "project-123")
	require.NoError(t, err)
	t.Cleanup(unsub)

	select {
	case got := <-factoryCalls:
		require.Equal(t, "project-123", got)
	case <-time.After(time.Second):
		t.Fatal("factory was not called")
	}

	require.Eventually(t, func() bool {
		return reader.calls.Load() >= 1
	}, time.Second, 5*time.Millisecond)

	first := waitForLifecycleEvent(t, events)
	assert.Equal(t, "bx-1", first.BeadID)
	assert.Equal(t, "created", first.Kind)
	assert.Equal(t, "bead bx-1 created: First bead", first.Summary)

	require.Eventually(t, func() bool {
		return reader.calls.Load() >= 2
	}, time.Second, 5*time.Millisecond)

	second := waitForLifecycleEvent(t, events)
	assert.Equal(t, "bx-1", second.BeadID)
	assert.Equal(t, "status_changed", second.Kind)
	assert.Equal(t, "status changed from open to closed", second.Summary)
}

func TestWatcherHub_PropagatesFactoryErrors(t *testing.T) {
	wantErr := errors.New("factory boom")
	hub := NewWatcherHub(func(string) (BeadReader, error) {
		return nil, wantErr
	}, time.Millisecond)
	t.Cleanup(hub.Close)

	ch, unsub, err := hub.SubscribeLifecycle(context.Background(), "project-123")
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, ch)
	assert.Nil(t, unsub)
}
