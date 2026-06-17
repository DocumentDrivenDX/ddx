package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

type lifecycleStateProvider struct {
	node             ddxgraphql.NodeStateSnapshot
	projectSnapshots map[string]*ddxgraphql.Project
	beadSnapshots    map[string]ddxgraphql.BeadSnapshot
	store            *bead.Store
	storeProjectID   string
}

func newLifecycleStateProvider(nodeID, nodeName string) *lifecycleStateProvider {
	now := time.Now().UTC()
	return &lifecycleStateProvider{
		node: ddxgraphql.NodeStateSnapshot{
			ID:        nodeID,
			Name:      nodeName,
			StartedAt: now.Add(-time.Hour),
			LastSeen:  now,
		},
		projectSnapshots: make(map[string]*ddxgraphql.Project),
		beadSnapshots:    make(map[string]ddxgraphql.BeadSnapshot),
	}
}

func (p *lifecycleStateProvider) addProject(id, name, path string) {
	if p.projectSnapshots == nil {
		p.projectSnapshots = make(map[string]*ddxgraphql.Project)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p.projectSnapshots[id] = &ddxgraphql.Project{
		ID:           id,
		Name:         name,
		Path:         path,
		RegisteredAt: now,
		LastSeen:     now,
	}
}

func (p *lifecycleStateProvider) addBeadSnapshot(snap ddxgraphql.BeadSnapshot) {
	if p.beadSnapshots == nil {
		p.beadSnapshots = make(map[string]ddxgraphql.BeadSnapshot)
	}
	p.beadSnapshots[snap.ID] = snap
}

func (p *lifecycleStateProvider) snapshotFromBead(b *bead.Bead) ddxgraphql.BeadSnapshot {
	return ddxgraphql.BeadSnapshot{
		ProjectID:   p.storeProjectID,
		ID:          b.ID,
		Title:       b.Title,
		Status:      b.Status,
		Priority:    b.Priority,
		IssueType:   b.IssueType,
		Owner:       b.Owner,
		CreatedAt:   b.CreatedAt,
		CreatedBy:   b.CreatedBy,
		UpdatedAt:   b.UpdatedAt,
		Labels:      append([]string(nil), b.Labels...),
		Parent:      b.Parent,
		Description: b.Description,
		Acceptance:  b.Acceptance,
		Notes:       b.Notes,
	}
}

func (p *lifecycleStateProvider) beadSnapshotsFromStore(status, label, projectID, search string) []ddxgraphql.BeadSnapshot {
	if p.store == nil {
		return nil
	}
	beads, err := p.store.ReadAll(context.Background())
	if err != nil {
		return nil
	}
	out := make([]ddxgraphql.BeadSnapshot, 0, len(beads))
	for i := range beads {
		snap := p.snapshotFromBead(&beads[i])
		if status != "" && snap.Status != status {
			continue
		}
		if label != "" {
			matched := false
			for _, l := range snap.Labels {
				if l == label {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if projectID != "" && snap.ProjectID != projectID {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				snap.ID, snap.Title, snap.Description, snap.Acceptance, snap.Notes,
			}, " "))
			if !strings.Contains(haystack, strings.ToLower(search)) {
				continue
			}
		}
		out = append(out, snap)
	}
	return out
}

func (p *lifecycleStateProvider) GetNodeSnapshot() ddxgraphql.NodeStateSnapshot {
	return p.node
}

func (p *lifecycleStateProvider) GetProjectSnapshots(bool) []*ddxgraphql.Project {
	out := make([]*ddxgraphql.Project, 0, len(p.projectSnapshots))
	for _, proj := range p.projectSnapshots {
		out = append(out, proj)
	}
	return out
}

func (p *lifecycleStateProvider) GetProjectSnapshotByID(id string) (*ddxgraphql.Project, bool) {
	proj, ok := p.projectSnapshots[id]
	return proj, ok
}

func (p *lifecycleStateProvider) GetBeadSnapshots(status, label, projectID, search string) []ddxgraphql.BeadSnapshot {
	if p.store != nil {
		return p.beadSnapshotsFromStore(status, label, projectID, search)
	}
	out := make([]ddxgraphql.BeadSnapshot, 0, len(p.beadSnapshots))
	for _, snap := range p.beadSnapshots {
		if status != "" && snap.Status != status {
			continue
		}
		if label != "" {
			matched := false
			for _, l := range snap.Labels {
				if l == label {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if projectID != "" && snap.ProjectID != projectID {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				snap.ID, snap.Title, snap.Description, snap.Acceptance, snap.Notes,
			}, " "))
			if !strings.Contains(haystack, strings.ToLower(search)) {
				continue
			}
		}
		out = append(out, snap)
	}
	return out
}

func (p *lifecycleStateProvider) GetBeadSnapshotsForProject(projectID, status, label, search string) []ddxgraphql.BeadSnapshot {
	return p.GetBeadSnapshots(status, label, projectID, search)
}

func (p *lifecycleStateProvider) GetBeadSnapshot(id string) (*ddxgraphql.BeadSnapshot, bool) {
	if p.store != nil {
		if b, err := p.store.Get(context.Background(), id); err == nil && b != nil {
			snap := p.snapshotFromBead(b)
			return &snap, true
		}
	}
	snap, ok := p.beadSnapshots[id]
	if !ok {
		return nil, false
	}
	copy := snap
	return &copy, true
}

func (p *lifecycleStateProvider) GetWorkersGraphQL(string) []*ddxgraphql.Worker { return nil }
func (p *lifecycleStateProvider) GetWorkerGraphQL(string) (*ddxgraphql.Worker, bool) {
	return nil, false
}
func (p *lifecycleStateProvider) GetWorkerLogGraphQL(string) *ddxgraphql.WorkerLog { return nil }
func (p *lifecycleStateProvider) GetWorkerProgressGraphQL(string) []*ddxgraphql.PhaseTransition {
	return nil
}
func (p *lifecycleStateProvider) GetWorkerPromptGraphQL(string) string { return "" }
func (p *lifecycleStateProvider) GetAgentSessionsGraphQL(string, *time.Time, *time.Time) []*ddxgraphql.AgentSession {
	return nil
}
func (p *lifecycleStateProvider) GetAgentSessionGraphQL(string) (*ddxgraphql.AgentSession, bool) {
	return nil, false
}
func (p *lifecycleStateProvider) GetSessionsCostSummaryGraphQL(string, *time.Time, *time.Time) *ddxgraphql.SessionsCostSummary {
	return &ddxgraphql.SessionsCostSummary{}
}
func (p *lifecycleStateProvider) GetExecDefinitionsGraphQL(string) []*ddxgraphql.ExecutionDefinition {
	return nil
}
func (p *lifecycleStateProvider) GetExecDefinitionGraphQL(string) (*ddxgraphql.ExecutionDefinition, bool) {
	return nil, false
}
func (p *lifecycleStateProvider) GetExecRunsGraphQL(string, string) []*ddxgraphql.ExecutionRun {
	return nil
}
func (p *lifecycleStateProvider) GetExecRunGraphQL(string) (*ddxgraphql.ExecutionRun, bool) {
	return nil, false
}
func (p *lifecycleStateProvider) GetExecRunLogGraphQL(string) *ddxgraphql.ExecutionRunLog { return nil }
func (p *lifecycleStateProvider) GetCoordinatorsGraphQL() []*ddxgraphql.CoordinatorMetricsEntry {
	return nil
}
func (p *lifecycleStateProvider) GetCoordinatorMetricsByProjectGraphQL(string) *ddxgraphql.CoordinatorMetrics {
	return nil
}

type lifecycleGraphQLResponse struct {
	BeadApprove *ddxgraphql.Bead `json:"beadApprove,omitempty"`
	BeadCancel  *ddxgraphql.Bead `json:"beadCancel,omitempty"`
	BeadBlock   *ddxgraphql.Bead `json:"beadBlock,omitempty"`
	BeadReopen  *ddxgraphql.Bead `json:"beadReopen,omitempty"`
	Federated   []struct {
		NodeID string           `json:"nodeId"`
		Bead   *ddxgraphql.Bead `json:"bead"`
	} `json:"federatedBeads,omitempty"`
}

func gqlPostWithHeaders(t *testing.T, h http.Handler, query string, headers map[string]string) map[string]json.RawMessage {
	t.Helper()
	rawBody, _ := json.Marshal(map[string]string{"query": query})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

func mustGraphQLErrorCode(t *testing.T, resp map[string]json.RawMessage) string {
	t.Helper()
	raw, ok := resp["errors"]
	if !ok {
		t.Fatalf("expected GraphQL errors, got %v", resp)
	}
	var parsed []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse errors: %v", err)
	}
	if len(parsed) == 0 {
		t.Fatal("expected at least one GraphQL error")
	}
	if parsed[0].Extensions == nil {
		t.Fatalf("expected extensions on error: %+v", parsed[0])
	}
	code, _ := parsed[0].Extensions["code"].(string)
	return code
}

func newFederatedLifecycleHandler(state ddxgraphql.StateProvider, workDir string, fed ddxgraphql.FederationProvider, nodeID string) http.Handler {
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:      state,
			WorkingDir: workDir,
			Actions:    testActionDispatcher{},
			Federation: fed,
			NodeID:     nodeID,
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gqlSrv.AddTransport(transport.POST{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(ddxgraphql.WithHTTPRequest(r.Context(), r))
		gqlSrv.ServeHTTP(w, r)
	})
}

func createSpokeLifecycleServer(t *testing.T, state *lifecycleStateProvider, workDir string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newFederatedLifecycleHandler(state, workDir, nil, "spoke-node"))
	t.Cleanup(srv.Close)
	return srv
}

