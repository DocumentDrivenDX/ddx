package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// mutationTestStateProvider is a minimal StateProvider for bead-mutation
// federation tests. Only project registry lookups are exercised.
type mutationTestStateProvider struct {
	node     NodeStateSnapshot
	projects []*Project
}

func (p *mutationTestStateProvider) GetNodeSnapshot() NodeStateSnapshot { return p.node }

func (p *mutationTestStateProvider) GetProjectSnapshots(_ bool) []*Project {
	out := make([]*Project, len(p.projects))
	for i := range p.projects {
		if p.projects[i] == nil {
			continue
		}
		copyProject := *p.projects[i]
		out[i] = &copyProject
	}
	return out
}

func (p *mutationTestStateProvider) GetProjectSnapshotByID(id string) (*Project, bool) {
	for _, proj := range p.projects {
		if proj != nil && proj.ID == id {
			copyProject := *proj
			return &copyProject, true
		}
	}
	return nil, false
}

func (p *mutationTestStateProvider) GetBeadSnapshots(_, _, _, _ string) []BeadSnapshot {
	return nil
}
func (p *mutationTestStateProvider) GetBeadSnapshotsForProject(_, _, _, _ string) []BeadSnapshot {
	return nil
}
func (p *mutationTestStateProvider) GetBeadSnapshot(string) (*BeadSnapshot, bool) {
	return nil, false
}
func (p *mutationTestStateProvider) GetWorkersGraphQL(string) []*Worker { return nil }
func (p *mutationTestStateProvider) GetWorkerGraphQL(string) (*Worker, bool) {
	return nil, false
}
func (p *mutationTestStateProvider) GetWorkerLogGraphQL(string) *WorkerLog { return nil }
func (p *mutationTestStateProvider) GetWorkerProgressGraphQL(string) []*PhaseTransition {
	return nil
}
func (p *mutationTestStateProvider) GetWorkerPromptGraphQL(string) string { return "" }
func (p *mutationTestStateProvider) GetAgentSessionsGraphQL(string, *time.Time, *time.Time) []*AgentSession {
	return nil
}
func (p *mutationTestStateProvider) GetAgentSessionGraphQL(string) (*AgentSession, bool) {
	return nil, false
}
func (p *mutationTestStateProvider) GetSessionsCostSummaryGraphQL(string, *time.Time, *time.Time) *SessionsCostSummary {
	return &SessionsCostSummary{}
}
func (p *mutationTestStateProvider) GetExecDefinitionsGraphQL(string) []*ExecutionDefinition {
	return nil
}
func (p *mutationTestStateProvider) GetExecDefinitionGraphQL(string) (*ExecutionDefinition, bool) {
	return nil, false
}
func (p *mutationTestStateProvider) GetExecRunsGraphQL(string, string) []*ExecutionRun { return nil }
func (p *mutationTestStateProvider) GetExecRunGraphQL(string) (*ExecutionRun, bool) {
	return nil, false
}
func (p *mutationTestStateProvider) GetExecRunLogGraphQL(string) *ExecutionRunLog { return nil }
func (p *mutationTestStateProvider) GetCoordinatorMetricsByProjectGraphQL(string) *CoordinatorMetrics {
	return nil
}
func (p *mutationTestStateProvider) GetCoordinatorsGraphQL() []*CoordinatorMetricsEntry {
	return nil
}

type mutationProjectFixture struct {
	root    string
	store   *bead.Store
	project Project
}

func newMutationProjectFixture(t *testing.T, projectID string) mutationProjectFixture {
	t.Helper()

	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)

	ddxDir := filepath.Join(root, ddxroot.DirName)
	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\nlibrary:\n  path: .ddx/plugins/ddx\n"
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	store := bead.NewStore(ddxDir)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init bead store: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return mutationProjectFixture{
		root:  root,
		store: store,
		project: Project{
			ID:           projectID,
			Name:         projectID,
			Path:         root,
			RegisteredAt: now,
			LastSeen:     now,
		},
	}
}

func newMutationState(nodeID string, fixtures ...mutationProjectFixture) *mutationTestStateProvider {
	projects := make([]*Project, 0, len(fixtures))
	for i := range fixtures {
		p := fixtures[i].project
		projects = append(projects, &p)
	}
	return &mutationTestStateProvider{
		node: NodeStateSnapshot{
			ID:        nodeID,
			Name:      nodeID,
			StartedAt: time.Now().Add(-time.Minute),
			LastSeen:  time.Now(),
		},
		projects: projects,
	}
}

