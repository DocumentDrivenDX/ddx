package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerStartupReconcileDesiredWorkersAcrossRegisteredProjects(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	rootWithoutDesired := t.TempDir()
	require.NoError(t, SaveWorkerDesiredState(rootA, &WorkerDesiredState{DesiredCount: 1}))
	require.NoError(t, SaveWorkerDesiredState(rootB, &WorkerDesiredState{DesiredCount: 1}))

	var reconciled []string
	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, startupRoot string, startupManager *WorkerManager) (ReconcileResult, error) {
		reconciled = append(reconciled, projectRoot)
		return ReconcileResult{}, nil
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	state := &ServerState{}
	state.RegisterProject(rootB)
	state.RegisterProject(rootWithoutDesired)
	srv := &Server{
		WorkingDir: rootA,
		workers:    NewWorkerManager(rootA),
		state:      state,
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs)
	assert.ElementsMatch(t, []string{rootA, rootB}, reconciled)
}

func TestServerStartupReconcileReportsInvalidDesiredState(t *testing.T) {
	root := t.TempDir()
	desiredPath := workerDesiredStatePath(root)
	require.NoError(t, os.MkdirAll(filepath.Dir(desiredPath), 0o755))
	require.NoError(t, os.WriteFile(desiredPath, []byte(`{"version":999,"desired_count":1}`), 0o644))

	called := false
	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, startupRoot string, startupManager *WorkerManager) (ReconcileResult, error) {
		called = true
		return ReconcileResult{}, nil
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	srv := &Server{
		WorkingDir: root,
		workers:    NewWorkerManager(root),
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "load desired worker state")
	assert.Contains(t, errs[0].Error(), "unsupported worker desired-state version")
	assert.False(t, called, "invalid desired state must not start a worker")
}

func TestServerStartupReconcileDeduplicatesStartupProject(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{DesiredCount: 1}))

	var reconciled []string
	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, startupRoot string, startupManager *WorkerManager) (ReconcileResult, error) {
		reconciled = append(reconciled, projectRoot)
		return ReconcileResult{}, nil
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	state := &ServerState{}
	state.RegisterProject(root)
	srv := &Server{
		WorkingDir: root,
		workers:    NewWorkerManager(root),
		state:      state,
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs)
	require.Equal(t, []string{root}, reconciled)
}
