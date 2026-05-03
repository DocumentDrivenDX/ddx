package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestRunsRedirectSessionsAndExecutions verifies Story 8 cleanup: legacy
// /sessions and /executions routes return 302 with a Sunset header, pointing
// to the layer-aware /runs page.
func TestRunsRedirectSessionsAndExecutions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	srv := New(":0", dir)

	cases := []struct {
		name         string
		path         string
		wantPathPart string // pathname or pathname-prefix the Location must start with
		wantLayer    string // expected ?layer= value, "" if N/A
		wantHarness  string // expected ?harness= value, "" if N/A
	}{
		{
			name:         "sessions redirects to runs?layer=run",
			path:         "/nodes/n1/projects/p1/sessions",
			wantPathPart: "/nodes/n1/projects/p1/runs",
			wantLayer:    "run",
		},
		{
			name:         "executions redirects to runs?layer=try",
			path:         "/nodes/n1/projects/p1/executions",
			wantPathPart: "/nodes/n1/projects/p1/runs",
			wantLayer:    "try",
		},
		{
			name:         "executions preserves harness query param",
			path:         "/nodes/n1/projects/p1/executions?harness=codex",
			wantPathPart: "/nodes/n1/projects/p1/runs",
			wantLayer:    "try",
			wantHarness:  "codex",
		},
		{
			name:         "executions detail redirects to runs/<id>",
			path:         "/nodes/n1/projects/p1/executions/exec-abc123",
			wantPathPart: "/nodes/n1/projects/p1/runs/exec-abc123",
		},
		{
			name:         "executions detail prepends exec- when missing",
			path:         "/nodes/n1/projects/p1/executions/abc123",
			wantPathPart: "/nodes/n1/projects/p1/runs/exec-abc123",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusFound {
				t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
			}
			loc := w.Header().Get("Location")
			if loc == "" {
				t.Fatalf("missing Location header")
			}
			parsed, err := url.Parse(loc)
			if err != nil {
				t.Fatalf("invalid Location URL %q: %v", loc, err)
			}
			if !strings.HasPrefix(parsed.Path, tc.wantPathPart) {
				t.Fatalf("Location path %q does not start with %q", parsed.Path, tc.wantPathPart)
			}
			if tc.wantLayer != "" {
				if got := parsed.Query().Get("layer"); got != tc.wantLayer {
					t.Fatalf("layer param: got %q, want %q (loc=%s)", got, tc.wantLayer, loc)
				}
			}
			if tc.wantHarness != "" {
				if got := parsed.Query().Get("harness"); got != tc.wantHarness {
					t.Fatalf("harness param: got %q, want %q (loc=%s)", got, tc.wantHarness, loc)
				}
			}
			if got := w.Header().Get("Sunset"); got != runsRedirectSunset {
				t.Fatalf("Sunset header: got %q, want %q", got, runsRedirectSunset)
			}
			if got := w.Header().Get("Deprecation"); got != "true" {
				t.Fatalf("Deprecation header: got %q, want %q", got, "true")
			}
		})
	}
}
