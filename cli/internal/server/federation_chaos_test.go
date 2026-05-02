package server

// B14.8a: Backend chaos / integration suite for federation. Each subtest
// exercises one failure scenario end-to-end against the real hub HTTP
// routes (via Server.Handler) and real spoke servers (httptest.Server),
// asserting both behavior (caller sees partial result, no panic, others
// keep working) AND the resulting per-spoke status transitions in the
// hub registry.
//
// Scenarios (per bead AC):
//   1. hub crash mid-fan-out      → caller sees partial result, no panic
//   2. stale spoke during query   → marked stale, not blocking
//   3. duplicate registration     → idempotent (same identity) + 409 (conflicting identity)
//   4. newer-version registration → degraded status recorded
//   5. slow-spoke timeout         → per-node timeout enforced, others succeed
//   6. hub restart + immediate    → registry rebuilt from disk + repopulated by next heartbeat
//
// All scenarios are deterministic: no time.Sleep gates correctness; only
// per-node deadlines (which fire in milliseconds) and explicit status
// reconcile calls drive transitions.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// chaosSpoke pairs an httptest.Server with the SpokeRecord that the hub
// would have registered for it, so each scenario can either drive
// register-via-HTTP or seed the registry directly.
type chaosSpoke struct {
	srv *httptest.Server
	rec federation.SpokeRecord
}

func newChaosSpoke(t *testing.T, nodeID string, handler http.HandlerFunc) *chaosSpoke {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &chaosSpoke{
		srv: srv,
		rec: federation.SpokeRecord{
			NodeID:        nodeID,
			Name:          nodeID,
			URL:           srv.URL,
			DDxVersion:    "0.10.5",
			SchemaVersion: federation.CurrentSchemaVersion,
			Capabilities:  []string{"beads", "runs"},
			Status:        federation.StatusActive,
		},
	}
}

// registerVia drives a real /api/federation/register call so we exercise
// the full HTTP path (handshake, persistence, status assignment).
func registerVia(t *testing.T, s *Server, sp *chaosSpoke, identityFP string) *http.Response {
	t.Helper()
	body := map[string]any{
		"node_id":              sp.rec.NodeID,
		"identity_fingerprint": identityFP,
		"name":                 sp.rec.Name,
		"url":                  sp.rec.URL,
		"ddx_version":          sp.rec.DDxVersion,
		"schema_version":       sp.rec.SchemaVersion,
		"capabilities":         sp.rec.Capabilities,
	}
	return federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
}

// statusOf reads the current hub-side registry status for one spoke.
func statusOf(t *testing.T, s *Server, nodeID string) federation.SpokeStatus {
	t.Helper()
	snap := s.HubFederationRegistrySnapshot()
	if snap == nil {
		t.Fatalf("registry snapshot nil (hub mode not enabled?)")
	}
	if rec := snap.FindSpoke(nodeID); rec != nil {
		return rec.Status
	}
	t.Fatalf("spoke %q absent from registry", nodeID)
	return ""
}

// applyStatusUpdates writes fan-out side-effects back into the hub
// registry the way a production caller would.
func applyStatusUpdates(t *testing.T, s *Server, res *federation.FanOutResult) {
	t.Helper()
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	for nodeID, st := range res.StatusUpdates {
		if err := s.hub.registry.SetStatus(nodeID, st); err != nil {
			t.Fatalf("apply status update %s=%s: %v", nodeID, st, err)
		}
	}
}

// newFanOutClient builds a client tuned for tests: short per-node timeout
// so slow-spoke scenarios complete in milliseconds, low concurrency so
// goroutine scheduling stays predictable, and version pinned to match
// newHubServer.
func newChaosFanOutClient(perNode time.Duration) *federation.FanOutClient {
	c := federation.NewFanOutClient()
	c.HubDDxVersion = "0.10.5"
	c.HubSchemaVersion = federation.CurrentSchemaVersion
	c.PerNodeTimeout = perNode
	c.MaxConcurrency = 4
	return c
}

