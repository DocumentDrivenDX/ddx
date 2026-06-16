//go:build !windows

package server

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLockHoldingWorkerScript(t *testing.T, path string) {
	t.Helper()
	writeTestScript(t, path, `
mkdir -p .git
printf held > .git/index.lock
sleep 60 &
wait
`)
}

func findLifecycleEvent(events []WorkerLifecycleEvent, action string) *WorkerLifecycleEvent {
	for i := range events {
		if events[i].Action == action {
			return &events[i]
		}
	}
	return nil
}

func TestManagedWorkerDeathDuringIndexCommitRemovesStaleIndexLock(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	binDir := t.TempDir()
	workerPath := filepath.Join(binDir, "ddx-worker")
	writeLockHoldingWorkerScript(t, workerPath)

	m := NewWorkerManager(root)
	m.workerBinaryPath = workerPath
	m.WatchdogKillGrace = 100 * time.Millisecond
	defer m.StopAll()

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(time.Second),
	})
	require.NoError(t, err)
	require.NoError(t, m.MarkManaged(rec.ID))

	lockPath := filepath.Join(root, ".git", "index.lock")
	require.Eventually(t, func() bool {
		_, err := os.Stat(lockPath)
		return err == nil
	}, 5*time.Second, 25*time.Millisecond, "worker must leave a stale index.lock before shutdown")

	require.NoError(t, m.Stop(rec.ID))
	final := waitForWorkerExit(t, m, rec.ID, 5*time.Second)
	require.Equal(t, "stopped", final.State)

	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "managed worker cleanup must remove stale .git/index.lock without manual intervention")

	event := findLifecycleEvent(final.Lifecycle, "index-lock-violation")
	require.NotNil(t, event, "managed worker exit must record index-lock cleanup evidence")
	assert.Contains(t, event.Detail, "outcome=removed")
	assert.Contains(t, event.Detail, "operation=index.commit")
	assert.Contains(t, event.Detail, "lock_path="+lockPath)
	assert.Contains(t, event.Detail, "worker="+rec.ID)
}

func TestManagedWorkerDeathDuringIndexCommitDoesNotRemoveLiveHolderLock(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	lockPath := filepath.Join(root, ".git", "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))

	prevLookup := managedWorkerIndexLockOwnerLookup
	t.Cleanup(func() { managedWorkerIndexLockOwnerLookup = prevLookup })
	managedWorkerIndexLockOwnerLookup = func(string) (int, error) { return os.Getpid(), nil }

	event, ok := recoverManagedWorkerIndexLockAfterExit(root, "worker-live-holder", 0)
	require.True(t, ok, "cleanup path must emit evidence when the lock is still present")
	require.NotNil(t, event)

	_, statErr := os.Stat(lockPath)
	assert.NoError(t, statErr, "cleanup path must not remove a lock held by a live process")
	assert.Contains(t, event.Detail, "outcome=kept-live-owner")
	assert.Contains(t, event.Detail, "holder_pid="+strconv.Itoa(os.Getpid()))
	assert.Contains(t, event.Detail, "operation=index.commit")
}

func TestManagedWorkerDeathDuringIndexCommitRecordsLockViolation(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	lockPath := filepath.Join(root, ".git", "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))

	deadPID := 9999999
	if processAlive(deadPID) {
		t.Skip("test pid unexpectedly alive")
	}

	prevLookup := managedWorkerIndexLockOwnerLookup
	t.Cleanup(func() { managedWorkerIndexLockOwnerLookup = prevLookup })
	managedWorkerIndexLockOwnerLookup = func(string) (int, error) { return deadPID, nil }

	event, ok := recoverManagedWorkerIndexLockAfterExit(root, "worker-20260616T203031-1ef9", 0)
	require.True(t, ok, "cleanup path must emit evidence when removing a stale lock")
	require.NotNil(t, event)

	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "stale lock must be removed after worker death")
	assert.Equal(t, "index-lock-violation", event.Action)
	assert.Equal(t, "ddx-server", event.Actor)
	assert.Contains(t, event.Detail, "worker=worker-20260616T203031-1ef9")
	assert.Contains(t, event.Detail, "holder_pid="+strconv.Itoa(deadPID))
	assert.Contains(t, event.Detail, "lock_path="+lockPath)
	assert.Contains(t, event.Detail, "operation=index.commit")
	assert.Contains(t, event.Detail, "outcome=removed")
}
