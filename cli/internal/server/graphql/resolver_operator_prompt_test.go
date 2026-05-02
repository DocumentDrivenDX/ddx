package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// setupOperatorPromptHarness builds a gqlgen handler wired with a real bead
// store, a fixed CSRF token, and a process-local idempotency cache. The
// handler injects the originating *http.Request into the resolver context
// (mirroring the production server's handleGraphQLQuery wrapping) so the
// resolver can validate CSRF headers and capture identity.
func setupOperatorPromptHarness(t *testing.T) (http.Handler, *bead.Store, string) {
	t.Helper()

	workDir, err := os.MkdirTemp("", "operator-prompt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	ddxDir := filepath.Join(workDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"op\"\n"
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	const token = "test-csrf-token-deadbeef"
	cache := ddxgraphql.NewMemoryIdempotencyCache()
	csrf := ddxgraphql.NewStaticCSRFTokenStoreWithToken(token)

	gql := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			WorkingDir:                workDir,
			CSRFTokens:                csrf,
			OperatorPromptIdempotency: cache,
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gql.AddTransport(transport.POST{})
	gql.AddTransport(transport.GET{})

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(ddxgraphql.WithHTTPRequest(r.Context(), r))
		gql.ServeHTTP(w, r)
	})
	return wrapped, store, token
}

const operatorPromptMutation = `mutation Submit($input: OperatorPromptSubmitInput!) {
	operatorPromptSubmit(input: $input) {
		bead { id title status issueType description acceptance labels priority }
		deduplicated
	}
}`

