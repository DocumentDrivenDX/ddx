package graphql_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// runsTestProvider extends a base testStateProvider with RunsStateProvider.
type runsTestProvider struct {
	*testStateProvider
	all []*ddxgraphql.Run
}

func (p *runsTestProvider) GetRunsGraphQL(projectID string, filter ddxgraphql.RunFilter) []*ddxgraphql.Run {
	var out []*ddxgraphql.Run
	for _, r := range p.all {
		if projectID != "" {
			// Match via beadId prefix for test fixture simplicity
			// (real impl uses project scope; here we allow all for empty projectID)
		}
		out = append(out, r)
	}
	return ddxgraphql.ApplyRunFilter(out, filter)
}

func (p *runsTestProvider) GetRunGraphQL(id string) (*ddxgraphql.Run, bool) {
	for _, r := range p.all {
		if r.ID == id {
			return r, true
		}
	}
	return nil, false
}

// makeRunWithHarness creates a run-layer Run with a custom harness.
func makeRunWithHarness(id string, layer ddxgraphql.RunLayer, status, harness string) *ddxgraphql.Run {
	r := makeRun(id, layer, status)
	if layer == ddxgraphql.RunLayerRun {
		r.Harness = &harness
	}
	return r
}

// makeRun creates a Run fixture for the given layer.
func makeRun(id string, layer ddxgraphql.RunLayer, status string) *ddxgraphql.Run {
	now := time.Now().UTC().Format(time.RFC3339)
	run := &ddxgraphql.Run{
		ID:          id,
		Layer:       layer,
		Status:      status,
		StartedAt:   &now,
		ChildRunIds: []string{},
	}
	switch layer {
	case ddxgraphql.RunLayerWork:
		qi := `{"beads":["ddx-001","ddx-002"]}`
		sc := "drain-all"
		run.QueueInputs = &qi
		run.StopCondition = &sc
		run.SelectedBeadIds = []string{"ddx-001", "ddx-002"}
		child1 := "try-001"
		run.ChildRunIds = []string{child1}
	case ddxgraphql.RunLayerTry:
		bead := "ddx-001"
		base := "abc123"
		result := "def456"
		wt := ".ddx/worktrees/try-001"
		mo := "merged"
		run.BeadID = &bead
		run.BaseRevision = &base
		run.ResultRevision = &result
		run.WorktreePath = &wt
		run.MergeOutcome = &mo
		parent := "work-001"
		run.ParentRunID = &parent
		child1 := "run-001"
		run.ChildRunIds = []string{child1}
	case ddxgraphql.RunLayerRun:
		bead := "ddx-001"
		harness := "claude"
		provider := "anthropic"
		model := "claude-sonnet-4-6"
		in := 1000
		out := 500
		dur := 12000
		cost := 0.015
		excerpt := "Task completed successfully."
		run.BeadID = &bead
		run.Harness = &harness
		run.Provider = &provider
		run.Model = &model
		run.TokensIn = &in
		run.TokensOut = &out
		run.DurationMs = &dur
		run.CostUsd = &cost
		run.OutputExcerpt = &excerpt
		run.EvidenceLinks = []string{".ddx/executions/20260501T010000-aabbccdd"}
		parent := "try-001"
		run.ParentRunID = &parent
	}
	return run
}

