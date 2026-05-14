package agent

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoop_StaysAliveWithEmptyQueue covers watch mode: the loop must
// NOT exit when nextCandidate returns no
// eligible bead. It must reset the per-Run attempted/hookFailed maps and
// sleep, then poll again. Cancelling the context is the only way out.
func TestLoop_StaysAliveWithEmptyQueue(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run on an empty queue")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Use a tiny poll interval so the test runs quickly; cancel after enough
	// wall-clock for several empty-poll cycles to confirm the loop is still
	// running and not bailing out after the first empty poll.
	ctx, cancel := context.WithCancel(context.Background())
	pollInterval := 5 * time.Millisecond
	cancelAfter := 50 * time.Millisecond
	go func() {
		time.Sleep(cancelAfter)
		cancel()
	}()

	start := time.Now()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: pollInterval,
	})
	elapsed := time.Since(start)

	// Context cancellation surfaces as the returned error. The key
	// invariant: the loop survived past the first empty poll.
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)
	assert.True(t, elapsed >= cancelAfter,
		"loop must run until ctx cancellation; ran for %s, expected >= %s", elapsed, cancelAfter)
}

func TestLoop_BinaryRefreshStopsBeforeClaim(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run after binary refresh requests a restart")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	checks := 0
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
		BinaryRefreshCheck: func(ctx context.Context) (bool, error) {
			checks++
			return true, nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, checks)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, "BinaryRefresh", result.StopCondition)
	assert.Equal(t, "binary_refresh", result.ExitReason)
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}

func TestWorkWatch_CheckpointDirtyReleasesClaimWithoutFailure(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			cancel()
			return ExecuteBeadReport{}, fmt.Errorf("pre-execute-bead checkpoint: checkpoint refused to absorb implementation changes outside DDx bookkeeping: cli/cmd/execute_loop_shared.go; commit those files in the bead's [ddx-<id>] substantive commit before rerunning")
		}),
	}

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		Log:          &logBuf,
		WakeCh:       make(chan struct{}),
	})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, 0, result.Failures)
	assert.Empty(t, result.Results)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)

	out := logBuf.String()
	assert.Contains(t, out, "watch: repo has uncommitted implementation changes; released ddx-0001")
	assert.NotContains(t, out, "failed:")
}