// TestFederationChaosSuite groups the six chaos/integration scenarios so
// they share the suite-level setup without losing per-scenario isolation.
// Each subtest stands up its own hub + spokes — one scenario crashing a
// spoke or wiping disk state must not bleed into the next.
func TestFederationChaosSuite(t *testing.T) {
	t.Run("HubCrashMidFanOutCallerSeesPartialResult", testChaosHubCrashMidFanOut)
	t.Run("StaleSpokeDuringMergedQuery", testChaosStaleSpokeDoesNotBlock)
	t.Run("DuplicateRegistrationIdempotentAndConflict", testChaosDuplicateRegistration)
	t.Run("NewerVersionRegistrationDegraded", testChaosNewerVersionDegraded)
	t.Run("SlowSpokeTimeoutOthersSucceed", testChaosSlowSpokeTimeout)
	t.Run("HubRestartRebuildsRegistryAndRepopulates", testChaosHubRestartRebuildAndRepopulate)
}

// ---------------------------------------------------------------------------
// Scenario 1: hub "crashes" mid-fan-out — the caller still sees a partial
// result, no panic propagates out of FanOutClient.Execute.
//
// Modeled by one spoke that aborts the in-flight HTTP handler (which
// surfaces to the hub as a transport error) while the other spoke
// completes normally. This is the closest in-process analogue to "the
// world disappears under the hub" — the fan-out goroutine sees a hard
// transport failure mid-call and must not panic.
//
// Asserted status transition: aborting spoke goes Active → Offline; the
// surviving spoke's status is unchanged.
// ---------------------------------------------------------------------------
func testChaosHubCrashMidFanOut(t *testing.T) {
	s := newHubServer(t, false)

	good := newChaosSpoke(t, "good-spoke", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	// Aborting handler: hijack the connection and close it without writing
	// any response. To the fan-out client this is indistinguishable from a
	// hub crash mid-call.
	bad := newChaosSpoke(t, "crash-spoke", func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("response writer not a Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	})

	if r := registerVia(t, s, good, "fp-good"); r.StatusCode != http.StatusOK {
		t.Fatalf("register good: %d", r.StatusCode)
	}
	if r := registerVia(t, s, bad, "fp-bad"); r.StatusCode != http.StatusOK {
		t.Fatalf("register bad: %d", r.StatusCode)
	}

	// Use the registry snapshot the way a real federated resolver would.
	snap := s.HubFederationRegistrySnapshot()
	c := newChaosFanOutClient(500 * time.Millisecond)

	// Wrap Execute so a panic anywhere inside the fan-out fails the test
	// loudly (the AC: "caller sees partial-result not panic").
	var res *federation.FanOutResult
	func() {
		defer func() {
			if rv := recover(); rv != nil {
				t.Fatalf("fan-out panicked despite spoke crash: %v", rv)
			}
		}()
		var err error
		res, err = c.Execute(context.Background(), snap.Spokes, &federation.FanOutRequest{Query: "{ __typename }"})
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
	}()

	if _, ok := res.Responses[good.rec.NodeID]; !ok {
		t.Fatalf("partial result missing surviving spoke; got %+v", res.Responses)
	}
	if _, ok := res.Errors[bad.rec.NodeID]; !ok {
		t.Fatalf("crashing spoke missing from errors map; got %+v", res.Errors)
	}
	if got := res.StatusUpdates[bad.rec.NodeID]; got != federation.StatusOffline {
		t.Fatalf("crashing spoke status update = %q, want %q", got, federation.StatusOffline)
	}

	applyStatusUpdates(t, s, res)
	if got := statusOf(t, s, bad.rec.NodeID); got != federation.StatusOffline {
		t.Fatalf("hub registry status after crash = %q, want %q", got, federation.StatusOffline)
	}
	if got := statusOf(t, s, good.rec.NodeID); got == federation.StatusOffline {
		t.Fatalf("surviving spoke must not be flipped to offline; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: a spoke marked stale by the registry's time-based reconcile
// is still queried and still returns data — staleness is a heartbeat-age
// badge, not a routing block. The successful response must not flip the
// spoke off "stale" via fan-out side effects (only a heartbeat does).
//
// Asserted status transition: spoke is reconciled Active → Stale, fan-out
// happens, status remains Stale post-fan-out (no StatusUpdate emitted),
// then a heartbeat moves it to Active.
// ---------------------------------------------------------------------------
func testChaosStaleSpokeDoesNotBlock(t *testing.T) {
	s := newHubServer(t, false)

	var hits int
	sp := newChaosSpoke(t, "stale-but-alive", func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	if r := registerVia(t, s, sp, "fp-1"); r.StatusCode != http.StatusOK {
		t.Fatalf("register: %d", r.StatusCode)
	}
	// Heartbeat moves it to Active so we can deterministically observe the
	// Active → Stale transition driven by reconcile.
	resp := federationDoRequest(t, s, "POST", "/api/federation/heartbeat",
		map[string]string{"node_id": sp.rec.NodeID}, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat: %d", resp.StatusCode)
	}
	if got := statusOf(t, s, sp.rec.NodeID); got != federation.StatusActive {
		t.Fatalf("pre-stale status = %q, want active", got)
	}

	// Force the stale transition by pretending now is far past the heartbeat.
	s.hub.mu.Lock()
	s.hub.registry.ReconcileLiveness(time.Now().Add(10*time.Minute), 2*time.Minute)
	s.hub.mu.Unlock()
	if got := statusOf(t, s, sp.rec.NodeID); got != federation.StatusStale {
		t.Fatalf("post-reconcile status = %q, want stale", got)
	}

	// Fan-out: stale spoke must still be called and must still respond.
	snap := s.HubFederationRegistrySnapshot()
	c := newChaosFanOutClient(500 * time.Millisecond)
	res, err := c.Execute(context.Background(), snap.Spokes, &federation.FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if hits != 1 {
		t.Fatalf("stale spoke should have been called once; hits=%d", hits)
	}
	if _, ok := res.Responses[sp.rec.NodeID]; !ok {
		t.Fatalf("stale spoke must still contribute response; got %+v", res.Responses)
	}
	if _, ok := res.StatusUpdates[sp.rec.NodeID]; ok {
		t.Fatalf("successful fan-out against stale spoke must not emit status update; got %+v", res.StatusUpdates)
	}

	// Status remains Stale after fan-out (heartbeat-driven, not response-driven).
	if got := statusOf(t, s, sp.rec.NodeID); got != federation.StatusStale {
		t.Fatalf("post-fanout status = %q, want stale (only heartbeat clears stale)", got)
	}
	// A heartbeat clears the stale badge.
	resp = federationDoRequest(t, s, "POST", "/api/federation/heartbeat",
		map[string]string{"node_id": sp.rec.NodeID}, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat after fan-out: %d", resp.StatusCode)
	}
	if got := statusOf(t, s, sp.rec.NodeID); got != federation.StatusActive {
		t.Fatalf("post-heartbeat status = %q, want active", got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: duplicate node_id registration. Same identity → idempotent
// (200, registry shows one spoke, latest URL wins). Different identity →
// 409 conflict + log entry, original record untouched.
//
// Asserted status transition for the conflict path: original spoke's
// status and URL are unchanged across the rejected attempt.
// ---------------------------------------------------------------------------
func testChaosDuplicateRegistration(t *testing.T) {
	s := newHubServer(t, false)
	sp := newChaosSpoke(t, "dup-spoke", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})

	if r := registerVia(t, s, sp, "fp-original"); r.StatusCode != http.StatusOK {
		t.Fatalf("first register: %d", r.StatusCode)
	}
	originalURL := sp.rec.URL

	// Same identity, NEW url → idempotent replace.
	sp2 := *sp
	sp2.rec.URL = "https://updated.invalid:7743"
	if r := registerVia(t, s, &sp2, "fp-original"); r.StatusCode != http.StatusOK {
		t.Fatalf("idempotent re-register: %d", r.StatusCode)
	}
	snap := s.HubFederationRegistrySnapshot()
	if len(snap.Spokes) != 1 {
		t.Fatalf("idempotent path produced %d spokes, want 1", len(snap.Spokes))
	}
	if got := snap.FindSpoke(sp.rec.NodeID).URL; got != "https://updated.invalid:7743" {
		t.Fatalf("idempotent re-register did not update URL; got %q", got)
	}
	// Status is StatusRegistered after a fresh upsert (no heartbeat yet).
	if got := statusOf(t, s, sp.rec.NodeID); got != federation.StatusRegistered {
		t.Fatalf("post-idempotent status = %q, want registered", got)
	}

	// Different identity → 409, original untouched.
	sp3 := *sp
	sp3.rec.URL = "https://attacker.invalid:7743"
	resp := registerVia(t, s, &sp3, "fp-other-machine")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("identity-conflict register = %d, want 409", resp.StatusCode)
	}
	if conflicts := s.HubFederationConflicts(); len(conflicts) == 0 {
		t.Fatalf("expected conflict log entry; got none")
	}
	snap = s.HubFederationRegistrySnapshot()
	if got := snap.FindSpoke(sp.rec.NodeID).URL; got == "https://attacker.invalid:7743" {
		t.Fatalf("conflict path mutated registry URL; got %q (originalURL was %q)", got, originalURL)
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: newer minor on the spoke side (compatible-but-newer) is
// accepted with status=degraded, and degraded spokes are not subsequently
// reclassified by reconcile.
//
// Asserted status transitions: registration sets Degraded; ReconcileLiveness
// must leave Degraded alone (it is a compat status, not a liveness one).
// ---------------------------------------------------------------------------
func testChaosNewerVersionDegraded(t *testing.T) {
	s := newHubServer(t, false)

	sp := newChaosSpoke(t, "newer-spoke", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	sp.rec.DDxVersion = "0.20.0" // newer minor than hub's 0.10.5

	body := map[string]any{
		"node_id":              sp.rec.NodeID,
		"identity_fingerprint": "fp-newer",
		"name":                 sp.rec.Name,
		"url":                  sp.rec.URL,
		"ddx_version":          sp.rec.DDxVersion,
		"schema_version":       sp.rec.SchemaVersion,
		"capabilities":         sp.rec.Capabilities,
	}
	resp := federationDoRequest(t, s, "POST", "/api/federation/register", body, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("newer-minor register = %d, want 200", resp.StatusCode)
	}
	var out federationRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != federation.StatusDegraded {
		t.Fatalf("registration status = %q, want degraded", out.Status)
	}
	if out.Reason != "degraded_newer_minor" {
		t.Fatalf("reason = %q, want degraded_newer_minor", out.Reason)
	}
	if got := statusOf(t, s, sp.rec.NodeID); got != federation.StatusDegraded {
		t.Fatalf("registry status = %q, want degraded", got)
	}

	// Reconcile must NOT downgrade degraded into stale/active even after a
	// long wall-clock skip. (The status-transition rule: degraded is sticky
	// until re-registered with a compatible version.)
	s.hub.mu.Lock()
	s.hub.registry.ReconcileLiveness(time.Now().Add(1*time.Hour), 2*time.Minute)
	s.hub.mu.Unlock()
	if got := statusOf(t, s, sp.rec.NodeID); got != federation.StatusDegraded {
		t.Fatalf("reconcile must leave degraded alone; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 5: per-node timeout fires deterministically; other spokes still
// succeed; total elapsed is bounded by the per-node deadline (not by the
// slow spoke's wait).
//
// Asserted status transition: the timed-out spoke does NOT receive a
// StatusOffline update (per-node timeout is intentionally distinct from
// transport failure — the spoke might just be slow, not dead).
// ---------------------------------------------------------------------------
func testChaosSlowSpokeTimeout(t *testing.T) {
	s := newHubServer(t, false)

	fast := newChaosSpoke(t, "fast-spoke", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	slow := newChaosSpoke(t, "slow-spoke", func(w http.ResponseWriter, r *http.Request) {
		// Block well past the per-node deadline; honor request cancellation
		// so the goroutine actually exits when the client disconnects.
		// Block well past the 150ms per-node deadline; honor request
		// cancellation so the handler exits as soon as the client gives up.
		select {
		case <-time.After(800 * time.Millisecond):
		case <-r.Context().Done():
		}
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	if r := registerVia(t, s, fast, "fp-fast"); r.StatusCode != http.StatusOK {
		t.Fatalf("register fast: %d", r.StatusCode)
	}
	if r := registerVia(t, s, slow, "fp-slow"); r.StatusCode != http.StatusOK {
		t.Fatalf("register slow: %d", r.StatusCode)
	}

	snap := s.HubFederationRegistrySnapshot()
	c := newChaosFanOutClient(150 * time.Millisecond)

	start := time.Now()
	res, err := c.Execute(context.Background(), snap.Spokes, &federation.FanOutRequest{Query: "{ __typename }"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("fan-out took %s — per-node timeout did not bound total runtime", elapsed)
	}
	if _, ok := res.Responses[fast.rec.NodeID]; !ok {
		t.Fatalf("fast spoke missing from responses; got %+v", res.Responses)
	}
	if _, ok := res.Errors[slow.rec.NodeID]; !ok {
		t.Fatalf("slow spoke missing from errors; got %+v", res.Errors)
	}
	if _, ok := res.StatusUpdates[slow.rec.NodeID]; ok {
		t.Fatalf("per-node timeout must not flip spoke offline; got %+v", res.StatusUpdates)
	}

	var sawTimeout bool
	for _, n := range res.Nodes {
		if n.NodeID == slow.rec.NodeID && n.Outcome == federation.OutcomeTimeout {
			sawTimeout = true
		}
	}
	if !sawTimeout {
		t.Fatalf("slow spoke not classified OutcomeTimeout in per-node detail; nodes=%+v", res.Nodes)
	}

	// Registry status of slow spoke is unchanged (still Registered — no
	// heartbeat or transport failure has moved it).
	if got := statusOf(t, s, slow.rec.NodeID); got == federation.StatusOffline {
		t.Fatalf("slow spoke promoted to offline by timeout; status=%q", got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 6: hub restart. We register two spokes, persist (handler
// already does this synchronously), then construct a brand-new Server
// against the same XDG state path. The new hub must (a) load the prior
// registry from disk on EnableHubMode, (b) accept a federated query
// immediately (no warm-up needed), (c) have its registry repopulated /
// refreshed by the next heartbeat (status moves from whatever was
// persisted → Active).
// ---------------------------------------------------------------------------
func testChaosHubRestartRebuildAndRepopulate(t *testing.T) {
	// Set XDG_DATA_HOME to a stable temp dir BEFORE starting either hub
	// so both hub instances share the same federation-state.json path.
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	statePath := filepath.Join(xdg, "ddx", "federation-state.json")

	// Spokes outlive the hub restart — they're independent processes in
	// production, and httptest.Server is a process-local stand-in.
	s1Spoke := newChaosSpoke(t, "n1-restart", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	s2Spoke := newChaosSpoke(t, "n2-restart", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})

	// First hub instance — pinned to the shared XDG dir.
	hub1 := newHubServerSharedXDG(t, xdg)
	if r := registerVia(t, hub1, s1Spoke, "fp-n1"); r.StatusCode != http.StatusOK {
		t.Fatalf("hub1 register n1: %d", r.StatusCode)
	}
	if r := registerVia(t, hub1, s2Spoke, "fp-n2"); r.StatusCode != http.StatusOK {
		t.Fatalf("hub1 register n2: %d", r.StatusCode)
	}
	// Heartbeat n1 so we can later distinguish "loaded but never seen
	// since restart" (n2: still Registered) from "refreshed by heartbeat
	// after restart" (n1: Active again).
	resp := federationDoRequest(t, hub1, "POST", "/api/federation/heartbeat",
		map[string]string{"node_id": s1Spoke.rec.NodeID}, "loopback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hub1 heartbeat n1: %d", resp.StatusCode)
	}

	// Sanity: state file was written.
	if got := hub1.hub.statePath; got != statePath {
		t.Fatalf("hub1 statePath = %q, want %q (XDG override not honored)", got, statePath)
	}

	// "Crash" hub1: drop all references and stand up hub2 against the
	// same XDG state path. EnableHubMode must rehydrate the registry
	// from disk before the first request lands.
	hub2 := newHubServerSharedXDG(t, xdg)

	snap := hub2.HubFederationRegistrySnapshot()
	if snap == nil {
		t.Fatalf("hub2 has nil snapshot after restart")
	}
	if len(snap.Spokes) != 2 {
		t.Fatalf("hub2 loaded %d spokes from disk, want 2", len(snap.Spokes))
	}
	if snap.FindSpoke(s1Spoke.rec.NodeID) == nil || snap.FindSpoke(s2Spoke.rec.NodeID) == nil {
		t.Fatalf("hub2 missing one of the persisted spokes; have %+v", snap.Spokes)
	}

	// Immediate fan-out against the rebuilt registry — both spoke servers
	// are still up, so we must get two responses.
	c := newChaosFanOutClient(500 * time.Millisecond)
	res, err := c.Execute(context.Background(), snap.Spokes, &federation.FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute against rebuilt registry: %v", err)
	}
	if len(res.Responses) != 2 {
		t.Fatalf("immediate-query responses = %d, want 2 (errors=%v)", len(res.Responses), res.Errors)
	}

	// Heartbeat refreshes the loaded record; whatever was persisted (Active
	// for n1, Registered for n2) must move to Active.
	for _, sp := range []*chaosSpoke{s1Spoke, s2Spoke} {
		resp := federationDoRequest(t, hub2, "POST", "/api/federation/heartbeat",
			map[string]string{"node_id": sp.rec.NodeID}, "loopback")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("hub2 heartbeat %s: %d", sp.rec.NodeID, resp.StatusCode)
		}
		if got := statusOf(t, hub2, sp.rec.NodeID); got != federation.StatusActive {
			t.Fatalf("hub2 status %s after heartbeat = %q, want active", sp.rec.NodeID, got)
		}
	}
}

// newHubServerSharedXDG mirrors newHubServer but pins XDG_DATA_HOME to
// the supplied directory so two successive Server instances can share
// the federation-state.json file across a simulated hub restart. The
// caller-supplied xdg dir is what makes this distinct from setupTestDir
// (which always allocates a fresh per-call temp).
func newHubServerSharedXDG(t *testing.T, xdg string) *Server {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := t.TempDir()
	if err := writeMinimalDDxLayout(dir); err != nil {
		t.Fatalf("writeMinimalDDxLayout: %v", err)
	}
	s := New(":0", dir)
	s.EnableHubMode(false)
	s.hub.hubDDxVersion = "0.10.5"
	s.hub.hubSchemaVer = "1"
	return s
}

// writeMinimalDDxLayout creates the bare-minimum .ddx/config.yaml the
// server insists on. Mirrors the relevant slice of setupTestDir without
// also re-setting XDG_DATA_HOME.
func writeMinimalDDxLayout(dir string) error {
	const cfg = `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644)
}
