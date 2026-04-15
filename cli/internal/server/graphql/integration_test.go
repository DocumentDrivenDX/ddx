package graphql_test

// TC-GQL-INT-001..010: GraphQL integration test suite — queries, mutations, subscriptions.
//
// All tests run against real bead stores backed by t.TempDir() + bead.NewStore
// and a real git repository. StateProvider is implemented as a thin in-process
// stub that loads data from the real bead store at construction time.
//
// Categories covered:
//   - Queries:    nodeInfo, projects, beads (with pagination), beadsByProject
//   - Mutations:  beadCreate, beadUpdate, beadClaim, beadUnclaim, beadReopen
//   - Subscriptions: beadLifecycle over a real WebSocket

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
)

// TestMain scrubs GIT_* environment variables so subprocess git calls in tests
// write to temp directories rather than the parent repository.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				_ = os.Unsetenv(kv[:idx])
			}
		}
	}
	os.Exit(m.Run())
}

// ─────────────────────────── shared setup ───────────────────────────────────

// setupIntegrationDir creates a temp directory with a real git repository and
// an initialised bead store. It returns the working directory and the store.
func setupIntegrationDir(t *testing.T) (workDir string, store *bead.Store) {
	t.Helper()
	workDir = t.TempDir()

	// Real git init.
	if out, err := exec.Command("git", "init", workDir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	ddxDir := filepath.Join(workDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Minimal config so bead.NewStore can resolve an ID prefix.
	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\n"
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Real bead store.
	store = bead.NewStore(ddxDir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	return workDir, store
}

// newGQLHandler builds a gqlgen HTTP handler with the supplied resolver fields.
func newGQLHandler(state ddxgraphql.StateProvider, workDir string, beadBus ddxgraphql.BeadLifecycleSubscriber) http.Handler {
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:      state,
			WorkingDir: workDir,
			BeadBus:    beadBus,
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gqlSrv.AddTransport(transport.POST{})
	gqlSrv.AddTransport(transport.GET{})
	gqlSrv.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	})
	return gqlSrv
}

// ─────────────────────────── testStateProvider ──────────────────────────────

// testStateProvider is a StateProvider backed by real bead data read from a
// bead.Store at construction time. It holds snapshots loaded from disk and
// filters them in-process to satisfy resolver queries.
type testStateProvider struct {
	node     ddxgraphql.NodeStateSnapshot
	projects []*ddxgraphql.Project
	beads    []ddxgraphql.BeadSnapshot
}

func newTestStateProvider(workDir string, store *bead.Store) *testStateProvider {
	now := time.Now()
	projID := "proj-integration-" + filepath.Base(workDir)

	allBeads, _ := store.ReadAll()
	snaps := make([]ddxgraphql.BeadSnapshot, 0, len(allBeads))
	for _, b := range allBeads {
		snaps = append(snaps, ddxgraphql.BeadSnapshot{
			ProjectID: projID,
			ID:        b.ID,
			Title:     b.Title,
			Status:    b.Status,
			Priority:  b.Priority,
			IssueType: b.IssueType,
			Owner:     b.Owner,
			CreatedAt: b.CreatedAt,
			UpdatedAt: b.UpdatedAt,
			Labels:    b.Labels,
		})
	}

	registeredAt := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	lastSeen := now.UTC().Format(time.RFC3339)
	return &testStateProvider{
		node: ddxgraphql.NodeStateSnapshot{
			ID:        "node-integration-test",
			Name:      "integration-test-node",
			StartedAt: now.Add(-time.Hour),
			LastSeen:  now,
		},
		projects: []*ddxgraphql.Project{
			{
				ID:           projID,
				Name:         "integration-test-project",
				Path:         workDir,
				RegisteredAt: registeredAt,
				LastSeen:     lastSeen,
			},
		},
		beads: snaps,
	}
}

func (p *testStateProvider) GetNodeSnapshot() ddxgraphql.NodeStateSnapshot { return p.node }

func (p *testStateProvider) GetProjectSnapshots(_ bool) []*ddxgraphql.Project {
	return p.projects
}

func (p *testStateProvider) GetProjectSnapshotByID(id string) (*ddxgraphql.Project, bool) {
	for _, proj := range p.projects {
		if proj.ID == id {
			return proj, true
		}
	}
	return nil, false
}

func (p *testStateProvider) GetBeadSnapshots(status, label, projectID, search string) []ddxgraphql.BeadSnapshot {
	var out []ddxgraphql.BeadSnapshot
	for _, b := range p.beads {
		if status != "" && b.Status != status {
			continue
		}
		if label != "" {
			found := false
			for _, l := range b.Labels {
				if l == label {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if projectID != "" && b.ProjectID != projectID {
			continue
		}
		out = append(out, b)
	}
	return out
}

// No-op implementations for resolver methods not exercised by these tests.
func (p *testStateProvider) GetWorkersGraphQL(_ string) []*ddxgraphql.Worker { return nil }
func (p *testStateProvider) GetWorkerGraphQL(_ string) (*ddxgraphql.Worker, bool) {
	return nil, false
}
func (p *testStateProvider) GetWorkerLogGraphQL(_ string) *ddxgraphql.WorkerLog { return nil }
func (p *testStateProvider) GetWorkerProgressGraphQL(_ string) []*ddxgraphql.PhaseTransition {
	return nil
}
func (p *testStateProvider) GetWorkerPromptGraphQL(_ string) string              { return "" }
func (p *testStateProvider) GetAgentSessionsGraphQL() []*ddxgraphql.AgentSession { return nil }
func (p *testStateProvider) GetAgentSessionGraphQL(_ string) (*ddxgraphql.AgentSession, bool) {
	return nil, false
}
func (p *testStateProvider) GetExecDefinitionsGraphQL(_ string) []*ddxgraphql.ExecutionDefinition {
	return nil
}
func (p *testStateProvider) GetExecDefinitionGraphQL(_ string) (*ddxgraphql.ExecutionDefinition, bool) {
	return nil, false
}
func (p *testStateProvider) GetExecRunsGraphQL(_, _ string) []*ddxgraphql.ExecutionRun { return nil }
func (p *testStateProvider) GetExecRunGraphQL(_ string) (*ddxgraphql.ExecutionRun, bool) {
	return nil, false
}
func (p *testStateProvider) GetExecRunLogGraphQL(_ string) *ddxgraphql.ExecutionRunLog { return nil }
func (p *testStateProvider) GetCoordinatorsGraphQL() []*ddxgraphql.CoordinatorMetricsEntry {
	return nil
}
func (p *testStateProvider) GetCoordinatorMetricsByProjectGraphQL(_ string) *ddxgraphql.CoordinatorMetrics {
	return nil
}

// ─────────────────────────── stubBeadBus ────────────────────────────────────

// stubBeadBus satisfies BeadLifecycleSubscriber and delivers a pre-loaded
// channel of events.
type stubBeadBus struct {
	ch chan bead.LifecycleEvent
}

func (s *stubBeadBus) SubscribeLifecycle(_ string) (<-chan bead.LifecycleEvent, func()) {
	return s.ch, func() {}
}

// ─────────────────────────── WebSocket helpers ──────────────────────────────

// wsMsg is a minimal graphql-transport-ws protocol message.
type wsMsg struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ─────────────────────────── HTTP query helper ──────────────────────────────

// gqlPost sends a GraphQL request via HTTP POST and returns the parsed body.
// It fails the test immediately on HTTP errors or top-level GraphQL errors.
func gqlPost(t *testing.T, h http.Handler, query string) map[string]json.RawMessage {
	t.Helper()
	rawBody, _ := json.Marshal(map[string]string{"query": query})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("GraphQL errors: %s", errs)
	}
	return resp
}

// ═══════════════════════════ QUERY TESTS ════════════════════════════════════

// TC-GQL-INT-001: Query.nodeInfo returns server node identity from StateProvider.
func TestIntegration_Query_NodeInfo(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	resp := gqlPost(t, h, `{ nodeInfo { id name startedAt lastSeen } }`)

	var data struct {
		NodeInfo struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			StartedAt string `json:"startedAt"`
			LastSeen  string `json:"lastSeen"`
		} `json:"nodeInfo"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	ni := data.NodeInfo
	if ni.ID != "node-integration-test" {
		t.Errorf("id: want %q, got %q", "node-integration-test", ni.ID)
	}
	if ni.Name != "integration-test-node" {
		t.Errorf("name: want %q, got %q", "integration-test-node", ni.Name)
	}
	if ni.StartedAt == "" {
		t.Error("expected non-empty startedAt")
	}
	if ni.LastSeen == "" {
		t.Error("expected non-empty lastSeen")
	}
}

// TC-GQL-INT-002: Query.projects returns the registered project with correct path.
func TestIntegration_Query_Projects(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	resp := gqlPost(t, h, `{ projects(first: 10) { edges { node { id name path } } totalCount } }`)

	var data struct {
		Projects struct {
			Edges []struct {
				Node struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Path string `json:"path"`
				} `json:"node"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	if data.Projects.TotalCount != 1 {
		t.Errorf("totalCount: want 1, got %d", data.Projects.TotalCount)
	}
	if len(data.Projects.Edges) != 1 {
		t.Fatalf("edges: want 1, got %d", len(data.Projects.Edges))
	}
	proj := data.Projects.Edges[0].Node
	if proj.Name != "integration-test-project" {
		t.Errorf("name: want %q, got %q", "integration-test-project", proj.Name)
	}
	if proj.Path != workDir {
		t.Errorf("path: want %q, got %q", workDir, proj.Path)
	}
}

// TC-GQL-INT-003: Query.beads returns beads loaded from real bead store, with pagination.
func TestIntegration_Query_Beads(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	// Create 3 beads in the real store before constructing the state provider.
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		b := &bead.Bead{Title: title, IssueType: "task", Priority: 1}
		if err := store.Create(b); err != nil {
			t.Fatalf("create bead %q: %v", title, err)
		}
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	resp := gqlPost(t, h, `{ beads(first: 10) { edges { node { id title status } } totalCount pageInfo { hasNextPage } } }`)

	var data struct {
		Beads struct {
			Edges []struct {
				Node struct {
					ID     string `json:"id"`
					Title  string `json:"title"`
					Status string `json:"status"`
				} `json:"node"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
			PageInfo   struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
		} `json:"beads"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	if data.Beads.TotalCount != 3 {
		t.Errorf("totalCount: want 3, got %d", data.Beads.TotalCount)
	}
	if len(data.Beads.Edges) != 3 {
		t.Errorf("edges: want 3, got %d", len(data.Beads.Edges))
	}
	if data.Beads.PageInfo.HasNextPage {
		t.Error("hasNextPage: want false when first=10 and only 3 beads exist")
	}
	for _, e := range data.Beads.Edges {
		if e.Node.Status != bead.StatusOpen {
			t.Errorf("bead %q: status want %q, got %q", e.Node.ID, bead.StatusOpen, e.Node.Status)
		}
	}
}

// TC-GQL-INT-004: Query.beadsByProject scopes results to a specific project ID.
func TestIntegration_Query_BeadsByProject(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	for _, title := range []string{"P1", "P2"} {
		b := &bead.Bead{Title: title, IssueType: "task"}
		if err := store.Create(b); err != nil {
			t.Fatalf("create bead %q: %v", title, err)
		}
	}

	state := newTestStateProvider(workDir, store)
	projID := state.projects[0].ID
	h := newGQLHandler(state, workDir, nil)

	query := `{ beadsByProject(projectID: "` + projID + `", first: 10) { edges { node { id title } } totalCount } }`
	resp := gqlPost(t, h, query)

	var data struct {
		BeadsByProject struct {
			Edges []struct {
				Node struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"node"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"beadsByProject"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	if data.BeadsByProject.TotalCount != 2 {
		t.Errorf("totalCount: want 2, got %d", data.BeadsByProject.TotalCount)
	}
	if len(data.BeadsByProject.Edges) != 2 {
		t.Errorf("edges: want 2, got %d", len(data.BeadsByProject.Edges))
	}
}

// ═══════════════════════════ MUTATION TESTS ═════════════════════════════════

// TC-GQL-INT-005: Mutation.beadCreate writes a new bead to the real store.
func TestIntegration_Mutation_BeadCreate(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadCreate(input: {
			title: "Integration test bead"
			issueType: "task"
			priority: 2
			labels: ["integration", "test"]
			description: "Created in integration test"
		}) { id title status priority issueType labels description }
	}`

	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadCreate struct {
			ID          string   `json:"id"`
			Title       string   `json:"title"`
			Status      string   `json:"status"`
			Priority    int      `json:"priority"`
			IssueType   string   `json:"issueType"`
			Labels      []string `json:"labels"`
			Description *string  `json:"description"`
		} `json:"beadCreate"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	b := data.BeadCreate
	if b.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if b.Title != "Integration test bead" {
		t.Errorf("title: want %q, got %q", "Integration test bead", b.Title)
	}
	if b.Status != bead.StatusOpen {
		t.Errorf("status: want %q, got %q", bead.StatusOpen, b.Status)
	}
	if b.Priority != 2 {
		t.Errorf("priority: want 2, got %d", b.Priority)
	}
	if b.IssueType != "task" {
		t.Errorf("issueType: want %q, got %q", "task", b.IssueType)
	}
	if len(b.Labels) != 2 {
		t.Errorf("labels: want 2, got %d: %v", len(b.Labels), b.Labels)
	}
	if b.Description == nil || *b.Description != "Created in integration test" {
		t.Errorf("description: unexpected value %v", b.Description)
	}

	// Verify the bead is persisted to the real store on disk.
	got, err := store.Get(b.ID)
	if err != nil {
		t.Fatalf("store.Get after create: %v", err)
	}
	if got.Title != "Integration test bead" {
		t.Errorf("store bead title: want %q, got %q", "Integration test bead", got.Title)
	}
}

// TC-GQL-INT-006: Mutation.beadUpdate modifies fields on an existing real bead.
func TestIntegration_Mutation_BeadUpdate(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	orig := &bead.Bead{Title: "Original title", IssueType: "task", Priority: 1}
	if err := store.Create(orig); err != nil {
		t.Fatalf("create bead: %v", err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadUpdate(id: "` + orig.ID + `", input: {
			title: "Updated title"
			priority: 3
		}) { id title priority status }
	}`

	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadUpdate struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Priority int    `json:"priority"`
			Status   string `json:"status"`
		} `json:"beadUpdate"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	b := data.BeadUpdate
	if b.ID != orig.ID {
		t.Errorf("id: want %q, got %q", orig.ID, b.ID)
	}
	if b.Title != "Updated title" {
		t.Errorf("title: want %q, got %q", "Updated title", b.Title)
	}
	if b.Priority != 3 {
		t.Errorf("priority: want 3, got %d", b.Priority)
	}
	if b.Status != bead.StatusOpen {
		t.Errorf("status: want %q unchanged, got %q", bead.StatusOpen, b.Status)
	}
}

// TC-GQL-INT-007: Mutation.beadClaim transitions bead to in_progress with assignee.
func TestIntegration_Mutation_BeadClaim(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	b := &bead.Bead{Title: "Claimable bead", IssueType: "task"}
	if err := store.Create(b); err != nil {
		t.Fatalf("create bead: %v", err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadClaim(id: "` + b.ID + `", assignee: "agent-01") { id status owner }
	}`

	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadClaim struct {
			ID     string  `json:"id"`
			Status string  `json:"status"`
			Owner  *string `json:"owner"`
		} `json:"beadClaim"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	if data.BeadClaim.Status != bead.StatusInProgress {
		t.Errorf("status: want %q, got %q", bead.StatusInProgress, data.BeadClaim.Status)
	}
	if data.BeadClaim.Owner == nil || *data.BeadClaim.Owner != "agent-01" {
		t.Errorf("owner: want %q, got %v", "agent-01", data.BeadClaim.Owner)
	}
}

// TC-GQL-INT-008: Mutation.beadUnclaim reverts a claimed bead back to open.
func TestIntegration_Mutation_BeadUnclaim(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	b := &bead.Bead{Title: "To be unclaimed", IssueType: "task"}
	if err := store.Create(b); err != nil {
		t.Fatalf("create bead: %v", err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	// Claim first.
	claimMut := `mutation { beadClaim(id: "` + b.ID + `", assignee: "agent-99") { id status } }`
	gqlPost(t, h, claimMut)

	// Now unclaim.
	unclaimMut := `mutation { beadUnclaim(id: "` + b.ID + `") { id status owner } }`
	resp := gqlPost(t, h, unclaimMut)

	var data struct {
		BeadUnclaim struct {
			ID     string  `json:"id"`
			Status string  `json:"status"`
			Owner  *string `json:"owner"`
		} `json:"beadUnclaim"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	if data.BeadUnclaim.Status != bead.StatusOpen {
		t.Errorf("status: want %q after unclaim, got %q", bead.StatusOpen, data.BeadUnclaim.Status)
	}
	if data.BeadUnclaim.Owner != nil && *data.BeadUnclaim.Owner != "" {
		t.Errorf("owner: want nil/empty after unclaim, got %v", data.BeadUnclaim.Owner)
	}
}

// TC-GQL-INT-009: Mutation.beadReopen sets a closed bead back to open.
func TestIntegration_Mutation_BeadReopen(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	b := &bead.Bead{Title: "Will be closed then reopened", IssueType: "task"}
	if err := store.Create(b); err != nil {
		t.Fatalf("create bead: %v", err)
	}
	// Close the bead directly via the store.
	if err := store.Update(b.ID, func(x *bead.Bead) { x.Status = bead.StatusClosed }); err != nil {
		t.Fatalf("close bead: %v", err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation { beadReopen(id: "` + b.ID + `") { id status } }`
	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadReopen struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"beadReopen"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	if data.BeadReopen.Status != bead.StatusOpen {
		t.Errorf("status: want %q after reopen, got %q", bead.StatusOpen, data.BeadReopen.Status)
	}
}

// ═══════════════════════════ SUBSCRIPTION TESTS ═════════════════════════════

// TC-GQL-INT-010: Subscription.beadLifecycle delivers lifecycle events over a real WebSocket.
//
// Events are pre-loaded into a buffered channel and delivered through the real
// subscription resolver (resolver_sub_bead.go). The test verifies all events
// arrive in order and that the subscription closes cleanly.
func TestIntegration_Subscription_BeadLifecycle(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	state := newTestStateProvider(workDir, store)

	// Pre-load 2 lifecycle events into a buffered channel.
	eventCh := make(chan bead.LifecycleEvent, 4)
	now := time.Now().UTC()
	events := []bead.LifecycleEvent{
		{
			EventID:   "it-aabb-0001",
			BeadID:    "it-aabb",
			Kind:      "created",
			Summary:   "bead it-aabb created: integration test",
			Timestamp: now,
		},
		{
			EventID:   "it-aabb-0002",
			BeadID:    "it-aabb",
			Kind:      "status_changed",
			Summary:   "status changed from open to closed",
			Timestamp: now.Add(time.Second),
		},
	}
	for _, e := range events {
		eventCh <- e
	}
	close(eventCh)

	beadBus := &stubBeadBus{ch: eventCh}
	h := newGQLHandler(state, workDir, beadBus)

	ts := httptest.NewServer(h)
	defer ts.Close()

	// Establish WebSocket connection using graphql-transport-ws subprotocol.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/graphql"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, map[string][]string{
		"Sec-WebSocket-Protocol": {"graphql-transport-ws"},
	})
	if err != nil {
		t.Fatalf("WebSocket dial: %v", err)
	}
	defer conn.Close()

	send := func(msg wsMsg) {
		t.Helper()
		data, _ := json.Marshal(msg)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			t.Fatalf("WebSocket write: %v", err)
		}
	}
	recv := func() wsMsg {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("WebSocket read: %v", err)
		}
		var msg wsMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return msg
	}

	// graphql-transport-ws handshake.
	send(wsMsg{Type: "connection_init"})
	ack := recv()
	if ack.Type != "connection_ack" {
		t.Fatalf("expected connection_ack, got %q", ack.Type)
	}

	// Send subscription.
	queryPayload, _ := json.Marshal(map[string]string{
		"query": `subscription { beadLifecycle(projectID: "/integration/test") { eventID beadID kind summary } }`,
	})
	send(wsMsg{ID: "sub-1", Type: "subscribe", Payload: queryPayload})

	// Collect events until the subscription is complete.
	type eventItem struct {
		EventID string  `json:"eventID"`
		BeadID  string  `json:"beadID"`
		Kind    string  `json:"kind"`
		Summary *string `json:"summary"`
	}

	var received []eventItem
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
loop:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for subscription events; got %d so far", len(received))
		default:
		}
		msg := recv()
		switch msg.Type {
		case "next":
			var payload struct {
				Data struct {
					BL eventItem `json:"beadLifecycle"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				t.Fatalf("unmarshal next payload: %v", err)
			}
			received = append(received, payload.Data.BL)
		case "complete":
			break loop
		case "ping":
			send(wsMsg{Type: "pong"})
		default:
			t.Logf("unexpected message type: %q", msg.Type)
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	checks := []struct {
		eventID string
		beadID  string
		kind    string
		summary string
	}{
		{"it-aabb-0001", "it-aabb", "created", "bead it-aabb created: integration test"},
		{"it-aabb-0002", "it-aabb", "status_changed", "status changed from open to closed"},
	}
	for i, chk := range checks {
		got := received[i]
		if got.EventID != chk.eventID {
			t.Errorf("event[%d] eventID: want %q, got %q", i, chk.eventID, got.EventID)
		}
		if got.BeadID != chk.beadID {
			t.Errorf("event[%d] beadID: want %q, got %q", i, chk.beadID, got.BeadID)
		}
		if got.Kind != chk.kind {
			t.Errorf("event[%d] kind: want %q, got %q", i, chk.kind, got.Kind)
		}
		if got.Summary == nil || *got.Summary != chk.summary {
			t.Errorf("event[%d] summary: want %q, got %v", i, chk.summary, got.Summary)
		}
	}
}
