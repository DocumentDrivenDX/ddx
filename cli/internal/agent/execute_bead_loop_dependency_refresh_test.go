package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/gitrepohealth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dependencyRefreshRunResult struct {
	result *ExecuteBeadLoopResult
	err    error
}

type dependencyRefreshGetStore struct {
	*bead.Store
	dependencyID  string
	dependencyErr error
}

func (s *dependencyRefreshGetStore) Get(ctx context.Context, id string) (*bead.Bead, error) {
	if id == s.dependencyID {
		return nil, s.dependencyErr
	}
	return s.Store.Get(ctx, id)
}

func newDependencyRefreshFixture(t *testing.T, withNext bool) (*bead.Store, *bead.Bead, *bead.Bead, *bead.Bead) {
	t.Helper()
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	target := &bead.Bead{ID: "ddx-dependency-refresh-target", Title: "Dependency refresh target", Priority: 0}
	dependency := &bead.Bead{
		ID:       "ddx-dependency-refresh-blocker",
		Title:    "Dependency blocker",
		Priority: 2,
		Labels:   []string{"decomposed"},
		Extra:    map[string]any{bead.ExtraExecutionElig: false},
	}
	require.NoError(t, store.Create(context.Background(), target))
	require.NoError(t, store.Create(context.Background(), dependency))
	var next *bead.Bead
	if withNext {
		next = &bead.Bead{ID: "ddx-dependency-refresh-next", Title: "Next ready bead", Priority: 1}
		require.NoError(t, store.Create(context.Background(), next))
	}
	return store, target, dependency, next
}

func dependencyRefreshConfig() config.ResolvedConfig {
	opts := config.TestLoopConfigOpts{Assignee: "dependency-refresh-worker"}
	return config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
}

func waitForDependencyRefreshRun(t *testing.T, done <-chan dependencyRefreshRunResult) dependencyRefreshRunResult {
	t.Helper()
	select {
	case completed := <-done:
		return completed
	case <-time.After(5 * time.Second):
		t.Fatal("execute-bead loop did not finish")
		return dependencyRefreshRunResult{}
	}
}

func waitForDependencyRefreshSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("execute-bead loop did not reach the readiness gate")
	}
}

func waitForDependencyRefreshBead(t *testing.T, observed <-chan *bead.Bead) *bead.Bead {
	t.Helper()
	select {
	case fresh := <-observed:
		return fresh
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not publish the fresh bead")
		return nil
	}
}

func assertDependencyRefreshClaimReleased(t *testing.T, store *bead.Store, beadID string) {
	t.Helper()
	fresh, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, fresh.Status)
	assert.Empty(t, fresh.Owner)
	heartbeatFresh, heartbeatFound, err := store.ClaimHeartbeatFresh(beadID)
	require.NoError(t, err)
	assert.False(t, heartbeatFound, "released claim must remove its lease sidecar")
	assert.False(t, heartbeatFresh)
}

func dependencyRefreshTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

func waitAtDependencyRefreshGate(ctx context.Context, release <-chan struct{}) (PreClaimIntakeResult, error) {
	select {
	case <-release:
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
	case <-ctx.Done():
		return PreClaimIntakeResult{}, ctx.Err()
	}
}

func TestExecuteBeadLoop_DependencyAddedDuringReadinessReleasesClaimWithoutDispatch(t *testing.T) {
	ctx := dependencyRefreshTestContext(t)
	store, target, dependency, _ := newDependencyRefreshFixture(t, false)
	readinessStarted := make(chan struct{})
	releaseReadiness := make(chan struct{})
	var executorCalls atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(context.Context, string) (ExecuteBeadReport, error) {
			executorCalls.Add(1)
			return ExecuteBeadReport{}, nil
		}),
	}

	done := make(chan dependencyRefreshRunResult, 1)
	go func() {
		result, err := worker.Run(ctx, dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: target.ID,
			PreClaimIntakeHook: func(context.Context, string) (PreClaimIntakeResult, error) {
				close(readinessStarted)
				return waitAtDependencyRefreshGate(ctx, releaseReadiness)
			},
		})
		done <- dependencyRefreshRunResult{result: result, err: err}
	}()

	waitForDependencyRefreshSignal(t, readinessStarted)
	require.NoError(t, store.DepAdd(context.Background(), target.ID, dependency.ID))
	close(releaseReadiness)
	completed := waitForDependencyRefreshRun(t, done)
	require.NoError(t, completed.err)
	require.NotNil(t, completed.result)
	assert.Zero(t, completed.result.Attempts)
	assert.Zero(t, executorCalls.Load())

	assertDependencyRefreshClaimReleased(t, store, target.ID)
}

