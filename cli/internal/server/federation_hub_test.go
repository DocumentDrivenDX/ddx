package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// federationDoRequest is a small helper that runs a request against an
// httptest.NewServer wrapping s.Handler() so we exercise the real net/http
// stack (RemoteAddr is populated by the test server, matching production
// trust-gating behavior). Pass remote="loopback" for trusted, "external"
// for an explicitly non-loopback request.
func federationDoRequest(t *testing.T, s *Server, method, path string, body any, remote string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	switch remote {
	case "external":
		req.RemoteAddr = "203.0.113.1:54321"
	default:
		req.RemoteAddr = "127.0.0.1:54321"
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Result()
}

func newHubServer(t *testing.T, allowPlainHTTP bool) *Server {
	t.Helper()
	dir := setupTestDir(t)
	s := New(":0", dir)
	s.EnableHubMode(allowPlainHTTP)
	// Pin hub version for handshake tests.
	s.hub.hubDDxVersion = "0.10.5"
	s.hub.hubSchemaVer = "1"
	return s
}

func decodeBody(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// AC: Routes mounted only when --hub-mode.
func TestFederationRoutesNotMountedWithoutHubMode(t *testing.T) {
	dir := setupTestDir(t)
	s := New(":0", dir)
	// Hub mode NOT enabled — federation routes must not be on the mux at all.
	for _, p := range s.routePatterns {
		if strings.Contains(p, "/api/federation/") {
			t.Fatalf("federation route registered without EnableHubMode: %s", p)
		}
	}
	// And a register request must not produce a federation register response
	// (no node_id-bearing JSON body); the request falls through to the SPA
	// fallback or a non-2xx error, but never the federation handler.
	resp := federationDoRequest(t, s, "POST", "/api/federation/register",
		map[string]string{"node_id": "n1"}, "loopback")
	var maybe federationRegisterResponse
	_ = json.NewDecoder(resp.Body).Decode(&maybe)
	if maybe.NodeID == "n1" {
		t.Fatalf("federation register handler ran with hub-mode off")
	}
}

// AC: routes mounted when hub-mode enabled.
func TestFederationRoutesMountedInHubMode(t *testing.T) {
	s := newHubServer(t, false)
	want := []string{
		"POST /api/federation/register",
		"POST /api/federation/heartbeat",
		"GET /api/federation/spokes",
		"DELETE /api/federation/spokes/{id}",
	}
	have := map[string]bool{}
	for _, p := range s.routePatterns {
		have[p] = true
	}
	for _, p := range want {
		if !have[p] {
			t.Errorf("expected route %q not mounted", p)
		}
	}
}

func goodRegisterPayload(node string) map[string]any {
	return map[string]any{
		"node_id":              node,
		"identity_fingerprint": "fp-" + node,
		"name":                 "spoke-" + node,
		"url":                  "https://" + node + ":7743",
		"ddx_version":          "0.10.5",
		"schema_version":       "1",
		"capabilities":         []string{"beads", "runs"},
	}
}

// AC: Re-register same node_id replaces by id. URL change updates registry.
func TestFederationRegisterIdempotentByNodeID(t *testing.T) {
	s := newHubServer(t, false)

	resp := federationDoRequest(t, s, "POST", "/api/federation/register", goodRegisterPayload("n1"), "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first register: %d", resp.StatusCode)
	}

	// Re-register: same node_id, same identity, NEW url.
	body := goodRegisterPayload("n1")
	body["url"] = "https://updated:7743"
	resp = federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-register: %d", resp.StatusCode)
	}

	snap := s.HubFederationRegistrySnapshot()
	if got := len(snap.Spokes); got != 1 {
		t.Fatalf("want 1 spoke after re-register, got %d", got)
	}
	if snap.Spokes[0].URL != "https://updated:7743" {
		t.Errorf("URL change did not update registry: got %q", snap.Spokes[0].URL)
	}
}

// AC: Duplicate node_id from different identity returns 409 + log.
func TestFederationRegisterDuplicateNodeIDDifferentIdentity(t *testing.T) {
	s := newHubServer(t, false)

	resp := federationDoRequest(t, s, "POST", "/api/federation/register", goodRegisterPayload("n1"), "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first register: %d", resp.StatusCode)
	}

	// Same node_id, DIFFERENT identity_fingerprint.
	body := goodRegisterPayload("n1")
	body["identity_fingerprint"] = "fp-other-machine"
	resp = federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 on identity conflict, got %d", resp.StatusCode)
	}

	conflicts := s.HubFederationConflicts()
	if len(conflicts) == 0 {
		t.Errorf("expected a conflict log entry; got none")
	}
}

// AC: Non-loopback non-ts-net peers refused unless --federation-allow-plain-http.
func TestFederationStrictDefaultRefusesPlainHTTP(t *testing.T) {
	s := newHubServer(t, false)
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", goodRegisterPayload("n1"), "external")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 from non-trusted peer with strict default, got %d", resp.StatusCode)
	}
}

// AC: Plain-HTTP path emits WARN per accepted registration.
func TestFederationPlainHTTPOptOutEmitsWarn(t *testing.T) {
	s := newHubServer(t, true)
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", goodRegisterPayload("n1"), "external")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 with allow-plain-http, got %d", resp.StatusCode)
	}
	warnings := s.HubFederationWarnings()
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 WARN per accepted registration, got %d (%v)", len(warnings), warnings)
	}

	// Second registration of the same spoke should also log WARN.
	body := goodRegisterPayload("n1")
	body["url"] = "https://other:7743"
	resp = federationDoRequest(t, s, "POST", "/api/federation/register", body, "external")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 on re-register, got %d", resp.StatusCode)
	}
	if got := len(s.HubFederationWarnings()); got != 2 {
		t.Errorf("want 2 WARN messages after second plain-HTTP registration, got %d", got)
	}
}

