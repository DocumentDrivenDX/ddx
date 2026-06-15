package graphql

import (
	"context"
	"testing"
)

// mockWorkerStateManager implements WorkerStateManager for tests.
type mockWorkerStateManager struct {
	setDesiredCalled  bool
	setDesiredRoot    string
	setDesiredCount   int
	setDesiredRestart bool
	setDesiredResult  *WorkerLifecycleResult
	setDesiredErr     error

	restartCalled bool
	restartID     string
	restartResult *WorkerLifecycleResult
	restartErr    error

	reconcileCalled bool
	reconcileRoot   string
	reconcileResult *WorkerLifecycleResult
	reconcileErr    error
}

func (m *mockWorkerStateManager) SetWorkerDesiredState(projectRoot string, desiredCount int, restartEnabled bool) (*WorkerLifecycleResult, error) {
	m.setDesiredCalled = true
	m.setDesiredRoot = projectRoot
	m.setDesiredCount = desiredCount
	m.setDesiredRestart = restartEnabled
	return m.setDesiredResult, m.setDesiredErr
}

func (m *mockWorkerStateManager) RestartWorker(id string) (*WorkerLifecycleResult, error) {
	m.restartCalled = true
	m.restartID = id
	return m.restartResult, m.restartErr
}

func (m *mockWorkerStateManager) ReconcileWorkers(projectRoot string) (*WorkerLifecycleResult, error) {
	m.reconcileCalled = true
	m.reconcileRoot = projectRoot
	return m.reconcileResult, m.reconcileErr
}

func TestGraphQLSetWorkerDesiredState(t *testing.T) {
	dir := t.TempDir()
	mock := &mockWorkerStateManager{
		setDesiredResult: &WorkerLifecycleResult{
			ID:    dir,
			State: "desired_count=2 restart=true",
			Kind:  "desired-state",
		},
	}
	r := &mutationResolver{&Resolver{WorkingDir: dir, WorkerState: mock}}

	result, err := r.SetWorkerDesiredState(context.Background(), dir, 2, true)
	if err != nil {
		t.Fatalf("SetWorkerDesiredState: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !mock.setDesiredCalled {
		t.Error("expected SetWorkerDesiredState to be called on manager")
	}
	if mock.setDesiredRoot != dir {
		t.Errorf("projectRoot: got %q, want %q", mock.setDesiredRoot, dir)
	}
	if mock.setDesiredCount != 2 {
		t.Errorf("desiredCount: got %d, want 2", mock.setDesiredCount)
	}
	if !mock.setDesiredRestart {
		t.Error("restartEnabled: got false, want true")
	}
}

func TestGraphQLRestartWorker(t *testing.T) {
	dir := t.TempDir()
	mock := &mockWorkerStateManager{
		restartResult: &WorkerLifecycleResult{
			ID:    "worker-new-001",
			State: "running",
			Kind:  "work",
		},
	}
	r := &mutationResolver{&Resolver{WorkingDir: dir, WorkerState: mock}}

	result, err := r.RestartWorker(context.Background(), "worker-old-001")
	if err != nil {
		t.Fatalf("RestartWorker: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "worker-new-001" {
		t.Errorf("result ID: got %q, want worker-new-001", result.ID)
	}
	if !mock.restartCalled {
		t.Error("expected RestartWorker to be called on manager")
	}
	if mock.restartID != "worker-old-001" {
		t.Errorf("restart id: got %q, want worker-old-001", mock.restartID)
	}
}

func TestGraphQLWorkersDistinguishManagedAndExternal(t *testing.T) {
	managedTrue := true
	managedFalse := false

	managedWorker := Worker{
		ID:      "worker-managed-001",
		Kind:    "work",
		State:   "running",
		Managed: &managedTrue,
	}
	externalWorker := Worker{
		ID:      "worker-external-001",
		Kind:    "work",
		State:   "running",
		Managed: &managedFalse,
	}
	reportedWorker := ReportedWorker{
		ID:      "worker-reported-001",
		Project: "/some/project",
		State:   "connected",
		Managed: false,
	}

	if managedWorker.Managed == nil || !*managedWorker.Managed {
		t.Error("managed worker: Managed should be true")
	}
	if externalWorker.Managed == nil || *externalWorker.Managed {
		t.Error("external worker: Managed should be false")
	}
	if reportedWorker.Managed {
		t.Error("reported worker: Managed should be false")
	}
}
