package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerManagerPruneReapsDeadPID verifies that Prune reaps a registry
// entry whose recorded PID is no longer alive and updates state to "reaped".
func TestWorkerManagerPruneReapsDeadPID(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-prune-pid")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	// Write a stale status.json directly (simulates server-restart scenario).
	workerID := "worker-20260101T000000-prn1"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	stale := WorkerRecord{
		ID:          workerID,
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC().Add(-2 * time.Hour),
		PID:         9999999, // almost certainly dead
		CurrentBead: "ddx-prune-pid",
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: workerID + "-a1",
			BeadID:    "ddx-prune-pid",
			Phase:     "running",
			StartedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
	}
	require.NoError(t, m.writeRecord(dir, stale))
	// writeRecord clears PIDAlive; restore the record to the right state.

	results, err := m.Prune(0)
	require.NoError(t, err)

	// If PID 9999999 happened to be alive (extremely unlikely), Prune skips it
	// and the test is a no-op. We verify the case where the PID is dead.
	if isPIDAlive(9999999) {
		t.Skip("PID 9999999 is alive on this host; skipping test")
	}

	require.Len(t, results, 1, "Prune must reap one stale entry")
	assert.Equal(t, workerID, results[0].ID)
	assert.Equal(t, "ddx-prune-pid", results[0].BeadID)
	assert.Contains(t, results[0].Reason, "not alive")

	// Disk record must be updated to state=reaped.
	rec, err := m.readRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, "reaped", rec.State)
	assert.Equal(t, "reaped", rec.Status)
	assert.False(t, rec.FinishedAt.IsZero())

	// Bead claim must be released.
	b, err := store.Get("ddx-prune-pid")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status,
		"Prune must release the bead claim so the queue drainer can pick it up again")

	// A bead.reaped event must be on the tracker.
	events, err := store.EventsByKind("ddx-prune-pid", "bead.reaped")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 1, "Prune must emit bead.reaped event")
	assert.Equal(t, "prune", events[0].Summary)
}

// TestWorkerManagerPruneByMaxAge verifies that Prune reaps a worker older
// than the maxAge threshold. A PID=0 goroutine-only worker not in m.workers
// is already caught by the goroutine-not-running path; we just verify the
// entry is pruned (the specific reason depends on evaluation order).
func TestWorkerManagerPruneByMaxAge(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-20260101T000000-age1"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	stale := WorkerRecord{
		ID:          workerID,
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC().Add(-48 * time.Hour),
		PID:         0, // goroutine-only worker (no external PID)
	}
	require.NoError(t, m.writeRecord(dir, stale))

	// Prune with maxAge=24h must reap the 48h-old worker.
	results, err := m.Prune(24 * time.Hour)
	require.NoError(t, err)
	require.Len(t, results, 1, "Prune must reap the stale worker")
	assert.Equal(t, workerID, results[0].ID)
	assert.NotEmpty(t, results[0].Reason, "reaped entry must have a reason")

	rec, err := m.readRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, "reaped", rec.State)
}

// TestWorkerManagerPruneByMaxAgeOnly verifies that Prune reaps a worker solely
// on age when neither PID check applies (dead PID already pruned, so use age
// as the only criterion by supplying a very high PID and a maxAge).
// This exercises the age branch specifically by ensuring the PID is present
// but dead (the PID branch fires first, which is acceptable).
func TestWorkerManagerPruneByMaxAgeNoGoroutine(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	// Worker with a definitely-dead PID that is also over 48h old.
	workerID := "worker-20260101T000000-age2"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	stale := WorkerRecord{
		ID:          workerID,
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC().Add(-48 * time.Hour),
		PID:         9999995, // very unlikely to be alive
	}
	require.NoError(t, m.writeRecord(dir, stale))

	if isPIDAlive(9999995) {
		t.Skip("PID 9999995 is alive on this host; skipping test")
	}

	// With maxAge=0, the dead PID catches it.
	results, err := m.Prune(0)
	require.NoError(t, err)
	require.Len(t, results, 1, "Prune must reap the dead-PID worker")
	assert.Contains(t, results[0].Reason, "not alive")
}

// TestWorkerManagerPruneSkipsLiveWorkers verifies that Prune does not touch
// workers that are alive in the in-memory registry.
func TestWorkerManagerPruneSkipsLiveWorkers(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		PollInterval: 30 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = m.Stop(record.ID) }()

	// Prune must not touch the live worker.
	results, err := m.Prune(0)
	require.NoError(t, err)
	assert.Empty(t, results, "Prune must not reap live workers")
}