func readAllBeads(t *testing.T, store *bead.Store) []bead.Bead {
	t.Helper()
	beads, err := store.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return beads
}

// mutationFederation is an in-process FederationProvider that routes
// ForwardMutation calls to spoke resolvers without real HTTP.
type mutationFederation struct {
	spokes        []federation.SpokeRecord
	runtimes      map[string]*mutationSpokeRuntime
	forwardCalls  int
	lastForwarded *federation.ForwardMutationRequest
}

type mutationSpokeRuntime struct {
	resolver *mutationResolver
	fixture  mutationProjectFixture
}

func newMutationSpokeRuntime(nodeID string, state *mutationTestStateProvider, fixture mutationProjectFixture) *mutationSpokeRuntime {
	return &mutationSpokeRuntime{
		resolver: &mutationResolver{&Resolver{
			State:      state,
			WorkingDir: fixture.root,
			NodeID:     nodeID,
		}},
		fixture: fixture,
	}
}

func (m *mutationFederation) Spokes() []federation.SpokeRecord {
	out := make([]federation.SpokeRecord, len(m.spokes))
	copy(out, m.spokes)
	return out
}

func (m *mutationFederation) FanOut(_ context.Context, _ *federation.FanOutRequest) (*federation.FanOutResult, error) {
	return &federation.FanOutResult{
		Responses:     map[string]json.RawMessage{},
		Errors:        map[string]error{},
		Skipped:       map[string]federation.SkipReason{},
		StatusUpdates: map[string]federation.SpokeStatus{},
		Nodes:         []federation.NodeResult{},
	}, nil
}

