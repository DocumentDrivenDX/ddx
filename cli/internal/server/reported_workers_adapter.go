package server

import (
	"sync"
	"time"

	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// reportedWorkersAdapter adapts the in-memory worker_ingest registry to the
// ddxgraphql.ReportedWorkersProvider interface used by the reportedWorkers
// query. now is overridable so tests can advance the freshness clock without
// mutating registry timestamps.
type reportedWorkersAdapter struct {
	reg *workerIngestRegistry
	mu  sync.RWMutex
	now func() time.Time
}

func newReportedWorkersAdapter(reg *workerIngestRegistry) *reportedWorkersAdapter {
	return &reportedWorkersAdapter{reg: reg, now: func() time.Time { return time.Now().UTC() }}
}

// setNow swaps the adapter's freshness clock. Safe to call concurrently with
// GraphQL reads. ADR-022 step 5c integration tests use this to advance
// synthetic time past the 2×/10× probe-interval thresholds without sleeping.
func (a *reportedWorkersAdapter) setNow(now func() time.Time) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	a.mu.Lock()
	a.now = now
	a.mu.Unlock()
}

func (a *reportedWorkersAdapter) currentNow() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.now()
}

// GetReportedWorkers implements ddxgraphql.ReportedWorkersProvider.
func (a *reportedWorkersAdapter) GetReportedWorkers() []*ddxgraphql.ReportedWorker {
	if a.reg == nil {
		return nil
	}
	now := a.currentNow()
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
