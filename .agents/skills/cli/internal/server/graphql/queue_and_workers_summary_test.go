package graphql_test

import (
	"encoding/json"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// TestGraphQLQueueAndWorkersSummary covers the resolver backing the global
// drain indicator (ddx-b6cf025c). Verifies:
//   - zeros for unknown project (no error)
//   - ready-bead count mirrors the bead store's Ready() result
//   - running-worker count filters by state == "running"
func TestGraphQLQueueAndWorkersSummary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)

	ready := &bead.Bead{Title: "ready bead", Status: bead.StatusOpen}
	if err := store.Create(ready); err != nil {
		t.Fatal(err)
	}
	second := &bead.Bead{Title: "also ready", Status: bead.StatusOpen}
	if err := store.Create(second); err != nil {
		t.Fatal(err)
	}
	dep := &bead.Bead{Title: "dep", Status: bead.StatusOpen}
	if err := store.Create(dep); err != nil {
		t.Fatal(err)
	}
	blocked := &bead.Bead{Title: "blocked", Status: bead.StatusOpen}
	if err := store.Create(blocked); err != nil {
		t.Fatal(err)
	}
	if err := store.DepAdd(blocked.ID, dep.ID); err != nil {
		t.Fatal(err)
	}

	state := newTestStateProvider(workDir, store)
	projectID := state.projects[0].ID
	state.workers = map[string][]*ddxgraphql.Worker{
		projectID: {
			{ID: "w1", State: "running", Kind: "execute-loop"},
			{ID: "w2", State: "running", Kind: "execute-loop"},
			{ID: "w3", State: "stopped", Kind: "execute-loop"},
		},
	}
	h := newGQLHandler(state, workDir, nil)

	t.Run("known project", func(t *testing.T) {
		resp := gqlPost(t, h, `{
			queueAndWorkersSummary(projectId: "`+projectID+`") {
				readyBeads
				runningWorkers
				totalWorkers
			}
		}`)
		var data struct {
			QueueAndWorkersSummary struct {
				ReadyBeads     int `json:"readyBeads"`
				RunningWorkers int `json:"runningWorkers"`
				TotalWorkers   int `json:"totalWorkers"`
			} `json:"queueAndWorkersSummary"`
		}
		if err := json.Unmarshal(resp["data"], &data); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := data.QueueAndWorkersSummary
		if got.ReadyBeads != 3 {
			t.Errorf("readyBeads: want 3, got %d", got.ReadyBeads)
		}
		if got.RunningWorkers != 2 {
			t.Errorf("runningWorkers: want 2, got %d", got.RunningWorkers)
		}
		if got.TotalWorkers != 3 {
			t.Errorf("totalWorkers: want 3, got %d", got.TotalWorkers)
		}
	})

	t.Run("unknown project returns zeros, not error", func(t *testing.T) {
		resp := gqlPost(t, h, `{
			queueAndWorkersSummary(projectId: "proj-missing") {
				readyBeads
				runningWorkers
				totalWorkers
			}
		}`)
		if raw, ok := resp["errors"]; ok && len(raw) > 0 && string(raw) != "null" {
			t.Fatalf("unexpected errors: %s", string(raw))
		}
		var data struct {
			QueueAndWorkersSummary struct {
				ReadyBeads     int `json:"readyBeads"`
				RunningWorkers int `json:"runningWorkers"`
				TotalWorkers   int `json:"totalWorkers"`
			} `json:"queueAndWorkersSummary"`
		}
		if err := json.Unmarshal(resp["data"], &data); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := data.QueueAndWorkersSummary
		if got.ReadyBeads != 0 || got.RunningWorkers != 0 || got.TotalWorkers != 0 {
			t.Fatalf("want zeros, got %+v", got)
		}
	})
}
