package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// reportedWorkersAdapter adapts the in-memory worker_ingest registry to the
// ddxgraphql.ReportedWorkersProvider interface used by the reportedWorkers
// query. It also synthesizes in-flight attempts from run-state.json as
// active workers. now is overridable so tests can advance the freshness
// clock without mutating registry timestamps.
type reportedWorkersAdapter struct {
	reg        *workerIngestRegistry
	workingDir string
	mu         sync.RWMutex
	now        func() time.Time
}

func newReportedWorkersAdapter(reg *workerIngestRegistry) *reportedWorkersAdapter {
	return &reportedWorkersAdapter{reg: reg, now: func() time.Time { return time.Now().UTC() }}
}

func newReportedWorkersAdapterWithWorkingDir(reg *workerIngestRegistry, workingDir string) *reportedWorkersAdapter {
	return &reportedWorkersAdapter{
		reg:        reg,
		workingDir: workingDir,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (a *reportedWorkersAdapter) currentNow() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.now()
}

// GetReportedWorkers implements ddxgraphql.ReportedWorkersProvider.
// Returns workers from the ingest registry plus synthesized active workers
// from live run-state.json files. The reportedWorkers query shows both
// registered worker reports (from the worker-server HTTP interface, ADR-022)
// and in-flight attempts (from run-state.json for inline work).
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

	// Synthesize active workers from live run-state.json for inline work.
	if a.workingDir != "" {
		inFlightWorkers := a.getInFlightWorkers(now)
		out = append(out, inFlightWorkers...)
	}

	return out
}

// getInFlightWorkers returns synthesized ReportedWorker entries for each
// live attempt in run-state.json that has not yet expired.
func (a *reportedWorkersAdapter) getInFlightWorkers(now time.Time) []*ddxgraphql.ReportedWorker {
	runStateDir := ddxroot.JoinProject(a.workingDir, agent.RunStateDirName)
	entries, err := os.ReadDir(runStateDir)
	if err != nil {
		return nil
	}

	var out []*ddxgraphql.ReportedWorker
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(runStateDir, entry.Name()))
		if err != nil {
			continue
		}
		var state agent.RunState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		// Skip expired runs.
		if !state.ExpiresAt.IsZero() && now.After(state.ExpiresAt) {
			continue
		}
		// Synthesize a ReportedWorker from the run-state.
		w := &ddxgraphql.ReportedWorker{
			ID:          "inline-" + state.AttemptID,
			Project:     a.workingDir,
			Harness:     state.Harness,
			State:       "connected",
			LastEventAt: state.RefreshedAt.UTC().Format(time.RFC3339Nano),
		}
		if state.BeadID != "" {
			b := state.BeadID
			w.CurrentBead = &b
		}
		if state.AttemptID != "" {
			att := state.AttemptID
			w.CurrentAttempt = &att
		}
		out = append(out, w)
	}
	return out
}
