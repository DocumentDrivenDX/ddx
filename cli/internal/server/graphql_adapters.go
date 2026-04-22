package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
// WorkerManager. Non execute-loop actions are handled in the GraphQL resolver
// because their real executors are intentionally deferred to a follow-up bead.
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
