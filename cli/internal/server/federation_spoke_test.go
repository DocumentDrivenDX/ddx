package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// hubFromServer wires the hub-mode Server into a real httptest.Server so the
// spoke under test can hit it over HTTP. The returned closer must be called.
//
// All requests against the returned URL will appear to originate from
// 127.0.0.1, so the hub's strict default trust gate accepts them — exactly
// what we want for the lifecycle integration tests.
func hubFromServer(t *testing.T, srv *Server) (string, func()) {
	t.Helper()
	httpSrv := httptest.NewServer(srv.Handler())
	t.Cleanup(httpSrv.Close)
	return httpSrv.URL, httpSrv.Close
}

// spokeStatePath is the conventional per-test spoke-state location under
// XDG_DATA_HOME (which setupTestDir already isolates).
func spokeStatePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "spoke-state.json")
}

// AC: --hub-address flag registers on startup. Spoke appears in hub registry.
func TestSpokeRegistersOnStart(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)

	dir := setupTestDir(t)
	spoke := New(":0", dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := spoke.EnableSpokeMode(ctx, hubURL, "https://spoke-self:9000",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(time.Hour), // disable heartbeat noise
		WithSpokeNodeID("spoke-1"),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}

	snap := hub.HubFederationRegistrySnapshot()
	if len(snap.Spokes) != 1 {
		t.Fatalf("hub should hold 1 spoke after register, got %d", len(snap.Spokes))
	}
	if snap.Spokes[0].NodeID != "spoke-1" {
		t.Errorf("hub registered wrong node_id: %q", snap.Spokes[0].NodeID)
	}
	if snap.Spokes[0].URL != "https://spoke-self:9000" {
		t.Errorf("hub registered wrong URL: %q", snap.Spokes[0].URL)
	}
	if !spoke.SpokeMode {
		t.Errorf("Server.SpokeMode should be true after EnableSpokeMode")
	}
}

// AC: Re-start with same node_id is idempotent (replaces, not duplicates).
func TestSpokeReregisterIsIdempotent(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)
	statePath := spokeStatePath(t)
	dir := setupTestDir(t)

	for i := 0; i < 3; i++ {
		s := New(":0", dir)
		ctx, cancel := context.WithCancel(context.Background())
		if err := s.EnableSpokeMode(ctx, hubURL, "https://spoke-self:9000",
			WithSpokeStatePath(statePath),
			WithSpokeHeartbeatInterval(time.Hour),
			WithSpokeNodeID("spoke-x"),
		); err != nil {
			t.Fatalf("iteration %d: EnableSpokeMode: %v", i, err)
		}
		_ = s.ShutdownSpoke(ctx)
		cancel()
	}

	snap := hub.HubFederationRegistrySnapshot()
	if len(snap.Spokes) > 1 {
		t.Errorf("idempotent re-registration produced %d spokes; want ≤1", len(snap.Spokes))
	}
}

// AC: URL change between restarts triggers re-registration with new URL.
func TestSpokeReregistersOnURLChange(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)
	statePath := spokeStatePath(t)
	dir := setupTestDir(t)

	s1 := New(":0", dir)
	ctx1, cancel1 := context.WithCancel(context.Background())
	if err := s1.EnableSpokeMode(ctx1, hubURL, "https://spoke-old:9000",
		WithSpokeStatePath(statePath),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("spoke-roving"),
	); err != nil {
		t.Fatalf("first start: %v", err)
	}
	cancel1()

	// Restart with a new URL.
	s2 := New(":0", dir)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if err := s2.EnableSpokeMode(ctx2, hubURL, "https://spoke-new:9000",
		WithSpokeStatePath(statePath),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("spoke-roving"),
	); err != nil {
		t.Fatalf("second start: %v", err)
	}

	if !s2.SpokeAgent().URLChanged() {
		t.Errorf("expected URLChanged=true after url change between restarts")
	}
	if got := s2.SpokeAgent().PreviousURL(); got != "https://spoke-old:9000" {
		t.Errorf("PreviousURL: want old URL, got %q", got)
	}

	snap := hub.HubFederationRegistrySnapshot()
	if len(snap.Spokes) != 1 {
		t.Fatalf("want 1 spoke after URL change, got %d", len(snap.Spokes))
	}
	if snap.Spokes[0].URL != "https://spoke-new:9000" {
		t.Errorf("hub did not pick up new URL: got %q", snap.Spokes[0].URL)
	}
}

