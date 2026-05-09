package agent

import (
	"context"
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