func TestFederatedBeadLifecycle_ForwardsAuditMetadata(t *testing.T) {
	hubWorkDir, _ := setupIntegrationDir(t)
	spokeWorkDir, spokeStore := setupIntegrationDir(t)

	hubState := newLifecycleStateProvider("hub-node", "hub-node")
	spokeState := newLifecycleStateProvider("spoke-node", "spoke-node")
	spokeProjectID := "proj-spoke-owner"
	spokeState.store = spokeStore
	spokeState.storeProjectID = spokeProjectID
	spokeState.addProject(spokeProjectID, "spoke-project", spokeWorkDir)

	cases := []struct {
		name    string
		query   string
		bead    *bead.Bead
		initial string
		expect  string
		summary string
		want    string
	}{
		{
			name:    "approve",
			query:   `mutation { beadApprove(id: "%s", note: "approved after federation") { id status projectID } }`,
			bead:    &bead.Bead{Title: "Approve me", Status: bead.StatusProposed},
			initial: bead.StatusProposed,
			expect:  bead.StatusOpen,
			summary: "approve",
			want:    "note=approved after federation",
		},
		{
			name:    "cancel",
			query:   `mutation { beadCancel(id: "%s", reason: "no longer needed") { id status projectID } }`,
			bead:    &bead.Bead{Title: "Cancel me", Status: bead.StatusOpen},
			initial: bead.StatusOpen,
			expect:  bead.StatusCancelled,
			summary: "cancel",
			want:    "reason=no longer needed",
		},
		{
			name:    "block",
			query:   `mutation { beadBlock(id: "%s", externalBlockerReason: "upstream blocked") { id status projectID } }`,
			bead:    &bead.Bead{Title: "Block me", Status: bead.StatusOpen},
			initial: bead.StatusOpen,
			expect:  bead.StatusBlocked,
			summary: "block",
			want:    "external_blocker_reason=upstream blocked",
		},
		{
			name:    "reopen",
			query:   `mutation { beadReopen(id: "%s") { id status projectID } }`,
			bead:    &bead.Bead{Title: "Reopen me", Status: bead.StatusOpen},
			initial: bead.StatusClosed,
			expect:  bead.StatusOpen,
			summary: "reopen",
			want:    "reason=graphql bead reopen",
		},
	}

	for _, tc := range cases {
		if err := spokeStore.Create(context.Background(), tc.bead); err != nil {
			t.Fatalf("%s create spoke bead: %v", tc.name, err)
		}
		if tc.initial == bead.StatusClosed {
			if err := spokeStore.SetLifecycleStatus(tc.bead.ID, bead.StatusClosed, bead.LifecycleTransitionOptions{ManualClose: true}); err != nil {
				t.Fatalf("%s close spoke bead: %v", tc.name, err)
			}
		}
		hubState.addBeadSnapshot(ddxgraphql.BeadSnapshot{
			ProjectID: spokeProjectID,
			ID:        tc.bead.ID,
			Title:     tc.bead.Title,
			Status:    tc.initial,
			CreatedAt: tc.bead.CreatedAt,
			UpdatedAt: tc.bead.UpdatedAt,
		})
	}

	spokeSrv := createSpokeLifecycleServer(t, spokeState, spokeWorkDir)

	now := time.Now().UTC()
	fed := &stubFederation{
		client:              federation.NewFanOutClient(),
		coordinatorIdentity: "hub-node",
		spokes: []federation.SpokeRecord{
			{
				NodeID:        "spoke-node",
				Name:          "spoke-node",
				URL:           spokeSrv.URL,
				DDxVersion:    "0.1.0",
				SchemaVersion: federation.CurrentSchemaVersion,
				Capabilities:  []string{"read", "write"},
				ProjectIDs:    []string{spokeProjectID},
				RegisteredAt:  now,
				LastHeartbeat: now,
				Status:        federation.StatusActive,
			},
		},
	}
	fed.client.HubDDxVersion = "0.1.0"
	fed.client.HubSchemaVersion = federation.CurrentSchemaVersion
	fed.client.PerNodeTimeout = 2 * time.Second

	hubHandler := newFederatedLifecycleHandler(hubState, hubWorkDir, fed, "hub-node")
	commonHeaders := map[string]string{
		"X-Tailscale-Node":      "origin-node-123",
		"X-DDx-Request-ID":      "req-federated",
		"X-DDx-Idempotency-Key": "idem-federated",
	}

	for _, tc := range cases {
		resp := gqlPostWithHeaders(t, hubHandler, fmt.Sprintf(tc.query, tc.bead.ID), commonHeaders)
		if _, ok := resp["errors"]; ok {
			t.Fatalf("%s mutation returned errors: %s", tc.name, resp["errors"])
		}
		var parsed lifecycleGraphQLResponse
		if err := json.Unmarshal(resp["data"], &parsed); err != nil {
			t.Fatalf("%s parse data: %v", tc.name, err)
		}

		var got *ddxgraphql.Bead
		switch tc.name {
		case "approve":
			got = parsed.BeadApprove
		case "cancel":
			got = parsed.BeadCancel
		case "block":
			got = parsed.BeadBlock
		case "reopen":
			got = parsed.BeadReopen
		}
		if got == nil {
			t.Fatalf("%s mutation returned nil bead", tc.name)
		}
		if got.Status != tc.expect {
			t.Fatalf("%s status = %s, want %s", tc.name, got.Status, tc.expect)
		}

		b, err := spokeStore.Get(context.Background(), tc.bead.ID)
		if err != nil {
			t.Fatalf("%s spoke get: %v", tc.name, err)
		}
		if b.Status != tc.expect {
			t.Fatalf("%s spoke status = %s, want %s", tc.name, b.Status, tc.expect)
		}

		events, err := spokeStore.Events(tc.bead.ID)
		if err != nil {
			t.Fatalf("%s spoke events: %v", tc.name, err)
		}
		found := false
		for _, e := range events {
			if e.Kind == "human-resolution" && e.Summary == tc.summary {
				found = true
				for _, want := range []string{
					"origin_identity=origin-node-123",
					"coordinator_identity=hub-node",
					"forwarding_path=hub-node -> spoke-node",
					"request_id=req-federated",
					"idempotency_key=idem-federated",
					tc.want,
				} {
					if !strings.Contains(e.Body, want) {
						t.Fatalf("%s event body missing %q:\n%s", tc.name, want, e.Body)
					}
				}
			}
		}
		if !found {
			t.Fatalf("%s expected human-resolution event summary=%s", tc.name, tc.summary)
		}
	}

	resp := gqlPostWithHeaders(t, hubHandler, `{ federatedBeads { nodeId bead { id status projectID } } }`, commonHeaders)
	if _, ok := resp["errors"]; ok {
		t.Fatalf("federatedBeads returned errors: %s", resp["errors"])
	}
	var federated struct {
		FederatedBeads []struct {
			NodeID string           `json:"nodeId"`
			Bead   *ddxgraphql.Bead `json:"bead"`
		} `json:"federatedBeads"`
	}
	if err := json.Unmarshal(resp["data"], &federated); err != nil {
		t.Fatalf("parse federated beads: %v", err)
	}
	gotByID := make(map[string]string, len(federated.FederatedBeads))
	for _, row := range federated.FederatedBeads {
		if row.Bead != nil {
			gotByID[row.Bead.ID] = row.Bead.Status
		}
	}
	for _, tc := range cases {
		if gotByID[tc.bead.ID] != tc.expect {
			t.Fatalf("federatedBeads status for %s = %s, want %s", tc.name, gotByID[tc.bead.ID], tc.expect)
		}
	}
}