// AC: Heartbeat interval 30s ± jitter. We can't directly assert 30s in a
// test (it would be slow), but we verify (a) the default is 30s, (b) the
// jitter window is non-zero and bounded by the configured fraction, and
// (c) heartbeats actually fire and flip the spoke status to "active".
func TestSpokeHeartbeatJitterBounds(t *testing.T) {
	if got := federation.DefaultHeartbeatInterval; got != 30*time.Second {
		t.Errorf("DefaultHeartbeatInterval drift: %v want 30s", got)
	}
	cfg := federation.SpokeConfig{
		NodeID:                  "n1",
		URL:                     "https://x",
		HubURL:                  "https://hub",
		HeartbeatInterval:       1 * time.Second,
		HeartbeatJitterFraction: 0.25,
		StatePath:               filepath.Join(t.TempDir(), "spoke.json"),
	}
	sp, err := federation.NewSpoke(cfg)
	if err != nil {
		t.Fatalf("NewSpoke: %v", err)
	}
	// Sample many delays — none should fall outside ±25% of the base interval.
	low := time.Duration(float64(cfg.HeartbeatInterval) * 0.75)
	high := time.Duration(float64(cfg.HeartbeatInterval) * 1.25)
	sawJitter := false
	for i := 0; i < 200; i++ {
		d := sp.NextHeartbeatDelay()
		if d < low || d > high {
			t.Fatalf("delay %v outside ±25%% bounds [%v,%v]", d, low, high)
		}
		if d != cfg.HeartbeatInterval {
			sawJitter = true
		}
	}
	if !sawJitter {
		t.Errorf("expected jitter to vary delays; saw constant %v", cfg.HeartbeatInterval)
	}
}

// Heartbeats actually flip status to active end-to-end.
func TestSpokeHeartbeatFlipsStatusActive(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)
	dir := setupTestDir(t)

	spoke := New(":0", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := spoke.EnableSpokeMode(ctx, hubURL, "https://spoke:9000",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(20*time.Millisecond),
		WithSpokeHeartbeatJitter(0.01),
		WithSpokeNodeID("spoke-hb"),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}

	// Wait for at least one heartbeat to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if spoke.SpokeAgent().HeartbeatCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if spoke.SpokeAgent().HeartbeatCount() < 1 {
		t.Fatalf("heartbeat never fired; count=%d err=%v",
			spoke.SpokeAgent().HeartbeatCount(), spoke.SpokeAgent().LastHeartbeatError())
	}

	snap := hub.HubFederationRegistrySnapshot()
	if len(snap.Spokes) != 1 {
		t.Fatalf("want 1 spoke, got %d", len(snap.Spokes))
	}
	if snap.Spokes[0].Status != federation.StatusActive {
		t.Errorf("want active after heartbeat, got %q", snap.Spokes[0].Status)
	}
}

// AC: Graceful shutdown sends deregister (best-effort).
func TestSpokeDeregistersOnShutdown(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)
	dir := setupTestDir(t)

	spoke := New(":0", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := spoke.EnableSpokeMode(ctx, hubURL, "https://spoke:9000",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("spoke-bye"),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}
	if got := len(hub.HubFederationRegistrySnapshot().Spokes); got != 1 {
		t.Fatalf("pre-shutdown: want 1 spoke, got %d", got)
	}

	if err := spoke.ShutdownSpoke(ctx); err != nil {
		t.Fatalf("ShutdownSpoke: %v", err)
	}
	if got := len(hub.HubFederationRegistrySnapshot().Spokes); got != 0 {
		t.Errorf("post-shutdown: want 0 spokes (deregistered), got %d", got)
	}
}

