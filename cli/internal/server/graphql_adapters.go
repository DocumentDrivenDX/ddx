package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// execLogAdapter implements ddxgraphql.ExecLogProvider using the exec store.
// LAYER 2 of the GraphQL multi-project fix (ddx-055e8d32): workingDir is no
// longer carried on the adapter — the resolver supplies the per-request
// workingDir via context, threaded through ExecLogProvider.GetExecLog.
type execLogAdapter struct{}

func (a *execLogAdapter) GetExecLog(workingDir, runID string) (string, string, error) {
	store := ddxexec.NewStore(workingDir)
	return store.Log(runID)
}

// coordMetricsAdapter implements ddxgraphql.CoordinatorMetricsProvider using
// the coordinator registry.
type coordMetricsAdapter struct {
	reg *coordinatorRegistry
}

func (a *coordMetricsAdapter) GetCoordinatorMetrics(projectRoot string) *ddxgraphql.CoordinatorMetricsSnap {
	m := a.reg.MetricsFor(projectRoot)
	if m == nil {
		return nil
	}
	return &ddxgraphql.CoordinatorMetricsSnap{
		Landed:          m.Landed,
		Preserved:       m.Preserved,
		Failed:          m.Failed,
		PushFailed:      m.PushFailed,
		TotalDurationMS: m.TotalDurationMS,
		TotalCommits:    m.TotalCommits,
	}
}

// workerDispatchAdapter implements GraphQL action dispatch using the live
// WorkerManager.
type workerDispatchAdapter struct {
	manager *WorkerManager
}

func (a *workerDispatchAdapter) DispatchWorker(ctx context.Context, kind string, projectRoot string, rawArgs *string) (*ddxgraphql.WorkerDispatchResult, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("worker dispatcher is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if kind != "work" {
		return nil, fmt.Errorf("unsupported worker kind %q", kind)
	}

	var spec executeloop.ExecuteLoopSpec
	count := 1
	if rawArgs != nil && *rawArgs != "" {
		if err := rejectLegacyExecuteLoopWorkerArgs([]byte(*rawArgs)); err != nil {
			return nil, err
		}
		parsedCount, err := workerDispatchCount([]byte(*rawArgs))
		if err != nil {
			return nil, err
		}
		count = parsedCount
		if err := json.Unmarshal([]byte(*rawArgs), &spec); err != nil {
			return nil, fmt.Errorf("invalid worker args JSON: %w", err)
		}
	}

	// Apply .ddx/config.yaml workers.default_spec + enforce workers.max_count
	// (ddx-b6cf025c). The max_count cap counts currently-running drain workers
	// for this project so the "+ Add worker" button can refuse cleanly.
	if wc := loadWorkersConfig(projectRoot); wc != nil {
		if defaultSpec := wc.DefaultSpec; defaultSpec != nil {
			if spec.Profile == "" {
				spec.Profile = defaultSpec.Profile
			}
			if spec.Effort == "" {
				spec.Effort = defaultSpec.Effort
			}
		}
		if wc.MaxCount != nil && *wc.MaxCount >= 0 {
			running := a.countRunningDrainWorkers(projectRoot)
			if running+count > *wc.MaxCount {
				return nil, fmt.Errorf("workers.max_count cap reached: %d running + %d requested exceeds limit %d", running, count, *wc.MaxCount)
			}
		}
	}

	// Default-path contract (ddx-755f5881 AC #1): an empty dispatch input
	// (no rawArgs, no workers.default_spec) must produce a spec with
	// Profile: "default" and nothing else. This eliminates the historical
	// 19-burn drain-queue failure mode where an empty spec fanned out into
	// per-powerClass ladder iteration with no upstream synthesis target.
	if spec.Profile == "" {
		spec.Profile = "default"
	}

	workerSpec, err := prepareExecuteLoopWorkerSpec(projectRoot, spec, executeloop.ModeWatch)
	if err != nil {
		return nil, err
	}
	workers := make([]*ddxgraphql.WorkerLifecycleResult, 0, count)
	startedIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		record, err := a.manager.StartExecuteLoop(workerSpec)
		if err != nil {
			for _, id := range startedIDs {
				_ = a.manager.Stop(id)
			}
			return nil, err
		}
		startedIDs = append(startedIDs, record.ID)
		workers = append(workers, &ddxgraphql.WorkerLifecycleResult{
			ID:    record.ID,
			State: record.State,
			Kind:  record.Kind,
		})
	}
	first := workers[0]
	return &ddxgraphql.WorkerDispatchResult{
		ID:      first.ID,
		State:   first.State,
		Kind:    first.Kind,
		Workers: workers,
	}, nil
}