func TestBeadLifecycle_RequiredReasonTypedErrors(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	localState := newLifecycleStateProvider("hub-node", "hub-node")
	localState.addProject("proj-local", "local-project", workDir)

	b := &bead.Bead{Title: "Needs validation", Status: bead.StatusProposed}
	if err := store.Create(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	localState.addBeadSnapshot(ddxgraphql.BeadSnapshot{
		ProjectID: "proj-local",
		ID:        b.ID,
		Title:     b.Title,
		Status:    b.Status,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	})
	localHandler := newFederatedLifecycleHandler(localState, workDir, nil, "hub-node")

	for _, tc := range []struct {
		name  string
		query string
		code  string
	}{
		{
			name:  "approve missing note",
			query: `mutation { beadApprove(id: "` + b.ID + `", note: "") { id } }`,
			code:  "BEAD_APPROVE_NOTE_REQUIRED",
		},
		{
			name:  "cancel missing reason",
			query: `mutation { beadCancel(id: "` + b.ID + `", reason: "") { id } }`,
			code:  "BEAD_CANCEL_REASON_REQUIRED",
		},
		{
			name:  "block missing reason",
			query: `mutation { beadBlock(id: "` + b.ID + `", externalBlockerReason: "") { id } }`,
			code:  "BEAD_BLOCK_EXTERNAL_BLOCKER_REASON_REQUIRED",
		},
	} {
		resp := gqlPostWithHeaders(t, localHandler, tc.query, map[string]string{"X-Tailscale-Node": "origin-node-123"})
		if got := mustGraphQLErrorCode(t, resp); got != tc.code {
			t.Fatalf("%s code = %s, want %s", tc.name, got, tc.code)
		}
	}

	remoteState := newLifecycleStateProvider("hub-node", "hub-node")
	remoteState.addBeadSnapshot(ddxgraphql.BeadSnapshot{
		ProjectID: "proj-remote",
		ID:        "bead-remote",
		Title:     "Remote bead",
		Status:    bead.StatusProposed,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	remoteState.addProject("proj-remote", "remote-project", "")

	remoteFed := &stubFederation{
		client:              federation.NewFanOutClient(),
		coordinatorIdentity: "hub-node",
		spokes: []federation.SpokeRecord{
			{
				NodeID:        "node-remote",
				Name:          "node-remote",
				URL:           "http://127.0.0.1:0",
				DDxVersion:    "0.1.0",
				SchemaVersion: federation.CurrentSchemaVersion,
				ProjectIDs:    []string{"proj-remote"},
				RegisteredAt:  time.Now().UTC(),
				LastHeartbeat: time.Now().UTC(),
				Status:        federation.StatusOffline,
			},
			{
				NodeID:        "node-readonly",
				Name:          "node-readonly",
				URL:           "http://127.0.0.1:0",
				DDxVersion:    "0.1.0",
				SchemaVersion: federation.CurrentSchemaVersion,
				ProjectIDs:    []string{"proj-readonly"},
				Capabilities:  []string{"read"},
				RegisteredAt:  time.Now().UTC(),
				LastHeartbeat: time.Now().UTC(),
				Status:        federation.StatusActive,
			},
		},
	}
	remoteHandler := newFederatedLifecycleHandler(remoteState, workDir, remoteFed, "hub-node")

	offlineResp := gqlPostWithHeaders(t, remoteHandler, `mutation { beadApprove(id: "bead-remote", note: "ok") { id } }`, map[string]string{"X-Tailscale-Node": "origin-node-123"})
	if got := mustGraphQLErrorCode(t, offlineResp); got != "BEAD_LIFECYCLE_TARGET_OFFLINE" {
		t.Fatalf("offline code = %s, want BEAD_LIFECYCLE_TARGET_OFFLINE", got)
	}

	remoteState.addBeadSnapshot(ddxgraphql.BeadSnapshot{
		ProjectID: "proj-readonly",
		ID:        "bead-readonly",
		Title:     "Readonly bead",
		Status:    bead.StatusProposed,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	readOnlyResp := gqlPostWithHeaders(t, remoteHandler, `mutation { beadApprove(id: "bead-readonly", note: "ok") { id } }`, map[string]string{"X-Tailscale-Node": "origin-node-123"})
	if got := mustGraphQLErrorCode(t, readOnlyResp); got != "BEAD_LIFECYCLE_TARGET_READ_ONLY" {
		t.Fatalf("read-only code = %s, want BEAD_LIFECYCLE_TARGET_READ_ONLY", got)
	}

	notOwnedState := newLifecycleStateProvider("hub-node", "hub-node")
	notOwnedState.addBeadSnapshot(ddxgraphql.BeadSnapshot{
		ProjectID: "proj-unowned",
		ID:        "bead-unowned",
		Title:     "Unowned bead",
		Status:    bead.StatusProposed,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	notOwnedState.addProject("proj-unowned", "unowned-project", "")
	notOwnedHandler := newFederatedLifecycleHandler(notOwnedState, workDir, &stubFederation{
		client:              federation.NewFanOutClient(),
		coordinatorIdentity: "hub-node",
		spokes: []federation.SpokeRecord{
			{
				NodeID:        "node-other",
				Name:          "node-other",
				URL:           "http://127.0.0.1:0",
				DDxVersion:    "0.1.0",
				SchemaVersion: federation.CurrentSchemaVersion,
				ProjectIDs:    []string{"proj-some-other"},
				RegisteredAt:  time.Now().UTC(),
				LastHeartbeat: time.Now().UTC(),
				Status:        federation.StatusActive,
			},
		},
	}, "hub-node")
	notOwnedResp := gqlPostWithHeaders(t, notOwnedHandler, `mutation { beadApprove(id: "bead-unowned", note: "ok") { id } }`, map[string]string{"X-Tailscale-Node": "origin-node-123"})
	if got := mustGraphQLErrorCode(t, notOwnedResp); got != "BEAD_LIFECYCLE_TARGET_NOT_OWNED" {
		t.Fatalf("not-owned code = %s, want BEAD_LIFECYCLE_TARGET_NOT_OWNED", got)
	}
}
