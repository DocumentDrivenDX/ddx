package server

// TC-GQL-004..006: GraphQL Query resolvers for beads and beadsByProject.
//
// Integration tests exercising the real ServerState path through the
// GraphQL handler. Each test starts a server backed by real state and
// fires POST /graphql requests, verifying pagination and filtering.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type gqlBeadsResp struct {
	Data struct {
		Beads struct {
			Edges []struct {
				Node struct {
					ID     string `json:"id"`
					Title  string `json:"title"`
					Status string `json:"status"`
				} `json:"node"`
				Cursor string `json:"cursor"`
			} `json:"edges"`
			PageInfo struct {
				HasNextPage bool    `json:"hasNextPage"`
				EndCursor   *string `json:"endCursor"`
			} `json:"pageInfo"`
			TotalCount int `json:"totalCount"`
		} `json:"beads"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func gqlBeadsQuery(t *testing.T, srv *Server, args string) gqlBeadsResp {
	t.Helper()
	query := fmt.Sprintf(`{ beads(%s) { edges { node { id title status } cursor } pageInfo { hasNextPage endCursor } totalCount } }`, args)
	rawBody, _ := json.Marshal(map[string]string{"query": query})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp gqlBeadsResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}
	return resp
}

// TC-GQL-004: Query.beads with first/after pagination returns no duplicates across pages.
func TestGraphQLBeadsPagination(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-test-node")

	workDir := setupTestDir(t) // creates 3 beads: bx-001, bx-002, bx-003
	srv := New(":0", workDir)

	// Page 1: first 2 beads.
	page1 := gqlBeadsQuery(t, srv, "first: 2")
	if page1.Data.Beads.TotalCount != 3 {
		t.Errorf("expected totalCount=3, got %d", page1.Data.Beads.TotalCount)
	}
	if len(page1.Data.Beads.Edges) != 2 {
		t.Fatalf("expected 2 edges on page 1, got %d", len(page1.Data.Beads.Edges))
	}
	if !page1.Data.Beads.PageInfo.HasNextPage {
		t.Error("expected hasNextPage=true after first page of 2")
	}
	if page1.Data.Beads.PageInfo.EndCursor == nil {
		t.Fatal("expected non-nil endCursor on page 1")
	}

	endCursor := *page1.Data.Beads.PageInfo.EndCursor

	// Page 2: beads after the first page's endCursor.
	page2 := gqlBeadsQuery(t, srv, fmt.Sprintf(`first: 10, after: "%s"`, endCursor))
	if len(page2.Data.Beads.Edges) != 1 {
		t.Fatalf("expected 1 edge on page 2, got %d", len(page2.Data.Beads.Edges))
	}
	if page2.Data.Beads.PageInfo.HasNextPage {
		t.Error("expected hasNextPage=false on last page")
	}

	// Verify no duplicates between page 1 and page 2.
	seen := make(map[string]bool)
	for _, e := range page1.Data.Beads.Edges {
		seen[e.Node.ID] = true
	}
	for _, e := range page2.Data.Beads.Edges {
		if seen[e.Node.ID] {
			t.Errorf("duplicate bead id %q found across pages", e.Node.ID)
		}
	}
}

// TC-GQL-005: Query.beads with status filter returns only matching beads.
func TestGraphQLBeadsStatusFilter(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	resp := gqlBeadsQuery(t, srv, `status: "closed"`)
	for _, e := range resp.Data.Beads.Edges {
		if e.Node.Status != "closed" {
			t.Errorf("expected status=closed, got %q for bead %s", e.Node.Status, e.Node.ID)
		}
	}
	if resp.Data.Beads.TotalCount != 1 {
		t.Errorf("expected 1 closed bead, got totalCount=%d", resp.Data.Beads.TotalCount)
	}
}

// TC-GQL-006: Query.beadsByProject returns beads scoped to a specific project.
func TestGraphQLBeadsByProject(t *testing.T) {
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

	query := fmt.Sprintf(`{ beadsByProject(projectID: "%s", first: 10) { edges { node { id title status } cursor } pageInfo { hasNextPage endCursor } totalCount } }`, projID)
	rawBody, _ := json.Marshal(map[string]string{"query": query})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			BeadsByProject struct {
				Edges []struct {
					Node struct {
						ID string `json:"id"`
					} `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage bool `json:"hasNextPage"`
				} `json:"pageInfo"`
				TotalCount int `json:"totalCount"`
			} `json:"beadsByProject"`
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
	if resp.Data.BeadsByProject.TotalCount != 3 {
		t.Errorf("expected 3 beads for project, got %d", resp.Data.BeadsByProject.TotalCount)
	}
	if resp.Data.BeadsByProject.PageInfo.HasNextPage {
		t.Error("expected hasNextPage=false when first=10 and only 3 beads exist")
	}
}
