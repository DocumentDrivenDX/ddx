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

func TestFederationForwardMutation_RoutesToOwner(t *testing.T) {
	s := newHubServer(t, false)
	setServerIdentity(t, s, "coord-456")

	var calls int
	spoke := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"forwarded":true}}`))
	}))
	t.Cleanup(spoke.Close)

	s.hub.mu.Lock()
	s.hub.registry = federation.NewRegistry()
	if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
		NodeID:     "node-a",
		Name:       "alpha",
		URL:        spoke.URL,
		ProjectIDs: []string{"proj-a"},
		Capabilities: []string{
			"read",
			"write",
		},
		Status: federation.StatusActive,
	}); err != nil {
		s.hub.mu.Unlock()
		t.Fatalf("upsert spoke: %v", err)
	}
	s.hub.mu.Unlock()

	resp, err := newHubFederationProvider(s).ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
		ForwardingPath:  []string{"hub-node", "node-a"},
		TargetNodeID:    "node-a",
		TargetProjectID: "proj-a",
		Body:            []byte(`{"query":"mutation { doThing }"}`),
	})
	if err != nil {
		t.Fatalf("forward mutation: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one spoke call, got %d", calls)
	}
	if resp.TargetNodeID != "node-a" || resp.TargetProjectID != "proj-a" {
		t.Fatalf("response target metadata mismatch: %+v", resp)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d", resp.StatusCode)
	}
}

func TestFederationForwardMutation_PreservesForwardingMetadata(t *testing.T) {
	s := newHubServer(t, false)
	setServerIdentity(t, s, "coord-456")

	var calls int
	var gotMethod string
	var gotPath string
	var gotHeaders http.Header
	var gotBody []byte
	spoke := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"forwarded":true}}`))
	}))
	t.Cleanup(spoke.Close)

	s.hub.mu.Lock()
	s.hub.registry = federation.NewRegistry()
	if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
		NodeID:     "node-a",
		Name:       "alpha",
		URL:        spoke.URL,
		ProjectIDs: []string{"proj-a"},
		Capabilities: []string{
			"read",
			"write",
		},
		Status: federation.StatusActive,
	}); err != nil {
		s.hub.mu.Unlock()
		t.Fatalf("upsert spoke: %v", err)
	}
	s.hub.mu.Unlock()

	provider := newHubFederationProvider(s)
	expectedVersion := "rev-17"
	resp, err := provider.ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
		OriginIdentity:       "localhost:127.0.0.1:55812",
		ForwardingPath:       []string{"hub-node", "node-a"},
		RequestID:            "req-123",
		IdempotencyKey:       "idem-456",
		TargetNodeID:         "node-a",
		TargetProjectID:      "proj-a",
		ExpectedVersion:      &expectedVersion,
		RequiredCapabilities: []string{"write"},
		Body:                 []byte(`{"query":"mutation { doThing }"}`),
		Headers: map[string]string{
			"X-DDx-Origin-Identity": "localhost:127.0.0.1:55812",
		},
	})
	if err != nil {
		t.Fatalf("forward mutation: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one spoke call, got %d", calls)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("spoke method = %s, want POST", gotMethod)
	}
	if gotPath != "/graphql" {
		t.Fatalf("spoke path = %s, want /graphql", gotPath)
	}
	if got := gotHeaders.Get("X-DDx-Origin-Identity"); got != "localhost:127.0.0.1:55812" {
		t.Fatalf("origin identity header = %q", got)
	}
	if got := gotHeaders.Get("X-DDx-Forwarding-Path"); got != "hub-node -> node-a" {
		t.Fatalf("forwarding path header = %q", got)
	}
	if got := gotHeaders.Get("X-DDx-Request-ID"); got != "req-123" {
		t.Fatalf("request id header = %q", got)
	}
	if got := gotHeaders.Get("X-DDx-Idempotency-Key"); got != "idem-456" {
		t.Fatalf("idempotency key header = %q", got)
	}
	if got := gotHeaders.Get("X-DDx-Target-Project-ID"); got != "proj-a" {
		t.Fatalf("target project header = %q", got)
	}
	if got := gotHeaders.Get("X-DDx-Expected-Version"); got != expectedVersion {
		t.Fatalf("expected version header = %q", got)
	}
	if string(gotBody) != `{"query":"mutation { doThing }"}` {
		t.Fatalf("spoke body = %s", string(gotBody))
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d", resp.StatusCode)
	}
	if got := string(resp.Body); got != `{"data":{"forwarded":true}}` {
		t.Fatalf("response body = %s", got)
	}
	if resp.TargetNodeID != "node-a" || resp.TargetProjectID != "proj-a" {
		t.Fatalf("response target metadata mismatch: %+v", resp)
	}
	if resp.RequestID != "req-123" || resp.IdempotencyKey != "idem-456" {
		t.Fatalf("response request metadata mismatch: %+v", resp)
	}
	if resp.ExpectedVersion == nil || *resp.ExpectedVersion != expectedVersion {
		t.Fatalf("response expected version mismatch: %+v", resp.ExpectedVersion)
	}
	if resp.OriginIdentity != "localhost:127.0.0.1:55812" {
		t.Fatalf("response origin identity = %q", resp.OriginIdentity)
	}
	if len(resp.ForwardingPath) != 2 || resp.ForwardingPath[0] != "hub-node" || resp.ForwardingPath[1] != "node-a" {
		t.Fatalf("response forwarding path = %+v", resp.ForwardingPath)
	}
}

