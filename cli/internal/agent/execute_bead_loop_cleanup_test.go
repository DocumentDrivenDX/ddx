package agent

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cleanupRunnerFunc func(context.Context) (ExecutionCleanupSummary, error)

func (f cleanupRunnerFunc) Cleanup(ctx context.Context) (ExecutionCleanupSummary, error) {
	return f(ctx)
}

type cleanupAwareStore struct {
	*bead.Store
	readyCalls     *int32
	readyCheckOnce sync.Once
	checkReadyFn   func()
}

func (s *cleanupAwareStore) ReadyExecution() ([]bead.Bead, error) {
	if s.readyCalls != nil {
		atomic.AddInt32(s.readyCalls, 1)
	}
	s.readyCheckOnce.Do(func() {
		if s.checkReadyFn != nil {
			s.checkReadyFn()
		}
	})
	return s.Store.ReadyExecution()
}

func (s *cleanupAwareStore) TouchClaimHeartbeat(id string) error {
	return s.Store.TouchClaimHeartbeat(id)
}

func (s *cleanupAwareStore) Unclaim(id string) error {
	return s.Store.Unclaim(id)
}

func (s *cleanupAwareStore) CloseWithEvidence(id, sessionID, commitSHA string) error {
	return s.Store.CloseWithEvidence(id, sessionID, commitSHA)
}

func (s *cleanupAwareStore) AppendEvent(id string, event bead.BeadEvent) error {
	return s.Store.AppendEvent(id, event)
}

func (s *cleanupAwareStore) Events(id string) ([]bead.BeadEvent, error) {
	return s.Store.Events(id)
}

func (s *cleanupAwareStore) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	return s.Store.SetExecutionCooldown(id, until, status, detail, baseRev)
}

func (s *cleanupAwareStore) IncrNoChangesCount(id string) (int, error) {
	return s.Store.IncrNoChangesCount(id)
}

func (s *cleanupAwareStore) Reopen(id, reason, notes string) error {
	return s.Store.Reopen(id, reason, notes)
}

type cleanupAdvanceAwareStore struct {
	ExecuteBeadLoopStore
	t                      *testing.T
	cleanupCalls           *int32
	readyCalls             *int32
	firstReadyCleanupCount int32
}

func (s *cleanupAdvanceAwareStore) ReadyExecution() ([]bead.Bead, error) {
	ready := atomic.AddInt32(s.readyCalls, 1)
	if ready == 1 {
		atomic.StoreInt32(&s.firstReadyCleanupCount, atomic.LoadInt32(s.cleanupCalls))
	}
	if ready == 2 && atomic.LoadInt32(s.cleanupCalls) <= atomic.LoadInt32(&s.firstReadyCleanupCount) {
		s.t.Fatalf("cleanup must run again before the next claim after setup failure")
	}
	return s.ExecuteBeadLoopStore.ReadyExecution()
}

func (s *cleanupAdvanceAwareStore) TouchClaimHeartbeat(id string) error {
	return s.ExecuteBeadLoopStore.TouchClaimHeartbeat(id)
}

func (s *cleanupAdvanceAwareStore) Unclaim(id string) error {
	return s.ExecuteBeadLoopStore.Unclaim(id)
}

func (s *cleanupAdvanceAwareStore) CloseWithEvidence(id, sessionID, commitSHA string) error {
	return s.ExecuteBeadLoopStore.CloseWithEvidence(id, sessionID, commitSHA)
}

func (s *cleanupAdvanceAwareStore) AppendEvent(id string, event bead.BeadEvent) error {
	return s.ExecuteBeadLoopStore.AppendEvent(id, event)
}

func (s *cleanupAdvanceAwareStore) Events(id string) ([]bead.BeadEvent, error) {
	return s.ExecuteBeadLoopStore.Events(id)
}

