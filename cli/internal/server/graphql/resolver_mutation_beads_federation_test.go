package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/stretchr/testify/require"
)

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
func (p *mutationTestStateProvider) GetCoordinatorsGraphQL() []*CoordinatorMetricsEntry { return nil }

type mutationProjectFixture struct {
	root    string
	store   *bead.Store
	project Project
}

func newMutationProjectFixture(t *testing.T, projectID string) mutationProjectFixture {
	t.Helper()

	root := t.TempDir()
	out, err := exec.Command("git", "init", root).CombinedOutput()
	require.NoError(t, err, "%s", out)

	ddxDir := filepath.Join(root, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\nlibrary:\n  path: .ddx/plugins/ddx\n"
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))

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
	require.NoError(t, err)
	return beads
}

func beadsFromStore(store *bead.Store) ([]bead.Bead, error) {
	return store.ReadAll(context.Background())
}

type mutationFederation struct {
	spokes   []federation.SpokeRecord
	runtimes map[string]*mutationSpokeRuntime
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

func (m *mutationFederation) spoke(nodeID string) *mutationSpokeRuntime {
	if m == nil {
		return nil
	}
	return m.runtimes[nodeID]
}

func (m *mutationFederation) FanOut(ctx context.Context, req *federation.FanOutRequest) (*federation.FanOutResult, error) {
	out := &federation.FanOutResult{
		Responses:     map[string]json.RawMessage{},
		Errors:        map[string]error{},
		Skipped:       map[string]federation.SkipReason{},
		StatusUpdates: map[string]federation.SpokeStatus{},
		Nodes:         []federation.NodeResult{},
	}
	if m == nil {
		return out, nil
	}

	projectID := ""
	if req != nil {
		if raw, ok := req.Variables["projectID"]; ok {
			switch v := raw.(type) {
			case string:
				projectID = v
			case json.RawMessage:
				_ = json.Unmarshal(v, &projectID)
			}
		}
	}

	registry := federation.NewRegistry()
	registry.Spokes = append(registry.Spokes, m.spokes...)
	if projectID != "" {
		owner, err := federation.RouteMutationToProjectOwner(registry, projectID)
		if err != nil {
			return out, nil
		}
		if runtime := m.spoke(owner.NodeID); runtime != nil {
			body, err := runtime.federatedBeadsResponse(projectID)
			if err != nil {
				return nil, err
			}
			out.Responses[owner.NodeID] = body
			out.Nodes = append(out.Nodes, federation.NodeResult{
				NodeID:   owner.NodeID,
				URL:      owner.URL,
				Outcome:  federation.OutcomeOK,
				Response: body,
			})
		}
		return out, nil
	}

	for _, spoke := range m.spokes {
		runtime := m.spoke(spoke.NodeID)
		if runtime == nil {
			continue
		}
		body, err := runtime.federatedBeadsResponse(spoke.ProjectIDs[0])
		if err != nil {
			return nil, err
		}
		out.Responses[spoke.NodeID] = body
		out.Nodes = append(out.Nodes, federation.NodeResult{
			NodeID:   spoke.NodeID,
			URL:      spoke.URL,
			Outcome:  federation.OutcomeOK,
			Response: body,
		})
	}
	return out, nil
}

func (m *mutationFederation) ForwardMutation(ctx context.Context, req *federation.ForwardMutationRequest) (*federation.ForwardMutationResponse, error) {
	if m == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	registry := federation.NewRegistry()
	registry.Spokes = append(registry.Spokes, m.spokes...)

	owner, err := federation.RouteMutationToProjectOwner(registry, req.TargetProjectID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "multiple registered owners"):
			return nil, federation.ErrForwardMutationBroadcastLike
		default:
			return nil, federation.ErrForwardMutationMissingOwner
		}
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
	if !hasWriteCapability(owner.Capabilities) {
		return nil, federation.ErrForwardMutationReadOnly
	}

	runtime := m.spoke(owner.NodeID)
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

	var (
		body []byte
	)
	switch {
	case strings.Contains(envelope.Query, "beadCreate"):
		var input BeadInput
		if raw, ok := envelope.Variables["input"]; ok {
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
		}
		bead, err := runtime.resolver.BeadCreate(WithWorkingDir(context.Background(), runtime.fixture.root), input)
		if err != nil {
			return nil, err
		}
		body, err = json.Marshal(map[string]any{
			"data": map[string]any{
				"beadCreate": bead,
			},
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
		bead, err := runtime.resolver.BeadUpdate(WithWorkingDir(context.Background(), runtime.fixture.root), id, input)
		if err != nil {
			return nil, err
		}
		body, err = json.Marshal(map[string]any{
			"data": map[string]any{
				"beadUpdate": bead,
			},
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

func (r *mutationSpokeRuntime) federatedBeadsResponse(projectID string) ([]byte, error) {
	beads, err := beadsFromStore(r.fixture.store)
	if err != nil {
		return nil, err
	}
	edges := make([]map[string]any, 0, len(beads))
	for i := range beads {
		snap := BeadSnapshot{
			ProjectID:   projectID,
			ID:          beads[i].ID,
			Title:       beads[i].Title,
			Status:      beads[i].Status,
			Priority:    beads[i].Priority,
			IssueType:   beads[i].IssueType,
			Owner:       beads[i].Owner,
			CreatedAt:   beads[i].CreatedAt,
			CreatedBy:   beads[i].CreatedBy,
			UpdatedAt:   beads[i].UpdatedAt,
			Labels:      append([]string(nil), beads[i].Labels...),
			Parent:      beads[i].Parent,
			Description: beads[i].Description,
			Acceptance:  beads[i].Acceptance,
			Notes:       beads[i].Notes,
		}
		for _, dep := range beads[i].Dependencies {
			snap.Dependencies = append(snap.Dependencies, BeadDependencySnapshot{
				IssueID:     dep.IssueID,
				DependsOnID: dep.DependsOnID,
				Type:        dep.Type,
				CreatedAt:   dep.CreatedAt,
				CreatedBy:   dep.CreatedBy,
				Metadata:    dep.Metadata,
			})
		}
		edges = append(edges, map[string]any{"node": beadFromSnapshot(snap)})
	}
	return json.Marshal(map[string]any{
		"data": map[string]any{
			"beads": map[string]any{
				"edges": edges,
			},
		},
	})
}

func hasWriteCapability(caps []string) bool {
	for _, cap := range caps {
		if strings.TrimSpace(cap) == "write" {
			return true
		}
	}
	return false
}

func TestFederatedBeadCreate_ForwardsToOwner(t *testing.T) {
	hubA := newMutationProjectFixture(t, "proj-a")
	hubB := newMutationProjectFixture(t, "proj-b")
	spokeA := newMutationProjectFixture(t, "proj-a")

	hubState := newMutationState("hub-node", hubA, hubB)
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
	projectID, ok := resolver.projectIDForWorkingDir(hubA.root)
	require.True(t, ok)
	require.Equal(t, "proj-a", projectID)
	routedProjectID, owner, routeErr := resolver.beadMutationOwner(hubA.root)
	require.NoError(t, routeErr)
	require.Equal(t, "proj-a", routedProjectID)
	require.NotNil(t, owner)
	require.Equal(t, "spoke-node", owner.NodeID)

	created, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{
		Title:       "federated create",
		IssueType:   sPtr("task"),
		Labels:      []string{"federated"},
		Description: sPtr("forward this bead"),
	})
	require.NoError(t, err)
	require.Equal(t, "federated create", created.Title)
	require.Equal(t, "task", created.IssueType)
	require.ElementsMatch(t, []string{"federated"}, created.Labels)

	require.Empty(t, readAllBeads(t, hubA.store), "hub-local store must stay empty")
	spokeBeads := readAllBeads(t, spokeA.store)
	require.Len(t, spokeBeads, 1)
	require.Equal(t, "federated create", spokeBeads[0].Title)
}

func TestFederatedBeadUpdate_ForwardsToOwner(t *testing.T) {
	hubA := newMutationProjectFixture(t, "proj-a")
	hubB := newMutationProjectFixture(t, "proj-b")
	spokeA := newMutationProjectFixture(t, "proj-a")

	seed := &bead.Bead{Title: "before update"}
	require.NoError(t, spokeA.store.Create(context.Background(), seed))

	hubState := newMutationState("hub-node", hubA, hubB)
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
	projectID, ok := resolver.projectIDForWorkingDir(hubA.root)
	require.True(t, ok)
	require.Equal(t, "proj-a", projectID)
	routedProjectID, owner, routeErr := resolver.beadMutationOwner(hubA.root)
	require.NoError(t, routeErr)
	require.Equal(t, "proj-a", routedProjectID)
	require.NotNil(t, owner)
	require.Equal(t, "spoke-node", owner.NodeID)

	updated, err := resolver.BeadUpdate(WithWorkingDir(context.Background(), hubA.root), seed.ID, BeadUpdateInput{
		Title:       sPtr("after update"),
		Description: sPtr("edited through hub"),
	})
	require.NoError(t, err)
	require.Equal(t, "after update", updated.Title)
	require.Equal(t, "edited through hub", derefString(updated.Description))

	q := &queryResolver{resolver.Resolver}
	rows, err := q.FederatedBeads(context.Background(), nil, nil, strPtr("proj-a"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "after update", rows[0].Bead.Title)
	require.Equal(t, "proj-a", derefString(rows[0].Bead.ProjectID))
}

func TestBeadMutation_ProjectIsolation(t *testing.T) {
	hubA := newMutationProjectFixture(t, "proj-a")
	hubB := newMutationProjectFixture(t, "proj-b")
	spokeA := newMutationProjectFixture(t, "proj-a")

	seed := &bead.Bead{Title: "project A seed"}
	require.NoError(t, spokeA.store.Create(context.Background(), seed))

	hubState := newMutationState("hub-node", hubA, hubB)
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
	projectID, ok := resolver.projectIDForWorkingDir(hubA.root)
	require.True(t, ok)
	require.Equal(t, "proj-a", projectID)
	routedProjectID, owner, routeErr := resolver.beadMutationOwner(hubA.root)
	require.NoError(t, routeErr)
	require.Equal(t, "proj-a", routedProjectID)
	require.NotNil(t, owner)
	require.Equal(t, "spoke-node", owner.NodeID)

	created, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{
		Title: "project A create",
	})
	require.NoError(t, err)
	require.Equal(t, "project A create", created.Title)
	require.Empty(t, readAllBeads(t, hubA.store))
	require.Empty(t, readAllBeads(t, hubB.store))

	updated, err := resolver.BeadUpdate(WithWorkingDir(context.Background(), hubA.root), created.ID, BeadUpdateInput{
		Notes: sPtr("project A update"),
	})
	require.NoError(t, err)
	require.Equal(t, "project A update", derefString(updated.Notes))
	require.Empty(t, readAllBeads(t, hubA.store))
	require.Empty(t, readAllBeads(t, hubB.store))
	spokeBeads := readAllBeads(t, spokeA.store)
	require.Len(t, spokeBeads, 2)
}

func TestBeadMutation_ForwardingTypedErrors(t *testing.T) {
	t.Run("missing owner", func(t *testing.T) {
		hubA := newMutationProjectFixture(t, "proj-a")
		hubState := newMutationState("hub-node", hubA)
		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubA.root,
			Federation: &mutationFederation{},
			NodeID:     "hub-node",
		}}

		_, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{Title: "missing owner"})
		require.ErrorIs(t, err, federation.ErrForwardMutationMissingOwner)
	})

	t.Run("stale", func(t *testing.T) {
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
					Status:       federation.StatusStale,
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

		_, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{Title: "stale"})
		require.ErrorIs(t, err, federation.ErrForwardMutationStale)
	})

	t.Run("offline", func(t *testing.T) {
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
		require.ErrorIs(t, err, federation.ErrForwardMutationOffline)
	})

	t.Run("read only", func(t *testing.T) {
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
					Capabilities: []string{"read"},
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

		_, err := resolver.BeadCreate(WithWorkingDir(context.Background(), hubA.root), BeadInput{Title: "readonly"})
		require.ErrorIs(t, err, federation.ErrForwardMutationReadOnly)
	})
}

func sPtr(s string) *string { return &s }

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
