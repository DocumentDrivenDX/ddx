package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoop_StaysAliveWithEmptyQueue covers ddx-dc157075 AC #2: with
// poll-interval > 0, the loop must NOT exit when nextCandidate returns no
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
		PollInterval: pollInterval,
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

// TestLoop_PreflightRejectionContinues covers ddx-dc157075 AC #3: when the
// routing preflight rejects the first ready bead, the loop must NOT exit —
// it must record the worker-level failure and proceed to the next ready bead.
// Without the fix the loop returns early after the first preflight failure
// and the second bead is silently abandoned.
func TestLoop_PreflightRejectionContinues(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-after-reject",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	// Preflight rejects only the first bead; the second is allowed through.
	var preflightCalls int32
	preflight := func(ctx context.Context, harness, model string) error {
		n := atomic.AddInt32(&preflightCalls, 1)
		if n == 1 {
			return agentlib.ErrHarnessModelIncompatible{
				Harness:         harness,
				Model:           model,
				SupportedModels: []string{"x"},
			}
		}
		return nil
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "claude", Model: "gpt-5"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// PollInterval=0 so the loop drains current ready work and exits when no
	// more candidates remain — we just need to prove it didn't bail on bead #1.
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		RoutePreflight: preflight,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Both beads were considered; the second was actually executed.
	assert.Equal(t, int32(2), atomic.LoadInt32(&preflightCalls),
		"preflight must be invoked for both candidates (rejection of #1 cannot abandon #2)")
	require.Len(t, executed, 1, "second bead must run once preflight passes for it")
	assert.Equal(t, second.ID, executed[0])

	// First bead is recorded as a worker-level failure.
	require.GreaterOrEqual(t, len(result.Results), 2)
	var firstReport, secondReport *ExecuteBeadReport
	for i := range result.Results {
		switch result.Results[i].BeadID {
		case first.ID:
			firstReport = &result.Results[i]
		case second.ID:
			secondReport = &result.Results[i]
		}
	}
	require.NotNil(t, firstReport, "first bead's failure record present")
	require.NotNil(t, secondReport, "second bead's success record present")
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, firstReport.Status)
	assert.True(t, strings.Contains(firstReport.Detail, "preflight"),
		"first bead failure detail must mention preflight; got %q", firstReport.Detail)
	assert.Equal(t, ExecuteBeadStatusSuccess, secondReport.Status)

	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Failures)

	// First bead must remain open (no claim, no executor invocation).
	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
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

	// Even with poll-interval > 0 (the new long-running default), --once must
	// still cause the loop to exit after one bead.
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		PollInterval: 30 * time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, executed, 1, "--once must process exactly one bead")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
}

// TestLoop_ExplicitPollZeroExits covers ddx-dc157075 back-compat: an explicit
// --poll-interval=0 (legacy "drain-and-exit" semantics) must still exit when
// the queue is empty, even though the new default is 30s.
func TestLoop_ExplicitPollZeroExits(t *testing.T) {
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

	// PollInterval=0 explicitly: exit when queue is empty rather than poll.
	// A bounded-time context guards against a regression that would hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		PollInterval: 0,
	})
	elapsed := time.Since(start)

	require.NoError(t, err, "explicit poll-interval=0 must return cleanly without timing out")
	require.NotNil(t, result)
	assert.True(t, elapsed < time.Second,
		"explicit poll=0 must exit promptly when queue is empty; took %s", elapsed)
	assert.True(t, result.NoReadyWork)
	assert.Equal(t, 0, result.Attempts)
}
