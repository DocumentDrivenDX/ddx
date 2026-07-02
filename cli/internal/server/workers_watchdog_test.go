package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newIdleHandle returns a manually-constructed workerHandle registered with m
// for tests that drive the watchdog directly without starting a real loop.
func newIdleHandle(t *testing.T, m *WorkerManager, id string, beadID string, startedAt, lastPhaseTS time.Time) (*workerHandle, *atomic.Bool) {
	t.Helper()
	var cancelled atomic.Bool
	cancel := context.CancelFunc(func() { cancelled.Store(true) })
	h := &workerHandle{
		record: WorkerRecord{
			ID:          id,
			Kind:        "work",
			State:       "running",
			Status:      "running",
			ProjectRoot: m.projectRoot,
			StartedAt:   startedAt,
			CurrentBead: beadID,
			CurrentAttempt: &CurrentAttemptInfo{
				AttemptID: id + "-a1",
				BeadID:    beadID,
				Phase:     "running",
				StartedAt: startedAt,
			},
		},
		cancel:       cancel,
		progressDone: make(chan struct{}),
		lastPhaseTS:  lastPhaseTS,
	}
	m.mu.Lock()
	m.workers[id] = h
	m.mu.Unlock()
	return h, &cancelled
}

// seedClaimedBead creates a ready bead and claims it so Unclaim() has work
// to do. Returns the bead store.
func seedClaimedBead(t *testing.T, root string, beadID string) *bead.Store {
	t.Helper()
	ddx := testutils.MakeInitializedDDxRoot(t, root)
	store := bead.NewStore(ddx)
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        beadID,
		Title:     "watchdog test bead",
		Status:    bead.StatusOpen,
		IssueType: bead.DefaultType,
	}))
	require.NoError(t, store.Claim(beadID, "worker-test"))
	return store
}

// startManagedWatchdogTree launches a stubborn managed-worker-style process
// tree for watchdog reaping tests. The root process ignores SIGTERM and owns
// two descendant process groups whose children also need cleanup.
func startManagedWatchdogTree(t *testing.T) (binDir string, rootPID, claudePID, claudeSleepPID, codexPID, codexSleepPID int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not available: %v", err)
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skipf("setsid not available: %v", err)
	}

	binDir = t.TempDir()
	pidDir := t.TempDir()
	writeFakeProviderBinary(t, binDir, "claude-tree", true)
	writeFakeProviderBinary(t, binDir, "codex-tree", true)
	writeFakeProviderBinary(t, binDir, "claude", false)
	writeFakeProviderBinary(t, binDir, "codex", false)

	cmd := exec.Command("sh", "-c", `trap '' TERM INT; sh -c 'trap "" TERM INT; sleep 600' & claude-tree & codex-tree & wait`)
	cmd.Env = envWithOverrides(map[string]string{
		"PATH":    binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"PID_DIR": pidDir,
	})
	rootPID = startProcessGroup(t, cmd)

	claudePID = waitForPIDFile(t, filepath.Join(pidDir, "claude-tree.pid"))
	claudeSleepPID = waitForPIDFile(t, filepath.Join(pidDir, "claude-tree.sleep.pid"))
	codexPID = waitForPIDFile(t, filepath.Join(pidDir, "codex-tree.pid"))
	codexSleepPID = waitForPIDFile(t, filepath.Join(pidDir, "codex-tree.sleep.pid"))

	t.Cleanup(func() {
		_ = syscall.Kill(-claudePID, syscall.SIGKILL)
		_ = syscall.Kill(-codexPID, syscall.SIGKILL)
		_ = syscall.Kill(claudeSleepPID, syscall.SIGKILL)
		_ = syscall.Kill(codexSleepPID, syscall.SIGKILL)
	})
	return binDir, rootPID, claudePID, claudeSleepPID, codexPID, codexSleepPID
}

