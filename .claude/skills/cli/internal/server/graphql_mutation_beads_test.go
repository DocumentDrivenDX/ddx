package server

// TC-GQL-MUT-001..005: GraphQL Mutation resolvers for bead lifecycle.
//
// Integration tests exercising beadCreate, beadUpdate, beadClaim, beadUnclaim,
// and beadReopen through the real GraphQL handler backed by a live bead store.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// gqlMutation sends a GraphQL mutation and returns the raw response body.
func gqlMutation(t *testing.T, srv *Server, mutation string) map[string]json.RawMessage {
	t.Helper()
	rawBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
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

// TC-GQL-MUT-001: beadCreate creates a new bead and returns it with expected fields.
func TestGraphQLBeadCreate(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	mutation := `mutation {
		beadCreate(input: {
			title: "GraphQL test bead"
			issueType: "task"
			priority: 1
			labels: ["graphql", "test"]
			description: "Created via GraphQL mutation"
			acceptance: "Must appear in bead list"
		}) {
			id
			title
			status
			priority
			issueType
			labels
			description
			acceptance
		}
	}`

	resp := gqlMutation(t, srv, mutation)

	var data struct {
		BeadCreate struct {
			ID          string   `json:"id"`
			Title       string   `json:"title"`
			Status      string   `json:"status"`
			Priority    int      `json:"priority"`
			IssueType   string   `json:"issueType"`
			Labels      []string `json:"labels"`
			Description *string  `json:"description"`
			Acceptance  *string  `json:"acceptance"`
		} `json:"beadCreate"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	b := data.BeadCreate
	if b.ID == "" {
		t.Error("expected non-empty id")
	}
	if b.Title != "GraphQL test bead" {
		t.Errorf("expected title %q, got %q", "GraphQL test bead", b.Title)
	}
	if b.Status != "open" {
		t.Errorf("expected status=open, got %q", b.Status)
	}
	if b.Priority != 1 {
		t.Errorf("expected priority=1, got %d", b.Priority)
	}
	if b.IssueType != "task" {
		t.Errorf("expected issueType=task, got %q", b.IssueType)
	}
	if len(b.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(b.Labels))
	}
	if b.Description == nil || *b.Description != "Created via GraphQL mutation" {
		t.Errorf("unexpected description: %v", b.Description)
	}
	if b.Acceptance == nil || *b.Acceptance != "Must appear in bead list" {
		t.Errorf("unexpected acceptance: %v", b.Acceptance)
	}

	// Verify the bead can be queried back.
	queryResp := gqlMutation(t, srv, `{ beads { edges { node { id title } } totalCount } }`)
	var qdata struct {
		Beads struct {
			TotalCount int `json:"totalCount"`
		} `json:"beads"`
	}
	if err := json.Unmarshal(queryResp["data"], &qdata); err != nil {
		t.Fatalf("parse query data: %v", err)
	}
	// setupTestDir creates 3 beads; we added 1 more.
	if qdata.Beads.TotalCount != 4 {
		t.Errorf("expected 4 total beads after create, got %d", qdata.Beads.TotalCount)
	}
}

// TC-GQL-MUT-002: beadUpdate updates fields on an existing bead.
func TestGraphQLBeadUpdate(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-test-node")

	workDir := setupTestDir(t) // has bx-001 (open), bx-002 (closed), bx-003
	srv := New(":0", workDir)

	mutation := `mutation {
		beadUpdate(id: "bx-001", input: {
			title: "Updated title"
			priority: 3
			notes: "some notes"
		}) {
			id
			title
			priority
			notes
			status
		}
	}`

	resp := gqlMutation(t, srv, mutation)

	var data struct {
		BeadUpdate struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			Priority int     `json:"priority"`
			Notes    *string `json:"notes"`
			Status   string  `json:"status"`
		} `json:"beadUpdate"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	b := data.BeadUpdate
	if b.ID != "bx-001" {
		t.Errorf("expected id=bx-001, got %q", b.ID)
	}
	if b.Title != "Updated title" {
		t.Errorf("expected title %q, got %q", "Updated title", b.Title)
	}
	if b.Priority != 3 {
		t.Errorf("expected priority=3, got %d", b.Priority)
	}
	if b.Notes == nil || *b.Notes != "some notes" {
		t.Errorf("unexpected notes: %v", b.Notes)
	}
	if b.Status != "open" {
		t.Errorf("expected status=open unchanged, got %q", b.Status)
	}
}

// TC-GQL-MUT-003: beadClaim sets bead to in_progress with the given assignee.
func TestGraphQLBeadClaim(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-test-node")

	workDir := setupTestDir(t) // bx-001 is open
	srv := New(":0", workDir)

	mutation := `mutation {
		beadClaim(id: "bx-001", assignee: "alice") {
			id
			status
			owner
		}
	}`

	resp := gqlMutation(t, srv, mutation)

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

	b := data.BeadClaim
	if b.ID != "bx-001" {
		t.Errorf("expected id=bx-001, got %q", b.ID)
	}
	if b.Status != "in_progress" {
		t.Errorf("expected status=in_progress, got %q", b.Status)
	}
	if b.Owner == nil || *b.Owner != "alice" {
		t.Errorf("expected owner=alice, got %v", b.Owner)
	}
}

// TC-GQL-MUT-004: beadUnclaim clears claim and reverts status to open.
func TestGraphQLBeadUnclaim(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-test-node")

	workDir := setupTestDir(t) // bx-001 is open
	srv := New(":0", workDir)

	// Claim first.
	claimMut := `mutation { beadClaim(id: "bx-001", assignee: "bob") { id status owner } }`
	gqlMutation(t, srv, claimMut)

	// Now unclaim.
	unclaimMut := `mutation { beadUnclaim(id: "bx-001") { id status owner } }`
	resp := gqlMutation(t, srv, unclaimMut)

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

	b := data.BeadUnclaim
	if b.ID != "bx-001" {
		t.Errorf("expected id=bx-001, got %q", b.ID)
	}
	if b.Status != "open" {
		t.Errorf("expected status=open after unclaim, got %q", b.Status)
	}
	if b.Owner != nil && *b.Owner != "" {
		t.Errorf("expected owner=nil after unclaim, got %v", b.Owner)
	}
}

// TC-GQL-MUT-005: beadReopen sets a closed bead back to open.
func TestGraphQLBeadReopen(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-test-node")

	workDir := setupTestDir(t) // bx-002 is closed
	srv := New(":0", workDir)

	mutation := `mutation {
		beadReopen(id: "bx-002") {
			id
			status
			owner
		}
	}`

	resp := gqlMutation(t, srv, mutation)

	var data struct {
		BeadReopen struct {
			ID     string  `json:"id"`
			Status string  `json:"status"`
			Owner  *string `json:"owner"`
		} `json:"beadReopen"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	b := data.BeadReopen
	if b.ID != "bx-002" {
		t.Errorf("expected id=bx-002, got %q", b.ID)
	}
	if b.Status != "open" {
		t.Errorf("expected status=open after reopen, got %q", b.Status)
	}
}
