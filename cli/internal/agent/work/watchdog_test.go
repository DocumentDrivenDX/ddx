package work

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWatchdogPhaseBudgetDefaults pins the documented default phase budgets so
// a change to the watchdog's patience is a deliberate, reviewed edit (AC #2).
func TestWatchdogPhaseBudgetDefaults(t *testing.T) {
	b := DefaultPhaseBudgets()
	assert.Equal(t, 5*time.Minute, b.Resolving, "default resolving budget must be 5m")
	assert.Equal(t, 30*time.Minute, b.Running, "default running budget must be 30m")

	// The budget selection routes the running phase to the longer window and
	// every other phase to the shorter resolving window.
	assert.Equal(t, b.Running, b.budgetForPhase(string(PhaseRunning)))
	assert.Equal(t, b.Resolving, b.budgetForPhase(string(PhaseQueueing)))
	assert.Equal(t, b.Resolving, b.budgetForPhase(""))
}

// TestProgressWatchdogObserveResetsOnProgress verifies the core observation
// logic: a non-empty route field resets the phase-empty timer, and the
// watchdog only fires once the phase-empty signature has persisted past the
// applicable budget.
func TestProgressWatchdogObserveResetsOnProgress(t *testing.T) {
	wd := &progressWatchdog{budgets: PhaseBudgets{Resolving: time.Minute, Running: 10 * time.Minute}}
	base := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

	// Phase-empty but within budget: no fire.
	fired, budget := wd.observe(base, "running", "", "", "")
	require.False(t, fired)
	require.Equal(t, 10*time.Minute, budget, "phase=running selects the running budget")

	// A heartbeat that carries harness+model is forward progress: reset.
	fired, _ = wd.observe(base.Add(20*time.Second), "running", "claude", "opus", "balanced")
	require.False(t, fired)

	// Back to phase-empty: the timer restarts from this observation, so even
	// well past the original start it has not yet crossed the budget.
	fired, _ = wd.observe(base.Add(30*time.Second), "running", "", "", "")
	require.False(t, fired)

	// Now persist phase-empty past the running budget from the restart point.
	fired, _ = wd.observe(base.Add(30*time.Second+11*time.Minute), "running", "", "", "")
	require.True(t, fired, "phase-empty past the running budget must fire")
}

// TestRunProgressWatchdogFiresOnPhaseEmpty drives empty-field snapshots through
// the runner for budget+epsilon and asserts OnWedged fires once with the
// snapshot and budget that tripped it.
func TestRunProgressWatchdogFiresOnPhaseEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tickCh := make(chan time.Time)
	base := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	var clockNanos atomic.Int64
	clockNanos.Store(base.UnixNano())
	nowFn := func() time.Time { return time.Unix(0, clockNanos.Load()).UTC() }

	var wedged workerstatus.LivenessRecord
	var firedBudget time.Duration
	fireCount := 0
	done := make(chan struct{})

	go func() {
		runProgressWatchdog(ctx, tickCh, ProgressWatchdogConfig{
			Budgets: PhaseBudgets{Resolving: time.Minute, Running: time.Minute},
			Now:     nowFn,
			Snapshot: func() workerstatus.LivenessRecord {
				return workerstatus.LivenessRecord{
					CurrentBead:    "ddx-wedged",
					AttemptID:      "att-wedged-001",
					Phase:          string(PhaseRunning),
					LastActivityAt: nowFn(),
				}
			},
			OnWedged: func(rec workerstatus.LivenessRecord, budget time.Duration) {
				// Reads here are published to the main goroutine by the
				// close(done) below, which happens-after this callback returns.
				fireCount++
				wedged = rec
				firedBudget = budget
			},
		})
		close(done)
	}()

	// First phase-empty tick at t0: starts the timer, does not fire.
	tickCh <- nowFn()
	// Advance the clock past the budget, then tick again: budget+epsilon.
	clockNanos.Store(base.Add(time.Minute + time.Second).UnixNano())
	tickCh <- nowFn()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("progress watchdog did not fire on persistent phase-empty heartbeats")
	}

	assert.Equal(t, 1, fireCount, "OnWedged must fire exactly once")
	assert.Equal(t, "ddx-wedged", wedged.CurrentBead)
	assert.Equal(t, "att-wedged-001", wedged.AttemptID)
	assert.Equal(t, time.Minute, firedBudget)
}

// TestRunExternalCloseWatcherFiresOnClose drives the watcher with a status
// reader that flips to closed and asserts OnClosed fires once and the watcher
// returns.
func TestRunExternalCloseWatcherFiresOnClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tickCh := make(chan time.Time)
	var closedNow atomic.Bool
	var closeCount atomic.Int32
	done := make(chan struct{})

	go func() {
		runExternalCloseWatcher(ctx, tickCh, ExternalCloseWatcherConfig{
			IsClosed: func() (bool, error) { return closedNow.Load(), nil },
			OnClosed: func() { closeCount.Add(1) },
		})
		close(done)
	}()

	// Not closed yet: watcher keeps polling.
	tickCh <- time.Now()
	// Another attempt closes the bead; next tick observes it.
	closedNow.Store(true)
	tickCh <- time.Now()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("external-close watcher did not fire when the bead transitioned to closed")
	}
	assert.Equal(t, int32(1), closeCount.Load(), "OnClosed must fire exactly once")
}
