package work

import (
	"context"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

// PhaseBudgets configures the progress watchdog's per-phase patience window.
// A worker that emits "phase-empty" heartbeats — harness, model, and route all
// empty — past the applicable budget is treated as wedged: nothing is making
// forward progress even though each heartbeat keeps advancing last_activity_at,
// so the liveness TTL alone reports the worker healthy. This is the 80-minute
// wedge observed in ddx-8f2e0ebf (criterion B), where phase=running heartbeats
// with empty harness/model/route persisted indefinitely.
type PhaseBudgets struct {
	// Resolving bounds how long phase-empty heartbeats may persist before a
	// route is resolved (no harness/model assigned yet) before the watchdog
	// fires.
	Resolving time.Duration
	// Running bounds how long phase-empty heartbeats may persist once the
	// worker has entered the running phase. It is longer than Resolving
	// because a genuinely running attempt holds the phase=running marker for
	// many minutes.
	Running time.Duration
}

// DefaultPhaseBudgets returns the watchdog's documented default budgets:
// 5 minutes while resolving a route and 30 minutes once running.
func DefaultPhaseBudgets() PhaseBudgets {
	return PhaseBudgets{
		Resolving: 5 * time.Minute,
		Running:   30 * time.Minute,
	}
}

// budgetForPhase selects the budget that applies to the given phase. The
// running phase uses the longer Running budget; every other phase (queueing,
// terminal, or empty) uses Resolving.
func (b PhaseBudgets) budgetForPhase(phase string) time.Duration {
	if phase == string(PhaseRunning) {
		return b.Running
	}
	return b.Resolving
}

// phaseEmpty reports the wedge signature: harness, model, and route all empty.
func phaseEmpty(harness, model, route string) bool {
	return strings.TrimSpace(harness) == "" &&
		strings.TrimSpace(model) == "" &&
		strings.TrimSpace(route) == ""
}

// progressWatchdog tracks how long the phase-empty signature has persisted and
// reports when it crosses the phase budget.
type progressWatchdog struct {
	budgets    PhaseBudgets
	emptySince time.Time
	emptyPhase string
}

// observe records one heartbeat snapshot. It reports whether the phase-empty
// signature (harness, model, and route all empty) has now persisted at or
// beyond the applicable phase budget, along with the budget that applied. A
// heartbeat carrying any non-empty route field resets the phase-empty timer:
// the worker made forward progress.
func (w *progressWatchdog) observe(now time.Time, phase, harness, model, route string) (fired bool, budget time.Duration) {
	if !phaseEmpty(harness, model, route) {
		w.emptySince = time.Time{}
		w.emptyPhase = ""
		return false, 0
	}
	if w.emptySince.IsZero() {
		w.emptySince = now
		w.emptyPhase = phase
	}
	budget = w.budgets.budgetForPhase(w.emptyPhase)
	return now.Sub(w.emptySince) >= budget, budget
}

// ProgressWatchdogConfig wires the phase-empty progress watchdog. The watchdog
// runs as a goroutine for the lifetime of an attempt, alongside WithHeartbeat:
// each tick it inspects the liveness snapshot and, when the phase-empty
// signature has persisted past the phase budget, it invokes OnWedged so the
// caller can cancel the attempt, release the lease, and flag the wedge for
// operator attention.
type ProgressWatchdogConfig struct {
	Budgets  PhaseBudgets
	Snapshot func() workerstatus.LivenessRecord
	Now      func() time.Time
	// OnWedged is invoked exactly once, with the snapshot that tripped the
	// watchdog and the budget it exceeded, when the phase-empty signature
	// persists past the budget.
	OnWedged func(rec workerstatus.LivenessRecord, budget time.Duration)
}

// RunProgressWatchdog runs the phase-empty progress watchdog until ctx is
// cancelled or the watchdog fires. On fire it invokes cfg.OnWedged once and
// returns. It is intended to run as a goroutine alongside WithHeartbeat.
func RunProgressWatchdog(ctx context.Context, interval time.Duration, cfg ProgressWatchdogConfig) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	runProgressWatchdog(ctx, ticker.C, cfg)
}

// runProgressWatchdog is the injectable variant used by tests to supply a stub
// tick channel.
func runProgressWatchdog(ctx context.Context, tickCh <-chan time.Time, cfg ProgressWatchdogConfig) {
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	wd := &progressWatchdog{budgets: cfg.Budgets}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tickCh:
			if cfg.Snapshot == nil {
				continue
			}
			rec := cfg.Snapshot()
			fired, budget := wd.observe(nowFn(), rec.Phase, rec.Harness, rec.Model, rec.Route)
			if fired {
				if cfg.OnWedged != nil {
					cfg.OnWedged(rec, budget)
				}
				return
			}
		}
	}
}

// ExternalCloseWatcherConfig wires the external-close watcher. It periodically
// re-reads the claimed bead's status; when the bead has been closed by another
// attempt it invokes OnClosed so the caller can terminate the current attempt
// promptly and release the lease without escalating a failure — the work is
// already done (ddx-8f2e0ebf: a wedged attempt held a lease for 2h+ after a
// parallel attempt landed the same bead).
type ExternalCloseWatcherConfig struct {
	// IsClosed reports whether the claimed bead has transitioned to closed.
	// A read error is treated as best-effort: it is ignored, not a close.
	IsClosed func() (bool, error)
	// OnClosed is invoked exactly once when IsClosed first reports true.
	OnClosed func()
}

// RunExternalCloseWatcher runs the external-close watcher until ctx is cancelled
// or the watched bead is observed closed. On close it invokes cfg.OnClosed once
// and returns. It is intended to run as a goroutine alongside WithHeartbeat.
func RunExternalCloseWatcher(ctx context.Context, interval time.Duration, cfg ExternalCloseWatcherConfig) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	runExternalCloseWatcher(ctx, ticker.C, cfg)
}

// runExternalCloseWatcher is the injectable variant used by tests to supply a
// stub tick channel.
func runExternalCloseWatcher(ctx context.Context, tickCh <-chan time.Time, cfg ExternalCloseWatcherConfig) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tickCh:
			if cfg.IsClosed == nil {
				continue
			}
			closed, err := cfg.IsClosed()
			if err != nil {
				// Best-effort: a transient read error is not a close.
				continue
			}
			if closed {
				if cfg.OnClosed != nil {
					cfg.OnClosed()
				}
				return
			}
		}
	}
}
