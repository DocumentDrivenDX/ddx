package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedWorkerRecordsProcessGroup(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	t.Setenv("DDX_BIN", testutils.BuildDDxBinary(t))

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.enableManagedLaunch()

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         executeloop.ModeWatch,
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, "running", record.State)
	require.Greater(t, record.PID, 0)
	require.Greater(t, record.PGID, 0)
	assert.Equal(t, record.PID, record.PGID, "managed worker should record the process-group root pid")
	assert.True(t, record.ServerManaged, "managed launch must set ServerManaged ownership marker")
	assert.True(t, record.ownsProcessBoundary())

	statusPath := filepath.Join(ddxroot.JoinProject(root, "workers", record.ID), "status.json")
	data, err := os.ReadFile(statusPath)
	require.NoError(t, err)

	var persisted WorkerRecord
	require.NoError(t, json.Unmarshal(data, &persisted))
	assert.Equal(t, record.PID, persisted.PID)
	assert.Equal(t, record.PGID, persisted.PGID)
	assert.True(t, persisted.ServerManaged)
	assert.NotZero(t, persisted.PID)
	assert.NotZero(t, persisted.PGID)

	t.Cleanup(func() {
		_ = cleanupManagedWorkerProcessTree(record.PID, nil, 0)
		_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)
	})
}

// TestManagedWorkerProcessGroupLaunchFailureLeavesActionableStatus proves a
// subprocess launch failure does not leave a server-managed worker looking
// live with PID zero / missing PGID. The pre-launch "running" snapshot is
// replaced by a terminal failed status with an actionable error.
func TestManagedWorkerProcessGroupLaunchFailureLeavesActionableStatus(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	missingBinary := filepath.Join(root, "no-such-ddx-binary")
	t.Setenv("DDX_BIN", missingBinary)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.enableManagedLaunch()

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         executeloop.ModeWatch,
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DDX_BIN")
	assert.Empty(t, record.ID, "failed launch must not return a live worker record")
	assert.Zero(t, record.PID)
	assert.Zero(t, record.PGID)

	// In-memory map must not retain a running handle for the failed launch.
	m.mu.Lock()
	activeCount := len(m.workers)
	m.mu.Unlock()
	assert.Zero(t, activeCount, "failed launch must not leave an active worker handle")

	listed, listErr := m.List()
	require.NoError(t, listErr)
	require.NotEmpty(t, listed, "launch failure should leave an actionable status.json on disk")

	foundFailed := false
	for _, rec := range listed {
		if rec.State == "running" {
			t.Fatalf("launch failure left a running worker looking live: id=%s pid=%d pgid=%d", rec.ID, rec.PID, rec.PGID)
		}
		if rec.State != "failed" {
			continue
		}
		foundFailed = true
		assert.Equal(t, "failed", rec.Status)
		assert.Zero(t, rec.PID, "failed launch must not look live with a process id")
		assert.Zero(t, rec.PGID, "failed launch must not look live with a process group")
		assert.NotEmpty(t, rec.Error, "failed launch must leave an actionable error")
		assert.Contains(t, rec.Error, "DDX_BIN")

		statusPath := filepath.Join(ddxroot.JoinProject(root, "workers", rec.ID), "status.json")
		data, readErr := os.ReadFile(statusPath)
		require.NoError(t, readErr)

		var persisted WorkerRecord
		require.NoError(t, json.Unmarshal(data, &persisted))
		assert.Equal(t, "failed", persisted.State)
		assert.Equal(t, "failed", persisted.Status)
		assert.Zero(t, persisted.PID)
		assert.Zero(t, persisted.PGID)
		assert.NotEmpty(t, persisted.Error)
		assert.Contains(t, persisted.Error, "DDX_BIN")
	}
	assert.True(t, foundFailed, "expected a failed worker status after launch failure")
}

