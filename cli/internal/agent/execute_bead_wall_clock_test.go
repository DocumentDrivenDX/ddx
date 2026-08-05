package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wallClockAttemptRecorder struct {
	mu       sync.Mutex
	attempts []string
}

func (r *wallClockAttemptRecorder) add(beadID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts = append(r.attempts, beadID)
}

func (r *wallClockAttemptRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.attempts...)
}

func wallClockBlockingExecutor(blockBeadID string, recorder *wallClockAttemptRecorder) ExecuteBeadExecutorFunc {
	return ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		recorder.add(beadID)
		if beadID == blockBeadID {
			if fn := onExecuteStartFromContext(ctx); fn != nil {
				fn()
			}
			if fn := onRouteResolvedFromContext(ctx); fn != nil {
				fn("route-harness", "route-provider", "route-model")
			}
			<-ctx.Done()
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: ctx.Err().Error(),
			}, ctx.Err()
		}
		return ExecuteBeadReport{
			BeadID: beadID,
			Status: ExecuteBeadStatusSuccess,
			Detail: "completed",
		}, nil
	})
}

func TestWorkAttemptWallClock_PopulatedRouteCannotEvadeDeadline(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	recorder := &wallClockAttemptRecorder{}

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: wallClockBlockingExecutor(first.ID, recorder),
	}
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:          "worker-a",
		HeartbeatInterval: 5 * time.Millisecond,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Once:                true,
		NoReview:            true,
		AttemptWallClock:    25 * time.Millisecond,
		AttemptWallClockSet: true,
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		require.NoError(t, err)
	}
	require.NotNil(t, result)

	require.Eventually(t, func() bool {
		return len(recorder.snapshot()) >= 1
	}, 2*time.Second, 5*time.Millisecond, "executor never started")
	assert.Equal(t, first.ID, recorder.snapshot()[0], "the claimed bead should enter the attempt before the wall-clock deadline fires")

	released, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, released.Status, "wall-clock expiry must release the lease")
	assert.Empty(t, released.Owner, "wall-clock expiry must clear the owner")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var wallClockEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "operator_attention" && events[i].Summary == FailureModeAttemptWallClockTimeout {
			wallClockEvent = &events[i]
			break
		}
	}
	require.NotNil(t, wallClockEvent, "attempt wall-clock expiry must emit operator_attention")
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(wallClockEvent.Body), &body))
	assert.Equal(t, first.ID, body["bead_id"])
	assert.NotEmpty(t, body["attempt_id"])
	assert.Equal(t, "25ms", body["budget"])
	assert.NotEmpty(t, body["elapsed"])
	assert.NotEmpty(t, body["last_activity_at"])
	assert.Equal(t, FailureModeAttemptWallClockTimeout, body["reason"])

	for _, event := range events {
		assert.NotEqual(t, FailureModeProgressWatchdog, event.Summary, "resolved route activity must not trip the phase-empty watchdog")
	}
}

func TestWorkAttemptWallClock_ReleasesLeaseAndContinuesQueue(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	recorder := &wallClockAttemptRecorder{}

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: wallClockBlockingExecutor(first.ID, recorder),
	}
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:          "worker-a",
		HeartbeatInterval: 5 * time.Millisecond,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:                executeloop.ModeDrain,
			NoReview:            true,
			AttemptWallClock:    25 * time.Millisecond,
			AttemptWallClockSet: true,
		})
		done <- err
	}()

	var seen []string
	require.Eventually(t, func() bool {
		seen = recorder.snapshot()
		return len(seen) >= 1
	}, 2*time.Second, 5*time.Millisecond, "first attempt never started")

	require.Eventually(t, func() bool {
		seen = recorder.snapshot()
		return len(seen) >= 2
	}, 2*time.Second, 5*time.Millisecond, "queue did not continue after wall-clock expiry")

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			require.NoError(t, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after the queue drained")
	}

	require.Len(t, seen, 2, "wall-clock expiry should free the lease and allow the next bead to claim")
	assert.Equal(t, first.ID, seen[0])
	assert.Equal(t, second.ID, seen[1])

	firstAfter, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, firstAfter.Status, "the timed-out bead must be released back to open")
	assert.Empty(t, firstAfter.Owner)

	secondAfter, err := store.Get(context.Background(), second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, secondAfter.Status, "the next ready bead must still complete normally")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var wallClockEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "operator_attention" && events[i].Summary == FailureModeAttemptWallClockTimeout {
			wallClockEvent = &events[i]
			break
		}
	}
	require.NotNil(t, wallClockEvent, "timed-out bead must record operator attention")
}
