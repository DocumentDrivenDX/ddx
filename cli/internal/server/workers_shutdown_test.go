package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerManagerShutdownStopsManagedProcessTrees(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}

	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-shutdown-tree")

	m := NewWorkerManager(root)
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	_, rootPID, claudePID, claudeSleepPID, codexPID, codexSleepPID := startManagedWatchdogTree(t)

	workerID := "worker-shutdown-tree"
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, workerID), 0o755))
	startedAt := time.Now().UTC().Add(-time.Minute)
	record := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   startedAt,
		PID:         rootPID,
		PGID:        rootPID,
		CurrentBead: "ddx-shutdown-tree",
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: workerID + "-a1",
			BeadID:    "ddx-shutdown-tree",
			Phase:     "running",
			StartedAt: startedAt,
		},
		Lifecycle: []WorkerLifecycleEvent{{
			Action:    "start",
			Actor:     "local-operator",
			Timestamp: startedAt,
			Detail:    "kind=work",
		}},
	}
	require.NoError(t, m.writeRecord(filepath.Join(m.rootDir, workerID), record))

	require.NoError(t, m.Shutdown())

	waitForProcessGone(t, rootPID)
	waitForProcessGone(t, claudePID)
	waitForProcessGone(t, claudeSleepPID)
	waitForProcessGone(t, codexPID)
	waitForProcessGone(t, codexSleepPID)

	b, err := store.Get(context.Background(), "ddx-shutdown-tree")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)

	events, err := store.EventsByKind("ddx-shutdown-tree", "bead.stopped")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "shutdown", events[0].Summary)
	assert.Contains(t, events[0].Body, "reason=shutdown")

	rec, err := m.readRecord(filepath.Join(m.rootDir, workerID))
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
	assert.False(t, rec.FinishedAt.IsZero())
	require.Len(t, rec.Lifecycle, 2)
	assert.Equal(t, "start", rec.Lifecycle[0].Action)
	assert.Equal(t, "stop", rec.Lifecycle[1].Action)
	assert.Contains(t, rec.Lifecycle[1].Detail, "cleanup=")
}

func TestWorkerManagerShutdownSkipsExternalReportedWorkers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}

	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-shutdown-skips")

	m := NewWorkerManager(root)
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	binDir, rootPID, claudePID, claudeSleepPID, codexPID, codexSleepPID := startManagedWatchdogTree(t)

	workerID := "worker-shutdown-skips"
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, workerID), 0o755))
	startedAt := time.Now().UTC().Add(-time.Minute)
	handle, cancelled := newManagedIdleHandle(t, m, workerID, "ddx-shutdown-skips", rootPID, startedAt, startedAt)
	m.mu.Lock()
	handle.record.PID = rootPID
	handle.record.PGID = rootPID
	handle.record.Lifecycle = []WorkerLifecycleEvent{{
		Action:    "start",
		Actor:     "local-operator",
		Timestamp: startedAt,
		Detail:    "kind=work",
	}}
	m.mu.Unlock()

	reportedCmd := exec.Command(filepath.Join(binDir, "claude"))
	reportedPID := startProcessGroup(t, reportedCmd)
	reportedReg := newWorkerIngestRegistry(root)
	reportedRec := reportedReg.register(workerIdentity{
		ProjectRoot:  root,
		Harness:      "claude",
		ExecutorPID:  reportedPID,
		ExecutorHost: "localhost",
		StartedAt:    startedAt,
	})
	require.NotEmpty(t, reportedRec.WorkerID)

	interactiveCmd := exec.Command(filepath.Join(binDir, "codex"))
	interactivePID := startProcessGroup(t, interactiveCmd)

	unrelatedCmd := exec.Command("sh", "-c", "sleep 600")
	unrelatedPID := startProcessGroup(t, unrelatedCmd)

	require.NoError(t, m.Shutdown())

	waitForProcessGone(t, rootPID)
	waitForProcessGone(t, claudePID)
	waitForProcessGone(t, claudeSleepPID)
	waitForProcessGone(t, codexPID)
	waitForProcessGone(t, codexSleepPID)

	if !testProcessAlive(reportedPID) {
		t.Fatalf("Shutdown killed external reported worker pid %d", reportedPID)
	}
	if !testProcessAlive(interactivePID) {
		t.Fatalf("Shutdown killed interactive Claude/Codex session pid %d", interactivePID)
	}
	if !testProcessAlive(unrelatedPID) {
		t.Fatalf("Shutdown killed unrelated process pid %d", unrelatedPID)
	}

	b, err := store.Get(context.Background(), "ddx-shutdown-skips")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)

	events, err := store.EventsByKind("ddx-shutdown-skips", "bead.stopped")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "stop", events[0].Summary)

	rec, err := m.readRecord(filepath.Join(m.rootDir, workerID))
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
	assert.False(t, rec.FinishedAt.IsZero())
	assert.True(t, cancelled.Load(), "Shutdown must cancel the worker goroutine")
}