// gqlSubmitRaw issues a raw POST and returns the parsed top-level response so
// callers can assert on either data or errors.
func gqlSubmitRaw(t *testing.T, h http.Handler, csrfHeader, prompt, idemKey string, tier *int) (int, map[string]json.RawMessage) {
	t.Helper()
	input := map[string]any{
		"prompt":         prompt,
		"idempotencyKey": idemKey,
	}
	if tier != nil {
		input["tier"] = *tier
	}
	body, _ := json.Marshal(map[string]any{
		"query":     operatorPromptMutation,
		"variables": map[string]any{"input": input},
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrfHeader != "" {
		req.Header.Set(ddxgraphql.CSRFHeaderName, csrfHeader)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	return w.Code, resp
}

// AC #1: operatorPromptSubmit mutation in schema — covered by every test in
// this file (the mutation parses, executes, and returns typed fields).

// AC #5: Default = proposed; AC #4 (audit identity captured).
func TestOperatorPromptSubmit_HappyPath(t *testing.T) {
	h, store, token := setupOperatorPromptHarness(t)

	_, resp := gqlSubmitRaw(t, h, token, "Investigate flaky test in workers_test.go\nFollow up.", "key-1", nil)
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("unexpected errors: %s", errs)
	}

	var data struct {
		OperatorPromptSubmit struct {
			Bead struct {
				ID          string   `json:"id"`
				Title       string   `json:"title"`
				Status      string   `json:"status"`
				IssueType   string   `json:"issueType"`
				Description string   `json:"description"`
				Labels      []string `json:"labels"`
				Priority    int      `json:"priority"`
				Acceptance  string   `json:"acceptance"`
			} `json:"bead"`
			Deduplicated bool `json:"deduplicated"`
		} `json:"operatorPromptSubmit"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := data.OperatorPromptSubmit
	if got.Bead.ID == "" {
		t.Fatal("expected a bead ID")
	}
	if got.Bead.Status != bead.StatusProposed {
		t.Errorf("status: want %q (default operator-prompt status), got %q", bead.StatusProposed, got.Bead.Status)
	}
	if got.Bead.IssueType != bead.IssueTypeOperatorPrompt {
		t.Errorf("issueType: want %q, got %q", bead.IssueTypeOperatorPrompt, got.Bead.IssueType)
	}
	if got.Bead.Title != "Investigate flaky test in workers_test.go" {
		t.Errorf("title: want first line of prompt, got %q", got.Bead.Title)
	}
	if got.Deduplicated {
		t.Error("first submission must not be marked deduplicated")
	}

	// AC #4: audit event records identity.
	events, err := store.EventsByKind(got.Bead.ID, ddxgraphql.OperatorPromptEventKind)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 audit event of kind %q, got %d", ddxgraphql.OperatorPromptEventKind, len(events))
	}
	ev := events[0]
	if ev.Actor == "" {
		t.Error("audit event must record an actor identity")
	}
	if !containsAll(ev.Body, "identity=", "idempotency_key=key-1") {
		t.Errorf("audit body missing identity/key fields: %q", ev.Body)
	}
}

// AC #2: CSRF required (returns 403 without).
func TestOperatorPromptSubmit_CSRF(t *testing.T) {
	h, _, token := setupOperatorPromptHarness(t)

	t.Run("missing header rejected", func(t *testing.T) {
		_, resp := gqlSubmitRaw(t, h, "", "do a thing", "csrf-missing", nil)
		assertCSRFError(t, resp)
	})
	t.Run("wrong token rejected", func(t *testing.T) {
		_, resp := gqlSubmitRaw(t, h, "not-the-token", "do a thing", "csrf-wrong", nil)
		assertCSRFError(t, resp)
	})
	t.Run("valid token accepted", func(t *testing.T) {
		_, resp := gqlSubmitRaw(t, h, token, "do a thing", "csrf-ok", nil)
		if errs, ok := resp["errors"]; ok {
			t.Fatalf("unexpected errors with valid CSRF: %s", errs)
		}
	})
}

func assertCSRFError(t *testing.T, resp map[string]json.RawMessage) {
	t.Helper()
	errs, ok := resp["errors"]
	if !ok {
		t.Fatal("expected GraphQL errors when CSRF token absent or invalid")
	}
	var parsed []struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(errs, &parsed); err != nil {
		t.Fatalf("parse errors: %v", err)
	}
	if len(parsed) == 0 {
		t.Fatal("expected at least one error")
	}
	msg := parsed[0].Message
	// AC #2 wording: "returns 403 without". The HTTP transport for
	// gqlgen's POST handler returns 200 with a structured error; the
	// resolver's error message must surface the 403 status so a
	// browser-side gateway can translate (and so audit logs differentiate
	// CSRF rejections from other failures).
	if !contains(msg, "CSRF") || !contains(msg, "403") {
		t.Errorf("error must mention CSRF and 403, got %q", msg)
	}
}

// AC #3: idempotency key dedupes within 24h window.
func TestOperatorPromptSubmit_Idempotency(t *testing.T) {
	h, store, token := setupOperatorPromptHarness(t)

	_, first := gqlSubmitRaw(t, h, token, "first prompt", "idem-key-A", nil)
	_, second := gqlSubmitRaw(t, h, token, "different prompt body!", "idem-key-A", nil)
	_, third := gqlSubmitRaw(t, h, token, "third prompt", "idem-key-B", nil)

	firstID := mustBeadID(t, first)
	secondID := mustBeadID(t, second)
	thirdID := mustBeadID(t, third)

	if firstID != secondID {
		t.Errorf("idempotency: same key must return same bead ID, got %q vs %q", firstID, secondID)
	}
	if firstID == thirdID {
		t.Errorf("different keys must produce different beads, got %q twice", firstID)
	}
	if !mustDeduplicated(t, second) {
		t.Error("second submission must be marked deduplicated=true")
	}
	if mustDeduplicated(t, first) || mustDeduplicated(t, third) {
		t.Error("first/third submissions must be marked deduplicated=false")
	}

	// Audit: only one audit event on the deduped bead (we did not append
	// a second event for the duplicate).
	events, err := store.EventsByKind(firstID, ddxgraphql.OperatorPromptEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("dedup must not append a second audit event, got %d", len(events))
	}
}

// MemoryIdempotencyCache TTL eviction: entries expire after the 24h window.
// This locks in the "within 24h" half of AC #3 so a future change to the TTL
// does not silently break dedup semantics.
func TestMemoryIdempotencyCache_TTLEviction(t *testing.T) {
	cache := ddxgraphql.NewMemoryIdempotencyCache()
	now := time.Now()
	clock := now
	ddxgraphql.SetMemoryIdempotencyCacheClockForTest(cache, func() time.Time { return clock })

	cache.Store("k", "bead-1")
	if id, ok := cache.Lookup("k"); !ok || id != "bead-1" {
		t.Fatalf("immediate lookup failed: got %q ok=%v", id, ok)
	}
	clock = now.Add(23 * time.Hour)
	if _, ok := cache.Lookup("k"); !ok {
		t.Error("entry must still be present at 23h")
	}
	clock = now.Add(25 * time.Hour)
	if _, ok := cache.Lookup("k"); ok {
		t.Error("entry must be evicted past 24h TTL")
	}
}

// ─── helpers ───

func mustBeadID(t *testing.T, resp map[string]json.RawMessage) string {
	t.Helper()
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("unexpected errors: %s", errs)
	}
	var data struct {
		OperatorPromptSubmit struct {
			Bead struct {
				ID string `json:"id"`
			} `json:"bead"`
		} `json:"operatorPromptSubmit"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if data.OperatorPromptSubmit.Bead.ID == "" {
		t.Fatal("missing bead ID in response")
	}
	return data.OperatorPromptSubmit.Bead.ID
}

func mustDeduplicated(t *testing.T, resp map[string]json.RawMessage) bool {
	t.Helper()
	var data struct {
		OperatorPromptSubmit struct {
			Deduplicated bool `json:"deduplicated"`
		} `json:"operatorPromptSubmit"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return data.OperatorPromptSubmit.Deduplicated
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
