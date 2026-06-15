package graphql

import (
	"context"
	"fmt"
)

// SetWorkerDesiredState saves the desired worker count and restart policy for
// a project. It does NOT go through the generated MutationResolver interface
// because that would require regenerating generated.go. Tests call it directly
// on *mutationResolver in package graphql.
func (r *mutationResolver) SetWorkerDesiredState(ctx context.Context, projectRoot string, desiredCount int, restartEnabled bool) (*WorkerLifecycleResult, error) {
	if r.WorkerState == nil {
		return nil, fmt.Errorf("worker state manager is not configured")
	}
	return r.WorkerState.SetWorkerDesiredState(projectRoot, desiredCount, restartEnabled)
}

// RestartWorker stops the named worker and starts a fresh one with the same
// spec. Tests call this directly on *mutationResolver in package graphql.
func (r *mutationResolver) RestartWorker(ctx context.Context, id string) (*WorkerLifecycleResult, error) {
	if r.WorkerState == nil {
		return nil, fmt.Errorf("worker state manager is not configured")
	}
	return r.WorkerState.RestartWorker(id)
}

// ReconcileWorkers runs one reconcile pass for projectRoot, starting missing
// workers and stopping excess workers to match the desired count.
func (r *mutationResolver) ReconcileWorkers(ctx context.Context, projectRoot string) (*WorkerLifecycleResult, error) {
	if r.WorkerState == nil {
		return nil, fmt.Errorf("worker state manager is not configured")
	}
	return r.WorkerState.ReconcileWorkers(projectRoot)
}
