package server

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	assert.Contains(t, errs[0].Error(), root, "error must name the project root so a skipped project is never silent")
	assert.Contains(t, errs[0].Error(), "load desired worker state")
	assert.Contains(t, errs[0].Error(), "unsupported worker desired-state version")
	assert.False(t, called, "invalid desired state must not start a worker")
}

func TestServerStartupReconcileReportsStaleDesiredProjectAudit(t *testing.T) {
	activeRoot := t.TempDir()
	staleRoot := t.TempDir()
	require.NoError(t, SaveWorkerDesiredState(activeRoot, &WorkerDesiredState{DesiredCount: 1}))

	var log bytes.Buffer
	prevLog := startupDesiredWorkerReconcileLog
	startupDesiredWorkerReconcileLog = &log
	t.Cleanup(func() { startupDesiredWorkerReconcileLog = prevLog })

	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, _ string, _ *WorkerManager) (ReconcileResult, error) {
		return ReconcileResult{}, nil
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	state := &ServerState{}
	state.RegisterProject(staleRoot)
	srv := &Server{
		WorkingDir: activeRoot,
		workers:    NewWorkerManager(activeRoot),
		state:      state,
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs)

	out := log.String()
	assert.Contains(t, out, "ddx-server: startup worker reconcile project="+staleRoot)
	assert.Contains(t, out, "status=stale")
	assert.Contains(t, out, "reason=desired worker state missing")
	assert.NotContains(t, out, "project="+activeRoot)
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

// TestServerStartupReconcileLaunchesDesiredWorkersAfterListen proves that
// reconcileDesiredWorkersOnce drives the real WorkerSupervisor reconcile path
// for every registered project and that workers are actually started — not just
// that the routing fires. The supervisors use fakeWorkerController so no real
// processes are spawned; this test validates the integration between the server
// discovery layer (desiredWorkerProjectRoots → startupDesiredWorkerReconcileProject)
// and the supervisor reconcile logic that is bypassed by the routing-only stubs
// in TestServerStartupReconcileDesiredWorkersAcrossRegisteredProjects.
func TestServerStartupReconcileLaunchesDesiredWorkersAfterListen(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	clock := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return clock }

	require.NoError(t, SaveWorkerDesiredState(rootA, &WorkerDesiredState{
		DesiredCount: 1,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
	}))
	require.NoError(t, SaveWorkerDesiredState(rootB, &WorkerDesiredState{
		DesiredCount: 1,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
	}))

	fakes := map[string]*fakeWorkerController{
		rootA: newFakeWorkerController(nowFn),
		rootB: newFakeWorkerController(nowFn),
	}
	results := map[string]ReconcileResult{}

	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, _ string, _ *WorkerManager) (ReconcileResult, error) {
		fake, ok := fakes[projectRoot]
		if !ok {
			return ReconcileResult{}, fmt.Errorf("unexpected project root in reconcile: %s", projectRoot)
		}
		sup := NewWorkerSupervisor(projectRoot, fake)
		sup.clock = nowFn
		res, err := sup.Reconcile()
		results[projectRoot] = res
		return res, err
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	state := &ServerState{}
	state.RegisterProject(rootB)
	srv := &Server{
		WorkingDir: rootA,
		workers:    NewWorkerManager(rootA),
		state:      state,
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs, "startup reconcile must produce no errors")

	// Both projects must have workers started through the real supervisor path.
	resA := results[rootA]
	resB := results[rootB]
	require.Len(t, resA.Started, 1, "rootA (server WorkingDir) must start one managed worker")
	require.Len(t, resB.Started, 1, "rootB (registered project) must start one managed worker")

	// Workers must be live in the fake controllers, confirming the full path fired.
	assert.Equal(t, workerStateRunning, fakes[rootA].records[resA.Started[0]].State)
	assert.Equal(t, workerStateRunning, fakes[rootB].records[resB.Started[0]].State)
}

