package server

// TC-GQL-004..005: GraphQL Query resolvers for documents and docGraph.
//
// Integration tests exercising the real ServerState + docgraph path through the
// GraphQL handler. Each test starts a server backed by real state and fires a
// POST /graphql request, verifying the response contains the expected values.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// setupDocTestDir creates a temp directory with docgraph markdown documents.
func setupDocTestDir(t *testing.T) (dir string, docIDs []string) {
	t.Helper()
	dir = setupTestDir(t)

	docs := []struct {
		path    string
		id      string
		content string
	}{
		{
			path:    filepath.Join(dir, "docs", "alpha.md"),
			id:      "alpha",
			content: "---\nddx:\n  id: alpha\n---\n# Alpha\n\nFirst document.\n",
		},
		{
			path:    filepath.Join(dir, "docs", "beta.md"),
			id:      "beta",
			content: "---\nddx:\n  id: beta\n  depends_on:\n    - alpha\n---\n# Beta\n\nSecond document.\n",
		},
	}

	for _, d := range docs {
		if err := os.MkdirAll(filepath.Dir(d.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d.path, []byte(d.content), 0o644); err != nil {
			t.Fatal(err)
		}
		docIDs = append(docIDs, d.id)
	}
	return dir, docIDs
}

// TC-GQL-004: Query.documents returns paginated documents from the working dir.
func TestGraphQLDocuments(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir, docIDs := setupDocTestDir(t)
	srv := New(":0", workDir)

	body := `{"query": "{ documents { edges { node { id path title dependsOn dependents } } totalCount } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Documents struct {
				Edges []struct {
					Node struct {
						ID         string   `json:"id"`
						Path       string   `json:"path"`
						Title      string   `json:"title"`
						DependsOn  []string `json:"dependsOn"`
						Dependents []string `json:"dependents"`
					} `json:"node"`
				} `json:"edges"`
				TotalCount int `json:"totalCount"`
			} `json:"documents"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}
	if resp.Data.Documents.TotalCount != len(docIDs) {
		t.Errorf("expected totalCount=%d, got %d", len(docIDs), resp.Data.Documents.TotalCount)
	}
	if len(resp.Data.Documents.Edges) != len(docIDs) {
		t.Errorf("expected %d edges, got %d", len(docIDs), len(resp.Data.Documents.Edges))
	}

	// Verify the alpha doc has beta as a dependent.
	foundAlpha := false
	for _, e := range resp.Data.Documents.Edges {
		if e.Node.ID == "alpha" {
			foundAlpha = true
			if len(e.Node.Dependents) != 1 || e.Node.Dependents[0] != "beta" {
				t.Errorf("expected alpha.dependents=[beta], got %v", e.Node.Dependents)
			}
		}
	}
	if !foundAlpha {
		t.Error("expected document with id 'alpha' in results")
	}
}

// TC-GQL-005: Query.docGraph returns the full document dependency graph.
func TestGraphQLDocGraph(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir, docIDs := setupDocTestDir(t)
	srv := New(":0", workDir)

	body := `{"query": "{ docGraph { rootDir documents { id } pathToId dependents warnings } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			DocGraph struct {
				RootDir   string `json:"rootDir"`
				Documents []struct {
					ID string `json:"id"`
				} `json:"documents"`
				PathToID   string   `json:"pathToId"`
				Dependents string   `json:"dependents"`
				Warnings   []string `json:"warnings"`
			} `json:"docGraph"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}

	dg := resp.Data.DocGraph
	if dg.RootDir == "" {
		t.Error("expected non-empty rootDir")
	}
	if len(dg.Documents) != len(docIDs) {
		t.Errorf("expected %d documents, got %d", len(docIDs), len(dg.Documents))
	}

	// pathToId should be a valid JSON object.
	var pathToID map[string]string
	if err := json.Unmarshal([]byte(dg.PathToID), &pathToID); err != nil {
		t.Errorf("pathToId is not valid JSON: %v", err)
	}
	if len(pathToID) != len(docIDs) {
		t.Errorf("expected pathToId to have %d entries, got %d", len(docIDs), len(pathToID))
	}

	// dependents should be a valid JSON object.
	var dependents map[string][]string
	if err := json.Unmarshal([]byte(dg.Dependents), &dependents); err != nil {
		t.Errorf("dependents is not valid JSON: %v", err)
	}
	// beta depends on alpha, so alpha should have beta as a dependent.
	if betaDeps, ok := dependents["alpha"]; !ok || len(betaDeps) != 1 || betaDeps[0] != "beta" {
		t.Errorf("expected dependents[alpha]=[beta], got %v", dependents["alpha"])
	}
}