// newManagedIdleHandle marks a synthetic worker as managed so the watchdog
// exercises the subprocess-backed path. PID is deliberately zero to verify
// the PGID fallback that keeps process-tree cleanup working.
func newManagedIdleHandle(t *testing.T, m *WorkerManager, id string, beadID string, rootPID int, startedAt, lastPhaseTS time.Time) (*workerHandle, *atomic.Bool) {
	t.Helper()
	h, cancelled := newIdleHandle(t, m, id, beadID, startedAt, lastPhaseTS)
	m.mu.Lock()
	h.managed = true
	h.record.PID = 0
	h.record.PGID = rootPID
	m.mu.Unlock()
	return h, cancelled
}

// TestWatchdogSweepReapsStalledWorker is the core AC test: a worker whose
// runtime exceeds WatchdogDeadline AND whose current attempt has sat without
// a phase transition longer than StallDeadline is reaped by one sweep.
func TestWatchdogSweepReapsStalledWorker(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-wd-01")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	m.WatchdogKillGrace = 10 * time.Millisecond
	defer m.StopWatchdog()

	now := time.Now().UTC()
	// StartedAt is 1s in the past and lastPhaseTS is 1s in the past —
	// both exceed the 1ms deadlines.
	h, cancelled := newIdleHandle(t, m, "worker-wd-01", "ddx-wd-01",
		now.Add(-1*time.Second), now.Add(-1*time.Second))

	m.watchdogSweep(now)

	m.mu.Lock()
	reaped := h.reaped
	state := h.record.State
	reapReason := h.record.ReapReason
	m.mu.Unlock()

	assert.True(t, reaped, "handle should be flagged reaped")
	assert.Equal(t, "reaped", state, "record.State must flip to 'reaped'")
	assert.Equal(t, "watchdog", reapReason)
	assert.True(t, cancelled.Load(), "watchdog must invoke cancel() for in-process workers")

	// AC: bead claim must be released back to open.
	b, err := store.Get(context.Background(), "ddx-wd-01")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status,
		"bead must return to open after watchdog releases the claim")

	// AC: a bead.reaped event must be on the tracker with reason=watchdog
	// and a duration mentioned in the body.
	events, err := store.EventsByKind("ddx-wd-01", "bead.reaped")
	require.NoError(t, err)
	require.Len(t, events, 1, "expected exactly one bead.reaped event")
	assert.Equal(t, "watchdog", events[0].Summary)
	assert.Contains(t, events[0].Body, "runtime=")
	assert.Contains(t, events[0].Body, "stalled=")
	assert.Contains(t, events[0].Body, "reason=watchdog")
}

// TestWatchdogSweepSkipsHealthyWorker: a worker whose runtime is under
// WatchdogDeadline (or whose lastPhaseTS is fresh) must NOT be reaped.
func TestWatchdogSweepSkipsHealthyWorker(t *testing.T) {
	root := t.TempDir()
	seedClaimedBead(t, root, "ddx-wd-healthy")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Hour // far in the future
	m.StallDeadline = 1 * time.Hour
	defer m.StopWatchdog()

	now := time.Now().UTC()
	h, cancelled := newIdleHandle(t, m, "worker-wd-healthy", "ddx-wd-healthy",
		now.Add(-1*time.Second), now.Add(-1*time.Second))

	m.watchdogSweep(now)

	m.mu.Lock()
	reaped := h.reaped
	state := h.record.State
	m.mu.Unlock()

	assert.False(t, reaped, "healthy worker must not be flagged reaped")
	assert.Equal(t, "running", state)
	assert.False(t, cancelled.Load(), "cancel() must not fire on healthy worker")
}

// TestWatchdogSweepSkipsNoCurrentAttempt: a worker between beads (CurrentAttempt nil)
// has no phase to stall on; do not reap even if past WatchdogDeadline.
func TestWatchdogSweepSkipsNoCurrentAttempt(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	defer m.StopWatchdog()

	now := time.Now().UTC()
	h, _ := newIdleHandle(t, m, "worker-idle-ok", "", now.Add(-1*time.Second), now.Add(-1*time.Second))
	m.mu.Lock()
	h.record.CurrentAttempt = nil
	h.record.CurrentBead = ""
	m.mu.Unlock()

	m.watchdogSweep(now)

	m.mu.Lock()
	reaped := h.reaped
	m.mu.Unlock()
	assert.False(t, reaped, "idle worker with no current attempt must not be reaped")
}