func (s *cleanupAdvanceAwareStore) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	return s.ExecuteBeadLoopStore.SetExecutionCooldown(id, until, status, detail, baseRev)
}

func (s *cleanupAdvanceAwareStore) IncrNoChangesCount(id string) (int, error) {
	return s.ExecuteBeadLoopStore.IncrNoChangesCount(id)
}

func (s *cleanupAdvanceAwareStore) Reopen(id, reason, notes string) error {
	return s.ExecuteBeadLoopStore.Reopen(id, reason, notes)
}

type cleanupAdvanceClaimStore struct {
	ExecuteBeadLoopStore
	t                      *testing.T
	cleanupCalls           *int32
	claimCalls             *int32
	firstClaimCleanupCount int32
}

func (s *cleanupAdvanceClaimStore) Claim(id, assignee string) error {
	claim := atomic.AddInt32(s.claimCalls, 1)
	if claim == 1 {
		atomic.StoreInt32(&s.firstClaimCleanupCount, atomic.LoadInt32(s.cleanupCalls))
	}
	if claim == 2 && atomic.LoadInt32(s.cleanupCalls) <= atomic.LoadInt32(&s.firstClaimCleanupCount) {
		s.t.Fatalf("cleanup must run again before the next claim after finalization failure")
	}
	return s.ExecuteBeadLoopStore.Claim(id, assignee)
}

func (s *cleanupAdvanceClaimStore) ClaimWithOptions(id, assignee, session, worktree string) error {
	claim := atomic.AddInt32(s.claimCalls, 1)
	if claim == 1 {
		atomic.StoreInt32(&s.firstClaimCleanupCount, atomic.LoadInt32(s.cleanupCalls))
	}
	if claim == 2 && atomic.LoadInt32(s.cleanupCalls) <= atomic.LoadInt32(&s.firstClaimCleanupCount) {
		s.t.Fatalf("cleanup must run again before the next claim after finalization failure")
	}
	if claimer, ok := s.ExecuteBeadLoopStore.(interface {
		ClaimWithOptions(id, assignee, session, worktree string) error
	}); ok {
		return claimer.ClaimWithOptions(id, assignee, session, worktree)
	}
	return s.ExecuteBeadLoopStore.Claim(id, assignee)
}

func TestWorkCleanup_RunsAtStartup(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	tempRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)
	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-startup-cleanup-20260506T154739-deadbeef")
	require.NoError(t, os.MkdirAll(stalePath, 0o755))
	require.NoError(t, WriteExecutionCleanupMetadata(stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-startup-cleanup",
		AttemptID:    "20260506T154739-deadbeef",
		WorktreePath: stalePath,
	}))

	inner, first, _ := newExecuteLoopTestStore(t)
	var cleanupCalls int32
	store := &cleanupAwareStore{
		Store: inner,
		checkReadyFn: func() {
			if atomic.LoadInt32(&cleanupCalls) == 0 {
				t.Fatalf("cleanup must run before the first ready claim")
			}
			assert.NoFileExists(t, stalePath)
		},
	}

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	runner := cleanupRunnerFunc(func(ctx context.Context) (ExecutionCleanupSummary, error) {
		atomic.AddInt32(&cleanupCalls, 1)
		return mgr.Cleanup(ctx)
	})

	worker := &ExecuteBeadWorker{Store: store, Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "sess-startup", ResultRev: "abc123"}, nil
	})}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		ProjectRoot:   projectRoot,
		CleanupRunner: runner,
		CleanupLog:    io.Discard,
		Once:          true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&cleanupCalls), int32(1))

	got, err := inner.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestWorkCleanup_RunsPeriodicallyWhilePolling(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	var cleanupCalls int32
	tickCh := make(chan time.Time, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := startExecutionCleanupWorker(ctx, projectRoot, cleanupRunnerFunc(func(context.Context) (ExecutionCleanupSummary, error) {
		atomic.AddInt32(&cleanupCalls, 1)
		return ExecutionCleanupSummary{}, nil
	}), time.Hour, tickCh, io.Discard, nil)
	defer stop(false)

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&cleanupCalls))

	tickCh <- time.Now()
	tickCh <- time.Now().Add(time.Second)

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&cleanupCalls) >= 2
	}, 2*time.Second, 10*time.Millisecond)
}

