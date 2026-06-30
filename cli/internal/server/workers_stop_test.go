package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerManagerStopSetsStoppedState covers the primary AC:
// `legacy agent workers stop <id>` (via WorkerManager.Stop) gracefully
// terminates a running worker and updates its state to "stopped", distinct
// from "failed" and "exited".
func TestWorkerManagerStopSetsStoppedState(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()
	// Keep the worker alive long enough to observe the stop path.
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.NoError(t, err)

	require.NoError(t, m.Stop(record.ID))

	final := waitForWorkerExit(t, m, record.ID, 5*time.Second)
	assert.Equal(t, "stopped", final.State,
		"Stop must flip WorkerRecord.State to 'stopped' (not 'exited' or 'failed')")
	assert.Equal(t, "stopped", final.Status)
	assert.False(t, final.FinishedAt.IsZero(), "FinishedAt must be set on stop")
	require.Len(t, final.Lifecycle, 2)
	assert.Equal(t, "start", final.Lifecycle[0].Action)
	assert.Equal(t, "local-operator", final.Lifecycle[0].Actor)
	assert.Equal(t, "stop", final.Lifecycle[1].Action)
	assert.Equal(t, "local-operator", final.Lifecycle[1].Actor)
}

// TestWorkerManagerStopIsIdempotent verifies that a second Stop call is a
// no-op — matching the watchdog reap semantics.
func TestWorkerManagerStopIsIdempotent(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
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
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	err := m.Stop("worker-does-not-exist")
	require.Error(t, err)
}

func TestWorkerDispatchAdapterStopWorkerUsesWorkerManagerStop(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, "worker-graphql-stop"), 0o755))
	now := time.Now().UTC()
	handle, cancelled := newIdleHandle(t, m, "worker-graphql-stop", "", now.Add(-time.Second), now.Add(-time.Second))

	result, err := (&workerDispatchAdapter{manager: m}).StopWorker(t.Context(), "worker-graphql-stop")
	require.NoError(t, err)
	assert.Equal(t, "worker-graphql-stop", result.ID)
	assert.Equal(t, "stopped", result.State)
	assert.True(t, cancelled.Load(), "GraphQL stop adapter must invoke WorkerManager.Stop cancellation")

	m.mu.Lock()
	state := handle.record.State
	m.mu.Unlock()
	assert.Equal(t, "stopped", state)
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
	// without running a full work.
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
	b, err := store.Get(context.Background(), "ddx-stop-claim")
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
// final record to disk so a later `legacy agent workers show <id>` (or the
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
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
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

func TestManagedWorkerStopKillsClaudeCodexDescendants(t *testing.T) {
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

	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-stop-tree")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-stop-tree"
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, workerID), 0o755))

	pidDir := t.TempDir()
	binDir := t.TempDir()
	writeFakeProviderBinary(t, binDir, "claude", true)
	writeFakeProviderBinary(t, binDir, "codex", true)

	cmd := exec.Command("sh", "-c", "claude & codex & wait")
	cmd.Env = envWithOverrides(map[string]string{
		"PATH":    binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"PID_DIR": pidDir,
	})
	rootPID := startProcessGroup(t, cmd)

	now := time.Now().UTC()
	handle, cancelled := newIdleHandle(t, m, workerID, "ddx-stop-tree", now.Add(-time.Second), now.Add(-time.Second))
	m.mu.Lock()
	handle.record.PID = rootPID
	m.mu.Unlock()

	claudePID := waitForPIDFile(t, filepath.Join(pidDir, "claude.pid"))
	claudeSleepPID := waitForPIDFile(t, filepath.Join(pidDir, "claude.sleep.pid"))
	codexPID := waitForPIDFile(t, filepath.Join(pidDir, "codex.pid"))
	codexSleepPID := waitForPIDFile(t, filepath.Join(pidDir, "codex.sleep.pid"))
	t.Cleanup(func() {
		_ = syscall.Kill(-claudePID, syscall.SIGKILL)
		_ = syscall.Kill(-codexPID, syscall.SIGKILL)
		_ = syscall.Kill(claudeSleepPID, syscall.SIGKILL)
		_ = syscall.Kill(codexSleepPID, syscall.SIGKILL)
	})

	require.NoError(t, m.Stop(workerID))

	assert.True(t, cancelled.Load(), "Stop must cancel the worker goroutine")
	waitForProcessGone(t, rootPID)
	waitForProcessGone(t, claudePID)
	waitForProcessGone(t, claudeSleepPID)
	waitForProcessGone(t, codexPID)
	waitForProcessGone(t, codexSleepPID)

	b, err := store.Get(context.Background(), "ddx-stop-tree")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status, "Stop must release the bead claim")

	events, err := store.EventsByKind("ddx-stop-tree", "bead.stopped")
	require.NoError(t, err)
	require.Len(t, events, 1, "Stop must emit one bead.stopped event")
	assert.Contains(t, events[0].Body, "reason=stop")

	rec, err := m.readRecord(filepath.Join(m.rootDir, workerID))
	require.NoError(t, err)
	require.Len(t, rec.Lifecycle, 1)
	assert.Equal(t, "stop", rec.Lifecycle[0].Action)
	assert.Contains(t, rec.Lifecycle[0].Detail, "cleanup=")
}

func TestManagedWorkerStopIsIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}

	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-stop-idem")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-stop-idem"
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, workerID), 0o755))

	cmd := exec.Command("sh", "-c", "sleep 600")
	rootPID := startProcessGroup(t, cmd)

	now := time.Now().UTC()
	handle, cancelled := newIdleHandle(t, m, workerID, "ddx-stop-idem", now.Add(-time.Second), now.Add(-time.Second))
	m.mu.Lock()
	handle.record.PID = rootPID
	m.mu.Unlock()

	require.NoError(t, m.Stop(workerID))
	require.NoError(t, m.Stop(workerID))

	assert.True(t, cancelled.Load(), "Stop must cancel the worker goroutine")
	waitForProcessGone(t, rootPID)

	b, err := store.Get(context.Background(), "ddx-stop-idem")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status, "bead claim must release exactly once")

	events, err := store.EventsByKind("ddx-stop-idem", "bead.stopped")
	require.NoError(t, err)
	require.Len(t, events, 1, "second Stop must not duplicate bead.stopped")

	rec, err := m.readRecord(filepath.Join(m.rootDir, workerID))
	require.NoError(t, err)
	require.Len(t, rec.Lifecycle, 1, "second Stop must not add another lifecycle entry")
	assert.Equal(t, "stop", rec.Lifecycle[0].Action)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
}

func TestManagedWorkerStopSkipsExternalReportedWorkers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}

	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-stop-skips")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-stop-skips"
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, workerID), 0o755))

	managedCmd := exec.Command("sh", "-c", "sleep 600")
	managedPID := startProcessGroup(t, managedCmd)

	now := time.Now().UTC()
	handle, _ := newIdleHandle(t, m, workerID, "ddx-stop-skips", now.Add(-time.Second), now.Add(-time.Second))
	m.mu.Lock()
	handle.record.PID = managedPID
	m.mu.Unlock()

	binDir := t.TempDir()
	writeFakeProviderBinary(t, binDir, "claude", false)
	writeFakeProviderBinary(t, binDir, "codex", false)

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
		StartedAt:    now,
	})
	require.NotEmpty(t, rec.WorkerID)

	interactiveCmd := exec.Command(filepath.Join(binDir, "codex"))
	interactiveCmd.Env = envWithOverrides(map[string]string{
		"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	interactivePID := startProcessGroup(t, interactiveCmd)

	unrelatedCmd := exec.Command("sh", "-c", "sleep 600")
	unrelatedPID := startProcessGroup(t, unrelatedCmd)

	require.NoError(t, m.Stop(workerID))

	waitForProcessGone(t, managedPID)
	if !testProcessAlive(reportedPID) {
		t.Fatalf("Stop killed external reported worker pid %d", reportedPID)
	}
	if !testProcessAlive(interactivePID) {
		t.Fatalf("Stop killed interactive Claude/Codex session pid %d", interactivePID)
	}
	if !testProcessAlive(unrelatedPID) {
		t.Fatalf("Stop killed unrelated process pid %d", unrelatedPID)
	}

	b, err := store.Get(context.Background(), "ddx-stop-skips")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)

	events, err := store.EventsByKind("ddx-stop-skips", "bead.stopped")
	require.NoError(t, err)
	require.Len(t, events, 1, "managed worker stop should emit one bead.stopped event")
}

func startProcessGroup(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", cmd.String(), err)
	}
	pid := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	})
	return pid
}

func envWithOverrides(overrides map[string]string) []string {
	env := os.Environ()
	for key, value := range overrides {
		env = setEnvValue(env, key, value)
	}
	return env
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func writeFakeProviderBinary(t *testing.T, dir, name string, spawnChildGroup bool) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := `#!/bin/sh
set -eu
pid_dir="${PID_DIR:-}"
name="` + name + `"
`
	if spawnChildGroup {
		if _, err := exec.LookPath("setsid"); err != nil {
			t.Skipf("setsid not available: %v", err)
		}
		script += `
printf '%s\n' "$$" > "$pid_dir/` + name + `.pid"
exec setsid sh -c '
  set -eu
  pid_dir=$1
  name=$2
  printf "%s\n" "$$" > "$pid_dir/${name}.group.pid"
  sleep 600 &
  sleep_pid=$!
  printf "%s\n" "$sleep_pid" > "$pid_dir/${name}.sleep.pid"
  trap "kill \"$sleep_pid\" 2>/dev/null || true; wait \"$sleep_pid\" 2>/dev/null || true; exit 0" TERM INT
  wait "$sleep_pid"
' sh "$pid_dir" "$name"
`
	} else {
		script += `
sleep 600
`
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(script, "\r\n", "\n")), 0o755); err != nil {
		t.Fatalf("write fake provider %s: %v", name, err)
	}
	return path
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pid file %s was not written", path)
	return 0
}

func testProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func waitForProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !testProcessAlive(pid) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("process pid %d still alive after cleanup", pid)
}
