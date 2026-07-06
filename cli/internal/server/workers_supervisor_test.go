package server

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerDesiredStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 2
	desired.DefaultSpec.Harness = "fiz"
	desired.DefaultSpec.Model = "qwen/qwen3.6"
	desired.DefaultSpec.Provider = "openrouter"
	desired.DefaultSpec.Profile = "default"
	desired.DefaultSpec.LabelFilter = "phase:reliability"
	desired.DefaultSpec.Mode = executeloop.ModeWatch
	desired.DefaultSpec.IdleInterval = executeloop.Duration{Duration: 45 * time.Second}
	desired.Restart.Enabled = true
	desired.Restart.MaxRestartsPerHour = 4
	desired.Restart.Backoff = executeloop.Duration{Duration: 25 * time.Second}
	desired.Restart.BackoffMax = executeloop.Duration{Duration: 5 * time.Minute}

	require.NoError(t, supervisor.SaveDesiredState(&desired))

	loaded, err := supervisor.LoadDesiredState()
	require.NoError(t, err)
	require.NoError(t, loaded.Validate())

	assert.Equal(t, root, loaded.ProjectRoot)
	assert.Equal(t, 2, loaded.DesiredCount)
	assert.Equal(t, "fiz", loaded.DefaultSpec.Harness)
	assert.Equal(t, "qwen/qwen3.6", loaded.DefaultSpec.Model)
	assert.Equal(t, "openrouter", loaded.DefaultSpec.Provider)
	assert.Equal(t, "default", loaded.DefaultSpec.Profile)
	assert.Equal(t, "phase:reliability", loaded.DefaultSpec.LabelFilter)
	assert.Equal(t, executeloop.ModeWatch, loaded.DefaultSpec.Mode)
	assert.Equal(t, 45*time.Second, loaded.DefaultSpec.IdleInterval.Duration)
	assert.Equal(t, executeloop.SpecCurrentVersion, loaded.DefaultSpec.SpecVersion)
	assert.True(t, loaded.Restart.Enabled)
	assert.Equal(t, 4, loaded.Restart.MaxRestartsPerHour)
	assert.Equal(t, 25*time.Second, loaded.Restart.Backoff.Duration)
	assert.Equal(t, 5*time.Minute, loaded.Restart.BackoffMax.Duration)
	assert.True(t, loaded.UpdatedAt.Equal(desired.UpdatedAt))
}

func TestWorkerSupervisorReconcileStartsAndStopsToDesiredCount(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installBlockingWorkerFactory(m)
	t.Cleanup(func() {
		stopProjectWorkers(t, m, root)
	})

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 2
	desired.DefaultSpec.OpaquePassthrough = true
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	seed, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:              executeloop.ModeWatch,
		IdleInterval:      executeLoopIdleInterval(30 * time.Second),
		OpaquePassthrough: true,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)

	require.NoError(t, supervisor.Reconcile())
	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 2
	}, 2*time.Second, 20*time.Millisecond)

	workers := runningManagedWorkers(t, m, root)
	require.Len(t, workers, 2)
	assert.Equal(t, seed.ID, workers[1].ID, "seed worker should remain the older worker")
	newestID := workers[0].ID

	desired.DesiredCount = 1
	require.NoError(t, supervisor.SaveDesiredState(&desired))
	require.NoError(t, supervisor.Reconcile())

	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		rec, err := m.Show(newestID)
		if err != nil {
			return false
		}
		return rec.State != "running"
	}, 2*time.Second, 20*time.Millisecond)

	oldest, err := m.Show(seed.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", oldest.State)
}

func TestWorkerSupervisorRestartBackoff(t *testing.T) {
	t.Run("respects backoff before restarting", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		desired.Restart.Enabled = true
		desired.Restart.MaxRestartsPerHour = 6
		desired.Restart.Backoff = executeloop.Duration{Duration: 150 * time.Millisecond}
		desired.Restart.BackoffMax = executeloop.Duration{Duration: 150 * time.Millisecond}
		require.NoError(t, supervisor.SaveDesiredState(&desired))

		seedTerminalFailedWorker(t, m, root, "worker-20260627T000001-failed")

		require.NoError(t, supervisor.Reconcile())
		assert.Zero(t, runningManagedWorkerCount(t, m, root))

		time.Sleep(200 * time.Millisecond)
		require.NoError(t, supervisor.Reconcile())

		require.Eventually(t, func() bool {
			return runningManagedWorkerCount(t, m, root) == 1
		}, 2*time.Second, 20*time.Millisecond)
	})

	t.Run("caps restarts per hour", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		desired.Restart.Enabled = true
		desired.Restart.MaxRestartsPerHour = 1
		desired.Restart.Backoff = executeloop.Duration{Duration: 10 * time.Millisecond}
		desired.Restart.BackoffMax = executeloop.Duration{Duration: 10 * time.Millisecond}
		require.NoError(t, supervisor.SaveDesiredState(&desired))

		seedTerminalFailedWorker(t, m, root, "worker-20260627T000002-failed")
		seedTerminalFailedWorker(t, m, root, "worker-20260627T000003-failed")

		require.NoError(t, supervisor.Reconcile())
		assert.Zero(t, runningManagedWorkerCount(t, m, root))
	})
}

