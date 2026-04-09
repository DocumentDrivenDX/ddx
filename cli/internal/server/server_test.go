package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestDir creates a temp directory with a library and bead store.
func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create .ddx/config.yaml so the server can find the library
	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configYAML := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create library with sample documents
	libDir := filepath.Join(dir, ".ddx", "plugins", "ddx")
	for _, cat := range []string{"prompts", "templates", "personas"} {
		catDir := filepath.Join(libDir, cat)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(libDir, "prompts", "hello.md"), []byte("# Hello prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "personas", "reviewer.md"), []byte("# Reviewer"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create beads.jsonl with sample beads
	beadOpen := `{"id":"bx-001","title":"Open bead","status":"open","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","labels":["p0"]}`
	beadClosed := `{"id":"bx-002","title":"Closed bead","status":"closed","priority":2,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	beadBlocked := `{"id":"bx-003","title":"Blocked bead","status":"open","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependencies":[{"issue_id":"bx-003","depends_on_id":"bx-001","type":"blocks"}]}`
	beadsContent := beadOpen + "\n" + beadClosed + "\n" + beadBlocked + "\n"
	if err := os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beadsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestListDocuments(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/documents", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var docs []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &docs); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(docs) < 2 {
		t.Fatalf("expected at least 2 documents, got %d", len(docs))
	}

	found := map[string]bool{}
	for _, d := range docs {
		found[d.Name] = true
	}
	if !found["hello.md"] {
		t.Error("expected hello.md in documents list")
	}
	if !found["reviewer.md"] {
		t.Error("expected reviewer.md in documents list")
	}
}

func TestListDocumentsFilterByType(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/documents?type=prompts", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var docs []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(docs))
	}
	if docs[0].Type != "prompts" {
		t.Errorf("expected type=prompts, got %s", docs[0].Type)
	}
}

func TestReadDocument(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/documents/prompts/hello.md", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var doc struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if doc.Content != "# Hello prompt" {
		t.Errorf("expected '# Hello prompt', got %q", doc.Content)
	}
}

func TestReadDocumentPathTraversal(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/documents/prompts/..%2F..%2Fetc%2Fpasswd", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("path traversal returned 200, expected error status")
	}
}

func TestReadDocumentNotFound(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/documents/nonexistent.md", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSearch(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/search?q=hello", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var results []struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 search result")
	}
	if results[0].Name != "hello.md" {
		t.Errorf("expected hello.md, got %s", results[0].Name)
	}
}

func TestSearchMissingQuery(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListBeads(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var beads []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(beads) != 3 {
		t.Fatalf("expected 3 beads, got %d", len(beads))
	}
}

func TestListBeadsFilterByStatus(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads?status=open", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var beads []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
		t.Fatal(err)
	}
	if len(beads) != 2 {
		t.Fatalf("expected 2 open beads, got %d", len(beads))
	}
	for _, b := range beads {
		if b.Status != "open" {
			t.Errorf("expected status=open, got %q for %s", b.Status, b.ID)
		}
	}
}

func TestShowBead(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads/bx-001", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var b struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.ID != "bx-001" {
		t.Errorf("expected bx-001, got %s", b.ID)
	}
	if b.Title != "Open bead" {
		t.Errorf("expected 'Open bead', got %q", b.Title)
	}
}

func TestShowBeadNotFound(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestBeadsReady(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads/ready", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var beads []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
		t.Fatal(err)
	}
	if len(beads) != 1 {
		t.Fatalf("expected 1 ready bead, got %d", len(beads))
	}
	if beads[0].ID != "bx-001" {
		t.Errorf("expected bx-001, got %s", beads[0].ID)
	}
}

func TestBeadsBlocked(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads/blocked", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var beads []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
		t.Fatal(err)
	}
	if len(beads) != 1 {
		t.Fatalf("expected 1 blocked bead, got %d", len(beads))
	}
	if beads[0].ID != "bx-003" {
		t.Errorf("expected bx-003, got %s", beads[0].ID)
	}
}

func TestBeadsStatus(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var counts struct {
		Open    int `json:"open"`
		Closed  int `json:"closed"`
		Blocked int `json:"blocked"`
		Ready   int `json:"ready"`
		Total   int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Total != 3 {
		t.Errorf("expected total=3, got %d", counts.Total)
	}
	if counts.Open != 2 {
		t.Errorf("expected open=2, got %d", counts.Open)
	}
	if counts.Closed != 1 {
		t.Errorf("expected closed=1, got %d", counts.Closed)
	}
}

func TestBeadDepTree(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads/dep/tree/bx-003", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		ID   string `json:"id"`
		Tree string `json:"tree"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ID != "bx-003" {
		t.Errorf("expected id=bx-003, got %s", result.ID)
	}
	if result.Tree == "" {
		t.Error("expected non-empty tree")
	}
}

func TestDocGraph(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/docs/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var nodes []json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("expected JSON array: %v", err)
	}
}

