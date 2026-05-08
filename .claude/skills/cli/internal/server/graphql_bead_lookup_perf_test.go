package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

const (
	beadLookupProjectCount     = 5
	beadLookupBeadsPerProject  = 100
	beadLookupTotalBeadCount   = beadLookupProjectCount * beadLookupBeadsPerProject
	beadLookupSampleIterations = 80
)

type beadLookupFixture struct {
	server        *Server
	handler       http.Handler
	projects      []ProjectEntry
	targetProject ProjectEntry
	target        bead.Bead
}

type graphQLBeadResponse struct {
	Data struct {
		Bead *struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Status    string `json:"status"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"bead"`
		BeadUpdate *struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"beadUpdate"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type legacyBeadLookupProvider struct {
	ddxgraphql.StateProvider
}

func (p legacyBeadLookupProvider) GetBeadSnapshot(id string) (*ddxgraphql.BeadSnapshot, bool) {
	for _, snap := range p.GetBeadSnapshots("", "", "", "") {
		if snap.ID == id {
			s := snap
			return &s, true
		}
	}
	return nil, false
}

func setupBeadLookupFixture(tb testing.TB) *beadLookupFixture {
	tb.Helper()
	tb.Setenv("XDG_DATA_HOME", tb.TempDir())
	tb.Setenv("DDX_NODE_NAME", "gql-bead-lookup-perf")

	root := tb.TempDir()
	projectPaths := make([]string, 0, beadLookupProjectCount)
	var target bead.Bead
	for projectIdx := 0; projectIdx < beadLookupProjectCount; projectIdx++ {
		projectPath := filepath.Join(root, fmt.Sprintf("project-%02d", projectIdx))
		beads := seedBeadLookupProject(tb, projectPath, projectIdx, beadLookupBeadsPerProject)
		projectPaths = append(projectPaths, projectPath)
		if projectIdx == beadLookupProjectCount-1 {
			target = beads[len(beads)-1]
		}
	}

	srv := New(":0", projectPaths[0])
	for _, projectPath := range projectPaths[1:] {
		srv.state.RegisterProject(projectPath)
	}
	projects := srv.state.GetProjects()
	if len(projects) != beadLookupProjectCount {
		tb.Fatalf("fixture projects: want %d, got %d", beadLookupProjectCount, len(projects))
	}

	var targetProject ProjectEntry
	for _, proj := range projects {
		if proj.Path == projectPaths[len(projectPaths)-1] {
			targetProject = proj
			break
		}
	}
	if targetProject.ID == "" {
		tb.Fatal("target project was not registered")
	}

	return &beadLookupFixture{
		server:        srv,
		handler:       srv.Handler(),
		projects:      projects,
		targetProject: targetProject,
		target:        target,
	}
}

func seedBeadLookupProject(tb testing.TB, projectPath string, projectIdx, beadCount int) []bead.Bead {
	tb.Helper()
	store := bead.NewStore(filepath.Join(projectPath, ".ddx"))
	if err := store.Init(); err != nil {
		tb.Fatalf("init bead store: %v", err)
	}
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	beads := make([]bead.Bead, 0, beadCount)
	for beadIdx := 0; beadIdx < beadCount; beadIdx++ {
		id := fmt.Sprintf("ddx-perf-%02d-%03d", projectIdx, beadIdx)
		now := baseTime.Add(time.Duration(projectIdx*beadCount+beadIdx) * time.Second)
		beads = append(beads, bead.Bead{
			ID:          id,
			Title:       fmt.Sprintf("Lookup fixture bead %02d/%03d", projectIdx, beadIdx),
			Status:      bead.StatusOpen,
			Priority:    bead.DefaultPriority,
			IssueType:   bead.DefaultType,
			CreatedAt:   now,
			UpdatedAt:   now,
			Labels:      []string{"perf", fmt.Sprintf("project-%02d", projectIdx)},
			Description: "GraphQL bead lookup performance fixture",
		})
	}
	if err := store.WriteAll(beads); err != nil {
		tb.Fatalf("seed bead store: %v", err)
	}
	return beads
}

func newGraphQLBeadLookupHandler(state ddxgraphql.StateProvider, workDir string) http.Handler {
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:      state,
			WorkingDir: workDir,
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gqlSrv.AddTransport(transport.POST{})
	return gqlSrv
}

func warmBeadLookupIndex(tb testing.TB, h http.Handler, projectID string) {
	tb.Helper()
	query := fmt.Sprintf(`{ beadsByProject(projectID: %q, first: 500) { totalCount edges { node { id title } } } }`, projectID)
	postGraphQLBeadLookup(tb, h, query)
}

func postGraphQLBeadLookup(tb testing.TB, h http.Handler, query string) graphQLBeadResponse {
	tb.Helper()
	resp, err := postGraphQLBeadLookupErr(h, query)
	if err != nil {
		tb.Fatal(err)
	}
	return resp
}

func postGraphQLBeadLookupErr(h http.Handler, query string) (graphQLBeadResponse, error) {
	rawBody, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return graphQLBeadResponse{}, fmt.Errorf("marshal GraphQL request: %w", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return graphQLBeadResponse{}, fmt.Errorf("GraphQL status: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp graphQLBeadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		return graphQLBeadResponse{}, fmt.Errorf("decode GraphQL response: %w\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		return graphQLBeadResponse{}, fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}
	return resp, nil
}

func beadDetailQuery(id string) string {
	return fmt.Sprintf(`{ bead(id: %q) { id title status updatedAt } }`, id)
}

func beadUpdateMutation(id, title string) string {
	return fmt.Sprintf(`mutation { beadUpdate(id: %q, input: { title: %q }) { id title updatedAt } }`, id, title)
}

func measureGraphQLBeadLookup(tb testing.TB, h http.Handler, beadID string, iterations int) (time.Duration, time.Duration) {
	tb.Helper()
	samples := make([]time.Duration, 0, iterations)
	query := beadDetailQuery(beadID)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		resp := postGraphQLBeadLookup(tb, h, query)
		elapsed := time.Since(start)
		if resp.Data.Bead == nil || resp.Data.Bead.ID != beadID || resp.Data.Bead.Title == "" {
			tb.Fatalf("unexpected bead response: %+v", resp.Data.Bead)
		}
		samples = append(samples, elapsed)
	}
	return percentileDurations(samples)
}

func percentileDurations(samples []time.Duration) (time.Duration, time.Duration) {
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return percentileDuration(samples, 50), percentileDuration(samples, 95)
}

func percentileDuration(sorted []time.Duration, pct int) time.Duration {
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

func reportLatencyPercentiles(b *testing.B, p50, p95 time.Duration) {
	b.ReportMetric(float64(p50.Microseconds())/1000, "p50_ms")
	b.ReportMetric(float64(p95.Microseconds())/1000, "p95_ms")
}

func BenchmarkGraphQLBeadLookupLatency(b *testing.B) {
	fixture := setupBeadLookupFixture(b)
	warmBeadLookupIndex(b, fixture.handler, fixture.targetProject.ID)

	samples := make([]time.Duration, 0, b.N)
	query := beadDetailQuery(fixture.target.ID)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		resp := postGraphQLBeadLookup(b, fixture.handler, query)
		if resp.Data.Bead == nil || resp.Data.Bead.ID != fixture.target.ID {
			b.Fatalf("unexpected bead response: %+v", resp.Data.Bead)
		}
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()
	p50, p95 := percentileDurations(samples)
	reportLatencyPercentiles(b, p50, p95)
}

func BenchmarkGraphQLBeadLookupLegacyScanLatency(b *testing.B) {
	fixture := setupBeadLookupFixture(b)
	h := newGraphQLBeadLookupHandler(
		legacyBeadLookupProvider{StateProvider: fixture.server.state},
		fixture.projects[0].Path,
	)

	samples := make([]time.Duration, 0, b.N)
	query := beadDetailQuery(fixture.target.ID)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		resp := postGraphQLBeadLookup(b, h, query)
		if resp.Data.Bead == nil || resp.Data.Bead.ID != fixture.target.ID {
			b.Fatalf("unexpected bead response: %+v", resp.Data.Bead)
		}
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()
	p50, p95 := percentileDurations(samples)
	reportLatencyPercentiles(b, p50, p95)
}

func TestServerStateBeadSnapshotLatencyBudget(t *testing.T) {
	fixture := setupBeadLookupFixture(t)
	warmBeadLookupIndex(t, fixture.handler, fixture.targetProject.ID)

	samples := make([]time.Duration, 0, beadLookupSampleIterations)
	for i := 0; i < beadLookupSampleIterations; i++ {
		start := time.Now()
		snap, ok := fixture.server.state.GetBeadSnapshot(fixture.target.ID)
		elapsed := time.Since(start)
		if !ok || snap.ID != fixture.target.ID || snap.Title == "" {
			t.Fatalf("unexpected snapshot: ok=%v snap=%+v", ok, snap)
		}
		samples = append(samples, elapsed)
	}
	p50, p95 := percentileDurations(samples)
	t.Logf("GetBeadSnapshot latency over %d beads / %d projects: p50=%s p95=%s", beadLookupTotalBeadCount, beadLookupProjectCount, p50, p95)
	if p95 > 50*time.Millisecond {
		t.Fatalf("GetBeadSnapshot p95 = %s, want <= 50ms", p95)
	}
}

func TestGraphQLBeadLookupLatencyBudget(t *testing.T) {
	fixture := setupBeadLookupFixture(t)
	warmBeadLookupIndex(t, fixture.handler, fixture.targetProject.ID)

	p50, p95 := measureGraphQLBeadLookup(t, fixture.handler, fixture.target.ID, beadLookupSampleIterations)
	t.Logf("GraphQL bead(id:) latency over %d beads / %d projects: p50=%s p95=%s", beadLookupTotalBeadCount, beadLookupProjectCount, p50, p95)
	if p95 > 200*time.Millisecond {
		t.Fatalf("GraphQL bead(id:) p95 = %s, want <= 200ms", p95)
	}
}

func TestGraphQLBeadLookupReadsExternalCLIUpdateOnNextRequest(t *testing.T) {
	fixture := setupBeadLookupFixture(t)
	warmBeadLookupIndex(t, fixture.handler, fixture.targetProject.ID)

	updatedTitle := "Updated through ddx bead update"
	runDDXBeadUpdate(t, fixture.targetProject.Path, fixture.target.ID, updatedTitle)

	resp := postGraphQLBeadLookup(t, fixture.handler, beadDetailQuery(fixture.target.ID))
	if resp.Data.Bead == nil {
		t.Fatal("bead lookup returned null after update")
	}
	if resp.Data.Bead.Title != updatedTitle {
		t.Fatalf("bead title after ddx bead update: want %q, got %q", updatedTitle, resp.Data.Bead.Title)
	}
}

func runDDXBeadUpdate(t *testing.T, projectPath, beadID, title string) {
	t.Helper()
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get package dir: %v", err)
	}
	moduleRoot, err := filepath.Abs(filepath.Join(pkgDir, "../.."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	cmd := exec.Command("go", "run", ".", "bead", "update", beadID, "--title", title)
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(),
		"DDX_DISABLE_UPDATE_CHECK=1",
		"DDX_BEAD_DIR="+filepath.Join(projectPath, ".ddx"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ddx bead update failed: %v\n%s", err, out)
	}
}

func TestGraphQLBeadLookupConcurrentQueriesAndUpdates(t *testing.T) {
	fixture := setupBeadLookupFixture(t)
	target := "ddx-perf-00-099"
	warmBeadLookupIndex(t, fixture.handler, fixture.projects[0].ID)

	const workers = 8
	const iterations = 20
	errs := make(chan error, workers*iterations*2)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iter := 0; iter < iterations; iter++ {
				title := fmt.Sprintf("concurrent update worker=%02d iter=%02d", worker, iter)
				updateResp, err := postGraphQLBeadLookupErr(fixture.handler, beadUpdateMutation(target, title))
				if err != nil {
					errs <- err
					return
				}
				if updateResp.Data.BeadUpdate == nil || updateResp.Data.BeadUpdate.ID != target {
					errs <- fmt.Errorf("unexpected update response: %+v", updateResp.Data.BeadUpdate)
					return
				}
				queryResp, err := postGraphQLBeadLookupErr(fixture.handler, beadDetailQuery(target))
				if err != nil {
					errs <- err
					return
				}
				if queryResp.Data.Bead == nil || queryResp.Data.Bead.ID != target {
					errs <- fmt.Errorf("unexpected query response: %+v", queryResp.Data.Bead)
					return
				}
				if queryResp.Data.Bead.Title == "" || !strings.Contains(queryResp.Data.Bead.Title, "update") {
					errs <- fmt.Errorf("torn bead response title %q", queryResp.Data.Bead.Title)
					return
				}
				if queryResp.Data.Bead.UpdatedAt == "" {
					errs <- fmt.Errorf("missing updatedAt in query response: %+v", queryResp.Data.Bead)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
