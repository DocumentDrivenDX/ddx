package work

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type stubHeartbeatStore struct {
	count atomic.Int32
}

func (s *stubHeartbeatStore) TouchClaimHeartbeat(_ string) error {
	s.count.Add(1)
	return nil
}

func TestWithHeartbeat_TickerFiredBeforeReturn(t *testing.T) {
	store := &stubHeartbeatStore{}
	tickCh := make(chan time.Time, 1)
	tickCh <- time.Now()

	_, err := withHeartbeatCh(context.Background(), "bead-1", tickCh, store, nil, func() (struct{}, error) {
		time.Sleep(10 * time.Millisecond)
		return struct{}{}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.count.Load() < 1 {
		t.Errorf("expected Heartbeat called at least once, got %d", store.count.Load())
	}
}

func TestWithHeartbeat_CancelPropagates(t *testing.T) {
	store := &stubHeartbeatStore{}
	ctx, cancel := context.WithCancel(context.Background())
	tickCh := make(chan time.Time) // unbuffered; no ticks will be sent

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = withHeartbeatCh(ctx, "bead-1", tickCh, store, nil, func() (struct{}, error) {
			cancel()
			return struct{}{}, errors.New("fn error")
		})
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("withHeartbeatCh did not return after context cancellation")
	}
}
