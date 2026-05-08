package server

// TC-GQL-MUT-006..007: GraphQL Mutation resolver for documentWrite.
//
// Integration tests exercising documentWrite through the real GraphQL handler
// backed by a live library path on disk.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TC-GQL-MUT-006: documentWrite creates or updates a document in the library.
func TestGraphQLDocumentWrite(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-docs-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	mutation := `mutation {
		documentWrite(path: "prompts/new-doc.md", content: "# New document\n\nCreated via GraphQL.") {
			id
			path
		}
	}`

	resp := gqlMutation(t, srv, mutation)

	var data struct {
		DocumentWrite struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"documentWrite"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse data: %v", err)
	}

	if data.DocumentWrite.Path == "" {
		t.Error("expected non-empty path in response")
	}

	// Verify the file was actually written to the library path.
	libPath := filepath.Join(workDir, ".ddx", "plugins", "ddx")
	written := filepath.Join(libPath, "prompts", "new-doc.md")
	content, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("expected file to exist at %s: %v", written, err)
	}
	if string(content) != "# New document\n\nCreated via GraphQL." {
		t.Errorf("unexpected file content: %q", string(content))
	}
}

// TC-GQL-MUT-007: documentWrite rejects path traversal attempts.
func TestGraphQLDocumentWritePathTraversal(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-docs-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	mutation := `mutation {
		documentWrite(path: "../../etc/passwd", content: "pwned") {
			id
			path
		}
	}`

	rawBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, hasErrors := resp["errors"]; !hasErrors {
		t.Error("expected GraphQL errors for path traversal, got none")
	}
}
