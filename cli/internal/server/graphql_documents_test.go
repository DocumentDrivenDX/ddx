package server

// TC-GQL-004..005: GraphQL Query resolvers for documents and docGraph.
//
// Integration tests exercising the real ServerState + docgraph path through the
// GraphQL handler. Each test starts a server backed by real state and fires a
// POST /graphql request, verifying the response contains the expected values.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupDocTestDir creates a temp directory with docgraph markdown documents.
func setupDocTestDir(t *testing.T) (dir string, docIDs []string) {
	t.Helper()
	dir = setupTestDir(t)

	docs := []struct {
		path    string
		id      string
		content string
	}{
		{
			path:    filepath.Join(dir, "docs", "alpha.md"),
			id:      "alpha",
			content: "---\nddx:\n  id: alpha\n---\n# Alpha\n\nFirst document.\n",
		},
		{
			path:    filepath.Join(dir, "docs", "beta.md"),
			id:      "beta",
			content: "---\nddx:\n  id: beta\n  depends_on:\n    - alpha\n---\n# Beta\n\nSecond document.\n",
		},
	}

	for _, d := range docs {
		if err := os.MkdirAll(filepath.Dir(d.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d.path, []byte(d.content), 0o644); err != nil {
			t.Fatal(err)
		}
		docIDs = append(docIDs, d.id)
	}
	return dir, docIDs
}

// TC-GQL-004: Query.documents returns paginated documents from the working dir.
func TestGraphQLDocuments(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir, docIDs := setupDocTestDir(t)
	srv := New(":0", workDir)

	body := `{"query": "{ documents { edges { node { id path title dependsOn dependents } } totalCount } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Documents struct {
				Edges []struct {
					Node struct {
						ID         string   `json:"id"`
						Path       string   `json:"path"`
						Title      string   `json:"title"`
						DependsOn  []string `json:"dependsOn"`
						Dependents []string `json:"dependents"`
					} `json:"node"`
				} `json:"edges"`
				TotalCount int `json:"totalCount"`
			} `json:"documents"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}
	if resp.Data.Documents.TotalCount != len(docIDs) {
		t.Errorf("expected totalCount=%d, got %d", len(docIDs), resp.Data.Documents.TotalCount)
	}
	if len(resp.Data.Documents.Edges) != len(docIDs) {
		t.Errorf("expected %d edges, got %d", len(docIDs), len(resp.Data.Documents.Edges))
	}

	// Verify the alpha doc has beta as a dependent.
	foundAlpha := false
	for _, e := range resp.Data.Documents.Edges {
		if e.Node.ID == "alpha" {
			foundAlpha = true
			if len(e.Node.Dependents) != 1 || e.Node.Dependents[0] != "beta" {
				t.Errorf("expected alpha.dependents=[beta], got %v", e.Node.Dependents)
			}
		}
	}
	if !foundAlpha {
		t.Error("expected document with id 'alpha' in results")
	}
}

