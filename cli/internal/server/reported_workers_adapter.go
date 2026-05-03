package server

import (
	"time"

	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// reportedWorkersAdapter adapts the in-memory worker_ingest registry to the
// ddxgraphql.ReportedWorkersProvider interface used by the reportedWorkers
// query. now is overridable so tests can advance the freshness clock without
// mutating registry timestamps.
type reportedWorkersAdapter struct {
	reg *workerIngestRegistry
	now func() time.Time
}

func newReportedWorkersAdapter(reg *workerIngestRegistry) *reportedWorkersAdapter {
	return &reportedWorkersAdapter{reg: reg, now: func() time.Time { return time.Now().UTC() }}
}

// GetReportedWorkers implements ddxgraphql.ReportedWorkersProvider.
func (a *reportedWorkersAdapter) GetReportedWorkers() []*ddxgraphql.ReportedWorker {
	if a.reg == nil {
		return nil
	}
	now := a.now()
	snap := a.reg.snapshot()
	out := make([]*ddxgraphql.ReportedWorker, 0, len(snap))
	for _, rec := range snap {
		w := &ddxgraphql.ReportedWorker{
			ID:                  rec.WorkerID,
			Project:             rec.Identity.ProjectRoot,
			Harness:             rec.Identity.Harness,
			State:               freshnessState(rec, now),
			LastEventAt:         rec.LastEventAt.UTC().Format(time.RFC3339Nano),
			MirrorFailuresCount: rec.MirrorFailuresCount,
			HadDroppedBackfill:  rec.HadDroppedBackfill,
		}
		if rec.CurrentBead != "" {
			cb := rec.CurrentBead
			w.CurrentBead = &cb
		}
		if rec.CurrentAttempt != "" {
			ca := rec.CurrentAttempt
			w.CurrentAttempt = &ca
		}
		out = append(out, w)
	}
	return out
}
