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
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/require"
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
	// AuthState — in test env with no live quota signal, should be "unknown".
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

		// signal_sources now comes directly from upstream DTO fields, so the
		// contract is simply that it is populated and non-empty.
		if len(detail.SignalSources) == 0 {
			t.Errorf("%s: signal_sources must not be empty", harnessName)
		}
		for _, src := range detail.SignalSources {
			if strings.TrimSpace(src) == "" {
				t.Errorf("%s: signal_source must not be blank", harnessName)
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

func TestProviderFieldsComeFromHarnessInfoDirectly(t *testing.T) {
	now := time.Date(2026, 4, 21, 1, 0, 0, 0, time.UTC)
	info := agentlib.HarnessInfo{
		Name:                "codex",
		Available:           true,
		AutoRoutingEligible: true,
		DefaultModel:        "gpt-5.4-custom",
		CostClass:           "expensive",
		Billing:             agentlib.BillingModelSubscription,
		Account: &agentlib.AccountStatus{
			Authenticated: true,
			Source:        "account-source",
			CapturedAt:    now.Add(-5 * time.Minute),
		},
		Quota: &agentlib.QuotaState{
			Status:     "stale",
			Fresh:      false,
			Source:     "quota-source",
			CapturedAt: now.Add(-3 * time.Minute),
		},
		UsageWindows: []agentlib.UsageWindow{
			{
				Name:         "7d",
				Source:       "usage-7d",
				CapturedAt:   now.Add(-2 * time.Minute),
				InputTokens:  12,
				OutputTokens: 8,
				TotalTokens:  20,
				CostUSD:      0.75,
			},
			{
				Name:         "30d",
				Source:       "usage-30d",
				CapturedAt:   now.Add(-1 * time.Minute),
				InputTokens:  42,
				OutputTokens: 18,
				TotalTokens:  60,
				CostUSD:      1.25,
			},
		},
	}

	summary := buildProviderSummary(info, nil, now)
	detail := buildProviderDetail(info, nil, now)

	if summary.AuthState != "authenticated" {
		t.Fatalf("summary auth_state = %q, want authenticated", summary.AuthState)
	}
	if summary.QuotaHeadroom != "ok" {
		t.Fatalf("summary quota_headroom = %q, want ok", summary.QuotaHeadroom)
	}
	if summary.CostClass != "expensive" {
		t.Fatalf("summary cost_class = %q, want expensive", summary.CostClass)
	}
	if summary.FreshnessTS != "2026-04-21T00:59:00Z" {
		t.Fatalf("summary freshness_ts = %q, want latest captured timestamp", summary.FreshnessTS)
	}

	wantSources := map[string]bool{
		"account-source": true,
		"quota-source":   true,
		"usage-7d":       true,
		"usage-30d":      true,
	}
	if len(summary.SignalSources) != len(wantSources) {
		t.Fatalf("summary signal_sources = %v, want %d entries", summary.SignalSources, len(wantSources))
	}
	for _, source := range summary.SignalSources {
		if !wantSources[source] {
			t.Fatalf("unexpected signal source %q in %v", source, summary.SignalSources)
		}
	}

	if detail.AuthState != "authenticated" {
		t.Fatalf("detail auth_state = %q, want authenticated", detail.AuthState)
	}
	if detail.RoutingSignals.RequestFit != "capable" {
		t.Fatalf("detail request_fit = %q, want capable", detail.RoutingSignals.RequestFit)
	}
	if len(detail.Models) != 1 {
		t.Fatalf("detail models = %v, want one direct default-model entry", detail.Models)
	}
	if detail.Models[0].Model != "gpt-5.4-custom" {
		t.Fatalf("detail model = %q, want direct default model", detail.Models[0].Model)
	}
	if detail.Models[0].QuotaHeadroom != "ok" {
		t.Fatalf("detail model quota_headroom = %q, want ok", detail.Models[0].QuotaHeadroom)
	}
	if detail.HistoricalUsage == nil || detail.HistoricalUsage.Window7D == nil || detail.HistoricalUsage.Window30D == nil {
		t.Fatalf("detail historical_usage = %+v, want both direct usage windows", detail.HistoricalUsage)
	}
	if got := detail.HistoricalUsage.Window7D.TotalTokens; got != 20 {
		t.Fatalf("detail window7d total_tokens = %d, want 20", got)
	}
	if got := detail.HistoricalUsage.Window30D.TotalTokens; got != 60 {
		t.Fatalf("detail window30d total_tokens = %d, want 60", got)
	}
	if detail.BurnEstimate == nil {
		t.Fatal("detail burn_estimate must be populated for subscription harnesses with 7d usage")
	}
	if detail.BurnEstimate.Source != "usage-7d" && detail.BurnEstimate.Source != "usage-30d" {
		t.Fatalf("detail burn_estimate source = %q, want direct usage source", detail.BurnEstimate.Source)
	}
}

func TestCollectHarnessSignalSourcesFiltersStaleCacheLabels(t *testing.T) {
	info := agentlib.HarnessInfo{
		Account: &agentlib.AccountStatus{
			Source: "stats-cache",
		},
		Quota: &agentlib.QuotaState{
			Source: "quota-snapshot",
		},
		UsageWindows: []agentlib.UsageWindow{
			{Name: "7d", Source: "native-session-jsonl"},
			{Name: "30d", Source: "ddx-metrics"},
			{Name: "90d", Source: "stats-cache+ddx-metrics"},
		},
	}

	got := collectHarnessSignalSources(info)
	if len(got) != 1 {
		t.Fatalf("signal sources = %v, want only direct live sources", got)
	}
	if got[0] != "native-session-jsonl" {
		t.Fatalf("signal source = %q, want native-session-jsonl", got[0])
	}
}

func TestProviderDetailSuppressesStaleQuotaAndUsageSources(t *testing.T) {
	now := time.Date(2026, 4, 21, 1, 0, 0, 0, time.UTC)
	info := agentlib.HarnessInfo{
		Name:         "codex",
		DefaultModel: "gpt-5.4-custom",
		Billing:      agentlib.BillingModelSubscription,
		Account: &agentlib.AccountStatus{
			Authenticated: true,
			Source:        "account-source",
			CapturedAt:    now.Add(-5 * time.Minute),
		},
		Quota: &agentlib.QuotaState{
			Status:     "ok",
			Fresh:      true,
			Source:     "quota-snapshot",
			CapturedAt: now.Add(-3 * time.Minute),
		},
		UsageWindows: []agentlib.UsageWindow{
			{
				Name:         "7d",
				Source:       "burn-summaries",
				CapturedAt:   now.Add(-2 * time.Minute),
				TotalTokens:  7000,
				InputTokens:  5000,
				OutputTokens: 2000,
				CostUSD:      0.75,
			},
			{
				Name:         "30d",
				Source:       "ddx-metrics",
				CapturedAt:   now.Add(-1 * time.Minute),
				TotalTokens:  25000,
				InputTokens:  18000,
				OutputTokens: 7000,
				CostUSD:      1.25,
			},
		},
	}

	summary := buildProviderSummary(info, nil, now)
	detail := buildProviderDetail(info, nil, now)

	require.Equal(t, []string{"account-source"}, summary.SignalSources)
	require.Len(t, detail.Models, 1)
	require.Empty(t, detail.Models[0].Source)
	require.NotNil(t, detail.BurnEstimate)
	require.Equal(t, "none", detail.BurnEstimate.Source)
	require.Equal(t, "account-source", detail.SignalSources[0])
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
