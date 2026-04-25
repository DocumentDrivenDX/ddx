package server

// server_inline_prompt_oversize_test.go covers FEAT-022 §9 / Stage D2:
// inline prompt bodies posted to /api/agent/run must hard-fail with HTTP
// 413 carrying { error, observed_bytes, cap_bytes, config_hint } when they
// exceed the configured cap.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerInlinePromptOversize(t *testing.T) {
	const testCap = 1024
	prev := serverPromptCapBytes
	serverPromptCapBytes = testCap
	t.Cleanup(func() { serverPromptCapBytes = prev })

	dir := setupTestDir(t)
	srv := New(":0", dir)

	huge := strings.Repeat("x", testCap*2)
	payload, err := json.Marshal(map[string]string{
		"harness": "claude",
		"prompt":  huge,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/run", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d (body=%s)", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, key := range []string{"error", "observed_bytes", "cap_bytes", "config_hint"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response body missing key %q: %v", key, body)
		}
	}
	if observed, ok := body["observed_bytes"].(float64); !ok || int(observed) != testCap*2 {
		t.Errorf("observed_bytes = %v, want %d", body["observed_bytes"], testCap*2)
	}
	if cap, ok := body["cap_bytes"].(float64); !ok || int(cap) != testCap {
		t.Errorf("cap_bytes = %v, want %d", body["cap_bytes"], testCap)
	}
	if hint, ok := body["config_hint"].(string); !ok || !strings.Contains(hint, "evidence_caps.max_prompt_bytes") {
		t.Errorf("config_hint missing config key hint: %v", body["config_hint"])
	}
}