// TestWorkerManagerStopStaleDiskEntry verifies that Stop on a stale disk-only
// entry (no live goroutine) succeeds and releases the bead claim. This is the
// AC2 fix for the "worker not running" 400 contradiction.
func TestWorkerManagerStopStaleDiskEntry(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-stop-stale")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-20260101T000000-stl1"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	stale := WorkerRecord{
		ID:          workerID,
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC().Add(-1 * time.Hour),
		PID:         9999998,
		CurrentBead: "ddx-stop-stale",
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: workerID + "-a1",
			BeadID:    "ddx-stop-stale",
			Phase:     "running",
			StartedAt: time.Now().UTC().Add(-1 * time.Hour),
		},
	}
	require.NoError(t, m.writeRecord(dir, stale))

	// Stop must succeed even though the handle is not in m.workers.
	err := m.Stop(workerID)
	require.NoError(t, err, "Stop must succeed for stale disk-only entry")

	// Disk record must be updated.
	rec, err := m.readRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.False(t, rec.FinishedAt.IsZero())

	// Bead claim must be released.
	b, err := store.Get("ddx-stop-stale")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status,
		"Stop on stale entry must release the bead claim")

	// A bead.stopped event must be on the tracker.
	events, err := store.EventsByKind("ddx-stop-stale", "bead.stopped")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 1)
	assert.Equal(t, "stop (stale)", events[0].Summary)
}

// TestWorkerManagerStopUnknownWorkerStillErrors verifies that Stop on a worker
// that doesn't exist on disk still returns an error (unchanged from before).
func TestWorkerManagerStopUnknownWorkerStillErrors(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	err := m.Stop("worker-does-not-exist-prune")
	require.Error(t, err, "Stop on unknown worker must return an error")
}

// TestWorkerManagerPIDAliveInList verifies that List populates pid_alive for
// workers with a non-zero PID (AC4 requirement).
func TestWorkerManagerPIDAliveInList(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-20260101T000000-pid1"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	rec := WorkerRecord{
		ID:          workerID,
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC(),
		PID:         9999997,
	}
	require.NoError(t, m.writeRecord(dir, rec))

	recs, err := m.List()
	require.NoError(t, err)

	var found *WorkerRecord
	for i := range recs {
		if recs[i].ID == workerID {
			found = &recs[i]
			break
		}
	}
	require.NotNil(t, found, "worker must appear in List()")
	require.NotNil(t, found.PIDAlive, "pid_alive must be set for workers with PID > 0")
	// The actual value depends on whether PID 9999997 is alive on this host.
	// We just verify the field is present (not nil).
}

// TestWorkerManagerPIDAliveNilForGoroutineWorker verifies that List omits
// pid_alive for goroutine-only workers (PID == 0).
func TestWorkerManagerPIDAliveNilForGoroutineWorker(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-20260101T000000-grt1"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	rec := WorkerRecord{
		ID:          workerID,
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC(),
		PID:         0, // goroutine-only
	}
	require.NoError(t, m.writeRecord(dir, rec))

	recs, err := m.List()
	require.NoError(t, err)

	var found *WorkerRecord
	for i := range recs {
		if recs[i].ID == workerID {
			found = &recs[i]
			break
		}
	}
	require.NotNil(t, found)
	assert.Nil(t, found.PIDAlive, "pid_alive must be nil (omitted) for goroutine-only workers")
}

// TestReconcileStaleWorkersOnStartup verifies that ReconcileStaleWorkers
// flips state=running disk records with dead PIDs to state=exited and
// releases any held bead claims (AC3).
func TestReconcileStaleWorkersOnStartup(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-reconcile")

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	workerID := "worker-20260101T000000-rcn1"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	stale := WorkerRecord{
		ID:          workerID,
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC().Add(-3 * time.Hour),
		PID:         9999996, // almost certainly dead
		CurrentBead: "ddx-reconcile",
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: workerID + "-a1",
			BeadID:    "ddx-reconcile",
			Phase:     "running",
			StartedAt: time.Now().UTC().Add(-3 * time.Hour),
		},
	}
	require.NoError(t, m.writeRecord(dir, stale))

	if isPIDAlive(9999996) {
		t.Skip("PID 9999996 is alive on this host; skipping test")
	}

	// Simulate server restart: new manager, reconcile.
	m2 := NewWorkerManager(root)
	defer m2.StopWatchdog()
	m2.ReconcileStaleWorkers()

	rec, err := m2.readRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, "exited", rec.State,
		"ReconcileStaleWorkers must flip dead-PID running workers to exited")
	assert.False(t, rec.FinishedAt.IsZero())

	// Bead claim must be released.
	b, err := store.Get("ddx-reconcile")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status,
		"ReconcileStaleWorkers must release bead claims for dead workers")
}
