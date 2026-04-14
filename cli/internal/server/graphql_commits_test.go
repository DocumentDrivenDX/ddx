package server

// TC-GQL-004..005: GraphQL Query resolver for commits with Relay cursor pagination.
//
// Integration tests exercising the real ServerState → git log path through the
// GraphQL handler. Each test starts a server backed by a real git repo and
// fires POST /graphql requests, verifying that commits are returned and that
// cursor-based pagination advances correctly.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TC-GQL-004: Query.commits returns real git log output for a registered project.
func TestGraphQLCommits(t *testing.T) {
	subjects := []string{"first commit", "second commit", "third commit"}
	_, srv, projID := setupCommitsTestDir(t, subjects)

	rawBody, _ := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`{ commits(projectID: "%s") { edges { node { sha shortSha author subject } cursor } pageInfo { hasNextPage hasPreviousPage startCursor endCursor } totalCount } }`, projID),
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Commits struct {
				Edges []struct {
					Node struct {
						Sha      string `json:"sha"`
						ShortSha string `json:"shortSha"`
						Author   string `json:"author"`
						Subject  string `json:"subject"`
					} `json:"node"`
					Cursor string `json:"cursor"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage     bool    `json:"hasNextPage"`
					HasPreviousPage bool    `json:"hasPreviousPage"`
					StartCursor     *string `json:"startCursor"`
					EndCursor       *string `json:"endCursor"`
				} `json:"pageInfo"`
				TotalCount int `json:"totalCount"`
			} `json:"commits"`
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

	commits := resp.Data.Commits
	if commits.TotalCount != 3 {
		t.Errorf("expected totalCount=3, got %d", commits.TotalCount)
	}
	if len(commits.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(commits.Edges))
	}
	// git log returns newest first: third, second, first.
	expectedOrder := []string{"third commit", "second commit", "first commit"}
	for i, want := range expectedOrder {
		if got := commits.Edges[i].Node.Subject; got != want {
			t.Errorf("edge[%d] subject: want %q, got %q", i, want, got)
		}
	}
	if commits.Edges[0].Node.Author != "Test User" {
		t.Errorf("expected author=Test User, got %q", commits.Edges[0].Node.Author)
	}
	if commits.Edges[0].Node.Sha == "" {
		t.Error("expected non-empty sha on first commit")
	}
	if commits.Edges[0].Node.ShortSha == "" {
		t.Error("expected non-empty shortSha on first commit")
	}
	if commits.PageInfo.HasNextPage {
		t.Error("expected hasNextPage=false when all commits fit on one page")
	}
	if commits.PageInfo.HasPreviousPage {
		t.Error("expected hasPreviousPage=false on the first page")
	}
}

// TC-GQL-005: Query.commits cursor pagination — first/after advances through pages.
func TestGraphQLCommitsCursorPagination(t *testing.T) {
	subjects := []string{"commit-A", "commit-B", "commit-C", "commit-D"}
	_, srv, projID := setupCommitsTestDir(t, subjects)

	// Fetch page 1: first 2 commits.
	page1Query := fmt.Sprintf(`{ commits(projectID: "%s", first: 2) { edges { node { subject } cursor } pageInfo { hasNextPage endCursor } totalCount } }`, projID)
	page1Body, _ := json.Marshal(map[string]string{"query": page1Query})
	req1 := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(page1Body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("page1: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	var resp1 struct {
		Data struct {
			Commits struct {
				Edges []struct {
					Node   struct{ Subject string } `json:"node"`
					Cursor string                   `json:"cursor"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage bool    `json:"hasNextPage"`
					EndCursor   *string `json:"endCursor"`
				} `json:"pageInfo"`
				TotalCount int `json:"totalCount"`
			} `json:"commits"`
		} `json:"data"`
		Errors []struct{ Message string } `json:"errors"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("page1: invalid JSON: %v", err)
	}
	if len(resp1.Errors) > 0 {
		t.Fatalf("page1 GraphQL errors: %v", resp1.Errors)
	}

	p1 := resp1.Data.Commits
	if p1.TotalCount != 4 {
		t.Errorf("page1: expected totalCount=4, got %d", p1.TotalCount)
	}
	if len(p1.Edges) != 2 {
		t.Fatalf("page1: expected 2 edges with first=2, got %d", len(p1.Edges))
	}
	if !p1.PageInfo.HasNextPage {
		t.Error("page1: expected hasNextPage=true")
	}
	if p1.PageInfo.EndCursor == nil {
		t.Fatal("page1: expected non-nil endCursor")
	}
	// Newest first: commit-D, commit-C, commit-B, commit-A
	if p1.Edges[0].Node.Subject != "commit-D" {
		t.Errorf("page1 edge[0]: want commit-D, got %q", p1.Edges[0].Node.Subject)
	}
	if p1.Edges[1].Node.Subject != "commit-C" {
		t.Errorf("page1 edge[1]: want commit-C, got %q", p1.Edges[1].Node.Subject)
	}

	// Fetch page 2: first 2 after the end cursor of page 1.
	endCursor := *p1.PageInfo.EndCursor
	page2Query := fmt.Sprintf(`{ commits(projectID: "%s", first: 2, after: "%s") { edges { node { subject } } pageInfo { hasNextPage hasPreviousPage } } }`, projID, endCursor)
	page2Body, _ := json.Marshal(map[string]string{"query": page2Query})
	req2 := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(page2Body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("page2: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp2 struct {
		Data struct {
			Commits struct {
				Edges []struct {
					Node struct{ Subject string } `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage     bool `json:"hasNextPage"`
					HasPreviousPage bool `json:"hasPreviousPage"`
				} `json:"pageInfo"`
			} `json:"commits"`
		} `json:"data"`
		Errors []struct{ Message string } `json:"errors"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("page2: invalid JSON: %v", err)
	}
	if len(resp2.Errors) > 0 {
		t.Fatalf("page2 GraphQL errors: %v", resp2.Errors)
	}

	p2 := resp2.Data.Commits
	if len(p2.Edges) != 2 {
		t.Fatalf("page2: expected 2 edges, got %d", len(p2.Edges))
	}
	if p2.Edges[0].Node.Subject != "commit-B" {
		t.Errorf("page2 edge[0]: want commit-B, got %q", p2.Edges[0].Node.Subject)
	}
	if p2.Edges[1].Node.Subject != "commit-A" {
		t.Errorf("page2 edge[1]: want commit-A, got %q", p2.Edges[1].Node.Subject)
	}
	if p2.PageInfo.HasNextPage {
		t.Error("page2: expected hasNextPage=false (last page)")
	}
	if !p2.PageInfo.HasPreviousPage {
		t.Error("page2: expected hasPreviousPage=true")
	}
}