func TestExecuteBeadLoop_DependencyAddedDuringReadinessContinuesToNextReadyBead(t *testing.T) {
	ctx := dependencyRefreshTestContext(t)
	store, target, dependency, next := newDependencyRefreshFixture(t, true)
	readinessStarted := make(chan struct{})
	releaseReadiness := make(chan struct{})
	var sink bytes.Buffer
	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "dependency-refresh", ResultRev: "abc123"}, nil
		}),
	}

	done := make(chan dependencyRefreshRunResult, 1)
	go func() {
		result, err := worker.Run(ctx, dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
			Once:      true,
			EventSink: &sink,
			PreClaimIntakeHook: func(_ context.Context, beadID string) (PreClaimIntakeResult, error) {
				if beadID == target.ID {
					close(readinessStarted)
					return waitAtDependencyRefreshGate(ctx, releaseReadiness)
				}
				return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
			},
		})
		done <- dependencyRefreshRunResult{result: result, err: err}
	}()

	waitForDependencyRefreshSignal(t, readinessStarted)
	require.NoError(t, store.DepAdd(context.Background(), target.ID, dependency.ID))
	close(releaseReadiness)
	completed := waitForDependencyRefreshRun(t, done)
	require.NoError(t, completed.err)
	require.NotNil(t, completed.result)
	assert.Equal(t, []string{next.ID}, executed)
	assert.Equal(t, 1, completed.result.Attempts)

	skips := loopEventDataByType(parseLoopEvents(t, sink.String()), "picker.skip_stale_candidate")
	require.Len(t, skips, 1)
	assert.Equal(t, target.ID, skips[0]["bead_id"])
	assert.Equal(t, "dependency_waiting", skips[0]["reason"])
	assert.Equal(t, "pre_attempt", skips[0]["stage"])
	assert.Equal(t, "bead is no longer execution-ready because dependencies remain open", skips[0]["detail"])
	assert.Equal(t, "dependency_waiting", skips[0]["failure_class"])
	assert.Equal(t, "wait", skips[0]["retry_action"])

	assertDependencyRefreshClaimReleased(t, store, target.ID)
}

func TestExecuteBeadLoop_MetadataOnlyMutationDuringReadinessStillDispatches(t *testing.T) {
	ctx := dependencyRefreshTestContext(t)
	store, target, _, _ := newDependencyRefreshFixture(t, false)
	readinessStarted := make(chan struct{})
	releaseReadiness := make(chan struct{})
	observed := make(chan *bead.Bead, 1)
	var sink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			fresh, _ := agenttry.BeadFromContext(ctx)
			observed <- fresh
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "metadata-refresh", ResultRev: "abc123"}, nil
		}),
	}

	done := make(chan dependencyRefreshRunResult, 1)
	go func() {
		result, err := worker.Run(ctx, dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: target.ID,
			EventSink:    &sink,
			PreClaimIntakeHook: func(context.Context, string) (PreClaimIntakeResult, error) {
				close(readinessStarted)
				return waitAtDependencyRefreshGate(ctx, releaseReadiness)
			},
		})
		done <- dependencyRefreshRunResult{result: result, err: err}
	}()

	waitForDependencyRefreshSignal(t, readinessStarted)
	require.NoError(t, store.AppendNotes(target.ID, "fresh note added during readiness"))
	close(releaseReadiness)
	completed := waitForDependencyRefreshRun(t, done)
	require.NoError(t, completed.err)
	require.NotNil(t, completed.result)
	assert.Equal(t, 1, completed.result.Attempts)
	fresh := waitForDependencyRefreshBead(t, observed)
	require.NotNil(t, fresh)
	assert.Contains(t, fresh.Notes, "fresh note added during readiness")
	assert.Empty(t, loopEventDataByType(parseLoopEvents(t, sink.String()), "picker.skip_stale_candidate"))
}

