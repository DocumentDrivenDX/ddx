package server

// B14.8b-pre: prove the production wiring path that connects the hub
// registry to the federationNodes GraphQL resolver. The other federation
// tests (resolver_federation_test.go) construct a *graphql.Resolver
// directly and inject a stub FederationProvider — they would still pass
// even if Server.handleGraphQLQuery never set Resolver.Federation. This
// test boots a real Server, calls EnableHubMode, registers a spoke via
// the real /api/federation/register HTTP route, posts a federationNodes
// query to /graphql, and asserts the spoke surfaces. If the wiring in
// server.go (the `if s.hub != nil { fedProvider = newHubFederationProvider(s) }`
// block) regresses to a nil provider the test fails.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

func TestFederationGraphQLWiringSurfacesRegisteredSpoke(t *testing.T) {
	s := newHubServer(t, false)

	// Register a spoke through the real HTTP register handler so the hub
	// registry is populated the same way production traffic populates it.
	if r := federationDoRequest(t, s, "POST", "/api/federation/register",
		goodRegisterPayload("wired"), "loopback"); r.StatusCode != 200 {
		t.Fatalf("register: %d", r.StatusCode)
	}

	// Hit /graphql through Server.Handler() so we exercise the production
	// wiring (handleGraphQLQuery → Resolver.Federation → hubFederationProvider
	// → hub.registry). Loopback RemoteAddr passes the isTrusted gate.
	body := map[string]any{
		"query": `{ federationNodes { nodeId status } }`,
	}
	resp := federationDoRequest(t, s, "POST", "/graphql", body, "loopback")
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("graphql POST: status=%d body=%s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data struct {
			FederationNodes []struct {
				NodeID string `json:"nodeId"`
				Status string `json:"status"`
			} `json:"federationNodes"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode graphql response: %v", err)
	}
	if len(out.Errors) > 0 {
		var msgs []string
		for _, e := range out.Errors {
			msgs = append(msgs, e.Message)
		}
		t.Fatalf("graphql errors: %s", strings.Join(msgs, "; "))
	}
	if len(out.Data.FederationNodes) != 1 {
		t.Fatalf("federationNodes: want 1 row, got %d (%+v)", len(out.Data.FederationNodes), out.Data.FederationNodes)
	}
	if got := out.Data.FederationNodes[0].NodeID; got != "wired" {
		t.Fatalf("federationNodes[0].nodeId = %q, want %q", got, "wired")
	}
}

// Without hub mode, Resolver.Federation must remain nil and the resolver
// must degrade to an empty list — not panic, not error.
func TestFederationGraphQLWiringEmptyWithoutHubMode(t *testing.T) {
	dir := setupTestDir(t)
	s := New(":0", dir)

	body := map[string]any{
		"query": `{ federationNodes { nodeId } }`,
	}
	resp := federationDoRequest(t, s, "POST", "/graphql", body, "loopback")
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("graphql POST: status=%d body=%s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data struct {
			FederationNodes []struct {
				NodeID string `json:"nodeId"`
			} `json:"federationNodes"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Errors) > 0 {
		t.Fatalf("graphql errors with hub mode off: %+v", out.Errors)
	}
	if len(out.Data.FederationNodes) != 0 {
		t.Fatalf("federationNodes without hub mode: want 0 rows, got %d", len(out.Data.FederationNodes))
	}
}

// TestFederatedWrite_RejectsOfflineSpoke asserts the owner-targeted write path
// fails closed when the owning spoke is offline: typed offline refusal, no
// spoke HTTP POST, and no hub-local write fallback.
func TestFederatedWrite_RejectsOfflineSpoke(t *testing.T) {
	s := newHubServer(t, false)
	var calls int
	spoke := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"shouldNotHappen":true}}`))
	}))
	t.Cleanup(spoke.Close)

	s.hub.mu.Lock()
	s.hub.registry = federation.NewRegistry()
	if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
		NodeID:       "node-offline",
		Name:         "offline",
		URL:          spoke.URL,
		ProjectIDs:   []string{"proj-offline"},
		Capabilities: []string{"read", "write"},
		Status:       federation.StatusOffline,
	}); err != nil {
		s.hub.mu.Unlock()
		t.Fatalf("upsert spoke: %v", err)
	}
	s.hub.mu.Unlock()

	_, err := newHubFederationProvider(s).ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
		ForwardingPath:  []string{"hub-node", "node-offline"},
		RequestID:       "req-offline",
		TargetNodeID:    "node-offline",
		TargetProjectID: "proj-offline",
		Body:            []byte(`{"query":"mutation { createThing }"}`),
	})
	if !errors.Is(err, federation.ErrForwardMutationOffline) {
		t.Fatalf("offline refusal = %v, want offline", err)
	}
	if calls != 0 {
		t.Fatalf("offline spoke must not receive a POST, got %d calls", calls)
	}
}

// TestFederatedWrite_IdempotentRequestID shows a second ForwardMutation with
// the same RequestID returns the cached response without a second spoke POST.
func TestFederatedWrite_IdempotentRequestID(t *testing.T) {
	s := newHubServer(t, false)
	setServerIdentity(t, s, "coord-456")

	var calls int
	spoke := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("X-Spoke-Call", "first")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"created":true}}`))
	}))
	t.Cleanup(spoke.Close)

	s.hub.mu.Lock()
	s.hub.registry = federation.NewRegistry()
	if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
		NodeID:       "node-a",
		Name:         "alpha",
		URL:          spoke.URL,
		ProjectIDs:   []string{"proj-a"},
		Capabilities: []string{"read", "write"},
		Status:       federation.StatusActive,
	}); err != nil {
		s.hub.mu.Unlock()
		t.Fatalf("upsert spoke: %v", err)
	}
	s.hub.mu.Unlock()

	provider := newHubFederationProvider(s)
	req := &federation.ForwardMutationRequest{
		ForwardingPath:  []string{"hub-node", "node-a"},
		RequestID:       "req-replay",
		TargetNodeID:    "node-a",
		TargetProjectID: "proj-a",
		Body:            []byte(`{"query":"mutation { createThing }"}`),
	}
	first, err := provider.ForwardMutation(context.Background(), req)
	if err != nil {
		t.Fatalf("first forward mutation: %v", err)
	}
	// Mutate the returned body so a shallow cache would corrupt the replay.
	first.Body[0] = 'X'

	second, err := provider.ForwardMutation(context.Background(), req)
	if err != nil {
		t.Fatalf("second forward mutation: %v", err)
	}
	if calls != 1 {
		t.Fatalf("same request id must not repeat remote mutation, got %d calls", calls)
	}
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("replayed status = %d", second.StatusCode)
	}
	if got := string(second.Body); got != `{"data":{"created":true}}` {
		t.Fatalf("replayed body = %s", got)
	}
	if got := second.Headers.Get("X-Spoke-Call"); got != "first" {
		t.Fatalf("replayed header = %q", got)
	}
}
