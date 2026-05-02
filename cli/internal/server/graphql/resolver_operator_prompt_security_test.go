package graphql_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// setupSecurityHarness mirrors setupOperatorPromptHarness but lets the test
// configure the prompt cap, build SHA, and node ID so the cap-error envelope
// and audit-field assertions can pin exact values.
func setupSecurityHarness(t *testing.T, capBytes int) (http.Handler, *bead.Store, string) {
	t.Helper()

	workDir, err := os.MkdirTemp("", "operator-prompt-security-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	ddxDir := filepath.Join(workDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"sec\"\n"
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
			PromptCapBytes:            capBytes,
			BuildSHA:                  "test-build-sha-cafebabe",
			NodeID:                    "test-node-id-001",
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gql.AddTransport(transport.POST{})

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(ddxgraphql.WithHTTPRequest(r.Context(), r))
		gql.ServeHTTP(w, r)
	})
	return wrapped, store, token
}

// postSubmit posts an OperatorPromptSubmit mutation via raw GraphQL using
// variables (the production transport path). headers are applied verbatim.
func postSubmit(t *testing.T, h http.Handler, headers map[string]string, prompt, idemKey string) (int, map[string]json.RawMessage) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"query": operatorPromptMutation,
		"variables": map[string]any{
			"input": map[string]any{
				"prompt":         prompt,
				"idempotencyKey": idemKey,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	return w.Code, resp
}

// AC: oversize returns 400 with cap-error envelope (extensions carry
// observed_bytes, cap_bytes, code=PROMPT_TOO_LARGE).
func TestOperatorPromptSubmit_OversizeViaGraphQLVariable(t *testing.T) {
	const cap = 1024
	h, store, token := setupSecurityHarness(t, cap)

	oversize := strings.Repeat("a", cap+1)
	_, resp := postSubmit(t, h, map[string]string{ddxgraphql.CSRFHeaderName: token}, oversize, "oversize-1")

	ext := mustCapErrorExtensions(t, resp)
	if got := intField(ext, "observed_bytes"); got != cap+1 {
		t.Errorf("observed_bytes: got %d, want %d", got, cap+1)
	}
	if got := intField(ext, "cap_bytes"); got != cap {
		t.Errorf("cap_bytes: got %d, want %d", got, cap)
	}
	if code, _ := ext["code"].(string); code != "PROMPT_TOO_LARGE" {
		t.Errorf("code: got %q, want PROMPT_TOO_LARGE", code)
	}
	if status, _ := ext["status"].(float64); int(status) != 400 {
		t.Errorf("status: got %v, want 400", ext["status"])
	}
	// No bead must be created on rejection.
	beads, err := store.List("", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(beads) != 0 {
		t.Errorf("oversize must not create a bead, got %d", len(beads))
	}
}

// AC: multibyte oversize — a prompt whose rune count is small but whose
// byte count exceeds the cap must still be rejected (cap is in bytes).
func TestOperatorPromptSubmit_OversizeMultibyte(t *testing.T) {
	const cap = 100
	h, _, token := setupSecurityHarness(t, cap)

	// "🔥" = 4 bytes per rune. 30 runes = 120 bytes > 100 cap; rune count
	// alone (30) would slip past a naive len(rune-slice) check.
	prompt := strings.Repeat("🔥", 30)
	if len(prompt) <= cap {
		t.Fatalf("test setup wrong: byte len %d should exceed cap %d", len(prompt), cap)
	}

	_, resp := postSubmit(t, h, map[string]string{ddxgraphql.CSRFHeaderName: token}, prompt, "multibyte-1")
	ext := mustCapErrorExtensions(t, resp)
	if got := intField(ext, "observed_bytes"); got != len(prompt) {
		t.Errorf("observed_bytes (multibyte): got %d, want %d", got, len(prompt))
	}
}

// AC: just-under-cap accepted.
func TestOperatorPromptSubmit_AtCapAccepted(t *testing.T) {
	const cap = 64
	h, _, token := setupSecurityHarness(t, cap)

	prompt := strings.Repeat("z", cap)
	_, resp := postSubmit(t, h, map[string]string{ddxgraphql.CSRFHeaderName: token}, prompt, "atcap-1")
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("at-cap submission must succeed; got errors: %s", errs)
	}
}

// AC: cross-origin browser POST rejected — when Origin header is present
// and its host portion does not match the request Host, the resolver
// returns the cross-origin error before any bead is created.
func TestOperatorPromptSubmit_CrossOriginRejected(t *testing.T) {
	h, store, token := setupSecurityHarness(t, 0)

	headers := map[string]string{
		ddxgraphql.CSRFHeaderName: token,
		"Origin":                  "https://evil.example",
	}
	_, resp := postSubmit(t, h, headers, "do a thing", "xorig-1")

	errs, ok := resp["errors"]
	if !ok {
		t.Fatal("expected cross-origin error")
	}
	var parsed []struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(errs, &parsed); err != nil {
		t.Fatalf("parse errors: %v", err)
	}
	if len(parsed) == 0 || !strings.Contains(parsed[0].Message, "cross-origin") {
		t.Errorf("expected cross-origin error message, got %v", parsed)
	}

	beads, _ := store.List("", "", nil)
	if len(beads) != 0 {
		t.Errorf("cross-origin must not create a bead, got %d", len(beads))
	}
}

// AC: same-origin POST accepted (Origin matches Host).
func TestOperatorPromptSubmit_SameOriginAccepted(t *testing.T) {
	h, _, token := setupSecurityHarness(t, 0)

	// httptest.NewRequest defaults Host to "example.com" for absolute path
	// targets; pair with an Origin scheme://example.com to satisfy the
	// same-origin check.
	headers := map[string]string{
		ddxgraphql.CSRFHeaderName: token,
		"Origin":                  "https://example.com",
	}
	_, resp := postSubmit(t, h, headers, "same-origin prompt", "same-orig-1")
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("same-origin submission must succeed; got errors: %s", errs)
	}
}

