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
	"encoding/json"
	"io"
	"strings"
	"testing"
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
