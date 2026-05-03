package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// TestOperatorCancel_SetsBeadExtra: POST /api/beads/<id>/cancel writes
// extra.cancel-requested:true on the bead store. ADR-022 §Cancel SLA.
func TestOperatorCancel_SetsBeadExtra(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("POST", "/api/beads/bx-001/cancel", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	b, err := store.Get("bx-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := b.Extra[bead.ExtraCancelRequested]; got != true {
		t.Fatalf("expected Extra[%q]=true, got %v", bead.ExtraCancelRequested, got)
	}
}

// TestOperatorCancel_Idempotent: a second cancel on a bead whose worker has
// already honored the first is a silent no-op (status 200, no error). The
// honored marker remains set.
func TestOperatorCancel_Idempotent(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)
	store := bead.NewStore(filepath.Join(dir, ".ddx"))

	// First cancel.
	req := httptest.NewRequest("POST", "/api/beads/bx-001/cancel", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first cancel: expected 200, got %d", w.Code)
	}

	// Simulate the worker honoring it.
	if err := store.MarkCancelHonored("bx-001"); err != nil {
		t.Fatalf("MarkCancelHonored: %v", err)
	}

	// Second cancel should be a silent success — no new cancel-requested
	// marker, honored stays set.
	req2 := httptest.NewRequest("POST", "/api/beads/bx-001/cancel", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second cancel: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	b, err := store.Get("bx-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := b.Extra[bead.ExtraCancelHonored]; got != true {
		t.Fatalf("expected cancel-honored=true after worker ack, got %v", got)
	}
}

// TestOperatorCancel_RequiresTrusted: cancel requires a trusted (loopback)
// connection, matching every other write endpoint.
func TestOperatorCancel_RequiresTrusted(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest("POST", "/api/beads/bx-001/cancel", nil)
	req.RemoteAddr = "203.0.113.1:9999"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 from non-loopback, got %d: %s", w.Code, w.Body.String())
	}
}