// AC: first bead event records the structured audit fields enumerated in
// the bead description (peer identity, origin node ID, build SHA,
// approval-mode, request ID, prompt SHA-256).
func TestOperatorPromptSubmit_AuditStructuredFields(t *testing.T) {
	h, store, token := setupSecurityHarness(t, 0)

	headers := map[string]string{
		ddxgraphql.CSRFHeaderName: token,
		"X-Request-Id":            "req-test-12345",
	}
	_, resp := postSubmit(t, h, headers, "audit prompt body", "audit-1")
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
	id := data.OperatorPromptSubmit.Bead.ID

	events, err := store.EventsByKind(id, ddxgraphql.OperatorPromptEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(events))
	}
	body := events[0].Body
	// Each enumerated audit field must appear in the immutable first event.
	required := []string{
		"identity=",          // peer identity classification
		"origin_node_id=",    // origin node ID
		"receiving_node_id=", // receiving node ID (paired)
		"build_sha=test-build-sha-cafebabe",
		"approval_mode=manual", // input.AutoApprove was nil
		"request_id=req-test-12345",
		// prompt SHA-256 of "audit prompt body"
		"prompt_sha256=78c5f7eb86b48be7d061b2f9d4a8e8a45d5ad8e10d1f04c92cb89a18a7b0db77",
		"idempotency_key=audit-1",
	}
	// Compute the expected SHA dynamically rather than hardcoding (literal
	// above was illustrative; recompute here for resilience).
	required[len(required)-2] = "prompt_sha256=" + sha256Hex("audit prompt body")
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("audit body missing %q\nbody: %s", want, body)
		}
	}
}

// AC: when AutoApprove=true is requested, the audit event records
// approval_mode=auto-requested (independent of whether the allowlist
// actually grants auto-approve to this identity).
func TestOperatorPromptSubmit_AuditApprovalMode(t *testing.T) {
	h, store, token := setupSecurityHarness(t, 0)

	body, _ := json.Marshal(map[string]any{
		"query": operatorPromptMutation,
		"variables": map[string]any{
			"input": map[string]any{
				"prompt":         "auto-approve prompt",
				"idempotencyKey": "approval-mode-1",
				"autoApprove":    true,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ddxgraphql.CSRFHeaderName, token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
		t.Fatal(err)
	}
	events, err := store.EventsByKind(data.OperatorPromptSubmit.Bead.ID, ddxgraphql.OperatorPromptEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(events))
	}
	if !strings.Contains(events[0].Body, "approval_mode=auto-requested") {
		t.Errorf("approval_mode=auto-requested missing from audit body: %s", events[0].Body)
	}
}

// ─── helpers ───

func mustCapErrorExtensions(t *testing.T, resp map[string]json.RawMessage) map[string]any {
	t.Helper()
	errs, ok := resp["errors"]
	if !ok {
		t.Fatalf("expected GraphQL errors for cap rejection; resp=%v", resp)
	}
	var parsed []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	}
	if err := json.Unmarshal(errs, &parsed); err != nil {
		t.Fatalf("parse errors: %v", err)
	}
	if len(parsed) == 0 {
		t.Fatal("no errors returned")
	}
	if parsed[0].Extensions == nil {
		t.Fatalf("error has no extensions; message=%s", parsed[0].Message)
	}
	return parsed[0].Extensions
}

func intField(m map[string]any, k string) int {
	v, ok := m[k]
	if !ok {
		return -1
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return -1
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
