package agent

import (
	"bytes"
	"context"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type loadPressureClaimSpyStore struct {
	*bead.Store
	claimCalls            int32
	claimWithOptionsCalls int32
}

func (s *loadPressureClaimSpyStore) Claim(id, assignee string) error {
	atomic.AddInt32(&s.claimCalls, 1)
	return s.Store.Claim(id, assignee)
}

func (s *loadPressureClaimSpyStore) ClaimWithOptions(id, assignee, session, worktree string) error {
	atomic.AddInt32(&s.claimWithOptionsCalls, 1)
	return s.Store.ClaimWithOptions(id, assignee, session, worktree)
}

type loadPressureLoopRunResult struct {
	result *ExecuteBeadLoopResult
	err    error
}

func overloadedLoadPressureSnapshot() workerstatus.LoadPressureSnapshot {
	return workerstatus.LoadPressureSnapshot{
		Load5:           24,
		CPUCount:        8,
		NormalizedRatio: 3,
		Supported:       true,
		Available:       true,
		Overloaded:      true,
	}
}

func testLoopResolvedConfig() config.ResolvedConfig {
	opts := config.TestLoopConfigOpts{Assignee: "worker"}
	return config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
}

func TestWorkLoop_LoadPressureBackoffBeforeClaim(t *testing.T) {
	inner, first, _ := newExecuteLoopTestStore(t)
	store := &loadPressureClaimSpyStore{Store: inner}
	type delayObservation struct {
		delay        time.Duration
		eventEmitted bool
	}
	delayStarted := make(chan delayObservation, 1)
	releaseDelay := make(chan struct{})
	var executorCalls int32

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&executorCalls, 1)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "load-pressure", ResultRev: "abc123"}, nil
		}),
	}
	var events bytes.Buffer
	done := make(chan loadPressureLoopRunResult, 1)
	go func() {
		result, err := worker.Run(context.Background(), testLoopResolvedConfig(), ExecuteBeadLoopRuntime{
			Once:                 true,
			EventSink:            &events,
			SessionID:            "load-pressure-backoff",
			LoadPressureSnapshot: overloadedLoadPressureSnapshot,
			LoadPressureSleeper: func(_ context.Context, delay time.Duration) error {
				delayStarted <- delayObservation{
					delay:        delay,
					eventEmitted: strings.Contains(events.String(), `"type":"loop.load_pressure_backoff"`),
				}
				<-releaseDelay
				return nil
			},
		})
		done <- loadPressureLoopRunResult{result: result, err: err}
	}()

	var observation delayObservation
	select {
	case observation = <-delayStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("load-pressure delay did not start")
	}
	assert.True(t, observation.eventEmitted, "backoff telemetry must be emitted before the sleeper starts")
	assert.Zero(t, atomic.LoadInt32(&store.claimCalls), "Claim must not occur until the load-pressure delay completes")
	assert.Zero(t, atomic.LoadInt32(&store.claimWithOptionsCalls), "ClaimWithOptions must not occur until the load-pressure delay completes")
	assert.Zero(t, atomic.LoadInt32(&executorCalls), "executor must not start until the load-pressure delay completes")
	close(releaseDelay)

	completed := <-done
	require.NoError(t, completed.err)
	require.NotNil(t, completed.result)
	assert.Equal(t, 1, completed.result.Attempts)
	assert.Zero(t, atomic.LoadInt32(&store.claimCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimWithOptionsCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&executorCalls))
	assert.Equal(t, 6*time.Second, observation.delay)

	byType := decodeLoopEventsByType(t, &events)
	require.Len(t, byType["loop.load_pressure_backoff"], 1)
	backoff := byType["loop.load_pressure_backoff"][0]
	assert.Equal(t, first.ID, backoff["bead_id"])
	assert.Equal(t, float64(24), backoff["load5"])
	assert.Equal(t, float64(8), backoff["cpu_count"])
	assert.Equal(t, float64(3), backoff["normalized_ratio"])
	assert.Equal(t, workerstatus.DefaultLoadPressureThreshold, backoff["threshold"])
	assert.Equal(t, true, backoff["supported"])
	assert.Equal(t, true, backoff["available"])
	assert.Equal(t, "6s", backoff["delay"])
	assert.Equal(t, float64(6000), backoff["delay_ms"])
	assert.Equal(t, "available", backoff["source_state"])
}

func TestWorkLoop_LoadPressureBackoffCancellation(t *testing.T) {
	inner, first, _ := newExecuteLoopTestStore(t)
	store := &loadPressureClaimSpyStore{Store: inner}
	delayStarted := make(chan struct{})
	var executorCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&executorCalls, 1)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess}, nil
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan loadPressureLoopRunResult, 1)
	go func() {
		result, err := worker.Run(ctx, testLoopResolvedConfig(), ExecuteBeadLoopRuntime{
			Once:                 true,
			LoadPressureSnapshot: overloadedLoadPressureSnapshot,
			LoadPressureSleeper: func(ctx context.Context, _ time.Duration) error {
				close(delayStarted)
				<-ctx.Done()
				return ctx.Err()
			},
		})
		done <- loadPressureLoopRunResult{result: result, err: err}
	}()

	select {
	case <-delayStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("load-pressure delay did not start")
	}
	cancel()
	select {
	case completed := <-done:
		require.ErrorIs(t, completed.err, context.Canceled)
		require.NotNil(t, completed.result)
		assert.Zero(t, completed.result.Attempts)
		assert.Empty(t, completed.result.Results)
	case <-time.After(time.Second):
		t.Fatal("worker did not exit promptly after cancellation")
	}

	assert.Zero(t, atomic.LoadInt32(&store.claimCalls))
	assert.Zero(t, atomic.LoadInt32(&store.claimWithOptionsCalls))
	assert.Zero(t, atomic.LoadInt32(&executorCalls))
	got, err := inner.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.NotContains(t, got.Extra, bead.ExtraRetryAfter)
	assert.NotContains(t, got.Extra, bead.ExtraCooldownBaseRev)
	assert.NotContains(t, got.Extra, bead.ExtraLastStatus)
	assert.NotContains(t, got.Extra, bead.ExtraLastDetail)
	_, found, err := inner.ClaimLease(first.ID)
	require.NoError(t, err)
	assert.False(t, found, "cancellation before claim must not create a lease")
}

