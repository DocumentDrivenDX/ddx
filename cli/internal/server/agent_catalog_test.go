package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAgentCatalog(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/catalog", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AgentCatalogResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Source != "file" && resp.Source != "built-in" {
		t.Errorf("unexpected source %q", resp.Source)
	}
	if resp.Tiers == nil {
		t.Error("expected non-nil tiers map")
	}
	if resp.Models == nil {
		t.Error("expected non-nil models slice")
	}
}

func TestHandleAgentModelsNoProviders(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/models?all=true", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var providers []AgentModelsProvider
	if err := json.Unmarshal(w.Body.Bytes(), &providers); err != nil {
		t.Fatalf("invalid JSON (got %s): %v", w.Body.String(), err)
	}
	// With minimal config and no provider keys, the array may be empty.
	// The contract is just that the route returns 200 + a JSON array.
}

func TestHandleAgentCapabilitiesNoHarness(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("GET", "/api/agent/capabilities", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Without configured providers we expect a non-200 with a JSON error body.
	if w.Code == http.StatusOK {
		// If a default harness is somehow available, that's also fine — just
		// ensure the response is valid JSON.
		var anyResp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &anyResp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		return
	}
	var errResp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error body, got %s: %v", w.Body.String(), err)
	}
	if errResp["error"] == "" {
		t.Error("expected non-empty error field")
	}
}

func TestMCPAgentCatalog(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_agent_catalog","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("expected content array with entries")
	}
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)
	// The catalog payload always has a "tiers" key (possibly empty object).
	if !strings.Contains(text, `"tiers"`) {
		t.Errorf("expected tiers field in catalog response, got: %s", text)
	}
	if !strings.Contains(text, `"source"`) {
		t.Errorf("expected source field in catalog response, got: %s", text)
	}
}

func TestMCPAgentModels(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_agent_models","arguments":{"all":true}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("expected content array with entries")
	}
	// Tool result must be valid JSON (an array — possibly empty).
	textMap := content[0].(map[string]any)
	text := textMap["text"].(string)
	var providers []AgentModelsProvider
	if err := json.Unmarshal([]byte(text), &providers); err != nil {
		t.Fatalf("expected JSON array body, got %q: %v", text, err)
	}
}

func TestMCPAgentCapabilitiesNoHarness(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	w := mcpRequest(t, srv, "tools/call", `{"name":"ddx_agent_capabilities","arguments":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("expected content array with entries")
	}
	textMap := content[0].(map[string]any)
	if _, ok := textMap["text"].(string); !ok {
		t.Error("expected text field in content entry")
	}
	// Either it returned an isError (no harness) or a JSON capabilities body.
	// Both are acceptable for this minimal-config test.
}