// TestServerStartupReconcileReplacesTerminatedWorker covers the observed restart
// symptom: desired_count=1 is saved for a project, the previous server run left a
// stale running disk record for that project, and after the server restarts exactly
// one fresh managed worker must appear without a manual ddx worker reconcile call.
func TestServerStartupReconcileReplacesTerminatedWorker(t *testing.T) {
	root := t.TempDir()

	clock := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return clock }

	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{
		DesiredCount: 1,
		DefaultSpec:  WorkerDefaultSpec{Mode: "watch", IdleInterval: "30s"},
		Restart:      WorkerRestartPolicy{Enabled: true},
	}))

	fake := newFakeWorkerController(nowFn)
	// Simulate a worker record left "running" on disk by the previous server run.
	// It has no live in-memory handle because the server just restarted.
	fake.seedRunning("worker-from-previous-run")

	var capturedResult ReconcileResult
	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, _ string, _ *WorkerManager) (ReconcileResult, error) {
		sup := NewWorkerSupervisor(projectRoot, fake)
		sup.clock = nowFn
		res, err := sup.Reconcile()
		capturedResult = res
		return res, err
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
	require.Empty(t, errs, "startup reconcile must produce no errors")

	// The stale disk record from the previous run must be marked stopped, not adopted.
	assert.Contains(t, capturedResult.StaleMarked, "worker-from-previous-run",
		"stale disk record from previous run must be reconciled to stopped")
	assert.Equal(t, workerStateStopped, fake.records["worker-from-previous-run"].State)

	// Exactly one fresh managed worker must be started to satisfy desired_count=1.
	require.Len(t, capturedResult.Started, 1, "exactly one fresh managed worker must start after server restart")
	newID := capturedResult.Started[0]
	assert.NotEqual(t, "worker-from-previous-run", newID,
		"fresh worker must have a new ID, not reuse the stale record")
	assert.Equal(t, workerStateRunning, fake.records[newID].State)
}

func TestServerStartupReconcileLogsNonNoopResults(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{DesiredCount: 1}))

	var log bytes.Buffer
	prevLog := startupDesiredWorkerReconcileLog
	startupDesiredWorkerReconcileLog = &log
	t.Cleanup(func() { startupDesiredWorkerReconcileLog = prevLog })

	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, _ string, _ *WorkerManager) (ReconcileResult, error) {
		return ReconcileResult{
			Started:        []string{"worker-new"},
			StaleMarked:    []string{"worker-stale"},
			RestartSkipped: []string{"dirty project root"},
		}, nil
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	srv := &Server{
		WorkingDir: root,
		workers:    NewWorkerManager(root),
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs)
	out := log.String()
	assert.Contains(t, out, "ddx-server: startup worker reconcile project="+root)
	assert.Contains(t, out, "started=1")
	assert.Contains(t, out, "stale_marked=1")
	assert.Contains(t, out, "restart_skipped=1")
}

func TestServerStartupReconcileDoesNotLogNoopResults(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveWorkerDesiredState(root, &WorkerDesiredState{DesiredCount: 0}))

	var log bytes.Buffer
	prevLog := startupDesiredWorkerReconcileLog
	startupDesiredWorkerReconcileLog = &log
	t.Cleanup(func() { startupDesiredWorkerReconcileLog = prevLog })

	prev := startupDesiredWorkerReconcileProject
	startupDesiredWorkerReconcileProject = func(projectRoot, _ string, _ *WorkerManager) (ReconcileResult, error) {
		return ReconcileResult{}, nil
	}
	t.Cleanup(func() { startupDesiredWorkerReconcileProject = prev })

	srv := &Server{
		WorkingDir: root,
		workers:    NewWorkerManager(root),
	}

	errs := srv.reconcileDesiredWorkersOnce()
	require.Empty(t, errs)
	assert.Empty(t, log.String(), "no-op startup reconcile should not spam the service log")
}