func TestWorkLoop_LoadPressureUnavailableFailsOpen(t *testing.T) {
	tests := []struct {
		name     string
		snapshot workerstatus.LoadPressureSnapshot
	}{
		{
			name: "unavailable",
			snapshot: workerstatus.LoadPressureSnapshot{
				Supported:  true,
				Diagnostic: "read /proc/loadavg: unavailable",
			},
		},
		{
			name: "unsupported",
			snapshot: workerstatus.LoadPressureSnapshot{
				Diagnostic: "load pressure unsupported on test platform",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner, _, _ := newExecuteLoopTestStore(t)
			store := &loadPressureClaimSpyStore{Store: inner}
			var sleepCalls int32
			var executorCalls int32
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
					atomic.AddInt32(&executorCalls, 1)
					return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "load-pressure-open", ResultRev: "abc123"}, nil
				}),
			}
			var events bytes.Buffer
			result, err := worker.Run(context.Background(), testLoopResolvedConfig(), ExecuteBeadLoopRuntime{
				Once:      true,
				EventSink: &events,
				LoadPressureSnapshot: func() workerstatus.LoadPressureSnapshot {
					return tc.snapshot
				},
				LoadPressureSleeper: func(context.Context, time.Duration) error {
					atomic.AddInt32(&sleepCalls, 1)
					return nil
				},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, 1, result.Attempts)
			assert.Zero(t, atomic.LoadInt32(&store.claimCalls))
			assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimWithOptionsCalls))
			assert.Equal(t, int32(1), atomic.LoadInt32(&executorCalls))
			assert.Zero(t, atomic.LoadInt32(&sleepCalls), "unavailable load pressure must fail open without sleeping")

			byType := decodeLoopEventsByType(t, &events)
			require.Len(t, byType["loop.load_pressure_unavailable"], 1)
			unavailable := byType["loop.load_pressure_unavailable"][0]
			assert.Contains(t, unavailable["diagnostic"], tc.snapshot.Diagnostic)
			assert.Equal(t, tc.name, unavailable["source_state"])
			assert.Empty(t, byType["loop.load_pressure_backoff"])
		})
	}
}

func TestLoadPressureBackoffDelay_BoundsAndStrictThreshold(t *testing.T) {
	tests := []struct {
		name      string
		ratio     float64
		threshold float64
		want      time.Duration
		justAbove bool
	}{
		{name: "below threshold", ratio: 2, threshold: 2.5, want: 0},
		{name: "equal threshold does not back off", ratio: 2.5, threshold: 2.5, want: 0},
		{name: "just above threshold", ratio: 2.500000001, threshold: 2.5, justAbove: true},
		{name: "proportional delay", ratio: 3, threshold: 2.5, want: 6 * time.Second},
		{name: "maximum bound", ratio: 100, threshold: 2.5, want: defaultLoadPressureBackoffMax},
		{name: "tiny positive threshold clamps before duration conversion", ratio: 3, threshold: math.SmallestNonzeroFloat64, want: defaultLoadPressureBackoffMax},
		{name: "infinite ratio clamps before duration conversion", ratio: math.Inf(1), threshold: 2.5, want: defaultLoadPressureBackoffMax},
		{name: "invalid threshold", ratio: 3, threshold: 0, want: 0},
		{name: "NaN threshold", ratio: 3, threshold: math.NaN(), want: 0},
		{name: "infinite threshold", ratio: 3, threshold: math.Inf(1), want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := loadPressureBackoffDelay(workerstatus.LoadPressureSnapshot{
				NormalizedRatio: tc.ratio,
				Threshold:       tc.threshold,
			})
			if tc.justAbove {
				assert.GreaterOrEqual(t, got, defaultLoadPressureBackoffBase)
				assert.Less(t, got, defaultLoadPressureBackoffBase+time.Millisecond)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExecuteBeadLoopRuntime_EffectiveLoadPressureThresholdRejectsNonFinite(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		want      float64
	}{
		{name: "zero", threshold: 0, want: workerstatus.DefaultLoadPressureThreshold},
		{name: "NaN", threshold: math.NaN(), want: workerstatus.DefaultLoadPressureThreshold},
		{name: "positive infinity", threshold: math.Inf(1), want: workerstatus.DefaultLoadPressureThreshold},
		{name: "negative infinity", threshold: math.Inf(-1), want: workerstatus.DefaultLoadPressureThreshold},
		{name: "configured", threshold: 3.25, want: 3.25},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := (ExecuteBeadLoopRuntime{LoadPressureThreshold: tc.threshold}).effectiveLoadPressureThreshold()
			assert.Equal(t, tc.want, got)
		})
	}
}
