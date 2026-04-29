package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/processmetrics"
)

// TestMain scrubs all GIT_* environment variables before running tests.
// Lefthook sets GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, GIT_AUTHOR_*,
// GIT_COMMITTER_*, GIT_CONFIG_PARAMETERS, etc. during pre-commit. If these
// leak into subprocess git calls made by tests (e.g. `git init` in a
// t.TempDir()), the subprocess writes to the PARENT repository's config —
// specifically a stray `worktree = /tmp/TestXxx/NNN` line that corrupts
// every subsequent git operation. Scrubbing at TestMain covers both raw
// exec.Command calls AND the production code paths exercised by tests.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				_ = os.Unsetenv(kv[:idx])
			}
		}
	}
	os.Exit(m.Run())
}

// setupTestDir creates a temp directory with a library and bead store.
// Isolates the server's persistent state (XDG_DATA_HOME) to a per-test temp
// dir so tests don't accumulate registered projects in the user's real state
// file. Tests that need to assert on the state file location should read
// os.Getenv("XDG_DATA_HOME") after calling setupTestDir rather than setting
// their own value before.
func setupTestDir(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
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

func writeSessionIndexLines(t *testing.T, workDir string, lines ...string) {
	t.Helper()
	logDir := filepath.Join(workDir, agent.DefaultLogDir)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		var entry agent.SessionEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Prompt != "" || entry.Response != "" {
			if entry.Correlation == nil {
				entry.Correlation = map[string]string{}
			}
			attemptID := entry.Correlation["attempt_id"]
			if attemptID == "" {
				attemptID = "session-" + entry.ID
				entry.Correlation["attempt_id"] = attemptID
			}
			bundleDir := filepath.Join(workDir, agent.ExecuteBeadArtifactDir, attemptID)
			if err := os.MkdirAll(bundleDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bundleDir, "prompt.md"), []byte(entry.Prompt), 0o644); err != nil {
				t.Fatal(err)
			}
			result := map[string]string{"response": entry.Response}
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bundleDir, "result.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		idx := agent.SessionIndexEntryFromLegacy(workDir, entry)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatal(err)
		}
		_, idx.CostPresent = raw["cost_usd"]
		if err := agent.AppendSessionIndex(logDir, idx, entry.Timestamp); err != nil {
			t.Fatal(err)
		}
	}
}

func setupProcessMetricsTestDir(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()

	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(filepath.Join(dir, agent.DefaultLogDir), 0o755); err != nil {
		t.Fatal(err)
	}

	beads := []string{
		`{"id":"bx-001","title":"Feature one","status":"closed","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T03:30:00Z","labels":["helix"],"spec-id":"FEAT-001","session_id":"as-001","events":[{"kind":"status","summary":"closed","created_at":"2026-01-01T01:00:00Z","source":"test"},{"kind":"status","summary":"open","created_at":"2026-01-01T02:00:00Z","source":"test"},{"kind":"status","summary":"closed","created_at":"2026-01-01T03:00:00Z","source":"test"}]}`,
		`{"id":"bx-002","title":"Feature two","status":"closed","priority":1,"issue_type":"task","created_at":"2026-01-02T00:00:00Z","updated_at":"2026-01-02T01:30:00Z","spec-id":"FEAT-001","session_id":"as-002","events":[{"kind":"status","summary":"closed","created_at":"2026-01-02T01:30:00Z","source":"test"}]}`,
	}
	if err := os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(beads[0]+"\n"+beads[1]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions := []string{
		`{"id":"as-001","timestamp":"2026-01-01T00:30:00Z","harness":"codex","model":"gpt-5.4","prompt_len":100,"input_tokens":100,"output_tokens":50,"total_tokens":150,"cost_usd":2.5,"duration_ms":1000,"exit_code":0,"correlation":{"bead_id":"bx-001"}}`,
		`{"id":"as-002","timestamp":"2026-01-02T00:45:00Z","harness":"claude","model":"claude-sonnet-4-6","prompt_len":120,"input_tokens":1000,"output_tokens":1000,"total_tokens":2000,"duration_ms":2000,"exit_code":0,"correlation":{"bead_id":"bx-002"}}`,
		`{"id":"as-003","timestamp":"2026-01-03T00:00:00Z","harness":"codex","prompt_len":50,"input_tokens":10,"output_tokens":20,"total_tokens":30,"duration_ms":150,"exit_code":0}`,
	}
	writeSessionIndexLines(t, dir, sessions...)

	return dir
}

func TestListDocuments(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/documents", nil)
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestProcessMetricsEndpoints(t *testing.T) {
	dir := setupProcessMetricsTestDir(t)
	srv := New(":0", dir)

	t.Run("summary", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/metrics/summary", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var summary processmetrics.AggregateSummary
		if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if summary.Beads.Total != 2 || summary.Beads.Closed != 2 || summary.Beads.Reopened != 1 {
			t.Fatalf("unexpected summary counts: %+v", summary.Beads)
		}
		if summary.Sessions.Total != 3 || summary.Sessions.Correlated != 2 || summary.Sessions.Uncorrelated != 1 {
			t.Fatalf("unexpected session summary: %+v", summary.Sessions)
		}
	})

	t.Run("cost", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/metrics/cost?feature=FEAT-001", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var report processmetrics.CostReport
		if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if report.Scope != "feature" || report.FeatureID != "FEAT-001" {
			t.Fatalf("unexpected report scope: %+v", report)
		}
		if len(report.Beads) != 2 || len(report.Features) != 1 {
			t.Fatalf("unexpected cost rows: %+v", report)
		}
		if report.Beads[0].BeadID != "bx-001" || report.Beads[1].BeadID != "bx-002" {
			t.Fatalf("unexpected bead ordering: %+v", report.Beads)
		}
	})

	t.Run("cycle-time", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/metrics/cycle-time", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var report processmetrics.CycleTimeReport
		if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if report.Summary.KnownCount != 2 || len(report.Beads) != 2 {
			t.Fatalf("unexpected cycle-time summary: %+v", report.Summary)
		}
		if report.Beads[0].BeadID != "bx-001" || report.Beads[1].BeadID != "bx-002" {
			t.Fatalf("unexpected cycle-time ordering: %+v", report.Beads)
		}
	})

	t.Run("rework", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/metrics/rework", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var report processmetrics.ReworkReport
		if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if report.Summary.KnownClosed != 2 || report.Summary.KnownReopened != 1 || report.Summary.RevisionCount != 1 {
			t.Fatalf("unexpected rework summary: %+v", report.Summary)
		}
		if len(report.Beads) != 2 {
			t.Fatalf("unexpected rework rows: %+v", report.Beads)
		}
	})

	t.Run("bad query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/metrics/cost?bead=bx-001&feature=FEAT-001", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestListBeads(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)
	// Isolate state to only this project so the aggregating handler sees exactly 3 beads.
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	srv.state.RegisterProject(dir)

	req := httptest.NewRequest("GET", "/api/beads", nil)
	req.RemoteAddr = "127.0.0.1:12345"
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
	// Isolate state to only this project so the aggregating handler sees exactly 2 open beads.
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	srv.state.RegisterProject(dir)

	req := httptest.NewRequest("GET", "/api/beads?status=open", nil)
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

// setupProjectWithBeads creates a temp dir with a .ddx/beads.jsonl containing
// one open, one in_progress, and one closed bead, all prefixed with beadPrefix.
// Also isolates XDG_DATA_HOME so servers constructed against the returned dir
// do not pollute the developer's real state file (ddx-15f7ee0b Fix A).
func setupProjectWithBeads(t *testing.T, beadPrefix string) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	open := fmt.Sprintf(`{"id":"%s-open","title":"Open bead","status":"open","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`, beadPrefix)
	inProgress := fmt.Sprintf(`{"id":"%s-ip","title":"In Progress bead","status":"in_progress","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`, beadPrefix)
	closed := fmt.Sprintf(`{"id":"%s-closed","title":"Closed bead","status":"closed","priority":1,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`, beadPrefix)
	content := open + "\n" + inProgress + "\n" + closed + "\n"
	if err := os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestListBeadsAllProjects(t *testing.T) {
	dir1 := setupProjectWithBeads(t, "p1")
	dir2 := setupProjectWithBeads(t, "p2")

	srv := New(":0", dir1)
	// Isolate state to only the two test projects (avoid contamination from global state).
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	p1 := srv.state.RegisterProject(dir1)
	p2 := srv.state.RegisterProject(dir2)

	t.Run("all beads from both projects", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/beads", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var beads []struct {
			ID        string `json:"id"`
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(beads) != 6 {
			t.Fatalf("expected 6 beads (3 per project), got %d", len(beads))
		}
		for _, b := range beads {
			if b.ProjectID == "" {
				t.Errorf("bead %s is missing project_id", b.ID)
			}
			if b.ProjectID != p1.ID && b.ProjectID != p2.ID {
				t.Errorf("bead %s has unexpected project_id %q", b.ID, b.ProjectID)
			}
		}
	})

	t.Run("filter by status=open returns two beads", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/beads?status=open", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		var beads []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
			t.Fatal(err)
		}
		if len(beads) != 2 {
			t.Fatalf("expected 2 open beads (one per project), got %d", len(beads))
		}
		for _, b := range beads {
			if b.Status != "open" {
				t.Errorf("expected status=open, got %q for bead %s", b.Status, b.ID)
			}
		}
	})

	t.Run("filter by project_id returns only that project's beads", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/beads?project_id="+p1.ID, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		var beads []struct {
			ID        string `json:"id"`
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
			t.Fatal(err)
		}
		if len(beads) != 3 {
			t.Fatalf("expected 3 beads from project 1, got %d", len(beads))
		}
		for _, b := range beads {
			if b.ProjectID != p1.ID {
				t.Errorf("expected project_id=%s, got %q for bead %s", p1.ID, b.ProjectID, b.ID)
			}
		}
	})
}

