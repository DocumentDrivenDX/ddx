package server

// TC-GQL-MUT-006..007: GraphQL Mutation resolver for documentWrite.
//
// Integration tests exercising documentWrite through the real GraphQL handler
// backed by a live library path on disk.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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
	libPath := filepath.Join(workDir, ddxroot.DirName, "plugins", "ddx")
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

// TC-GQL-MUT-008: documentWrite rejects stale expectedHash values with a conflict error.
func TestGraphQLDocumentWriteExpectedHashConflict(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-mut-docs-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	libPath := filepath.Join(workDir, ddxroot.DirName, "plugins", "ddx")
	docPath := filepath.Join(libPath, "prompts", "conflict.md")
	original := "# Conflict\n\nOriginal content.\n"
	if err := os.WriteFile(docPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(original))
	expectedHash := hex.EncodeToString(sum[:])

	updated := "# Conflict\n\nFresh content.\n"
	if err := os.WriteFile(docPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	mutation := fmt.Sprintf(`mutation {
		documentWrite(path: "prompts/conflict.md", content: "# Conflict\n\nStale content.", expectedHash: "%s") {
			id
			path
		}
	}`, expectedHash)

	rawBody, _ := json.Marshal(map[string]string{"query": mutation})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Errors []struct {
			Message    string         `json:"message"`
			Extensions map[string]any `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected one GraphQL error, got %+v", resp.Errors)
	}
	if got := resp.Errors[0].Extensions["code"]; got != "DOCUMENT_WRITE_CONFLICT" {
		t.Fatalf("expected conflict error code, got %+v", resp.Errors[0].Extensions)
	}
	if !strings.Contains(strings.ToLower(resp.Errors[0].Message), "conflict") {
		t.Fatalf("expected conflict message, got %q", resp.Errors[0].Message)
	}

	body, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != updated {
		t.Fatalf("conflict write overwrote current content:\nwant %q\n got %q", updated, string(body))
	}
}
