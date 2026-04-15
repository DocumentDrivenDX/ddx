package server

import (
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
