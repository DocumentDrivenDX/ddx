package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the lock-scoping contract for ddx-329a08b8: the parent-repo
// git index lock (.git/index.lock) and the DDx tracker lock
// (.ddx/.git-tracker.lock) are acquired only for the brief git/tracker
// mutation windows (pre-dispatch checkpoint + post-subprocess commit/audit),
// never across the multi-minute LLM harness subprocess wait. Holding either
// lock across the subprocess (the original 2026-05-17 regression) blocks
// concurrent bead create/update/close with "tracker lock timeout" errors.

// lockScopeProbeRunner stands in for the LLM harness subprocess. While
// "running" it polls the parent-repo index lock and the DDx tracker lock to
// record whether either is held across the subprocess wait, and writes a
// tracked file into the worktree so the post-subprocess SynthesizeCommit
// performs a real git stage/commit.
type lockScopeProbeRunner struct {
	projectRoot string
	pollWindow  time.Duration

	mu              sync.Mutex
	subprocessStart time.Time
	subprocessEnd   time.Time
	indexLockHeld   bool
	trackerLockHeld bool
	polls           int
}

func (r *lockScopeProbeRunner) Run(opts RunArgs) (*Result, error) {
	indexLock := filepath.Join(r.projectRoot, ".git", "index.lock")
	trackerLock := ddxroot.JoinProject(r.projectRoot, ".git-tracker.lock")

	start := time.Now()
	r.mu.Lock()
	r.subprocessStart = start
	r.mu.Unlock()

	// Produce a real worktree change so the post-subprocess git stage/commit
	// (SynthesizeCommit) has tracked content to commit.
	if opts.WorkDir != "" {
		_ = os.WriteFile(filepath.Join(opts.WorkDir, "agent-change.txt"), []byte("change\n"), 0o644)
	}

	deadline := start.Add(r.pollWindow)
	for {
		now := time.Now()
		// index.lock counts as held-across-subprocess only when it exists with
		// an mtime at or after the subprocess started; a pre-existing lock with
		// an older mtime is permitted by the acceptance contract.
		if info, err := os.Stat(indexLock); err == nil && !info.ModTime().Before(start) {
			r.mu.Lock()
			r.indexLockHeld = true
			r.mu.Unlock()
		}
		// The tracker lock is a DDx-managed directory created only around a
		// mutation, so any presence during the subprocess means it spans the wait.
		if _, err := os.Stat(trackerLock); err == nil {
			r.mu.Lock()
			r.trackerLockHeld = true
			r.mu.Unlock()
		}
		r.mu.Lock()
		r.polls++
		r.mu.Unlock()
		if !now.Before(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	r.mu.Lock()
	r.subprocessEnd = time.Now()
	r.mu.Unlock()
	return &Result{ExitCode: 0}, nil
}

// lockSample records one tracker-lock acquire+release cycle observed via the
// TrackerLockMetricsSink seam, with the wall-clock release time so tests can
// place the acquisition relative to the subprocess window.
type lockSample struct {
	sample     TrackerLockSample
	releasedAt time.Time
}

type lockSampleRecorder struct {
	mu      sync.Mutex
	samples []lockSample
}

func (rec *lockSampleRecorder) record(s TrackerLockSample) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.samples = append(rec.samples, lockSample{sample: s, releasedAt: time.Now()})
}

func (rec *lockSampleRecorder) snapshot() []lockSample {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	out := make([]lockSample, len(rec.samples))
	copy(out, rec.samples)
	return out
}

// runLockScopeProbe drives a real ExecuteBeadWithConfig attempt against a real
// git repo with the worktree backend and a lockScopeProbeRunner subprocess,
// capturing tracker-lock acquisitions through the metrics sink. It returns the
// probe, the recorder, the result, and the project root.
func runLockScopeProbe(t *testing.T) (*lockScopeProbeRunner, *lockSampleRecorder, *ExecuteBeadResult, string) {
	t.Helper()

	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	// Enable worktree-scoped config before any worktree is added so the
	// execute-bead git-isolation step (which sets --worktree config) succeeds.
	runGitInteg(t, projectRoot, "config", "extensions.worktreeConfig", "true")

	// Seed durable DDx bookkeeping dirt so the locked pre-dispatch sequence has
	// a real tracker/checkpoint commit to make. This makes the lock genuinely
	// guard a git index mutation, so "not held across the subprocess" is a
	// meaningful assertion rather than a vacuous one.
	metricsPath := filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
	require.NoError(t, os.WriteFile(metricsPath, []byte(`{"seed":"lock-scope"}`+"\n"), 0o644))

	rec := &lockSampleRecorder{}
	prevSink := SetTrackerLockMetricsSink(rec.record)
	t.Cleanup(func() { SetTrackerLockMetricsSink(prevSink) })

	runner := &lockScopeProbeRunner{projectRoot: projectRoot, pollWindow: 200 * time.Millisecond}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg,
		ExecuteBeadRuntime{AgentRunner: runner}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)

	return runner, rec, res, projectRoot
}

