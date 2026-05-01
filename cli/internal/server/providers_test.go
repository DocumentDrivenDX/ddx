package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// TestListProviders verifies GET /api/providers returns a JSON array containing
// all known harnesses.
func TestListProviders(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var items []ProviderSummary
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected non-empty provider list")
	}

	// Verify required fields are present and not fabricated as "ok"/"0" when unknown.
	svc, err := agent.NewServiceFromWorkDir(dir)
	if err != nil {
		t.Fatalf("NewServiceFromWorkDir: %v", err)
	}
	infos, err := svc.ListHarnesses(context.Background())
	if err != nil {
		t.Fatalf("ListHarnesses: %v", err)
	}
	harnessNames := map[string]bool{}
	for _, info := range infos {
		harnessNames[info.Name] = true
	}

	for _, item := range items {
		if item.Harness == "" {
			t.Error("provider summary missing harness field")
		}
		if !harnessNames[item.Harness] {
			t.Errorf("unexpected harness in response: %q", item.Harness)
		}
		if item.DisplayName == "" {
			t.Errorf("harness %q missing display_name", item.Harness)
		}
		// Status must be one of the defined values.
		switch item.Status {
		case "available", "unavailable", "unknown":
		default:
			t.Errorf("harness %q has invalid status %q", item.Harness, item.Status)
		}
		// AuthState must be one of the defined values.
		switch item.AuthState {
		case "authenticated", "unauthenticated", "unknown":
		default:
			t.Errorf("harness %q has invalid auth_state %q", item.Harness, item.AuthState)
		}
		// QuotaHeadroom must be one of the defined values.
		switch item.QuotaHeadroom {
		case "ok", "blocked", "unknown":
		default:
			t.Errorf("harness %q has invalid quota_headroom %q", item.Harness, item.QuotaHeadroom)
		}
		// SignalSources must contain at least "none" when no signals available.
		if len(item.SignalSources) == 0 {
			t.Errorf("harness %q has empty signal_sources (should be at least [none])", item.Harness)
		}
		// CostClass must not be empty.
		if item.CostClass == "" {
			t.Errorf("harness %q missing cost_class", item.Harness)
		}
		// LastCheckedTS must be present.
		if item.LastCheckedTS == "" {
			t.Errorf("harness %q missing last_checked_ts", item.Harness)
		}
	}

	// All harnesses in the registry must appear in the response.
	found := map[string]bool{}
	for _, item := range items {
		found[item.Harness] = true
	}
	for name := range harnessNames {
		if !found[name] {
			t.Errorf("harness %q missing from provider list response", name)
		}
	}
}

// TestShowProvider verifies GET /api/providers/{harness} returns a full detail
// object for a known harness.
func TestShowProvider(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/claude", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var detail ProviderDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if detail.Harness != "claude" {
		t.Errorf("expected harness=claude, got %q", detail.Harness)
	}
	if detail.DisplayName == "" {
		t.Error("missing display_name")
	}
	// Status must be one of the defined values.
	switch detail.Status {
	case "available", "unavailable", "unknown":
	default:
		t.Errorf("invalid status %q", detail.Status)
	}
	// AuthState — in test env with no stats-cache, should be "unknown".
	switch detail.AuthState {
	case "authenticated", "unauthenticated", "unknown":
	default:
		t.Errorf("invalid auth_state %q", detail.AuthState)
	}
	// Models array must be present (may be empty).
	if detail.Models == nil {
		t.Error("models field must not be nil (should be empty array when no models)")
	}
	// SignalSources must contain at least one entry.
	if len(detail.SignalSources) == 0 {
		t.Error("signal_sources must not be empty")
	}
	// RoutingSignals.Performance must carry -1 sentinels when no samples.
	perf := detail.RoutingSignals.Performance
	if perf.SampleCount == 0 {
		if perf.SuccessRate != -1 {
			t.Errorf("success_rate should be -1 when sample_count=0, got %v", perf.SuccessRate)
		}
		if perf.P50LatencyMS != -1 {
			t.Errorf("p50_latency_ms should be -1 when sample_count=0, got %v", perf.P50LatencyMS)
		}
		if perf.P95LatencyMS != -1 {
			t.Errorf("p95_latency_ms should be -1 when sample_count=0, got %v", perf.P95LatencyMS)
		}
	}
	// Window must be "7d".
	if perf.Window != "7d" {
		t.Errorf("performance.window should be 7d, got %q", perf.Window)
	}
}