// Bead ddx-12cae4dd: documents query must not surface agent worktree copies
// checked out under .claude/worktrees/, and returned paths must be relative
// (never absolute) so the web UI can build valid routes.
func TestGraphQLDocuments_SkipsClaudeWorktreesAndUsesRelativePaths(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir := setupTestDir(t)

	files := map[string]string{
		filepath.Join(workDir, "docs", "helix", "00-discover", "research", "AC-AGENT-001-agent-harness-ac.md"): "---\nddx:\n  id: agent-harness-ac\n---\n# Agent Harness AC\n",
		// Agent scratch copy with frontmatter — must be ignored.
		filepath.Join(workDir, ".claude", "worktrees", "agent-a0673989", "docs", "helix", "00-discover", "research", "AC-AGENT-001-agent-harness-ac.md"): "---\nddx:\n  id: worktree-shadow\n---\n# Shadow Copy\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	srv := New(":0", workDir)

	body := `{"query": "{ documents { edges { node { id path } } totalCount } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Documents struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Path string `json:"path"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"documents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}

	sawCanonical := false
	for _, e := range resp.Data.Documents.Edges {
		path := filepath.ToSlash(e.Node.Path)
		if strings.HasPrefix(path, "/") {
			t.Errorf("document %q has absolute path %q (leaking filesystem root into web URL)", e.Node.ID, e.Node.Path)
		}
		if strings.Contains(path, ".claude/") {
			t.Errorf("document %q path %q is inside .claude/ (worktree scratch must not be surfaced)", e.Node.ID, e.Node.Path)
		}
		if e.Node.ID == "worktree-shadow" {
			t.Errorf("worktree shadow document was surfaced in documents query")
		}
		if e.Node.ID == "agent-harness-ac" {
			sawCanonical = true
			if want := "docs/helix/00-discover/research/AC-AGENT-001-agent-harness-ac.md"; path != want {
				t.Errorf("got agent-harness-ac path %q, want %q", e.Node.Path, want)
			}
		}
	}
	if !sawCanonical {
		t.Error("expected canonical docs/helix/00-discover/research/AC-AGENT-001-agent-harness-ac.md document in response")
	}
}

// Bead ddx-12cae4dd AC#5: documentByPath must resolve documents walked by the
// docgraph (under workingDir), not only those under the configured library.
// Before the fix, clicking a docgraph entry outside the library returned null
// and the web UI rendered "Document not found".
func TestGraphQLDocumentByPath_ResolvesDocgraphTrackedDoc(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir := setupTestDir(t)
	docPath := filepath.Join(workDir, "docs", "helix", "00-discover", "research", "AC-AGENT-001-agent-harness-ac.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nddx:\n  id: agent-harness-ac\n---\n# Agent Harness AC\n\nBody content.\n"
	if err := os.WriteFile(docPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(":0", workDir)

	query := `{"query": "{ documentByPath(path: \"docs/helix/00-discover/research/AC-AGENT-001-agent-harness-ac.md\") { id path content } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(query))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			DocumentByPath *struct {
				ID      string `json:"id"`
				Path    string `json:"path"`
				Content string `json:"content"`
			} `json:"documentByPath"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if resp.Data.DocumentByPath == nil {
		t.Fatalf("documentByPath returned null for docgraph-tracked doc; body: %s", w.Body.String())
	}
	if resp.Data.DocumentByPath.ID != "agent-harness-ac" {
		t.Errorf("got id %q, want %q", resp.Data.DocumentByPath.ID, "agent-harness-ac")
	}
	if !strings.Contains(resp.Data.DocumentByPath.Content, "Body content.") {
		t.Errorf("content missing expected body text: %q", resp.Data.DocumentByPath.Content)
	}
	if strings.HasPrefix(resp.Data.DocumentByPath.Path, "/") {
		t.Errorf("path %q leaks absolute filesystem root", resp.Data.DocumentByPath.Path)
	}
}

// Bead ddx-12cae4dd AC#4: tolerate legacy absolute document paths emitted
// before docgraph paths were normalised, while still returning the clean
// relative path expected by current callers.
func TestGraphQLDocumentByPath_ResolvesLegacyAbsoluteDocgraphPath(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir := setupTestDir(t)
	docPath := filepath.Join(workDir, "docs", "helix", "00-discover", "research", "AC-AGENT-001-agent-harness-ac.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nddx:\n  id: agent-harness-ac\n---\n# Agent Harness AC\n\nLegacy absolute path content.\n"
	if err := os.WriteFile(docPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(":0", workDir)

	payload, err := json.Marshal(map[string]any{
		"query":     `query DocumentByPath($path: String!) { documentByPath(path: $path) { id path content } }`,
		"variables": map[string]string{"path": docPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			DocumentByPath *struct {
				ID      string `json:"id"`
				Path    string `json:"path"`
				Content string `json:"content"`
			} `json:"documentByPath"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if resp.Data.DocumentByPath == nil {
		t.Fatalf("documentByPath returned null for legacy absolute path; body: %s", w.Body.String())
	}
	if resp.Data.DocumentByPath.ID != "agent-harness-ac" {
		t.Errorf("got id %q, want %q", resp.Data.DocumentByPath.ID, "agent-harness-ac")
	}
	if got, want := filepath.ToSlash(resp.Data.DocumentByPath.Path), "docs/helix/00-discover/research/AC-AGENT-001-agent-harness-ac.md"; got != want {
		t.Errorf("got returned path %q, want %q", got, want)
	}
	if !strings.Contains(resp.Data.DocumentByPath.Content, "Legacy absolute path content.") {
		t.Errorf("content missing expected body text: %q", resp.Data.DocumentByPath.Content)
	}
}

// TC-GQL-005: Query.docGraph returns the full document dependency graph.
func TestGraphQLDocGraph(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir, docIDs := setupDocTestDir(t)
	srv := New(":0", workDir)

	body := `{"query": "{ docGraph { rootDir documents { id } pathToId dependents warnings } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			DocGraph struct {
				RootDir   string `json:"rootDir"`
				Documents []struct {
					ID string `json:"id"`
				} `json:"documents"`
				PathToID   string   `json:"pathToId"`
				Dependents string   `json:"dependents"`
				Warnings   []string `json:"warnings"`
			} `json:"docGraph"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}

	dg := resp.Data.DocGraph
	if dg.RootDir == "" {
		t.Error("expected non-empty rootDir")
	}
	if len(dg.Documents) != len(docIDs) {
		t.Errorf("expected %d documents, got %d", len(docIDs), len(dg.Documents))
	}

	// pathToId should be a valid JSON object.
	var pathToID map[string]string
	if err := json.Unmarshal([]byte(dg.PathToID), &pathToID); err != nil {
		t.Errorf("pathToId is not valid JSON: %v", err)
	}
	if len(pathToID) != len(docIDs) {
		t.Errorf("expected pathToId to have %d entries, got %d", len(docIDs), len(pathToID))
	}

	// dependents should be a valid JSON object.
	var dependents map[string][]string
	if err := json.Unmarshal([]byte(dg.Dependents), &dependents); err != nil {
		t.Errorf("dependents is not valid JSON: %v", err)
	}
	// beta depends on alpha, so alpha should have beta as a dependent.
	if betaDeps, ok := dependents["alpha"]; !ok || len(betaDeps) != 1 || betaDeps[0] != "beta" {
		t.Errorf("expected dependents[alpha]=[beta], got %v", dependents["alpha"])
	}
}

func TestGraphQLDocGraphIssues(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-doc-test-node")

	workDir := setupTestDir(t)
	files := map[string]string{
		"docs/alpha.md": "---\nddx:\n  id: shared.id\n---\n# Alpha\n",
		"docs/beta.md":  "---\nddx:\n  id: shared.id\n---\n# Beta\n",
		"docs/gamma.md": "---\nddx:\n  id: doc.gamma\n  depends_on:\n    - ghost.doc\n---\n# Gamma\n",
	}
	for rel, content := range files {
		path := filepath.Join(workDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	srv := New(":0", workDir)
	body := `{"query": "{ docGraphIssues { kind path id message relatedPath } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			DocGraphIssues []struct {
				Kind        string  `json:"kind"`
				Path        *string `json:"path"`
				ID          *string `json:"id"`
				Message     string  `json:"message"`
				RelatedPath *string `json:"relatedPath"`
			} `json:"docGraphIssues"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}

	if len(resp.Data.DocGraphIssues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %#v", len(resp.Data.DocGraphIssues), resp.Data.DocGraphIssues)
	}
	seen := map[string]bool{}
	for _, issue := range resp.Data.DocGraphIssues {
		seen[issue.Kind] = true
		if issue.Message == "" {
			t.Errorf("issue %q has empty message", issue.Kind)
		}
		if issue.Kind == "duplicate_id" && issue.RelatedPath == nil {
			t.Error("duplicate_id issue should expose relatedPath")
		}
		if issue.Kind == "missing_dep" && (issue.ID == nil || *issue.ID != "ghost.doc") {
			t.Errorf("missing_dep issue id = %v, want ghost.doc", issue.ID)
		}
	}
	for _, kind := range []string{"duplicate_id", "missing_dep"} {
		if !seen[kind] {
			t.Errorf("expected %s issue in docGraphIssues response", kind)
		}
	}
}
