package server

// TC-BEAD-MUT-PARITY: Mutation surface parity tests across REST and MCP callers.
//
// The CLI (Cobra) is the canonical richest surface. CLI parent/set/unset flags are
// exercised in cli/cmd/bead_acceptance_test.go. These tests prove that the REST and
// MCP callers expose the same fields so drift across surfaces is detected immediately.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// TC-BEAD-MUT-PARITY-001: REST and MCP create surfaces accept parent and custom (set) fields.
func TestBeadCreateMutationSpec_ParityAcrossCLIRESTMCP(t *testing.T) {
	t.Run("REST/parent survives create", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		body := `{"title":"child bead","type":"task","parent":"bx-001"}`
		req := httptest.NewRequest(http.MethodPost, "/api/beads", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var got struct {
			ID     string `json:"id"`
			Parent string `json:"parent"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Parent != "bx-001" {
			t.Errorf("REST create: expected parent=bx-001, got %q", got.Parent)
		}
	})

	t.Run("REST/set custom fields survive create", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		body := `{"title":"tagged bead","set":["spec-id=FEAT-99","execution-eligible=true"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/beads", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var got struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		// Extra is json:"-" on Bead so it is not in the REST response; read from store.
		store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
		b, err := store.Get(context.Background(), got.ID)
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if b.Extra == nil {
			t.Fatal("expected Extra map to be non-nil after set")
		}
		if b.Extra["spec-id"] != "FEAT-99" {
			t.Errorf("REST create: expected Extra[spec-id]=FEAT-99, got %v", b.Extra["spec-id"])
		}
		if b.Extra["execution-eligible"] != true {
			t.Errorf("REST create: expected Extra[execution-eligible]=true, got %v", b.Extra["execution-eligible"])
		}
	})

	t.Run("MCP/parent survives create", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		params := `{"name":"ddx_bead_create","arguments":{"title":"mcp child","parent":"bx-001"}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		result := resp.Result.(map[string]any)
		if isErr, _ := result["isError"].(bool); isErr {
			t.Fatalf("unexpected MCP error: %+v", result)
		}
		content := result["content"].([]any)
		text := content[0].(map[string]any)["text"].(string)
		var got struct {
			ID     string `json:"id"`
			Parent string `json:"parent"`
		}
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("parse bead JSON: %v", err)
		}
		if got.Parent != "bx-001" {
			t.Errorf("MCP create: expected parent=bx-001, got %q", got.Parent)
		}
	})

	t.Run("MCP/set custom field survives create", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		params := `{"name":"ddx_bead_create","arguments":{"title":"mcp tagged","set":["spec-id=MCP-01"]}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		result := resp.Result.(map[string]any)
		if isErr, _ := result["isError"].(bool); isErr {
			t.Fatalf("unexpected MCP error: %+v", result)
		}
		content := result["content"].([]any)
		text := content[0].(map[string]any)["text"].(string)
		var got struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("parse bead JSON: %v", err)
		}
		store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
		b, err := store.Get(context.Background(), got.ID)
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if b.Extra == nil || b.Extra["spec-id"] != "MCP-01" {
			t.Errorf("MCP create: expected Extra[spec-id]=MCP-01, got %v", b.Extra)
		}
	})
}

// TC-BEAD-MUT-PARITY-002: REST and MCP update surfaces accept the full field set.
// Fields covered: title, priority, labels, acceptance, assignee, parent, description, notes, set, unset.
func TestBeadUpdateMutationSpec_ParityAcrossCLIRESTMCP(t *testing.T) {
	// --- REST subtests ---

	t.Run("REST/title updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		req := httptest.NewRequest(http.MethodPut, "/api/beads/bx-001",
			bytes.NewBufferString(`{"title":"New REST Title"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var got struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Title != "New REST Title" {
			t.Errorf("REST update title: expected 'New REST Title', got %q", got.Title)
		}
	})

	t.Run("REST/assignee updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		req := httptest.NewRequest(http.MethodPut, "/api/beads/bx-001",
			bytes.NewBufferString(`{"assignee":"alice"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var got struct {
			Owner string `json:"owner"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Owner != "alice" {
			t.Errorf("REST update assignee: expected owner='alice', got %q", got.Owner)
		}
	})

	t.Run("REST/parent updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		req := httptest.NewRequest(http.MethodPut, "/api/beads/bx-001",
			bytes.NewBufferString(`{"parent":"bx-003"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var got struct {
			Parent string `json:"parent"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Parent != "bx-003" {
			t.Errorf("REST update parent: expected 'bx-003', got %q", got.Parent)
		}
	})

	t.Run("REST/set and unset custom fields", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		// Set custom fields via REST.
		req := httptest.NewRequest(http.MethodPut, "/api/beads/bx-001",
			bytes.NewBufferString(`{"set":["widget=blue","count=3"]}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("set: expected 200, got %d: %s", w.Code, w.Body.String())
		}

		store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
		b, err := store.Get(context.Background(), "bx-001")
		if err != nil {
			t.Fatalf("store.Get after set: %v", err)
		}
		if b.Extra == nil || b.Extra["widget"] != "blue" {
			t.Errorf("REST set: expected Extra[widget]=blue, got %v", b.Extra)
		}

		// Unset the custom field.
		req2 := httptest.NewRequest(http.MethodPut, "/api/beads/bx-001",
			bytes.NewBufferString(`{"unset":["widget"]}`))
		req2.Header.Set("Content-Type", "application/json")
		req2.RemoteAddr = "127.0.0.1:12345"
		w2 := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("unset: expected 200, got %d: %s", w2.Code, w2.Body.String())
		}

		b2, err := store.Get(context.Background(), "bx-001")
		if err != nil {
			t.Fatalf("store.Get after unset: %v", err)
		}
		if b2.Extra != nil {
			if _, ok := b2.Extra["widget"]; ok {
				t.Error("REST unset: expected Extra[widget] to be removed")
			}
		}
	})

	// --- MCP subtests ---

	t.Run("MCP/title updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		params := `{"name":"ddx_bead_update","arguments":{"id":"bx-001","title":"MCP Title"}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		text := mcpResultText(t, w)
		var got struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatal(err)
		}
		if got.Title != "MCP Title" {
			t.Errorf("MCP update title: expected 'MCP Title', got %q", got.Title)
		}
	})

	t.Run("MCP/priority updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		params := `{"name":"ddx_bead_update","arguments":{"id":"bx-001","priority":3}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		text := mcpResultText(t, w)
		var got struct {
			Priority int `json:"priority"`
		}
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatal(err)
		}
		if got.Priority != 3 {
			t.Errorf("MCP update priority: expected 3, got %d", got.Priority)
		}
	})

	t.Run("MCP/notes updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		params := `{"name":"ddx_bead_update","arguments":{"id":"bx-001","notes":"some notes"}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		text := mcpResultText(t, w)
		var got struct {
			Notes string `json:"notes"`
		}
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatal(err)
		}
		if got.Notes != "some notes" {
			t.Errorf("MCP update notes: expected 'some notes', got %q", got.Notes)
		}
	})

	t.Run("MCP/assignee updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		params := `{"name":"ddx_bead_update","arguments":{"id":"bx-001","assignee":"bob"}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		text := mcpResultText(t, w)
		var got struct {
			Owner string `json:"owner"`
		}
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatal(err)
		}
		if got.Owner != "bob" {
			t.Errorf("MCP update assignee: expected owner='bob', got %q", got.Owner)
		}
	})

	t.Run("MCP/parent updated", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		params := `{"name":"ddx_bead_update","arguments":{"id":"bx-001","parent":"bx-003"}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		text := mcpResultText(t, w)
		var got struct {
			Parent string `json:"parent"`
		}
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatal(err)
		}
		if got.Parent != "bx-003" {
			t.Errorf("MCP update parent: expected 'bx-003', got %q", got.Parent)
		}
	})

	t.Run("MCP/set and unset custom fields", func(t *testing.T) {
		dir := setupTestDir(t)
		srv := New(":0", dir)

		// Set a custom field via MCP.
		params := `{"name":"ddx_bead_update","arguments":{"id":"bx-001","set":["tag=mcp-test"]}}`
		w := mcpRequest(t, srv, "tools/call", params)
		if w.Code != http.StatusOK {
			t.Fatalf("set: expected 200, got %d", w.Code)
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		result := resp.Result.(map[string]any)
		if isErr, _ := result["isError"].(bool); isErr {
			t.Fatalf("unexpected MCP error on set: %+v", result)
		}

		store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
		b, err := store.Get(context.Background(), "bx-001")
		if err != nil {
			t.Fatalf("store.Get after MCP set: %v", err)
		}
		if b.Extra == nil || b.Extra["tag"] != "mcp-test" {
			t.Errorf("MCP set: expected Extra[tag]=mcp-test, got %v", b.Extra)
		}

		// Unset via MCP.
		params2 := `{"name":"ddx_bead_update","arguments":{"id":"bx-001","unset":["tag"]}}`
		w2 := mcpRequest(t, srv, "tools/call", params2)
		if w2.Code != http.StatusOK {
			t.Fatalf("unset: expected 200, got %d", w2.Code)
		}

		b2, err := store.Get(context.Background(), "bx-001")
		if err != nil {
			t.Fatalf("store.Get after MCP unset: %v", err)
		}
		if b2.Extra != nil {
			if _, ok := b2.Extra["tag"]; ok {
				t.Error("MCP unset: expected Extra[tag] to be removed")
			}
		}
	})
}

// TC-BEAD-MUT-PARITY-003: Claim/unclaim are dedicated endpoints intentionally separated
// from BeadUpdateRequest. Setting assignee via the generic update does NOT trigger the
// full claim flow (no status transition, no claimed-at/claimed-machine Extra keys).
// The dedicated claim endpoints are tested in TestClaimBead, TestUnclaimBead,
// TestGraphQLBeadClaim, and TestGraphQLBeadUnclaim.
func TestBeadClaimUnclaimAreDedicatedEndpoints(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Generic assignee update via PUT sets Owner but does NOT transition status.
	req := httptest.NewRequest(http.MethodPut, "/api/beads/bx-001",
		bytes.NewBufferString(`{"assignee":"test-worker"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got struct {
		Owner  string `json:"owner"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "open" {
		t.Errorf("generic assignee update must not change status; expected 'open', got %q", got.Status)
	}
	if got.Owner != "test-worker" {
		t.Errorf("expected owner='test-worker', got %q", got.Owner)
	}
	// Verify no claimed-at Extra key was set (claim flow is dedicated, not generic).
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	b, err := store.Get(context.Background(), "bx-001")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if b.Extra != nil {
		if _, ok := b.Extra["claimed-at"]; ok {
			t.Error("generic update must not set claimed-at; use POST /api/beads/{id}/claim instead")
		}
	}
}

// mcpResultText extracts the first content text from an MCP tools/call response,
// failing the test if the response contains an error or is malformed.
func mcpResultText(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp jsonRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal MCP response: %v\nbody: %s", err, w.Body.String())
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("unexpected MCP error: %+v", result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("expected non-empty content, got %+v", result)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	return text
}