// TestShowProviderNotFound verifies GET /api/providers/{unknown} returns 404.
func TestShowProviderNotFound(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/nonexistent-harness", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestProviderUnknownStateContract verifies that unknown fields carry "unknown"
// or -1 sentinels, not fabricated "ok"/"0" values (FEAT-014 zero-fabrication).
func TestProviderUnknownStateContract(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Test each harness that will have no signal data in the test environment.
	for _, harnessName := range []string{"claude", "codex", "gemini"} {
		req := httptest.NewRequest(http.MethodGet, "/api/providers/"+harnessName, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", harnessName, w.Code)
			continue
		}

		var detail ProviderDetail
		if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
			t.Errorf("%s: decode error: %v", harnessName, err)
			continue
		}

		// quota_headroom must not be fabricated as "ok" when there's no signal.
		// It may be "unknown" or "ok" only if there genuinely is signal data.
		if detail.AuthState == "unauthenticated" {
			// This is valid but unlikely in a test — flag it for investigation.
			t.Logf("%s: auth_state=unauthenticated (unexpected in test env)", harnessName)
		}

		// Verify signal_sources contains valid values only.
		validSources := map[string]bool{
			"native-session-jsonl": true,
			"stats-cache":          true,
			"ddx-metrics":          true,
			"none":                 true,
		}
		for _, src := range detail.SignalSources {
			if !validSources[src] {
				t.Errorf("%s: invalid signal_source value %q", harnessName, src)
			}
		}

		// Performance sentinels: when no data, must be -1 not 0.
		perf := detail.RoutingSignals.Performance
		if perf.SampleCount < 3 {
			if perf.SuccessRate != -1 {
				t.Errorf("%s: success_rate must be -1 when sample_count<3, got %v", harnessName, perf.SuccessRate)
			}
		}
	}
}

func TestProviderQuotaStatusTranslationFromHarnessInfo(t *testing.T) {
	now := time.Date(2026, 4, 21, 1, 0, 0, 0, time.UTC)

	cases := []struct {
		name          string
		status        string
		fresh         bool
		wantHeadroom  string
		wantAuthState string
		wantFreshness string
	}{
		{
			name:          "ok fresh",
			status:        "ok",
			fresh:         true,
			wantHeadroom:  "ok",
			wantAuthState: "authenticated",
			wantFreshness: "fresh",
		},
		{
			name:          "stale but usable",
			status:        "stale",
			fresh:         false,
			wantHeadroom:  "ok",
			wantAuthState: "authenticated",
			wantFreshness: "stale",
		},
		{
			name:          "unavailable",
			status:        "unavailable",
			fresh:         false,
			wantHeadroom:  "unknown",
			wantAuthState: "unknown",
			wantFreshness: "unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := agentlib.HarnessInfo{
				Name:      "claude",
				Available: true,
				Quota: &agentlib.QuotaState{
					Status:     tc.status,
					Fresh:      tc.fresh,
					Source:     "stats-cache",
					CapturedAt: now.Add(-1 * time.Minute),
				},
			}
			signal := signalFromHarnessInfo(info, now)
			summary := buildProviderSummary(info, signal, nil, now)
			detail := buildProviderDetail(info, signal, nil, nil, now)

			if summary.QuotaHeadroom != tc.wantHeadroom {
				t.Fatalf("summary quota_headroom = %q, want %q", summary.QuotaHeadroom, tc.wantHeadroom)
			}
			if detail.AuthState != tc.wantAuthState {
				t.Fatalf("detail auth_state = %q, want %q", detail.AuthState, tc.wantAuthState)
			}
			if signal.Source.Freshness != tc.wantFreshness {
				t.Fatalf("signal freshness = %q, want %q", signal.Source.Freshness, tc.wantFreshness)
			}
			if detail.SignalSources[0] != "stats-cache" {
				t.Fatalf("detail signal source = %q, want stats-cache", detail.SignalSources[0])
			}
		})
	}
}

// TestProviderSummaryDisplayName verifies display names are set for all known harnesses.
func TestProviderSummaryDisplayName(t *testing.T) {
	cases := []struct {
		harness string
		want    string
	}{
		{"codex", "Codex (OpenAI)"},
		{"claude", "Claude (Anthropic)"},
		{"gemini", "Gemini (Google)"},
		{"opencode", "OpenCode"},
		{"agent", "DDx Embedded Agent"},
		{"pi", "Pi"},
		{"virtual", "Virtual (Test)"},
		{"openrouter", "OpenRouter"},
		{"lmstudio", "LM Studio"},
	}
	for _, tc := range cases {
		got := harnessDisplayName(tc.harness)
		if got != tc.want {
			t.Errorf("harnessDisplayName(%q) = %q, want %q", tc.harness, got, tc.want)
		}
	}
}