// TestProjectScopedBeadRoutes verifies that /api/projects/{project}/beads/*
// routes resolve {project} via the projectScoped middleware and that the
// returned data comes from the scoped project's bead store — not the server's
// default WorkingDir and not an aggregate across projects.
func TestProjectScopedBeadRoutes(t *testing.T) {
	dir1 := setupProjectWithBeads(t, "p1")
	dir2 := setupProjectWithBeads(t, "p2")

	srv := New(":0", dir1)
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	p1 := srv.state.RegisterProject(dir1)
	p2 := srv.state.RegisterProject(dir2)

	t.Run("show scopes to project 1", func(t *testing.T) {
		url := "/api/projects/" + p1.ID + "/beads/p1-open"
		req := httptest.NewRequest("GET", url, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var b struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		if b.ID != "p1-open" {
			t.Errorf("expected p1-open, got %q", b.ID)
		}
	})

	t.Run("show scopes to project 2", func(t *testing.T) {
		url := "/api/projects/" + p2.ID + "/beads/p2-closed"
		req := httptest.NewRequest("GET", url, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var b struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		if b.ID != "p2-closed" {
			t.Errorf("expected p2-closed, got %q", b.ID)
		}
	})

	t.Run("cross-project lookup returns 404", func(t *testing.T) {
		// p1-open does not exist in project 2's bead store.
		url := "/api/projects/" + p2.ID + "/beads/p1-open"
		req := httptest.NewRequest("GET", url, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("list scopes to project", func(t *testing.T) {
		url := "/api/projects/" + p1.ID + "/beads"
		req := httptest.NewRequest("GET", url, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var beads []struct {
			ID        string `json:"id"`
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &beads); err != nil {
			t.Fatal(err)
		}
		if len(beads) != 3 {
			t.Fatalf("expected 3 beads from project 1, got %d", len(beads))
		}
		for _, b := range beads {
			if b.ProjectID != p1.ID {
				t.Errorf("expected project_id=%s, got %q for bead %s", p1.ID, b.ProjectID, b.ID)
			}
			if !strings.HasPrefix(b.ID, "p1-") {
				t.Errorf("expected p1- prefix, got %s", b.ID)
			}
		}
	})

	t.Run("status scopes to project", func(t *testing.T) {
		url := "/api/projects/" + p2.ID + "/beads/status"
		req := httptest.NewRequest("GET", url, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var counts map[string]int
		if err := json.Unmarshal(w.Body.Bytes(), &counts); err != nil {
			t.Fatal(err)
		}
		if counts["total"] != 3 {
			t.Errorf("expected total=3 (project 2's beads only), got %d", counts["total"])
		}
	})

	t.Run("unknown project returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/proj-00000000/beads", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("resolves project by path too", func(t *testing.T) {
		// Paths contain '/' which must be percent-encoded so the mux keeps them
		// inside the {project} capture rather than promoting to path segments.
		urlPath := "/api/projects/" + url.PathEscape(p1.Path) + "/beads/status"
		req := httptest.NewRequest("GET", urlPath, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestProjectLegacyRoutesSingleton verifies that legacy /api/... routes
// continue to work when exactly one project is registered (singleton
// compatibility shim).
func TestProjectLegacyRoutesSingleton(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Isolate state to exactly one project so singleton middleware kicks in.
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	srv.state.RegisterProject(dir)

	t.Run("legacy beads/status", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/beads/status", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var counts map[string]int
		if err := json.Unmarshal(w.Body.Bytes(), &counts); err != nil {
			t.Fatal(err)
		}
		if counts["total"] != 3 {
			t.Errorf("expected total=3, got %d", counts["total"])
		}
	})

	t.Run("legacy documents list", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/documents", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestBeadDepTree(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/beads/dep/tree/bx-003", nil)
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

	session1 := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","surface":"codex","canonical_target":"gpt-4","model":"gpt-4","prompt_len":100,"prompt_source":"stdin","native_session_id":"native-001","native_log_ref":"log-001","trace_id":"trace-001","span_id":"span-001","tokens":500,"input_tokens":350,"output_tokens":150,"total_tokens":500,"duration_ms":2000,"exit_code":0}`
	session2 := `{"id":"as-0002","timestamp":"2026-01-01T11:00:00Z","harness":"claude","surface":"claude","canonical_target":"sonnet","model":"sonnet","prompt_len":200,"prompt_source":"file","native_session_id":"native-002","native_log_ref":"log-002","trace_id":"trace-002","span_id":"span-002","tokens":800,"input_tokens":450,"output_tokens":350,"total_tokens":800,"duration_ms":3000,"exit_code":0}`
	writeSessionIndexLines(t, dir, session1, session2)

	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sessions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	// Most recent first
	if sessions[0]["id"] != "as-0002" {
		t.Errorf("expected most recent first (as-0002), got %v", sessions[0]["id"])
	}
	if _, ok := sessions[0]["prompt"]; ok {
		t.Errorf("did not expect prompt in session list payload: %v", sessions[0])
	}
	if _, ok := sessions[0]["response"]; ok {
		t.Errorf("did not expect response in session list payload: %v", sessions[0])
	}
	if sessions[0]["native_session_id"] != "native-002" || sessions[0]["trace_id"] != "trace-002" {
		t.Errorf("expected native refs in session list payload, got %v", sessions[0])
	}
}

func TestAgentSessionsFilterByHarness(t *testing.T) {
	dir := setupTestDir(t)

	session1 := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"tokens":500,"duration_ms":2000,"exit_code":0}`
	session2 := `{"id":"as-0002","timestamp":"2026-01-01T11:00:00Z","harness":"claude","model":"sonnet","prompt_len":200,"tokens":800,"duration_ms":3000,"exit_code":0}`
	writeSessionIndexLines(t, dir, session1, session2)

	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions?harness=codex", nil)
	req.RemoteAddr = "127.0.0.1:12345"
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

	session := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","surface":"codex","canonical_target":"gpt-4","model":"gpt-4","prompt_len":100,"prompt_source":"stdin","prompt":"inspect me","response":"done","correlation":{"bead_id":"hx-123"},"native_session_id":"native-123","native_log_ref":"log-123","trace_id":"trace-123","span_id":"span-123","tokens":500,"duration_ms":2000,"exit_code":0}`
	writeSessionIndexLines(t, dir, session)

	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions/as-0001", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sess struct {
		ID                string            `json:"id"`
		Harness           string            `json:"harness"`
		Tokens            int               `json:"tokens"`
		PromptAvailable   bool              `json:"prompt_available"`
		ResponseAvailable bool              `json:"response_available"`
		Prompt            string            `json:"prompt"`
		Response          string            `json:"response"`
		Correlation       map[string]string `json:"correlation"`
		NativeSessionID   string            `json:"native_session_id"`
		NativeLogRef      string            `json:"native_log_ref"`
		TraceID           string            `json:"trace_id"`
		SpanID            string            `json:"span_id"`
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
	if !sess.PromptAvailable || !sess.ResponseAvailable {
		t.Fatalf("expected prompt/response availability flags to be true, got %+v", sess)
	}
	if sess.Prompt != "inspect me" || sess.Response != "done" {
		t.Errorf("expected prompt/response to be returned, got %q / %q", sess.Prompt, sess.Response)
	}
	if sess.Correlation["bead_id"] != "hx-123" {
		t.Errorf("expected bead correlation, got %v", sess.Correlation)
	}
	if sess.NativeSessionID != "native-123" || sess.NativeLogRef != "log-123" || sess.TraceID != "trace-123" || sess.SpanID != "span-123" {
		t.Errorf("expected native refs in detail payload, got %+v", sess)
	}
}

func TestAgentSessionDetailUnavailableContent(t *testing.T) {
	dir := setupTestDir(t)

	session := `{"id":"as-0002","timestamp":"2026-01-01T10:05:00Z","harness":"claude","surface":"claude","canonical_target":"sonnet","model":"sonnet","prompt_len":0,"native_session_id":"native-456","trace_id":"trace-456","span_id":"span-456","duration_ms":1500,"exit_code":0}`
	writeSessionIndexLines(t, dir, session)

	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions/as-0002", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := payload["prompt_available"]; got != false {
		t.Fatalf("expected prompt_available=false, got %v", got)
	}
	if got := payload["response_available"]; got != false {
		t.Fatalf("expected response_available=false, got %v", got)
	}
	if _, ok := payload["prompt"]; ok {
		t.Fatalf("did not expect prompt field in unavailable detail payload: %v", payload)
	}
	if _, ok := payload["response"]; ok {
		t.Fatalf("did not expect response field in unavailable detail payload: %v", payload)
	}
}

func TestAgentSessionDetailNotFound(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/sessions/nonexistent", nil)
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	if len(tools) != 37 {
		t.Fatalf("expected 37 MCP tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		names[toolMap["name"].(string)] = true
	}
	expected := []string{
		"ddx_list_documents", "ddx_read_document", "ddx_search", "ddx_resolve_persona",
		"ddx_list_beads", "ddx_show_bead", "ddx_bead_ready", "ddx_bead_status",
		"ddx_bead_blocked", "ddx_bead_dep_tree",
		"ddx_bead_create", "ddx_bead_update", "ddx_bead_claim",
		"ddx_doc_graph", "ddx_doc_stale", "ddx_doc_show", "ddx_doc_deps", "ddx_doc_dependents",
		"ddx_agent_sessions",
		"ddx_exec_definitions", "ddx_exec_show", "ddx_exec_history",
		"ddx_exec_dispatch", "ddx_agent_dispatch",
		"ddx_doc_changed",
		"ddx_doc_write", "ddx_doc_history", "ddx_doc_diff",
		"ddx_list_projects", "ddx_show_project",
		"ddx_worker_list", "ddx_worker_show", "ddx_worker_log",
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

	session := `{"id":"as-0001","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"tokens":500,"duration_ms":2000,"exit_code":0}`
	writeSessionIndexLines(t, dir, session)

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
// It scrubs inherited GIT_* environment variables so tests remain isolated
// when run from inside git hooks (lefthook sets GIT_DIR, GIT_WORK_TREE, etc.).
func runGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

// --- SvelteKit SPA embed tests (Stage 4.16: SvelteKit build output verified) ---

func TestSPAServesIndexHTML(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from embedded SvelteKit SPA, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "data-sveltekit-preload-data") {
		t.Errorf("expected SvelteKit shell in response body, got: %s", body)
	}
}

func TestSPAFallbackForClientRoute(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/nodes/abc/projects/def/beads", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Deep SPA route: no static file, falls back to index.html (SvelteKit shell) → 200
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SPA fallback to index.html) for deep link, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "data-sveltekit-preload-data") {
		t.Errorf("expected SvelteKit shell for deep link fallback, got: %s", body)
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("claim failed: %d", w.Code)
	}

	// Then unclaim
	req = httptest.NewRequest("POST", "/api/beads/bx-001/unclaim", nil)
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

	// Top-level requireTrusted middleware now gates the entire /mcp endpoint.
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-trusted MCP request, got %d", w.Code)
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

	// Top-level requireTrusted middleware now gates the entire /mcp endpoint.
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-trusted MCP request, got %d", w.Code)
	}
}

func TestMCPExecDispatchTrustedAllowed(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_exec_dispatch","arguments":{"id":"bench-startup"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
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
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("expected content in response")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if strings.Contains(text, "forbidden") {
		t.Errorf("expected trusted dispatch to not be forbidden, got %q", text)
	}
}

func TestMCPAgentDispatchTrustedAllowed(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_agent_dispatch","arguments":{"harness":"claude","prompt":"hello"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
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
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("expected content in response")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if strings.Contains(text, "forbidden") {
		t.Errorf("expected trusted dispatch to not be forbidden, got %q", text)
	}
}

func TestMCPWriteToolsUntrustedForbidden(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	writeTools := []struct {
		name string
		body string
	}{
		{
			name: "ddx_bead_create",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_create","arguments":{"title":"test","type":"task"}}}`,
		},
		{
			name: "ddx_bead_update",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_update","arguments":{"id":"hx-1234","status":"closed"}}}`,
		},
		{
			name: "ddx_bead_claim",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_claim","arguments":{"id":"hx-1234"}}}`,
		},
		{
			name: "ddx_doc_write",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_doc_write","arguments":{"id":"prompts/test.md","content":"hello"}}}`,
		},
	}

	for _, tc := range writeTools {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/mcp", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "10.0.0.1:9999"
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)

			// Top-level requireTrusted middleware gates the entire /mcp endpoint.
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for untrusted %s, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestMCPWriteToolsTrustedAllowed(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	writeTools := []struct {
		name string
		body string
	}{
		{
			name: "ddx_bead_create",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_create","arguments":{"title":"test","type":"task"}}}`,
		},
		{
			name: "ddx_bead_update",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_update","arguments":{"id":"hx-1234","status":"closed"}}}`,
		},
		{
			name: "ddx_bead_claim",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_bead_claim","arguments":{"id":"hx-1234"}}}`,
		},
		{
			name: "ddx_doc_write",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_doc_write","arguments":{"id":"prompts/test.md","content":"hello"}}}`,
		},
	}

	for _, tc := range writeTools {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/mcp", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "127.0.0.1:12345"
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
			content, _ := result["content"].([]any)
			if len(content) == 0 {
				t.Fatal("expected content in response")
			}
			text, _ := content[0].(map[string]any)["text"].(string)
			if strings.Contains(text, "forbidden") {
				t.Errorf("expected trusted %s to not be forbidden, got %q", tc.name, text)
			}
		})
	}
}

func TestRESTWriteEndpointsUntrustedForbidden(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"createBead", "POST", "/api/beads", `{"title":"t"}`},
		{"updateBead", "PUT", "/api/beads/bx-001", `{"status":"closed"}`},
		{"claimBead", "POST", "/api/beads/bx-001/claim", `{}`},
		{"unclaimBead", "POST", "/api/beads/bx-001/unclaim", `{}`},
		{"reopenBead", "POST", "/api/beads/bx-001/reopen", `{}`},
		{"beadDeps", "POST", "/api/beads/bx-001/deps", `{"action":"add","dep_id":"bx-002"}`},
		{"writeDocument", "PUT", "/api/documents/prompts/test.md", `{"content":"hello"}`},
		{"docWrite", "PUT", "/api/docs/bx-001", `{"content":"hello"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "192.168.1.100:12345"
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("expected 403 for untrusted %s, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestRESTWriteEndpointsTrustedAllowed(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"createBead", "POST", "/api/beads", `{"title":"t"}`},
		{"updateBead", "PUT", "/api/beads/bx-001", `{"status":"closed"}`},
		{"claimBead", "POST", "/api/beads/bx-001/claim", `{}`},
		{"unclaimBead", "POST", "/api/beads/bx-001/unclaim", `{}`},
		{"reopenBead", "POST", "/api/beads/bx-001/reopen", `{}`},
		{"beadDeps", "POST", "/api/beads/bx-001/deps", `{"action":"add","dep_id":"bx-002"}`},
		{"writeDocument", "PUT", "/api/documents/prompts/test.md", `{"content":"hello"}`},
		{"docWrite", "PUT", "/api/docs/bx-001", `{"content":"hello"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code == http.StatusForbidden {
				t.Errorf("expected trusted %s to not be forbidden, got 403", tc.name)
			}
		})
	}
}

// TestAllNonHealthHandlersGateOnIsTrusted walks every route registered on the
// mux and verifies that non-loopback requests receive 403. /api/health and
// /api/ready are the only exceptions. If a new handler is added via the
// trusted() helper in routes() and it somehow doesn't get the gate, this test
// catches it. If a handler is registered outside routes() (bypassing the
// route() helper), it won't appear in routePatterns and the test won't cover
// it — but it also won't appear in the count, making it obvious during review.
func TestAllNonHealthHandlersGateOnIsTrusted(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	allowlist := map[string]bool{
		"GET /api/health": true,
		"GET /api/ready":  true,
	}

	if len(srv.routePatterns) == 0 {
		t.Fatal("no route patterns recorded; route() helper may have been bypassed")
	}

	for _, pattern := range srv.routePatterns {
		if allowlist[pattern] {
			continue
		}

		t.Run(pattern, func(t *testing.T) {
			method, path := trustTestSplitPattern(pattern)
			path = trustTestFillPathParams(path)

			req := httptest.NewRequest(method, path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "203.0.113.1:12345" // TEST-NET-3, guaranteed non-loopback
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("%s returned %d; want 403 — is the isTrusted() gate missing?", pattern, w.Code)
			}
		})
	}
}

// trustTestSplitPattern extracts method and path from a Go 1.22+ mux pattern.
func trustTestSplitPattern(pattern string) (method, path string) {
	if i := strings.Index(pattern, " "); i >= 0 {
		return pattern[:i], pattern[i+1:]
	}
	return "GET", pattern
}

// trustTestFillPathParams replaces {name} and {name...} placeholders with
// dummy values so the mux routes the request to the registered handler.
func trustTestFillPathParams(path string) string {
	path = strings.ReplaceAll(path, "{path...}", "test/dummy.md")
	for strings.Contains(path, "{") {
		start := strings.Index(path, "{")
		end := strings.Index(path, "}")
		if start < 0 || end < 0 || end < start {
			break
		}
		path = path[:start] + "test-id" + path[end+1:]
	}
	return path
}

func TestMCPToolsListIncludesExec(t *testing.T) {
	dir := setupExecTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
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

// setupNodeTestDir creates a temp dir and sets XDG_DATA_HOME and DDX_NODE_NAME
// so node-state tests are isolated from the real user state file.
func setupNodeTestDir(t *testing.T) (workDir string) {
	t.Helper()
	workDir = setupTestDir(t)
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "test-node")
	return workDir
}

func TestGetNode(t *testing.T) {
	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	req := httptest.NewRequest("GET", "/api/node", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var node struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if node.Name != "test-node" {
		t.Errorf("expected name=test-node, got %q", node.Name)
	}
	if !strings.HasPrefix(node.ID, "node-") {
		t.Errorf("expected id to start with 'node-', got %q", node.ID)
	}
}

func TestListProjects(t *testing.T) {
	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var projects []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Server registers workDir as a project on startup.
	if len(projects) != 1 {
		t.Fatalf("expected 1 project (the startup project), got %d", len(projects))
	}
	if projects[0].Path != workDir {
		t.Errorf("expected path=%s, got %s", workDir, projects[0].Path)
	}
	if !strings.HasPrefix(projects[0].ID, "proj-") {
		t.Errorf("expected id to start with 'proj-', got %q", projects[0].ID)
	}
}

func TestRegisterProject(t *testing.T) {
	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	// Register a second project path.
	body := `{"path":"/tmp/other-project"}`
	req := httptest.NewRequest("POST", "/api/projects/register", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entry struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry.Path != "/tmp/other-project" {
		t.Errorf("expected path=/tmp/other-project, got %s", entry.Path)
	}
	if !strings.HasPrefix(entry.ID, "proj-") {
		t.Errorf("expected id prefix 'proj-', got %q", entry.ID)
	}

	// Confirm it now appears in GET /api/projects.
	req2 := httptest.NewRequest("GET", "/api/projects", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	var projects []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	found := false
	for _, p := range projects {
		if p.Path == "/tmp/other-project" {
			found = true
		}
	}
	if !found {
		t.Error("registered project not returned by GET /api/projects")
	}
}

func TestRegisterProjectMissingPath(t *testing.T) {
	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	req := httptest.NewRequest("POST", "/api/projects/register", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegisterProjectIdempotent(t *testing.T) {
	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	body := `{"path":"/tmp/idempotent-project"}`
	for range 3 {
		req := httptest.NewRequest("POST", "/api/projects/register", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on repeat registration, got %d", w.Code)
		}
	}

	// Should still have exactly 2 projects: startup + idempotent.
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var projects []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects after 3 idempotent registrations, got %d", len(projects))
	}
}

// setupCommitsTestDir sets up an isolated state dir, a git repo with the given
// number of commits, and returns (dir, server, projectID).
func setupCommitsTestDir(t *testing.T, subjects []string) (string, *Server, string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "test-node")

	dir := setupTestDir(t)
	runGit(t, "init", dir)
	runGit(t, "-C", dir, "config", "user.email", "test@test.com")
	runGit(t, "-C", dir, "config", "user.name", "Test User")

	for i, subject := range subjects {
		name := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(name, []byte(subject), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, "-C", dir, "add", ".")
		runGit(t, "-C", dir, "commit", "-m", subject)
	}

	srv := New(":0", dir)
	return dir, srv, projectID(dir)
}

func TestListCommits(t *testing.T) {
	subjects := []string{"first commit", "second commit", "third commit"}
	_, srv, projID := setupCommitsTestDir(t, subjects)

	req := httptest.NewRequest("GET", "/api/projects/"+projID+"/commits", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var commits []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}
	// Newest first: third, second, first.
	expectedOrder := []string{"third commit", "second commit", "first commit"}
	for i, want := range expectedOrder {
		if got := commits[i]["subject"]; got != want {
			t.Errorf("commit[%d] subject: want %q, got %v", i, want, got)
		}
	}
	// Check required fields are present on the first commit.
	first := commits[0]
	for _, field := range []string{"sha", "short_sha", "author", "date", "subject", "body", "bead_refs"} {
		if _, ok := first[field]; !ok {
			t.Errorf("commit missing field %q", field)
		}
	}
	if author := first["author"]; author != "Test User" {
		t.Errorf("expected author=Test User, got %v", author)
	}
}

func TestListCommitsLimitParam(t *testing.T) {
	subjects := []string{"one", "two", "three"}
	_, srv, projID := setupCommitsTestDir(t, subjects)

	req := httptest.NewRequest("GET", "/api/projects/"+projID+"/commits?limit=1", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var commits []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit with limit=1, got %d", len(commits))
	}
}

func TestListCommitsProjectNotFound(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "test-node")
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/projects/proj-nosuch/commits", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListCommitsBeadRefs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "test-node")

	dir := setupTestDir(t)
	runGit(t, "init", dir)
	runGit(t, "-C", dir, "config", "user.email", "test@test.com")
	runGit(t, "-C", dir, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", dir, "add", ".")
	// Multi-line commit message with a body referencing a bead.
	runGit(t, "-C", dir, "commit", "-m", "feat: add thing", "-m", "Closes ddx-abc12345")

	srv := New(":0", dir)
	projID := projectID(dir)

	req := httptest.NewRequest("GET", "/api/projects/"+projID+"/commits", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var commits []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	refs, ok := commits[0]["bead_refs"].([]any)
	if !ok {
		t.Fatalf("expected bead_refs to be an array, got %T", commits[0]["bead_refs"])
	}
	found := false
	for _, r := range refs {
		if s, _ := r.(string); s == "ddx-abc12345" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected bead_refs to contain ddx-abc12345, got %v", refs)
	}
}

func TestMCPListProjects(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "test-node")

	dir := setupTestDir(t)
	srv := New(":0", dir)

	// tools/list should include ddx_list_projects.
	w := mcpRequest(t, srv, "tools/list", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ddx_list_projects") {
		t.Error("expected tools/list to include ddx_list_projects")
	}

	// tools/call should return the registered project.
	w = mcpRequest(t, srv, "tools/call", `{"name":"ddx_list_projects","arguments":{}}`)
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
		t.Fatal("expected content array")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, projectID(dir)) {
		t.Errorf("expected tool output to contain project ID %s, got %s", projectID(dir), text)
	}
}

func TestMCPShowProject(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "test-node")

	dir := setupTestDir(t)
	srv := New(":0", dir)
	projID := projectID(dir)

	// Show by ID.
	body := fmt.Sprintf(`{"name":"ddx_show_project","arguments":{"id":%q}}`, projID)
	w := mcpRequest(t, srv, "tools/call", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, _ := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("expected success, got error: %v", result)
	}
	content, _ := result["content"].([]any)
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, projID) {
		t.Errorf("expected output to contain %s, got %s", projID, text)
	}

	// Show by path.
	body = fmt.Sprintf(`{"name":"ddx_show_project","arguments":{"path":%q}}`, dir)
	w = mcpRequest(t, srv, "tools/call", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, _ = resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("expected success for path lookup, got error: %v", result)
	}
	content, _ = result["content"].([]any)
	text, _ = content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, projID) {
		t.Errorf("expected output to contain %s, got %s", projID, text)
	}

	// Missing project should error.
	body = `{"name":"ddx_show_project","arguments":{"id":"proj-nosuch"}}`
	w = mcpRequest(t, srv, "tools/call", body)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, _ = resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Errorf("expected isError=true for missing project, got %v", result)
	}
}

// TestMCPProjectAwareToolSelection verifies that project-local MCP tools
// honour an optional "project" argument: with two projects registered, passing
// project=<id> routes the tool at that project's data store; omitting project
// returns a disambiguation error; project=<path> resolves identically to the
// ID form; and an unknown project ID returns an error. Covers FEAT-002/SD-019
// AC (2), (3), and the disambiguation semantics required by the bead.
func TestMCPProjectAwareToolSelection(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "mcp-project-aware-node")

	dir1 := setupProjectWithBeads(t, "p1")
	dir2 := setupProjectWithBeads(t, "p2")

	srv := New(":0", dir1)
	// Reset state to exactly the two test projects so we get the multi-project
	// disambiguation codepath.
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	p1 := srv.state.RegisterProject(dir1)
	p2 := srv.state.RegisterProject(dir2)

	// Helper that unwraps the MCP tool result payload.
	toolCall := func(t *testing.T, body string) (text string, isErr bool) {
		t.Helper()
		w := mcpRequest(t, srv, "tools/call", body)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON-RPC response: %v", err)
		}
		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("expected result map, got %T", resp.Result)
		}
		content, _ := result["content"].([]any)
		if len(content) == 0 {
			t.Fatal("expected non-empty content")
		}
		text, _ = content[0].(map[string]any)["text"].(string)
		isErr, _ = result["isError"].(bool)
		return text, isErr
	}

	t.Run("project arg selects project 1", func(t *testing.T) {
		body := fmt.Sprintf(`{"name":"ddx_list_beads","arguments":{"project":%q}}`, p1.ID)
		text, isErr := toolCall(t, body)
		if isErr {
			t.Fatalf("unexpected error: %s", text)
		}
		if !strings.Contains(text, "p1-open") {
			t.Errorf("expected p1-open in project 1 beads, got: %s", text)
		}
		if strings.Contains(text, "p2-") {
			t.Errorf("project 1 tool call should not leak p2 beads, got: %s", text)
		}
	})

	t.Run("project arg selects project 2", func(t *testing.T) {
		body := fmt.Sprintf(`{"name":"ddx_list_beads","arguments":{"project":%q}}`, p2.ID)
		text, isErr := toolCall(t, body)
		if isErr {
			t.Fatalf("unexpected error: %s", text)
		}
		if !strings.Contains(text, "p2-open") {
			t.Errorf("expected p2-open in project 2 beads, got: %s", text)
		}
		if strings.Contains(text, "p1-") {
			t.Errorf("project 2 tool call should not leak p1 beads, got: %s", text)
		}
	})

	t.Run("project arg accepts path form", func(t *testing.T) {
		body := fmt.Sprintf(`{"name":"ddx_bead_status","arguments":{"project":%q}}`, dir1)
		text, isErr := toolCall(t, body)
		if isErr {
			t.Fatalf("unexpected error: %s", text)
		}
		var counts map[string]int
		if err := json.Unmarshal([]byte(text), &counts); err != nil {
			t.Fatalf("expected JSON counts, got: %s", text)
		}
		// setupProjectWithBeads creates 1 open, 1 in_progress, 1 closed per project.
		if counts["total"] != 3 {
			t.Errorf("expected total=3 for project 1, got %d", counts["total"])
		}
	})

	t.Run("show_bead scoped to project 1 cannot find project 2 beads", func(t *testing.T) {
		// p2-closed is in project 2; asking for it scoped to project 1 must error.
		body := fmt.Sprintf(`{"name":"ddx_show_bead","arguments":{"id":"p2-closed","project":%q}}`, p1.ID)
		text, isErr := toolCall(t, body)
		if !isErr {
			t.Errorf("expected isError=true for cross-project lookup, got: %s", text)
		}
	})

	t.Run("omitted project arg with >1 project returns disambiguation error", func(t *testing.T) {
		body := `{"name":"ddx_list_beads","arguments":{}}`
		text, isErr := toolCall(t, body)
		if !isErr {
			t.Fatalf("expected disambiguation error, got success: %s", text)
		}
		if !strings.Contains(text, "multiple projects") {
			t.Errorf("expected 'multiple projects' in error, got: %s", text)
		}
	})

	t.Run("unknown project id returns error", func(t *testing.T) {
		body := `{"name":"ddx_list_beads","arguments":{"project":"proj-doesnotexist"}}`
		text, isErr := toolCall(t, body)
		if !isErr {
			t.Fatalf("expected error for unknown project, got success: %s", text)
		}
		if !strings.Contains(text, "project not found") {
			t.Errorf("expected 'project not found' in error, got: %s", text)
		}
	})

	t.Run("singleton compat: omitted project arg works when exactly one project registered", func(t *testing.T) {
		// Reset state to a single project and re-invoke without the project arg.
		srv.state.mu.Lock()
		srv.state.Projects = nil
		srv.state.mu.Unlock()
		srv.state.RegisterProject(dir1)

		body := `{"name":"ddx_list_beads","arguments":{}}`
		text, isErr := toolCall(t, body)
		if isErr {
			t.Fatalf("expected singleton compat to succeed, got error: %s", text)
		}
		if !strings.Contains(text, "p1-open") {
			t.Errorf("expected p1-open in singleton call output, got: %s", text)
		}
	})
}

