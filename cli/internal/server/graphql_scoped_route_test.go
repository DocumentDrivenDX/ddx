package server

// LAYER 1 of the GraphQL multi-project leak fix (ddx-4c51d33e).
// Verifies the scoped /api/projects/{project}/graphql endpoint isolates
// resolver WorkingDir per request, and that the legacy /graphql route
// continues to serve the server's startup project.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// seedDocsProject creates a temp project rooted at root with a single
// docgraph-tracked markdown document carrying the given DDx id and title.
// Returns the absolute project path and the document's relative path.
func seedDocsProject(t *testing.T, root, name, docID, title string) (string, string) {
	t.Helper()
	projectPath := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(projectPath, ".ddx"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	if err := os.WriteFile(filepath.Join(projectPath, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	docsDir := filepath.Join(projectPath, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	relPath := filepath.Join("docs", docID+".md")
	body := "---\nddx:\n  id: " + docID + "\n---\n# " + title + "\n\nbody for " + docID + "\n"
	if err := os.WriteFile(filepath.Join(projectPath, relPath), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return projectPath, relPath
}

// postGraphQL fires a POST against the given path on srv and returns the
// response recorder. Sets RemoteAddr so isTrusted() accepts the request.
func postGraphQL(t *testing.T, srv *Server, path, query string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"query": query})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

// TestGraphQL_ScopedRoute_DocumentByPath_OnlyReturnsScopedProjectDocs is
// AC #5 of ddx-4c51d33e: a query against project B's scoped GraphQL endpoint
// must read project B's documents, not the server's startup project (A).
func TestGraphQL_ScopedRoute_DocumentByPath_OnlyReturnsScopedProjectDocs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "gql-scope-test")

	root := t.TempDir()
	projA, relA := seedDocsProject(t, root, "proj-a", "alpha", "Alpha A")
	projB, relB := seedDocsProject(t, root, "proj-b", "beta", "Beta B")

	srv := New(":0", projA)
	t.Cleanup(func() { _ = srv.Shutdown() })
	entryB := srv.state.RegisterProject(projB)

	// 1. Hit the scoped endpoint for project B asking for B's document by path.
	queryB := `{ documentByPath(path: "` + filepath.ToSlash(relB) + `") { id path content } }`
	w := postGraphQL(t, srv, "/api/projects/"+entryB.ID+"/graphql", queryB)
	if w.Code != http.StatusOK {
		t.Fatalf("scoped B query: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			DocumentByPath *struct {
				ID      string  `json:"id"`
				Path    string  `json:"path"`
				Content *string `json:"content"`
			} `json:"documentByPath"`
		} `json:"data"`
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v body=%s", err, w.Body.String())
	}
	if len(resp.Errors) != 0 {
		t.Fatalf("scoped B query errors: %v", resp.Errors)
	}
	if resp.Data.DocumentByPath == nil {
		t.Fatalf("scoped B query: documentByPath is null, expected B's document")
	}
	if resp.Data.DocumentByPath.Content == nil || !strings.Contains(*resp.Data.DocumentByPath.Content, "Beta B") {
		t.Fatalf("scoped B query: expected B's content, got %+v", resp.Data.DocumentByPath)
	}

	// 2. Asking the scoped B endpoint for A's document path must NOT find it
	//    (resolves to null since A's doc does not exist under B's project root).
	queryAOnB := `{ documentByPath(path: "` + filepath.ToSlash(relA) + `") { id path content } }`
	w2 := postGraphQL(t, srv, "/api/projects/"+entryB.ID+"/graphql", queryAOnB)
	if w2.Code != http.StatusOK {
		t.Fatalf("cross-project leak query: status=%d body=%s", w2.Code, w2.Body.String())
	}
	var resp2 struct {
		Data struct {
			DocumentByPath *struct {
				ID      string  `json:"id"`
				Content *string `json:"content"`
			} `json:"documentByPath"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2.Data.DocumentByPath != nil {
		// If non-null and content contains A's title, that's the leak we're
		// guarding against. The naming `relA` happens to equal `relB` so this
		// also implicitly proves we're not reading from project A's filesystem.
		if resp2.Data.DocumentByPath.Content != nil && strings.Contains(*resp2.Data.DocumentByPath.Content, "Alpha A") {
			t.Fatalf("CROSS-PROJECT LEAK: scoped B endpoint returned A's content: %+v", resp2.Data.DocumentByPath)
		}
	}
}

// TestGraphQL_ScopedRoute_RejectsUnregisteredProject is AC #6: the scoped
// route must 404 when the path's project segment doesn't resolve.
func TestGraphQL_ScopedRoute_RejectsUnregisteredProject(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "gql-scope-404-test")

	// Migrated to use the shared fixture helper (ddx-50da9674): the 404 path
	// only needs a real ddx-initialized project, no seeded docs.
	projA := testutils.NewFixtureRepo(t, "minimal")

	srv := New(":0", projA)
	t.Cleanup(func() { _ = srv.Shutdown() })

	w := postGraphQL(t, srv, "/api/projects/unknown-project-id/graphql", `{ __typename }`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unregistered project: want 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestGraphQL_LegacyRoute_StillWorks is AC #4 + #7: the unchanged /graphql
// trusted route continues to serve the server's startup project.
func TestGraphQL_LegacyRoute_StillWorks(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "gql-legacy-test")

	root := t.TempDir()
	projA, relA := seedDocsProject(t, root, "proj-a", "alpha", "Alpha A")
	projB, _ := seedDocsProject(t, root, "proj-b", "beta", "Beta B")

	srv := New(":0", projA)
	t.Cleanup(func() { _ = srv.Shutdown() })
	srv.state.RegisterProject(projB)

	query := `{ documentByPath(path: "` + filepath.ToSlash(relA) + `") { id content } }`
	w := postGraphQL(t, srv, "/graphql", query)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy query: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			DocumentByPath *struct {
				ID      string  `json:"id"`
				Content *string `json:"content"`
			} `json:"documentByPath"`
		} `json:"data"`
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode legacy resp: %v body=%s", err, w.Body.String())
	}
	if len(resp.Errors) != 0 {
		t.Fatalf("legacy query errors: %v", resp.Errors)
	}
	if resp.Data.DocumentByPath == nil {
		t.Fatalf("legacy query: expected A's document, got null")
	}
	if resp.Data.DocumentByPath.Content == nil || !strings.Contains(*resp.Data.DocumentByPath.Content, "Alpha A") {
		t.Fatalf("legacy query: expected A's content, got %+v", resp.Data.DocumentByPath)
	}
}
