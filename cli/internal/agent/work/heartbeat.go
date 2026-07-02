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
// on a background goroutine at the given interval. When reporter is non-nil,
// reporter.OnTick is invoked after each TouchClaimHeartbeat so callers can
// mirror the same liveness signal to a worker-status sidecar and the
// optional server probe without rewriting the bead tracker. The goroutine
// is stopped and waited for before WithHeartbeat returns.
func WithHeartbeat[T any](ctx context.Context, beadID string, interval time.Duration, store heartbeatStore, reporter LivenessReporter, fn func() (T, error)) (T, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	return withHeartbeatCh(ctx, beadID, ticker.C, store, reporter, fn)
}

// withHeartbeatCh is the injectable variant used by tests to supply a stub tick channel.
func withHeartbeatCh[T any](ctx context.Context, beadID string, tickCh <-chan time.Time, store heartbeatStore, reporter LivenessReporter, fn func() (T, error)) (T, error) {
	hbCtx, hbCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-hbCtx.Done():
				return
			case tick := <-tickCh:
				_ = store.TouchClaimHeartbeat(beadID)
				if reporter != nil {
					reporter.OnTick(tick)
				}
			}
		}
	}()
	result, err := fn()
	hbCancel()
	wg.Wait()
	return result, err
}