func TestExecuteBeadLoop_DependencySatisfiedBeforeDispatchUsesFreshGraph(t *testing.T) {
	ctx := dependencyRefreshTestContext(t)
	store, target, dependency, _ := newDependencyRefreshFixture(t, false)
	readinessStarted := make(chan struct{})
	releaseReadiness := make(chan struct{})
	observed := make(chan *bead.Bead, 1)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			fresh, _ := agenttry.BeadFromContext(ctx)
			observed <- fresh
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "dependency-satisfied", ResultRev: "abc123"}, nil
		}),
	}

	done := make(chan dependencyRefreshRunResult, 1)
	go func() {
		result, err := worker.Run(ctx, dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: target.ID,
			PreClaimIntakeHook: func(context.Context, string) (PreClaimIntakeResult, error) {
				close(readinessStarted)
				return waitAtDependencyRefreshGate(ctx, releaseReadiness)
			},
		})
		done <- dependencyRefreshRunResult{result: result, err: err}
	}()

	waitForDependencyRefreshSignal(t, readinessStarted)
	require.NoError(t, store.DepAdd(context.Background(), target.ID, dependency.ID))
	require.NoError(t, store.Close(context.Background(), dependency.ID))
	close(releaseReadiness)
	completed := waitForDependencyRefreshRun(t, done)
	require.NoError(t, completed.err)
	require.NotNil(t, completed.result)
	assert.Equal(t, 1, completed.result.Attempts)
	fresh := waitForDependencyRefreshBead(t, observed)
	require.NotNil(t, fresh)
	assert.Contains(t, fresh.DepIDs(), dependency.ID)
}

func TestExecuteBeadLoop_MissingDependencyDuringFinalRefreshReleasesClaim(t *testing.T) {
	ctx := dependencyRefreshTestContext(t)
	store, target, dependency, _ := newDependencyRefreshFixture(t, false)
	readinessStarted := make(chan struct{})
	releaseReadiness := make(chan struct{})
	var sink bytes.Buffer
	var executorCalls atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: &dependencyRefreshGetStore{
			Store:         store,
			dependencyID:  dependency.ID,
			dependencyErr: fmt.Errorf("%w: %s", bead.ErrNotFound, dependency.ID),
		},
		Executor: ExecuteBeadExecutorFunc(func(context.Context, string) (ExecuteBeadReport, error) {
			executorCalls.Add(1)
			return ExecuteBeadReport{}, nil
		}),
	}

	done := make(chan dependencyRefreshRunResult, 1)
	go func() {
		result, err := worker.Run(ctx, dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: target.ID,
			EventSink:    &sink,
			PreClaimIntakeHook: func(context.Context, string) (PreClaimIntakeResult, error) {
				close(readinessStarted)
				return waitAtDependencyRefreshGate(ctx, releaseReadiness)
			},
		})
		done <- dependencyRefreshRunResult{result: result, err: err}
	}()

	waitForDependencyRefreshSignal(t, readinessStarted)
	require.NoError(t, store.DepAdd(context.Background(), target.ID, dependency.ID))
	close(releaseReadiness)
	completed := waitForDependencyRefreshRun(t, done)
	require.NoError(t, completed.err)
	require.NotNil(t, completed.result)
	assert.Zero(t, completed.result.Attempts)
	assert.Zero(t, executorCalls.Load())
	assertDependencyRefreshClaimReleased(t, store, target.ID)

	skips := loopEventDataByType(parseLoopEvents(t, sink.String()), "picker.skip_stale_candidate")
	require.Len(t, skips, 1)
	assert.Equal(t, "dependency_waiting", skips[0]["reason"])
	assert.Equal(t, "bead is no longer execution-ready because dependencies remain open", skips[0]["detail"])
	assert.Equal(t, "dependency_waiting", skips[0]["failure_class"])
	assert.Equal(t, "wait", skips[0]["retry_action"])
}