func (m *mutationFederation) ForwardMutation(ctx context.Context, req *federation.ForwardMutationRequest) (*federation.ForwardMutationResponse, error) {
	if m == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	m.forwardCalls++
	if req != nil {
		// Shallow copy so tests can inspect the request after ForwardMutation returns.
		cp := *req
		cp.ForwardingPath = append([]string(nil), req.ForwardingPath...)
		cp.Body = append([]byte(nil), req.Body...)
		if req.Headers != nil {
			cp.Headers = make(map[string]string, len(req.Headers))
			for k, v := range req.Headers {
				cp.Headers[k] = v
			}
		}
		m.lastForwarded = &cp
	}
	registry := federation.NewRegistry()
	for _, spoke := range m.spokes {
		if err := registry.UpsertSpoke(spoke); err != nil {
			return nil, err
		}
	}

	owner, err := federation.RouteMutationToProjectOwner(registry, req.TargetProjectID)
	if err != nil {
		if strings.Contains(err.Error(), "multiple registered owners") {
			return nil, federation.ErrForwardMutationBroadcastLike
		}
		return nil, federation.ErrForwardMutationMissingOwner
	}
	if owner.NodeID != req.TargetNodeID {
		return nil, federation.ErrForwardMutationBroadcastLike
	}
	if owner.Status == federation.StatusOffline {
		return nil, federation.ErrForwardMutationOffline
	}
	if owner.Status == federation.StatusStale {
		return nil, federation.ErrForwardMutationStale
	}
	if !beadWriteCapability(owner.Capabilities) {
		return nil, federation.ErrForwardMutationReadOnly
	}

	runtime := m.runtimes[owner.NodeID]
	if runtime == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}

	var envelope struct {
		Query     string                     `json:"query"`
		Variables map[string]json.RawMessage `json:"variables"`
	}
	if err := json.Unmarshal(req.Body, &envelope); err != nil {
		return nil, err
	}

	// Inject loop-prevention header so the spoke writes locally.
	httpReq, _ := http.NewRequest(http.MethodPost, "/graphql", nil)
	if originID := req.Headers[beadMutationOriginServerHeader]; originID != "" {
		httpReq.Header.Set(beadMutationOriginServerHeader, originID)
	}
	spokeCtx := WithHTTPRequest(context.Background(), httpReq)
	spokeCtx = WithWorkingDir(spokeCtx, runtime.fixture.root)

	var body []byte
	switch {
	case strings.Contains(envelope.Query, "beadCreate"):
		var input BeadInput
		if raw, ok := envelope.Variables["input"]; ok {
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
		}
		created, err := runtime.resolver.BeadCreate(spokeCtx, input)
		if err != nil {
			return nil, err
		}
		body, err = json.Marshal(map[string]any{
			"data": map[string]any{"beadCreate": created},
		})
		if err != nil {
			return nil, err
		}
	case strings.Contains(envelope.Query, "beadUpdate"):
		var input BeadUpdateInput
		var id string
		if raw, ok := envelope.Variables["input"]; ok {
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
		}
		if raw, ok := envelope.Variables["id"]; ok {
			if err := json.Unmarshal(raw, &id); err != nil {
				return nil, err
			}
		}
		updated, err := runtime.resolver.BeadUpdate(spokeCtx, id, input)
		if err != nil {
			return nil, err
		}
		body, err = json.Marshal(map[string]any{
			"data": map[string]any{"beadUpdate": updated},
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unexpected mutation envelope query: %s", envelope.Query)
	}

	return &federation.ForwardMutationResponse{
		OriginIdentity:  req.OriginIdentity,
		ForwardingPath:  append([]string(nil), req.ForwardingPath...),
		RequestID:       req.RequestID,
		IdempotencyKey:  req.IdempotencyKey,
		TargetNodeID:    req.TargetNodeID,
		TargetProjectID: req.TargetProjectID,
		StatusCode:      http.StatusOK,
		Headers:         http.Header{"Content-Type": []string{"application/json"}},
		Body:            body,
	}, nil
}

func beadWriteCapability(caps []string) bool {
	for _, cap := range caps {
		if strings.TrimSpace(cap) == "write" {
			return true
		}
	}
	return false
}

func beadFedStrPtr(s string) *string { return &s }

func beadFedDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// TestFederatedBeadWrite_ForwardsToOwner proves hub-mode beadCreate and
// beadUpdate for a spoke-owned project route through Federation.ForwardMutation
// to the owning spoke and never create a hub-local phantom bead. It also
// covers offline and missing-owner refusals (no hub-local write).
func TestFederatedBeadWrite_ForwardsToOwner(t *testing.T) {
	t.Run("create forwards to owner", func(t *testing.T) {
		hubA := newMutationProjectFixture(t, "proj-a")
		spokeA := newMutationProjectFixture(t, "proj-a")

		hubState := newMutationState("hub-node", hubA)
		spokeState := newMutationState("spoke-node", spokeA)

		fed := &mutationFederation{
			spokes: []federation.SpokeRecord{
				{
					NodeID:       "spoke-node",
					Name:         "spoke-a",
					ProjectIDs:   []string{"proj-a"},
					Capabilities: []string{"read", "write"},
					Status:       federation.StatusActive,
				},
			},
			runtimes: map[string]*mutationSpokeRuntime{
				"spoke-node": newMutationSpokeRuntime("spoke-node", spokeState, spokeA),
			},
		}

		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubA.root,
			Federation: fed,
			NodeID:     "hub-node",
		}}

		created, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{
			Title:       "federated create",
			IssueType:   beadFedStrPtr("task"),
			Labels:      []string{"federated"},
			Description: beadFedStrPtr("forward this bead"),
		})
		if err != nil {
			t.Fatalf("BeadCreate: %v", err)
		}
		if created.Title != "federated create" {
			t.Fatalf("title = %q, want federated create", created.Title)
		}
		if created.IssueType != "task" {
			t.Fatalf("issueType = %q, want task", created.IssueType)
		}

		if fed.forwardCalls != 1 {
			t.Fatalf("ForwardMutation calls = %d, want 1", fed.forwardCalls)
		}
		if fed.lastForwarded == nil {
			t.Fatal("expected lastForwarded request")
		}
		if fed.lastForwarded.TargetNodeID != "spoke-node" {
			t.Fatalf("TargetNodeID = %q, want spoke-node", fed.lastForwarded.TargetNodeID)
		}
		if fed.lastForwarded.TargetProjectID != "proj-a" {
			t.Fatalf("TargetProjectID = %q, want proj-a", fed.lastForwarded.TargetProjectID)
		}
		if len(fed.lastForwarded.ForwardingPath) < 2 {
			t.Fatalf("ForwardingPath = %v, want hub + spoke", fed.lastForwarded.ForwardingPath)
		}
		if got := fed.lastForwarded.Headers[beadMutationOriginServerHeader]; got != "hub-node" {
			t.Fatalf("origin server header = %q, want hub-node", got)
		}
		if !strings.Contains(string(fed.lastForwarded.Body), "beadCreate") {
			t.Fatalf("forward body missing beadCreate: %s", fed.lastForwarded.Body)
		}

		if beads := readAllBeads(t, hubA.store); len(beads) != 0 {
			t.Fatalf("hub-local store must stay empty, got %d beads", len(beads))
		}
		spokeBeads := readAllBeads(t, spokeA.store)
		if len(spokeBeads) != 1 {
			t.Fatalf("spoke beads = %d, want 1", len(spokeBeads))
		}
		if spokeBeads[0].Title != "federated create" {
			t.Fatalf("spoke title = %q", spokeBeads[0].Title)
		}
	})

	t.Run("update forwards to owner", func(t *testing.T) {
		hubA := newMutationProjectFixture(t, "proj-a")
		spokeA := newMutationProjectFixture(t, "proj-a")

		seed := &bead.Bead{Title: "before update"}
		if err := spokeA.store.Create(context.Background(), seed); err != nil {
			t.Fatalf("seed spoke bead: %v", err)
		}

		hubState := newMutationState("hub-node", hubA)
		spokeState := newMutationState("spoke-node", spokeA)

		fed := &mutationFederation{
			spokes: []federation.SpokeRecord{
				{
					NodeID:       "spoke-node",
					Name:         "spoke-a",
					ProjectIDs:   []string{"proj-a"},
					Capabilities: []string{"read", "write"},
					Status:       federation.StatusActive,
				},
			},
			runtimes: map[string]*mutationSpokeRuntime{
				"spoke-node": newMutationSpokeRuntime("spoke-node", spokeState, spokeA),
			},
		}

		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubA.root,
			Federation: fed,
			NodeID:     "hub-node",
		}}

		updated, err := resolver.BeadUpdate(WithWorkingDir(context.Background(), hubA.root), seed.ID, BeadUpdateInput{
			Title:       beadFedStrPtr("after update"),
			Description: beadFedStrPtr("edited through hub"),
		})
		if err != nil {
			t.Fatalf("BeadUpdate: %v", err)
		}
		if updated.Title != "after update" {
			t.Fatalf("title = %q, want after update", updated.Title)
		}
		if beadFedDeref(updated.Description) != "edited through hub" {
			t.Fatalf("description = %q", beadFedDeref(updated.Description))
		}

		if fed.forwardCalls != 1 {
			t.Fatalf("ForwardMutation calls = %d, want 1", fed.forwardCalls)
		}
		if !strings.Contains(string(fed.lastForwarded.Body), "beadUpdate") {
			t.Fatalf("forward body missing beadUpdate: %s", fed.lastForwarded.Body)
		}

		if beads := readAllBeads(t, hubA.store); len(beads) != 0 {
			t.Fatalf("hub-local store must stay empty after update, got %d", len(beads))
		}
		spokeBeads := readAllBeads(t, spokeA.store)
		if len(spokeBeads) != 1 || spokeBeads[0].Title != "after update" {
			t.Fatalf("spoke beads after update: %+v", spokeBeads)
		}
	})

	t.Run("offline refuses without hub-local write", func(t *testing.T) {
		hubA := newMutationProjectFixture(t, "proj-a")
		spokeA := newMutationProjectFixture(t, "proj-a")
		hubState := newMutationState("hub-node", hubA)
		spokeState := newMutationState("spoke-node", spokeA)

		fed := &mutationFederation{
			spokes: []federation.SpokeRecord{
				{
					NodeID:       "spoke-node",
					Name:         "spoke-a",
					ProjectIDs:   []string{"proj-a"},
					Capabilities: []string{"read", "write"},
					Status:       federation.StatusOffline,
				},
			},
			runtimes: map[string]*mutationSpokeRuntime{
				"spoke-node": newMutationSpokeRuntime("spoke-node", spokeState, spokeA),
			},
		}
		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubA.root,
			Federation: fed,
			NodeID:     "hub-node",
		}}

		_, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{Title: "offline"})
		if !errors.Is(err, federation.ErrForwardMutationOffline) {
			t.Fatalf("offline error = %v, want offline", err)
		}
		if beads := readAllBeads(t, hubA.store); len(beads) != 0 {
			t.Fatalf("hub-local phantom on offline: %d beads", len(beads))
		}
		if beads := readAllBeads(t, spokeA.store); len(beads) != 0 {
			t.Fatalf("spoke must not write on offline: %d beads", len(beads))
		}
	})

	t.Run("missing owner refuses without hub-local write", func(t *testing.T) {
		hubA := newMutationProjectFixture(t, "proj-a")
		hubState := newMutationState("hub-node", hubA)
		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubA.root,
			Federation: &mutationFederation{},
			NodeID:     "hub-node",
		}}

		_, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{Title: "missing owner"})
		if !errors.Is(err, federation.ErrForwardMutationMissingOwner) {
			t.Fatalf("missing-owner error = %v, want missing-owner", err)
		}
		if beads := readAllBeads(t, hubA.store); len(beads) != 0 {
			t.Fatalf("hub-local phantom on missing-owner: %d beads", len(beads))
		}
	})
}
