package graphql_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// recordingWaker captures every WakeProject(projectRoot) call. Used to
// assert that operatorPromptApprove and operatorPromptSubmit (auto-approve)
// signal the local execute-loop coordinator, satisfying the wake-on-approve
// AC of S15-5.
type recordingWaker struct {
	mu    sync.Mutex
	calls []string
	count int32
}

func (w *recordingWaker) WakeProject(projectRoot string) int {
	w.mu.Lock()
	w.calls = append(w.calls, projectRoot)
	w.mu.Unlock()
	atomic.AddInt32(&w.count, 1)
	return 1
}

func (w *recordingWaker) Calls() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.calls...)
}

func setupWakeHarness(t *testing.T, allowlist []string, waker ddxgraphql.ExecuteLoopWaker) (http.Handler, *bead.Store, string, string) {
	t.Helper()
	workDir, err := os.MkdirTemp("", "operator-prompt-wake-")
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
			ExecuteLoopWaker:                   waker,
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gql.AddTransport(transport.POST{})

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(ddxgraphql.WithHTTPRequest(r.Context(), r))
		gql.ServeHTTP(w, r)
	})
	return wrapped, store, token, workDir
}

// AC: "Execute-loop wakes on approve event without waiting for next tick".
// The resolver must call WakeProject after a successful manual approve so
// the local execute-loop drops its idle-poll sleep and re-scans the queue.
func TestOperatorPromptApprove_SignalsWake(t *testing.T) {
	waker := &recordingWaker{}
	h, _, token, workDir := setupWakeHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, waker)

	id, _ := submitProposed(t, h, token, "wake me", "k1", nil)
	if got := atomic.LoadInt32(&waker.count); got != 0 {
		t.Fatalf("submit without auto-approve must not wake; got %d wakes", got)
	}

	resp := postGQL(t, h, token, approveMutation, map[string]any{"id": id})
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("approve returned errors: %s", errs)
	}
	calls := waker.Calls()
	if len(calls) != 1 {
		t.Fatalf("approve must signal exactly one wake; got %v", calls)
	}
	if calls[0] != workDir {
		t.Errorf("wake project root: want %q, got %q", workDir, calls[0])
	}
}

// AC: auto-approve on submit must also wake the loop in the same tick.
func TestOperatorPromptSubmit_AutoApprove_SignalsWake(t *testing.T) {
	waker := &recordingWaker{}
	h, _, token, workDir := setupWakeHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, waker)

	autoApprove := true
	id, raw := submitProposed(t, h, token, "auto wake", "k1", &autoApprove)
	if id == "" {
		t.Fatal("submit failed")
	}
	var data struct {
		OperatorPromptSubmit struct {
			AutoApproved bool `json:"autoApproved"`
		} `json:"operatorPromptSubmit"`
	}
	if err := json.Unmarshal(raw["data"], &data); err != nil {
		t.Fatal(err)
	}
	if !data.OperatorPromptSubmit.AutoApproved {
		t.Fatal("expected auto-approve to fire")
	}
	calls := waker.Calls()
	if len(calls) != 1 {
		t.Fatalf("auto-approve must signal exactly one wake; got %v", calls)
	}
	if calls[0] != workDir {
		t.Errorf("wake project root: want %q, got %q", workDir, calls[0])
	}
}

// AC: cancel must NOT wake the loop (no work to claim).
func TestOperatorPromptCancel_DoesNotSignalWake(t *testing.T) {
	waker := &recordingWaker{}
	h, _, token, _ := setupWakeHarness(t, nil, waker)

	id, _ := submitProposed(t, h, token, "cancel", "k1", nil)
	resp := postGQL(t, h, token, cancelMutation, map[string]any{"id": id})
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("cancel returned errors: %s", errs)
	}
	if got := atomic.LoadInt32(&waker.count); got != 0 {
		t.Errorf("cancel must not wake; got %d wakes", got)
	}
}

// Sanity: if the resolver has no waker wired (test-time configuration without
// a WorkerManager), approve still succeeds. Verifies the nil-guard.
func TestOperatorPromptApprove_NilWakerNoCrash(t *testing.T) {
	h, _, token, _ := setupWakeHarness(t, []string{ddxgraphql.OperatorPromptAllowlistLocalhostSentinel}, nil)
	id, _ := submitProposed(t, h, token, "no waker", "k1", nil)
	resp := postGQL(t, h, token, approveMutation, map[string]any{"id": id})
	if errs, ok := resp["errors"]; ok {
		t.Fatalf("approve returned errors: %s", errs)
	}
	w := httptest.NewRecorder()
	_ = w
}