func TestDocStale(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/docs/stale", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHealth(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" {
		t.Errorf("expected status=ok, got %s", result.Status)
	}
}

func TestReady(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/ready", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ready" {
		t.Errorf("expected status=ready, got %s", result.Status)
	}
	if result.Checks["beads"] != "ok" {
		t.Errorf("expected beads=ok, got %s", result.Checks["beads"])
	}
}

func TestAgentSessions(t *testing.T) {
	dir := setupTestDir(t)

	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	session1 := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"tokens":500,"duration_ms":2000,"exit_code":0}`
	session2 := `{"id":"as-0002","timestamp":"2026-01-01T11:00:00Z","harness":"claude","model":"sonnet","prompt_len":200,"tokens":800,"duration_ms":3000,"exit_code":0}`
	if err := os.WriteFile(filepath.Join(logDir, "sessions.jsonl"), []byte(session1+"\n"+session2+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sessions []struct {
		ID      string `json:"id"`
		Harness string `json:"harness"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	// Most recent first
	if sessions[0].ID != "as-0002" {
		t.Errorf("expected most recent first (as-0002), got %s", sessions[0].ID)
	}
}

func TestAgentSessionsFilterByHarness(t *testing.T) {
	dir := setupTestDir(t)

	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	session1 := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"tokens":500,"duration_ms":2000,"exit_code":0}`
	session2 := `{"id":"as-0002","timestamp":"2026-01-01T11:00:00Z","harness":"claude","model":"sonnet","prompt_len":200,"tokens":800,"duration_ms":3000,"exit_code":0}`
	if err := os.WriteFile(filepath.Join(logDir, "sessions.jsonl"), []byte(session1+"\n"+session2+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions?harness=codex", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sessions []struct {
		ID      string `json:"id"`
		Harness string `json:"harness"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Harness != "codex" {
		t.Errorf("expected harness=codex, got %s", sessions[0].Harness)
	}
}

func TestAgentSessionDetail(t *testing.T) {
	dir := setupTestDir(t)

	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	session := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"prompt":"inspect me","response":"done","correlation":{"bead_id":"hx-123"},"tokens":500,"duration_ms":2000,"exit_code":0}`
	if err := os.WriteFile(filepath.Join(logDir, "sessions.jsonl"), []byte(session+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions/as-0001", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sess struct {
		ID          string            `json:"id"`
		Harness     string            `json:"harness"`
		Tokens      int               `json:"tokens"`
		Prompt      string            `json:"prompt"`
		Response    string            `json:"response"`
		Correlation map[string]string `json:"correlation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	if sess.ID != "as-0001" {
		t.Errorf("expected as-0001, got %s", sess.ID)
	}
	if sess.Tokens != 500 {
		t.Errorf("expected tokens=500, got %d", sess.Tokens)
	}
	if sess.Prompt != "inspect me" || sess.Response != "done" {
		t.Errorf("expected prompt/response to be returned, got %q / %q", sess.Prompt, sess.Response)
	}
	if sess.Correlation["bead_id"] != "hx-123" {
		t.Errorf("expected bead correlation, got %v", sess.Correlation)
	}
}

func TestAgentSessionDetailNotFound(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// --- MCP endpoint tests ---

func mcpRequest(t *testing.T, srv *Server, method string, params string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `"`
	if params != "" {
		body += `,"params":` + params
	}
	body += "}"
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestMCPInitialize(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "initialize", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result to be a map")
	}
	info, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("expected serverInfo in result")
	}
	if info["name"] != "ddx-server" {
		t.Errorf("expected name=ddx-server, got %v", info["name"])
	}
}

