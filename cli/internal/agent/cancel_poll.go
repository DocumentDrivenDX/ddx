package agent

import (
	"context"
	"sync/atomic"
	"time"
)

// startCancelPoll launches a goroutine that polls the operator-cancel marker
// on the bead every CancelPollInterval. On the first positive read it writes
// cancel-honored:true and cancels dispatchCancel so the in-flight agent
// invocation aborts at the next safe point (between LLM turns / git ops).
//
// Returns nil, nil when the cancel store is not wired (BeadCancel is optional
// runtime plumbing and only the server worker provides it). Otherwise returns
// an *atomic.Bool the caller inspects after the agent dispatch returns to
// decide whether to emit a preserved_for_review/operator_cancel result, and a
// stop function the caller must call to cancel the poll and block until its
// goroutine has actually exited — mirroring startRunStateRefresh's contract
// so the poll goroutine never outlives the attempt that started it.
func startCancelPoll(ctx context.Context, dispatchCancel context.CancelFunc, beadID string, store BeadCancelStore) (*atomic.Bool, func()) {
	if store == nil || beadID == "" {
		return nil, func() {}
	}
	honored := &atomic.Bool{}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(CancelPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				requested, err := store.IsCancelRequested(beadID)
				if err != nil || !requested {
					continue
				}
				honored.Store(true)
				_ = store.MarkCancelHonored(beadID)
				dispatchCancel()
				return
			}
		}
	}()
	return honored, func() {
		cancel()
		<-done
	}
}
