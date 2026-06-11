package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type singleTouchHeartbeatStore struct {
	*bead.Store
	touched atomic.Uint32
}

func (s *singleTouchHeartbeatStore) TouchClaimHeartbeat(id string) error {
	if s.touched.CompareAndSwap(0, 1) {
		return s.Store.TouchClaimHeartbeat(id)
	}
	return nil
}

type singleScanReadyStore struct {
	*bead.Store
	readyCalls atomic.Uint32
}

func (s *singleScanReadyStore) ReadyExecution() ([]bead.Bead, error) {
	if s.readyCalls.Add(1) == 1 {
		return s.Store.ReadyExecution()
	}
	return nil, nil
}

func (s *singleScanReadyStore) ReadyExecutionBreakdown() (bead.ReadyExecutionBreakdown, error) {
	return s.Store.ReadyExecutionBreakdown()
}

func currentMachineIDForTest() string {
	if envID := os.Getenv("DDX_MACHINE_ID"); envID != "" {
		return envID
	}
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}

// TestChaos_StaleButAliveLeaseDoesNotPermitSecondExecution exercises the
// combined intake-heartbeat + live-same-machine-PID takeover contract.
// If ddx-d333ad8a is reverted, the first worker's claim heartbeat never
// refreshes while intake is blocked. If ddx-ba93de9e is reverted, the second
// worker can reclaim once the lease is stale even though the original PID is
// still alive on the same machine.
func TestChaos_StaleButAliveLeaseDoesNotPermitSecondExecution(t *testing.T) {
	withPreClaimHeartbeatTiming(t, 40*time.Millisecond, 120*time.Millisecond)

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	beadID := "ddx-stale-but-alive"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:       beadID,
		Title:    "stale but alive lease",
		Priority: 0,
	}))

	workerAStore := &singleTouchHeartbeatStore{Store: store}
	workerBStore := &singleScanReadyStore{Store: store}

	cfgAOpts := config.TestLoopConfigOpts{
		Assignee:          "worker-a",
		HeartbeatInterval: 40 * time.Millisecond,
	}
	rcfgA := config.NewTestConfigForLoop(cfgAOpts).Resolve(config.TestLoopOverrides(cfgAOpts))

	cfgBOpts := config.TestLoopConfigOpts{
		Assignee:          "worker-b",
		HeartbeatInterval: 40 * time.Millisecond,
	}
	rcfgB := config.NewTestConfigForLoop(cfgBOpts).Resolve(config.TestLoopOverrides(cfgBOpts))

	var (
		aExecCalls atomic.Int32
		bExecCalls atomic.Int32
		aSink      bytes.Buffer
		bSink      bytes.Buffer
	)

	aStarted := make(chan struct{})
	releaseA := make(chan struct{})
	var releaseAOnce sync.Once
	defer func() {
		releaseAOnce.Do(func() {
			close(releaseA)
		})
	}()

	aDone := make(chan struct {
		result *ExecuteBeadLoopResult
		err    error
	}, 1)
	bDone := make(chan struct {
		result *ExecuteBeadLoopResult
		err    error
	}, 1)

	aWorker := &ExecuteBeadWorker{
		Store: workerAStore,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			aExecCalls.Add(1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				AttemptID: "att-a",
				Status:    ExecuteBeadStatusSuccess,
				ResultRev: "rev-a",
			}, nil
		}),
	}
	bWorker := &ExecuteBeadWorker{
		Store: workerBStore,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			bExecCalls.Add(1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				AttemptID: "att-b",
				Status:    ExecuteBeadStatusSuccess,
				ResultRev: "rev-b",
			}, nil
		}),
	}

	go func() {
		result, err := aWorker.Run(context.Background(), rcfgA, ExecuteBeadLoopRuntime{
			Once:            true,
			EventSink:       &aSink,
			PreClaimTimeout: 2 * time.Second,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				select {
				case <-aStarted:
				default:
					close(aStarted)
				}
				select {
				case <-releaseA:
				case <-ctx.Done():
					return PreClaimIntakeResult{}, ctx.Err()
				}
				return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
			},
		})
		aDone <- struct {
			result *ExecuteBeadLoopResult
			err    error
		}{result: result, err: err}
	}()

	select {
	case <-aStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("worker A never entered pre-claim intake")
	}

	initialLease, found, err := store.ClaimLease(beadID)
	require.NoError(t, err)
	require.True(t, found, "claim lease must exist after worker A claims the bead")
	require.Equal(t, os.Getpid(), initialLease.PID, "claim lease must record the live worker PID")
	require.Equal(t, currentMachineIDForTest(), initialLease.Machine, "claim lease must record the current machine")

	require.Eventually(t, func() bool {
		lease, found, err := store.ClaimLease(beadID)
		return err == nil && found && lease.UpdatedAt.After(initialLease.UpdatedAt)
	}, 2*time.Second, 10*time.Millisecond,
		"claim heartbeat must refresh while worker A is blocked in intake")

	require.Eventually(t, func() bool {
		lease, found, err := store.ClaimLease(beadID)
		return err == nil && found && time.Since(lease.UpdatedAt) > bead.HeartbeatTTL
	}, 2*time.Second, 10*time.Millisecond,
		"stale-but-alive lease must age past HeartbeatTTL while the original PID remains live")

	go func() {
		result, err := bWorker.Run(context.Background(), rcfgB, ExecuteBeadLoopRuntime{
			Once:      true,
			EventSink: &bSink,
		})
		bDone <- struct {
			result *ExecuteBeadLoopResult
			err    error
		}{result: result, err: err}
	}()

	bRun := <-bDone
	require.NoError(t, bRun.err)
	require.NotNil(t, bRun.result)

	releaseAOnce.Do(func() {
		close(releaseA)
	})

	aRun := <-aDone
	require.NoError(t, aRun.err)
	require.NotNil(t, aRun.result)

	assert.Equal(t, int32(1), aExecCalls.Load(), "worker A must execute exactly once")
	assert.Equal(t, int32(0), bExecCalls.Load(), "worker B must never reach the executor")
	assert.Equal(t, 1, aRun.result.Attempts, "worker A must record exactly one attempt")
	assert.Equal(t, 0, bRun.result.Attempts, "worker B must not record an attempt")

	aEvents := parseLoopEvents(t, aSink.String())
	bEvents := parseLoopEvents(t, bSink.String())
	assert.Len(t, loopEventDataByType(aEvents, "bead.claimed"), 1, "worker A must claim exactly once")
	assert.Len(t, loopEventDataByType(bEvents, "bead.claimed"), 0, "worker B must never claim the bead")

	events, err := store.Events(beadID)
	require.NoError(t, err)
	var executeBeadEvents int
	for _, ev := range events {
		if ev.Kind == "execute-bead" {
			executeBeadEvents++
			continue
		}
		if ev.Kind != "attempt.terminated" {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		assert.NotEqual(t, "bead_closed_externally", body["reason"],
			"no attempt.terminated/bead_closed_externally event is expected when the second worker never claims")
	}
	assert.Equal(t, 1, executeBeadEvents, "worker A must execute exactly once")
}