// TC-022: GET /api/agent/workers returns workers from multiple registered
// projects in a single response array.
func TestAgentWorkersAggregatesAcrossProjects(t *testing.T) {
	// Isolate server state from the real user state file.
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "test-node-aggregate")

	rootA := setupTestDir(t)
	rootB := t.TempDir()

	// Write a pre-canned worker record under project A (older).
	writeTestWorkerRecord(t, rootA, "w-aaa111aaa111", WorkerRecord{
		ID:          "w-aaa111aaa111",
		Kind:        "execute-loop",
		State:       "exited",
		ProjectRoot: rootA,
		StartedAt:   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	})

	// Write a pre-canned worker record under project B (newer).
	writeTestWorkerRecord(t, rootB, "w-bbb222bbb222", WorkerRecord{
		ID:          "w-bbb222bbb222",
		Kind:        "execute-loop",
		State:       "exited",
		ProjectRoot: rootB,
		StartedAt:   time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
	})

	srv := New(":0", rootA)
	// Register project B so the server knows about it.
	srv.state.RegisterProject(rootB)

	req := httptest.NewRequest("GET", "/api/agent/workers", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var workers []WorkerRecord
	if err := json.Unmarshal(w.Body.Bytes(), &workers); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	ids := map[string]bool{}
	for _, wr := range workers {
		ids[wr.ID] = true
	}

	if !ids["w-aaa111aaa111"] {
		t.Error("expected worker from project A (w-aaa111aaa111) in /api/agent/workers response")
	}
	if !ids["w-bbb222bbb222"] {
		t.Error("expected worker from project B (w-bbb222bbb222) in /api/agent/workers response")
	}

	// Verify ordering: worker B started later so it must appear first.
	if len(workers) >= 2 {
		// Find positions of the two known workers.
		posA, posB := -1, -1
		for i, wr := range workers {
			if wr.ID == "w-aaa111aaa111" {
				posA = i
			}
			if wr.ID == "w-bbb222bbb222" {
				posB = i
			}
		}
		if posA >= 0 && posB >= 0 && posB > posA {
			t.Errorf("expected newer worker B (pos %d) to appear before older worker A (pos %d)", posB, posA)
		}
	}
}

