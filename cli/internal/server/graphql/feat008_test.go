package graphql_test

import (
	"encoding/json"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func TestIntegration_FEAT008StubOperations(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	b := &bead.Bead{Title: "FEAT-008 fixture", Status: bead.StatusOpen}
	if err := store.Create(b); err != nil {
		t.Fatal(err)
	}
	state := newTestStateProvider(workDir, store)
	projectID := state.projects[0].ID
	h := newGQLHandler(state, workDir, nil)

	query := `{
		queueSummary(projectId: "` + projectID + `") { ready blocked inProgress }
		efficacyRows { rowKey harness provider model attempts successes successRate medianInputTokens medianOutputTokens medianDurationMs medianCostUsd warning { kind threshold } }
		efficacyAttempts(rowKey: "codex|openai|gpt-5") { rowKey attempts { beadId outcome durationMs costUsd evidenceBundleUrl } }
		comparisons { id state armCount }
		pluginsList { name version installedVersion type description keywords status registrySource diskBytes manifest skills prompts templates }
		pluginDetail(name: "helix") { name version installedVersion type description keywords status registrySource diskBytes manifest skills prompts templates }
		projectBindings(projectId: "` + projectID + `")
		paletteSearch(query: "feat") {
			documents { kind path title }
			beads { kind id title }
			actions { kind id label }
			navigation { kind route title }
		}
		personas { name roles description body source bindings { projectId role persona } }
		bead(id: "` + b.ID + `") { id title status }
		worker(id: "worker-missing") { id recentEvents { kind text name inputs output } }
	}`
	resp := gqlPost(t, h, query)

	var data struct {
		QueueSummary struct {
			Ready      int `json:"ready"`
			Blocked    int `json:"blocked"`
			InProgress int `json:"inProgress"`
		} `json:"queueSummary"`
		EfficacyRows []struct {
			RowKey string `json:"rowKey"`
		} `json:"efficacyRows"`
		PluginsList []struct {
			Name string `json:"name"`
		} `json:"pluginsList"`
		PluginDetail struct {
			Name string `json:"name"`
		} `json:"pluginDetail"`
		PaletteSearch struct {
			Documents []struct {
				Title string `json:"title"`
			} `json:"documents"`
		} `json:"paletteSearch"`
		Bead struct {
			ID string `json:"id"`
		} `json:"bead"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if data.QueueSummary.Ready != 1 {
		t.Fatalf("queueSummary.ready: want 1, got %d", data.QueueSummary.Ready)
	}
	if len(data.EfficacyRows) == 0 {
		t.Fatal("expected fixture efficacy rows")
	}
	if len(data.PluginsList) == 0 || data.PluginDetail.Name != "helix" {
		t.Fatal("expected fixture plugin data")
	}
	if len(data.PaletteSearch.Documents) == 0 {
		t.Fatal("expected fixture palette results")
	}
	if data.Bead.ID != b.ID {
		t.Fatalf("bead(id): want %q, got %q", b.ID, data.Bead.ID)
	}

	gqlPost(t, h, `mutation {
		workerDispatch(kind: "execute-loop", projectId: "`+projectID+`") { id state kind }
		pluginDispatch(name: "frontend-design", action: "install", scope: "project") { id state action }
		comparisonDispatch(arms: [{ model: "gpt-5", prompt: "Summarize file X" }, { model: "claude-sonnet-4-6", prompt: "Summarize file X" }]) { id state armCount }
		personaBind(role: "code-reviewer", persona: "code-reviewer", projectId: "`+projectID+`") { ok role persona }
		beadClose(id: "`+b.ID+`", reason: "done") { id status }
	}`)
}