// TestManagedWorkerProcessBoundarySkipsExternalReportedWorkers proves
// process-boundary recording (PID/PGID + ServerManaged) is applied only to
// server-owned work workers, not external reported workers or interactive
// Claude/Codex sessions.
func TestManagedWorkerProcessBoundarySkipsExternalReportedWorkers(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	t.Setenv("DDX_BIN", testutils.BuildDDxBinary(t))

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.enableManagedLaunch()

	managed, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         executeloop.ModeWatch,
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.NoError(t, err)
	require.True(t, managed.ServerManaged)
	require.Greater(t, managed.PID, 0)
	require.Greater(t, managed.PGID, 0)
	t.Cleanup(func() {
		_ = cleanupManagedWorkerProcessTree(managed.PID, nil, 0)
		_ = waitForWorkerExit(t, m, managed.ID, 10*time.Second)
	})

	// In-process StartExecuteLoop (managed launch disabled) must not invent
	// process-group ownership for goroutine workers.
	inProcess := NewWorkerManager(root)
	defer inProcess.StopWatchdog()
	inProcessRecord, err := inProcess.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         executeloop.ModeWatch,
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.NoError(t, err)
	assert.False(t, inProcessRecord.ServerManaged)
	assert.Zero(t, inProcessRecord.PID)
	assert.Zero(t, inProcessRecord.PGID)
	assert.False(t, inProcessRecord.ownsProcessBoundary())
	t.Cleanup(func() {
		_ = inProcess.Stop(inProcessRecord.ID)
		_ = waitForWorkerExit(t, inProcess, inProcessRecord.ID, 5*time.Second)
	})

	// External reported workers retain executor_pid on the ingest identity
	// only — never as WorkerRecord process-boundary ownership.
	reg := newWorkerIngestRegistry(root)
	reported := reg.register(workerIdentity{
		ProjectRoot:  root,
		Harness:      "claude",
		ExecutorPID:  424242,
		ExecutorHost: "localhost",
		StartedAt:    time.Now().UTC(),
	})
	require.Equal(t, 424242, reported.Identity.ExecutorPID)

	// Interactive sessions are also outside the managed WorkerRecord model.
	// A disk status.json that happens to mention a PID without ServerManaged
	// must not look server-owned.
	externalDir := filepath.Join(m.rootDir, "worker-external-report")
	require.NoError(t, os.MkdirAll(externalDir, 0o755))
	externalRec := WorkerRecord{
		ID:          "worker-external-report",
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		// Observed PID only — no PGID ownership and no ServerManaged marker.
		PID:       515151,
		StartedAt: time.Now().UTC(),
	}
	require.NoError(t, m.writeRecord(externalDir, externalRec))
	persisted, err := m.readRecord(externalDir)
	require.NoError(t, err)
	assert.Equal(t, 515151, persisted.PID)
	assert.Zero(t, persisted.PGID, "external reports must not invent PGID ownership")
	assert.False(t, persisted.ServerManaged)
	assert.False(t, persisted.ownsProcessBoundary())

	// Managed launch remains the only path that records a full process boundary.
	assert.True(t, managed.ownsProcessBoundary())
	assert.NotEqual(t, managed.ID, reported.WorkerID)
}

