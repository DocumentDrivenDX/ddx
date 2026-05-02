package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// setupApprovalHarness wires a gqlgen handler with the same CSRF / identity
// plumbing as the production server, plus a configurable auto-approve
// allowlist. The tsnetUser argument, when non-empty, is injected as the
// X-Tailscale-User header on every request — that is how production
// classifies a request as ts-net-originated, and the locked Story 15
// decision excludes such requests from auto-approve regardless of the
// allowlist contents.
func setupApprovalHarness(t *testing.T, allowlist []string, tsnetUser string) (http.Handler, *bead.Store, string) {
	t.Helper()

	workDir, err := os.MkdirTemp("", "operator-prompt-approval-")
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
			WorkingDir:                         workDir,
			CSRFTokens:                         csrf,
			OperatorPromptIdempotency:          cache,
			OperatorPromptAutoApproveAllowlist: append([]string(nil), allowlist...),
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gql.AddTransport(transport.POST{})
	gql.AddTransport(transport.GET{})

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tsnetUser != "" {
			r.Header.Set("X-Tailscale-User", tsnetUser)
		}
		r = r.WithContext(ddxgraphql.WithHTTPRequest(r.Context(), r))
		gql.ServeHTTP(w, r)
	})
	return wrapped, store, token
}

func postGQL(t *testing.T, h http.Handler, csrfToken string, query string, vars map[string]any) map[string]json.RawMessage {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"query":     query,
		"variables": vars,
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrfToken != "" {
		req.Header.Set(ddxgraphql.CSRFHeaderName, csrfToken)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

const submitMutation = `mutation Submit($input: OperatorPromptSubmitInput!) {
	operatorPromptSubmit(input: $input) {
		bead { id status }
		deduplicated
		autoApproved
	}
}`

const approveMutation = `mutation Approve($id: ID!) {
	operatorPromptApprove(id: $id) {
		bead { id status }
	}
}`

const cancelMutation = `mutation Cancel($id: ID!) {
	operatorPromptCancel(id: $id) {
		bead { id status }
	}
}`

func submitProposed(t *testing.T, h http.Handler, token, prompt, key string, autoApprove *bool) (string, map[string]json.RawMessage) {
	t.Helper()
	input := map[string]any{
		"prompt":         prompt,
		"idempotencyKey": key,
	}
	if autoApprove != nil {
		input["autoApprove"] = *autoApprove
	}
	resp := postGQL(t, h, token, submitMutation, map[string]any{"input": input})
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("submit returned errors: %s", errs)
	}
	var data struct {
		OperatorPromptSubmit struct {
			Bead struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"bead"`
			Deduplicated bool `json:"deduplicated"`
			AutoApproved bool `json:"autoApproved"`
		} `json:"operatorPromptSubmit"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if data.OperatorPromptSubmit.Bead.ID == "" {
		t.Fatal("expected bead ID")
	}
	return data.OperatorPromptSubmit.Bead.ID, resp
}

// AC #1 (mutations exist) + AC #4 happy: manual approve transitions
// proposed → open and emits the operator_prompt_approve audit event.
func TestOperatorPromptApprove_Happy(t *testing.T) {
	h, store, token := setupApprovalHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, "")
	id, _ := submitProposed(t, h, token, "approve me", "k1", nil)

	resp := postGQL(t, h, token, approveMutation, map[string]any{"id": id})
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("approve returned errors: %s", errs)
	}
	var data struct {
		OperatorPromptApprove struct {
			Bead struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"bead"`
		} `json:"operatorPromptApprove"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := data.OperatorPromptApprove.Bead.Status; got != bead.StatusOpen {
		t.Errorf("status after approve: want %q, got %q", bead.StatusOpen, got)
	}
	events, err := store.EventsByKind(id, ddxgraphql.OperatorPromptApproveEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("want 1 approve audit event, got %d", len(events))
	}
}

// AC #1 + AC #4 happy: manual cancel transitions proposed → cancelled and
// emits the operator_prompt_cancel audit event. Cancellation is open to
// any trusted identity (the allowlist gates approval, not rejection).
func TestOperatorPromptCancel_Happy(t *testing.T) {
	h, store, token := setupApprovalHarness(t, nil, "")
	id, _ := submitProposed(t, h, token, "cancel me", "k1", nil)

	resp := postGQL(t, h, token, cancelMutation, map[string]any{"id": id})
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("cancel returned errors: %s", errs)
	}
	var data struct {
		OperatorPromptCancel struct {
			Bead struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"bead"`
		} `json:"operatorPromptCancel"`
	}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := data.OperatorPromptCancel.Bead.Status; got != bead.StatusCancelled {
		t.Errorf("status after cancel: want %q, got %q", bead.StatusCancelled, got)
	}
	events, err := store.EventsByKind(id, ddxgraphql.OperatorPromptCancelEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("want 1 cancel audit event, got %d", len(events))
	}
}

// AC #2 denied: an empty allowlist blocks operatorPromptApprove even for
// the localhost identity that submitted the prompt. The bead remains in
// the proposed status and no approve audit event is appended.
func TestOperatorPromptApprove_DeniedEmptyAllowlist(t *testing.T) {
	h, store, token := setupApprovalHarness(t, nil, "")
	id, _ := submitProposed(t, h, token, "needs approval", "k1", nil)

	resp := postGQL(t, h, token, approveMutation, map[string]any{"id": id})
	errs, ok := resp["errors"]
	if !ok {
		t.Fatalf("expected approve to be denied, got data %s", resp["data"])
	}
	var parsed []struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(errs, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed) == 0 || !contains(parsed[0].Message, "allowlist") {
		t.Errorf("denial must reference the allowlist, got %q", parsed)
	}
	persisted, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != bead.StatusProposed {
		t.Errorf("bead must remain proposed on denial, got %q", persisted.Status)
	}
	events, _ := store.EventsByKind(id, ddxgraphql.OperatorPromptApproveEventKind)
	if len(events) != 0 {
		t.Errorf("want 0 approve audit events on denial, got %d", len(events))
	}
}

// AC #3 denied: ts-net identities are NEVER eligible for auto-approve, even
// when the allowlist contains the localhost sentinel. The submission
// succeeds (bead is created in proposed) but autoApproved=false.
func TestOperatorPromptSubmit_AutoApproveDeniedForTsnet(t *testing.T) {
	// Allowlist contains both the localhost sentinel (so localhost may
	// auto-approve) AND the ts-net actor (so the ts-net peer is permitted
	// to submit at all). The locked Story 15 decision still denies the
	// auto-approve transition for ts-net regardless of allowlist contents.
	h, store, token := setupApprovalHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel, "alice@example.com"}, "alice@example.com")

	autoApprove := true
	id, raw := submitProposed(t, h, token, "do a ts-net thing", "k1", &autoApprove)

	var data struct {
		OperatorPromptSubmit struct {
			AutoApproved bool `json:"autoApproved"`
			Bead         struct {
				Status string `json:"status"`
			} `json:"bead"`
		} `json:"operatorPromptSubmit"`
	}
	if err := json.Unmarshal(raw["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.OperatorPromptSubmit.AutoApproved {
		t.Error("ts-net identity must NEVER be auto-approved (locked decision)")
	}
	if data.OperatorPromptSubmit.Bead.Status != bead.StatusProposed {
		t.Errorf("ts-net submission must remain proposed, got %q", data.OperatorPromptSubmit.Bead.Status)
	}
	events, _ := store.EventsByKind(id, ddxgraphql.OperatorPromptApproveEventKind)
	if len(events) != 0 {
		t.Errorf("auto-approve audit event must NOT be appended for ts-net, got %d", len(events))
	}
}

// AC #3 happy + AC #2: a localhost identity on the allowlist auto-approves
// on submit (proposed → open) and emits the operator_prompt_approve audit
// event in addition to the original submit event.
func TestOperatorPromptSubmit_AutoApproveAllowedForLocalhost(t *testing.T) {
	h, store, token := setupApprovalHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, "")

	autoApprove := true
	id, raw := submitProposed(t, h, token, "auto-approved thing", "k1", &autoApprove)

	var data struct {
		OperatorPromptSubmit struct {
			AutoApproved bool `json:"autoApproved"`
			Bead         struct {
				Status string `json:"status"`
			} `json:"bead"`
		} `json:"operatorPromptSubmit"`
	}
	if err := json.Unmarshal(raw["data"], &data); err != nil {
		t.Fatal(err)
	}
	if !data.OperatorPromptSubmit.AutoApproved {
		t.Error("localhost identity on allowlist must be auto-approved")
	}
	if data.OperatorPromptSubmit.Bead.Status != bead.StatusOpen {
		t.Errorf("auto-approved bead must be open, got %q", data.OperatorPromptSubmit.Bead.Status)
	}
	events, err := store.EventsByKind(id, ddxgraphql.OperatorPromptApproveEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("want 1 auto-approve audit event, got %d", len(events))
	}
}

// AC #2 + AC #3: CSRF still gates approve and cancel.
func TestOperatorPromptApprove_CSRFRequired(t *testing.T) {
	h, _, token := setupApprovalHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, "")
	id, _ := submitProposed(t, h, token, "csrf-approve", "k1", nil)

	resp := postGQL(t, h, "", approveMutation, map[string]any{"id": id})
	errs, ok := resp["errors"]
	if !ok {
		t.Fatal("approve must require CSRF token")
	}
	var parsed []struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(errs, &parsed)
	if len(parsed) == 0 || !contains(parsed[0].Message, "CSRF") || !contains(parsed[0].Message, "403") {
		t.Errorf("approve CSRF error must mention CSRF and 403, got %q", parsed)
	}
}

func TestOperatorPromptCancel_CSRFRequired(t *testing.T) {
	h, _, token := setupApprovalHarness(t, nil, "")
	id, _ := submitProposed(t, h, token, "csrf-cancel", "k1", nil)

	resp := postGQL(t, h, "", cancelMutation, map[string]any{"id": id})
	errs, ok := resp["errors"]
	if !ok {
		t.Fatal("cancel must require CSRF token")
	}
	var parsed []struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(errs, &parsed)
	if len(parsed) == 0 || !contains(parsed[0].Message, "CSRF") || !contains(parsed[0].Message, "403") {
		t.Errorf("cancel CSRF error must mention CSRF and 403, got %q", parsed)
	}
}

// AC #1: approve / cancel reject beads that are not in the proposed status
// (e.g. an already-cancelled bead cannot be re-approved). Ensures the
// state machine cannot be backed up via these mutations.
func TestOperatorPromptApprove_RejectsNonProposed(t *testing.T) {
	h, _, token := setupApprovalHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, "")
	id, _ := submitProposed(t, h, token, "double-action", "k1", nil)

	// Cancel first so it leaves the proposed status.
	if resp := postGQL(t, h, token, cancelMutation, map[string]any{"id": id}); resp["errors"] != nil {
		t.Fatalf("setup cancel errored: %s", resp["errors"])
	}

	resp := postGQL(t, h, token, approveMutation, map[string]any{"id": id})
	if _, ok := resp["errors"]; !ok {
		t.Fatal("approve on cancelled bead must error")
	}
}

// AC #4: per-project allowlist enforced — a ts-net peer not on the
// allow_identities allowlist receives a 403 on operatorPromptSubmit and no
// bead is created. This is the "default empty allowlist → ts-net peers see
// read-only UI" guarantee enforced server-side.
func TestOperatorPromptSubmit_TsnetNotInAllowlistRejected(t *testing.T) {
	h, _, token := setupApprovalHarness(t, nil, "bob@example.com")

	resp := postGQL(t, h, token, submitMutation, map[string]any{
		"input": map[string]any{
			"prompt":         "from a ts-net peer",
			"idempotencyKey": "k1",
		},
	})
	errs, ok := resp["errors"]
	if !ok {
		t.Fatalf("expected ts-net submit to be denied, got data %s", resp["data"])
	}
	var parsed []struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(errs, &parsed)
	if len(parsed) == 0 || !contains(parsed[0].Message, "allow_identities") || !contains(parsed[0].Message, "403") {
		t.Errorf("denial must reference allow_identities and 403, got %q", parsed)
	}
}

// AC #4: a ts-net peer that IS on the per-project allowlist may submit
// (still proposed; auto-approve remains localhost-only).
func TestOperatorPromptSubmit_TsnetOnAllowlistAccepted(t *testing.T) {
	h, _, token := setupApprovalHarness(t, []string{"carol@example.com"}, "carol@example.com")

	id, _ := submitProposed(t, h, token, "from a permitted ts-net peer", "k1", nil)
	if id == "" {
		t.Fatal("expected submit to succeed when ts-net actor is on allowlist")
	}
}

func TestOperatorPromptCancel_RejectsNonProposed(t *testing.T) {
	h, _, token := setupApprovalHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, "")
	id, _ := submitProposed(t, h, token, "double-action", "k1", nil)

	if resp := postGQL(t, h, token, approveMutation, map[string]any{"id": id}); resp["errors"] != nil {
		t.Fatalf("setup approve errored: %s", resp["errors"])
	}

	resp := postGQL(t, h, token, cancelMutation, map[string]any{"id": id})
	if _, ok := resp["errors"]; !ok {
		t.Fatal("cancel on already-open bead must error")
	}
}