func TestMCPToolsList(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/list", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result map")
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("expected tools array")
	}
	if len(tools) != 25 {
		t.Fatalf("expected 25 MCP tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		names[toolMap["name"].(string)] = true
	}
	expected := []string{
		"ddx_list_documents", "ddx_read_document", "ddx_search", "ddx_resolve_persona",
		"ddx_list_beads", "ddx_show_bead", "ddx_bead_ready", "ddx_bead_status",
		"ddx_bead_create", "ddx_bead_update", "ddx_bead_claim",
		"ddx_doc_graph", "ddx_doc_stale", "ddx_doc_show", "ddx_doc_deps",
		"ddx_agent_sessions",
		"ddx_exec_definitions", "ddx_exec_show", "ddx_exec_history",
		"ddx_exec_dispatch", "ddx_agent_dispatch",
		"ddx_doc_changed",
		"ddx_doc_write", "ddx_doc_history", "ddx_doc_diff",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing MCP tool: %s", name)
		}
	}
}

func TestMCPListDocuments(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_list_documents","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result map")
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("expected content array with entries")
	}
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)
	if !strings.Contains(text, "hello.md") {
		t.Errorf("expected hello.md in MCP response, got: %s", text)
	}
}

func TestMCPReadDocument(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_read_document","arguments":{"path":"prompts/hello.md"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)
	if text != "# Hello prompt" {
		t.Errorf("expected '# Hello prompt', got %q", text)
	}
}

func TestMCPListBeads(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_list_beads","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)

	var beads []map[string]any
	if err := json.Unmarshal([]byte(text), &beads); err != nil {
		t.Fatalf("MCP beads response not valid JSON: %v", err)
	}
	if len(beads) != 3 {
		t.Errorf("expected 3 beads, got %d", len(beads))
	}
}

func TestMCPBeadReady(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_bead_ready","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)

	var beads []map[string]any
	if err := json.Unmarshal([]byte(text), &beads); err != nil {
		t.Fatalf("MCP ready response not valid JSON: %v", err)
	}
	if len(beads) != 1 {
		t.Errorf("expected 1 ready bead, got %d", len(beads))
	}
}

func TestMCPShowBead(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_show_bead","arguments":{"id":"bx-001"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)

	var b map[string]any
	if err := json.Unmarshal([]byte(text), &b); err != nil {
		t.Fatalf("MCP show_bead not valid JSON: %v", err)
	}
	if b["id"] != "bx-001" {
		t.Errorf("expected bx-001, got %v", b["id"])
	}
}

func TestMCPBeadStatus(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_bead_status","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)

	var counts map[string]any
	if err := json.Unmarshal([]byte(text), &counts); err != nil {
		t.Fatalf("MCP bead_status not valid JSON: %v", err)
	}
	if counts["total"].(float64) != 3 {
		t.Errorf("expected total=3, got %v", counts["total"])
	}
}

func TestMCPSearch(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_search","arguments":{"query":"hello"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)
	if !strings.Contains(text, "hello.md") {
		t.Errorf("expected hello.md in search results, got: %s", text)
	}
}