func TestFederationForwardMutation_RejectsOfflineOrReadOnly(t *testing.T) {
	t.Run("offline", func(t *testing.T) {
		s := newHubServer(t, false)
		var calls int
		spoke := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(spoke.Close)

		s.hub.mu.Lock()
		s.hub.registry = federation.NewRegistry()
		if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
			NodeID:     "node-offline",
			Name:       "offline",
			URL:        spoke.URL,
			ProjectIDs: []string{"proj-offline"},
			Status:     federation.StatusOffline,
		}); err != nil {
			s.hub.mu.Unlock()
			t.Fatalf("upsert spoke: %v", err)
		}
		s.hub.mu.Unlock()

		_, err := newHubFederationProvider(s).ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
			ForwardingPath:  []string{"hub-node", "node-offline"},
			TargetNodeID:    "node-offline",
			TargetProjectID: "proj-offline",
			Body:            []byte(`{"query":"mutation { __typename }"}`),
		})
		if !errors.Is(err, federation.ErrForwardMutationOffline) {
			t.Fatalf("offline refusal = %v, want offline", err)
		}
		if calls != 0 {
			t.Fatalf("offline spoke must not be contacted, got %d calls", calls)
		}
	})

	t.Run("stale", func(t *testing.T) {
		s := newHubServer(t, false)
		var calls int
		spoke := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(spoke.Close)

		s.hub.mu.Lock()
		s.hub.registry = federation.NewRegistry()
		if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
			NodeID:     "node-stale",
			Name:       "stale",
			URL:        spoke.URL,
			ProjectIDs: []string{"proj-stale"},
			Status:     federation.StatusStale,
		}); err != nil {
			s.hub.mu.Unlock()
			t.Fatalf("upsert spoke: %v", err)
		}
		s.hub.mu.Unlock()

		_, err := newHubFederationProvider(s).ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
			ForwardingPath:  []string{"hub-node", "node-stale"},
			TargetNodeID:    "node-stale",
			TargetProjectID: "proj-stale",
			Body:            []byte(`{"query":"mutation { __typename }"}`),
		})
		if !errors.Is(err, federation.ErrForwardMutationStale) {
			t.Fatalf("stale refusal = %v, want stale", err)
		}
		if calls != 0 {
			t.Fatalf("stale spoke must not be contacted, got %d calls", calls)
		}
	})

	t.Run("read_only", func(t *testing.T) {
		s := newHubServer(t, false)
		var calls int
		spoke := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(spoke.Close)

		s.hub.mu.Lock()
		s.hub.registry = federation.NewRegistry()
		if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
			NodeID:     "node-readonly",
			Name:       "readonly",
			URL:        spoke.URL,
			ProjectIDs: []string{"proj-readonly"},
			Capabilities: []string{
				"read",
			},
			Status: federation.StatusActive,
		}); err != nil {
			s.hub.mu.Unlock()
			t.Fatalf("upsert spoke: %v", err)
		}
		s.hub.mu.Unlock()

		_, err := newHubFederationProvider(s).ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
			ForwardingPath:  []string{"hub-node", "node-readonly"},
			TargetNodeID:    "node-readonly",
			TargetProjectID: "proj-readonly",
			Body:            []byte(`{"query":"mutation { __typename }"}`),
		})
		if !errors.Is(err, federation.ErrForwardMutationReadOnly) {
			t.Fatalf("read_only refusal = %v, want read_only", err)
		}
		if calls != 0 {
			t.Fatalf("read-only spoke must not be contacted, got %d calls", calls)
		}
	})
}

