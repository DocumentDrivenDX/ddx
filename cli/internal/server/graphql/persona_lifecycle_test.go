package graphql_test

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

// TestIntegration_PersonaLifecycle covers AC #3 and AC #4:
//   - personaCreate/personaUpdate/personaDelete persist to `.ddx/personas`.
//   - Mutations gated on source: library personas are rejected with a typed error.
//   - A CLI-initiated create is then visible to the GraphQL `personas` query,
//     proving that UI and CLI share the same storage.
func TestIntegration_PersonaLifecycle(t *testing.T) {
	workDir, store := setupIntegrationDir(t)
	_ = store

	// Seed a library persona under the test workDir. The default library
	// path resolution points at `.ddx/plugins/ddx/personas`, but to keep
	// the test hermetic we override DDX_LIBRARY_BASE_PATH.
	libraryRoot := filepath.Join(workDir, "library")
	libPersonasDir := filepath.Join(libraryRoot, "personas")
	if err := os.MkdirAll(libPersonasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	libBody := `---
name: architect
roles: [architect]
description: Library architect
tags: []
---

# Library Architect
`
	if err := os.WriteFile(filepath.Join(libPersonasDir, "architect.md"), []byte(libBody), 0o644); err != nil {
		t.Fatal(err)
	}

	// Rewrite the ddx config to point at libraryRoot.
	cfg := `version: "2.0"
library:
  path: ` + libraryRoot + `
bead:
  id_prefix: "it"
`
	if err := os.WriteFile(filepath.Join(workDir, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DDX_LIBRARY_BASE_PATH", libraryRoot)

	state := newTestStateProvider(workDir, store)
	h := newGQLHandler(state, workDir, nil)
	projectID := state.projects[0].ID

	// AC#3: create a new project persona.
	newBody := `---\nname: our-reviewer\nroles: [code-reviewer]\ndescription: Our reviewer\ntags: []\n---\n\n# Our Reviewer\n`
	// JSON-escape the body.
	createQuery := `mutation($name: String!, $body: String!, $pid: String!) {
		personaCreate(name: $name, body: $body, projectId: $pid) { id name source }
	}`
	resp := gqlMutation(t, h, createQuery, map[string]any{
		"name": "our-reviewer",
		"body": newBody,
		"pid":  projectID,
	})
	if resp["data"] == nil {
		t.Fatalf("no data: %s", resp["errors"])
	}
	assertFileExists(t, filepath.Join(workDir, ".ddx", "personas", "our-reviewer.md"))

	// AC#3: update the project persona.
	updateQuery := `mutation($name: String!, $body: String!, $pid: String!) {
		personaUpdate(name: $name, body: $body, projectId: $pid) { id name source }
	}`
	updatedBody := `---\nname: our-reviewer\nroles: [code-reviewer]\ndescription: Our updated reviewer\ntags: []\n---\n\n# Our Reviewer (updated)\n`
	_ = gqlMutation(t, h, updateQuery, map[string]any{
		"name": "our-reviewer",
		"body": updatedBody,
		"pid":  projectID,
	})

	// AC#3: library persona cannot be updated.
	resp = gqlMutation(t, h, updateQuery, map[string]any{
		"name": "architect",
		"body": updatedBody,
		"pid":  projectID,
	})
	if !bytes.Contains([]byte(resp["errors"]), []byte("library")) {
		t.Fatalf("expected library error, got: %s", resp["errors"])
	}

	// AC#3: library persona cannot be deleted.
	deleteQuery := `mutation($name: String!, $pid: String!) {
		personaDelete(name: $name, projectId: $pid) { ok name }
	}`
	resp = gqlMutation(t, h, deleteQuery, map[string]any{
		"name": "architect",
		"pid":  projectID,
	})
	if !bytes.Contains([]byte(resp["errors"]), []byte("library")) {
		t.Fatalf("expected library error, got: %s", resp["errors"])
	}

	// AC#4: CLI parity — create a persona using the writer directly, then
	// verify GraphQL personas query sees it. We use os.WriteFile as the
	// proxy for the CLI since the CLI uses the same filesystem path.
	cliBody := "---\nname: cli-made\nroles: [implementer]\ndescription: Made via CLI\ntags: []\n---\n\n# CLI\n"
	if err := os.WriteFile(filepath.Join(workDir, ".ddx", "personas", "cli-made.md"), []byte(cliBody), 0o644); err != nil {
		t.Fatal(err)
	}

	listQuery := `query($pid: String) { personas(projectId: $pid) { name source } }`
	resp = gqlMutation(t, h, listQuery, map[string]any{"pid": projectID})
	if !bytes.Contains([]byte(resp["data"]), []byte("cli-made")) {
		t.Fatalf("expected cli-made in personas list, got: %s", resp["data"])
	}
	if !bytes.Contains([]byte(resp["data"]), []byte("our-reviewer")) {
		t.Fatalf("expected our-reviewer in personas list, got: %s", resp["data"])
	}
	// AC#2: architect persona visible with library source.
	if !bytes.Contains([]byte(resp["data"]), []byte(`"name":"architect","source":"library"`)) {
		t.Fatalf("expected architect with library source, got: %s", resp["data"])
	}

	// AC#3: project delete succeeds.
	resp = gqlMutation(t, h, deleteQuery, map[string]any{
		"name": "our-reviewer",
		"pid":  projectID,
	})
	if resp["errors"] != nil {
		t.Fatalf("unexpected errors: %s", resp["errors"])
	}
	if _, err := os.Stat(filepath.Join(workDir, ".ddx", "personas", "our-reviewer.md")); !os.IsNotExist(err) {
		t.Fatalf("expected our-reviewer.md removed; got err=%v", err)
	}

	// AC#3: fork library into project.
	forkQuery := `mutation($ln: String!, $new: String, $pid: String!) {
		personaFork(libraryName: $ln, newName: $new, projectId: $pid) { name source }
	}`
	newName := "architect-local"
	resp = gqlMutation(t, h, forkQuery, map[string]any{
		"ln":  "architect",
		"new": newName,
		"pid": projectID,
	})
	if resp["errors"] != nil {
		t.Fatalf("fork error: %s", resp["errors"])
	}
	assertFileExists(t, filepath.Join(workDir, ".ddx", "personas", "architect-local.md"))
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

// gqlMutation sends a GraphQL request with variables and returns the raw
// parsed body, letting the caller inspect either data or errors.
func gqlMutation(t *testing.T, h http.Handler, query string, vars map[string]any) map[string]json.RawMessage {
	t.Helper()
	// The body fields in this test are raw markdown; escape newlines by
	// JSON-marshaling.
	for k, v := range vars {
		if s, ok := v.(string); ok {
			vars[k] = strings.ReplaceAll(s, "\\n", "\n")
		}
	}
	payload := map[string]any{
		"query":     query,
		"variables": vars,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp
}
