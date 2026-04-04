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

	// Verify both our test documents appear
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

	// Go's ServeMux cleans URLs with ".." before dispatch, resulting in a
	// redirect. Verify that a crafted path does not leak file content.
	// Use a path that survives mux cleaning but still contains "..".
	req := httptest.NewRequest("GET", "/api/documents/prompts/..%2F..%2Fetc%2Fpasswd", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Should not succeed (either 400 or 404 is acceptable)
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
	// bx-001 is open with no deps (ready), bx-003 is open but blocked
	if len(beads) != 1 {
		t.Fatalf("expected 1 ready bead, got %d", len(beads))
	}
	if beads[0].ID != "bx-001" {
		t.Errorf("expected bx-001, got %s", beads[0].ID)
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

func TestDocGraph(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/docs/graph", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Graph should return an array (may be empty for test dir with no docs with frontmatter)
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
	if len(tools) < 4 {
		t.Fatalf("expected at least 4 MCP tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		names[toolMap["name"].(string)] = true
	}
	for _, expected := range []string{"ddx_list_documents", "ddx_read_document", "ddx_list_beads", "ddx_bead_ready"} {
		if !names[expected] {
			t.Errorf("missing MCP tool: %s", expected)
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
	// The content text should be a JSON array of documents
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
