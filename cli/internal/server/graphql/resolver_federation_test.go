package graphql_test

// Federation resolver integration tests (B14.6b). Exercises the four federated
// queries (federationNodes, federatedBeads, federatedRuns, federatedProjects)
// against two mock spoke servers. Verifies row shape, routing metadata, the
// partial-result rendering when one spoke is offline, and the version-skew
// rendering when one spoke is filtered by the handshake.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// stubFederation is a minimal FederationProvider backed by a fixed list of
// spokes and a real (in-process) FanOutClient pointed at httptest spoke
// servers.
type stubFederation struct {
	spokes []federation.SpokeRecord
	client *federation.FanOutClient
}

func (s *stubFederation) Spokes() []federation.SpokeRecord {
	out := make([]federation.SpokeRecord, len(s.spokes))
	copy(out, s.spokes)
	return out
}

func (s *stubFederation) FanOut(ctx context.Context, req *federation.FanOutRequest) (*federation.FanOutResult, error) {
	return s.client.Execute(ctx, s.spokes, req)
}

// newSpokeServer spins up an httptest server whose /graphql endpoint returns
// the canned JSON payload supplied by handler.
func newSpokeServer(t *testing.T, handlerFn func(req map[string]any) any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(handlerFn(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newGQLHandlerWithFederation wires a minimal Resolver pointed at the supplied
// FederationProvider. The local query state provider is the same stub the
// other integration tests use, but no local data is exercised here.
func newGQLHandlerWithFederation(t *testing.T, fed ddxgraphql.FederationProvider) http.Handler {
	t.Helper()
	workDir, store := setupIntegrationDir(t)
	state := newTestStateProvider(workDir, store)
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:      state,
			WorkingDir: workDir,
			Actions:    testActionDispatcher{},
			Federation: fed,
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gqlSrv.AddTransport(transport.POST{})
	return gqlSrv
}

// TestFederation_QueryShape_TwoHealthySpokes exercises all four federated
// queries against two mock spokes, both healthy, and verifies that each
// returned row carries the routing metadata (nodeId, projectId, projectUrl,
// writeCapability, status) along with the typed payload shape.
func TestFederation_QueryShape_TwoHealthySpokes(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)

	spoke1 := newSpokeServer(t, func(req map[string]any) any {
		// Same canned response for any query — body is asserted by the
		// resolver decode path, not by query string.
		return map[string]any{
			"data": map[string]any{
				"beads": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "ddx-A1", "title": "spoke1 bead", "status": "open",
						"priority": 0, "issueType": "task",
						"createdAt": "2026-05-01T00:00:00Z", "updatedAt": "2026-05-01T00:00:00Z",
						"projectID": "p1",
					}},
				}},
				"runs": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "run-1", "layer": "run", "status": "success",
						"projectID": "p1", "childRunIds": []string{},
					}},
				}},
				"projects": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "p1", "name": "spoke1-proj", "path": "/tmp/p1",
						"registeredAt": "2026-04-30T00:00:00Z",
						"lastSeen":     "2026-05-02T00:00:00Z",
					}},
				}},
			},
		}
	})
	spoke2 := newSpokeServer(t, func(req map[string]any) any {
		return map[string]any{
			"data": map[string]any{
				"beads": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "ddx-B1", "title": "spoke2 bead", "status": "open",
						"priority": 1, "issueType": "task",
						"createdAt": "2026-05-01T00:00:00Z", "updatedAt": "2026-05-01T00:00:00Z",
						"projectID": "p2",
					}},
				}},
				"runs": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "run-2", "layer": "run", "status": "running",
						"projectID": "p2", "childRunIds": []string{},
					}},
				}},
				"projects": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "p2", "name": "spoke2-proj", "path": "/tmp/p2",
						"registeredAt": "2026-04-30T00:00:00Z",
						"lastSeen":     "2026-05-02T00:00:00Z",
					}},
				}},
			},
		}
	})

	fed := &stubFederation{
		client: federation.NewFanOutClient(),
		spokes: []federation.SpokeRecord{
			{
				NodeID: "node-spoke1", Name: "spoke1", URL: spoke1.URL,
				DDxVersion: "0.1.0", SchemaVersion: federation.CurrentSchemaVersion,
				Capabilities: []string{"read", "@identity:abc"},
				RegisteredAt: now, LastHeartbeat: now, Status: federation.StatusActive,
			},
			{
				NodeID: "node-spoke2", Name: "spoke2", URL: spoke2.URL,
				DDxVersion: "0.1.0", SchemaVersion: federation.CurrentSchemaVersion,
				Capabilities: []string{"read", "write"},
				RegisteredAt: now, LastHeartbeat: now, Status: federation.StatusActive,
			},
		},
	}
	fed.client.HubDDxVersion = "0.1.0"
	fed.client.HubSchemaVersion = federation.CurrentSchemaVersion
	fed.client.PerNodeTimeout = 2 * time.Second

	h := newGQLHandlerWithFederation(t, fed)

	// federationNodes: surface should match spoke registry; @identity:* is
	// stripped; writeCapability flips per-spoke; lastHeartbeat is non-null
	// because the registry has a heartbeat timestamp.
	respNodes := gqlPost(t, h, `{ federationNodes {
		id nodeId name url status ddxVersion schemaVersion
		capabilities registeredAt lastHeartbeat writeCapability lastError
	} }`)
	var nodesData struct {
		FederationNodes []struct {
			ID              string   `json:"id"`
			NodeID          string   `json:"nodeId"`
			Name            string   `json:"name"`
			URL             string   `json:"url"`
			Status          string   `json:"status"`
			DdxVersion      string   `json:"ddxVersion"`
			SchemaVersion   string   `json:"schemaVersion"`
			Capabilities    []string `json:"capabilities"`
			RegisteredAt    string   `json:"registeredAt"`
			LastHeartbeat   *string  `json:"lastHeartbeat"`
			WriteCapability bool     `json:"writeCapability"`
			LastError       *string  `json:"lastError"`
		} `json:"federationNodes"`
	}
	if err := json.Unmarshal(respNodes["data"], &nodesData); err != nil {
		t.Fatalf("unmarshal nodes: %v", err)
	}
	if len(nodesData.FederationNodes) != 2 {
		t.Fatalf("want 2 federation nodes, got %d", len(nodesData.FederationNodes))
	}
	for _, n := range nodesData.FederationNodes {
		for _, c := range n.Capabilities {
			if len(c) > 0 && c[0] == '@' {
				t.Errorf("synthetic capability %q leaked through, expected filtered", c)
			}
		}
		if n.LastHeartbeat == nil {
			t.Errorf("node %s: expected non-null lastHeartbeat", n.NodeID)
		}
	}
	if got := nodesData.FederationNodes[0].WriteCapability; got != false {
		t.Errorf("spoke1 writeCapability: want false, got %v", got)
	}
	if got := nodesData.FederationNodes[1].WriteCapability; got != true {
		t.Errorf("spoke2 writeCapability: want true, got %v", got)
	}

	// federatedBeads: one row per spoke; each row carries projectUrl ==
	// spoke URL and the bead nested payload.
	respBeads := gqlPost(t, h, `{ federatedBeads {
		nodeId projectId projectUrl writeCapability status bead { id title status projectID }
	} }`)
	var beadsData struct {
		FederatedBeads []struct {
			NodeID          string  `json:"nodeId"`
			ProjectID       *string `json:"projectId"`
			ProjectURL      string  `json:"projectUrl"`
			WriteCapability bool    `json:"writeCapability"`
			Status          string  `json:"status"`
			Bead            struct {
				ID        string  `json:"id"`
				Title     string  `json:"title"`
				Status    string  `json:"status"`
				ProjectID *string `json:"projectID"`
			} `json:"bead"`
		} `json:"federatedBeads"`
	}
	if err := json.Unmarshal(respBeads["data"], &beadsData); err != nil {
		t.Fatalf("unmarshal beads: %v", err)
	}
	if len(beadsData.FederatedBeads) != 2 {
		t.Fatalf("want 2 federated beads, got %d", len(beadsData.FederatedBeads))
	}
	seenURL := map[string]bool{spoke1.URL: false, spoke2.URL: false}
	for _, fb := range beadsData.FederatedBeads {
		if _, ok := seenURL[fb.ProjectURL]; !ok {
			t.Errorf("unexpected projectUrl %q", fb.ProjectURL)
		}
		seenURL[fb.ProjectURL] = true
		if fb.ProjectID == nil || *fb.ProjectID == "" {
			t.Errorf("row %s: expected non-empty projectId", fb.NodeID)
		}
		if fb.Bead.ID == "" {
			t.Errorf("row %s: expected non-empty bead.id", fb.NodeID)
		}
	}
	for url, hit := range seenURL {
		if !hit {
			t.Errorf("expected a federated row for spoke URL %s", url)
		}
	}

	// federatedRuns: one row per spoke; layer/status round-trip.
	respRuns := gqlPost(t, h, `{ federatedRuns {
		nodeId projectId projectUrl writeCapability status run { id layer status }
	} }`)
	var runsData struct {
		FederatedRuns []struct {
			NodeID          string `json:"nodeId"`
			ProjectURL      string `json:"projectUrl"`
			WriteCapability bool   `json:"writeCapability"`
			Status          string `json:"status"`
			Run             struct {
				ID     string `json:"id"`
				Layer  string `json:"layer"`
				Status string `json:"status"`
			} `json:"run"`
		} `json:"federatedRuns"`
	}
	if err := json.Unmarshal(respRuns["data"], &runsData); err != nil {
		t.Fatalf("unmarshal runs: %v", err)
	}
	if len(runsData.FederatedRuns) != 2 {
		t.Fatalf("want 2 federated runs, got %d", len(runsData.FederatedRuns))
	}
	for _, fr := range runsData.FederatedRuns {
		if fr.Run.ID == "" || fr.Run.Layer == "" {
			t.Errorf("row %s: malformed run payload %+v", fr.NodeID, fr.Run)
		}
	}

	// federatedProjects: one row per spoke; projectId == project.id.
	respProjects := gqlPost(t, h, `{ federatedProjects {
		nodeId projectId projectUrl writeCapability status project { id name path }
	} }`)
	var projectsData struct {
		FederatedProjects []struct {
			NodeID     string `json:"nodeId"`
			ProjectID  string `json:"projectId"`
			ProjectURL string `json:"projectUrl"`
			Project    struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"project"`
		} `json:"federatedProjects"`
	}
	if err := json.Unmarshal(respProjects["data"], &projectsData); err != nil {
		t.Fatalf("unmarshal projects: %v", err)
	}
	if len(projectsData.FederatedProjects) != 2 {
		t.Fatalf("want 2 federated projects, got %d", len(projectsData.FederatedProjects))
	}
	for _, fp := range projectsData.FederatedProjects {
		if fp.ProjectID != fp.Project.ID {
			t.Errorf("row %s: routing projectId %q != project.id %q",
				fp.NodeID, fp.ProjectID, fp.Project.ID)
		}
	}
}

// TestFederation_PartialResult_OneSpokeOffline verifies that when one of two
// spokes is unreachable, the federated query still returns rows from the
// healthy spoke and federationNodes still surfaces both rows (so the UI can
// render the offline spoke as a partial-result indicator). The offline spoke
// contributes no rows to federatedBeads/Runs/Projects.
func TestFederation_PartialResult_OneSpokeOffline(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)

	good := newSpokeServer(t, func(req map[string]any) any {
		return map[string]any{
			"data": map[string]any{
				"beads": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "ddx-G1", "title": "alive bead", "status": "open",
						"priority": 0, "issueType": "task",
						"createdAt": "2026-05-01T00:00:00Z", "updatedAt": "2026-05-01T00:00:00Z",
						"projectID": "p-good",
					}},
				}},
			},
		}
	})

	fed := &stubFederation{
		client: federation.NewFanOutClient(),
		spokes: []federation.SpokeRecord{
			{
				NodeID: "node-good", Name: "good", URL: good.URL,
				DDxVersion: "0.1.0", SchemaVersion: federation.CurrentSchemaVersion,
				RegisteredAt: now, LastHeartbeat: now, Status: federation.StatusActive,
			},
			{
				NodeID: "node-dead", Name: "dead",
				URL:        "http://127.0.0.1:1", // port 1: connection refused
				DDxVersion: "0.1.0", SchemaVersion: federation.CurrentSchemaVersion,
				RegisteredAt: now, LastHeartbeat: now, Status: federation.StatusActive,
			},
		},
	}
	fed.client.HubDDxVersion = "0.1.0"
	fed.client.HubSchemaVersion = federation.CurrentSchemaVersion
	fed.client.PerNodeTimeout = 500 * time.Millisecond

	h := newGQLHandlerWithFederation(t, fed)

	respNodes := gqlPost(t, h, `{ federationNodes { nodeId status } }`)
	var nodesData struct {
		FederationNodes []struct {
			NodeID string `json:"nodeId"`
			Status string `json:"status"`
		} `json:"federationNodes"`
	}
	if err := json.Unmarshal(respNodes["data"], &nodesData); err != nil {
		t.Fatalf("unmarshal nodes: %v", err)
	}
	if len(nodesData.FederationNodes) != 2 {
		t.Fatalf("federationNodes: want 2 rows including offline spoke, got %d", len(nodesData.FederationNodes))
	}

	respBeads := gqlPost(t, h, `{ federatedBeads { nodeId bead { id } } }`)
	var beadsData struct {
		FederatedBeads []struct {
			NodeID string `json:"nodeId"`
			Bead   struct {
				ID string `json:"id"`
			} `json:"bead"`
		} `json:"federatedBeads"`
	}
	if err := json.Unmarshal(respBeads["data"], &beadsData); err != nil {
		t.Fatalf("unmarshal beads: %v", err)
	}
	if len(beadsData.FederatedBeads) != 1 {
		t.Fatalf("federatedBeads: want exactly 1 row from the live spoke, got %d", len(beadsData.FederatedBeads))
	}
	if beadsData.FederatedBeads[0].NodeID != "node-good" {
		t.Errorf("federatedBeads: want row from node-good, got %q", beadsData.FederatedBeads[0].NodeID)
	}
}

// TestFederation_VersionSkew_RendersAndSkips verifies that a spoke advertising
// an incompatible ddx_version is rendered in federationNodes (so the UI can
// show a version-skew badge) but is filtered out of the data fan-out, leaving
// only compatible spokes contributing rows.
func TestFederation_VersionSkew_RendersAndSkips(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)

	compatible := newSpokeServer(t, func(req map[string]any) any {
		return map[string]any{
			"data": map[string]any{
				"beads": map[string]any{"edges": []map[string]any{
					{"node": map[string]any{
						"id": "ddx-V1", "title": "compat bead", "status": "open",
						"priority": 0, "issueType": "task",
						"createdAt": "2026-05-01T00:00:00Z", "updatedAt": "2026-05-01T00:00:00Z",
					}},
				}},
			},
		}
	})

	skewed := newSpokeServer(t, func(req map[string]any) any {
		t.Errorf("skewed spoke should not be contacted by fan-out")
		return map[string]any{"data": map[string]any{}}
	})

	fed := &stubFederation{
		client: federation.NewFanOutClient(),
		spokes: []federation.SpokeRecord{
			{
				NodeID: "node-compat", Name: "compat", URL: compatible.URL,
				DDxVersion: "0.1.0", SchemaVersion: federation.CurrentSchemaVersion,
				RegisteredAt: now, LastHeartbeat: now, Status: federation.StatusActive,
			},
			{
				NodeID: "node-skew", Name: "skew", URL: skewed.URL,
				// Major-version skew → handshake rejects.
				DDxVersion: "9.0.0", SchemaVersion: federation.CurrentSchemaVersion,
				RegisteredAt: now, LastHeartbeat: now, Status: federation.StatusDegraded,
			},
		},
	}
	fed.client.HubDDxVersion = "0.1.0"
	fed.client.HubSchemaVersion = federation.CurrentSchemaVersion
	fed.client.PerNodeTimeout = 500 * time.Millisecond

	h := newGQLHandlerWithFederation(t, fed)

	respNodes := gqlPost(t, h, `{ federationNodes { nodeId status ddxVersion } }`)
	var nodesData struct {
		FederationNodes []struct {
			NodeID     string `json:"nodeId"`
			Status     string `json:"status"`
			DdxVersion string `json:"ddxVersion"`
		} `json:"federationNodes"`
	}
	if err := json.Unmarshal(respNodes["data"], &nodesData); err != nil {
		t.Fatalf("unmarshal nodes: %v", err)
	}
	if len(nodesData.FederationNodes) != 2 {
		t.Fatalf("federationNodes: want 2 rows (skewed spoke must still be visible), got %d", len(nodesData.FederationNodes))
	}
	foundSkew := false
	for _, n := range nodesData.FederationNodes {
		if n.NodeID == "node-skew" {
			foundSkew = true
			if n.Status != "degraded" {
				t.Errorf("skewed spoke status: want degraded, got %q", n.Status)
			}
			if n.DdxVersion != "9.0.0" {
				t.Errorf("skewed spoke ddxVersion not rendered through federationNodes")
			}
		}
	}
	if !foundSkew {
		t.Error("skewed spoke missing from federationNodes")
	}

	respBeads := gqlPost(t, h, `{ federatedBeads { nodeId } }`)
	var beadsData struct {
		FederatedBeads []struct {
			NodeID string `json:"nodeId"`
		} `json:"federatedBeads"`
	}
	if err := json.Unmarshal(respBeads["data"], &beadsData); err != nil {
		t.Fatalf("unmarshal beads: %v", err)
	}
	if len(beadsData.FederatedBeads) != 1 {
		t.Fatalf("federatedBeads: want 1 row from compatible spoke only, got %d", len(beadsData.FederatedBeads))
	}
	if beadsData.FederatedBeads[0].NodeID != "node-compat" {
		t.Errorf("federatedBeads row: want node-compat, got %q", beadsData.FederatedBeads[0].NodeID)
	}
}
