package server

// TC-011: Host+User State and Node Identity
//
// Verifies that ddx-server runs as a per-user host daemon with state at
// XDG_DATA_HOME/ddx/server-state.json and writes XDG_DATA_HOME/ddx/server.addr,
// per FEAT-020. Tests use XDG_DATA_HOME env-var overrides so they never touch
// the real user state file.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stateDir returns the XDG_DATA_HOME/ddx path used by a server started under
// the current XDG_DATA_HOME env var (which tests override via t.Setenv).
func stateDir(t *testing.T) string {
	t.Helper()
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		t.Fatal("XDG_DATA_HOME not set; call t.Setenv before using stateDir")
	}
	return filepath.Join(xdg, "ddx")
}

// TC-011.1 — Server writes server-state.json under XDG_DATA_HOME/ddx.
func TestStateFileLocation(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "test-node")

	// setupTestDir isolates XDG_DATA_HOME to its own temp dir.
	workDir := setupTestDir(t)
	xdgDir := os.Getenv("XDG_DATA_HOME")
	srv := New(":0", workDir)

	// save() is called internally by New() via RegisterProject; verify the file.
	stateFile := filepath.Join(xdgDir, "ddx", "server-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("server-state.json not written at %s: %v", stateFile, err)
	}

	var state struct {
		SchemaVersion string `json:"schema_version"`
		Node          struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"node"`
		Projects []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("server-state.json not valid JSON: %v", err)
	}
	if state.Node.Name != "test-node" {
		t.Errorf("expected node name=test-node, got %q", state.Node.Name)
	}
	if !strings.HasPrefix(state.Node.ID, "node-") {
		t.Errorf("expected node ID to start with 'node-', got %q", state.Node.ID)
	}
	if len(state.Projects) == 0 {
		t.Error("expected at least one project in state file")
	}
	_ = srv
}

// TC-011.2 — Server writes server.addr under XDG_DATA_HOME/ddx with URL,
// node name, and node ID fields.
func TestAddrFileLocation(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "addr-test-node")

	workDir := setupTestDir(t)
	xdgDir := os.Getenv("XDG_DATA_HOME")
	srv := New(":0", workDir)

	// writeAddrFile is normally called from Serve/ListenAndServe, but we can
	// exercise it directly since we need to verify the file format.
	srv.writeAddrFile("http")

	addrFile := filepath.Join(xdgDir, "ddx", "server.addr")
	data, err := os.ReadFile(addrFile)
	if err != nil {
		t.Fatalf("server.addr not written at %s: %v", addrFile, err)
	}

	var af struct {
		Node   string `json:"node"`
		NodeID string `json:"node_id"`
		URL    string `json:"url"`
		PID    int    `json:"pid"`
	}
	if err := json.Unmarshal(data, &af); err != nil {
		t.Fatalf("server.addr not valid JSON: %v", err)
	}
	if af.Node != "addr-test-node" {
		t.Errorf("expected node=addr-test-node, got %q", af.Node)
	}
	if !strings.HasPrefix(af.NodeID, "node-") {
		t.Errorf("expected node_id to start with 'node-', got %q", af.NodeID)
	}
	if af.URL == "" {
		t.Error("expected non-empty url in server.addr")
	}
	if af.PID == 0 {
		t.Error("expected non-zero pid in server.addr")
	}
}

// TC-011.3 — GET /api/node returns a stable node-<hash> ID derived from
// DDX_NODE_NAME (or hostname). Covered by TestGetNode in server_test.go;
// this test adds the stability assertion: two requests must return the same ID.
func TestNodeIdentityStable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "stable-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	getNode := func() string {
		req := httptest.NewRequest("GET", "/api/node", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var node struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		return node.ID
	}

	id1 := getNode()
	id2 := getNode()

	if !strings.HasPrefix(id1, "node-") {
		t.Errorf("expected node ID prefix 'node-', got %q", id1)
	}
	if id1 != id2 {
		t.Errorf("node ID changed between requests: %q vs %q", id1, id2)
	}
}

// TC-011.4 — Projects registered before a simulated restart are returned by
// GET /api/projects after a fresh server is created against the same state dir.
func TestStateFileProjectsPersistAcrossRestart(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "persist-node")

	workDir := setupTestDir(t)

	// Start first server; it registers workDir automatically.
	srv1 := New(":0", workDir)

	// Register an additional project path.
	extraPath := t.TempDir()
	srv1.state.RegisterProject(extraPath)
	if err := srv1.state.save(); err != nil {
		t.Fatalf("save() failed: %v", err)
	}

	// Verify first server has both projects.
	before := srv1.state.GetProjects()
	if len(before) != 2 {
		t.Fatalf("expected 2 projects before restart, got %d", len(before))
	}

	// Simulate a restart: create a new Server backed by the same XDG state dir.
	srv2 := New(":0", workDir)

	// GET /api/projects on the new server must return the same projects.
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv2.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var projects []struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects after restart, got %d", len(projects))
	}

	paths := map[string]bool{}
	for _, p := range projects {
		paths[p.Path] = true
	}
	if !paths[workDir] {
		t.Errorf("workDir %s missing from project list after restart", workDir)
	}
	if !paths[extraPath] {
		t.Errorf("extraPath %s missing from project list after restart", extraPath)
	}
}

// TC-011.6 — A second server start overwrites server.addr (last-writer wins).
// The first instance's addr is no longer valid after the second instance writes.
func TestAddrFileOverwrittenBySecondInstance(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "overwrite-test-node")

	workDir := setupTestDir(t)
	xdgDir := os.Getenv("XDG_DATA_HOME")
	addrFile := filepath.Join(xdgDir, "ddx", "server.addr")

	// First server writes addr with its Addr field.
	srv1 := New(":127.0.0.1:18081", workDir)
	srv1.writeAddrFile("http")

	data1, err := os.ReadFile(addrFile)
	if err != nil {
		t.Fatalf("server.addr not written after first instance: %v", err)
	}
	var af1 struct {
		URL string `json:"url"`
		PID int    `json:"pid"`
	}
	if err := json.Unmarshal(data1, &af1); err != nil {
		t.Fatalf("first addr JSON invalid: %v", err)
	}
	if !strings.Contains(af1.URL, "18081") {
		t.Errorf("expected first addr to contain port 18081, got %q", af1.URL)
	}

	// Second server writes addr with a different port, overwriting the file.
	srv2 := New(":127.0.0.1:18082", workDir)
	srv2.writeAddrFile("http")

	data2, err := os.ReadFile(addrFile)
	if err != nil {
		t.Fatalf("server.addr missing after second instance: %v", err)
	}
	var af2 struct {
		URL string `json:"url"`
		PID int    `json:"pid"`
	}
	if err := json.Unmarshal(data2, &af2); err != nil {
		t.Fatalf("second addr JSON invalid: %v", err)
	}
	if !strings.Contains(af2.URL, "18082") {
		t.Errorf("expected second addr to contain port 18082, got %q", af2.URL)
	}
	// The file content must differ from the first write.
	if string(data1) == string(data2) {
		t.Error("server.addr was not overwritten by second instance")
	}
}