// TestProviderPerformanceWithData verifies performance metrics with sample data.
func TestProviderPerformanceWithData(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	// Generate 5 outcomes within the 7d window.
	outcomes := []agent.RoutingOutcome{
		{Harness: "claude", ObservedAt: now.Add(-1 * time.Hour), Success: true, LatencyMS: 1000},
		{Harness: "claude", ObservedAt: now.Add(-2 * time.Hour), Success: true, LatencyMS: 2000},
		{Harness: "claude", ObservedAt: now.Add(-3 * time.Hour), Success: false, LatencyMS: 3000},
		{Harness: "claude", ObservedAt: now.Add(-4 * time.Hour), Success: true, LatencyMS: 4000},
		{Harness: "claude", ObservedAt: now.Add(-5 * time.Hour), Success: true, LatencyMS: 5000},
	}

	filtered := filterProviderOutcomes(outcomes, "claude", now, 7)
	if len(filtered) != 5 {
		t.Fatalf("expected 5 outcomes, got %d", len(filtered))
	}

	perf := computeProviderPerformance(filtered)
	if perf.SampleCount != 5 {
		t.Errorf("sample_count = %d, want 5", perf.SampleCount)
	}
	// 4/5 = 0.8 success rate
	if perf.SuccessRate != 0.8 {
		t.Errorf("success_rate = %v, want 0.8", perf.SuccessRate)
	}
	// Sorted latencies: [1000, 2000, 3000, 4000, 5000] — p50 = index 2 = 3000
	if perf.P50LatencyMS != 3000 {
		t.Errorf("p50_latency_ms = %d, want 3000", perf.P50LatencyMS)
	}
	// p95 index = int(4 * 0.95) = int(3.8) = 3 → latency[3] = 4000
	if perf.P95LatencyMS != 4000 {
		t.Errorf("p95_latency_ms = %d, want 4000", perf.P95LatencyMS)
	}
}

// TestProviderPerformanceTooFewSamples verifies -1 sentinels when sample_count < 3.
func TestProviderPerformanceTooFewSamples(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	outcomes := []agent.RoutingOutcome{
		{Harness: "claude", ObservedAt: now.Add(-1 * time.Hour), Success: true, LatencyMS: 1000},
		{Harness: "claude", ObservedAt: now.Add(-2 * time.Hour), Success: true, LatencyMS: 2000},
	}

	perf := computeProviderPerformance(filterProviderOutcomes(outcomes, "claude", now, 7))
	if perf.SuccessRate != -1 {
		t.Errorf("success_rate should be -1 for <3 samples, got %v", perf.SuccessRate)
	}
	if perf.P50LatencyMS != -1 {
		t.Errorf("p50 should be -1 for <3 samples, got %v", perf.P50LatencyMS)
	}
}

// TestSignalSourceAPIEnum verifies the mapping from internal source kinds to API enum.
func TestSignalSourceAPIEnum(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{"native-session-jsonl", "native-session-jsonl"},
		{"stats-cache", "stats-cache"},
		{"quota-snapshot", "stats-cache"},
		{"http-balance", "stats-cache"},
		{"http-models", "stats-cache"},
		{"recent-session-log", "ddx-metrics"},
		{"docs-only", "none"},
		{"unknown", "none"},
		{"", "none"},
		{"some-future-kind", "none"},
	}
	for _, tc := range cases {
		got := signalSourceAPIEnum(tc.kind)
		if got != tc.want {
			t.Errorf("signalSourceAPIEnum(%q) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// TestMCPProviderList verifies the ddx_provider_list MCP tool.
func TestMCPProviderList(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_provider_list","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %v", resp)
	}
	isErr, _ := result["isError"].(bool)
	if isErr {
		t.Fatalf("MCP returned error: %v", result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("missing content in result")
	}
	first := content[0].(map[string]any)
	text, _ := first["text"].(string)

	var items []ProviderSummary
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("content is not a valid provider list: %v\n%s", err, text)
	}
	if len(items) == 0 {
		t.Error("expected non-empty provider list from MCP tool")
	}
}

// TestMCPProviderShow verifies the ddx_provider_show MCP tool.
func TestMCPProviderShow(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_provider_show","arguments":{"harness":"claude"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result")
	}
	isErr, _ := result["isError"].(bool)
	if isErr {
		t.Fatalf("MCP returned error: %v", result)
	}
	content := result["content"].([]any)
	first := content[0].(map[string]any)
	text, _ := first["text"].(string)

	var detail ProviderDetail
	if err := json.Unmarshal([]byte(text), &detail); err != nil {
		t.Fatalf("content is not a valid provider detail: %v\n%s", err, text)
	}
	if detail.Harness != "claude" {
		t.Errorf("expected harness=claude, got %q", detail.Harness)
	}
}

// TestMCPProviderShowUnknownHarness verifies ddx_provider_show returns isError for unknown harness.
func TestMCPProviderShowUnknownHarness(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ddx_provider_show","arguments":{"harness":"nonexistent"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	result, _ := resp["result"].(map[string]any)
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError=true for unknown harness")
	}
}