// TestWatchdogSweepReapIsIdempotent: a second sweep over an already-reaped
// handle must not re-fire cancel() or emit a second bead.reaped event.
func TestWatchdogSweepReapIsIdempotent(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-wd-idem")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	m.WatchdogKillGrace = 1 * time.Millisecond
	defer m.StopWatchdog()

	now := time.Now().UTC()
	_, _ = newIdleHandle(t, m, "worker-idem", "ddx-wd-idem",
		now.Add(-1*time.Second), now.Add(-1*time.Second))

	m.watchdogSweep(now)
	m.watchdogSweep(now) // second sweep must be a no-op

	events, err := store.EventsByKind("ddx-wd-idem", "bead.reaped")
	require.NoError(t, err)
	assert.Len(t, events, 1, "second sweep must not double-emit bead.reaped")
}

// TestWatchdogDeadlinesConfigurable verifies that ddx server config values
// flow into the manager via LoadWithWorkingDir.
func TestWatchdogDeadlinesConfigurable(t *testing.T) {
	root := t.TempDir()
	ddx := filepath.Join(root, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddx, 0o755))
	// Minimal config.yaml with a server.watchdog_deadline / stall_deadline override.
	cfgPath := filepath.Join(ddx, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(
		"version: \"1.0\"\n"+
			"library:\n  path: .ddx/plugins/ddx\n  repository:\n    url: https://example/lib\n    branch: main\n"+
			"server:\n"+
			"  watchdog_deadline: 42m\n"+
			"  stall_deadline: 7m\n",
	), 0o644))

	m := NewWorkerManager(root)
	assert.Equal(t, 42*time.Minute, m.WatchdogDeadline,
		"WatchdogDeadline must come from config.server.watchdog_deadline")
	assert.Equal(t, 7*time.Minute, m.StallDeadline,
		"StallDeadline must come from config.server.stall_deadline")

	// Defaults are applied for unset fields.
	w, s, c, g := m.watchdogDeadlines()
	assert.Equal(t, 42*time.Minute, w)
	assert.Equal(t, 7*time.Minute, s)
	assert.Equal(t, defaultWatchdogCheckInterval, c)
	assert.Equal(t, defaultWatchdogKillGrace, g)
}

// TestWatchdogDeadlinesDefaultsWhenNoConfig: with no config at all, the
// manager uses the built-in defaults (6h / 1h / 1m / 30s).
func TestWatchdogDeadlinesDefaultsWhenNoConfig(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	w, s, c, g := m.watchdogDeadlines()
	assert.Equal(t, defaultWatchdogDeadline, w)
	assert.Equal(t, defaultStallDeadline, s)
	assert.Equal(t, defaultWatchdogCheckInterval, c)
	assert.Equal(t, defaultWatchdogKillGrace, g)
}

// TestWatchdogStartedOnFirstWorkerLaunch: sync.Once semantics — ensureWatchdog
// must only spawn one goroutine even if StartExecuteLoop is called many times.
func TestWatchdogStartedOnFirstWorkerLaunch(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	// Force long deadlines so the supervisor does not reap the test worker.
	m.WatchdogDeadline = 1 * time.Hour
	m.StallDeadline = 1 * time.Hour
	m.WatchdogCheckInterval = 1 * time.Hour
	defer m.StopWatchdog()

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Mode: "once"})
	require.NoError(t, err)
	_ = waitForWorkerExit(t, m, rec.ID, 10*time.Second)

	// ensureWatchdog is guarded by sync.Once — subsequent calls are no-ops.
	m.ensureWatchdog()
	m.ensureWatchdog()
	m.ensureWatchdog()
	// No direct way to count goroutines, but StopWatchdog must remain safe
	// and close() cannot panic twice because of the recover() guard.
	m.StopWatchdog()
	m.StopWatchdog() // idempotent
}