// Best-effort deregister returns nil even if the hub is unreachable.
func TestSpokeDeregisterBestEffortOnDeadHub(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, closeHub := hubFromServer(t, hub)

	dir := setupTestDir(t)
	spoke := New(":0", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := spoke.EnableSpokeMode(ctx, hubURL, "https://spoke:9000",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("spoke-orphan"),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}

	// Kill the hub. Deregister must not error.
	closeHub()
	if err := spoke.ShutdownSpoke(ctx); err != nil {
		t.Errorf("best-effort shutdown should not surface error; got %v", err)
	}
}

// AC: --hub-mode and --hub-address both set produces hub_spoke role.
func TestFederationRoleHubSpoke(t *testing.T) {
	// Spawn a peer hub that this server registers with.
	peer := newHubServer(t, false)
	peerURL, _ := hubFromServer(t, peer)

	dir := setupTestDir(t)
	s := New(":0", dir)
	s.EnableHubMode(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.EnableSpokeMode(ctx, peerURL, "https://hubspoke:9000",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("hubspoke-1"),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}
	if got := s.federationRole(); got != "hub_spoke" {
		t.Errorf("federationRole: want hub_spoke, got %q", got)
	}
}

// AC: federationRole is correct in each combination.
func TestFederationRoleMatrix(t *testing.T) {
	cases := []struct {
		name string
		hub  bool
		spk  bool
		want string
	}{
		{"standalone", false, false, "standalone"},
		{"hub-only", true, false, "hub"},
		{"spoke-only", false, true, "spoke"},
		{"hub-spoke", true, true, "hub_spoke"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := setupTestDir(t)
			s := New(":0", dir)
			s.HubMode = c.hub
			s.SpokeMode = c.spk
			if got := s.federationRole(); got != c.want {
				t.Errorf("want %q got %q", c.want, got)
			}
		})
	}
}

// AC: /api/node exposes federation_role.
func TestNodeAPIExposesFederationRole(t *testing.T) {
	dir := setupTestDir(t)
	s := New(":0", dir)
	s.HubMode = true

	req := httptest.NewRequest(http.MethodGet, "/api/node", nil)
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, _ := body["federation_role"].(string); got != "hub" {
		t.Errorf("federation_role: want hub, got %q (body=%v)", got, body)
	}
}

// EnableSpokeMode rejects empty hub URL.
func TestEnableSpokeModeRequiresHubURL(t *testing.T) {
	dir := setupTestDir(t)
	s := New(":0", dir)
	err := s.EnableSpokeMode(context.Background(), "", "https://x",
		WithSpokeStatePath(spokeStatePath(t)),
	)
	if err == nil || !strings.Contains(err.Error(), "URL required") {
		t.Errorf("want hub-URL-required error, got %v", err)
	}
}

// EnableSpokeMode is idempotent: a second call with different args is a no-op.
func TestEnableSpokeModeIdempotent(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)
	dir := setupTestDir(t)
	s := New(":0", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.EnableSpokeMode(ctx, hubURL, "https://x:1",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("idempo"),
	); err != nil {
		t.Fatalf("first call: %v", err)
	}
	first := s.SpokeAgent()
	if err := s.EnableSpokeMode(ctx, hubURL, "https://OTHER:2",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("idempo"),
	); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if s.SpokeAgent() != first {
		t.Errorf("second EnableSpokeMode replaced the agent; should have been no-op")
	}
}

// Spoke state file persists URL across restarts (whitebox).
func TestSpokeStatePersistsURL(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)
	statePath := spokeStatePath(t)
	dir := setupTestDir(t)

	s := New(":0", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.EnableSpokeMode(ctx, hubURL, "https://persisted:9000",
		WithSpokeStatePath(statePath),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("persist-1"),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}

	st, err := federation.LoadSpokeState(statePath)
	if err != nil {
		t.Fatalf("LoadSpokeState: %v", err)
	}
	if st.LastURL != "https://persisted:9000" {
		t.Errorf("LastURL not persisted: %q", st.LastURL)
	}
	if st.NodeID != "persist-1" {
		t.Errorf("NodeID not persisted: %q", st.NodeID)
	}
	if st.IdentityFingerprint == "" {
		t.Errorf("IdentityFingerprint empty")
	}
}

// Sanity: hub URL parsing handles trailing slashes.
func TestHubURLTrailingSlashHandled(t *testing.T) {
	hub := newHubServer(t, false)
	hubURL, _ := hubFromServer(t, hub)
	// Append a trailing slash.
	dir := setupTestDir(t)
	s := New(":0", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.EnableSpokeMode(ctx, hubURL+"/", "https://x:1",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeHeartbeatInterval(time.Hour),
		WithSpokeNodeID("trailing-slash"),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}
	if got := len(hub.HubFederationRegistrySnapshot().Spokes); got != 1 {
		t.Errorf("hub did not register spoke when hub URL had trailing slash; spokes=%d", got)
	}
}

// Sanity: hub URL must parse.
func TestEnableSpokeModeRejectsBogusHubURL(t *testing.T) {
	dir := setupTestDir(t)
	s := New(":0", dir)
	err := s.EnableSpokeMode(context.Background(), "://broken", "https://x",
		WithSpokeStatePath(spokeStatePath(t)),
		WithSpokeNodeID("bogus"),
	)
	if err == nil {
		t.Errorf("expected error from bogus hub URL")
	}
}
