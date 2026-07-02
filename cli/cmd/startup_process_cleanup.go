package cmd

import (
	"context"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

type startupProcessCleanupFunc func(ctx context.Context, projectRoot, tempRoot string, summary *agent.ExecutionCleanupSummary, runStates []agent.RunState, registered map[string]struct{}, now time.Time) error

type startupAttemptProcessLivenessProbe struct {
	liveWorkers []workerstatus.LiveWorker
}

func defaultStartupProcessCleanup(ctx context.Context, projectRoot, tempRoot string, summary *agent.ExecutionCleanupSummary, runStates []agent.RunState, registered map[string]struct{}, now time.Time) error {
	if summary == nil || projectRoot == "" {
		return nil
	}
	liveWorkers, err := workerstatus.New().Scan(ctx)
	if err != nil {
		summary.Warnings = append(summary.Warnings, agent.ExecutionCleanupWarning{
			Path:    projectRoot,
			Class:   "worker_status_scan",
			Message: err.Error(),
		})
		liveWorkers = nil
	} else {
		liveWorkers = workerstatus.FilterByProject(liveWorkers, projectRoot)
	}

	mgr := agent.NewExecutionCleanupManager(projectRoot, &agent.RealGitOps{})
	mgr.TempRoot = tempRoot
	return mgr.CleanupAttemptProcesses(ctx, summary, runStates, registered, startupAttemptProcessLivenessProbe{
		liveWorkers: liveWorkers,
	}, now)
}

func (p startupAttemptProcessLivenessProbe) IsLive(meta agent.ExecutionCleanupMetadata, runState *agent.RunState, now time.Time) (bool, string) {
	live, reason := agent.DefaultExecutionCleanupLiveness(meta, runState, now)
	if live {
		return true, reason
	}
	if p.matchesLiveWorker(meta) {
		return true, "live worker sidecar"
	}
	return false, reason
}

func (p startupAttemptProcessLivenessProbe) matchesLiveWorker(meta agent.ExecutionCleanupMetadata) bool {
	for _, worker := range p.liveWorkers {
		if worker.AttemptID != "" && worker.AttemptID == meta.AttemptID {
			return true
		}
		if worker.BeadID != "" && worker.BeadID == meta.BeadID {
			return true
		}
		if worker.ExecutionWorktree != "" && meta.WorktreePath != "" && workerstatus.SamePath(worker.ExecutionWorktree, meta.WorktreePath) {
			return true
		}
	}
	return false
}
