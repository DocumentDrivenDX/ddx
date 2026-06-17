//go:build !windows

package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerServiceRestart_ReconcilesDesiredWorkers(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	binDir := t.TempDir()
	workerPath := filepath.Join(binDir, "ddx-worker")
	writeSleepingWorkerScript(t, workerPath)

	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
		DesiredCount: 1,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
		Restart:      WorkerRestartPolicy{Enabled: true},
	}))

	firstManager := NewWorkerManager(root)
	firstManager.workerBinaryPath = workerPath
	firstManager.WatchdogKillGrace = 100 * time.Millisecond
	t.Cleanup(firstManager.StopAll)

	firstResult, err := firstManager.ReconcileDesiredWorkers()
	require.NoError(t, err)
	require.Len(t, firstResult.Started, 1, "initial server run must launch the desired worker")
	firstID := firstResult.Started[0]
	require.Len(t, liveManagedWorkerRecords(t, root), 1)

	firstManager.StopAll()
	require.Eventually(t, func() bool {
		return len(liveManagedWorkerRecords(t, root)) == 0
	}, 5*time.Second, 50*time.Millisecond, "simulated service restart should stop the original worker")

	restartedManager := NewWorkerManager(root)
	restartedManager.workerBinaryPath = workerPath
	restartedManager.WatchdogKillGrace = 100 * time.Millisecond
	t.Cleanup(restartedManager.StopAll)

	prevSkip := skipStartupDesiredWorkerReconcileForTestBinary
	skipStartupDesiredWorkerReconcileForTestBinary = false
	t.Cleanup(func() { skipStartupDesiredWorkerReconcileForTestBinary = prevSkip })

	prevDelays := startupDesiredWorkerReconcileDelays
	startupDesiredWorkerReconcileDelays = []time.Duration{10 * time.Millisecond}
	t.Cleanup(func() { startupDesiredWorkerReconcileDelays = prevDelays })

	srv := &Server{
		WorkingDir: root,
		workers:    restartedManager,
	}
	srv.reconcileDesiredWorkersAfterStartup()

	require.Eventually(t, func() bool {
		live := liveManagedWorkerRecords(t, root)
		if len(live) != 1 {
			return false
		}
		return live[0].ID != firstID
	}, 5*time.Second, 50*time.Millisecond, "startup reconcile must restore desired_count=1 without manual reconcile")
}

func TestServerStartupReconcileDesiredWorkersProcessBackedNoDuplicates(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	setupBeadStore(t, rootA)
	setupBeadStore(t, rootB)

	binDir := t.TempDir()
	workerPath := filepath.Join(binDir, "ddx-worker")
	writeSleepingWorkerScript(t, workerPath)

	for _, root := range []string{rootA, rootB} {
		require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
			DesiredCount: 1,
			DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
			Restart:      WorkerRestartPolicy{Enabled: true},
		}))
	}

	var managers []*WorkerManager
	newManager := func(projectRoot string) *WorkerManager {
		m := NewWorkerManager(projectRoot)
		m.workerBinaryPath = workerPath
		m.WatchdogKillGrace = 100 * time.Millisecond
		managers = append(managers, m)
		return m
	}
	t.Cleanup(func() {
		for _, m := range managers {
			m.StopAll()
		}
	})

	startupManager := newManager(rootA)
	resultsByProject := map[string][]ReconcileResult{}
	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, startupRoot string, startupManager *WorkerManager) (ReconcileResult, error) {
		manager := startupManager
		if projectRoot != startupRoot {
			manager = newManager(projectRoot)
		}
		res, err := manager.ReconcileDesiredWorkers()
		resultsByProject[projectRoot] = append(resultsByProject[projectRoot], res)
		return res, err
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	state := &ServerState{}
	state.RegisterProject(rootB)
	srv := &Server{
		WorkingDir: rootA,
		workers:    startupManager,
		state:      state,
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs)
	assert.Len(t, resultsByProject[rootA][0].Started, 1)
	assert.Len(t, resultsByProject[rootB][0].Started, 1)
	assert.Len(t, liveManagedWorkerRecords(t, rootA), 1)
	assert.Len(t, liveManagedWorkerRecords(t, rootB), 1)

	errs = srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs)
	assert.Empty(t, resultsByProject[rootA][1].Started, "second startup pass must not duplicate the startup project worker")
	assert.Empty(t, resultsByProject[rootB][1].Started, "second startup pass must not duplicate registered project workers")
	assert.Len(t, liveManagedWorkerRecords(t, rootA), 1)
	assert.Len(t, liveManagedWorkerRecords(t, rootB), 1)
}

func liveManagedWorkerRecords(t *testing.T, projectRoot string) []WorkerRecord {
	t.Helper()
	m := NewWorkerManager(projectRoot)
	records, err := m.List()
	require.NoError(t, err)
	var live []WorkerRecord
	for _, rec := range records {
		if rec.Managed && rec.State == workerStateRunning && rec.PID > 0 && rec.PIDAlive != nil && *rec.PIDAlive {
			live = append(live, rec)
		}
	}
	return live
}
