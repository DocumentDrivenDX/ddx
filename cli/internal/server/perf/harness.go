package perf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"
)

// Target is one entry in the benchmark matrix. Adding a new GraphQL query
// shape is a single-entry change: append a Target to Targets() with an
// in-process call and a GraphQL query string. Both are exercised on every
// harness run (ddx-9ce6842a AC §1).
type Target struct {
	Name       string
	Query      string
	InProcess  func(f *BeadFixture) error
	SkipInHTTP bool
}

// Result holds timings and metadata for one Target over one fixture.
type Result struct {
	Name       string       `json:"name"`
	Iterations int          `json:"iterations"`
	InProcess  Percentiles  `json:"in_process_ms"`
	HTTP       *Percentiles `json:"http_ms,omitempty"`
}

// Percentiles holds p50/p95/p99 in floating-point milliseconds.
type Percentiles struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

// MatrixReport is the full output of one RunMatrix call — a list of per-target
// results plus fixture/environment metadata. It is what baseline reports
// compare against and what CI will diff in a follow-up bead.
type MatrixReport struct {
	GeneratedAt string    `json:"generated_at"`
	Environment string    `json:"environment"`
	Fixture     FixtureID `json:"fixture"`
	Targets     []Result  `json:"targets"`
}

// FixtureID is the fixture-shape metadata stamped into every report so a
// regression check can sanity-check that it's comparing apples to apples.
type FixtureID struct {
	Projects                int `json:"projects"`
	BeadsPerProject         int `json:"beads_per_project"`
	TotalBeads              int `json:"total_beads"`
	AgentSessionsPerProject int `json:"agent_sessions_per_project"`
}

// DefaultIterations is the sample size used when a run request does not
// override it. Chosen so p95/p99 are defensible on a 5,000-bead fixture
// without extending benchmark wall time past a few seconds.
const DefaultIterations = 40

// Targets returns the harness matrix covering the five query shapes named in
// ddx-9ce6842a AC §1: bead(id:), beadsByProject, beads (cross-project),
// docGraph, agentSessions. Adding a new target is a single append here.
func Targets() []Target {
	return []Target{
		{
			Name:  "bead",
			Query: `query Q($id: ID!) { bead(id: $id) { id title status updatedAt } }`,
			InProcess: func(f *BeadFixture) error {
				snap, ok := f.Server.State().GetBeadSnapshot(f.TargetBeadID)
				if !ok {
					return fmt.Errorf("bead not found: %s", f.TargetBeadID)
				}
				if snap.ID != f.TargetBeadID {
					return fmt.Errorf("unexpected bead id: %s", snap.ID)
				}
				return nil
			},
		},
		{
			Name:  "beadsByProject",
			Query: `query Q($project: String!, $first: Int!) { beadsByProject(projectID: $project, first: $first) { totalCount edges { node { id title status } } } }`,
			InProcess: func(f *BeadFixture) error {
				snaps := f.Server.State().GetBeadSnapshotsForProject(f.TargetProject.ID, "", "", "")
				if len(snaps) == 0 {
					return fmt.Errorf("no snapshots for project %s", f.TargetProject.ID)
				}
				return nil
			},
		},
		{
			Name:  "beadsByProject_statusOpen",
			Query: `query Q($project: String!, $first: Int!) { beadsByProject(projectID: $project, first: $first, status: "open") { totalCount edges { node { id } } } }`,
			InProcess: func(f *BeadFixture) error {
				snaps := f.Server.State().GetBeadSnapshotsForProject(f.TargetProject.ID, "open", "", "")
				if len(snaps) == 0 {
					return fmt.Errorf("no open snapshots for project %s", f.TargetProject.ID)
				}
				return nil
			},
		},
		{
			Name:  "beads_crossProject",
			Query: `query Q($first: Int!) { beads(first: $first) { totalCount edges { node { id title status } } } }`,
			InProcess: func(f *BeadFixture) error {
				snaps := f.Server.State().GetBeadSnapshots("", "", "", "")
				if len(snaps) != f.TotalBeads() {
					return fmt.Errorf("cross-project snapshots: want %d, got %d", f.TotalBeads(), len(snaps))
				}
				return nil
			},
		},
		{
			Name:  "docGraph",
			Query: `{ docGraph { rootDir documents { id path title } } }`,
			InProcess: func(f *BeadFixture) error {
				// The docGraph resolver is the in-process call — there is no
				// cheaper "direct state" equivalent for it, so we exercise
				// the HTTP path twice (once here via the handler, once in
				// the HTTP measurement below). The second invocation still
				// reports the distribution we care about.
				resp, err := postGraphQL(f.Handler, `{ docGraph { rootDir documents { id path title } } }`, nil)
				if err != nil {
					return err
				}
				if len(resp.Errors) > 0 {
					return fmt.Errorf("docGraph errors: %+v", resp.Errors)
				}
				return nil
			},
		},
		{
			Name:  "agentSessions",
			Query: `{ agentSessions(first: 50) { totalCount edges { node { id harness model } } } }`,
			InProcess: func(f *BeadFixture) error {
				sessions := f.Server.State().GetAgentSessionsGraphQL()
				_ = sessions // may be empty when fixture seeds 0 sessions; still exercised
				return nil
			},
		},
	}
}

