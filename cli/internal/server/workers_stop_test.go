package server

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerManagerStopSetsStoppedState covers the primary AC:
// `ddx agent workers stop <id>` (via WorkerManager.Stop) gracefully
// terminates a running worker and updates its state to "stopped", distinct
// from "failed" and "exited".
func TestWorkerManagerStopSetsStoppedState(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	// Keep the worker alive long enough to observe the stop path.
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		PollInterval: 30 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, m.Stop(record.ID))

	final := waitForWorkerExit(t, m, record.ID, 5*time.Second)
	assert.Equal(t, "stopped", final.State,
		"Stop must flip WorkerRecord.State to 'stopped' (not 'exited' or 'failed')")
	assert.Equal(t, "stopped", final.Status)
	assert.False(t, final.FinishedAt.IsZero(), "FinishedAt must be set on stop")
}

// TestWorkerManagerStopIsIdempotent verifies that a second Stop call is a
// no-op — matching the watchdog reap semantics.
func TestWorkerManagerStopIsIdempotent(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		PollInterval: 30 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, m.Stop(record.ID))
	// Second call must not return an error, even though the worker is
	// already flagged stopped.
	require.NoError(t, m.Stop(record.ID))
}

// TestWorkerManagerStopUnknownWorker: calling Stop on an unknown id must
// return an error so operators can distinguish typos from stale workers.
func TestWorkerManagerStopUnknownWorker(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	err := m.Stop("worker-does-not-exist")
	require.Error(t, err)
}

// TestWorkerManagerStopReleasesBeadClaim: when the worker has claimed a bead,
// Stop must release the claim (status=open) and emit a bead.stopped event
// so operators can see why the worker was terminated.
func TestWorkerManagerStopReleasesBeadClaim(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-stop-claim")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	// Manually register an idle handle with a claimed bead. This is the
	// same trick the watchdog tests use to drive the termination path
	// without running a full execute-loop.
	now := time.Now().UTC()
	h, cancelled := newIdleHandle(t, m, "worker-stop-claim", "ddx-stop-claim",
		now.Add(-1*time.Second), now.Add(-1*time.Second))

	require.NoError(t, m.Stop("worker-stop-claim"))

	// State must flip to "stopped".
	m.mu.Lock()
	state := h.record.State
	status := h.record.Status
	finishedAt := h.record.FinishedAt
	m.mu.Unlock()
	assert.Equal(t, "stopped", state)
	assert.Equal(t, "stopped", status)
	assert.False(t, finishedAt.IsZero())
	assert.True(t, cancelled.Load(), "Stop must invoke cancel() so in-process code exits")

	// Bead claim must be released back to open.
	b, err := store.Get("ddx-stop-claim")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status,
		"bead must return to open after Stop releases the claim")

	// A bead.stopped event must be on the tracker with reason=stop and
	// the expected body shape (runtime + pid + reason).
	events, err := store.EventsByKind("ddx-stop-claim", "bead.stopped")
	require.NoError(t, err)
	require.Len(t, events, 1, "expected exactly one bead.stopped event")
	assert.Equal(t, "stop", events[0].Summary)
	assert.Contains(t, events[0].Body, "runtime=")
	assert.Contains(t, events[0].Body, "pid=")
	assert.Contains(t, events[0].Body, "reason=stop")
}

// TestWorkerManagerStopPersistsStoppedToDisk: the graceful path writes the
// final record to disk so a later `ddx agent workers show <id>` (or the
// worker-list sweep) reports state=stopped even after the process exits.
func TestWorkerManagerStopPersistsStoppedToDisk(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	// The manager writes records into <rootDir>/<id>/status.json; for the
	// idle-handle shortcut we pre-create that directory so writeRecord can
	// land its payload.
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, "worker-stopping-disk"), 0o755))

	now := time.Now().UTC()
	_, _ = newIdleHandle(t, m, "worker-stopping-disk", "",
		now.Add(-1*time.Second), now.Add(-1*time.Second))

	require.NoError(t, m.Stop("worker-stopping-disk"))

	rec, err := m.readRecord(filepath.Join(m.rootDir, "worker-stopping-disk"))
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
}

// TestWorkerManagerStopSIGTERMtoSIGKILL: when the worker tracks a PID whose
// process ignores SIGTERM, Stop must escalate to SIGKILL within the grace
// window. This matches the watchdog's process-level reaper semantics and
// proves the shared kill path works for operator-driven stops too.
func TestWorkerManagerStopSIGTERMtoSIGKILL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group signal semantics differ on Windows; covered separately")
	}

	root := t.TempDir()
	seedClaimedBead(t, root, "ddx-stop-wedge")

	m := NewWorkerManager(root)
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	// Child process that traps SIGTERM and would otherwise sleep 60s.
	cmd := exec.Command("sh", "-c", `trap '' TERM; sleep 60`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	waitErrCh := make(chan error, 1)
	go func() { waitErrCh <- cmd.Wait() }()

	now := time.Now().UTC()
	h, _ := newIdleHandle(t, m, "worker-stop-wedge", "ddx-stop-wedge",
		now.Add(-1*time.Second), now.Add(-1*time.Second))
	m.mu.Lock()
	h.record.PID = cmd.Process.Pid
	m.mu.Unlock()

	require.NoError(t, m.Stop("worker-stop-wedge"))

	// The child must be terminated within grace + slack. Because SIGTERM
	// was trapped, the signal that actually kills it must be SIGKILL.
	select {
	case err := <-waitErrCh:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			require.True(t, ok, "expected syscall.WaitStatus")
			assert.True(t, ws.Signaled(), "process must have been terminated by a signal")
			assert.Equal(t, syscall.SIGKILL, ws.Signal(),
				"wedged subprocess must receive SIGKILL after SIGTERM is ignored")
		} else if err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill() // cleanup safeguard
		t.Fatal("Stop did not escalate to SIGKILL within grace+slack")
	}

	m.mu.Lock()
	state := h.record.State
	m.mu.Unlock()
	assert.Equal(t, "stopped", state)
}

// TestRunWorkerPreservesStoppedState: when runWorker finishes after Stop()
// has already flipped state=stopped, its final record write must keep the
// terminal state rather than overwriting it with "exited" or "failed".
// This is the state-preservation fix that makes the AC provable.
func TestRunWorkerPreservesStoppedState(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	// Keep the worker alive so we can stop it while it is still polling.
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		PollInterval: 30 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, m.Stop(record.ID))
	final := waitForWorkerExit(t, m, record.ID, 5*time.Second)

	// The runWorker goroutine has finished and written its own record.
	// Without the preservation fix, the final State would be "exited" or
	// "failed"; with the fix, it remains "stopped".
	assert.Equal(t, "stopped", final.State,
		"runWorker must preserve 'stopped' state when Stop has already terminalized the record")
}
