package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// TestGraphQLBeadsNeedsHumanQuery verifies that beadsNeedsHuman returns only
// needs_human-labeled open beads and that needsHuman/needsHumanReason fields
// are populated from the bead store's Extra metadata.
func TestGraphQLBeadsNeedsHumanQuery(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	// Ordinary open bead must NOT appear in beadsNeedsHuman.
	ordinary := &bead.Bead{Title: "Ordinary bead", Status: bead.StatusOpen}
	if err := store.Create(ordinary); err != nil {
		t.Fatal(err)
	}

	nh := &bead.Bead{
		Title:  "Needs human bead",
		Status: bead.StatusOpen,
		Labels: []string{"needs_human"},
	}
	if err := store.Create(nh); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(nh.ID, func(b *bead.Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["needs-human-reason"] = "review-block"
		b.Extra["needs-human-suggested-action"] = "split or retry"
		b.Extra["needs-human-summary"] = "Agent failed 3 times"
	}); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	resp := gqlPost(t, h, `{
		beadsNeedsHuman(first: 10) {
			edges { node { id title needsHuman needsHumanReason needsHumanSuggestedAction needsHumanSummary } }
			totalCount
		}
	}`)

	var data struct {
		BeadsNeedsHuman struct {
			Edges []struct {
				Node struct {
					ID                        string  `json:"id"`
					Title                     string  `json:"title"`
					NeedsHuman                bool    `json:"needsHuman"`
					NeedsHumanReason          *string `json:"needsHumanReason"`
					NeedsHumanSuggestedAction *string `json:"needsHumanSuggestedAction"`
					NeedsHumanSummary         *string `json:"needsHumanSummary"`
				} `json:"node"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"beadsNeedsHuman"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	result := data.BeadsNeedsHuman
	if result.TotalCount != 1 {
		t.Fatalf("totalCount: want 1, got %d", result.TotalCount)
	}
	if len(result.Edges) != 1 {
		t.Fatalf("edges: want 1, got %d", len(result.Edges))
	}
	node := result.Edges[0].Node
	if node.ID != nh.ID {
		t.Errorf("id: want %q, got %q", nh.ID, node.ID)
	}
	if !node.NeedsHuman {
		t.Error("needsHuman: want true")
	}
	if node.NeedsHumanReason == nil || *node.NeedsHumanReason != "review-block" {
		t.Errorf("needsHumanReason: want %q, got %v", "review-block", node.NeedsHumanReason)
	}
	if node.NeedsHumanSuggestedAction == nil || *node.NeedsHumanSuggestedAction != "split or retry" {
		t.Errorf("needsHumanSuggestedAction: want %q, got %v", "split or retry", node.NeedsHumanSuggestedAction)
	}
	if node.NeedsHumanSummary == nil || *node.NeedsHumanSummary != "Agent failed 3 times" {
		t.Errorf("needsHumanSummary: want %q, got %v", "Agent failed 3 times", node.NeedsHumanSummary)
	}
}

// TestGraphQLBeadHumanResolveRetry verifies that beadHumanResolve with action
// retry removes the needs_human label and appends a human-resolution event.
func TestGraphQLBeadHumanResolveRetry(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	nh := &bead.Bead{
		Title:  "Needs human bead",
		Status: bead.StatusOpen,
		Labels: []string{"needs_human"},
	}
	if err := store.Create(nh); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadHumanResolve(id: "` + nh.ID + `", action: retry, note: "retrying with smaller scope") {
			id needsHuman labels
		}
	}`
	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadHumanResolve struct {
			ID         string   `json:"id"`
			NeedsHuman bool     `json:"needsHuman"`
			Labels     []string `json:"labels"`
		} `json:"beadHumanResolve"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	got := data.BeadHumanResolve
	if got.ID != nh.ID {
		t.Errorf("id: want %q, got %q", nh.ID, got.ID)
	}
	if got.NeedsHuman {
		t.Error("needsHuman: want false after retry resolution")
	}
	for _, l := range got.Labels {
		if l == "needs_human" {
			t.Error("labels: needs_human should be removed after retry")
			break
		}
	}

	// Verify a human-resolution event was appended.
	events, err := store.Events(nh.ID)
	if err != nil {
		t.Fatalf("store.Events: %v", err)
	}
	var found bool
	for _, e := range events {
		if e.Kind == "human-resolution" && e.Summary == "retry" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected human-resolution event with summary=retry; got events: %v", events)
	}
}

// TestGraphQLBeadHumanResolveRequiresNote verifies that all beadHumanResolve
// actions reject empty notes.
func TestGraphQLBeadHumanResolveRequiresNote(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	nh := &bead.Bead{
		Title:  "Needs human bead",
		Status: bead.StatusOpen,
		Labels: []string{"needs_human"},
	}
	if err := store.Create(nh); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	for _, action := range []string{"retry", "split", "obsolete", "defer"} {
		mutation := `mutation {
			beadHumanResolve(id: "` + nh.ID + `", action: ` + action + `, note: "") {
				id
			}
		}`
		rawBody, _ := json.Marshal(map[string]string{"query": mutation})
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		var resp map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("action=%s: invalid JSON: %v", action, err)
		}
		if resp["errors"] == nil {
			t.Errorf("action=%s: expected error when note is empty, got data=%s", action, resp["data"])
		}
	}
}

// TestGraphQLBeadStatusCountsNeedsHuman verifies that beadsStatus returns
// needsHuman and workerReady counts consistent with store.Status().
func TestGraphQLBeadStatusCountsNeedsHuman(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	// 2 ordinary open, 1 needs_human, 1 closed.
	for i := 0; i < 2; i++ {
		b := &bead.Bead{Title: "Open bead", Status: bead.StatusOpen}
		if err := store.Create(b); err != nil {
			t.Fatal(err)
		}
	}
	nh := &bead.Bead{Title: "Needs human", Status: bead.StatusOpen, Labels: []string{"needs_human"}}
	if err := store.Create(nh); err != nil {
		t.Fatal(err)
	}
	closed := &bead.Bead{Title: "Closed bead", Status: bead.StatusOpen}
	if err := store.Create(closed); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(closed.ID); err != nil {
		t.Fatal(err)
	}

	storeCounts, err := store.Status()
	if err != nil {
		t.Fatalf("store.Status: %v", err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	resp := gqlPost(t, h, `{ beadsStatus { open closed blocked ready total needsHuman workerReady } }`)

	var data struct {
		BeadsStatus struct {
			Open        int `json:"open"`
			Closed      int `json:"closed"`
			Blocked     int `json:"blocked"`
			Ready       int `json:"ready"`
			Total       int `json:"total"`
			NeedsHuman  int `json:"needsHuman"`
			WorkerReady int `json:"workerReady"`
		} `json:"beadsStatus"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	got := data.BeadsStatus
	if got.Open != storeCounts.Open {
		t.Errorf("open: want %d (from store), got %d", storeCounts.Open, got.Open)
	}
	if got.Closed != storeCounts.Closed {
		t.Errorf("closed: want %d (from store), got %d", storeCounts.Closed, got.Closed)
	}
	if got.Total != storeCounts.Total {
		t.Errorf("total: want %d (from store), got %d", storeCounts.Total, got.Total)
	}
	if got.NeedsHuman != 1 {
		t.Errorf("needsHuman: want 1, got %d", got.NeedsHuman)
	}
	if got.WorkerReady < 0 {
		t.Errorf("workerReady: want >= 0, got %d", got.WorkerReady)
	}
}