// Loopback peers should NOT trigger plain-HTTP WARNs even when the opt-out is
// enabled — the opt-out concerns only non-trusted peers.
func TestFederationLoopbackDoesNotEmitPlainHTTPWarn(t *testing.T) {
	s := newHubServer(t, true)
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", goodRegisterPayload("n1"), "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("loopback register: %d", resp.StatusCode)
	}
	if got := len(s.HubFederationWarnings()); got != 0 {
		t.Errorf("loopback registration should not emit plain-HTTP WARN; got %d", got)
	}
}

// AC: Version mismatch — major reject with 4xx + reason.
func TestFederationRegisterVersionMajorMismatchRejected(t *testing.T) {
	s := newHubServer(t, false)
	body := goodRegisterPayload("n1")
	body["ddx_version"] = "1.0.0"
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Fatalf("want 4xx for major mismatch, got %d", resp.StatusCode)
	}
	var out map[string]string
	decodeBody(t, resp, &out)
	if out["reason"] != "ddx_major_mismatch" {
		t.Errorf("expected reason=ddx_major_mismatch, got %q", out["reason"])
	}
}

// AC: Schema mismatch reject with 4xx + reason.
func TestFederationRegisterSchemaMismatchRejected(t *testing.T) {
	s := newHubServer(t, false)
	body := goodRegisterPayload("n1")
	body["schema_version"] = "99"
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Fatalf("want 4xx for schema mismatch, got %d", resp.StatusCode)
	}
	var out map[string]string
	decodeBody(t, resp, &out)
	if out["reason"] != "schema_mismatch" {
		t.Errorf("expected reason=schema_mismatch, got %q", out["reason"])
	}
}

// AC: newer minor accepted with degraded status.
func TestFederationRegisterNewerMinorAcceptedDegraded(t *testing.T) {
	s := newHubServer(t, false)
	body := goodRegisterPayload("n1")
	body["ddx_version"] = "0.20.0"
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for newer minor, got %d", resp.StatusCode)
	}
	var out federationRegisterResponse
	decodeBody(t, resp, &out)
	if out.Status != federation.StatusDegraded {
		t.Errorf("want status=degraded for newer minor, got %q", out.Status)
	}
	if out.Reason != "degraded_newer_minor" {
		t.Errorf("want reason=degraded_newer_minor, got %q", out.Reason)
	}
}

// Heartbeat happy path: registers, heartbeats, status flips to active.
func TestFederationHeartbeatActivatesSpoke(t *testing.T) {
	s := newHubServer(t, false)
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", goodRegisterPayload("n1"), "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	resp = federationDoRequest(t, s, "POST", "/api/federation/heartbeat",
		map[string]string{"node_id": "n1"}, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat: %d", resp.StatusCode)
	}
	snap := s.HubFederationRegistrySnapshot()
	if snap.Spokes[0].Status != federation.StatusActive {
		t.Errorf("want active after heartbeat, got %q", snap.Spokes[0].Status)
	}
	if snap.Spokes[0].LastHeartbeat.IsZero() {
		t.Errorf("LastHeartbeat not set")
	}
}

// Heartbeat for unknown node returns 404.
func TestFederationHeartbeatUnknownReturns404(t *testing.T) {
	s := newHubServer(t, false)
	resp := federationDoRequest(t, s, "POST", "/api/federation/heartbeat",
		map[string]string{"node_id": "ghost"}, "loopback")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

// AC: GET /spokes lists registered spokes; DELETE deregisters.
func TestFederationListAndDeregister(t *testing.T) {
	s := newHubServer(t, false)
	for _, n := range []string{"n1", "n2", "n3"} {
		resp := federationDoRequest(t, s, "POST", "/api/federation/register", goodRegisterPayload(n), "loopback")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("register %s: %d", n, resp.StatusCode)
		}
	}

	resp := federationDoRequest(t, s, "GET", "/api/federation/spokes", nil, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d", resp.StatusCode)
	}
	var listed struct {
		SchemaVersion string                   `json:"schema_version"`
		Spokes        []federation.SpokeRecord `json:"spokes"`
	}
	decodeBody(t, resp, &listed)
	if len(listed.Spokes) != 3 {
		t.Fatalf("want 3 spokes, got %d", len(listed.Spokes))
	}

	resp = federationDoRequest(t, s, "DELETE", "/api/federation/spokes/n2", nil, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d", resp.StatusCode)
	}

	snap := s.HubFederationRegistrySnapshot()
	if len(snap.Spokes) != 2 {
		t.Fatalf("want 2 spokes after delete, got %d", len(snap.Spokes))
	}
	for _, sp := range snap.Spokes {
		if sp.NodeID == "n2" {
			t.Errorf("n2 still present after delete")
		}
	}
}

// DELETE on missing spoke returns 404.
func TestFederationDeregisterMissingReturns404(t *testing.T) {
	s := newHubServer(t, false)
	resp := federationDoRequest(t, s, "DELETE", "/api/federation/spokes/ghost", nil, "loopback")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

// Bad JSON on register returns 400.
func TestFederationRegisterRejectsBadJSON(t *testing.T) {
	s := newHubServer(t, false)
	req := httptest.NewRequest("POST", "/api/federation/register",
		strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad JSON, got %d", rec.Code)
	}
}

// Missing node_id returns 400.
func TestFederationRegisterRejectsMissingNodeID(t *testing.T) {
	s := newHubServer(t, false)
	body := goodRegisterPayload("n1")
	delete(body, "node_id")
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}