// RunMatrix executes every Target `iterations` times, once via the in-process
// call and once via the HTTP handler, and returns the combined report. If
// iterations ≤ 0 the harness uses DefaultIterations.
func RunMatrix(tb testing.TB, f *BeadFixture, iterations int) MatrixReport {
	tb.Helper()
	if iterations <= 0 {
		iterations = DefaultIterations
	}
	targets := Targets()

	// Warm once per target so the first sample doesn't carry cold-cache cost
	// (bead-location index, os page cache). The p50/p95/p99 we report reflect
	// steady-state behaviour, which is what users feel on repeat navigations.
	for _, t := range targets {
		if t.InProcess != nil {
			_ = t.InProcess(f)
		}
		if !t.SkipInHTTP && t.Query != "" {
			_, _ = postGraphQL(f.Handler, t.Query, variablesFor(t, f))
		}
	}

	results := make([]Result, 0, len(targets))
	for _, t := range targets {
		r := Result{Name: t.Name, Iterations: iterations}
		if t.InProcess != nil {
			samples := make([]time.Duration, 0, iterations)
			for i := 0; i < iterations; i++ {
				start := time.Now()
				if err := t.InProcess(f); err != nil {
					tb.Fatalf("perf: target %s in-process failed: %v", t.Name, err)
				}
				samples = append(samples, time.Since(start))
			}
			r.InProcess = percentileMillis(samples)
		}
		if !t.SkipInHTTP && t.Query != "" {
			samples := make([]time.Duration, 0, iterations)
			vars := variablesFor(t, f)
			for i := 0; i < iterations; i++ {
				start := time.Now()
				resp, err := postGraphQL(f.Handler, t.Query, vars)
				elapsed := time.Since(start)
				if err != nil {
					tb.Fatalf("perf: target %s HTTP failed: %v", t.Name, err)
				}
				if len(resp.Errors) > 0 {
					tb.Fatalf("perf: target %s graphql errors: %+v", t.Name, resp.Errors)
				}
				samples = append(samples, elapsed)
			}
			p := percentileMillis(samples)
			r.HTTP = &p
		}
		results = append(results, r)
	}

	return MatrixReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Environment: Environment(),
		Fixture: FixtureID{
			Projects:                f.Spec.Projects,
			BeadsPerProject:         f.Spec.BeadsPerProject,
			TotalBeads:              f.TotalBeads(),
			AgentSessionsPerProject: f.Spec.AgentSessionsPerProject,
		},
		Targets: results,
	}
}

func variablesFor(t Target, f *BeadFixture) map[string]any {
	switch t.Name {
	case "bead":
		return map[string]any{"id": f.TargetBeadID}
	case "beadsByProject", "beadsByProject_statusOpen":
		return map[string]any{"project": f.TargetProject.ID, "first": 50}
	case "beads_crossProject":
		return map[string]any{"first": 50}
	default:
		return nil
	}
}

type graphQLResponse struct {
	Data   json.RawMessage  `json:"data"`
	Errors []map[string]any `json:"errors,omitempty"`
}

// postGraphQL submits one GraphQL query through the HTTP handler and returns
// the decoded response. It is the single HTTP entry point used by both the
// harness and the smoke tests so they share transport timings.
func postGraphQL(h http.Handler, query string, variables map[string]any) (graphQLResponse, error) {
	payload := map[string]any{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return graphQLResponse{}, fmt.Errorf("marshal: %w", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return graphQLResponse{}, fmt.Errorf("status %d: %s", w.Code, w.Body.String())
	}
	var resp graphQLResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		return graphQLResponse{}, fmt.Errorf("decode: %w body=%s", err, w.Body.String())
	}
	return resp, nil
}

// PostGraphQL is the exported counterpart used by smoke tests in other
// packages that want to reuse the harness's HTTP entry point.
func PostGraphQL(h http.Handler, query string, variables map[string]any) error {
	resp, err := postGraphQL(h, query, variables)
	if err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return fmt.Errorf("graphql errors: %+v", resp.Errors)
	}
	return nil
}

func percentileMillis(samples []time.Duration) Percentiles {
	if len(samples) == 0 {
		return Percentiles{}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return Percentiles{
		P50: toMillis(percentile(samples, 50)),
		P95: toMillis(percentile(samples, 95)),
		P99: toMillis(percentile(samples, 99)),
	}
}

func percentile(sorted []time.Duration, pct int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted)*pct + 99) / 100
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return sorted[idx-1]
}

func toMillis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}
