package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
  path: ".ddx/library"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create library with sample documents
	libDir := filepath.Join(dir, ".ddx", "library")
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
	if len(tools) != 16 {
		t.Fatalf("expected 16 MCP tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		names[toolMap["name"].(string)] = true
	}
	expected := []string{
		"ddx_list_documents", "ddx_read_document", "ddx_search", "ddx_resolve_persona",
		"ddx_list_beads", "ddx_show_bead", "ddx_bead_ready", "ddx_bead_status",
		"ddx_doc_graph", "ddx_doc_stale", "ddx_doc_show", "ddx_doc_deps",
		"ddx_agent_sessions",
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
