package server

// TC-GQL-020..024: GraphQL integration tests for stage 2.5–2.7 resolver backfill.
//
// Each test spins up a real server backed by real state, fires POST /graphql
// requests, and asserts Relay connection shape and a representative data item.
// These tests were missing when the resolver implementations landed; they
// verify that a broken resolver-to-store path would be caught.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// writeWorkerRecord writes a WorkerRecord as status.json under the workers dir
// so the GraphQL workers resolver can read it without a live worker.
func writeWorkerRecord(t *testing.T, workDir string, rec WorkerRecord) {
	t.Helper()
	dir := filepath.Join(workDir, ".ddx", "workers", rec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSessionsJSONL writes session entries to the sharded session index.
func writeSessionsJSONL(t *testing.T, workDir string, entries []map[string]any) {
	t.Helper()
	dir := filepath.Join(workDir, ".ddx", "agent-logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		var entry agent.SessionEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatal(err)
		}
		if err := agent.AppendSessionIndex(dir, agent.SessionIndexEntryFromLegacy(workDir, entry), entry.Timestamp); err != nil {
			t.Fatal(err)
		}
	}
}

// gqlPost fires a single POST /graphql request and returns the decoded response body.
func gqlPost(t *testing.T, srv *Server, query string) []byte {
	t.Helper()
	rawBody, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}

// gqlErrors returns any errors from a decoded GraphQL response body.
func gqlErrors(body []byte) []string {
	var resp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &resp)
	msgs := make([]string, len(resp.Errors))
	for i, e := range resp.Errors {
		msgs[i] = e.Message
	}
	return msgs
}

// ─── TC-GQL-020: workers / workersByProject / worker(id) ─────────────────────