// TestWatchdogDoesNotReapFinishedWorker: a worker that has already exited
// (FinishedAt non-zero) is not a reap candidate.
func TestWatchdogDoesNotReapFinishedWorker(t *testing.T) {
	root := t.TempDir()
	seedClaimedBead(t, root, "ddx-wd-finished")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	defer m.StopWatchdog()

	now := time.Now().UTC()
	h, cancelled := newIdleHandle(t, m, "worker-done", "ddx-wd-finished",
		now.Add(-1*time.Second), now.Add(-1*time.Second))
	m.mu.Lock()
	h.record.FinishedAt = now.Add(-100 * time.Millisecond)
	m.mu.Unlock()

	m.watchdogSweep(now)

	m.mu.Lock()
	reaped := h.reaped
	m.mu.Unlock()
	assert.False(t, reaped, "finished worker must not be reaped")
	assert.False(t, cancelled.Load())
}

// TestWatchdogSIGTERMtoSIGKILLEscalationOnWedgedSubprocess is the
// process-level reaper AC: when the worker tracks a PID for a subprocess
// that ignores SIGTERM (and a goroutine that ignores ctx), the watchdog
// must escalate to SIGKILL within the grace window.
func TestWatchdogSIGTERMtoSIGKILLEscalationOnWedgedSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group signal semantics differ on Windows; covered by reaper file's platform-specific impl")
	}

	root := t.TempDir()
	seedClaimedBead(t, root, "ddx-wd-wedged")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	m.WatchdogKillGrace = 300 * time.Millisecond // enough for one SIGTERM poll, short enough for a fast test
	defer m.StopWatchdog()

	// Spawn a child process that ignores SIGTERM and would otherwise sleep
	// for 60s. A goroutine in the test represents "handler that ignores
	// ctx" — it waits on cmd.Wait() rather than any context, so only an
	// OS-level kill can shut it down.
	cmd := exec.Command("sh", "-c", `trap '' TERM; sleep 60`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	waitErrCh := make(chan error, 1)
	go func() {
		// This goroutine ignores ctx by design; it exits only when the OS
		// reaps the child. The watchdog must make that happen.
		waitErrCh <- cmd.Wait()
	}()

	// Manually register a workerHandle with the wedged subprocess's PID.
	now := time.Now().UTC()
	h, _ := newIdleHandle(t, m, "worker-wedged", "ddx-wd-wedged",
		now.Add(-1*time.Second), now.Add(-1*time.Second))
	m.mu.Lock()
	h.record.PID = cmd.Process.Pid
	m.mu.Unlock()

	m.watchdogSweep(now)

	// Within grace (+ slack) the child process must exit — via SIGKILL
	// because we trapped SIGTERM. Wait up to 3s as a generous slack.
	select {
	case err := <-waitErrCh:
		// Exit must have been signal-driven.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			require.True(t, ok, "expected syscall.WaitStatus")
			assert.True(t, ws.Signaled(), "process must have been terminated by a signal")
			// SIGKILL is the expected signal because SIGTERM was trapped.
			assert.Equal(t, syscall.SIGKILL, ws.Signal(),
				"wedged subprocess must receive SIGKILL after SIGTERM is ignored")
		} else if err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill() // cleanup safeguard
		t.Fatal("watchdog did not reap the wedged subprocess in time")
	}

	// Record state reflects the reap.
	m.mu.Lock()
	state := h.record.State
	m.mu.Unlock()
	assert.Equal(t, "reaped", state)
}

