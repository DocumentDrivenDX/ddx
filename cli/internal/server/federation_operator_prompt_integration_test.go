package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

const operatorPromptSubmitMutation = `mutation Submit($input: OperatorPromptSubmitInput!) {
	operatorPromptSubmit(input: $input) {
		bead {
			id
			status
			issueType
		}
		deduplicated
		autoApproved
	}
}`

type requestCapture struct {
	mu      sync.Mutex
	method  string
	path    string
	headers http.Header
	body    []byte
}

func (c *requestCapture) record(r *http.Request) {
	copiedBody, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(copiedBody))

	c.mu.Lock()
	defer c.mu.Unlock()
	c.method = r.Method
	c.path = r.URL.Path
	c.headers = r.Header.Clone()
	c.body = append([]byte(nil), copiedBody...)
}

func (c *requestCapture) snapshot() (method, path string, headers http.Header, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.method, c.path, c.headers.Clone(), append([]byte(nil), c.body...)
}

func setServerNodeIdentity(t *testing.T, s *Server, name, id string) {
	t.Helper()
	s.state.Node.Name = name
	s.state.Node.ID = id
}

func newPromptSpokeServer(t *testing.T, workDir, name, id, csrfToken string) (*Server, *httptest.Server, *requestCapture) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	s := New(":0", workDir)
	setServerNodeIdentity(t, s, name, id)
	s.csrfTokens = ddxgraphql.NewStaticCSRFTokenStoreWithToken(csrfToken)

	capture := &requestCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/graphql" {
			capture.record(r)
		}
		s.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return s, srv, capture
}

func postJSON(t *testing.T, url string, headers map[string]string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func assertStatusAndErrorContains(t *testing.T, resp *http.Response, want int, substr string) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read error body: %v", err)
	}
	if !strings.Contains(string(body), substr) {
		t.Fatalf("error body = %s, want substring %q", string(body), substr)
	}
}

