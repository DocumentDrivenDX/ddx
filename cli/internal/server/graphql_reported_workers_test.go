package server

// ADR-022 step 5a: GraphQL reportedWorkers query exposes the worker_ingest
// derived view, classifying each worker by freshness state.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// reportedWorkersResponse decodes a GraphQL reply for the reportedWorkers query.
type reportedWorkersResponse struct {
	Data struct {
		ReportedWorkers []struct {
			ID                  string  `json:"id"`
			Project             string  `json:"project"`
			Harness             string  `json:"harness"`
			State               string  `json:"state"`
			LastEventAt         string  `json:"lastEventAt"`
			MirrorFailuresCount int     `json:"mirrorFailuresCount"`
			HadDroppedBackfill  bool    `json:"hadDroppedBackfill"`
			CurrentBead         *string `json:"currentBead"`
			CurrentAttempt      *string `json:"currentAttempt"`
		} `json:"reportedWorkers"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// newReportedWorkersHandler builds a GraphQL handler whose reportedWorkers
// resolver reads from reg, with now overridable so tests can advance the
// freshness clock without juggling LastEventAt timestamps.
func newReportedWorkersHandler(reg *workerIngestRegistry, now func() time.Time) http.Handler {
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:           &emptyStateProvider{},
			WorkingDir:      "/tmp/ignored",
			ReportedWorkers: &reportedWorkersAdapter{reg: reg, now: now},
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gqlSrv.AddTransport(transport.POST{})
	return gqlSrv
}

func postReportedWorkers(t *testing.T, h http.Handler) reportedWorkersResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"query": `{ reportedWorkers { id project harness state lastEventAt mirrorFailuresCount hadDroppedBackfill currentBead currentAttempt } }`,
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp reportedWorkersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", resp.Errors)
	}
	return resp
}

// TestGraphQL_Workers_FreshnessFields exercises the connected/stale/disconnected
// transitions by holding LastEventAt fixed and advancing the resolver's clock
// past the rev-5 freshness thresholds (probe interval = 30s).
func TestGraphQL_Workers_FreshnessFields(t *testing.T) {
	t.Parallel()

	reg := newWorkerIngestRegistry(t.TempDir())
	t.Cleanup(func() { _ = reg.close() })

	rec := reg.register(workerIdentity{
		ProjectRoot: "/proj/alpha",
		Harness:     "claude",
		StartedAt:   time.Now().UTC(),
	})
	// Drive recordEvent so currentBead/currentAttempt are populated and
	// LastEventAt is set to a fixed reference instant.
	if err := reg.recordEvent(rec.WorkerID, workerEvent{
		BeadID:    "bead-1",
		AttemptID: "attempt-1",
		Kind:      "phase",
	}); err != nil {
		t.Fatalf("recordEvent: %v", err)
	}

	// Pin LastEventAt so the resolver's clock cleanly drives the transition.
	reference := time.Now().UTC()
	reg.mu.Lock()
	reg.workers[rec.WorkerID].LastEventAt = reference
	reg.mu.Unlock()

	cases := []struct {
		name  string
		now   time.Time
		state string
	}{
		// Probe interval is 30s; connected ≤ 60s, stale ≤ 300s, disconnected else.
		{"connected", reference.Add(10 * time.Second), "connected"},
		{"stale", reference.Add(2 * time.Minute), "stale"},
		{"disconnected", reference.Add(20 * time.Minute), "disconnected"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			now := tc.now
			h := newReportedWorkersHandler(reg, func() time.Time { return now })
			resp := postReportedWorkers(t, h)
			if got := len(resp.Data.ReportedWorkers); got != 1 {
				t.Fatalf("want 1 reported worker, got %d", got)
			}
			w := resp.Data.ReportedWorkers[0]
			if w.State != tc.state {
				t.Fatalf("state: want %q, got %q", tc.state, w.State)
			}
			if w.ID != rec.WorkerID {
				t.Fatalf("id: want %q, got %q", rec.WorkerID, w.ID)
			}
			if w.Project != "/proj/alpha" {
				t.Fatalf("project: want /proj/alpha, got %q", w.Project)
			}
			if w.Harness != "claude" {
				t.Fatalf("harness: want claude, got %q", w.Harness)
			}
			if w.CurrentBead == nil || *w.CurrentBead != "bead-1" {
				t.Fatalf("currentBead: want bead-1, got %v", w.CurrentBead)
			}
			if w.CurrentAttempt == nil || *w.CurrentAttempt != "attempt-1" {
				t.Fatalf("currentAttempt: want attempt-1, got %v", w.CurrentAttempt)
			}
		})
	}
}

// TestGraphQL_Workers_MultipleWorkersPerProject verifies that two workers
// reporting under the same project_root both surface in the derived view —
// the duplicate-worker visibility ADR-022 promises operators.
func TestGraphQL_Workers_MultipleWorkersPerProject(t *testing.T) {
	t.Parallel()

	reg := newWorkerIngestRegistry(t.TempDir())
	t.Cleanup(func() { _ = reg.close() })

	const project = "/proj/beta"
	rec1 := reg.register(workerIdentity{ProjectRoot: project, Harness: "claude"})
	rec2 := reg.register(workerIdentity{ProjectRoot: project, Harness: "codex"})
	if rec1.WorkerID == rec2.WorkerID {
		t.Fatalf("expected distinct worker_ids, got %q == %q", rec1.WorkerID, rec2.WorkerID)
	}

	now := time.Now().UTC()
	h := newReportedWorkersHandler(reg, func() time.Time { return now })
	resp := postReportedWorkers(t, h)
	if got := len(resp.Data.ReportedWorkers); got != 2 {
		t.Fatalf("want 2 reported workers, got %d", got)
	}
	seen := map[string]string{}
	for _, w := range resp.Data.ReportedWorkers {
		if w.Project != project {
			t.Fatalf("project: want %q, got %q", project, w.Project)
		}
		seen[w.ID] = w.Harness
	}
	if seen[rec1.WorkerID] != "claude" {
		t.Fatalf("worker1 harness: want claude, got %q", seen[rec1.WorkerID])
	}
	if seen[rec2.WorkerID] != "codex" {
		t.Fatalf("worker2 harness: want codex, got %q", seen[rec2.WorkerID])
	}
}

// emptyStateProvider satisfies ddxgraphql.StateProvider for these tests, which
// only exercise the reportedWorkers query — no bead/project state is needed.
type emptyStateProvider struct{}

func (*emptyStateProvider) GetNodeSnapshot() ddxgraphql.NodeStateSnapshot {
	return ddxgraphql.NodeStateSnapshot{}
}
func (*emptyStateProvider) GetProjectSnapshots(bool) []*ddxgraphql.Project { return nil }
func (*emptyStateProvider) GetProjectSnapshotByID(string) (*ddxgraphql.Project, bool) {
	return nil, false
}
func (*emptyStateProvider) GetBeadSnapshots(string, string, string, string) []ddxgraphql.BeadSnapshot {
	return nil
}
func (*emptyStateProvider) GetBeadSnapshotsForProject(string, string, string, string) []ddxgraphql.BeadSnapshot {
	return nil
}
func (*emptyStateProvider) GetBeadSnapshot(string) (*ddxgraphql.BeadSnapshot, bool) {
	return nil, false
}
func (*emptyStateProvider) GetWorkersGraphQL(string) []*ddxgraphql.Worker      { return nil }
func (*emptyStateProvider) GetWorkerGraphQL(string) (*ddxgraphql.Worker, bool) { return nil, false }
func (*emptyStateProvider) GetWorkerLogGraphQL(string) *ddxgraphql.WorkerLog   { return nil }
func (*emptyStateProvider) GetWorkerProgressGraphQL(string) []*ddxgraphql.PhaseTransition {
	return nil
}
func (*emptyStateProvider) GetWorkerPromptGraphQL(string) string { return "" }
func (*emptyStateProvider) GetAgentSessionsGraphQL(*time.Time, *time.Time) []*ddxgraphql.AgentSession {
	return nil
}
func (*emptyStateProvider) GetAgentSessionGraphQL(string) (*ddxgraphql.AgentSession, bool) {
	return nil, false
}
func (*emptyStateProvider) GetSessionsCostSummaryGraphQL(string, *time.Time, *time.Time) *ddxgraphql.SessionsCostSummary {
	return nil
}
func (*emptyStateProvider) GetExecDefinitionsGraphQL(string) []*ddxgraphql.ExecutionDefinition {
	return nil
}
func (*emptyStateProvider) GetExecDefinitionGraphQL(string) (*ddxgraphql.ExecutionDefinition, bool) {
	return nil, false
}
func (*emptyStateProvider) GetExecRunsGraphQL(string, string) []*ddxgraphql.ExecutionRun {
	return nil
}
func (*emptyStateProvider) GetExecRunGraphQL(string) (*ddxgraphql.ExecutionRun, bool) {
	return nil, false
}
func (*emptyStateProvider) GetExecRunLogGraphQL(string) *ddxgraphql.ExecutionRunLog { return nil }
func (*emptyStateProvider) GetCoordinatorsGraphQL() []*ddxgraphql.CoordinatorMetricsEntry {
	return nil
}
func (*emptyStateProvider) GetCoordinatorMetricsByProjectGraphQL(string) *ddxgraphql.CoordinatorMetrics {
	return nil
}