// TestWatchdogLoopPeriodicallyChecks: verify the goroutine-driven loop
// actually reaps a stalled worker without a manual watchdogSweep call.
func TestWatchdogLoopPeriodicallyChecks(t *testing.T) {
	root := t.TempDir()
	seedClaimedBead(t, root, "ddx-wd-loop")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	m.WatchdogCheckInterval = 20 * time.Millisecond
	m.WatchdogKillGrace = 10 * time.Millisecond
	defer m.StopWatchdog()

	now := time.Now().UTC()
	h, _ := newIdleHandle(t, m, "worker-loop", "ddx-wd-loop",
		now.Add(-1*time.Second), now.Add(-1*time.Second))

	// Starting the watchdog goroutine — normally gated behind the first
	// StartExecuteLoop call.
	m.ensureWatchdog()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		reaped := h.reaped
		m.mu.Unlock()
		if reaped {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	m.mu.Lock()
	reaped := h.reaped
	m.mu.Unlock()
	assert.True(t, reaped, "supervisor goroutine must reap stalled worker")
}

// TestDrainProgressUpdatesLastPhaseTS: drainProgress must refresh
// handle.lastPhaseTS on non-heartbeat events so the watchdog can measure
// the stall window.
func TestDrainProgressUpdatesLastPhaseTS(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	handle := &workerHandle{
		record:      WorkerRecord{ID: "w-phase-ts", Kind: "work", State: "running"},
		lastPhaseTS: time.Time{},
	}
	ch := make(chan agent.ProgressEvent, 4)
	go m.drainProgress("w-phase-ts", handle, ch)

	t0 := time.Now().UTC()
	ch <- agent.ProgressEvent{
		EventID: "e1", AttemptID: "a1", BeadID: "ddx-x", Phase: "queueing",
		PhaseSeq: 1, Heartbeat: false, TS: t0,
	}
	// Heartbeat must NOT advance lastPhaseTS.
	ch <- agent.ProgressEvent{
		EventID: "e2", AttemptID: "a1", BeadID: "ddx-x", Phase: "queueing",
		PhaseSeq: 1, Heartbeat: true, TS: t0.Add(5 * time.Second),
	}
	close(ch)

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		got := handle.lastPhaseTS
		m.mu.Unlock()
		if !got.IsZero() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	m.mu.Lock()
	got := handle.lastPhaseTS
	m.mu.Unlock()

	require.False(t, got.IsZero(), "lastPhaseTS must be set on first phase event")
	assert.True(t, got.Equal(t0), "heartbeat must not advance lastPhaseTS; got=%v want=%v", got, t0)
}

// TestManagedWorkerWatchdogReapKillsProcessTree verifies that the watchdog
// reaps a stalled managed worker, escalates to SIGKILL when the process tree
// ignores SIGTERM, and removes all managed descendants.
func TestManagedWorkerWatchdogReapKillsProcessTree(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-wd-managed-tree")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	_, rootPID, claudePID, claudeSleepPID, codexPID, codexSleepPID := startManagedWatchdogTree(t)
	now := time.Now().UTC()
	h, cancelled := newManagedIdleHandle(t, m, "worker-wd-managed-tree", "ddx-wd-managed-tree",
		rootPID, now.Add(-1*time.Second), now.Add(-1*time.Second))

	m.watchdogSweep(now)

	m.mu.Lock()
	reaped := h.reaped
	state := h.record.State
	reapReason := h.record.ReapReason
	lastError := h.record.LastError
	m.mu.Unlock()

	assert.True(t, reaped, "managed worker should be flagged reaped")
	assert.Equal(t, "reaped", state, "record.State must flip to 'reaped'")
	assert.Equal(t, "watchdog", reapReason)
	assert.True(t, cancelled.Load(), "watchdog must invoke cancel() for managed workers")
	assert.Contains(t, lastError, "cleanup=")
	assert.Contains(t, lastError, "sigkill=", "watchdog cleanup must escalate to SIGKILL for the stubborn root process")

	b, err := store.Get(context.Background(), "ddx-wd-managed-tree")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status, "watchdog must release the bead claim")

	events, err := store.EventsByKind("ddx-wd-managed-tree", "bead.reaped")
	require.NoError(t, err)
	require.Len(t, events, 1, "expected exactly one bead.reaped event")
	assert.Equal(t, "watchdog", events[0].Summary)

	waitForProcessGone(t, rootPID)
	waitForProcessGone(t, claudePID)
	waitForProcessGone(t, claudeSleepPID)
	waitForProcessGone(t, codexPID)
	waitForProcessGone(t, codexSleepPID)
}

