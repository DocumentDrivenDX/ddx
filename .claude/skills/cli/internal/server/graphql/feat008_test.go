package graphql_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
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
	started := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	if err := agent.AppendSessionIndex(agent.SessionLogDirForWorkDir(workDir), agent.SessionIndexEntry{
		ID:           "session-closed",
		ProjectID:    agent.ProjectIDForPath(workDir),
		BeadID:       closed.ID,
		Harness:      "codex",
		Provider:     "openai",
		Model:        "gpt-5",
		StartedAt:    started,
		EndedAt:      started.Add(34 * time.Second),
		DurationMS:   34000,
		CostUSD:      0.0123,
		CostPresent:  true,
		InputTokens:  1200,
		OutputTokens: 450,
		Outcome:      "success",
		BundlePath:   filepath.ToSlash(filepath.Join(".ddx", "executions", "attempt-001")),
	}, started); err != nil {
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
		t.Fatal("expected efficacy rows from session index")
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

	mutationResp := gqlPost(t, h, `mutation {
		workerDispatch(kind: "realign-specs", projectId: "`+projectID+`") { id state kind }
		pluginDispatch(name: "local-ui", action: "install", scope: "project") { id state action }
		comparisonDispatch(arms: [{ model: "gpt-5", prompt: "Summarize file X" }, { model: "claude-sonnet-4-6", prompt: "Summarize file X" }]) { id state armCount }
		personaBind(role: "code-reviewer", persona: "code-reviewer", projectId: "`+projectID+`") { ok role persona }
		beadClose(id: "`+ready.ID+`", reason: "done") { id status }
	}`)
	var mutationData struct {
		PluginDispatch struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Action string `json:"action"`
		} `json:"pluginDispatch"`
		ComparisonDispatch struct {
			ID       string `json:"id"`
			State    string `json:"state"`
			ArmCount int    `json:"armCount"`
		} `json:"comparisonDispatch"`
	}
	if err := json.Unmarshal(mutationResp["data"], &mutationData); err != nil {
		t.Fatalf("parse mutation data: %v", err)
	}
	if strings.HasPrefix(mutationData.PluginDispatch.ID, "queued-plugin-") || mutationData.PluginDispatch.State != "installed" || mutationData.PluginDispatch.Action != "install" {
		t.Fatalf("pluginDispatch did not run the real plugin path: %+v", mutationData.PluginDispatch)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".ddx", "plugin-dispatches", mutationData.PluginDispatch.ID+".json")); err != nil {
		t.Fatalf("pluginDispatch did not write an audit record: %v", err)
	}
	if strings.HasPrefix(mutationData.ComparisonDispatch.ID, "queued-comparison-") || mutationData.ComparisonDispatch.State != "queued" || mutationData.ComparisonDispatch.ArmCount != 2 {
		t.Fatalf("comparisonDispatch did not create a real queued record: %+v", mutationData.ComparisonDispatch)
	}
	comparisonPath := filepath.Join(workDir, ".ddx", "comparisons", mutationData.ComparisonDispatch.ID+".json")
	if _, err := os.Stat(comparisonPath); err != nil {
		t.Fatalf("comparisonDispatch did not write a comparison record: %v", err)
	}

	comparisonResp := gqlPost(t, h, `{ comparisons { id state armCount } }`)
	var comparisonData struct {
		Comparisons []struct {
			ID       string `json:"id"`
			State    string `json:"state"`
			ArmCount int    `json:"armCount"`
		} `json:"comparisons"`
	}
	if err := json.Unmarshal(comparisonResp["data"], &comparisonData); err != nil {
		t.Fatalf("parse comparisons data: %v", err)
	}
	if len(comparisonData.Comparisons) != 1 || comparisonData.Comparisons[0].ID != mutationData.ComparisonDispatch.ID || comparisonData.Comparisons[0].ArmCount != 2 {
		t.Fatalf("comparisons did not read back dispatched record: %+v", comparisonData.Comparisons)
	}

	cfgData, err := os.ReadFile(filepath.Join(workDir, ".ddx", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfgData), "code-reviewer") {
		t.Fatalf("personaBind did not write .ddx/config.yaml: %s", cfgData)
	}
}