func TestWorkerSupervisorMarksStaleRunningRecordsStopped(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-supervisor-stale")
	writeStaleClaimLeaseForTest(t, store, bead.ClaimLeaseRecord{
		BeadID:    "ddx-supervisor-stale",
		Owner:     "worker-test",
		Machine:   "stale-machine",
		StartedAt: time.Now().UTC().Add(-3 * time.Hour),
		UpdatedAt: time.Now().UTC().Add(-3 * time.Hour),
		PID:       9999998,
	})

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 0
	desired.Restart.Enabled = false
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	workerID := "worker-20260627T000004-stale"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	stale := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC().Add(-1 * time.Hour),
		PID:         0,
		CurrentBead: "ddx-supervisor-stale",
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: workerID + "-a1",
			BeadID:    "ddx-supervisor-stale",
			Phase:     "running",
			StartedAt: time.Now().UTC().Add(-1 * time.Hour),
		},
	}
	require.NoError(t, m.writeRecord(dir, stale))

	require.NoError(t, supervisor.Reconcile())

	rec, err := m.readRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
	assert.False(t, rec.FinishedAt.IsZero())
	assert.False(t, m.hasWorkerHandle(workerID), "stale disk entry must not be adopted into memory")

	b, err := store.Get(context.Background(), "ddx-supervisor-stale")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)
}

func TestSupervisorReconcile_FreshClaimingWorkerSurvivesTicks(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.Restart.Enabled = false
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	store := bead.NewStore(ddxroot.JoinProject(root))
	beadID := "ddx-supervisor-fresh-claim"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        beadID,
		Title:     "fresh claim survives reconcile",
		Status:    bead.StatusOpen,
		Priority:  1,
		IssueType: bead.DefaultType,
	}))
	require.NoError(t, store.Claim(beadID, "ddx"))

	now := time.Now().UTC()
	liveID := "worker-20260705T000000-live"
	liveHandle, _ := newManagedIdleHandle(t, m, liveID, beadID, os.Getpid(), now.Add(-2*time.Second), now.Add(-time.Second))
	liveHandle.record.CurrentAttempt = &CurrentAttemptInfo{
		AttemptID: liveID + "-a1",
		BeadID:    beadID,
		Phase:     "running",
		StartedAt: now.Add(-2 * time.Second),
	}
	liveDir := filepath.Join(m.rootDir, liveID)
	require.NoError(t, os.MkdirAll(liveDir, 0o755))
	require.NoError(t, m.writeRecord(liveDir, liveHandle.record))

	staleID := "worker-20260705T000001-stale"
	staleDir := filepath.Join(m.rootDir, staleID)
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	stale := WorkerRecord{
		ID:          staleID,
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   now.Add(-5 * time.Minute),
		PID:         0,
		CurrentBead: beadID,
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: staleID + "-a1",
			BeadID:    beadID,
			Phase:     "running",
			StartedAt: now.Add(-5 * time.Minute),
		},
	}
	require.NoError(t, m.writeRecord(staleDir, stale))

	for i := 0; i < 3; i++ {
		require.NoError(t, supervisor.ReconcileAt(now.Add(time.Duration(i)*time.Second)))
	}

	liveRec, err := m.readRecord(liveDir)
	require.NoError(t, err)
	assert.Equal(t, "running", liveRec.State)

	staleRec, err := m.readRecord(staleDir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", staleRec.State)
	assert.Equal(t, "stopped", staleRec.Status)

	b, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusInProgress, b.Status)

	lease, found, err := store.ClaimLease(beadID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "ddx", lease.Owner)
	assert.True(t, m.hasWorkerHandle(liveID))
}

func installBlockingWorkerFactory(m *WorkerManager) {
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				<-ctx.Done()
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: ctx.Err().Error(),
				}, ctx.Err()
			}),
		}
	}
}

func seedTerminalFailedWorker(t *testing.T, m *WorkerManager, root, workerID string) {
	t.Helper()
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	now := time.Now().UTC()
	rec := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "failed",
		Status:      "failed",
		ProjectRoot: root,
		StartedAt:   now.Add(-2 * time.Minute),
		FinishedAt:  now,
		LastError:   "injected test failure",
	}
	require.NoError(t, m.writeRecord(dir, rec))
}

func runningManagedWorkerCount(t *testing.T, m *WorkerManager, projectRoot string) int {
	t.Helper()
	recs := runningManagedWorkers(t, m, projectRoot)
	return len(recs)
}

func runningManagedWorkers(t *testing.T, m *WorkerManager, projectRoot string) []WorkerRecord {
	t.Helper()
	recs, err := m.List()
	require.NoError(t, err)

	out := make([]WorkerRecord, 0, len(recs))
	for _, rec := range recs {
		if rec.Kind != "work" || rec.ProjectRoot != projectRoot || rec.State != "running" {
			continue
		}
		out = append(out, rec)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

func stopProjectWorkers(t *testing.T, m *WorkerManager, projectRoot string) {
	t.Helper()

	recs, err := m.List()
	if err != nil {
		return
	}
	for _, rec := range recs {
		if rec.Kind != "work" || rec.ProjectRoot != projectRoot {
			continue
		}
		if rec.State != "running" && rec.State != "stopping" {
			continue
		}
		_ = m.Stop(rec.ID)
	}

	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, projectRoot) == 0
	}, 2*time.Second, 20*time.Millisecond)
}