func TestWorkCleanup_RunsAfterSetupFailureBeforeNextClaim(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	inner, first, second := newExecuteLoopTestStore(t)
	var appendCalls int32
	store := &errorInjectingStore{
		ExecuteBeadLoopStore: inner,
		onAppendEvent: func(id string, event bead.BeadEvent) error {
			if id == first.ID && atomic.AddInt32(&appendCalls, 1) == 1 {
				return errors.New("finalization failed")
			}
			return nil
		},
	}

	var cleanupCalls int32
	guard := &cleanupAdvanceClaimStore{
		ExecuteBeadLoopStore: store,
		t:                    t,
		cleanupCalls:         &cleanupCalls,
		claimCalls:           new(int32),
	}

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	runner := cleanupRunnerFunc(func(ctx context.Context) (ExecutionCleanupSummary, error) {
		atomic.AddInt32(&cleanupCalls, 1)
		return mgr.Cleanup(ctx)
	})

	worker := &ExecuteBeadWorker{
		Store: guard,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "sess-setup", ResultRev: "feedface"}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		ProjectRoot:   projectRoot,
		CleanupRunner: runner,
		CleanupLog:    io.Discard,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&appendCalls), int32(1))

	gotSecond, err := inner.Get(context.Background(), second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotSecond.Status)
}

type blockingCleanupRunner struct {
	started   chan struct{}
	release   chan struct{}
	calls     int32
	active    int32
	maxActive int32
}

func (r *blockingCleanupRunner) Cleanup(ctx context.Context) (ExecutionCleanupSummary, error) {
	atomic.AddInt32(&r.calls, 1)
	active := atomic.AddInt32(&r.active, 1)
	for {
		prev := atomic.LoadInt32(&r.maxActive)
		if active <= prev || atomic.CompareAndSwapInt32(&r.maxActive, prev, active) {
			break
		}
	}
	if r.started != nil {
		select {
		case <-r.started:
		default:
			close(r.started)
		}
	}
	select {
	case <-ctx.Done():
	case <-r.release:
	}
	atomic.AddInt32(&r.active, -1)
	return ExecutionCleanupSummary{}, nil
}

func TestWorkCleanup_UsesProjectLock(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	runner := &blockingCleanupRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	firstDone := make(chan struct {
		summary ExecutionCleanupSummary
		skipped bool
		err     error
	}, 1)
	go func() {
		summary, skipped, err := runExecutionCleanupPass(context.Background(), projectRoot, runner, io.Discard, nil, "startup")
		firstDone <- struct {
			summary ExecutionCleanupSummary
			skipped bool
			err     error
		}{summary: summary, skipped: skipped, err: err}
	}()

	<-runner.started
	summary, skipped, err := runExecutionCleanupPass(context.Background(), projectRoot, runner, io.Discard, nil, "periodic")
	require.NoError(t, err)
	assert.True(t, skipped)
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.calls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.maxActive))
	assert.Empty(t, summary.ProjectRoot)

	close(runner.release)
	firstResult := <-firstDone
	require.NoError(t, firstResult.err)
	assert.False(t, firstResult.skipped)
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.calls))
}