func workerDispatchCount(raw []byte) (int, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return 1, fmt.Errorf("invalid worker args JSON: %w", err)
	}
	value, ok := fields["count"]
	if !ok || len(value) == 0 || string(value) == "null" {
		return 1, nil
	}
	var n int
	if err := json.Unmarshal(value, &n); err == nil {
		if n < 1 {
			return 0, fmt.Errorf("count must be >= 1")
		}
		return n, nil
	}
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return 0, fmt.Errorf("count must be an integer")
	}
	parsed, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("count must be an integer")
	}
	if parsed < 1 {
		return 0, fmt.Errorf("count must be >= 1")
	}
	return parsed, nil
}

func rejectLegacyExecuteLoopWorkerArgs(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("invalid worker args JSON: %w", err)
	}
	if _, ok := fields["poll_interval"]; ok {
		return fmt.Errorf("poll_interval is not supported for work worker dispatch; use mode=\"watch\" and idle_interval")
	}
	if _, ok := fields["once"]; ok {
		return fmt.Errorf("once is not supported for work worker dispatch; use mode=\"once\"")
	}
	return nil
}

// loadWorkersConfig reads .ddx/config.yaml at projectRoot and returns the
// workers block, or nil when unset / on error. Errors are swallowed because
// a missing or malformed config must not block the dispatch path.
func loadWorkersConfig(projectRoot string) *config.WorkersConfig {
	if projectRoot == "" {
		return nil
	}
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.Workers
}

// countRunningDrainWorkers counts work workers currently in state
// "running" for projectRoot. Returns 0 on any error.
func (a *workerDispatchAdapter) countRunningDrainWorkers(projectRoot string) int {
	if a == nil || a.manager == nil {
		return 0
	}
	recs, err := a.manager.List()
	if err != nil {
		return 0
	}
	count := 0
	for _, rec := range recs {
		if rec.Kind == "work" && rec.State == "running" && sameCanonicalPath(rec.ProjectRoot, projectRoot) {
			count++
		}
	}
	return count
}

func (a *workerDispatchAdapter) DispatchPlugin(ctx context.Context, projectRoot string, name string, action string, scope string) (*ddxgraphql.PluginDispatchResult, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("worker dispatcher is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record, err := a.manager.StartPluginAction(PluginActionWorkerSpec{
		ProjectRoot: projectRoot,
		Name:        name,
		Action:      action,
		Scope:       scope,
	}, func(runCtx context.Context) (string, error) {
		if err := runCtx.Err(); err != nil {
			return "", err
		}
		return ddxgraphql.DispatchPluginAction(projectRoot, name, action)
	})
	if err != nil {
		return nil, err
	}
	return &ddxgraphql.PluginDispatchResult{
		ID:     record.ID,
		State:  record.State,
		Action: action,
	}, nil
}

func (a *workerDispatchAdapter) StopWorker(ctx context.Context, id string) (*ddxgraphql.WorkerLifecycleResult, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("worker dispatcher is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := a.manager.RequestStop(id); err != nil {
		return nil, err
	}
	return &ddxgraphql.WorkerLifecycleResult{
		ID:    id,
		State: "stopping",
		Kind:  "work",
	}, nil
}

// workerStateManagerAdapter implements ddxgraphql.WorkerStateManager using the
// live WorkerManager and WorkerSupervisor machinery.
type workerStateManagerAdapter struct {
	manager *WorkerManager
}

func (a *workerStateManagerAdapter) SetWorkerDesiredState(projectRoot string, desiredCount int, restartEnabled bool) (*ddxgraphql.WorkerLifecycleResult, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("worker state manager is not configured")
	}
	state := &WorkerDesiredState{
		Version:      WorkerDesiredStateVersion,
		ProjectRoot:  projectRoot,
		DesiredCount: desiredCount,
		Restart:      WorkerRestartPolicy{Enabled: restartEnabled},
	}
	if err := SaveWorkerDesiredState(projectRoot, state); err != nil {
		return nil, fmt.Errorf("set desired state: %w", err)
	}
	return &ddxgraphql.WorkerLifecycleResult{
		ID:    projectRoot,
		State: fmt.Sprintf("desired_count=%d restart=%v", desiredCount, restartEnabled),
		Kind:  "desired-state",
	}, nil
}

func (a *workerStateManagerAdapter) RestartWorker(id string) (*ddxgraphql.WorkerLifecycleResult, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("worker state manager is not configured")
	}
	rec, err := a.manager.RestartWorker(id, 0)
	if err != nil {
		return nil, err
	}
	return &ddxgraphql.WorkerLifecycleResult{
		ID:    rec.ID,
		State: rec.State,
		Kind:  rec.Kind,
	}, nil
}

func (a *workerStateManagerAdapter) ReconcileWorkers(projectRoot string) (*ddxgraphql.WorkerLifecycleResult, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("worker state manager is not configured")
	}
	result, err := a.manager.ReconcileDesiredWorkers()
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	summary := fmt.Sprintf("started=%d restarted=%d stopped=%d stale=%d",
		len(result.Started), len(result.Restarted), len(result.Stopped), len(result.StaleMarked))
	return &ddxgraphql.WorkerLifecycleResult{
		ID:    projectRoot,
		State: summary,
		Kind:  "reconcile",
	}, nil
}