// TestExecuteBead_IndexLockNotHeldAcrossSubprocess asserts that .git/index.lock
// is not freshly held for the duration of the harness subprocess wait.
func TestExecuteBead_IndexLockNotHeldAcrossSubprocess(t *testing.T) {
	runner, _, res, _ := runLockScopeProbe(t)

	runner.mu.Lock()
	indexHeld := runner.indexLockHeld
	polls := runner.polls
	runner.mu.Unlock()

	require.Greater(t, polls, 1, "probe must poll across the subprocess window")
	assert.NotEmpty(t, res.AttemptID, "run must have dispatched the subprocess")
	assert.False(t, indexHeld,
		".git/index.lock was held (mtime at/after subprocess start) during the harness subprocess wait")
}

// TestExecuteBead_TrackerLockNotHeldAcrossSubprocess asserts the same property
// for .ddx/.git-tracker.lock.
func TestExecuteBead_TrackerLockNotHeldAcrossSubprocess(t *testing.T) {
	runner, rec, _, _ := runLockScopeProbe(t)

	runner.mu.Lock()
	trackerHeld := runner.trackerLockHeld
	polls := runner.polls
	runner.mu.Unlock()

	require.Greater(t, polls, 1, "probe must poll across the subprocess window")
	assert.False(t, trackerHeld,
		".ddx/.git-tracker.lock was held during the harness subprocess wait")
	// Non-vacuity: the tracker lock IS exercised at least once (pre-dispatch
	// mutation), so its absence during the subprocess is a meaningful signal
	// rather than the lock never being used.
	assert.NotEmpty(t, rec.snapshot(),
		"tracker lock was never acquired; absence-during-subprocess would be vacuous")
}

// TestExecuteBead_LocksReacquiredForMutation asserts the locks ARE acquired
// (briefly) for the post-subprocess git stage/commit and tracker update, and
// that no acquisition straddles the subprocess wait window.
func TestExecuteBead_LocksReacquiredForMutation(t *testing.T) {
	runner, rec, res, projectRoot := runLockScopeProbe(t)

	// Post-subprocess git stage/commit: the worktree change written by the probe
	// was committed by SynthesizeCommit after the subprocess returned.
	assert.NotEqual(t, res.BaseRev, res.ResultRev,
		"post-subprocess git stage/commit did not advance the worktree HEAD")

	// Post-subprocess tracker update: the durable-audit commit re-acquires the
	// tracker lock (the same path FinalizeDurableAttemptAudit uses).
	require.NoError(t, CommitDurableAuditOutputs(projectRoot, res.AttemptID))

	samples := rec.snapshot()
	require.NotEmpty(t, samples, "tracker lock was never acquired for mutation")

	runner.mu.Lock()
	subStart := runner.subprocessStart
	subEnd := runner.subprocessEnd
	runner.mu.Unlock()
	require.False(t, subStart.IsZero(), "subprocess start not recorded")
	require.False(t, subEnd.IsZero(), "subprocess end not recorded")

	postSubprocess := 0
	for i, s := range samples {
		acquiredAt := s.releasedAt.Add(-s.sample.Hold)
		// No acquisition may straddle the subprocess wait: the lock is released
		// before the subprocess starts and (re)acquired only after it returns.
		spans := !acquiredAt.After(subStart) && !s.releasedAt.Before(subEnd)
		assert.Falsef(t, spans,
			"tracker lock acquisition #%d straddles the subprocess wait (acquired=%s released=%s sub=[%s,%s])",
			i, acquiredAt, s.releasedAt, subStart, subEnd)
		if acquiredAt.After(subEnd) {
			postSubprocess++
		}
	}
	assert.Positive(t, postSubprocess,
		"expected at least one tracker-lock acquisition for the post-subprocess mutation")
}

