package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// disruptionEventCapture is an in-memory capture of structured loop events
// emitted via writeLoopEvent. Tests use this to assert the
// `disruption_detected` event surfaces with the expected reason kind.
type disruptionEventCapture struct {
	mu     atomic.Value
	lines  []string
	wrote  int32
	wrErr  error
	closed atomic.Bool
}

func (c *disruptionEventCapture) Write(p []byte) (int, error) {
	if c.wrErr != nil {
		return 0, c.wrErr
	}
	atomic.AddInt32(&c.wrote, int32(len(p)))
	cur, _ := c.mu.Load().([]string)
	c.mu.Store(append(cur, string(p)))
	return len(p), nil
}

func (c *disruptionEventCapture) all() []string {
	cur, _ := c.mu.Load().([]string)
	return cur
}

// TestLoop_DisruptedExecution_NoCooldown asserts ddx-5b3e57f4 AC #1, #3, #7:
// when the executor returns a context.Canceled error mid-execution, the loop
// classifies the failure as Disrupted, does NOT call SetExecutionCooldown,
// and leaves the bead immediately re-claimable.
func TestLoop_DisruptedExecution_NoCooldown(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	cancelCtx, cancel := context.WithCancel(context.Background())
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			cancel()
			// Simulate the worker being killed during execution: BaseRev
			// snapshot was taken but no commit was made, so BaseRev ==
			// ResultRev. Without the Disrupted classification the loop
			// would mistake this for a genuine no_changes outcome and
			// park the bead under noProgressCooldown.
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "context canceled",
				BaseRev:   "abc1234",
				ResultRev: "abc1234",
			}, context.Canceled
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(cancelCtx, rcfg, ExecuteBeadLoopRuntime{Once: true})
	// ctx was cancelled mid-run; the loop returns the cancel error after
	// the iteration completes. We assert on the result regardless.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, result)
	require.GreaterOrEqual(t, len(result.Results), 1)
	report := result.Results[0]
	assert.True(t, report.Disrupted, "report must be marked Disrupted on context.Canceled")
	assert.Equal(t, "context_canceled", report.DisruptionReason)
	assert.Empty(t, report.RetryAfter,
		"Disrupted report must NOT carry a retry_after — no cooldown applied")

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	if got.Extra != nil {
		_, hasRetry := got.Extra["work-retry-after"]
		assert.False(t, hasRetry,
			"Disrupted bead must not have work-retry-after persisted")
	}
}

// TestLoop_GenuineNoProgress_StillCooldowns asserts ddx-5b3e57f4 AC #4: a
// model that returns clean (no error) with BaseRev == ResultRev and no
// Disrupted marker still hits the noProgressCooldown branch. This proves the
// disruption fix is targeted to disrupted attempts; TD-031 further narrows
// no_changes so unjustified no_changes remains open without a retry cooldown.
func TestLoop_GenuineNoProgress_NoDefaultCooldown(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				SessionID: "sess-noprog",
				BaseRev:   "feedface00112233",
				ResultRev: "feedface00112233",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	report := result.Results[0]
	assert.False(t, report.Disrupted,
		"clean no_changes return is not disrupted")
	require.Empty(t, report.RetryAfter,
		"unjustified no_changes must not be parked under noProgressCooldown by default")

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)
	_, hasRetry := got.Extra["work-retry-after"]
	assert.False(t, hasRetry,
		"unjustified no_changes bead must not have work-retry-after persisted")
	assert.Contains(t, got.Labels, NoChangesLabelUnjustified)
}

