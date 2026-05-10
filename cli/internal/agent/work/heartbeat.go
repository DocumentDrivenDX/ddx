package work

import (
	"context"
	"sync"
	"time"
)

type heartbeatStore interface {
	TouchClaimHeartbeat(string) error
}

// WithHeartbeat runs fn while periodically calling store.TouchClaimHeartbeat
// on a background goroutine at the given interval. The goroutine is stopped
// and waited for before WithHeartbeat returns.
func WithHeartbeat[T any](ctx context.Context, beadID string, interval time.Duration, store heartbeatStore, fn func() (T, error)) (T, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	return withHeartbeatCh(ctx, beadID, ticker.C, store, fn)
}

// withHeartbeatCh is the injectable variant used by tests to supply a stub tick channel.
func withHeartbeatCh[T any](ctx context.Context, beadID string, tickCh <-chan time.Time, store heartbeatStore, fn func() (T, error)) (T, error) {
	hbCtx, hbCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-tickCh:
				_ = store.TouchClaimHeartbeat(beadID)
			}
		}
	}()
	result, err := fn()
	hbCancel()
	wg.Wait()
	return result, err
}