func TestFederationForwardMutation_RejectsBroadcastTarget(t *testing.T) {
	t.Run("missing owner", func(t *testing.T) {
		s := newHubServer(t, false)
		s.hub.mu.Lock()
		s.hub.registry = federation.NewRegistry()
		if err := s.hub.registry.UpsertSpoke(federation.SpokeRecord{
			NodeID:     "node-a",
			Name:       "alpha",
			URL:        "https://alpha.example",
			ProjectIDs: []string{"proj-a"},
			Status:     federation.StatusActive,
		}); err != nil {
			s.hub.mu.Unlock()
			t.Fatalf("upsert spoke: %v", err)
		}
		s.hub.mu.Unlock()

		_, err := newHubFederationProvider(s).ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
			ForwardingPath:  []string{"hub-node", "node-a"},
			TargetNodeID:    "node-a",
			TargetProjectID: "proj-missing",
			Body:            []byte(`{"query":"mutation { __typename }"}`),
		})
		var refusal *federation.ForwardMutationRefusalError
		if !errors.As(err, &refusal) || refusal.Kind != federation.ForwardMutationRefusalMissingOwner {
			t.Fatalf("missing owner refusal = %v, want missing-owner", err)
		}
	})

	t.Run("multi-owner", func(t *testing.T) {
		s := newHubServer(t, false)
		s.hub.mu.Lock()
		s.hub.registry = federation.NewRegistry()
		for _, spoke := range []federation.SpokeRecord{
			{
				NodeID:     "node-a",
				Name:       "alpha",
				URL:        "https://alpha.example",
				ProjectIDs: []string{"proj-shared"},
				Status:     federation.StatusActive,
			},
			{
				NodeID:     "node-b",
				Name:       "beta",
				URL:        "https://beta.example",
				ProjectIDs: []string{"proj-shared"},
				Status:     federation.StatusActive,
			},
		} {
			if err := s.hub.registry.UpsertSpoke(spoke); err != nil {
				s.hub.mu.Unlock()
				t.Fatalf("upsert spoke %s: %v", spoke.NodeID, err)
			}
		}
		s.hub.mu.Unlock()

		_, err := newHubFederationProvider(s).ForwardMutation(context.Background(), &federation.ForwardMutationRequest{
			ForwardingPath:  []string{"hub-node", "node-a"},
			TargetNodeID:    "node-a",
			TargetProjectID: "proj-shared",
			Body:            []byte(`{"query":"mutation { __typename }"}`),
		})
		var refusal *federation.ForwardMutationRefusalError
		if !errors.As(err, &refusal) || refusal.Kind != federation.ForwardMutationRefusalBroadcastLike {
			t.Fatalf("multi-owner refusal = %v, want broadcast-like", err)
		}
	})
}