func TestFederation_OperatorPromptSubmit_CoordinatorToSpoke_IdentityUnchanged(t *testing.T) {
	hubDir := setupTestDir(t)
	hub := New(":0", hubDir)
	setServerIdentity(t, hub, "coord-456")
	hub.EnableHubMode(false)
	hubSrv := httptest.NewServer(hub.Handler())
	t.Cleanup(hubSrv.Close)

	spokeDir := setupTestDir(t)
	spokeName := "coord-456"
	spokeID := "spoke-123"
	spoke, spokeSrv, capture := newPromptSpokeServer(t, spokeDir, spokeName, spokeID, "shared-csrf-token")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() {
		_ = spoke.ShutdownSpoke(context.Background())
	})
	if err := spoke.EnableSpokeMode(ctx, hubSrv.URL, spokeSrv.URL,
		WithSpokeStatePath(filepath.Join(t.TempDir(), "spoke-state.json")),
		WithSpokeHeartbeatInterval(time.Hour),
	); err != nil {
		t.Fatalf("EnableSpokeMode: %v", err)
	}

	projID := projectID(spokeDir)
	requestBody := map[string]any{
		"query": operatorPromptSubmitMutation,
		"variables": map[string]any{
			"input": map[string]any{
				"prompt":         "Investigate the federation forwarding path.",
				"idempotencyKey": "federation-forward-1",
			},
		},
	}

	resp := postJSON(t, hubSrv.URL+"/api/federation/projects/"+projID+"/graphql",
		map[string]string{
			"X-CSRF-Token":     "shared-csrf-token",
			"X-Tailscale-Node": "coord-456",
			"X-Request-Id":     "req-forward-1",
		},
		requestBody,
	)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("forwarded mutation status = %d, body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		Data struct {
			OperatorPromptSubmit struct {
				Bead struct {
					ID        string `json:"id"`
					Status    string `json:"status"`
					IssueType string `json:"issueType"`
				} `json:"bead"`
				Deduplicated bool `json:"deduplicated"`
				AutoApproved bool `json:"autoApproved"`
			} `json:"operatorPromptSubmit"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode forwarded response: %v", err)
	}
	if len(out.Errors) > 0 {
		t.Fatalf("forwarded mutation returned errors: %+v", out.Errors)
	}
	if out.Data.OperatorPromptSubmit.Bead.ID == "" {
		t.Fatal("expected bead ID in forwarded mutation result")
	}
	if out.Data.OperatorPromptSubmit.Bead.Status != bead.StatusProposed {
		t.Fatalf("bead status = %q, want %q", out.Data.OperatorPromptSubmit.Bead.Status, bead.StatusProposed)
	}
	if out.Data.OperatorPromptSubmit.Bead.IssueType != bead.IssueTypeOperatorPrompt {
		t.Fatalf("issue type = %q, want %q", out.Data.OperatorPromptSubmit.Bead.IssueType, bead.IssueTypeOperatorPrompt)
	}
	if out.Data.OperatorPromptSubmit.Deduplicated {
		t.Fatal("first forwarded submission must not be deduplicated")
	}

	spokeStore := bead.NewStore(filepath.Join(spokeDir, ddxroot.DirName))
	persisted, err := spokeStore.Get(context.Background(), out.Data.OperatorPromptSubmit.Bead.ID)
	if err != nil {
		t.Fatalf("read spoke bead: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected bead to exist on spoke")
	}
	events, err := spokeStore.EventsByKind(persisted.ID, ddxgraphql.OperatorPromptEventKind)
	if err != nil {
		t.Fatalf("read spoke audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 operator_prompt_submit event, got %d", len(events))
	}
	if !strings.Contains(events[0].Body, "origin_node_id=coord-456") {
		t.Fatalf("audit body missing origin identity: %s", events[0].Body)
	}
	if !strings.Contains(events[0].Body, "receiving_node_id=spoke-123") {
		t.Fatalf("audit body missing receiving node id: %s", events[0].Body)
	}

	method, path, headers, body := capture.snapshot()
	if method != http.MethodPost || path != "/graphql" {
		t.Fatalf("captured request = %s %s, want POST /graphql", method, path)
	}
	if got := headers.Get("X-DDx-Origin-Identity"); got != "coord-456" {
		t.Fatalf("origin header = %q, want %q", got, "coord-456")
	}
	if got := headers.Get("X-DDx-Coordinator-Identity"); got != "coord-456" {
		t.Fatalf("coordinator header = %q, want %q", got, "coord-456")
	}
	if got := headers.Get("X-Tailscale-Node"); got != "coord-456" {
		t.Fatalf("direct peer header = %q, want %q", got, "coord-456")
	}
	if !bytes.Contains(body, []byte(`operatorPromptSubmit`)) {
		t.Fatalf("forwarded body missing mutation name: %s", string(body))
	}
}

func TestFederation_OperatorPromptSubmit_UntrustedCoordinator(t *testing.T) {
	spokeDir := setupTestDir(t)
	spoke, spokeSrv, _ := newPromptSpokeServer(t, spokeDir, "coord-456", "spoke-123", "shared-csrf-token")
	t.Cleanup(func() {
		_ = spoke.ShutdownSpoke(context.Background())
	})

	resp := postJSON(t, spokeSrv.URL+"/graphql",
		map[string]string{
			"X-CSRF-Token":               "shared-csrf-token",
			"X-Tailscale-Node":           "rogue-coord",
			"X-DDx-Origin-Identity":      "coord-456",
			"X-DDx-Coordinator-Identity": "rogue-coord",
		},
		map[string]any{
			"query": operatorPromptSubmitMutation,
			"variables": map[string]any{
				"input": map[string]any{
					"prompt":         "Should fail.",
					"idempotencyKey": "untrusted-coord",
				},
			},
		},
	)
	assertStatusAndErrorContains(t, resp, http.StatusForbidden, "untrusted coordinator identity")
}

func TestFederation_OperatorPromptSubmit_MissingOriginHeader(t *testing.T) {
	spokeDir := setupTestDir(t)
	spoke, spokeSrv, _ := newPromptSpokeServer(t, spokeDir, "coord-456", "spoke-123", "shared-csrf-token")
	t.Cleanup(func() {
		_ = spoke.ShutdownSpoke(context.Background())
	})

	resp := postJSON(t, spokeSrv.URL+"/graphql",
		map[string]string{
			"X-CSRF-Token":               "shared-csrf-token",
			"X-Tailscale-Node":           "coord-456",
			"X-DDx-Coordinator-Identity": "coord-456",
		},
		map[string]any{
			"query": operatorPromptSubmitMutation,
			"variables": map[string]any{
				"input": map[string]any{
					"prompt":         "Should fail.",
					"idempotencyKey": "missing-origin",
				},
			},
		},
	)
	assertStatusAndErrorContains(t, resp, http.StatusForbidden, "forwarded mutation requires origin and coordinator attestation")
}

func TestFederation_OperatorPromptSubmit_TamperedOriginHeader(t *testing.T) {
	spokeDir := setupTestDir(t)
	spoke, spokeSrv, _ := newPromptSpokeServer(t, spokeDir, "coord-456", "spoke-123", "shared-csrf-token")
	t.Cleanup(func() {
		_ = spoke.ShutdownSpoke(context.Background())
	})

	resp := postJSON(t, spokeSrv.URL+"/graphql",
		map[string]string{
			"X-CSRF-Token":               "shared-csrf-token",
			"X-Tailscale-Node":           "coord-456",
			"X-DDx-Origin-Identity":      "tampered-origin",
			"X-DDx-Coordinator-Identity": "coord-456",
		},
		map[string]any{
			"query": operatorPromptSubmitMutation,
			"variables": map[string]any{
				"input": map[string]any{
					"prompt":         "Should fail.",
					"idempotencyKey": "tampered-origin",
				},
			},
		},
	)
	assertStatusAndErrorContains(t, resp, http.StatusForbidden, "untrusted origin identity")
}
