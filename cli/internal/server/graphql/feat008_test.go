package graphql_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
)

func TestIntegration_FEAT008BackendOperations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)

	docPath := filepath.Join(workDir, "docs", "palette-alpha.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("---\nddx:\n  id: palette-alpha\n---\n# Palette Alpha\n\nThis body must not be required for command palette indexing.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pluginRoot := filepath.Join(t.TempDir(), "local-ui")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "skills", "ui-polish"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "package.yaml"), []byte(`name: local-ui
version: 1.2.3
description: Local UI plugin
type: plugin
source: file://local-ui
api_version: "1"
keywords: [ui, local]
install:
  root:
    source: "."
    target: "~/.ddx/plugins/local-ui"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "skills", "ui-polish", "SKILL.md"), []byte("---\nname: ui-polish\ndescription: Polish UI\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := registry.SaveState(&registry.InstalledState{Installed: []registry.InstalledEntry{{
		Name:        "local-ui",
		Version:     "1.2.3",
		Type:        registry.PackageTypePlugin,
		Source:      "file://local-ui",
		InstalledAt: time.Now().UTC(),
		Files:       []string{pluginRoot},
	}}}); err != nil {
		t.Fatal(err)
	}

	ready := &bead.Bead{Title: "Palette ready bead", Status: bead.StatusOpen}
	if err := store.Create(ready); err != nil {
		t.Fatal(err)
	}
	dep := &bead.Bead{Title: "Blocking prerequisite", Status: bead.StatusOpen}
	if err := store.Create(dep); err != nil {
		t.Fatal(err)
	}
	blocked := &bead.Bead{Title: "Palette blocked bead", Status: bead.StatusOpen}
	if err := store.Create(blocked); err != nil {
		t.Fatal(err)
	}
	if err := store.DepAdd(blocked.ID, dep.ID); err != nil {
		t.Fatal(err)
	}
	inProgress := &bead.Bead{Title: "Running bead", Status: bead.StatusOpen}
	if err := store.Create(inProgress); err != nil {
		t.Fatal(err)
	}
	if err := store.Claim(inProgress.ID, "agent-01"); err != nil {
		t.Fatal(err)
	}
	closed := &bead.Bead{Title: "Closed evidence bead", Status: bead.StatusOpen}
	if err := store.Create(closed); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(closed.ID, bead.BeadEvent{
		Kind: "routing",
		Body: `{"resolved_provider":"openai","resolved_model":"gpt-5"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(closed.ID, bead.BeadEvent{
		Kind: "cost",
		Body: `{"attempt_id":"attempt-001","harness":"codex","provider":"openai","model":"gpt-5","input_tokens":1200,"output_tokens":450,"cost_usd":0.0123,"duration_ms":34000,"exit_code":0}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(closed.ID); err != nil {
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
		pluginDetail(name: "local-ui") { name version installedVersion type description keywords status registrySource diskBytes manifest skills prompts templates }
		projectBindings(projectId: "` + projectID + `")
		paletteSearch(query: "palette") {
			documents { kind path title }
			beads { kind id title }
			actions { kind id label }
			navigation { kind route title }
		}
		personas { name roles description body source bindings { projectId role persona } }
		bead(id: "` + ready.ID + `") { id title status }
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
			RowKey             string   `json:"rowKey"`
			Attempts           int      `json:"attempts"`
			Successes          int      `json:"successes"`
			MedianInputTokens  int      `json:"medianInputTokens"`
			MedianOutputTokens int      `json:"medianOutputTokens"`
			MedianDurationMs   int      `json:"medianDurationMs"`
			MedianCostUsd      *float64 `json:"medianCostUsd"`
		} `json:"efficacyRows"`
		EfficacyAttempts struct {
			Attempts []struct {
				BeadID            string   `json:"beadId"`
				Outcome           string   `json:"outcome"`
				DurationMs        int      `json:"durationMs"`
				CostUsd           *float64 `json:"costUsd"`
				EvidenceBundleURL string   `json:"evidenceBundleUrl"`
			} `json:"attempts"`
		} `json:"efficacyAttempts"`
		PluginsList []struct {
			Name string `json:"name"`
		} `json:"pluginsList"`
		PluginDetail struct {
			Name    string   `json:"name"`
			Status  string   `json:"status"`
			Skills  []string `json:"skills"`
			Version string   `json:"version"`
		} `json:"pluginDetail"`
		PaletteSearch struct {
			Documents []struct {
				Title string `json:"title"`
			} `json:"documents"`
			Beads []struct {
				ID string `json:"id"`
			} `json:"beads"`
		} `json:"paletteSearch"`
		Bead struct {
			ID string `json:"id"`
		} `json:"bead"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if data.QueueSummary.Ready < 2 {
		t.Fatalf("queueSummary.ready: want at least 2, got %d", data.QueueSummary.Ready)
	}
	if data.QueueSummary.Blocked != 1 {
		t.Fatalf("queueSummary.blocked: want 1, got %d", data.QueueSummary.Blocked)
	}
	if data.QueueSummary.InProgress != 1 {
		t.Fatalf("queueSummary.inProgress: want 1, got %d", data.QueueSummary.InProgress)
	}
	if len(data.EfficacyRows) == 0 {
		t.Fatal("expected efficacy rows from bead evidence")
	}
	row := data.EfficacyRows[0]
	if row.RowKey != "codex|openai|gpt-5" || row.Attempts != 1 || row.Successes != 1 || row.MedianInputTokens != 1200 || row.MedianOutputTokens != 450 || row.MedianDurationMs != 34000 {
		t.Fatalf("unexpected efficacy row: %+v", row)
	}
	if row.MedianCostUsd == nil || *row.MedianCostUsd != 0.0123 {
		t.Fatalf("unexpected median cost: %+v", row.MedianCostUsd)
	}
	if len(data.EfficacyAttempts.Attempts) != 1 || data.EfficacyAttempts.Attempts[0].BeadID != closed.ID || data.EfficacyAttempts.Attempts[0].Outcome != "succeeded" {
		t.Fatalf("unexpected efficacy attempts: %+v", data.EfficacyAttempts.Attempts)
	}
	if len(data.PluginsList) == 0 || data.PluginDetail.Name != "local-ui" || data.PluginDetail.Status != "installed" || data.PluginDetail.Version != "1.2.3" || len(data.PluginDetail.Skills) != 1 {
		t.Fatalf("expected installed plugin data, got %+v", data.PluginDetail)
	}
	if len(data.PaletteSearch.Documents) == 0 || len(data.PaletteSearch.Beads) == 0 {
		t.Fatalf("expected real palette document and bead results, got %+v", data.PaletteSearch)
	}
	if data.Bead.ID != ready.ID {
		t.Fatalf("bead(id): want %q, got %q", ready.ID, data.Bead.ID)
	}

	gqlPost(t, h, `mutation {
		workerDispatch(kind: "realign-specs", projectId: "`+projectID+`") { id state kind }
		pluginDispatch(name: "local-ui", action: "install", scope: "project") { id state action }
		comparisonDispatch(arms: [{ model: "gpt-5", prompt: "Summarize file X" }, { model: "claude-sonnet-4-6", prompt: "Summarize file X" }]) { id state armCount }
		personaBind(role: "code-reviewer", persona: "code-reviewer", projectId: "`+projectID+`") { ok role persona }
		beadClose(id: "`+ready.ID+`", reason: "done") { id status }
	}`)

	cfgData, err := os.ReadFile(filepath.Join(workDir, ".ddx", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfgData), "code-reviewer") {
		t.Fatalf("personaBind did not write .ddx/config.yaml: %s", cfgData)
	}
}
