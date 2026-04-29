package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"gopkg.in/yaml.v3"
)

func TestListMCPServers_Empty(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest(http.MethodGet, "/api/mcp-servers", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestListMCPServers_WithData(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	libDir := filepath.Join(dir, ".ddx", "plugins", "ddx")
	mcpDir := filepath.Join(libDir, "mcp-servers")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	regYAML := `version: "1.0.0"
servers:
  - name: github
    category: development
    description: "GitHub integration"
  - name: postgres
    category: database
    description: "PostgreSQL integration"
`
	if err := os.WriteFile(filepath.Join(mcpDir, "registry.yml"), []byte(regYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mcp-servers", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []mcpServerEntry
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(result))
	}
	if result[0].Name != "github" {
		t.Errorf("expected github, got %q", result[0].Name)
	}
	if result[0].Category != "development" {
		t.Errorf("expected development, got %q", result[0].Category)
	}
}

func TestListMCPServers_RequiresTrust(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest(http.MethodGet, "/api/mcp-servers", nil)
	req.RemoteAddr = "203.0.113.1:12345" // non-loopback
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 from non-trusted origin, got %d", w.Code)
	}
}

func TestListPlugins_Empty(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	t.Setenv("HOME", t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestListPlugins_WithData(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	home := t.TempDir()
	t.Setenv("HOME", home)
	ddxDir := filepath.Join(home, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{
				Name:        "helix",
				Version:     "0.3.2",
				Type:        registry.PackageTypeWorkflow,
				Source:      "https://github.com/DocumentDrivenDX/helix",
				InstalledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddxDir, "installed.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []pluginInfo
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(result))
	}
	if result[0].Name != "helix" {
		t.Errorf("expected helix, got %q", result[0].Name)
	}
	if result[0].Type != "workflow" {
		t.Errorf("expected workflow, got %q", result[0].Type)
	}
}

func TestListPlugins_RequiresTrust(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	req.RemoteAddr = "203.0.113.1:12345" // non-loopback
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 from non-trusted origin, got %d", w.Code)
	}
}

func TestMCPListMCPServersToolCall(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	libDir := filepath.Join(dir, ".ddx", "plugins", "ddx")
	mcpDir := filepath.Join(libDir, "mcp-servers")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	regYAML := `version: "1.0.0"
servers:
  - name: github
    category: development
    description: "GitHub integration"
`
	if err := os.WriteFile(filepath.Join(mcpDir, "registry.yml"), []byte(regYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_list_mcp_servers","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "github") {
		t.Errorf("expected github in MCP response, got: %s", text)
	}
}

func TestMCPListPluginsToolCall(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	home := t.TempDir()
	t.Setenv("HOME", home)
	ddxDir := filepath.Join(home, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{
				Name:        "ddx",
				Version:     "0.4.7",
				Type:        registry.PackageTypePlugin,
				Source:      "https://github.com/DocumentDrivenDX/ddx",
				InstalledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddxDir, "installed.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_list_plugins","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result := resp.Result.(map[string]any)
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "ddx") {
		t.Errorf("expected ddx in MCP response, got: %s", text)
	}
}
