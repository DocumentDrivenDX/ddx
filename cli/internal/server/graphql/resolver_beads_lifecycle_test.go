package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// TestGraphQLBeadsReadyAndBlockedUseLifecycleBuckets verifies that:
// - beadsReady returns open beads with no unmet deps (not proposed, not dep-waiting)
// - beadsBlocked returns ONLY external-blocked beads (status=blocked with external reason)
// - beadsDependencyWaiting returns open beads with unmet deps
func TestGraphQLBeadsReadyAndBlockedUseLifecycleBuckets(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	ready := &bead.Bead{Title: "Ready bead", Status: bead.StatusOpen}
	dep := &bead.Bead{Title: "Dependency bead", Status: bead.StatusOpen}
	waiting := &bead.Bead{Title: "Waiting bead", Status: bead.StatusOpen}
	proposed := &bead.Bead{Title: "Operator attention", Status: bead.StatusProposed}
	extBlocked := &bead.Bead{Title: "Externally blocked", Status: bead.StatusOpen}
	for _, b := range []*bead.Bead{ready, dep, waiting, proposed, extBlocked} {
		if err := store.Create(context.Background(), b); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DepAdd(waiting.ID, dep.ID); err != nil {
		t.Fatal(err)
	}
	// Transition extBlocked to blocked with an external reason
	if err := store.TransitionLifecycle(extBlocked.ID, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: "waiting for upstream API",
		Reason:                "test",
	}, nil); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	resp := gqlPost(t, h, `{
		beadsReady(first: 10) { edges { node { id status } } totalCount }
		beadsBlocked(first: 10) { edges { node { id status } } totalCount }
		beadsDependencyWaiting(first: 10) { edges { node { id status } } totalCount }
	}`)
	var data struct {
		BeadsReady struct {
			Edges []struct {
				Node struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"node"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"beadsReady"`
		BeadsBlocked struct {
			Edges []struct {
				Node struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"node"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"beadsBlocked"`
		BeadsDependencyWaiting struct {
			Edges []struct {
				Node struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"node"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"beadsDependencyWaiting"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	// Ready: ready + dep (not waiting, not proposed, not extBlocked)
	if data.BeadsReady.TotalCount != 2 {
		t.Fatalf("ready totalCount: want 2, got %d", data.BeadsReady.TotalCount)
	}
	readyIDs := map[string]bool{}
	for _, edge := range data.BeadsReady.Edges {
		readyIDs[edge.Node.ID] = true
	}
	if !readyIDs[ready.ID] || !readyIDs[dep.ID] || readyIDs[waiting.ID] || readyIDs[proposed.ID] || readyIDs[extBlocked.ID] {
		t.Fatalf("unexpected ready IDs: %#v", readyIDs)
	}
	// Blocked: only external-blocked bead
	if data.BeadsBlocked.TotalCount != 1 {
		t.Fatalf("blocked totalCount: want 1, got %d", data.BeadsBlocked.TotalCount)
	}
	if len(data.BeadsBlocked.Edges) != 1 || data.BeadsBlocked.Edges[0].Node.ID != extBlocked.ID {
		t.Fatalf("blocked edge: want %s, got %+v", extBlocked.ID, data.BeadsBlocked.Edges)
	}
	// DependencyWaiting: only waiting bead
	if data.BeadsDependencyWaiting.TotalCount != 1 {
		t.Fatalf("dependencyWaiting totalCount: want 1, got %d", data.BeadsDependencyWaiting.TotalCount)
	}
	if len(data.BeadsDependencyWaiting.Edges) != 1 || data.BeadsDependencyWaiting.Edges[0].Node.ID != waiting.ID {
		t.Fatalf("dependencyWaiting edge: want %s, got %+v", waiting.ID, data.BeadsDependencyWaiting.Edges)
	}
}

// TestGraphQLBeadStatusCountsLifecycleStates verifies all six status buckets
// plus derived counts (operatorAttention, dependencyWaiting, externalBlocked).
func TestGraphQLBeadStatusCountsLifecycleStates(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	// Seed beads in all six statuses
	openBead := &bead.Bead{Title: "Open bead", Status: bead.StatusOpen}
	inProgressBead := &bead.Bead{Title: "In-progress bead", Status: bead.StatusOpen}
	proposedBead := &bead.Bead{Title: "Proposed bead", Status: bead.StatusProposed}
	blockedBead := &bead.Bead{Title: "Blocked bead", Status: bead.StatusOpen}
	depBead := &bead.Bead{Title: "Dep target bead", Status: bead.StatusOpen}
	waitingBead := &bead.Bead{Title: "Waiting bead (dep-wait)", Status: bead.StatusOpen}
	closedBead := &bead.Bead{Title: "Closed bead", Status: bead.StatusOpen}
	cancelledBead := &bead.Bead{Title: "Cancelled bead", Status: bead.StatusOpen}

	for _, b := range []*bead.Bead{openBead, inProgressBead, proposedBead, blockedBead, depBead, waitingBead, closedBead, cancelledBead} {
		if err := store.Create(context.Background(), b); err != nil {
			t.Fatal(err)
		}
	}

	// Transition inProgress to in_progress via claim
	if err := store.Claim(inProgressBead.ID, "test-worker"); err != nil {
		t.Fatal(err)
	}
	// Transition blocked to blocked with external reason
	if err := store.TransitionLifecycle(blockedBead.ID, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: "waiting for infra",
		Reason:                "test",
	}, nil); err != nil {
		t.Fatal(err)
	}
	// Add dep to waitingBead
	if err := store.DepAdd(waitingBead.ID, depBead.ID); err != nil {
		t.Fatal(err)
	}
	// Close closedBead
	if err := store.Close(context.Background(), closedBead.ID); err != nil {
		t.Fatal(err)
	}
	// Cancel cancelledBead
	if err := store.TransitionLifecycle(cancelledBead.ID, bead.StatusCancelled, bead.LifecycleTransitionOptions{
		Reason: "test cancel",
	}, nil); err != nil {
		t.Fatal(err)
	}

	storeCounts, err := store.Status()
	if err != nil {
		t.Fatalf("store.Status: %v", err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	resp := gqlPost(t, h, `{
		beadsStatus {
			open inProgress closed blocked proposed cancelled
			ready workerReady dependencyWaiting externalBlocked operatorAttention total
		}
	}`)

	var data struct {
		BeadsStatus struct {
			Open              int `json:"open"`
			InProgress        int `json:"inProgress"`
			Closed            int `json:"closed"`
			Blocked           int `json:"blocked"`
			Proposed          int `json:"proposed"`
			Cancelled         int `json:"cancelled"`
			Ready             int `json:"ready"`
			WorkerReady       int `json:"workerReady"`
			DependencyWaiting int `json:"dependencyWaiting"`
			ExternalBlocked   int `json:"externalBlocked"`
			OperatorAttention int `json:"operatorAttention"`
			Total             int `json:"total"`
		} `json:"beadsStatus"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	got := data.BeadsStatus
	if got.Open != storeCounts.Open {
		t.Errorf("open: want %d, got %d", storeCounts.Open, got.Open)
	}
	if got.InProgress != storeCounts.InProgress {
		t.Errorf("inProgress: want %d, got %d", storeCounts.InProgress, got.InProgress)
	}
	if got.Closed != storeCounts.Closed {
		t.Errorf("closed: want %d, got %d", storeCounts.Closed, got.Closed)
	}
	if got.Blocked != storeCounts.Blocked {
		t.Errorf("blocked: want %d, got %d", storeCounts.Blocked, got.Blocked)
	}
	if got.Proposed != storeCounts.Proposed {
		t.Errorf("proposed: want %d, got %d", storeCounts.Proposed, got.Proposed)
	}
	if got.Cancelled != storeCounts.Cancelled {
		t.Errorf("cancelled: want %d, got %d", storeCounts.Cancelled, got.Cancelled)
	}
	if got.Total != storeCounts.Total {
		t.Errorf("total: want %d, got %d", storeCounts.Total, got.Total)
	}
	if got.OperatorAttention != storeCounts.Proposed {
		t.Errorf("operatorAttention: want proposed count %d, got %d", storeCounts.Proposed, got.OperatorAttention)
	}
	if got.DependencyWaiting == 0 {
		t.Errorf("dependencyWaiting: want >0, got 0")
	}
	if got.ExternalBlocked == 0 {
		t.Errorf("externalBlocked: want >0, got 0")
	}
}

// TestGraphQLBeadProposedApprove verifies that beadApprove transitions
// proposed->open and appends a human-resolution event.
func TestGraphQLBeadProposedApprove(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	proposed := &bead.Bead{Title: "Proposed bead", Status: bead.StatusProposed}
	if err := store.Create(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadApprove(id: "` + proposed.ID + `", note: "approved after review") {
			id status
		}
	}`
	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadApprove struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"beadApprove"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if data.BeadApprove.Status != bead.StatusOpen {
		t.Errorf("status: want %q, got %q", bead.StatusOpen, data.BeadApprove.Status)
	}

	// Verify event was appended
	events, err := store.Events(proposed.ID)
	if err != nil {
		t.Fatalf("store.Events: %v", err)
	}
	var found bool
	for _, e := range events {
		if e.Kind == "human-resolution" && e.Summary == "approve" {
			found = true
		}
	}
	if !found {
		t.Error("expected human-resolution event with summary=approve")
	}
}

// TestGraphQLBeadProposedApproveRejectsEmptyNote verifies that beadApprove
// rejects an empty note.
func TestGraphQLBeadProposedApproveRejectsEmptyNote(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	proposed := &bead.Bead{Title: "Proposed bead", Status: bead.StatusProposed}
	if err := store.Create(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadApprove(id: "` + proposed.ID + `", note: "") {
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
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["errors"] == nil {
		t.Error("expected error for empty note, got none")
	}
}

// TestGraphQLBeadCancelTerminal verifies cancellation and terminal state.
func TestGraphQLBeadCancelTerminal(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	openBead := &bead.Bead{Title: "Open bead to cancel", Status: bead.StatusOpen}
	dependent := &bead.Bead{Title: "Dependent bead", Status: bead.StatusOpen}
	for _, b := range []*bead.Bead{openBead, dependent} {
		if err := store.Create(context.Background(), b); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DepAdd(dependent.ID, openBead.ID); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadCancel(id: "` + openBead.ID + `", reason: "no longer needed") {
			id status
		}
	}`
	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadCancel struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"beadCancel"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if data.BeadCancel.Status != bead.StatusCancelled {
		t.Errorf("status: want %q, got %q", bead.StatusCancelled, data.BeadCancel.Status)
	}

	// Verify it's terminal: try to transition again
	err := store.TransitionLifecycle(openBead.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{ManualReopen: true}, nil)
	if err == nil {
		t.Error("expected error transitioning from cancelled (terminal), got nil")
	}

	// Verify dependent is NOT automatically closed
	dep, err := store.Get(context.Background(), dependent.ID)
	if err != nil {
		t.Fatalf("store.Get dependent: %v", err)
	}
	if dep.Status == bead.StatusClosed {
		t.Error("dependent bead should not be auto-closed when dependency is cancelled")
	}
}

// TestGraphQLBeadBlockExternal verifies beadBlock blocks a bead with external reason.
func TestGraphQLBeadBlockExternal(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	openBead := &bead.Bead{Title: "Open bead to block", Status: bead.StatusOpen}
	if err := store.Create(context.Background(), openBead); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadBlock(id: "` + openBead.ID + `", externalBlockerReason: "waiting for partner API") {
			id status
		}
	}`
	resp := gqlPost(t, h, mutation)

	var data struct {
		BeadBlock struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"beadBlock"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if data.BeadBlock.Status != bead.StatusBlocked {
		t.Errorf("status: want %q, got %q", bead.StatusBlocked, data.BeadBlock.Status)
	}

	// Verify ExternalBlockerReason was persisted
	b, err := store.Get(context.Background(), openBead.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	reason, _ := b.Extra[bead.ExtraLifecycleExternalBlockerReason].(string)
	if reason == "" {
		t.Error("expected ExternalBlockerReason to be set")
	}

	// The bead should appear in beadsBlocked
	blockedQ := gqlPost(t, h, `{
		beadsBlocked(first: 10) { edges { node { id } } }
		beadsDependencyWaiting(first: 10) { edges { node { id } } }
	}`)
	var blockedData struct {
		BeadsBlocked struct {
			Edges []struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"beadsBlocked"`
		BeadsDependencyWaiting struct {
			Edges []struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"beadsDependencyWaiting"`
	}
	if err := json.Unmarshal(blockedQ["data"], &blockedData); err != nil {
		t.Fatalf("parse blocked data: %v", err)
	}
	foundInBlocked := false
	for _, e := range blockedData.BeadsBlocked.Edges {
		if e.Node.ID == openBead.ID {
			foundInBlocked = true
		}
	}
	if !foundInBlocked {
		t.Error("expected blocked bead to appear in beadsBlocked")
	}
	for _, e := range blockedData.BeadsDependencyWaiting.Edges {
		if e.Node.ID == openBead.ID {
			t.Error("blocked bead should NOT appear in beadsDependencyWaiting")
		}
	}
}

// TestGraphQLBeadBlockRequiresReason verifies beadBlock rejects empty reason.
func TestGraphQLBeadBlockRequiresReason(t *testing.T) {
	workDir, store := setupIntegrationDir(t)

	openBead := &bead.Bead{Title: "Open bead", Status: bead.StatusOpen}
	if err := store.Create(context.Background(), openBead); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	mutation := `mutation {
		beadBlock(id: "` + openBead.ID + `", externalBlockerReason: "") {
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
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["errors"] == nil {
		t.Error("expected error for empty externalBlockerReason, got none")
	}
}

// TestGraphQLBeadHumanResolveRemoved verifies that the old beadHumanResolve,
// HumanResolveAction, and beadsNeedsHuman symbols have been removed from the schema.
func TestGraphQLBeadHumanResolveRemoved(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)

	queries := []string{
		`{ beadsNeedsHuman(first: 1) { totalCount } }`,
		`mutation { beadHumanResolve(id: "x", action: retry, note: "test") { id } }`,
		`{ __type(name: "HumanResolveAction") { name } }`,
	}
	for _, q := range queries {
		rawBody, _ := json.Marshal(map[string]string{"query": q})
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		var resp map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("query %q: invalid JSON: %v", q, err)
		}

		// For __type query on a removed type, data.type should be null (not an error)
		if strings.Contains(q, "__type") {
			var typeData struct {
				Type *struct {
					Name string `json:"name"`
				} `json:"__type"`
			}
			if err := json.Unmarshal(resp["data"], &typeData); err != nil {
				t.Fatalf("query %q: parse data: %v", q, err)
			}
			if typeData.Type != nil {
				t.Errorf("query %q: expected __type to be null for removed HumanResolveAction, got %v", q, typeData.Type)
			}
			continue
		}
		if resp["errors"] == nil {
			t.Errorf("query %q: expected errors for removed symbol, got none (data=%s)", q, resp["data"])
		}
	}
}
