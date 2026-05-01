package server

// TC-GQL-001..003: GraphQL Query resolvers for node and projects.
//
// Integration tests exercising the real ServerState path through the
// GraphQL handler. Each test starts a server backed by real state and
// fires a POST /graphql request, verifying the response contains the
// expected values.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TC-GQL-001: Query.nodeInfo returns the running server node's id and name.
func TestGraphQLNodeInfo(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	body := `{"query": "{ nodeInfo { id name } }"}`
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
			NodeInfo struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"nodeInfo"`
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
	if !strings.HasPrefix(resp.Data.NodeInfo.ID, "node-") {
		t.Errorf("expected nodeInfo.id to start with 'node-', got %q", resp.Data.NodeInfo.ID)
	}
	if resp.Data.NodeInfo.Name != "gql-test-node" {
		t.Errorf("expected nodeInfo.name=gql-test-node, got %q", resp.Data.NodeInfo.Name)
	}
}

// TC-GQL-002: Query.projects returns real registered project data.
func TestGraphQLProjects(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	body := `{"query": "{ projects { edges { node { id path name } } totalCount } }"}`
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
			Projects struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Path string `json:"path"`
						Name string `json:"name"`
					} `json:"node"`
				} `json:"edges"`
				TotalCount int `json:"totalCount"`
			} `json:"projects"`
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
	if resp.Data.Projects.TotalCount == 0 {
		t.Error("expected totalCount > 0")
	}
	if len(resp.Data.Projects.Edges) == 0 {
		t.Error("expected at least one project edge")
	}
	found := false
	for _, e := range resp.Data.Projects.Edges {
		if e.Node.Path == workDir {
			found = true
			if !strings.HasPrefix(e.Node.ID, "proj-") {
				t.Errorf("expected project id to start with 'proj-', got %q", e.Node.ID)
			}
		}
	}
	if !found {
		t.Errorf("workDir %s not found in projects response", workDir)
	}
}

// TC-GQL-003: Query.node(id) returns a Project when given a project ID.
func TestGraphQLNodeByProjectID(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	projects := srv.state.GetProjects()
	if len(projects) == 0 {
		t.Fatal("no projects registered")
	}
	projID := projects[0].ID

	rawBody, _ := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`{ node(id: "%s") { id } }`, projID),
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Node struct {
				ID string `json:"id"`
			} `json:"node"`
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
	if resp.Data.Node.ID != projID {
		t.Errorf("expected node.id=%q, got %q", projID, resp.Data.Node.ID)
	}
}