func TestExecuteBeadLoop_DependencyReadErrorReleasesClaimAndPropagates(t *testing.T) {
	ctx := dependencyRefreshTestContext(t)
	store, target, dependency, _ := newDependencyRefreshFixture(t, false)
	readinessStarted := make(chan struct{})
	releaseReadiness := make(chan struct{})
	injectedErr := errors.New("injected dependency read failure")
	var executorCalls atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: &dependencyRefreshGetStore{
			Store:         store,
			dependencyID:  dependency.ID,
			dependencyErr: injectedErr,
		},
		Executor: ExecuteBeadExecutorFunc(func(context.Context, string) (ExecuteBeadReport, error) {
			executorCalls.Add(1)
			return ExecuteBeadReport{}, nil
		}),
	}

	done := make(chan dependencyRefreshRunResult, 1)
	go func() {
		result, err := worker.Run(ctx, dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: target.ID,
			PreClaimIntakeHook: func(context.Context, string) (PreClaimIntakeResult, error) {
				close(readinessStarted)
				return waitAtDependencyRefreshGate(ctx, releaseReadiness)
			},
		})
		done <- dependencyRefreshRunResult{result: result, err: err}
	}()

	waitForDependencyRefreshSignal(t, readinessStarted)
	require.NoError(t, store.DepAdd(context.Background(), target.ID, dependency.ID))
	close(releaseReadiness)
	completed := waitForDependencyRefreshRun(t, done)
	require.ErrorIs(t, completed.err, injectedErr)
	require.NotNil(t, completed.result)
	assert.Zero(t, completed.result.Attempts)
	assert.Zero(t, executorCalls.Load())
	assertDependencyRefreshClaimReleased(t, store, target.ID)
}

func TestExecuteBeadLoop_DependencyAddedDuringFinalGitRepairReleasesClaimWithoutDispatch(t *testing.T) {
	store, target, dependency, _ := newDependencyRefreshFixture(t, false)
	var repairCalls atomic.Int32
	var executorCalls atomic.Int32
	var sink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(context.Context, string) (ExecuteBeadReport, error) {
			executorCalls.Add(1)
			return ExecuteBeadReport{}, nil
		}),
		preDispatchGitRepairer: func(context.Context, string) gitrepohealth.RepairResult {
			if repairCalls.Add(1) == 2 {
				require.NoError(t, store.DepAdd(context.Background(), target.ID, dependency.ID))
			}
			return gitrepohealth.RepairResult{StatusSucceeded: true}
		},
	}

	result, err := worker.Run(context.Background(), dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: target.ID,
		EventSink:    &sink,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(2), repairCalls.Load(), "test must mutate the graph during the final git repair")
	assert.Zero(t, result.Attempts)
	assert.Zero(t, executorCalls.Load())
	assertDependencyRefreshClaimReleased(t, store, target.ID)

	skips := loopEventDataByType(parseLoopEvents(t, sink.String()), "picker.skip_stale_candidate")
	require.Len(t, skips, 1)
	assert.Equal(t, "dependency_waiting", skips[0]["reason"])
	assert.Equal(t, "pre_attempt", skips[0]["stage"])
	assert.Equal(t, "dependency_waiting", skips[0]["failure_class"])
	assert.Equal(t, "wait", skips[0]["retry_action"])
}

func TestExecuteBeadLoop_StaleReadinessStateSkipsBeforeFailingFinalGitRepair(t *testing.T) {
	store, target, _, _ := newDependencyRefreshFixture(t, false)
	var repairCalls atomic.Int32
	var executorCalls atomic.Int32
	var sink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(context.Context, string) (ExecuteBeadReport, error) {
			executorCalls.Add(1)
			return ExecuteBeadReport{}, nil
		}),
		preDispatchGitRepairer: func(context.Context, string) gitrepohealth.RepairResult {
			if repairCalls.Add(1) == 1 {
				return gitrepohealth.RepairResult{StatusSucceeded: true}
			}
			return gitrepohealth.RepairResult{StatusSucceeded: false, StatusStderr: "injected final repair failure"}
		},
	}

	result, err := worker.Run(context.Background(), dependencyRefreshConfig(), ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: target.ID,
		EventSink:    &sink,
		PreClaimIntakeHook: func(context.Context, string) (PreClaimIntakeResult, error) {
			require.NoError(t, store.Update(context.Background(), target.ID, func(fresh *bead.Bead) {
				if fresh.Extra == nil {
					fresh.Extra = make(map[string]any)
				}
				fresh.Extra["superseded-by"] = "ddx-replacement"
			}))
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), repairCalls.Load(), "stale readiness state must skip before final git repair")
	assert.Zero(t, result.Attempts)
	assert.Zero(t, executorCalls.Load())
	assert.Nil(t, result.OperatorAttention)
	assertDependencyRefreshClaimReleased(t, store, target.ID)

	events := parseLoopEvents(t, sink.String())
	skips := loopEventDataByType(events, "picker.skip_stale_candidate")
	require.Len(t, skips, 1)
	assert.Equal(t, "superseded", skips[0]["reason"])
	assert.Equal(t, "pre_attempt", skips[0]["stage"])
	assert.Empty(t, loopEventDataByType(events, "loop.operator_attention"))
}