// TestManagedWorkerStopTargetsOnlyServerOwnedProcessGroups proves stop/reap
// process-group targeting is gated by server-managed ownership and does not
// target externally reported worker PIDs.
func TestManagedWorkerStopTargetsOnlyServerOwnedProcessGroups(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is covered by Unix implementation")
	}

	root := t.TempDir()
	setupBeadStore(t, root)
	store := seedClaimedBead(t, root, "ddx-boundary-stop")

	m := NewWorkerManager(root)
	m.WatchdogKillGrace = 300 * time.Millisecond
	defer m.StopWatchdog()

	managedCmd := exec.Command("sh", "-c", "sleep 600")
	managedPID := startProcessGroup(t, managedCmd)

	// A non-owned handle that merely carries a PID must not be process-group
	// targeted by Stop.
	externalCmd := exec.Command("sh", "-c", "sleep 600")
	externalPID := startProcessGroup(t, externalCmd)

	now := time.Now().UTC()
	owned, _ := newIdleHandle(t, m, "worker-owned-boundary", "ddx-boundary-stop",
		now.Add(-time.Second), now.Add(-time.Second))
	m.mu.Lock()
	markServerOwnedProcessBoundary(t, owned, managedPID)
	m.mu.Unlock()

	unownedID := "worker-unowned-pid"
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, unownedID), 0o755))
	unowned, _ := newIdleHandle(t, m, unownedID, "", now.Add(-time.Second), now.Add(-time.Second))
	m.mu.Lock()
	unowned.record.PID = externalPID
	// Intentionally leave ServerManaged=false and managed=false.
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
	reported := reg.register(workerIdentity{
		ProjectRoot:  root,
		Harness:      "claude",
		ExecutorPID:  reportedPID,
		ExecutorHost: "localhost",
		StartedAt:    now,
	})
	require.NotEmpty(t, reported.WorkerID)

	interactiveCmd := exec.Command(filepath.Join(binDir, "codex"))
	interactiveCmd.Env = envWithOverrides(map[string]string{
		"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	interactivePID := startProcessGroup(t, interactiveCmd)

	// Stop the server-owned worker: its process group must die.
	require.NoError(t, m.Stop(owned.record.ID))
	waitForProcessGone(t, managedPID)

	// Stop the unowned PID-bearing handle: cancel only, no process kill.
	require.NoError(t, m.Stop(unownedID))
	if !testProcessAlive(externalPID) {
		t.Fatalf("Stop process-group targeted unowned external PID %d", externalPID)
	}

	// Reap path must also refuse unowned process-group targeting.
	reapPIDCmd := exec.Command("sh", "-c", "sleep 600")
	reapPID := startProcessGroup(t, reapPIDCmd)
	reapHandle, _ := newIdleHandle(t, m, "worker-unowned-reap", "ddx-boundary-stop",
		now.Add(-time.Second), now.Add(-time.Second))
	m.mu.Lock()
	reapHandle.record.PID = reapPID
	m.mu.Unlock()
	m.reapWorker(reapHandle.record.ID, reapHandle, reapPID, "", time.Hour, time.Hour, "watchdog")
	if !testProcessAlive(reapPID) {
		t.Fatalf("reap process-group targeted unowned PID %d", reapPID)
	}

	if !testProcessAlive(reportedPID) {
		t.Fatalf("stop/reap killed external reported worker pid %d", reportedPID)
	}
	if !testProcessAlive(interactivePID) {
		t.Fatalf("stop/reap killed interactive Claude/Codex session pid %d", interactivePID)
	}

	b, err := store.Get(context.Background(), "ddx-boundary-stop")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)

	// Cleanup survivors so the test process does not leak sleep children.
	t.Cleanup(func() {
		_ = cleanupManagedWorkerProcessTree(externalPID, nil, 0)
		_ = cleanupManagedWorkerProcessTree(reapPID, nil, 0)
		_ = cleanupManagedWorkerProcessTree(reportedPID, nil, 0)
		_ = cleanupManagedWorkerProcessTree(interactivePID, nil, 0)
	})
}

// TestManagedWorkerExternalReportsKeepExistingPIDSemantics proves external
// worker reports continue to persist their current PID/status behavior without
// invented PGID ownership.
func TestManagedWorkerExternalReportsKeepExistingPIDSemantics(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	reg := newWorkerIngestRegistry(root)
	now := time.Now().UTC()
	reported := reg.register(workerIdentity{
		ProjectRoot:  root,
		Harness:      "claude",
		Model:        "sonnet",
		ExecutorPID:  777001,
		ExecutorHost: "host-a",
		StartedAt:    now,
	})
	require.NotEmpty(t, reported.WorkerID)
	assert.Equal(t, 777001, reported.Identity.ExecutorPID)
	assert.Equal(t, "claude", reported.Identity.Harness)
	assert.Equal(t, "host-a", reported.Identity.ExecutorHost)

	// Snapshot must surface the same executor PID without promoting the
	// report into a managed WorkerRecord process boundary.
	snap := reg.snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, 777001, snap[0].Identity.ExecutorPID)
	assert.Equal(t, reported.WorkerID, snap[0].WorkerID)

	// Disk status for a non-managed report may carry an observed PID for
	// display/liveness, but must not invent PGID or ServerManaged ownership.
	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	dir := filepath.Join(m.rootDir, "worker-reported-only")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	rec := WorkerRecord{
		ID:          "worker-reported-only",
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		Harness:     "claude",
		PID:         777001,
		StartedAt:   now,
	}
	require.NoError(t, m.writeRecord(dir, rec))

	// Round-trip: PID preserved, PGID/ServerManaged remain empty/false.
	data, err := os.ReadFile(filepath.Join(dir, "status.json"))
	require.NoError(t, err)
	var persisted WorkerRecord
	require.NoError(t, json.Unmarshal(data, &persisted))
	assert.Equal(t, 777001, persisted.PID)
	assert.Equal(t, "running", persisted.State)
	assert.Equal(t, "running", persisted.Status)
	assert.Zero(t, persisted.PGID)
	assert.False(t, persisted.ServerManaged)
	assert.False(t, persisted.ownsProcessBoundary())

	// Explicitly assert the JSON does not invent pgid/server_managed keys.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasPGID := raw["pgid"]
	_, hasManaged := raw["server_managed"]
	assert.False(t, hasPGID, "external report must not invent pgid field")
	assert.False(t, hasManaged, "external report must not invent server_managed field")
}