// TestLoop_DisruptionEventEmitted asserts ddx-5b3e57f4 AC #5: a
// `disruption_detected` event is appended to the bead and to the loop event
// sink when a Disrupted classification fires.
func TestLoop_DisruptionEventEmitted(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	transportErr := errors.New("dial tcp 127.0.0.1:443: connection refused")
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    transportErr.Error(),
				BaseRev:   "deadbeef",
				ResultRev: "deadbeef",
			}, transportErr
		}),
	}

	sink := &disruptionEventCapture{}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: sink,
		SessionID: "sess-disrupt",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	report := result.Results[0]
	assert.True(t, report.Disrupted, "connectivity error must be classified Disrupted")
	assert.Equal(t, "provider_connectivity", report.DisruptionReason)
	assert.Empty(t, report.RetryAfter, "provider-connectivity Disrupted bead must skip cooldown")

	// Sink event surface
	// The disruption event is emitted at the generic transport classifier;
	// the returned report is subsequently refined to provider_connectivity.
	var found bool
	for _, line := range sink.all() {
		if strings.Contains(line, `"type":"disruption_detected"`) &&
			strings.Contains(line, `"reason":"transport_error"`) {
			found = true
			break
		}
	}
	assert.True(t, found, "disruption_detected event with reason=transport_error must be emitted to event sink; got: %v", sink.all())

	// Bead event surface
	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	var beadEv *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "disruption_detected" {
			beadEv = &events[i]
			break
		}
	}
	require.NotNil(t, beadEv, "disruption_detected event must be appended to the bead")
	assert.Equal(t, "transport_error", beadEv.Summary)
}

// runInterruptedWorkAttempt drives one canceled execute-loop attempt on a
// hermetic store fixture. newExecuteLoopTestStore pins private HOME/XDG/
// DDX_EXEC_WT_DIR roots under a per-test fixture so this path never inherits
// process-global DDx/config roots or enumerates a host-shared scratch tree.
func runInterruptedWorkAttempt(t *testing.T) (*bead.Store, *bead.Bead, *ExecuteBeadLoopResult, error) {
	t.Helper()

	store, candidate, _ := newExecuteLoopTestStore(t)
	requirePinnedExecuteLoopRoots(t, store)

	cancelCtx, cancel := context.WithCancel(context.Background())
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			cancel()
			<-ctx.Done()
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "context canceled",
				BaseRev:   "abc1234",
				ResultRev: "abc1234",
			}, ctx.Err()
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(cancelCtx, rcfg, ExecuteBeadLoopRuntime{Once: true})
	return store, candidate, result, err
}

// requirePinnedExecuteLoopRoots fails fast when the interrupted-work fixture
// is missing private DDx/config root pins and would fall through to host-
// global HOME/cache/temp trees.
func requirePinnedExecuteLoopRoots(t *testing.T, store *bead.Store) {
	t.Helper()
	require.NotNil(t, store)

	execWT := os.Getenv(config.ExecutionWorktreeRootEnv)
	require.NotEmpty(t, execWT, "interrupted-work fixture must pin %s", config.ExecutionWorktreeRootEnv)
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(execWT),
		"DDX_EXEC_WT_DIR must not resolve to the host process temp root")

	// pinExecuteLoopPrivateRoots places store/ and exec-wt/ as siblings under
	// one private fixture root; reject any store that lives outside that tree.
	require.Equal(t, filepath.Dir(store.Dir), filepath.Dir(execWT),
		"store and exec-wt must share the private fixture root")

	projectRoot := filepath.Dir(store.Dir)
	tempRoot := config.ExecutionTempRoot(projectRoot)
	scratchRoot := config.ExecutionScratchRoot(projectRoot)
	require.Equal(t, filepath.Clean(execWT), filepath.Clean(tempRoot))
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(tempRoot))
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(scratchRoot))
	assertPathUnder(t, store.Dir, filepath.Dir(execWT))
	assertPathUnder(t, tempRoot, filepath.Dir(execWT))
	assertPathUnder(t, scratchRoot, filepath.Dir(execWT))
}

// TestWorkInterrupt_InFlightAttemptUnclaimsBead verifies that an in-flight
// interrupted attempt releases the claim and returns the bead to open so it is
// re-claimable. Fixture roots are private via runInterruptedWorkAttempt.
func TestWorkInterrupt_InFlightAttemptUnclaimsBead(t *testing.T) {
	store, candidate, result, err := runInterruptedWorkAttempt(t)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)
	assert.True(t, result.Results[0].Disrupted)

	// Re-assert hermetic roots after the interrupt path so a mid-run env leak
	// cannot silently re-point cleanup at a host-global DDx scratch tree.
	requirePinnedExecuteLoopRoots(t, store)

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
}