func TestWorkWatchIdleStdout_PrintsQueueStatusAndHumanBlockers(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	seedWatchIdleQueue(t, store)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run on an idle queue")
			return ExecuteBeadReport{}, nil
		}),
		Now: fixedWatchStdoutTime,
	}

	ctx, cancel := context.WithCancel(context.Background())
	progressCh := make(chan ProgressEvent, 16)
	done := make(chan struct{})
	var idleEvents int32
	go func() {
		for {
			select {
			case evt := <-progressCh:
				if evt.Phase == "loop.idle" {
					atomic.AddInt32(&idleEvents, 1)
					cancel()
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: 30 * time.Second,
		Log:          &logBuf,
		ProgressCh:   progressCh,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.NotNil(t, result.QueueSnapshot)
	assert.Equal(t, 3, result.QueueSnapshot.HumanReviewBlockerCount)
	assert.Equal(t, 30, result.QueueSnapshot.HumanReviewBlockedTotal)
	require.Len(t, result.QueueSnapshot.HumanReviewBlockers, 3)
	assert.Equal(t, int32(1), atomic.LoadInt32(&idleEvents), "watch mode must still emit loop.idle progress")

	out := logBuf.String()
	assert.Contains(t, out, "12:34:56 idle: no execution-ready beads; sleeping 30s")
	assert.Contains(t, out, "queue: execution-ready=0")
	assert.Contains(t, out, "operator-attention=4") // 3 proposed blockers + 1 standalone proposed bead
	assert.Contains(t, out, "needs-human/investigation=3")
	assert.Contains(t, out, "cooldown/deferred=1")
	assert.Contains(t, out, "next-retry=")
	assert.Contains(t, out, "execution-ineligible=1")
	assert.Contains(t, out, "superseded=1")
	assert.Contains(t, out, "epics=1")
	assert.Contains(t, out, "epic-closure-candidates=1")
	assert.Contains(t, out, "30 beads blocked behind 3 needs-human blockers")
	assert.Contains(t, out, "1. ddx-human-1 Needs human 1 (10 downstream)")
	assert.Contains(t, out, "2. ddx-human-2 Needs human 2 (10 downstream)")
	assert.Contains(t, out, "3. ddx-human-3 Needs human 3 (10 downstream)")
}

func TestWorkWatchIdleStdout_RepeatedPollKeepsCompactQueueStatus(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	seedWatchIdleQueue(t, store)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run on an idle queue")
			return ExecuteBeadReport{}, nil
		}),
		Now: fixedWatchStdoutTime,
	}

	ctx, cancel := context.WithCancel(context.Background())
	wakeCh := make(chan struct{}, 1)
	progressCh := make(chan ProgressEvent, 16)
	done := make(chan struct{})
	var idleEvents int32
	go func() {
		for {
			select {
			case evt := <-progressCh:
				if evt.Phase != "loop.idle" {
					continue
				}
				if atomic.AddInt32(&idleEvents, 1) == 1 {
					wakeCh <- struct{}{}
				} else {
					cancel()
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		Log:          &logBuf,
		ProgressCh:   progressCh,
		WakeCh:       wakeCh,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, int32(2), atomic.LoadInt32(&idleEvents))

	out := logBuf.String()
	assert.Equal(t, 2, strings.Count(out, "idle: no execution-ready beads; sleeping 1h0m0s"))
	assert.Equal(t, 2, strings.Count(out, "queue: execution-ready=0"))
	assert.Equal(t, 2, strings.Count(out, "30 beads blocked behind 3 needs-human blockers"))
	assert.Equal(t, 1, strings.Count(out, "ddx-human-1 Needs human 1 (10 downstream)"))
	assert.Equal(t, 1, strings.Count(out, "ddx-human-2 Needs human 2 (10 downstream)"))
	assert.Equal(t, 1, strings.Count(out, "ddx-human-3 Needs human 3 (10 downstream)"))
}

func TestWorkWatchStdout_PrintsNextReadyTransitionAfterIdle(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	ctx, cancel := context.WithCancel(context.Background())
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			cancel()
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-after-idle",
				ResultRev: "feedface",
			}, nil
		}),
		Now: fixedWatchStdoutTime,
	}

	wakeCh := make(chan struct{}, 1)
	progressCh := make(chan ProgressEvent, 16)

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Run the worker in a goroutine so the main goroutine can process progress
	// events synchronously, eliminating goroutine-scheduling races under load.
	type workerResult struct {
		result *ExecuteBeadLoopResult
		err    error
	}
	workerDone := make(chan workerResult, 1)
	go func() {
		r, e := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:         executeloop.ModeWatch,
			IdleInterval: time.Hour,
			Log:          &logBuf,
			ProgressCh:   progressCh,
			WakeCh:       wakeCh,
		})
		workerDone <- workerResult{r, e}
	}()

	// Deterministic: on loop.idle, create the bead and send the wake signal
	// immediately from the main goroutine with no scheduling gap.
	var activeEvents int32
	var beadCreateErr error
	var wr workerResult
	idleHandled := false
	finished := false
	for !finished {
		select {
		case evt := <-progressCh:
			switch evt.Phase {
			case "loop.idle":
				if !idleHandled {
					idleHandled = true
					beadCreateErr = store.Create(&bead.Bead{
						ID:       "ddx-ready-after-idle",
						Title:    "spec: define full DDx temp cleanup in work cycle",
						Priority: 0,
					})
					wakeCh <- struct{}{}
				}
			case "loop.active":
				atomic.AddInt32(&activeEvents, 1)
			}
		case wr = <-workerDone:
			finished = true
		}
	}
	// Drain events emitted just before the worker signalled done.
drainLoop:
	for {
		select {
		case evt := <-progressCh:
			if evt.Phase == "loop.active" {
				atomic.AddInt32(&activeEvents, 1)
			}
		default:
			break drainLoop
		}
	}

	require.NoError(t, beadCreateErr)
	require.ErrorIs(t, wr.err, context.Canceled)
	require.NotNil(t, wr.result)
	assert.Equal(t, int32(1), atomic.LoadInt32(&activeEvents), "watch mode must still emit loop.active progress")

	out := logBuf.String()
	transition := "taking next ready bead from queue: ddx-ready-after-idle — spec: define full DDx temp cleanup in work cycle"
	header := "▶ ddx-ready-after-idle: spec: define full DDx temp cleanup in work cycle"
	assert.Contains(t, out, "\n12:34:56 "+transition+"\n")
	assert.Contains(t, out, header)
	assert.Less(t, strings.Index(out, transition), strings.Index(out, header))
}

// TestDrain_RoutingPreflightRunsOnce covers the bootstrap behavior required
// by ddx-848069a3: routing preflight runs once before the drain loop starts
// and does not repeat as additional beads are processed.
func TestDrain_RoutingPreflightRunsOnce(t *testing.T) {
	inner, first, second := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var executed []string
	var preflightCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-after-preflight",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "claude", Model: "gpt-5"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
		RoutePreflight: func(ctx context.Context, harness, model string) error {
			atomic.AddInt32(&preflightCalls, 1)
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&preflightCalls), "preflight must run once at startup")
	assert.Equal(t, int32(2), atomic.LoadInt32(&store.claimCalls), "both ready beads must still be claimed")
	require.Len(t, executed, 2, "both ready beads must execute")
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 2, result.Successes)

	gotFirst, err := inner.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotFirst.Status)
	gotSecond, err := inner.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotSecond.Status)
}

