package graphql_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// executionsTestProvider extends a base testStateProvider with the optional
// ExecutionsStateProvider methods so the executions/execution/executionToolCalls
// resolvers route to in-memory data.
type executionsTestProvider struct {
	*testStateProvider
	all   []*ddxgraphql.Execution
	calls map[string][]*ddxgraphql.ExecutionToolCall
}

func (p *executionsTestProvider) GetExecutionsGraphQL(projectID string, filter ddxgraphql.ExecutionFilter) []*ddxgraphql.Execution {
	if projectID == "" {
		return nil
	}
	out := make([]*ddxgraphql.Execution, 0, len(p.all))
	for _, e := range p.all {
		if e.ProjectID != projectID {
			continue
		}
		if filter.BeadID != "" && (e.BeadID == nil || *e.BeadID != filter.BeadID) {
			continue
		}
		if filter.Verdict != "" && (e.Verdict == nil || !strings.EqualFold(*e.Verdict, filter.Verdict)) {
			continue
		}
		if filter.Harness != "" && (e.Harness == nil || *e.Harness != filter.Harness) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (p *executionsTestProvider) GetExecutionGraphQL(id string) (*ddxgraphql.Execution, bool) {
	for _, e := range p.all {
		if e.ID == id {
			return e, true
		}
	}
	return nil, false
}

func (p *executionsTestProvider) GetExecutionToolCallsGraphQL(id string) []*ddxgraphql.ExecutionToolCall {
	return p.calls[id]
}

func makeExecution(projectID, id, beadID, harness, verdict string) *ddxgraphql.Execution {
	bID := beadID
	h := harness
	v := verdict
	dur := 1500
	cost := 0.012
	tokens := 880
	prompt := fmt.Sprintf("prompt body for %s", id)
	manifest := fmt.Sprintf(`{"bead_id":"%s","attempt_id":"%s"}`, beadID, id)
	result := fmt.Sprintf(`{"verdict":"%s","rationale":"reason for %s"}`, verdict, id)
	return &ddxgraphql.Execution{
		ID:         id,
		ProjectID:  projectID,
		BeadID:     &bID,
		Harness:    &h,
		Verdict:    &v,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		BundlePath: ".ddx/executions/" + id,
		DurationMs: &dur,
		CostUsd:    &cost,
		Tokens:     &tokens,
		Prompt:     &prompt,
		Manifest:   &manifest,
		Result:     &result,
	}
}

func writeExecutionFixture(t *testing.T, projectRoot, id, bead string) {
	t.Helper()
	dir := filepath.Join(projectRoot, ".ddx", "executions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"attempt_id": id, "bead_id": bead, "created_at": time.Now().UTC().Format(time.RFC3339)}
	mb, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"verdict":"PASS"}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("hi"), 0o644)
}

// TestIntegration_Query_Executions covers list, detail, and tool-call paths.
func TestIntegration_Query_Executions(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	base := newTestStateProvider(workDir, store)
	projID := base.projects[0].ID

	// Seed in-process executions via the test provider.
	provider := &executionsTestProvider{
		testStateProvider: base,
		all: []*ddxgraphql.Execution{
			makeExecution(projID, "20260423T010000-aaaa1111", "ddx-001", "claude", "PASS"),
			makeExecution(projID, "20260423T020000-bbbb2222", "ddx-002", "codex", "BLOCK"),
			makeExecution(projID, "20260423T030000-cccc3333", "ddx-001", "claude", "BLOCK"),
		},
		calls: map[string][]*ddxgraphql.ExecutionToolCall{},
	}
	for i := 0; i < 60; i++ {
		seq := i
		name := "Bash"
		input := fmt.Sprintf(`{"command":"echo %d"}`, i)
		out := fmt.Sprintf("step %d", i)
		provider.calls["20260423T020000-bbbb2222"] = append(provider.calls["20260423T020000-bbbb2222"], &ddxgraphql.ExecutionToolCall{
			ID:     fmt.Sprintf("tc-%d", seq),
			Name:   name,
			Seq:    seq,
			Inputs: &input,
			Output: &out,
		})
	}
	h := newGQLHandler(provider, workDir, nil)

	// ─── list ────────────────────────────────────────────────────────────────
	resp := gqlPost(t, h, fmt.Sprintf(`{
		executions(projectId: %q, first: 50) {
			edges { node { id beadId verdict harness } cursor }
			pageInfo { hasNextPage }
			totalCount
		}
	}`, projID))
	var listOut struct {
		Executions struct {
			Edges []struct {
				Node struct {
					ID      string  `json:"id"`
					BeadID  *string `json:"beadId"`
					Verdict *string `json:"verdict"`
					Harness *string `json:"harness"`
				} `json:"node"`
				Cursor string `json:"cursor"`
			} `json:"edges"`
			TotalCount int `json:"totalCount"`
		} `json:"executions"`
	}
	if err := json.Unmarshal(resp["data"], &listOut); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if listOut.Executions.TotalCount != 3 {
		t.Fatalf("expected total 3, got %d", listOut.Executions.TotalCount)
	}

	// ─── filter by verdict ───────────────────────────────────────────────────
	resp = gqlPost(t, h, fmt.Sprintf(`{
		executions(projectId: %q, verdict: "BLOCK", first: 50) { totalCount }
	}`, projID))
	var verdictOut struct {
		Executions struct {
			TotalCount int `json:"totalCount"`
		} `json:"executions"`
	}
	_ = json.Unmarshal(resp["data"], &verdictOut)
	if verdictOut.Executions.TotalCount != 2 {
		t.Fatalf("expected 2 BLOCK executions, got %d", verdictOut.Executions.TotalCount)
	}

	// ─── detail ──────────────────────────────────────────────────────────────
	resp = gqlPost(t, h, `{
		execution(id: "20260423T020000-bbbb2222") { id verdict prompt manifest result }
	}`)
	var detailOut struct {
		Execution struct {
			ID       string  `json:"id"`
			Verdict  *string `json:"verdict"`
			Prompt   *string `json:"prompt"`
			Manifest *string `json:"manifest"`
			Result   *string `json:"result"`
		} `json:"execution"`
	}
	_ = json.Unmarshal(resp["data"], &detailOut)
	if detailOut.Execution.ID != "20260423T020000-bbbb2222" {
		t.Fatalf("expected detail id, got %q", detailOut.Execution.ID)
	}
	if detailOut.Execution.Prompt == nil || *detailOut.Execution.Prompt == "" {
		t.Fatal("expected prompt body")
	}
	if detailOut.Execution.Manifest == nil || !strings.Contains(*detailOut.Execution.Manifest, "bbbb2222") {
		t.Fatal("expected manifest body")
	}

	// ─── tool calls (first page) ─────────────────────────────────────────────
	resp = gqlPost(t, h, `{
		executionToolCalls(id: "20260423T020000-bbbb2222", first: 50) {
			edges { node { id name seq } cursor }
			pageInfo { hasNextPage endCursor }
			totalCount
		}
	}`)
	var toolOut struct {
		ExecutionToolCalls struct {
			Edges []struct {
				Node struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Seq  int    `json:"seq"`
				} `json:"node"`
				Cursor string `json:"cursor"`
			} `json:"edges"`
			PageInfo struct {
				HasNextPage bool    `json:"hasNextPage"`
				EndCursor   *string `json:"endCursor"`
			} `json:"pageInfo"`
			TotalCount int `json:"totalCount"`
		} `json:"executionToolCalls"`
	}
	_ = json.Unmarshal(resp["data"], &toolOut)
	if toolOut.ExecutionToolCalls.TotalCount != 60 {
		t.Fatalf("expected 60 tool calls, got %d", toolOut.ExecutionToolCalls.TotalCount)
	}
	if len(toolOut.ExecutionToolCalls.Edges) != 50 {
		t.Fatalf("expected 50 edges in first page, got %d", len(toolOut.ExecutionToolCalls.Edges))
	}
	if !toolOut.ExecutionToolCalls.PageInfo.HasNextPage {
		t.Fatal("expected hasNextPage=true after 50 of 60")
	}
	// Don't reference workDir / writeExecutionFixture in this test path; this
	// keeps the on-disk loader behavior covered by state_executions_test.go.
	_ = writeExecutionFixture
}
