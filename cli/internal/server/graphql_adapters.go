package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// execLogAdapter implements ddxgraphql.ExecLogProvider using the exec store.
type execLogAdapter struct {
	workingDir string
}

func (a *execLogAdapter) GetExecLog(runID string) (string, string, error) {
	store := ddxexec.NewStore(a.workingDir)
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
	if kind != "execute-loop" {
		return nil, fmt.Errorf("unsupported worker kind %q", kind)
	}

	var req struct {
		Harness       string `json:"harness"`
		Model         string `json:"model"`
		Profile       string `json:"profile"`
		Provider      string `json:"provider"`
		ModelRef      string `json:"model_ref"`
		Effort        string `json:"effort"`
		LabelFilter   string `json:"label_filter"`
		Once          bool   `json:"once"`
		PollInterval  string `json:"poll_interval"`
		NoReview      bool   `json:"no_review"`
		ReviewHarness string `json:"review_harness"`
		ReviewModel   string `json:"review_model"`
		MinTier       string `json:"min_tier"`
		MaxTier       string `json:"max_tier"`
	}
	if rawArgs != nil && *rawArgs != "" {
		if err := json.Unmarshal([]byte(*rawArgs), &req); err != nil {
			return nil, fmt.Errorf("invalid worker args JSON: %w", err)
		}
	}

	// Apply .ddx/config.yaml workers.default_spec + enforce workers.max_count
	// (ddx-b6cf025c). The max_count cap counts currently-running drain workers
	// for this project so the "+ Add worker" button can refuse cleanly.
	if wc := loadWorkersConfig(projectRoot); wc != nil {
		if spec := wc.DefaultSpec; spec != nil {
			if req.Harness == "" {
				req.Harness = spec.Harness
			}
			if req.Profile == "" {
				req.Profile = spec.Profile
			}
			if req.Effort == "" {
				req.Effort = spec.Effort
			}
			if req.MinTier == "" {
				req.MinTier = spec.MinTier
			}
			if req.MaxTier == "" {
				req.MaxTier = spec.MaxTier
			}
		}
		if wc.MaxCount != nil && *wc.MaxCount >= 0 {
			running := a.countRunningDrainWorkers(projectRoot)
			if running >= *wc.MaxCount {
				return nil, fmt.Errorf("workers.max_count cap reached: %d running (limit %d)", running, *wc.MaxCount)
			}
		}
	}

	// Default-path contract (ddx-755f5881 AC #1): an empty dispatch input
	// (no rawArgs, no workers.default_spec) must produce a spec with
	// Profile: "default" and nothing else. This eliminates the historical
	// 19-burn drain-queue failure mode where an empty spec fanned out into
	// per-tier ladder iteration with no upstream synthesis target.
	if req.Profile == "" {
		req.Profile = "default"
	}

	var pollInterval time.Duration
	if req.PollInterval != "" {
		d, err := time.ParseDuration(req.PollInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid poll_interval: %w", err)
		}
		pollInterval = d
	}

	record, err := a.manager.StartExecuteLoop(ExecuteLoopWorkerSpec{
		ProjectRoot:   projectRoot,
		Harness:       req.Harness,
		Model:         req.Model,
		Profile:       req.Profile,
		Provider:      req.Provider,
		ModelRef:      req.ModelRef,
		Effort:        req.Effort,
		LabelFilter:   req.LabelFilter,
		Once:          req.Once,
		PollInterval:  pollInterval,
		NoReview:      req.NoReview,
		ReviewHarness: req.ReviewHarness,
		ReviewModel:   req.ReviewModel,
		MinTier:       req.MinTier,
		MaxTier:       req.MaxTier,
	})
	if err != nil {
		return nil, err
	}
	return &ddxgraphql.WorkerDispatchResult{
		ID:    record.ID,
		State: record.State,
		Kind:  record.Kind,
	}, nil
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

// countRunningDrainWorkers counts execute-loop workers currently in state
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
		if rec.Kind == "execute-loop" && rec.State == "running" && rec.ProjectRoot == projectRoot {
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
	if err := a.manager.Stop(id); err != nil {
		return nil, err
	}
	rec, err := a.manager.Show(id)
	if err != nil {
		return &ddxgraphql.WorkerLifecycleResult{ID: id, State: "stopping", Kind: "execute-loop"}, nil
	}
	return &ddxgraphql.WorkerLifecycleResult{
		ID:    rec.ID,
		State: rec.State,
		Kind:  rec.Kind,
	}, nil
}