// TestRunInterruptedWorkAttemptUsesPrivateRoots proves runInterruptedWorkAttempt
// re-pins fixture-local DDx/config roots even when process-global HOME/XDG/
// cache roots are poisoned with a host-looking shared exec-wt tree.
func TestRunInterruptedWorkAttemptUsesPrivateRoots(t *testing.T) {
	sharedHome := t.TempDir()
	sharedCache := filepath.Join(sharedHome, ".cache", "ddx", "exec-wt")
	require.NoError(t, os.MkdirAll(sharedCache, 0o755))
	sharedConfigDir := filepath.Join(sharedHome, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(sharedConfigDir, 0o755))
	// Host-looking global config that would redirect execution temp onto the
	// shared cache if the interrupt fixture failed to pin private roots.
	require.NoError(t, os.WriteFile(
		filepath.Join(sharedConfigDir, "config.yaml"),
		[]byte("version: \"1.0\"\nexecutions:\n  temp_worktree_root: "+sharedCache+"\n"),
		0o644,
	))

	// Poison process-global roots the way a host environment would.
	t.Setenv("HOME", sharedHome)
	t.Setenv("XDG_DATA_HOME", filepath.Join(sharedHome, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(sharedHome, ".cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(sharedHome, ".config"))
	t.Setenv(config.ExecutionWorktreeRootEnv, "")

	store, candidate, result, err := runInterruptedWorkAttempt(t)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, store)
	require.NotNil(t, candidate)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)
	assert.True(t, result.Results[0].Disrupted)

	// Store and config-derived roots stay outside the poisoned shared home.
	assertPathOutside(t, store.Dir, sharedHome)
	require.NotEqual(t, filepath.Clean(sharedHome), filepath.Clean(os.Getenv("HOME")))
	require.NotEqual(t, filepath.Clean(filepath.Join(sharedHome, ".cache")), filepath.Clean(os.Getenv("XDG_CACHE_HOME")))
	execWT := os.Getenv(config.ExecutionWorktreeRootEnv)
	require.NotEmpty(t, execWT)
	require.NotEqual(t, filepath.Clean(sharedCache), filepath.Clean(execWT))
	assertPathOutside(t, execWT, sharedHome)

	projectRoot := filepath.Dir(store.Dir)
	tempRoot := config.ExecutionTempRoot(projectRoot)
	scratchRoot := config.ExecutionScratchRoot(projectRoot)
	require.Equal(t, filepath.Clean(execWT), filepath.Clean(tempRoot))
	assertPathOutside(t, tempRoot, sharedHome)
	assertPathOutside(t, scratchRoot, sharedHome)
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(tempRoot))
	require.NotEqual(t, filepath.Clean(os.TempDir()), filepath.Clean(scratchRoot))

	// Interrupt semantics must still hold on the hermetic fixture.
	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
}

// TestWorkInterrupt_DoesNotSetNoProgressCooldown verifies that interrupted
// work stays off the work retry cooldown and does not increment the
// no-progress suppression path.
func TestWorkInterrupt_DoesNotSetNoProgressCooldown(t *testing.T) {
	store, candidate, result, err := runInterruptedWorkAttempt(t)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)
	report := result.Results[0]
	assert.True(t, report.Disrupted)
	assert.Equal(t, "context_canceled", report.DisruptionReason)
	assert.Empty(t, report.RetryAfter)

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	_, hasRetry := got.Extra["work-retry-after"]
	assert.False(t, hasRetry, "interrupted work must not persist work-retry-after")
}