func TestMCPAgentSessions(t *testing.T) {
	dir := setupTestDir(t)

	logDir := filepath.Join(dir, ".ddx", "agent-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	session := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"tokens":500,"duration_ms":2000,"exit_code":0}`
	if err := os.WriteFile(filepath.Join(logDir, "sessions.jsonl"), []byte(session+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_agent_sessions","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)
	if !strings.Contains(text, "as-0001") {
		t.Errorf("expected as-0001 in sessions, got: %s", text)
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "unknown/method", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}

func TestMCPUnknownTool(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"nonexistent_tool","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	if result["isError"] != true {
		t.Error("expected isError=true for unknown tool")
	}
}

// setupGitTestDir creates a temp directory that is also a git repository,
// with a markdown document that has DDx frontmatter (so it appears in the doc graph).
func setupGitTestDir(t *testing.T) (dir string, docID string) {
	t.Helper()
	dir = setupTestDir(t)

	// Initialize a git repo in the temp dir.
	runGit(t, "init", dir)
	runGit(t, "-C", dir, "config", "user.email", "test@test.com")
	runGit(t, "-C", dir, "config", "user.name", "Test User")

	// Create a markdown file with DDx frontmatter so the docgraph can find it.
	docID = "TD-TEST-001"
	docPath := filepath.Join(dir, "docs", "test-doc.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nddx:\n  id: " + docID + "\n---\n# Test Document\n\nInitial content.\n"
	if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Commit the file so git log has history.
	runGit(t, "-C", dir, "add", "docs/test-doc.md")
	runGit(t, "-C", dir, "commit", "-m", "add test document")

	return dir, docID
}

// runGit runs a git command and fails the test if it returns an error.
func runGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestDocWriteEndpoint(t *testing.T) {
	dir, docID := setupGitTestDir(t)
	srv := New(":0", dir)

	body := strings.NewReader(`{"content":"# Updated\n\nNew content."}`)
	req := httptest.NewRequest("PUT", "/api/docs/"+docID, body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Status string `json:"status"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("expected status=ok, got %q", result.Status)
	}
	if result.Path == "" {
		t.Error("expected non-empty path in response")
	}
}

func TestDocHistoryEndpoint(t *testing.T) {
	dir, docID := setupGitTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/docs/"+docID+"/history", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []struct {
		Hash    string `json:"hash"`
		Date    string `json:"date"`
		Author  string `json:"author"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("expected JSON array: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one history entry")
	}
	if entries[0].Hash == "" {
		t.Error("expected non-empty hash")
	}
	if entries[0].Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestDocDiffEndpoint(t *testing.T) {
	dir, docID := setupGitTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/docs/"+docID+"/diff", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Diff string `json:"diff"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("expected JSON object with diff: %v", err)
	}
	// diff may be empty (no uncommitted changes) — just verify the key exists
	_ = result.Diff
}

func TestBeadEndpoints(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var beads []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
		t.Fatalf("expected JSON array: %v", err)
	}
	if len(beads) == 0 {
		t.Fatal("expected at least one bead")
	}

	// Verify IDs are non-empty
	for _, b := range beads {
		if b.ID == "" {
			t.Error("expected non-empty bead ID")
		}
	}
}

func TestMCPDocTools(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/list", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result map")
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("expected tools array")
	}

	names := map[string]bool{}
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		names[toolMap["name"].(string)] = true
	}

	required := []string{"ddx_doc_write", "ddx_doc_history", "ddx_doc_diff"}
	for _, name := range required {
		if !names[name] {
			t.Errorf("missing MCP tool: %s", name)
		}
	}
}

// --- SPA handler tests ---

func TestSPAServesIndexHTML(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<html") {
		t.Error("expected HTML content from SPA index")
	}
}

func TestSPAFallbackForClientRoute(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// A client-side route like /beads should serve index.html
	req := httptest.NewRequest("GET", "/beads", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<html") {
		t.Error("expected HTML content from SPA index for client route")
	}
}

