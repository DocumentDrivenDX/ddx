package agent

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

// runPreclaimDecompositionHookWithResolvingLiveness publishes candidate-scoped
// phase=resolving liveness for the duration of the preclaim decomposition
// hook, heartbeats last_activity_at while the provider runs, and attaches
// provider-child metadata that excludes processes present in the pre-hook
// baseline. The sidecar uses an empty attempt_id so this model work is not
// reported as an implementation attempt. No bead claim or lease is created
// here — that remains the caller's responsibility.
func runPreclaimDecompositionHookWithResolvingLiveness(
	ctx context.Context,
	hook func(context.Context, string) (*PreClaimDecomposition, error),
	beadID string,
	liveness *work.SidecarLivenessReporter,
	harness, model, profile string,
	heartbeatInterval time.Duration,
	now func() time.Time,
) (*PreClaimDecomposition, error) {
	if hook == nil {
		return nil, nil
	}
	if now == nil {
		now = time.Now
	}
	if liveness == nil {
		return hook(ctx, beadID)
	}

	workerPID := os.Getpid()
	phase := string(work.PhaseResolving)
	baselineAt := now().UTC()
	baselineChildren := scanProviderChildrenForStatus(context.Background(), workerPID, "", harness, phase, baselineAt)
	baseline := providerChildPIDSet(baselineChildren)

	// Capture and temporarily replace the child probe so candidate metadata
	// only reports provider descendants introduced after the hook baseline
	// under this worker. Processes belonging to other workers are already
	// excluded by the rootPID-scoped scanner.
	prevProbe := liveness.SwapChildProbe(func(route, harnessName, probePhase string) []workerstatus.ProviderChild {
		children := scanProviderChildrenForStatus(context.Background(), workerPID, route, harnessName, probePhase, time.Now().UTC())
		return filterProviderChildrenAfterBaseline(children, baseline)
	})
	defer liveness.SwapChildProbe(prevProbe)

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

	return hook(ctx, beadID)
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
