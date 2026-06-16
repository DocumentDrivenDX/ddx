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

type blockingHeartbeatStore struct {
	entered chan struct{}
	release chan struct{}
	once    atomic.Bool
}

func (s *blockingHeartbeatStore) TouchClaimHeartbeat(_ string) error {
	if s.once.CompareAndSwap(false, true) {
		close(s.entered)
	}
	<-s.release
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

func TestWithHeartbeat_BlockedHeartbeatDoesNotBlockFnReturn(t *testing.T) {
	prevStopWait := heartbeatStopWait
	heartbeatStopWait = 10 * time.Millisecond
	defer func() {
		heartbeatStopWait = prevStopWait
	}()

	store := &blockingHeartbeatStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(store.release)
	tickCh := make(chan time.Time, 1)
	tickCh <- time.Now()

	done := make(chan error, 1)
	go func() {
		_, err := withHeartbeatCh(context.Background(), "bead-1", tickCh, store, nil, func() (struct{}, error) {
			select {
			case <-store.entered:
			case <-time.After(1 * time.Second):
				return struct{}{}, errors.New("heartbeat did not start")
			}
			return struct{}{}, nil
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("withHeartbeatCh blocked behind TouchClaimHeartbeat after fn returned")
	}
}

func TestWithHeartbeat_ClosedTickChannelReturns(t *testing.T) {
	store := &stubHeartbeatStore{}
	tickCh := make(chan time.Time)
	close(tickCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = withHeartbeatCh(context.Background(), "bead-1", tickCh, store, nil, func() (struct{}, error) {
			return struct{}{}, nil
		})
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("withHeartbeatCh did not return after tick channel closed")
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
