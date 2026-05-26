package agent

import (
	"context"
	"os"
	"path/filepath"
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
	hold       time.Duration
	releasedAt time.Time
}

type lockSampleRecorder struct {
	mu      sync.Mutex
	samples []lockSample
}

func (rec *lockSampleRecorder) record(s TrackerLockSample) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.samples = append(rec.samples, lockSample{hold: s.Hold, releasedAt: time.Now()})
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
	prevSink := TrackerLockMetricsSink
	TrackerLockMetricsSink = rec.record
	t.Cleanup(func() { TrackerLockMetricsSink = prevSink })

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
		acquiredAt := s.releasedAt.Add(-s.hold)
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