func TestWorkInterrupt_NoChangesLikeCanceledAttemptDoesNotWriteTrackerNoise(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)
	cancelCtx, cancel := context.WithCancel(context.Background())
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			cancel()
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				Detail:    "context canceled",
				BaseRev:   "abc1234",
				ResultRev: "abc1234",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(cancelCtx, rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)
	assert.True(t, result.Results[0].Disrupted)
	assert.Equal(t, "context_canceled", result.Results[0].DisruptionReason)

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.NotContains(t, got.Labels, NoChangesLabelUnjustified)
	assert.NotContains(t, got.Labels, NoChangesLabelUnverified)
	assert.NotContains(t, got.Extra, "work-retry-after")
	assert.NotContains(t, got.Extra, "work-last-status")
	assert.NotContains(t, got.Extra, "work-no-changes-count")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotContains(t,
			[]string{"no_changes_unjustified", "execute-bead", "loop-error", "execution-routing-intent", "disruption_detected"},
			ev.Kind,
			"interrupted attempt must not write terminal/noise event %q", ev.Kind,
		)
	}
}

// TestWorkInterrupt_RemovesClaimLiveness verifies that cleanup removes the
// external claim heartbeat so the bead can be reclaimed immediately.
func TestWorkInterrupt_RemovesClaimLiveness(t *testing.T) {
	store, candidate, _, err := runInterruptedWorkAttempt(t)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}

	fresh, found, err := store.ClaimHeartbeatFresh(candidate.ID)
	require.NoError(t, err)
	assert.False(t, found, "interrupted work must remove the claim heartbeat file")
	assert.False(t, fresh, "removed heartbeat cannot be fresh")

	require.NoError(t, store.Claim(candidate.ID, "worker-b"), "bead must be re-claimable after cleanup")
	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "worker-b", got.Owner)
}

// TestInterruptedAfterTerminalMutation_DoesNotUndoClose verifies that a
// cancellation that lands after the bead has been successfully closed does not
// reopen or unclaim the bead.
func TestInterruptedAfterTerminalMutation_DoesNotUndoClose(t *testing.T) {
	realStore, candidate, _ := newExecuteLoopTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	store := &errorInjectingStore{
		ExecuteBeadLoopStore: realStore,
		onCloseWithEvidence: func(id, sessionID, commitSHA string) error {
			err := realStore.CloseWithEvidence(id, sessionID, commitSHA)
			cancel()
			return err
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-close",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{Once: true})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	got, err := realStore.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.Equal(t, "worker", got.Owner)

	fresh, found, err := realStore.ClaimHeartbeatFresh(candidate.ID)
	require.NoError(t, err)
	assert.False(t, found, "closed bead must not retain a live claim heartbeat")
	assert.False(t, fresh)
}

// TestClassifyDisruption_Markers asserts the transport-error marker set
// recognizes a representative sample of disruption-class strings, and that
// non-transport errors are not misclassified.
func TestClassifyDisruption_Markers(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		want   bool
		reason string
	}{
		{"connection_refused", errors.New("dial tcp: connection refused"), true, "transport_error"},
		{"connection_reset", errors.New("read: connection reset by peer"), true, "transport_error"},
		{"deadline_exceeded", errors.New("Post: context deadline exceeded"), true, "transport_error"},
		{"bad_gateway", errors.New("502 bad gateway from upstream"), true, "transport_error"},
		{"plain_error", errors.New("model declined to commit"), false, ""},
		{"nil_err", nil, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, ok := classifyDisruption(context.Background(), tc.err)
			assert.Equal(t, tc.want, ok)
			assert.Equal(t, tc.reason, reason)
		})
	}
}

// TestClassifyDisruption_ContextErrors covers the ctx.Err() branch of
// classifyDisruption: cancelled and deadline-exceeded contexts must classify
// as Disrupted regardless of executorErr.
func TestClassifyDisruption_ContextErrors(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reason, ok := classifyDisruption(ctx, nil)
		assert.True(t, ok)
		assert.Equal(t, "context_canceled", reason)
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		reason, ok := classifyDisruption(ctx, nil)
		assert.True(t, ok)
		assert.Equal(t, "context_deadline", reason)
	})
}
