package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// seedBundle writes a single execute-bead bundle into projectRoot.
func seedBundle(t *testing.T, projectRoot, attemptID, beadID, harness, verdict string, withToolCalls bool) {
	t.Helper()
	bundleDir := filepath.Join(projectRoot, agent.ExecuteBeadArtifactDir, attemptID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Derive created_at from the attempt-id timestamp so ordering tests can
	// rely on the bundle id prefix.
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if t2, err := time.Parse("20060102T150405", strings.SplitN(attemptID, "-", 2)[0]); err == nil {
		createdAt = t2.UTC().Format(time.RFC3339)
	}
	manifest := map[string]any{
		"attempt_id": attemptID,
		"bead_id":    beadID,
		"base_rev":   "abcdef1234567890",
		"created_at": createdAt,
		"requested":  map[string]string{"harness": harness, "model": "claude-sonnet-4-6"},
		"bead":       map[string]string{"id": beadID, "title": "Test bead " + beadID},
		"paths": map[string]string{
			"dir":      filepath.Join(agent.ExecuteBeadArtifactDir, attemptID),
			"prompt":   filepath.Join(agent.ExecuteBeadArtifactDir, attemptID, "prompt.md"),
			"manifest": filepath.Join(agent.ExecuteBeadArtifactDir, attemptID, "manifest.json"),
			"result":   filepath.Join(agent.ExecuteBeadArtifactDir, attemptID, "result.json"),
		},
	}
	mb, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
	result := map[string]any{
		"bead_id":     beadID,
		"attempt_id":  attemptID,
		"verdict":     verdict,
		"outcome":     verdict,
		"status":      strings.ToLower(verdict),
		"rationale":   "rationale text for " + attemptID,
		"harness":     harness,
		"session_id":  "eb-" + attemptID,
		"duration_ms": 12345,
		"cost_usd":    0.42,
		"tokens":      1234,
		"exit_code":   0,
		"started_at":  time.Now().UTC().Format(time.RFC3339),
		"finished_at": time.Now().UTC().Add(12 * time.Second).Format(time.RFC3339),
	}
	rb, _ := json.Marshal(result)
	if err := os.WriteFile(filepath.Join(bundleDir, "result.json"), rb, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "prompt.md"), []byte("prompt body for "+attemptID), 0o644); err != nil {
		t.Fatal(err)
	}
	if withToolCalls {
		embedded := filepath.Join(bundleDir, "embedded")
		if err := os.MkdirAll(embedded, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(embedded, "agent-eb-"+attemptID+".jsonl")
		var sb strings.Builder
		for i := 0; i < 50; i++ {
			frame := map[string]any{
				"kind":   "tool_call",
				"name":   "Bash",
				"ts":     time.Now().UTC().Format(time.RFC3339),
				"inputs": map[string]string{"command": fmt.Sprintf("echo %d", i)},
				"output": fmt.Sprintf("step-%d-output", i),
			}
			b, _ := json.Marshal(frame)
			sb.Write(b)
			sb.WriteByte('\n')
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newServerStateForTest(t *testing.T, projectRoot string) *ServerState {
	t.Helper()
	return &ServerState{
		Projects: []ProjectEntry{{
			ID:   "proj-test",
			Name: "test-project",
			Path: projectRoot,
		}},
		workingDir: projectRoot,
	}
}

func TestExecutions_ListAndDetail(t *testing.T) {
	root := t.TempDir()
	seedBundle(t, root, "20260423T010000-aaaa1111", "ddx-001", "claude", "PASS", false)
	seedBundle(t, root, "20260423T020000-bbbb2222", "ddx-002", "codex", "BLOCK", true)
	seedBundle(t, root, "20260423T030000-cccc3333", "ddx-001", "claude", "BLOCK", false)

	s := newServerStateForTest(t, root)

	all := s.GetExecutionsGraphQL("proj-test", ddxgraphql.ExecutionFilter{})
	if got := len(all); got != 3 {
		t.Fatalf("expected 3 executions, got %d", got)
	}
	// Newest first.
	if all[0].ID != "20260423T030000-cccc3333" {
		t.Fatalf("expected newest first, got %s", all[0].ID)
	}

	// Filter by bead.
	beadOnly := s.GetExecutionsGraphQL("proj-test", ddxgraphql.ExecutionFilter{BeadID: "ddx-001"})
	if len(beadOnly) != 2 {
		t.Fatalf("expected 2 executions for ddx-001, got %d", len(beadOnly))
	}

	// Filter by verdict.
	blockOnly := s.GetExecutionsGraphQL("proj-test", ddxgraphql.ExecutionFilter{Verdict: "BLOCK"})
	if len(blockOnly) != 2 {
		t.Fatalf("expected 2 BLOCK executions, got %d", len(blockOnly))
	}

	// Filter by harness.
	codexOnly := s.GetExecutionsGraphQL("proj-test", ddxgraphql.ExecutionFilter{Harness: "codex"})
	if len(codexOnly) != 1 {
		t.Fatalf("expected 1 codex execution, got %d", len(codexOnly))
	}

	// Detail load.
	exec, ok := s.GetExecutionGraphQL("20260423T020000-bbbb2222")
	if !ok {
		t.Fatal("expected detail load to succeed")
	}
	if exec.Prompt == nil || !strings.Contains(*exec.Prompt, "prompt body for 20260423T020000-bbbb2222") {
		t.Fatal("expected prompt body to be loaded")
	}
	if exec.Result == nil || !strings.Contains(*exec.Result, "BLOCK") {
		t.Fatal("expected result body to be loaded")
	}
	if exec.SessionID == nil || *exec.SessionID == "" {
		t.Fatal("expected sessionId to be present")
	}

	// Tool-call stream for the bundle that has one.
	calls := s.GetExecutionToolCallsGraphQL("20260423T020000-bbbb2222")
	if got := len(calls); got != 50 {
		t.Fatalf("expected 50 tool calls, got %d", got)
	}
	if calls[0].Name != "Bash" {
		t.Fatalf("expected first tool call to be Bash, got %q", calls[0].Name)
	}
}

func TestExecutions_TerseManifestSchema(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, agent.ExecuteBeadArtifactDir, "20260423T040000-dddd4444")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	terse := map[string]string{
		"harness":       "claude",
		"model":         "claude-opus",
		"base_rev":      "ffff1111",
		"result_rev":    "eeee2222",
		"verdict":       "PASS",
		"bead_id":       "ddx-terse",
		"execution_dir": filepath.Join(agent.ExecuteBeadArtifactDir, "20260423T040000-dddd4444"),
	}
	mb, _ := json.Marshal(terse)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"verdict":"PASS","rationale":"ok"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newServerStateForTest(t, root)
	all := s.GetExecutionsGraphQL("proj-test", ddxgraphql.ExecutionFilter{})
	if len(all) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(all))
	}
	exec := all[0]
	if exec.BeadID == nil || *exec.BeadID != "ddx-terse" {
		t.Fatal("expected bead id from terse manifest")
	}
	if exec.Verdict == nil || *exec.Verdict != "PASS" {
		t.Fatal("expected verdict from terse manifest")
	}
	if exec.Harness == nil || *exec.Harness != "claude" {
		t.Fatal("expected harness from terse manifest")
	}
}

// Perf-shaped sanity check: scanning 1k bundles should complete promptly so
// the list-view p95 budget (200ms HTTP) is realistic on a dev laptop.
func TestExecutions_ListPerf_1000(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := t.TempDir()
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("20260423T05%04d-%08x", i, i*37)
		bead := fmt.Sprintf("ddx-%04d", i%50)
		verdict := "PASS"
		if i%3 == 0 {
			verdict = "BLOCK"
		}
		seedBundle(t, root, id, bead, "claude", verdict, i%10 == 0)
	}
	s := newServerStateForTest(t, root)
	start := time.Now()
	all := s.GetExecutionsGraphQL("proj-test", ddxgraphql.ExecutionFilter{})
	elapsed := time.Since(start)
	if len(all) != 1000 {
		t.Fatalf("expected 1000 executions, got %d", len(all))
	}
	if elapsed > 2*time.Second {
		t.Fatalf("scanning 1000 executions took %s, expected < 2s", elapsed)
	}
	t.Logf("scanned 1000 executions in %s", elapsed)
}
