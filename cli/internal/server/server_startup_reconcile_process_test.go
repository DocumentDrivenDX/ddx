//go:build !windows

package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
