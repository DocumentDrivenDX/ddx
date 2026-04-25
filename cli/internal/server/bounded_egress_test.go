package server

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// TestServerBoundedEgress verifies FEAT-022 §10 / §11 — every in-scope MCP
// and REST surface that inlines library-doc, execution-bundle, diff,
// session, or persona content runs the body through evidence.ClampOutput
// and exposes truncated/original_bytes metadata.
//
// Each subtest issues a request (or direct call for non-handler entry
// points) against an in-scope site, with a fixture body sized at 2× the
// effective inline cap. The bound is verified via the canonical
// truncation marker and the per-payload truncated/original_bytes fields.
func TestServerBoundedEgress(t *testing.T) {
	// Lower the inline cap for the duration of the test so fixtures stay
	// small. Restore it afterwards so unrelated tests in the package see
	// the production default.
	prev := serverInlineCapBytes
	serverInlineCapBytes = 256
	t.Cleanup(func() { serverInlineCapBytes = prev })

	cap := serverInlineCapBytes
	bigBody := strings.Repeat("A", 2*cap)
	smallBody := "tiny"

	t.Run("mcp_read_document_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		// Overwrite the fixture prompt with an oversized body.
		fixturePath := filepath.Join(dir, ".ddx", "plugins", "ddx", "prompts", "hello.md")
		if err := os.WriteFile(fixturePath, []byte(bigBody), 0o644); err != nil {
			t.Fatal(err)
		}

		srv := New(":0", dir)
		w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_read_document","arguments":{"path":"prompts/hello.md"}}`)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		text, truncated, originalBytes := mcpFirstContentFields(t, w)
		if !truncated {
			t.Fatalf("expected truncated=true, got false")
		}
		if originalBytes != len(bigBody) {
			t.Fatalf("expected original_bytes=%d, got %d", len(bigBody), originalBytes)
		}
		if len(text) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("clamped text length %d exceeds cap+marker (%d)", len(text), cap+len(evidence.TruncationMarker))
		}
		if !strings.HasSuffix(text, evidence.TruncationMarker) {
			t.Fatalf("expected text to end with TruncationMarker, got tail %q", tail(text, 64))
		}
	})

	t.Run("mcp_read_document_not_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		fixturePath := filepath.Join(dir, ".ddx", "plugins", "ddx", "prompts", "hello.md")
		if err := os.WriteFile(fixturePath, []byte(smallBody), 0o644); err != nil {
			t.Fatal(err)
		}
		srv := New(":0", dir)
		w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_read_document","arguments":{"path":"prompts/hello.md"}}`)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		text, truncated, originalBytes := mcpFirstContentFields(t, w)
		if truncated {
			t.Fatalf("expected truncated=false, got true")
		}
		if originalBytes != len(smallBody) {
			t.Fatalf("expected original_bytes=%d, got %d", len(smallBody), originalBytes)
		}
		if text != smallBody {
			t.Fatalf("expected text=%q, got %q", smallBody, text)
		}
	})

	t.Run("rest_read_document_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		fixturePath := filepath.Join(dir, ".ddx", "plugins", "ddx", "prompts", "hello.md")
		if err := os.WriteFile(fixturePath, []byte(bigBody), 0o644); err != nil {
			t.Fatal(err)
		}
		srv := New(":0", dir)
		req := httptest.NewRequest("GET", "/api/documents/prompts/hello.md", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Content       string `json:"content"`
			Truncated     bool   `json:"truncated"`
			OriginalBytes int    `json:"original_bytes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if !resp.Truncated {
			t.Fatalf("expected truncated=true")
		}
		if resp.OriginalBytes != len(bigBody) {
			t.Fatalf("expected original_bytes=%d, got %d", len(bigBody), resp.OriginalBytes)
		}
		if len(resp.Content) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("content length %d exceeds cap+marker", len(resp.Content))
		}
	})

	t.Run("rest_read_document_not_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		fixturePath := filepath.Join(dir, ".ddx", "plugins", "ddx", "prompts", "hello.md")
		if err := os.WriteFile(fixturePath, []byte(smallBody), 0o644); err != nil {
			t.Fatal(err)
		}
		srv := New(":0", dir)
		req := httptest.NewRequest("GET", "/api/documents/prompts/hello.md", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Content       string `json:"content"`
			Truncated     bool   `json:"truncated"`
			OriginalBytes int    `json:"original_bytes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if resp.Truncated {
			t.Fatalf("expected truncated=false")
		}
		if resp.OriginalBytes != len(smallBody) {
			t.Fatalf("expected original_bytes=%d, got %d", len(smallBody), resp.OriginalBytes)
		}
		if resp.Content != smallBody {
			t.Fatalf("expected content=%q, got %q", smallBody, resp.Content)
		}
	})

	t.Run("rest_agent_session_detail_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		// Build a session entry whose Prompt and Response bodies both
		// exceed the cap. writeSessionIndexLines materializes prompt.md
		// and result.json under the bundle dir, but detailAgentSession
		// reads them from the session entry's already-loaded fields, so
		// we must include them inline in the JSONL line.
		entry := map[string]any{
			"id":          "as-be-001",
			"timestamp":   "2026-01-01T10:00:00Z",
			"harness":     "claude",
			"prompt":      bigBody,
			"response":    bigBody,
			"prompt_len":  len(bigBody),
			"duration_ms": 1,
			"exit_code":   0,
		}
		line, _ := json.Marshal(entry)
		writeSessionIndexLines(t, dir, string(line))

		srv := New(":0", dir)
		req := httptest.NewRequest("GET", "/api/agent/sessions/as-be-001", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Prompt        string `json:"prompt"`
			Response      string `json:"response"`
			Truncated     bool   `json:"truncated"`
			OriginalBytes int    `json:"original_bytes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if !resp.Truncated {
			t.Fatalf("expected truncated=true")
		}
		want := 2 * len(bigBody)
		if resp.OriginalBytes != want {
			t.Fatalf("expected original_bytes=%d (sum of prompt+response), got %d", want, resp.OriginalBytes)
		}
		if len(resp.Prompt) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("clamped prompt length %d exceeds cap+marker", len(resp.Prompt))
		}
		if len(resp.Response) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("clamped response length %d exceeds cap+marker", len(resp.Response))
		}
	})

	t.Run("rest_agent_session_detail_not_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		entry := map[string]any{
			"id":          "as-be-002",
			"timestamp":   "2026-01-01T11:00:00Z",
			"harness":     "claude",
			"prompt":      smallBody,
			"response":    smallBody,
			"prompt_len":  len(smallBody),
			"duration_ms": 1,
			"exit_code":   0,
		}
		line, _ := json.Marshal(entry)
		writeSessionIndexLines(t, dir, string(line))

		srv := New(":0", dir)
		req := httptest.NewRequest("GET", "/api/agent/sessions/as-be-002", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Prompt        string `json:"prompt"`
			Response      string `json:"response"`
			Truncated     bool   `json:"truncated"`
			OriginalBytes int    `json:"original_bytes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if resp.Truncated {
			t.Fatalf("expected truncated=false")
		}
		if resp.OriginalBytes != 2*len(smallBody) {
			t.Fatalf("expected original_bytes=%d, got %d", 2*len(smallBody), resp.OriginalBytes)
		}
	})

	t.Run("rest_agent_worker_prompt_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		// Stage a prompt.md inside an attempt directory and wire a
		// worker record that points at it. handleAgentWorkerPrompt
		// resolves the attempt via WorkerManager.Show.
		srv := New(":0", dir)
		attemptID := "20260425T100000-be01"
		bundleDir := filepath.Join(dir, ".ddx", "executions", attemptID)
		if err := os.MkdirAll(bundleDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "prompt.md"), []byte(bigBody), 0o644); err != nil {
			t.Fatal(err)
		}
		stageWorkerWithAttempt(t, srv, "wkr-be-001", attemptID)

		req := httptest.NewRequest("GET", "/api/agent/workers/wkr-be-001/prompt", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Prompt        string `json:"prompt"`
			Truncated     bool   `json:"truncated"`
			OriginalBytes int    `json:"original_bytes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if !resp.Truncated {
			t.Fatalf("expected truncated=true")
		}
		if resp.OriginalBytes != len(bigBody) {
			t.Fatalf("expected original_bytes=%d, got %d", len(bigBody), resp.OriginalBytes)
		}
		if len(resp.Prompt) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("clamped prompt length %d exceeds cap+marker", len(resp.Prompt))
		}
	})

	t.Run("state_executions_load_bundle_detail_truncated", func(t *testing.T) {
		dir := setupTestDir(t)
		bundleID := "20260425T100000-bd01"
		bundleDir := filepath.Join(dir, ".ddx", "executions", bundleID)
		if err := os.MkdirAll(bundleDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), []byte(strings.Repeat("M", 2*cap)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "prompt.md"), []byte(bigBody), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "result.json"), []byte(strings.Repeat("R", 2*cap)), 0o644); err != nil {
			t.Fatal(err)
		}

		exec := loadExecutionBundleDetail("p1", dir, bundleDir, bundleID)
		if exec == nil {
			t.Fatal("expected non-nil execution")
		}
		if exec.Prompt == nil {
			t.Fatal("expected Prompt to be loaded")
		}
		if len(*exec.Prompt) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("Prompt length %d exceeds cap+marker", len(*exec.Prompt))
		}
		if !strings.HasSuffix(*exec.Prompt, evidence.TruncationMarker) {
			t.Fatalf("expected Prompt to end with TruncationMarker")
		}
		if exec.Manifest == nil || len(*exec.Manifest) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("Manifest exceeded cap")
		}
		if exec.Result == nil || len(*exec.Result) > cap+len(evidence.TruncationMarker) {
			t.Fatalf("Result exceeded cap")
		}
	})
}

// stageWorkerWithAttempt registers a worker with the server's WorkerManager
// in a state where Show(id) returns a record that points at attemptID.
func stageWorkerWithAttempt(t *testing.T, srv *Server, workerID, attemptID string) {
	t.Helper()
	if srv.workers == nil {
		t.Fatal("server has nil workers")
	}
	srv.workers.mu.Lock()
	defer srv.workers.mu.Unlock()
	if srv.workers.workers == nil {
		srv.workers.workers = map[string]*workerHandle{}
	}
	srv.workers.workers[workerID] = &workerHandle{
		record: WorkerRecord{
			ID: workerID,
			LastAttempt: &LastAttemptInfo{
				AttemptID: attemptID,
			},
		},
	}
}

func mcpFirstContentFields(t *testing.T, w *httptest.ResponseRecorder) (text string, truncated bool, originalBytes int) {
	t.Helper()
	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("rpc error: %s", resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	contentAny, ok := result["content"].([]any)
	if !ok || len(contentAny) == 0 {
		t.Fatalf("expected content array, got %v", result["content"])
	}
	first, ok := contentAny[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first content to be map")
	}
	text, _ = first["text"].(string)
	truncated, _ = first["truncated"].(bool)
	if ob, ok := first["original_bytes"].(float64); ok {
		originalBytes = int(ob)
	}
	return
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// TestEvidencePrimitiveUsage is the static-analysis gate for FEAT-022 §10
// (Stage E1). It asserts that every mcpContent{Type:"text", ...}
// composite-literal construction in cli/internal/server/server.go
// either:
//
//   - is sourced from a Text field that originates from
//     evidence.ClampOutput in the immediately enclosing function, OR
//   - carries an //evidence:allow-unbounded line-comment annotation on
//     the literal itself.
//
// The package-level mcpText helper is the single mcpContent literal site
// in this file; every other caller must route through that helper. The
// test parses server.go, walks the AST, and rejects any unannotated
// non-helper literal so that future contributors cannot reintroduce
// unbounded text egress on the MCP surface.
func TestEvidencePrimitiveUsage(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "server.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}

	// Build a map from line number → comment text for line-comments so
	// we can check the //evidence:allow-unbounded annotation that may
	// sit on the line before the literal.
	commentByLine := map[int]string{}
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			line := fset.Position(c.Slash).Line
			commentByLine[line] = c.Text
		}
	}

	var problems []string
	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		ident, ok := cl.Type.(*ast.Ident)
		if !ok || ident.Name != "mcpContent" {
			return true
		}
		// Only consider literals whose Type field is "text".
		hasTextType := false
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Type" {
				continue
			}
			if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Value == `"text"` {
				hasTextType = true
			}
		}
		if !hasTextType {
			return true
		}
		// Allow when the line immediately before the literal carries
		// the annotation comment.
		startLine := fset.Position(cl.Lbrace).Line
		annotated := false
		for delta := 1; delta <= 3; delta++ {
			if c, ok := commentByLine[startLine-delta]; ok {
				if strings.Contains(c, "evidence:allow-unbounded") && strings.Contains(c, `reason="`) {
					annotated = true
					break
				}
			}
		}
		if !annotated {
			problems = append(problems, fset.Position(cl.Lbrace).String())
		}
		return true
	})

	if len(problems) > 0 {
		t.Fatalf("found unannotated mcpContent{Type:\"text\"} literal sites in server.go (each must use evidence.ClampOutput via the mcpText helper, or carry //evidence:allow-unbounded). Sites:\n  %s",
			strings.Join(problems, "\n  "))
	}
}

// Compile-time assertion that the response-shape types still expose the
// FEAT-022 §10 fields. A future renamer would otherwise silently flatten
// these and the egress contract would degrade undetected.
var _ = agentSessionDetail{Truncated: false, OriginalBytes: 0}