// setupExecTestDir creates a temp directory with exec definition and run data.
func setupExecTestDir(t *testing.T) string {
	t.Helper()
	dir := setupTestDir(t)

	ddxDir := filepath.Join(dir, ".ddx")

	// Create exec-definitions.jsonl
	defBead := `{"id":"bench-startup","title":"Execution definition for MET-startup","status":"open","priority":2,"issue_type":"exec_definition","created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z","labels":["artifact:MET-startup","executor:command"],"definition":{"id":"bench-startup","artifact_ids":["MET-startup"],"executor":{"kind":"command","command":["go","test","-bench=."],"timeout_ms":30000},"result":{"metric":{"unit":"ms"}},"evaluation":{"comparison":"lower-is-better"},"active":true,"created_at":"2026-04-01T00:00:00Z"}}`
	if err := os.WriteFile(filepath.Join(ddxDir, "exec-definitions.jsonl"), []byte(defBead+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create exec-runs.jsonl
	runBead := `{"id":"bench-startup@2026-04-01T10:00:00Z-1","title":"Execution run for MET-startup","status":"closed","priority":2,"issue_type":"exec_run","created_at":"2026-04-01T10:00:00Z","updated_at":"2026-04-01T10:00:01Z","labels":["artifact:MET-startup","status:success","definition:bench-startup"],"manifest":{"run_id":"bench-startup@2026-04-01T10:00:00Z-1","definition_id":"bench-startup","artifact_ids":["MET-startup"],"started_at":"2026-04-01T10:00:00Z","finished_at":"2026-04-01T10:00:01Z","status":"success","exit_code":0,"attachments":{"stdout":"exec-runs.d/bench-startup@2026-04-01T10:00:00Z-1/stdout.log","stderr":"exec-runs.d/bench-startup@2026-04-01T10:00:00Z-1/stderr.log","result":"exec-runs.d/bench-startup@2026-04-01T10:00:00Z-1/result.json"}}}`
	if err := os.WriteFile(filepath.Join(ddxDir, "exec-runs.jsonl"), []byte(runBead+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create attachment directory and files
	runDir := filepath.Join(ddxDir, "exec-runs.d", "bench-startup@2026-04-01T10:00:00Z-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("7.2 ms"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	resultJSON := `{"metric":{"artifact_id":"MET-startup","definition_id":"bench-startup","observed_at":"2026-04-01T10:00:00Z","status":"pass","value":7.2,"unit":"ms","samples":[7.2]},"stdout":"7.2 ms","value":7.2,"unit":"ms","parsed":true}`
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte(resultJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestExecDefinitionsList(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/definitions", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var defs []struct {
		ID          string   `json:"id"`
		ArtifactIDs []string `json:"artifact_ids"`
		Active      bool     `json:"active"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &defs); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].ID != "bench-startup" {
		t.Errorf("expected id=bench-startup, got %s", defs[0].ID)
	}
}

func TestExecDefinitionsFilterByArtifact(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/definitions?artifact=MET-startup", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var defs []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &defs); err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}

	// Filter with non-matching artifact
	req = httptest.NewRequest("GET", "/api/exec/definitions?artifact=MET-nonexistent", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if err := json.Unmarshal(w.Body.Bytes(), &defs); err != nil {
		t.Fatal(err)
	}
	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions for non-matching filter, got %d", len(defs))
	}
}

func TestExecDefinitionShow(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/definitions/bench-startup", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var def struct {
		ID       string `json:"id"`
		Executor struct {
			Kind string `json:"kind"`
		} `json:"executor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &def); err != nil {
		t.Fatal(err)
	}
	if def.ID != "bench-startup" {
		t.Errorf("expected id=bench-startup, got %s", def.ID)
	}
	if def.Executor.Kind != "command" {
		t.Errorf("expected executor.kind=command, got %s", def.Executor.Kind)
	}
}

func TestExecDefinitionShowNotFound(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/definitions/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestExecRunsList(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/runs", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var runs []struct {
		RunID        string `json:"run_id"`
		DefinitionID string `json:"definition_id"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Status != "success" {
		t.Errorf("expected status=success, got %s", runs[0].Status)
	}
}

func TestExecRunsFilterByDefinition(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/runs?definition=bench-startup", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var runs []struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestExecRunShow(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/runs/bench-startup@2026-04-01T10:00:00Z-1", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Value  float64 `json:"value"`
		Unit   string  `json:"unit"`
		Parsed bool    `json:"parsed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Value != 7.2 {
		t.Errorf("expected value=7.2, got %f", result.Value)
	}
	if result.Unit != "ms" {
		t.Errorf("expected unit=ms, got %s", result.Unit)
	}
}

func TestExecRunLog(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/runs/bench-startup@2026-04-01T10:00:00Z-1/log", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var logs struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &logs); err != nil {
		t.Fatal(err)
	}
	if logs.Stdout != "7.2 ms" {
		t.Errorf("expected stdout='7.2 ms', got %q", logs.Stdout)
	}
}

func TestExecRunNotFound(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/exec/runs/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMCPExecDefinitions(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_exec_definitions","arguments":{}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected MCP content")
	}
	if !strings.Contains(resp.Result.Content[0].Text, "bench-startup") {
		t.Error("expected bench-startup in MCP response")
	}
}

func TestMCPExecShow(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_exec_show","arguments":{"id":"bench-startup"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected MCP content")
	}
	if !strings.Contains(resp.Result.Content[0].Text, "bench-startup") {
		t.Error("expected bench-startup in MCP exec show response")
	}
}

func TestMCPExecHistory(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_exec_history","arguments":{}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected MCP content")
	}
	if !strings.Contains(resp.Result.Content[0].Text, "bench-startup") {
		t.Error("expected bench-startup in MCP exec history response")
	}
}

