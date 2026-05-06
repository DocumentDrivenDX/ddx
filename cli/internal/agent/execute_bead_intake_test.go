package agent

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntake_ActionableAtomic_ClaimsNormally(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var intakeCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeCalls), "intake hook must run once")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "actionable atomic intake must proceed to Claim")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestIntake_InfrastructureErrorFailsOpen(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var eventSink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake-warn",
				ResultRev: "def456",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &eventSink,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{}, fmt.Errorf("intake service unavailable")
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "intake_error must fail open to Claim")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Contains(t, eventSink.String(), "pre_claim_intake.warn")
	assert.Contains(t, eventSink.String(), "intake_error")
	assert.Contains(t, eventSink.String(), "intake service unavailable")
}

func TestIntake_HookRunsBeforeClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	var preflightSeen int32
	var intakeSeen int32
	store := &claimCountingStore{
		Store: inner,
		beforeClaim: func() {
			if atomic.LoadInt32(&preflightSeen) == 0 {
				t.Fatal("route preflight must run before Claim")
			}
			if atomic.LoadInt32(&intakeSeen) == 0 {
				t.Fatal("PreClaimIntakeHook must run before Claim")
			}
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake-order",
				ResultRev: "fedcba",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "claude", Model: "claude-sonnet-4-6"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		RoutePreflight: func(ctx context.Context, harness, model string) error {
			atomic.StoreInt32(&preflightSeen, 1)
			return nil
		},
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			if atomic.LoadInt32(&preflightSeen) == 0 {
				t.Fatal("PreClaimIntakeHook must run after route preflight")
			}
			atomic.StoreInt32(&intakeSeen, 1)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&preflightSeen), "route preflight must run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeSeen), "intake hook must run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "Claim must run after preflight and intake")
}

func TestIntake_NonAtomicSkipsClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var intakeCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run when intake is non-atomic")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeAmbiguousNeedsHuman,
				Detail:  "need human clarification",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls), "non-atomic intake must skip Claim")
	assert.Equal(t, 0, result.Attempts)

	got, err := inner.Get("ddx-0001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}
