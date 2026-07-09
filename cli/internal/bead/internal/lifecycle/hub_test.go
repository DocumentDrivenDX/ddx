package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedReader struct {
	calls   atomic.Int32
	batches [][]BeadSnapshot
}

func (r *scriptedReader) ReadAll(ctx context.Context) ([]BeadSnapshot, error) {
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
	return append([]BeadSnapshot(nil), r.batches[idx]...), nil
}

func waitForEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case evt, ok := <-ch:
		if !ok {
			t.Fatal("lifecycle event channel closed unexpectedly")
		}
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle event")
		return Event{}
	}
}

func TestHub_UsesProvidedFactory(t *testing.T) {
	reader := &scriptedReader{
		batches: [][]BeadSnapshot{
			{{ID: "bx-1", Title: "First bead", Status: "open"}},
			{{ID: "bx-1", Title: "First bead", Status: "closed"}},
		},
	}
	factoryCalls := make(chan string, 1)
	hub := NewHub(func(projectID string) (Reader, error) {
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

	first := waitForEvent(t, events)
	assert.Equal(t, "bx-1", first.BeadID)
	assert.Equal(t, "created", first.Kind)
	assert.Equal(t, "bead bx-1 created: First bead", first.Summary)

	require.Eventually(t, func() bool {
		return reader.calls.Load() >= 2
	}, time.Second, 5*time.Millisecond)

	second := waitForEvent(t, events)
	assert.Equal(t, "bx-1", second.BeadID)
	assert.Equal(t, "status_changed", second.Kind)
	assert.Equal(t, "status changed from open to closed", second.Summary)
}

func TestHub_PropagatesFactoryErrors(t *testing.T) {
	wantErr := errors.New("factory boom")
	hub := NewHub(func(string) (Reader, error) {
		return nil, wantErr
	}, time.Millisecond)
	t.Cleanup(hub.Close)

	ch, unsub, err := hub.SubscribeLifecycle(context.Background(), "project-123")
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, ch)
	assert.Nil(t, unsub)
}