// TestLoop_OnceFlagStillExits covers ddx-dc157075 back-compat: --once must
// still terminate the loop after exactly one ready bead is processed, even
// when more beads are queue-ready. This guards against an over-eager fix
// that drops the Once gate.
func TestLoop_OnceFlagStillExits(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-once",
				ResultRev: "cafebabe",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Even in watch-capable runtimes, once mode must still cause the loop to
	// exit after one bead.
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeOnce,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, executed, 1, "--once must process exactly one bead")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
}

// TestLoop_ExplicitDrainExits covers drain-and-exit semantics: an empty queue
// is terminal unless watch mode is selected.
func TestLoop_ExplicitDrainExits(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run when queue is empty")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Drain mode exits when the queue is empty rather than idling. A
	// bounded-time context guards against a regression that would hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	elapsed := time.Since(start)

	require.NoError(t, err, "drain mode must return cleanly without timing out")
	require.NotNil(t, result)
	assert.True(t, elapsed < time.Second,
		"explicit poll=0 must exit promptly when queue is empty; took %s", elapsed)
	assert.True(t, result.NoReadyWork)
	assert.Equal(t, 0, result.Attempts)
}

func fixedWatchStdoutTime() time.Time {
	return time.Date(2026, 5, 9, 12, 34, 56, 789000000, time.UTC)
}

func seedWatchIdleQueue(t *testing.T, store *bead.Store) {
	t.Helper()

	upstream := &bead.Bead{ID: "ddx-upstream", Title: "External blocker", Priority: 0}
	require.NoError(t, store.Create(upstream))
	require.NoError(t, store.UpdateWithLifecycleStatus(upstream.ID, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: "waiting for upstream",
		Reason:                "test fixture",
		Source:                "test",
	}, nil))

	for i := 1; i <= 3; i++ {
		blocker := &bead.Bead{
			ID:       fmt.Sprintf("ddx-human-%d", i),
			Title:    fmt.Sprintf("Needs human %d", i),
			Priority: i,
		}
		blocker.AddDep(upstream.ID, "blocks")
		require.NoError(t, store.Create(blocker))
		require.NoError(t, store.UpdateWithLifecycleStatus(blocker.ID, bead.StatusProposed, bead.LifecycleTransitionOptions{
			OperatorRequired: true,
			Reason:           "test fixture: operator attention required",
			Source:           "test",
		}, nil))

		prevID := blocker.ID
		for n := 1; n <= 10; n++ {
			downstream := &bead.Bead{
				ID:       fmt.Sprintf("ddx-down-%d-%02d", i, n),
				Title:    fmt.Sprintf("Downstream %d %02d", i, n),
				Priority: 4,
			}
			downstream.AddDep(prevID, "blocks")
			require.NoError(t, store.Create(downstream))
			prevID = downstream.ID
		}
	}

	require.NoError(t, store.Create(&bead.Bead{
		ID:       "ddx-proposed",
		Title:    "Proposed operator attention",
		Status:   bead.StatusProposed,
		Priority: 3,
	}))
	require.NoError(t, store.Create(&bead.Bead{
		ID:       "ddx-not-eligible",
		Title:    "Execution ineligible",
		Priority: 4,
		Extra:    map[string]any{bead.ExtraExecutionElig: false},
	}))
	require.NoError(t, store.Create(&bead.Bead{
		ID:       "ddx-superseded",
		Title:    "Superseded",
		Priority: 4,
		Extra:    map[string]any{"superseded-by": "ddx-replacement"},
	}))

	cooldown := &bead.Bead{ID: "ddx-cooldown", Title: "Retry later", Priority: 4}
	require.NoError(t, store.Create(cooldown))
	// Use time.Now().Add so the cooldown is always in the future regardless of when
	// the test runs relative to fixedWatchStdoutTime (which is a fixed past date).
	require.NoError(t, store.SetExecutionCooldown(cooldown.ID, time.Now().Add(24*time.Hour), ExecuteBeadStatusNoChanges, "retry later", ""))

	ordinaryEpic := &bead.Bead{ID: "ddx-epic-open", Title: "Open epic", IssueType: "epic", Priority: 4}
	ordinaryEpicChild := &bead.Bead{ID: "ddx-epic-open-child", Title: "Open epic child", Parent: ordinaryEpic.ID, Priority: 4}
	ordinaryEpicChild.AddDep(upstream.ID, "blocks")
	require.NoError(t, store.Create(ordinaryEpic))
	require.NoError(t, store.Create(ordinaryEpicChild))

	closureEpic := &bead.Bead{ID: "ddx-epic-closure", Title: "Closure epic", IssueType: "epic", Priority: 4}
	closedChild := &bead.Bead{
		ID:       "ddx-epic-closed-child",
		Title:    "Closed epic child",
		Status:   bead.StatusClosed,
		Parent:   closureEpic.ID,
		Priority: 4,
	}
	require.NoError(t, store.Create(closureEpic))
	require.NoError(t, store.Create(closedChild))
}