// writeTestWorkerRecord writes a WorkerRecord as status.json under the project's
// .ddx/workers/<id>/ directory — the same layout used by WorkerManager.
func writeTestWorkerRecord(t *testing.T, projectRoot, id string, rec WorkerRecord) {
	t.Helper()
	dir := filepath.Join(projectRoot, ".ddx", "workers", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Project registry: deduplication, linked-worktree resolution, reachability
// sweep, and startup migration.
// ---------------------------------------------------------------------------

// TestRegisterProjectDeduplicate registers the same project path 5 times and
// asserts that GET /api/projects returns exactly 1 project whose last_seen is
// at or after the time of the first registration call.
func TestRegisterProjectDeduplicate(t *testing.T) {
	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	start := time.Now()

	for range 5 {
		body := fmt.Sprintf(`{"path":%q}`, workDir)
		req := httptest.NewRequest("POST", "/api/projects/register", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var projects []struct {
		Path     string    `json:"path"`
		LastSeen time.Time `json:"last_seen"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project after 5+1 duplicate registrations, got %d", len(projects))
	}
	if projects[0].LastSeen.Before(start) {
		t.Errorf("last_seen %v should be >= start of registrations %v", projects[0].LastSeen, start)
	}
}

// TestRegisterProjectLinkedWorktree registers a path inside a git linked
// worktree and asserts that the server stores the PRIMARY worktree path, not
// the linked worktree path.
func TestRegisterProjectLinkedWorktree(t *testing.T) {
	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	// Create a primary git repo with one commit.
	primary := t.TempDir()
	runGit(t, "init", primary)
	runGit(t, "-C", primary, "config", "user.email", "test@test.com")
	runGit(t, "-C", primary, "config", "user.name", "Test")
	sentinel := filepath.Join(primary, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("primary"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", primary, "add", ".")
	runGit(t, "-C", primary, "commit", "-m", "init")

	// Create a linked worktree.
	linked := filepath.Join(t.TempDir(), "linked")
	runGit(t, "-C", primary, "worktree", "add", "--detach", linked)
	t.Cleanup(func() { runGit(t, "-C", primary, "worktree", "remove", "--force", linked) })

	// Register the linked worktree path.
	body := fmt.Sprintf(`{"path":%q}`, linked)
	req := httptest.NewRequest("POST", "/api/projects/register", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entry struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Canonicalize primary for comparison (handles /tmp symlinks on macOS).
	canonicalPrimary := canonicalizePath(primary)
	if entry.Path != canonicalPrimary {
		t.Errorf("expected path=%q (primary worktree), got %q (linked worktree)", canonicalPrimary, entry.Path)
	}
}

// TestSweepProjectsUnreachable registers a project, deletes its directory,
// runs SweepProjects, and asserts the project is marked unreachable with a
// tombstone timestamp. It then verifies GET /api/projects hides it by default
// and exposes it with ?include_unreachable=true.
func TestSweepProjectsUnreachable(t *testing.T) {
	// The test registers a t.TempDir() path (under /tmp) — the test-dir
	// sweep would drop it unconditionally. This test targets the tombstone
	// semantic for missing paths, so disable the filter.
	withTestDirFilterDisabled(t)

	workDir := setupNodeTestDir(t)
	srv := New(":0", workDir)

	// Create and register an extra project directory.
	projDir := t.TempDir()
	body := fmt.Sprintf(`{"path":%q}`, projDir)
	req := httptest.NewRequest("POST", "/api/projects/register", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("register: expected 200, got %d", w.Code)
	}

	// Delete the project directory.
	if err := os.RemoveAll(projDir); err != nil {
		t.Fatal(err)
	}

	// Run the reachability sweep.
	swept := srv.state.SweepProjects()

	// Find the swept entry for projDir — path has been canonicalized.
	var found *ProjectEntry
	for i := range swept {
		if swept[i].Path == canonicalizePath(projDir) {
			found = &swept[i]
			break
		}
	}
	if found == nil {
		t.Fatal("swept entry for deleted project not found in SweepProjects result")
	}
	if !found.Unreachable {
		t.Error("expected Unreachable=true after path deleted")
	}
	if found.TombstonedAt == nil {
		t.Error("expected TombstonedAt to be set")
	}

	// Default GET /api/projects should hide unreachable entries.
	req2 := httptest.NewRequest("GET", "/api/projects", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	var defaultList []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &defaultList); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, p := range defaultList {
		if p.Path == canonicalizePath(projDir) {
			t.Error("unreachable project should be hidden from default listing")
		}
	}

	// With ?include_unreachable=true it should appear.
	req3 := httptest.NewRequest("GET", "/api/projects?include_unreachable=true", nil)
	req3.RemoteAddr = "127.0.0.1:12345"
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)
	var fullList []struct {
		Path        string `json:"path"`
		Unreachable bool   `json:"unreachable"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &fullList); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	found2 := false
	for _, p := range fullList {
		if p.Path == canonicalizePath(projDir) {
			found2 = true
			if !p.Unreachable {
				t.Error("expected unreachable=true in full listing")
			}
		}
	}
	if !found2 {
		t.Error("unreachable project should appear with ?include_unreachable=true")
	}
}

// TestMigrateDeduplicatesDuplicateEntries writes a state file with 50
// duplicate entries for the same canonical path, starts the server, and
// asserts the post-startup state has exactly 1 entry for that path.
func TestMigrateDeduplicatesDuplicateEntries(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "test-node")
	// Uses t.TempDir() as dupPath — the default test-dir filter would drop
	// every duplicate. This test targets the dedupe semantic, not the
	// filter, so disable the filter here.
	withTestDirFilterDisabled(t)

	workDir := setupTestDir(t)
	xdgDir := os.Getenv("XDG_DATA_HOME")

	// Use a real directory that exists so the reachability sweep keeps it.
	dupPath := t.TempDir()

	// Build a state file with 50 duplicate entries for dupPath.
	type entry struct {
		ID           string    `json:"id"`
		Name         string    `json:"name"`
		Path         string    `json:"path"`
		RegisteredAt time.Time `json:"registered_at"`
		LastSeen     time.Time `json:"last_seen"`
	}
	type stateFile struct {
		SchemaVersion string    `json:"schema_version"`
		Node          NodeState `json:"node"`
		Projects      []entry   `json:"projects"`
	}
	entries := make([]entry, 50)
	base := time.Now().UTC().Add(-time.Hour)
	for i := range 50 {
		entries[i] = entry{
			ID:           fmt.Sprintf("proj-dup%02d", i),
			Name:         "dup",
			Path:         dupPath,
			RegisteredAt: base.Add(time.Duration(i) * time.Second),
			LastSeen:     base.Add(time.Duration(i) * time.Second),
		}
	}
	sf := stateFile{
		SchemaVersion: "1",
		Node:          NodeState{Name: "test-node", ID: "node-test"},
		Projects:      entries,
	}
	stateDir := filepath.Join(xdgDir, "ddx")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "server-state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Start the server — migration runs during loadServerState.
	srv := New(":0", workDir)

	// GET /api/projects?include_unreachable=true to see everything.
	req := httptest.NewRequest("GET", "/api/projects?include_unreachable=true", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var projects []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Count entries for dupPath — must be exactly 1 after migration.
	count := 0
	for _, p := range projects {
		if p.Path == dupPath {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 entry for dupPath after migration, got %d (total projects: %d)", count, len(projects))
	}
}

// TestProjectScopedExecRoutes verifies that /api/projects/{project}/exec/*
// routes resolve {project} via the projectScoped middleware and serve data
// from the scoped project's exec store — not the server's default WorkingDir.
func TestProjectScopedExecRoutes(t *testing.T) {
	dir1 := setupExecTestDir(t)
	dir2 := setupTestDir(t) // no exec data

	srv := New(":0", dir1)
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	p1 := srv.state.RegisterProject(dir1)
	p2 := srv.state.RegisterProject(dir2)

	t.Run("definitions list scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/exec/definitions", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var defs []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &defs); err != nil {
			t.Fatal(err)
		}
		if len(defs) != 1 || defs[0].ID != "bench-startup" {
			t.Errorf("expected bench-startup, got %+v", defs)
		}
	})

	t.Run("definitions list empty for project 2", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p2.ID+"/exec/definitions", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var defs []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &defs); err != nil {
			t.Fatal(err)
		}
		if len(defs) != 0 {
			t.Errorf("expected 0 definitions for project 2, got %d", len(defs))
		}
	})

	t.Run("definition show scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/exec/definitions/bench-startup", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cross-project definition show returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p2.ID+"/exec/definitions/bench-startup", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("runs list scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/exec/runs", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var runs []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
			t.Fatal(err)
		}
		if len(runs) != 1 {
			t.Errorf("expected 1 run, got %d", len(runs))
		}
	})

	t.Run("run show scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/exec/runs/bench-startup@2026-04-01T10:00:00Z-1", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("run log scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/exec/runs/bench-startup@2026-04-01T10:00:00Z-1/log", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var logs struct {
			Stdout string `json:"stdout"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &logs); err != nil {
			t.Fatal(err)
		}
		if logs.Stdout != "7.2 ms" {
			t.Errorf("expected stdout=7.2 ms, got %q", logs.Stdout)
		}
	})

	t.Run("exec dispatch 404s for unknown project", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/projects/proj-00000000/exec/run/bench-startup", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("exec dispatch forbidden from non-localhost", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/projects/"+p1.ID+"/exec/run/bench-startup", nil)
		req.RemoteAddr = "203.0.113.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestProjectScopedAgentSessionsAndMetrics verifies that scoped agent/sessions
// and metrics routes serve data rooted at the resolved project, not the
// server's default WorkingDir.
func TestProjectScopedAgentSessionsAndMetrics(t *testing.T) {
	// Project 1 has sessions; project 2 has no sessions.
	dir1 := setupTestDir(t)
	s1 := `{"id":"as-P1-A","timestamp":"2026-01-01T10:00:00Z","harness":"codex","model":"gpt-4","prompt_len":100,"duration_ms":1000,"exit_code":0}`
	s2 := `{"id":"as-P1-B","timestamp":"2026-01-01T11:00:00Z","harness":"claude","model":"sonnet","prompt_len":200,"duration_ms":2000,"exit_code":0}`
	writeSessionIndexLines(t, dir1, s1, s2)

	dir2 := setupTestDir(t)

	srv := New(":0", dir1)
	srv.state.mu.Lock()
	srv.state.Projects = nil
	srv.state.mu.Unlock()
	p1 := srv.state.RegisterProject(dir1)
	p2 := srv.state.RegisterProject(dir2)

	t.Run("sessions list scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/agent/sessions", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var sessions []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
			t.Fatal(err)
		}
		if len(sessions) != 2 {
			t.Fatalf("expected 2 sessions, got %d", len(sessions))
		}
	})

	t.Run("sessions list empty for project 2", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p2.ID+"/agent/sessions", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var sessions []any
		if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
			t.Fatal(err)
		}
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions for project 2, got %d", len(sessions))
		}
	})

	t.Run("session detail scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/agent/sessions/as-P1-A", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cross-project session detail returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p2.ID+"/agent/sessions/as-P1-A", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("metrics summary scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/metrics/summary", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("metrics cost scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/metrics/cost", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("metrics cycle-time scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/metrics/cycle-time", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("metrics rework scopes to project 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+p1.ID+"/metrics/rework", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("metrics summary unknown project returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/proj-00000000/metrics/summary", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestProjectScopedAgentWorkerRoutes verifies that scoped worker routes
// resolve the project via projectScoped middleware and serve records from
// that project's worker store only.
func TestProjectScopedAgentWorkerRoutes(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "test-node-scoped-workers")

	rootA := setupTestDir(t)
	rootB := setupTestDir(t)

	writeTestWorkerRecord(t, rootA, "w-scoped-A", WorkerRecord{
		ID:          "w-scoped-A",
		Kind:        "execute-loop",
		State:       "exited",
		ProjectRoot: rootA,
		StartedAt:   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	})
	writeTestWorkerRecord(t, rootB, "w-scoped-B", WorkerRecord{
		ID:          "w-scoped-B",
		Kind:        "execute-loop",
		State:       "exited",
		ProjectRoot: rootB,
		StartedAt:   time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
	})

	srv := New(":0", rootA)
	pA, _ := srv.state.GetProjectByPath(rootA)
	pB := srv.state.RegisterProject(rootB)

	t.Run("agent workers scoped to project A", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+pA.ID+"/agent/workers", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var workers []WorkerRecord
		if err := json.Unmarshal(w.Body.Bytes(), &workers); err != nil {
			t.Fatal(err)
		}
		if len(workers) != 1 || workers[0].ID != "w-scoped-A" {
			t.Errorf("expected only w-scoped-A, got %+v", workers)
		}
	})

	t.Run("agent workers scoped to project B", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+pB.ID+"/agent/workers", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var workers []WorkerRecord
		if err := json.Unmarshal(w.Body.Bytes(), &workers); err != nil {
			t.Fatal(err)
		}
		if len(workers) != 1 || workers[0].ID != "w-scoped-B" {
			t.Errorf("expected only w-scoped-B, got %+v", workers)
		}
	})

	t.Run("agent worker show scoped to project B", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+pB.ID+"/agent/workers/w-scoped-B", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var rec WorkerRecord
		if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		if rec.ID != "w-scoped-B" {
			t.Errorf("expected w-scoped-B, got %s", rec.ID)
		}
	})

	t.Run("cross-project worker show returns 404", func(t *testing.T) {
		// w-scoped-A exists under project A, but we ask project B for it.
		req := httptest.NewRequest("GET", "/api/projects/"+pB.ID+"/agent/workers/w-scoped-A", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("agent coordinators scoped returns array", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/"+pA.ID+"/agent/coordinators", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var entries []CoordinatorMetricsEntry
		if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
			t.Fatalf("expected JSON array of coordinator entries: %v", err)
		}
	})

	t.Run("unknown project 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/projects/proj-deadbeef/agent/workers", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}