func TestCreateBead(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"title":"New task","type":"task","priority":1,"labels":["p0","area:cli"],"description":"A test bead","acceptance":"It works"}`
	req := httptest.NewRequest("POST", "/api/beads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Error("expected non-empty bead ID")
	}
	if created.Title != "New task" {
		t.Errorf("expected title='New task', got %q", created.Title)
	}
}

func TestCreateBeadMissingTitle(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"type":"task"}`
	req := httptest.NewRequest("POST", "/api/beads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateBead(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"description":"Updated description"}`
	req := httptest.NewRequest("PUT", "/api/beads/bx-001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Description != "Updated description" {
		t.Errorf("expected description='Updated description', got %q", updated.Description)
	}
}

func TestUpdateBeadNotFound(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"description":"test"}`
	req := httptest.NewRequest("PUT", "/api/beads/nonexistent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestClaimBead(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"assignee":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/beads/bx-001/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "claimed" {
		t.Errorf("expected status=claimed, got %s", resp["status"])
	}
}

func TestUnclaimBead(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// First claim
	claimBody := `{"assignee":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/beads/bx-001/claim", strings.NewReader(claimBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("claim failed: %d", w.Code)
	}

	// Then unclaim
	req = httptest.NewRequest("POST", "/api/beads/bx-001/unclaim", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReopenBead(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// bx-002 is closed
	body := `{"reason":"Need more work"}`
	req := httptest.NewRequest("POST", "/api/beads/bx-002/reopen", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "reopened" {
		t.Errorf("expected status=reopened, got %s", resp["status"])
	}
}

func TestBeadDepsAdd(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"action":"add","dep_id":"bx-002"}`
	req := httptest.NewRequest("POST", "/api/beads/bx-001/deps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPBeadCreate(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_create","arguments":{"title":"MCP bead","type":"task"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Result.IsError {
		t.Fatalf("MCP bead create returned error: %s", resp.Result.Content[0].Text)
	}
	if !strings.Contains(resp.Result.Content[0].Text, "MCP bead") {
		t.Error("expected 'MCP bead' in response")
	}
}

func TestMCPBeadClaim(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_claim","arguments":{"id":"bx-001","assignee":"agent"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Result.IsError {
		t.Fatalf("MCP bead claim returned error: %s", resp.Result.Content[0].Text)
	}
	if !strings.Contains(resp.Result.Content[0].Text, "claimed") {
		t.Error("expected 'claimed' in response")
	}
}

func TestExecDispatchLocalhostOnly(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("POST", "/api/exec/run/bench-startup", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-localhost, got %d", w.Code)
	}
}

func TestAgentDispatchLocalhostOnly(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"harness":"claude","prompt":"hello"}`
	req := httptest.NewRequest("POST", "/api/agent/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-localhost, got %d", w.Code)
	}
}

func TestAgentDispatchMissingHarness(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"prompt":"hello"}`
	req := httptest.NewRequest("POST", "/api/agent/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing harness, got %d", w.Code)
	}
}

func TestMCPExecDispatchUntrustedForbidden(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_exec_dispatch","arguments":{"id":"bench-startup"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 JSON-RPC response, got %d", w.Code)
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result map")
	}
	isError, _ := result["isError"].(bool)
	if !isError {
		t.Fatal("expected isError=true for non-trusted MCP exec dispatch")
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("expected error content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "forbidden") {
		t.Errorf("expected forbidden message, got %q", text)
	}
}

func TestMCPAgentDispatchUntrustedForbidden(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_agent_dispatch","arguments":{"harness":"claude","prompt":"hello"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 JSON-RPC response, got %d", w.Code)
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result map")
	}
	isError, _ := result["isError"].(bool)
	if !isError {
		t.Fatal("expected isError=true for non-trusted MCP agent dispatch")
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("expected error content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "forbidden") {
		t.Errorf("expected forbidden message, got %q", text)
	}
}

func TestMCPToolsListIncludesExec(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	respBody := w.Body.String()
	for _, tool := range []string{"ddx_exec_definitions", "ddx_exec_show", "ddx_exec_history"} {
		if !strings.Contains(respBody, tool) {
			t.Errorf("expected tools/list to include %s", tool)
		}
	}
}