// TestManagedWorkerWatchdogReapReleasesClaim verifies the claim is released
// and exactly one bead.reaped event is recorded when watchdog reaps a stalled
// managed worker.
func TestManagedWorkerWatchdogReapReleasesClaim(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-wd-managed-claim")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	_, rootPID, _, _, _, _ := startManagedWatchdogTree(t)
	now := time.Now().UTC()
	_, cancelled := newManagedIdleHandle(t, m, "worker-wd-managed-claim", "ddx-wd-managed-claim",
		rootPID, now.Add(-1*time.Second), now.Add(-1*time.Second))

	m.watchdogSweep(now)

	assert.True(t, cancelled.Load(), "watchdog must cancel managed workers before cleanup completes")

	b, err := store.Get(context.Background(), "ddx-wd-managed-claim")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status, "watchdog must unclaim the stalled bead")

	events, err := store.EventsByKind("ddx-wd-managed-claim", "bead.reaped")
	require.NoError(t, err)
	require.Len(t, events, 1, "expected exactly one bead.reaped event")
	assert.Equal(t, "watchdog", events[0].Summary)

	waitForProcessGone(t, rootPID)
}

// TestManagedWorkerWatchdogReapSkipsExternalReportedWorkers verifies that the
// watchdog only cleans the managed worker tree and leaves unrelated external
// worker reports and process groups alone.
func TestManagedWorkerWatchdogReapSkipsExternalReportedWorkers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}

	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-wd-managed-skip")

	m := NewWorkerManager(root)
	m.WatchdogDeadline = 1 * time.Millisecond
	m.StallDeadline = 1 * time.Millisecond
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	binDir, rootPID, claudePID, claudeSleepPID, codexPID, codexSleepPID := startManagedWatchdogTree(t)

	reportedCmd := exec.Command(filepath.Join(binDir, "claude"))
	reportedCmd.Env = envWithOverrides(map[string]string{
		"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	reportedPID := startProcessGroup(t, reportedCmd)

	reg := newWorkerIngestRegistry(root)
	rec := reg.register(workerIdentity{
		ProjectRoot:  root,
		Harness:      "claude",
		ExecutorPID:  reportedPID,
		ExecutorHost: "localhost",
		StartedAt:    time.Now().UTC(),
	})
	require.NotEmpty(t, rec.WorkerID)

	interactiveCmd := exec.Command(filepath.Join(binDir, "codex"))
	interactiveCmd.Env = envWithOverrides(map[string]string{
		"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	interactivePID := startProcessGroup(t, interactiveCmd)

	unrelatedCmd := exec.Command("sh", "-c", "sleep 600")
	unrelatedPID := startProcessGroup(t, unrelatedCmd)

	now := time.Now().UTC()
	_, cancelled := newManagedIdleHandle(t, m, "worker-wd-managed-skip", "ddx-wd-managed-skip",
		rootPID, now.Add(-1*time.Second), now.Add(-1*time.Second))

	m.watchdogSweep(now)

	assert.True(t, cancelled.Load(), "watchdog must cancel the managed worker")
	waitForProcessGone(t, rootPID)
	waitForProcessGone(t, claudePID)
	waitForProcessGone(t, claudeSleepPID)
	waitForProcessGone(t, codexPID)
	waitForProcessGone(t, codexSleepPID)

	if !testProcessAlive(reportedPID) {
		t.Fatalf("watchdog killed external reported worker pid %d", reportedPID)
	}
	if !testProcessAlive(interactivePID) {
		t.Fatalf("watchdog killed interactive Claude/Codex session pid %d", interactivePID)
	}
	if !testProcessAlive(unrelatedPID) {
		t.Fatalf("watchdog killed unrelated process pid %d", unrelatedPID)
	}

	b, err := store.Get(context.Background(), "ddx-wd-managed-skip")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status, "watchdog must release the bead claim")

	events, err := store.EventsByKind("ddx-wd-managed-skip", "bead.reaped")
	require.NoError(t, err)
	require.Len(t, events, 1, "watchdog must emit exactly one bead.reaped event")
	assert.Equal(t, "watchdog", events[0].Summary)
}