type signalPrepareBackend struct {
	inner   AttemptBackend
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (b *signalPrepareBackend) Name() string { return b.inner.Name() }

func (b *signalPrepareBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	if b.started != nil {
		b.once.Do(func() { close(b.started) })
	}
	if b.release != nil {
		<-b.release
	}
	return b.inner.Prepare(ctx, req)
}

func (b *signalPrepareBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return b.inner.Run(ctx, req)
}

func (b *signalPrepareBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.PublishResult(ctx, ws, res)
}

func (b *signalPrepareBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.Cleanup(ctx, ws)
}

type delayPrepareBackend struct {
	inner AttemptBackend
	delay time.Duration
}

func (b delayPrepareBackend) Name() string { return b.inner.Name() }

func (b delayPrepareBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	if b.delay > 0 {
		time.Sleep(b.delay)
	}
	return b.inner.Prepare(ctx, req)
}

func (b delayPrepareBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return b.inner.Run(ctx, req)
}

func (b delayPrepareBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	return b.inner.PublishResult(ctx, ws, res)
}

func (b delayPrepareBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.Cleanup(ctx, ws)
}

func trackerLockDurations(samples []lockSample, section string) []time.Duration {
	durations := make([]time.Duration, 0, len(samples))
	for _, s := range samples {
		if section != "" && s.sample.Section != section {
			continue
		}
		durations = append(durations, s.sample.Hold)
	}
	return durations
}

func percentileDuration(values []time.Duration, pct float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if pct <= 0 {
		return sorted[0]
	}
	if pct >= 1 {
		return sorted[len(sorted)-1]
	}
	rank := int(pct*float64(len(sorted)-1) + 0.999999999)
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func TestChaos_AttemptPrepareDoesNotHoldMainGitLockForSlowClone(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	runGitInteg(t, projectRoot, "config", "extensions.worktreeConfig", "true")

	prepareStarted := make(chan struct{})
	releasePrepare := make(chan struct{})
	backend := &signalPrepareBackend{
		inner:   WorktreeAttemptBackend{},
		started: prepareStarted,
		release: releasePrepare,
	}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	runErr := make(chan error, 1)
	go func() {
		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
			AgentRunner:    writeFileAgentRunner{},
			AttemptBackend: backend,
		}, &RealGitOps{})
		if err == nil && res == nil {
			err = fmt.Errorf("ExecuteBeadWithConfig returned nil result")
		}
		runErr <- err
	}()

	select {
	case <-prepareStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for Prepare to begin")
	}

	lockAcquired := make(chan error, 1)
	go func() {
		lockAcquired <- withMainGitLock(projectRoot, "chaos_prepare_probe", func() error { return nil })
	}()

	select {
	case err := <-lockAcquired:
		require.NoError(t, err, "main-git lock acquisition should not be blocked by Prepare")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for another goroutine to acquire the main-git lock")
	}

	close(releasePrepare)
	require.NoError(t, <-runErr)
}

func TestPerformance_PreDispatchMutationWindowP95UnderBudget(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	runGitInteg(t, projectRoot, "config", "extensions.worktreeConfig", "true")

	rec := &lockSampleRecorder{}
	prevSink := SetTrackerLockMetricsSink(rec.record)
	t.Cleanup(func() { SetTrackerLockMetricsSink(prevSink) })

	const runs = 3
	backend := delayPrepareBackend{
		inner: WorktreeAttemptBackend{},
		delay: 2500 * time.Millisecond,
	}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	const beadID = "ddx-int-0001"

	metricsPath := filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl")
	for i := 0; i < runs; i++ {
		require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
		require.NoError(t, os.WriteFile(metricsPath, []byte(fmt.Sprintf(`{"seed":"pre-dispatch-%d"}`+"\n", i)), 0o644))

		res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
			AgentRunner:    writeFileAgentRunner{},
			AttemptBackend: backend,
		}, &RealGitOps{})
		require.NoErrorf(t, err, "run %d should complete successfully", i)
		require.NotNilf(t, res, "run %d should produce a result", i)
		require.NotEmptyf(t, res.ResultRev, "run %d should advance the worktree HEAD", i)
	}

	durations := trackerLockDurations(rec.snapshot(), "pre_dispatch_commits")
	require.NotEmpty(t, durations, "pre-dispatch tracker lock samples were not recorded")

	p95 := percentileDuration(durations, 0.95)
	max := percentileDuration(durations, 1.0)
	t.Logf("pre-dispatch tracker lock hold times: samples=%d p95=%s max=%s", len(durations), p95, max)
	require.Less(t, p95, 2*time.Second, "p95 tracker-lock hold time regressed")
	require.Less(t, max, 5*time.Second, "max tracker-lock hold time regressed")
}
