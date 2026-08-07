package agent

import (
	"context"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
)

// runPreclaimDecompositionHookWithResolvingLiveness publishes candidate-scoped
// phase=resolving liveness for the duration of the preclaim decomposition
// hook and heartbeats last_activity_at while the provider runs. The sidecar
// uses an empty attempt_id so this model work is not reported as an
// implementation attempt. No bead claim or lease is created here — that
// remains the caller's responsibility.
func runPreclaimDecompositionHookWithResolvingLiveness(
	ctx context.Context,
	hook func(context.Context, string) (*PreClaimDecomposition, error),
	beadID string,
	liveness *work.SidecarLivenessReporter,
	harness, model, profile string,
	timeout time.Duration,
	heartbeatInterval time.Duration,
	now func() time.Time,
) (*PreClaimDecomposition, error) {
	if hook == nil {
		return nil, nil
	}
	if now == nil {
		now = time.Now
	}
	if timeout <= 0 {
		timeout = work.DefaultPreClaimTimeout
	}

	if liveness == nil {
		return hook(ctx, beadID)
	}

	liveness.SetCandidateResolving(beadID, harness, model, profile)
	// Immediate tick so operators see resolving state before the first
	// heartbeat interval elapses.
	liveness.OnTick(now())
	defer func() {
		liveness.ClearAttempt()
		liveness.OnTick(now())
	}()

	if heartbeatInterval <= 0 {
		heartbeatInterval = 5 * time.Millisecond
	}
	stopHB := startLivenessOnlyHeartbeat(ctx, liveness, heartbeatInterval, now)
	defer stopHB()

	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return hook(hookCtx, beadID)
}

// startLivenessOnlyHeartbeat refreshes the worker sidecar on a ticker without
// touching claim leases. Preclaim decomposition must not invent or refresh a
// bead lease merely to report resolving activity.
func startLivenessOnlyHeartbeat(ctx context.Context, liveness *work.SidecarLivenessReporter, interval time.Duration, now func() time.Time) func() {
	if liveness == nil {
		return func() {}
	}
	if now == nil {
		now = time.Now
	}
	hbCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case tick := <-ticker.C:
				// Prefer the ticker's wall time when present; fall back to now().
				at := tick
				if at.IsZero() {
					at = now()
				}
				liveness.OnTick(at)
			}
		}
	}()
	return func() {
		cancel()
		wg.Wait()
	}
}