func TestBackgroundCleanupEndToEnd_UsesLockAndJitter(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	delay := jitteredCleanupDelay(10*time.Minute, rand.New(rand.NewSource(1)))
	assert.GreaterOrEqual(t, delay, 8*time.Minute)
	assert.LessOrEqual(t, delay, 12*time.Minute)

	runner := &blockingCleanupRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	runnerSummary := ExecutionCleanupSummary{
		ProjectRoot:          projectRoot,
		TempRoot:             filepath.Join(projectRoot, ddxroot.DirName, "tmp"),
		RemovedRunStateFiles: 1,
		BytesReclaimed:       1,
		InodesReclaimed:      1,
	}
	originalCleanup := runner.Cleanup
	wrappedRunner := cleanupRunnerFunc(func(ctx context.Context) (ExecutionCleanupSummary, error) {
		_, err := originalCleanup(ctx)
		return runnerSummary, err
	})

	type cleanupEvent struct {
		typ    string
		reason string
	}
	var (
		eventsMu sync.Mutex
		events   []cleanupEvent
	)
	emit := func(typ string, data map[string]any) {
		reason, _ := data["reason"].(string)
		eventsMu.Lock()
		events = append(events, cleanupEvent{typ: typ, reason: reason})
		eventsMu.Unlock()
	}
	eventCount := func(typ string) int {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		count := 0
		for _, ev := range events {
			if ev.typ == typ {
				count++
			}
		}
		return count
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tickA := make(chan time.Time, 4)
	tickB := make(chan time.Time, 4)
	stopA := startExecutionCleanupWorker(ctx, projectRoot, wrappedRunner, time.Hour, tickA, io.Discard, emit)
	stopB := startExecutionCleanupWorker(ctx, projectRoot, wrappedRunner, time.Hour, tickB, io.Discard, emit)

	tickA <- time.Unix(100, 0)
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first cleanup pass did not start")
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.calls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.active))

	tickB <- time.Unix(101, 0)
	require.Eventually(t, func() bool {
		return eventCount("cleanup.skipped") >= 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.calls), "second worker must skip while project cleanup lock is held")
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.maxActive), "cleanup passes must not overlap")

	close(runner.release)
	require.Eventually(t, func() bool {
		return eventCount("cleanup.pass") >= 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&runner.active))

	tickA <- time.Unix(102, 0)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&runner.calls) >= 2 && eventCount("cleanup.pass") >= 2
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	stopA(false)
	stopB(false)
}

func TestWorkCleanup_ShutdownPassRunsOnSignal(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	inner, first, _ := newExecuteLoopTestStore(t)

	var cleanupCalls int32
	worker := &ExecuteBeadWorker{
		Store: inner,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			<-ctx.Done()
			return ExecuteBeadReport{
				BeadID:           beadID,
				Status:           ExecuteBeadStatusExecutionFailed,
				Detail:           "context cancelled",
				Disrupted:        true,
				DisruptionReason: "context_canceled",
				OutcomeReason:    "context_cancelled",
				SessionID:        "sess-shutdown",
				ResultRev:        "deadbeef",
			}, ctx.Err()
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		result *ExecuteBeadLoopResult
		err    error
	}, 1)
	go func() {
		result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			ProjectRoot: projectRoot,
			CleanupRunner: cleanupRunnerFunc(func(context.Context) (ExecutionCleanupSummary, error) {
				atomic.AddInt32(&cleanupCalls, 1)
				return ExecutionCleanupSummary{}, nil
			}),
			CleanupLog: io.Discard,
			PreClaimHook: func(context.Context) error {
				return nil
			},
		})
		done <- struct {
			result *ExecuteBeadLoopResult
			err    error
		}{result: result, err: err}
	}()

	require.Eventually(t, func() bool {
		lease, found, err := inner.ClaimLease(first.ID)
		return err == nil && found && lease.Owner != ""
	}, 2*time.Second, 10*time.Millisecond)

	beforeCancel := atomic.LoadInt32(&cleanupCalls)
	cancel()

	select {
	case result := <-done:
		require.ErrorIs(t, result.err, context.Canceled)
		require.NotNil(t, result.result)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&cleanupCalls) > beforeCancel
	}, 2*time.Second, 10*time.Millisecond)

	got, err := inner.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Owner)
}