// TestIntegration_Query_Runs covers list, layer filter, detail, and parent/child paths.
func TestIntegration_Query_Runs(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	base := newTestStateProvider(workDir, store)
	projID := base.projects[0].ID
	if err := store.Create(&bead.Bead{ID: "ddx-001", Title: "Run detail audit target", Status: bead.StatusOpen}); err != nil {
		t.Fatalf("seed audit bead: %v", err)
	}

	workRun := makeRun("work-001", ddxgraphql.RunLayerWork, "success")
	tryRun := makeRun("try-001", ddxgraphql.RunLayerTry, "success")
	runRun := makeRun("run-001", ddxgraphql.RunLayerRun, "success")
	runFail := makeRunWithHarness("run-002", ddxgraphql.RunLayerRun, "failure", "codex")
	workRun.ProjectID = &projID
	tryRun.ProjectID = &projID
	runRun.ProjectID = &projID
	runFail.ProjectID = &projID

	provider := &runsTestProvider{
		testStateProvider: base,
		all:               []*ddxgraphql.Run{workRun, tryRun, runRun, runFail},
	}
	h := newGQLHandler(provider, workDir, nil)

	// ─── list all runs ─────────────────────────────────────────────────────
	resp := gqlPost(t, h, fmt.Sprintf(`{
		runs(projectID: %q, first: 50) {
			edges { node { id layer status childRunIds } cursor }
			pageInfo { hasNextPage }
			totalCount
		}
	}`, projID))
	var listOut struct {
		Runs struct {
			Edges []struct {
				Node struct {
					ID          string   `json:"id"`
					Layer       string   `json:"layer"`
					Status      string   `json:"status"`
					ChildRunIds []string `json:"childRunIds"`
				} `json:"node"`
				Cursor string `json:"cursor"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(resp["data"], &listOut); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if listOut.Runs.TotalCount != 4 {
		t.Fatalf("expected total 4, got %d", listOut.Runs.TotalCount)
	}

	// ─── layer filter: work ────────────────────────────────────────────────
	resp = gqlPost(t, h, fmt.Sprintf(`{
		runs(projectID: %q, layer: work, first: 50) { totalCount edges { node { id } } }
	}`, projID))
	var workOut struct {
		Runs struct {
			TotalCount int `json:"totalCount"`
			Edges      []struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(resp["data"], &workOut); err != nil {
		t.Fatalf("parse work filter: %v", err)
	}
	if workOut.Runs.TotalCount != 1 {
		t.Fatalf("expected 1 work run, got %d", workOut.Runs.TotalCount)
	}
	if workOut.Runs.Edges[0].Node.ID != "work-001" {
		t.Fatalf("expected work-001, got %q", workOut.Runs.Edges[0].Node.ID)
	}

	// ─── layer filter: try ─────────────────────────────────────────────────
	resp = gqlPost(t, h, fmt.Sprintf(`{
		runs(projectID: %q, layer: try, first: 50) { totalCount }
	}`, projID))
	var tryOut struct {
		Runs struct {
			TotalCount int `json:"totalCount"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(resp["data"], &tryOut); err != nil {
		t.Fatalf("parse try filter: %v", err)
	}
	if tryOut.Runs.TotalCount != 1 {
		t.Fatalf("expected 1 try run, got %d", tryOut.Runs.TotalCount)
	}

	// ─── layer filter: run ─────────────────────────────────────────────────
	resp = gqlPost(t, h, fmt.Sprintf(`{
		runs(projectID: %q, layer: run, first: 50) { totalCount }
	}`, projID))
	var runLayerOut struct {
		Runs struct {
			TotalCount int `json:"totalCount"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(resp["data"], &runLayerOut); err != nil {
		t.Fatalf("parse run filter: %v", err)
	}
	if runLayerOut.Runs.TotalCount != 2 {
		t.Fatalf("expected 2 run-layer runs, got %d", runLayerOut.Runs.TotalCount)
	}

	// ─── detail: work layer ────────────────────────────────────────────────
	resp = gqlPost(t, h, `{
		run(id: "work-001") {
			id layer status childRunIds queueInputs stopCondition selectedBeadIds
		}
	}`)
	var workDetailOut struct {
		Run struct {
			ID              string   `json:"id"`
			Layer           string   `json:"layer"`
			Status          string   `json:"status"`
			ChildRunIds     []string `json:"childRunIds"`
			QueueInputs     *string  `json:"queueInputs"`
			StopCondition   *string  `json:"stopCondition"`
			SelectedBeadIds []string `json:"selectedBeadIds"`
		} `json:"run"`
	}
	if err := json.Unmarshal(resp["data"], &workDetailOut); err != nil {
		t.Fatalf("parse work detail: %v", err)
	}
	if workDetailOut.Run.ID != "work-001" {
		t.Fatalf("expected work-001, got %q", workDetailOut.Run.ID)
	}
	if workDetailOut.Run.Layer != "work" {
		t.Fatalf("expected layer=work, got %q", workDetailOut.Run.Layer)
	}
	if len(workDetailOut.Run.ChildRunIds) != 1 || workDetailOut.Run.ChildRunIds[0] != "try-001" {
		t.Fatalf("expected childRunIds=[try-001], got %v", workDetailOut.Run.ChildRunIds)
	}
	if workDetailOut.Run.QueueInputs == nil {
		t.Fatal("expected queueInputs to be non-nil for work layer")
	}
	if workDetailOut.Run.StopCondition == nil || *workDetailOut.Run.StopCondition != "drain-all" {
		t.Fatalf("expected stopCondition=drain-all, got %v", workDetailOut.Run.StopCondition)
	}

	// ─── detail: try layer ─────────────────────────────────────────────────
	resp = gqlPost(t, h, `{
		run(id: "try-001") {
			id layer status parentRunId childRunIds
			baseRevision resultRevision worktreePath mergeOutcome
		}
	}`)
	var tryDetailOut struct {
		Run struct {
			ID             string   `json:"id"`
			Layer          string   `json:"layer"`
			Status         string   `json:"status"`
			ParentRunID    *string  `json:"parentRunId"`
			ChildRunIds    []string `json:"childRunIds"`
			BaseRevision   *string  `json:"baseRevision"`
			ResultRevision *string  `json:"resultRevision"`
			WorktreePath   *string  `json:"worktreePath"`
			MergeOutcome   *string  `json:"mergeOutcome"`
		} `json:"run"`
	}
	if err := json.Unmarshal(resp["data"], &tryDetailOut); err != nil {
		t.Fatalf("parse try detail: %v", err)
	}
	if tryDetailOut.Run.Layer != "try" {
		t.Fatalf("expected layer=try, got %q", tryDetailOut.Run.Layer)
	}
	if tryDetailOut.Run.ParentRunID == nil || *tryDetailOut.Run.ParentRunID != "work-001" {
		t.Fatalf("expected parentRunId=work-001, got %v", tryDetailOut.Run.ParentRunID)
	}
	if len(tryDetailOut.Run.ChildRunIds) != 1 || tryDetailOut.Run.ChildRunIds[0] != "run-001" {
		t.Fatalf("expected childRunIds=[run-001], got %v", tryDetailOut.Run.ChildRunIds)
	}
	if tryDetailOut.Run.BaseRevision == nil || *tryDetailOut.Run.BaseRevision != "abc123" {
		t.Fatalf("expected baseRevision=abc123, got %v", tryDetailOut.Run.BaseRevision)
	}

	// ─── detail: run layer ─────────────────────────────────────────────────
	resp = gqlPost(t, h, `{
		run(id: "run-001") {
			id layer status parentRunId
			harness provider model tokensIn tokensOut costUsd durationMs
			outputExcerpt evidenceLinks
		}
	}`)
	var runDetailOut struct {
		Run struct {
			ID            string   `json:"id"`
			Layer         string   `json:"layer"`
			Status        string   `json:"status"`
			ParentRunID   *string  `json:"parentRunId"`
			Harness       *string  `json:"harness"`
			Provider      *string  `json:"provider"`
			Model         *string  `json:"model"`
			TokensIn      *int     `json:"tokensIn"`
			TokensOut     *int     `json:"tokensOut"`
			CostUsd       *float64 `json:"costUsd"`
			DurationMs    *int     `json:"durationMs"`
			OutputExcerpt *string  `json:"outputExcerpt"`
			EvidenceLinks []string `json:"evidenceLinks"`
		} `json:"run"`
	}
	if err := json.Unmarshal(resp["data"], &runDetailOut); err != nil {
		t.Fatalf("parse run detail: %v", err)
	}
	if runDetailOut.Run.Layer != "run" {
		t.Fatalf("expected layer=run, got %q", runDetailOut.Run.Layer)
	}
	if runDetailOut.Run.Harness == nil || *runDetailOut.Run.Harness != "claude" {
		t.Fatalf("expected harness=claude, got %v", runDetailOut.Run.Harness)
	}
	if runDetailOut.Run.DurationMs == nil || *runDetailOut.Run.DurationMs != 12000 {
		t.Fatalf("expected durationMs=12000, got %v", runDetailOut.Run.DurationMs)
	}
	if len(runDetailOut.Run.EvidenceLinks) != 1 {
		t.Fatalf("expected 1 evidence link, got %d", len(runDetailOut.Run.EvidenceLinks))
	}

	events, err := store.Events("ddx-001")
	if err != nil {
		t.Fatalf("get audit events: %v", err)
	}
	seen := map[string]bool{
		"run_id=try-001": false,
		"run_id=run-001": false,
	}
	count := 0
	for _, ev := range events {
		if ev.Kind != "run_detail_view" {
			continue
		}
		count++
		for token := range seen {
			if strings.Contains(ev.Body, token) {
				seen[token] = true
			}
		}
		if !strings.Contains(ev.Body, "visibility=project_membership") {
			t.Fatalf("expected visibility marker in audit body, got %q", ev.Body)
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 run_detail_view events, got %d", count)
	}
	for token, ok := range seen {
		if !ok {
			t.Fatalf("missing audit event body token %q", token)
		}
	}

	// ─── missing id returns nil (not error) ──────────────────────────────
	// gqlPost already fails the test on GraphQL errors; a nil Run with no
	// errors satisfies AC "returns nil not error".
	resp = gqlPost(t, h, `{ run(id: "does-not-exist") { id } }`)
	var missingOut struct {
		Run *struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(resp["data"], &missingOut); err != nil {
		t.Fatalf("parse missing run: %v", err)
	}
	if missingOut.Run != nil {
		t.Fatalf("expected nil for missing run, got %+v", missingOut.Run)
	}
}

func TestAuditEvent_RawTranscriptViewed(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	base := newTestStateProvider(workDir, store)
	projID := base.projects[0].ID
	beadID := "ddx-raw-transcript"
	if err := store.Create(&bead.Bead{ID: beadID, Title: "Raw transcript audit target", Status: bead.StatusOpen}); err != nil {
		t.Fatalf("seed audit bead: %v", err)
	}

	run := makeRun("run-raw-001", ddxgraphql.RunLayerRun, "success")
	run.ProjectID = &projID
	run.BeadID = &beadID
	prompt := "prompt body"
	response := "response body"
	stderr := "stderr body"
	run.Prompt = &prompt
	run.Response = &response
	run.Stderr = &stderr

	provider := &runsTestProvider{
		testStateProvider: base,
		all:               []*ddxgraphql.Run{run},
	}
	h := newGQLHandler(provider, workDir, nil)

	resp := gqlPost(t, h, `{ run(id: "run-raw-001") { id layer status artifactId } }`)
	var headerOut struct {
		Run *struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(resp["data"], &headerOut); err != nil {
		t.Fatalf("parse header query: %v", err)
	}
	if headerOut.Run == nil || headerOut.Run.ID != "run-raw-001" {
		t.Fatalf("expected header query to resolve run, got %+v", headerOut.Run)
	}
	events, err := store.EventsByKind(beadID, "raw_transcript_viewed")
	if err != nil {
		t.Fatalf("get raw transcript events after header query: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no raw_transcript_viewed events before transcript access, got %d", len(events))
	}

	resp = gqlPost(t, h, `{ run(id: "run-raw-001") { id prompt response stderr } }`)
	var transcriptOut struct {
		Run *struct {
			ID       string  `json:"id"`
			Prompt   *string `json:"prompt"`
			Response *string `json:"response"`
			Stderr   *string `json:"stderr"`
		} `json:"run"`
	}
	if err := json.Unmarshal(resp["data"], &transcriptOut); err != nil {
		t.Fatalf("parse transcript query: %v", err)
	}
	if transcriptOut.Run == nil || transcriptOut.Run.Prompt == nil || transcriptOut.Run.Response == nil || transcriptOut.Run.Stderr == nil {
		t.Fatalf("expected transcript query to return prompt/response/stderr, got %+v", transcriptOut.Run)
	}

	events, err = store.EventsByKind(beadID, "raw_transcript_viewed")
	if err != nil {
		t.Fatalf("get raw transcript events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 raw_transcript_viewed event, got %d", len(events))
	}
	if !strings.Contains(events[0].Body, "run_id=run-raw-001") {
		t.Fatalf("raw transcript audit body missing run id: %q", events[0].Body)
	}
	if !strings.Contains(events[0].Body, "fields=prompt,response,stderr") {
		t.Fatalf("raw transcript audit body missing transcript fields: %q", events[0].Body)
	}
	if !strings.Contains(events[0].Body, "visibility=project_membership") {
		t.Fatalf("raw transcript audit body missing visibility marker: %q", events[0].Body)
	}
}

// TestRunFilter_LayerFilter covers applyRunFilter edge cases.
func TestRunFilter_LayerFilter(t *testing.T) {
	work := ddxgraphql.RunLayerWork
	try := ddxgraphql.RunLayerTry
	run := ddxgraphql.RunLayerRun

	runs := []*ddxgraphql.Run{
		makeRun("work-001", ddxgraphql.RunLayerWork, "success"),
		makeRun("try-001", ddxgraphql.RunLayerTry, "success"),
		makeRun("run-001", ddxgraphql.RunLayerRun, "success"),
		makeRunWithHarness("run-002", ddxgraphql.RunLayerRun, "failure", "codex"),
	}

	cases := []struct {
		name    string
		filter  ddxgraphql.RunFilter
		wantLen int
		wantIDs []string
	}{
		{"no filter", ddxgraphql.RunFilter{}, 4, nil},
		{"layer=work", ddxgraphql.RunFilter{Layer: &work}, 1, []string{"work-001"}},
		{"layer=try", ddxgraphql.RunFilter{Layer: &try}, 1, []string{"try-001"}},
		{"layer=run", ddxgraphql.RunFilter{Layer: &run}, 2, nil},
		{"status=failure", ddxgraphql.RunFilter{Status: "failure"}, 1, []string{"run-002"}},
		{"status=success", ddxgraphql.RunFilter{Status: "success"}, 3, nil},
		{"harness=claude", ddxgraphql.RunFilter{Harness: "claude"}, 1, []string{"run-001"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ddxgraphql.ApplyRunFilter(runs, tc.filter)
			if len(got) != tc.wantLen {
				t.Fatalf("filter %q: expected %d runs, got %d", tc.name, tc.wantLen, len(got))
			}
			for _, wantID := range tc.wantIDs {
				found := false
				for _, r := range got {
					if r.ID == wantID {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("filter %q: expected run %q in result", tc.name, wantID)
				}
			}
		})
	}
}