// TC-GQL-020: Workers, workersByProject, and worker(id) resolve real records
// written to disk under .ddx/workers/<id>/status.json.
func TestGraphQLWorkers(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-worker-test-node")

	workDir := setupTestDir(t)
	startedAt := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	writeWorkerRecord(t, workDir, WorkerRecord{
		ID:          "wk-test-001",
		Kind:        "execute-loop",
		State:       "exited",
		ProjectRoot: workDir,
		Harness:     "claude",
		StartedAt:   startedAt,
	})

	srv := New(":0", workDir)

	// Query.workers — Relay connection over all workers.
	body := gqlPost(t, srv, `{ workers { edges { node { id kind state projectRoot } cursor } pageInfo { hasNextPage } totalCount } }`)
	if errs := gqlErrors(body); len(errs) > 0 {
		t.Fatalf("workers GraphQL errors: %v", errs)
	}
	var resp struct {
		Data struct {
			Workers struct {
				Edges []struct {
					Node struct {
						ID          string `json:"id"`
						Kind        string `json:"kind"`
						State       string `json:"state"`
						ProjectRoot string `json:"projectRoot"`
					} `json:"node"`
					Cursor string `json:"cursor"`
				} `json:"edges"`
				PageInfo   struct{ HasNextPage bool } `json:"pageInfo"`
				TotalCount int                        `json:"totalCount"`
			} `json:"workers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, body)
	}
	if resp.Data.Workers.TotalCount != 1 {
		t.Errorf("expected totalCount=1, got %d", resp.Data.Workers.TotalCount)
	}
	if len(resp.Data.Workers.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(resp.Data.Workers.Edges))
	}
	node := resp.Data.Workers.Edges[0].Node
	if node.ID != "wk-test-001" {
		t.Errorf("expected id=wk-test-001, got %q", node.ID)
	}
	if node.Kind != "execute-loop" {
		t.Errorf("expected kind=execute-loop, got %q", node.Kind)
	}
	if node.State != "exited" {
		t.Errorf("expected state=exited, got %q", node.State)
	}
	if node.ProjectRoot != workDir {
		t.Errorf("expected projectRoot=%q, got %q", workDir, node.ProjectRoot)
	}
	if resp.Data.Workers.Edges[0].Cursor == "" {
		t.Error("expected non-empty cursor on worker edge")
	}

	// Query.workersByProject — scoped by the registered project id (not path).
	proj, ok := srv.state.GetProjectByPath(workDir)
	if !ok {
		t.Fatalf("expected workDir %q to be registered as a project", workDir)
	}
	body2 := gqlPost(t, srv, fmt.Sprintf(`{ workersByProject(projectID: %q) { totalCount } }`, proj.ID))
	if errs := gqlErrors(body2); len(errs) > 0 {
		t.Fatalf("workersByProject GraphQL errors: %v", errs)
	}
	var resp2 struct {
		Data struct {
			WorkersByProject struct{ TotalCount int } `json:"workersByProject"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body2, &resp2); err != nil {
		t.Fatalf("workersByProject: invalid JSON: %v", err)
	}
	if resp2.Data.WorkersByProject.TotalCount != 1 {
		t.Errorf("workersByProject: expected totalCount=1, got %d", resp2.Data.WorkersByProject.TotalCount)
	}

	// Query.worker(id) — fetch a specific record by ID.
	body3 := gqlPost(t, srv, `{ worker(id: "wk-test-001") { id kind state } }`)
	if errs := gqlErrors(body3); len(errs) > 0 {
		t.Fatalf("worker(id) GraphQL errors: %v", errs)
	}
	var resp3 struct {
		Data struct {
			Worker struct {
				ID    string `json:"id"`
				Kind  string `json:"kind"`
				State string `json:"state"`
			} `json:"worker"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body3, &resp3); err != nil {
		t.Fatalf("worker(id): invalid JSON: %v", err)
	}
	if resp3.Data.Worker.ID != "wk-test-001" {
		t.Errorf("worker(id): expected id=wk-test-001, got %q", resp3.Data.Worker.ID)
	}
	if resp3.Data.Worker.Kind != "execute-loop" {
		t.Errorf("worker(id): expected kind=execute-loop, got %q", resp3.Data.Worker.Kind)
	}
	if resp3.Data.Worker.State != "exited" {
		t.Errorf("worker(id): expected state=exited, got %q", resp3.Data.Worker.State)
	}
}

// TestGraphQLWorkersByProjectScopedByID verifies that workersByProject filters
// by the project id (not path). Regression test for ddx-05b4cc9d: the resolver
// used to compare the id argument to WorkerRecord.ProjectRoot (a path), so the
// per-project workers view was always empty for any non-empty project id.
func TestGraphQLWorkersByProjectScopedByID(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-workers-scope-test-node")

	// Project A is the server's own working dir (auto-registered by New()).
	// Project B is a second registered project with a different path / id.
	workDirA := setupTestDir(t)
	workDirB := t.TempDir()

	startedAt := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	writeWorkerRecord(t, workDirA, WorkerRecord{
		ID:          "wk-scope-A",
		Kind:        "execute-loop",
		State:       "running",
		ProjectRoot: workDirA,
		Harness:     "claude",
		StartedAt:   startedAt,
	})
	// Worker B lives under the workers dir of the server's working directory
	// (that is where GetWorkersGraphQL reads from) but targets project B via
	// its ProjectRoot field — mirroring how ExecuteLoopWorkerSpec.ProjectRoot
	// redirects a worker to another registered project.
	writeWorkerRecord(t, workDirA, WorkerRecord{
		ID:          "wk-scope-B",
		Kind:        "execute-loop",
		State:       "running",
		ProjectRoot: workDirB,
		Harness:     "claude",
		StartedAt:   startedAt.Add(time.Minute),
	})

	srv := New(":0", workDirA)
	projA, ok := srv.state.GetProjectByPath(workDirA)
	if !ok {
		t.Fatalf("project A at %q not registered", workDirA)
	}
	projB := srv.state.RegisterProject(workDirB)

	if projA.ID == projB.ID {
		t.Fatalf("expected distinct project ids, got %q", projA.ID)
	}

	queryTotal := func(q string) int {
		t.Helper()
		body := gqlPost(t, srv, q)
		if errs := gqlErrors(body); len(errs) > 0 {
			t.Fatalf("GraphQL errors for %q: %v", q, errs)
		}
		var resp struct {
			Data struct {
				Workers struct {
					TotalCount int `json:"totalCount"`
					Edges      []struct {
						Node struct {
							ID string `json:"id"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"workersByProject,omitempty"`
				Global struct {
					TotalCount int `json:"totalCount"`
					Edges      []struct {
						Node struct {
							ID string `json:"id"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"workers,omitempty"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("invalid JSON for %q: %v", q, err)
		}
		return resp.Data.Workers.TotalCount + resp.Data.Global.TotalCount
	}

	// workersByProject(projectID: projA.ID) must return exactly worker A.
	workersForProject := func(id string) []string {
		t.Helper()
		q := fmt.Sprintf(`{ workersByProject(projectID: %q) { totalCount edges { node { id } } } }`, id)
		body := gqlPost(t, srv, q)
		if errs := gqlErrors(body); len(errs) > 0 {
			t.Fatalf("GraphQL errors for %q: %v", q, errs)
		}
		var resp struct {
			Data struct {
				WorkersByProject struct {
					TotalCount int `json:"totalCount"`
					Edges      []struct {
						Node struct {
							ID string `json:"id"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"workersByProject"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("invalid JSON for %q: %v", q, err)
		}
		ids := make([]string, len(resp.Data.WorkersByProject.Edges))
		for i, e := range resp.Data.WorkersByProject.Edges {
			ids[i] = e.Node.ID
		}
		if resp.Data.WorkersByProject.TotalCount != len(ids) {
			t.Fatalf("totalCount %d != edges %d", resp.Data.WorkersByProject.TotalCount, len(ids))
		}
		return ids
	}

	if got := workersForProject(projA.ID); len(got) != 1 || got[0] != "wk-scope-A" {
		t.Errorf("workersByProject(%q) = %v, want [wk-scope-A]", projA.ID, got)
	}
	if got := workersForProject(projB.ID); len(got) != 1 || got[0] != "wk-scope-B" {
		t.Errorf("workersByProject(%q) = %v, want [wk-scope-B]", projB.ID, got)
	}

	// Unknown project id → empty list, no error.
	if got := workersForProject("proj-does-not-exist"); len(got) != 0 {
		t.Errorf("workersByProject(unknown) = %v, want []", got)
	}

	// Global workers query returns both workers.
	if total := queryTotal(`{ workers { totalCount edges { node { id } } } }`); total != 2 {
		t.Errorf("workers (global) totalCount = %d, want 2", total)
	}
}

// ─── TC-GQL-021: agentSessions / agentSession(id) ────────────────────────────

// TC-GQL-021: agentSessions and agentSession(id) return real entries read from
// the per-project monthly session index shards.
func TestGraphQLAgentSessions(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-session-test-node")

	workDir := setupTestDir(t)
	writeSessionsJSONL(t, workDir, []map[string]any{
		// Older session — completed, claude harness.
		{"id": "as-test-001", "timestamp": "2026-04-15T10:00:00Z", "harness": "claude", "model": "claude-sonnet-4-6", "duration_ms": 1500, "exit_code": 0},
		// Newer session — failed, codex harness (has non-empty error field).
		{"id": "as-test-002", "timestamp": "2026-04-15T11:00:00Z", "harness": "codex", "model": "gpt-4o", "duration_ms": 200, "exit_code": 1, "error": "context deadline exceeded"},
	})
	srv := New(":0", workDir)

	// Query.agentSessions — Relay connection (newest first).
	body := gqlPost(t, srv, `{ agentSessions { edges { node { id harness status startedAt } cursor } pageInfo { hasNextPage } totalCount } }`)
	if errs := gqlErrors(body); len(errs) > 0 {
		t.Fatalf("agentSessions GraphQL errors: %v", errs)
	}
	var resp struct {
		Data struct {
			AgentSessions struct {
				Edges []struct {
					Node struct {
						ID        string `json:"id"`
						Harness   string `json:"harness"`
						Status    string `json:"status"`
						StartedAt string `json:"startedAt"`
					} `json:"node"`
					Cursor string `json:"cursor"`
				} `json:"edges"`
				PageInfo   struct{ HasNextPage bool } `json:"pageInfo"`
				TotalCount int                        `json:"totalCount"`
			} `json:"agentSessions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, body)
	}
	if resp.Data.AgentSessions.TotalCount != 2 {
		t.Errorf("expected totalCount=2, got %d", resp.Data.AgentSessions.TotalCount)
	}
	if len(resp.Data.AgentSessions.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(resp.Data.AgentSessions.Edges))
	}
	// Newest first: as-test-002 (11:00) then as-test-001 (10:00).
	if resp.Data.AgentSessions.Edges[0].Node.ID != "as-test-002" {
		t.Errorf("expected first session=as-test-002 (newest), got %q", resp.Data.AgentSessions.Edges[0].Node.ID)
	}
	if resp.Data.AgentSessions.Edges[0].Node.Status != "failed" {
		t.Errorf("expected as-test-002 status=failed (has error), got %q", resp.Data.AgentSessions.Edges[0].Node.Status)
	}
	if resp.Data.AgentSessions.Edges[1].Node.ID != "as-test-001" {
		t.Errorf("expected second session=as-test-001, got %q", resp.Data.AgentSessions.Edges[1].Node.ID)
	}
	if resp.Data.AgentSessions.Edges[1].Node.Status != "completed" {
		t.Errorf("expected as-test-001 status=completed, got %q", resp.Data.AgentSessions.Edges[1].Node.Status)
	}

	// Query.agentSession(id) — fetch by ID.
	body2 := gqlPost(t, srv, `{ agentSession(id: "as-test-001") { id harness status } }`)
	if errs := gqlErrors(body2); len(errs) > 0 {
		t.Fatalf("agentSession(id) GraphQL errors: %v", errs)
	}
	var resp2 struct {
		Data struct {
			AgentSession struct {
				ID      string `json:"id"`
				Harness string `json:"harness"`
				Status  string `json:"status"`
			} `json:"agentSession"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body2, &resp2); err != nil {
		t.Fatalf("agentSession(id): invalid JSON: %v", err)
	}
	if resp2.Data.AgentSession.ID != "as-test-001" {
		t.Errorf("agentSession(id): expected id=as-test-001, got %q", resp2.Data.AgentSession.ID)
	}
	if resp2.Data.AgentSession.Harness != "claude" {
		t.Errorf("agentSession(id): expected harness=claude, got %q", resp2.Data.AgentSession.Harness)
	}
	if resp2.Data.AgentSession.Status != "completed" {
		t.Errorf("agentSession(id): expected status=completed, got %q", resp2.Data.AgentSession.Status)
	}
}

// ─── TC-GQL-022: execDefinitions / execRuns / execDefinition(id) / execRun(id) ──

// TC-GQL-022: execDefinitions, execDefinition(id), execRuns, and execRun(id)
// return real data from the exec store after SaveDefinition / SaveRunRecord.
func TestGraphQLExecutions(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-exec-test-node")

	workDir := setupTestDir(t)
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)

	store := ddxexec.NewStore(workDir)
	if err := store.SaveDefinition(ddxexec.Definition{
		ID:          "exec-test-def@1",
		ArtifactIDs: []string{"MET-TEST-001"},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: []string{"true"},
		},
		Active:    true,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	if err := store.SaveRunRecord(ddxexec.RunRecord{
		RunManifest: ddxexec.RunManifest{
			RunID:        "run-test-001",
			DefinitionID: "exec-test-def@1",
			ArtifactIDs:  []string{"MET-TEST-001"},
			StartedAt:    now,
			FinishedAt:   now.Add(100 * time.Millisecond),
			Status:       ddxexec.StatusSuccess,
			ExitCode:     0,
		},
		Result: ddxexec.RunResult{Stdout: "ok\n"},
	}); err != nil {
		t.Fatalf("SaveRunRecord: %v", err)
	}

	srv := New(":0", workDir)

	// Query.execDefinitions — connection over all active definitions.
	body := gqlPost(t, srv, `{ execDefinitions { edges { node { id active } cursor } pageInfo { hasNextPage } totalCount } }`)
	if errs := gqlErrors(body); len(errs) > 0 {
		t.Fatalf("execDefinitions GraphQL errors: %v", errs)
	}
	var resp struct {
		Data struct {
			ExecDefinitions struct {
				Edges []struct {
					Node struct {
						ID     string `json:"id"`
						Active bool   `json:"active"`
					} `json:"node"`
					Cursor string `json:"cursor"`
				} `json:"edges"`
				PageInfo   struct{ HasNextPage bool } `json:"pageInfo"`
				TotalCount int                        `json:"totalCount"`
			} `json:"execDefinitions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, body)
	}
	if resp.Data.ExecDefinitions.TotalCount != 1 {
		t.Errorf("execDefinitions: expected totalCount=1, got %d", resp.Data.ExecDefinitions.TotalCount)
	}
	if len(resp.Data.ExecDefinitions.Edges) != 1 {
		t.Fatalf("execDefinitions: expected 1 edge, got %d", len(resp.Data.ExecDefinitions.Edges))
	}
	if resp.Data.ExecDefinitions.Edges[0].Node.ID != "exec-test-def@1" {
		t.Errorf("execDefinitions: expected id=exec-test-def@1, got %q", resp.Data.ExecDefinitions.Edges[0].Node.ID)
	}
	if !resp.Data.ExecDefinitions.Edges[0].Node.Active {
		t.Error("execDefinitions: expected active=true")
	}

	// Query.execDefinition(id) — fetch specific definition by ID.
	body2 := gqlPost(t, srv, `{ execDefinition(id: "exec-test-def@1") { id active } }`)
	if errs := gqlErrors(body2); len(errs) > 0 {
		t.Fatalf("execDefinition(id) GraphQL errors: %v", errs)
	}
	var resp2 struct {
		Data struct {
			ExecDefinition struct {
				ID     string `json:"id"`
				Active bool   `json:"active"`
			} `json:"execDefinition"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body2, &resp2); err != nil {
		t.Fatalf("execDefinition(id): invalid JSON: %v", err)
	}
	if resp2.Data.ExecDefinition.ID != "exec-test-def@1" {
		t.Errorf("execDefinition(id): expected id=exec-test-def@1, got %q", resp2.Data.ExecDefinition.ID)
	}
	if !resp2.Data.ExecDefinition.Active {
		t.Error("execDefinition(id): expected active=true")
	}

	// Query.execRuns — connection over all run records.
	body3 := gqlPost(t, srv, `{ execRuns { edges { node { id status } cursor } pageInfo { hasNextPage } totalCount } }`)
	if errs := gqlErrors(body3); len(errs) > 0 {
		t.Fatalf("execRuns GraphQL errors: %v", errs)
	}
	var resp3 struct {
		Data struct {
			ExecRuns struct {
				Edges []struct {
					Node struct {
						ID     string `json:"id"`
						Status string `json:"status"`
					} `json:"node"`
					Cursor string `json:"cursor"`
				} `json:"edges"`
				PageInfo   struct{ HasNextPage bool } `json:"pageInfo"`
				TotalCount int                        `json:"totalCount"`
			} `json:"execRuns"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body3, &resp3); err != nil {
		t.Fatalf("execRuns: invalid JSON: %v\nbody: %s", err, body3)
	}
	if resp3.Data.ExecRuns.TotalCount != 1 {
		t.Errorf("execRuns: expected totalCount=1, got %d", resp3.Data.ExecRuns.TotalCount)
	}
	if len(resp3.Data.ExecRuns.Edges) != 1 {
		t.Fatalf("execRuns: expected 1 edge, got %d", len(resp3.Data.ExecRuns.Edges))
	}
	if resp3.Data.ExecRuns.Edges[0].Node.ID != "run-test-001" {
		t.Errorf("execRuns: expected id=run-test-001, got %q", resp3.Data.ExecRuns.Edges[0].Node.ID)
	}
	if resp3.Data.ExecRuns.Edges[0].Node.Status != "success" {
		t.Errorf("execRuns: expected status=success, got %q", resp3.Data.ExecRuns.Edges[0].Node.Status)
	}

	// Query.execRun(id) — fetch specific run by ID.
	body4 := gqlPost(t, srv, `{ execRun(id: "run-test-001") { id status } }`)
	if errs := gqlErrors(body4); len(errs) > 0 {
		t.Fatalf("execRun(id) GraphQL errors: %v", errs)
	}
	var resp4 struct {
		Data struct {
			ExecRun struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"execRun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body4, &resp4); err != nil {
		t.Fatalf("execRun(id): invalid JSON: %v", err)
	}
	if resp4.Data.ExecRun.ID != "run-test-001" {
		t.Errorf("execRun(id): expected id=run-test-001, got %q", resp4.Data.ExecRun.ID)
	}
	if resp4.Data.ExecRun.Status != "success" {
		t.Errorf("execRun(id): expected status=success, got %q", resp4.Data.ExecRun.Status)
	}
}

// ─── TC-GQL-023: personas / persona(name) / personaByRole ───────────────────

// TC-GQL-023: personas, persona(name), and personaByRole return real personas
// loaded from the library's personas directory.
//
// DDX_LIBRARY_BASE_PATH is set to an absolute path so the resolver can find
// files in the test's temp directory regardless of the process's working dir.
func TestGraphQLPersonas(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-persona-test-node")

	workDir := setupTestDir(t)
	libDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
	// Override library path so NewPersonaLoader resolves to an absolute dir.
	t.Setenv("DDX_LIBRARY_BASE_PATH", libDir)

	// Write a persona file with valid frontmatter.
	personaContent := "---\nname: test-reviewer\nroles: [code-reviewer]\ndescription: Test code reviewer persona\ntags: [test, review]\n---\n\n# Test Reviewer\n\nYou are a test code reviewer.\n"
	if err := os.WriteFile(filepath.Join(libDir, "personas", "test-reviewer.md"), []byte(personaContent), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(":0", workDir)

	// Query.personas — flat array over all personas in the library.
	body := gqlPost(t, srv, `{ personas { id name roles description body source bindings { projectId role persona } } }`)
	if errs := gqlErrors(body); len(errs) > 0 {
		t.Fatalf("personas GraphQL errors: %v", errs)
	}
	var resp struct {
		Data struct {
			Personas []struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				Roles       []string `json:"roles"`
				Description string   `json:"description"`
				Body        string   `json:"body"`
				Source      string   `json:"source"`
				Bindings    []struct {
					ProjectID string `json:"projectId"`
					Role      string `json:"role"`
					Persona   string `json:"persona"`
				} `json:"bindings"`
			} `json:"personas"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, body)
	}
	// The library directory also contains reviewer.md (no frontmatter) — it is
	// skipped. Only test-reviewer.md with valid frontmatter should appear.
	foundTestReviewer := false
	for _, p := range resp.Data.Personas {
		if p.Name == "test-reviewer" {
			foundTestReviewer = true
			if p.ID != "persona-test-reviewer" {
				t.Errorf("expected id=persona-test-reviewer, got %q", p.ID)
			}
			if len(p.Roles) == 0 || p.Roles[0] != "code-reviewer" {
				t.Errorf("expected roles=[code-reviewer], got %v", p.Roles)
			}
			if p.Description != "Test code reviewer persona" {
				t.Errorf("expected description=%q, got %q", "Test code reviewer persona", p.Description)
			}
			if p.Name != "test-reviewer" {
				t.Errorf("expected name=test-reviewer, got %q", p.Name)
			}
			if p.Body == "" {
				t.Error("expected non-empty persona body")
			}
			if p.Source == "" {
				t.Error("expected non-empty persona source")
			}
		}
	}
	if !foundTestReviewer {
		t.Errorf("test-reviewer not found in personas response (count=%d)", len(resp.Data.Personas))
	}

	// Query.persona(name) — fetch by name.
	body2 := gqlPost(t, srv, `{ persona(name: "test-reviewer") { id name description } }`)
	if errs := gqlErrors(body2); len(errs) > 0 {
		t.Fatalf("persona(name) GraphQL errors: %v", errs)
	}
	var resp2 struct {
		Data struct {
			Persona struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"persona"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body2, &resp2); err != nil {
		t.Fatalf("persona(name): invalid JSON: %v", err)
	}
	if resp2.Data.Persona.Name != "test-reviewer" {
		t.Errorf("persona(name): expected name=test-reviewer, got %q", resp2.Data.Persona.Name)
	}
	if resp2.Data.Persona.ID != "persona-test-reviewer" {
		t.Errorf("persona(name): expected id=persona-test-reviewer, got %q", resp2.Data.Persona.ID)
	}

	// Query.personaByRole — fetch by role name.
	body3 := gqlPost(t, srv, `{ personaByRole(role: "code-reviewer") { id name } }`)
	if errs := gqlErrors(body3); len(errs) > 0 {
		t.Fatalf("personaByRole GraphQL errors: %v", errs)
	}
	var resp3 struct {
		Data struct {
			PersonaByRole struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"personaByRole"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body3, &resp3); err != nil {
		t.Fatalf("personaByRole: invalid JSON: %v", err)
	}
	if resp3.Data.PersonaByRole.Name != "test-reviewer" {
		t.Errorf("personaByRole: expected name=test-reviewer, got %q", resp3.Data.PersonaByRole.Name)
	}
	if resp3.Data.PersonaByRole.ID != "persona-test-reviewer" {
		t.Errorf("personaByRole: expected id=persona-test-reviewer, got %q", resp3.Data.PersonaByRole.ID)
	}
}

// ─── TC-GQL-024: coordinators / coordinatorMetricsByProject ──────────────────

// TC-GQL-024: coordinators and coordinatorMetricsByProject return real entries
// from the coordinator registry. Accessing the registry via Get() creates a
// coordinator for the project root so AllMetrics() returns it.
func TestGraphQLCoordinators(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-coord-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	// Trigger coordinator creation for the working dir. Without this call,
	// AllMetrics() returns an empty slice since coordinators are lazily created.
	srv.workers.LandCoordinators.Get(workDir)
	t.Cleanup(func() { srv.workers.LandCoordinators.StopAll() })

	// Query.coordinators — list of all coordinator metrics entries.
	body := gqlPost(t, srv, `{ coordinators { projectRoot metrics { landed preserved failed } } }`)
	if errs := gqlErrors(body); len(errs) > 0 {
		t.Fatalf("coordinators GraphQL errors: %v", errs)
	}
	var resp struct {
		Data struct {
			Coordinators []struct {
				ProjectRoot string `json:"projectRoot"`
				Metrics     struct {
					Landed    int `json:"landed"`
					Preserved int `json:"preserved"`
					Failed    int `json:"failed"`
				} `json:"metrics"`
			} `json:"coordinators"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, body)
	}
	if len(resp.Data.Coordinators) != 1 {
		t.Errorf("expected 1 coordinator entry, got %d", len(resp.Data.Coordinators))
	}
	if len(resp.Data.Coordinators) > 0 && resp.Data.Coordinators[0].ProjectRoot != workDir {
		t.Errorf("expected projectRoot=%q, got %q", workDir, resp.Data.Coordinators[0].ProjectRoot)
	}

	// Query.coordinatorMetricsByProject — null for unknown project, non-null for known.
	body2 := gqlPost(t, srv, fmt.Sprintf(`{ coordinatorMetricsByProject(projectRoot: %q) { landed preserved failed } }`, workDir))
	if errs := gqlErrors(body2); len(errs) > 0 {
		t.Fatalf("coordinatorMetricsByProject GraphQL errors: %v", errs)
	}
	var resp2 struct {
		Data struct {
			CoordinatorMetricsByProject *struct {
				Landed    int `json:"landed"`
				Preserved int `json:"preserved"`
				Failed    int `json:"failed"`
			} `json:"coordinatorMetricsByProject"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body2, &resp2); err != nil {
		t.Fatalf("coordinatorMetricsByProject: invalid JSON: %v", err)
	}
	if resp2.Data.CoordinatorMetricsByProject == nil {
		t.Fatal("coordinatorMetricsByProject: expected non-null metrics for known project root")
	}
	// A freshly-created coordinator has zero counters — that is the correct ground state.
	m := resp2.Data.CoordinatorMetricsByProject
	if m.Landed != 0 || m.Preserved != 0 || m.Failed != 0 {
		t.Errorf("expected zero counters for fresh coordinator, got landed=%d preserved=%d failed=%d",
			m.Landed, m.Preserved, m.Failed)
	}

	// coordinatorMetricsByProject for an unknown root must return null (not error).
	body3 := gqlPost(t, srv, `{ coordinatorMetricsByProject(projectRoot: "/no/such/project") { landed } }`)
	if errs := gqlErrors(body3); len(errs) > 0 {
		t.Fatalf("coordinatorMetricsByProject(unknown) GraphQL errors: %v", errs)
	}
	var resp3 struct {
		Data struct {
			CoordinatorMetricsByProject *struct {
				Landed int `json:"landed"`
			} `json:"coordinatorMetricsByProject"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body3, &resp3); err != nil {
		t.Fatalf("coordinatorMetricsByProject(unknown): invalid JSON: %v", err)
	}
	if resp3.Data.CoordinatorMetricsByProject != nil {
		t.Error("coordinatorMetricsByProject: expected null for unknown project root")
	}
}
