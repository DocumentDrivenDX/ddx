package server

// Provider availability and utilization endpoints — FEAT-002 §26-27.
// Field semantics, unknown-state rules, and the zero-fabrication contract are
// governed by FEAT-014 (dashboard read model).

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
)

// ---- Read-model types ----

// ProviderSummary is the list-response item for GET /api/providers.
// -1 sentinels on numeric fields mean unknown (FEAT-014 zero-fabrication).
type ProviderSummary struct {
	Harness            string   `json:"harness"`
	DisplayName        string   `json:"display_name"`
	Status             string   `json:"status"`         // available | unavailable | unknown
	AuthState          string   `json:"auth_state"`     // authenticated | unauthenticated | unknown
	QuotaHeadroom      string   `json:"quota_headroom"` // ok | blocked | unknown
	SignalSources      []string `json:"signal_sources"`
	FreshnessTS        string   `json:"freshness_ts,omitempty"`
	LastCheckedTS      string   `json:"last_checked_ts,omitempty"`
	RecentSuccessRate  float64  `json:"recent_success_rate"`   // -1 when sample_count < 3
	RecentLatencyP50MS int      `json:"recent_latency_p50_ms"` // -1 when unknown
	CostClass          string   `json:"cost_class"`
}

// ProviderModelQuota is per-model quota info within a provider detail.
type ProviderModelQuota struct {
	Model         string `json:"model"`
	QuotaHeadroom string `json:"quota_headroom"` // ok | blocked | unknown
	Source        string `json:"source"`
	SourceNote    string `json:"source_note,omitempty"`
}

// ProviderUsageWindow holds token/cost totals for one time window.
// -1 sentinel means unknown per FEAT-014 zero-fabrication contract.
type ProviderUsageWindow struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	CostNote     string  `json:"cost_note,omitempty"`
}

// ProviderHistoricalUsage holds 7d and 30d usage windows.
type ProviderHistoricalUsage struct {
	Window7D  *ProviderUsageWindow `json:"window_7d,omitempty"`
	Window30D *ProviderUsageWindow `json:"window_30d,omitempty"`
}

// ProviderBurnEstimate is the DDx-derived subscription-pressure estimate.
type ProviderBurnEstimate struct {
	DailyTokenRate   float64 `json:"daily_token_rate"`  // -1 when unknown
	SubscriptionBurn string  `json:"subscription_burn"` // low | moderate | high | unknown
	Source           string  `json:"source,omitempty"`
	Confidence       string  `json:"confidence"` // high | medium | low
	FreshnessTS      string  `json:"freshness_ts,omitempty"`
}

// ProviderPerformance holds DDx-observed latency/success metrics.
type ProviderPerformance struct {
	P50LatencyMS int     `json:"p50_latency_ms"` // -1 when unknown
	P95LatencyMS int     `json:"p95_latency_ms"` // -1 when unknown
	SuccessRate  float64 `json:"success_rate"`   // -1 when sample_count < 3
	SampleCount  int     `json:"sample_count"`
	Window       string  `json:"window"` // "7d"
}

// ProviderRoutingSignals is the routing signal summary within a provider detail.
type ProviderRoutingSignals struct {
	Availability string              `json:"availability"` // available | unavailable | unknown
	RequestFit   string              `json:"request_fit"`  // capable | unknown
	CostEstimate string              `json:"cost_estimate"`
	Performance  ProviderPerformance `json:"performance"`
}

// ProviderDetail is the response shape for GET /api/providers/{harness}.
type ProviderDetail struct {
	Harness         string                   `json:"harness"`
	DisplayName     string                   `json:"display_name"`
	Status          string                   `json:"status"`
	AuthState       string                   `json:"auth_state"`
	Models          []ProviderModelQuota     `json:"models"`
	HistoricalUsage *ProviderHistoricalUsage `json:"historical_usage,omitempty"`
	BurnEstimate    *ProviderBurnEstimate    `json:"burn_estimate,omitempty"`
	RoutingSignals  ProviderRoutingSignals   `json:"routing_signals"`
	SignalSources   []string                 `json:"signal_sources"`
	FreshnessTS     string                   `json:"freshness_ts,omitempty"`
}

// ---- HTTP handlers ----

// handleListProviders serves GET /api/providers — list all configured harnesses
// with routing availability, auth state, quota/headroom, and signal freshness.
// Not project-scoped; provider config is host+user global.
func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	infos, err := listHarnessInfos(r.Context(), s.WorkingDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	report := liveRouteStatusReport(r.Context(), s.WorkingDir)

	result := make([]ProviderSummary, 0, len(infos))
	for _, info := range infos {
		result = append(result, buildProviderSummary(info, report, now))
	}
	writeJSON(w, http.StatusOK, result)
}

// handleShowProvider serves GET /api/providers/{harness} — full routing signal
// snapshot per FEAT-014 read-model fields.
func (s *Server) handleShowProvider(w http.ResponseWriter, r *http.Request) {
	harnessName := r.PathValue("harness")
	if harnessName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "harness required"})
		return
	}
	infos, err := listHarnessInfos(r.Context(), s.WorkingDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	info, ok := findHarnessInfo(infos, harnessName)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "harness not found"})
		return
	}

	now := time.Now().UTC()
	report := liveRouteStatusReport(r.Context(), s.WorkingDir)

	detail := buildProviderDetail(info, report, now)
	writeJSON(w, http.StatusOK, detail)
}

// ---- MCP tool implementations ----

func (s *Server) mcpProviderList() mcpToolResult {
	now := time.Now().UTC()
	infos, err := listHarnessInfos(context.Background(), s.WorkingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}

	report := liveRouteStatusReport(context.Background(), s.WorkingDir)

	result := make([]ProviderSummary, 0, len(infos))
	for _, info := range infos {
		result = append(result, buildProviderSummary(info, report, now))
	}
	data, err := json.Marshal(result)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpProviderShow(harnessName string) mcpToolResult {
	if harnessName == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("harness required")}, IsError: true}
	}
	infos, err := listHarnessInfos(context.Background(), s.WorkingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	info, ok := findHarnessInfo(infos, harnessName)
	if !ok {
		return mcpToolResult{Content: []mcpContent{mcpText("harness not found: " + harnessName)}, IsError: true}
	}

	now := time.Now().UTC()
	report := liveRouteStatusReport(context.Background(), s.WorkingDir)

	detail := buildProviderDetail(info, report, now)
	data, err := json.Marshal(detail)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("{}")}}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

// ---- Build helpers ----

func buildProviderSummary(
	info agentlib.HarnessInfo,
	report *agentlib.RouteStatusReport,
	now time.Time,
) ProviderSummary {
	perf := providerPerformanceFromRouteStatus(report, info.Name)
	sources := collectHarnessSignalSources(info)

	var freshnessTS string
	if ts := providerFreshnessTS(info); !ts.IsZero() {
		freshnessTS = ts.UTC().Format(time.RFC3339)
	}

	return ProviderSummary{
		Harness:            info.Name,
		DisplayName:        harnessDisplayName(info.Name),
		Status:             providerStatusStrInfo(info),
		AuthState:          harnessAuthStateStr(info),
		QuotaHeadroom:      harnessQuotaHeadroomStr(info),
		SignalSources:      sources,
		FreshnessTS:        freshnessTS,
		LastCheckedTS:      now.UTC().Format(time.RFC3339),
		RecentSuccessRate:  perf.SuccessRate,
		RecentLatencyP50MS: perf.P50LatencyMS,
		CostClass:          info.CostClass,
	}
}

func buildProviderDetail(
	info agentlib.HarnessInfo,
	report *agentlib.RouteStatusReport,
	now time.Time,
) ProviderDetail {
	perf := providerPerformanceFromRouteStatus(report, info.Name)
	sources := collectHarnessSignalSources(info)

	var freshnessTS string
	if ts := providerFreshnessTS(info); !ts.IsZero() {
		freshnessTS = ts.UTC().Format(time.RFC3339)
	}

	models := buildModelQuotaList(info)
	historicalUsage := computeProviderHistoricalUsage(info)
	burnEstimate := computeProviderBurnEstimate(info, historicalUsage)

	statusStr := providerStatusStrInfo(info)
	return ProviderDetail{
		Harness:         info.Name,
		DisplayName:     harnessDisplayName(info.Name),
		Status:          statusStr,
		AuthState:       harnessAuthStateStr(info),
		Models:          models,
		HistoricalUsage: historicalUsage,
		BurnEstimate:    burnEstimate,
		RoutingSignals: ProviderRoutingSignals{
			Availability: statusStr,
			RequestFit:   harnessRequestFitStr(info),
			CostEstimate: "unknown",
			Performance:  perf,
		},
		SignalSources: sources,
		FreshnessTS:   freshnessTS,
	}
}

func buildModelQuotaList(info agentlib.HarnessInfo) []ProviderModelQuota {
	if info.DefaultModel == "" {
		return []ProviderModelQuota{}
	}

	source := ""
	if info.Quota != nil {
		source = normalizeHarnessSignalSource(info.Quota.Source)
	}

	return []ProviderModelQuota{{
		Model:         info.DefaultModel,
		QuotaHeadroom: harnessQuotaHeadroomStr(info),
		Source:        source,
	}}
}

func computeProviderHistoricalUsage(info agentlib.HarnessInfo) *ProviderHistoricalUsage {
	window7d := usageWindowFromLiveWindows(info, "7d")
	window30d := usageWindowFromLiveWindows(info, "30d")
	if window7d == nil && window30d == nil {
		return nil
	}
	return &ProviderHistoricalUsage{
		Window7D:  window7d,
		Window30D: window30d,
	}
}

func usageWindowFromLiveWindows(info agentlib.HarnessInfo, windowName string) *ProviderUsageWindow {
	for _, w := range info.UsageWindows {
		if w.Name != windowName {
			continue
		}
		result := &ProviderUsageWindow{
			InputTokens:  w.InputTokens,
			OutputTokens: w.OutputTokens,
			TotalTokens:  w.TotalTokens,
			CostUSD:      w.CostUSD,
		}
		if info.Billing == agentlib.BillingModelSubscription {
			result.CostUSD = 0
			result.CostNote = "subscription plan; per-token cost not billed"
		} else if info.CostClass == "local" {
			result.CostUSD = 0
		} else if result.CostUSD == 0 {
			result.CostUSD = -1
		}
		return result
	}
	return nil
}

func latestHarnessUsageSource(info agentlib.HarnessInfo) string {
	var latest time.Time
	var source string
	for _, window := range info.UsageWindows {
		normalized := normalizeHarnessSignalSource(window.Source)
		if normalized == "" {
			continue
		}
		if window.CapturedAt.IsZero() {
			if source == "" {
				source = normalized
			}
			continue
		}
		if source == "" || window.CapturedAt.After(latest) {
			latest = window.CapturedAt.UTC()
			source = normalized
		}
	}
	return source
}

func computeProviderBurnEstimate(
	info agentlib.HarnessInfo,
	usage *ProviderHistoricalUsage,
) *ProviderBurnEstimate {
	if info.Billing != agentlib.BillingModelSubscription {
		return nil
	}

	if usage == nil || usage.Window7D == nil || usage.Window7D.TotalTokens <= 0 {
		return nil
	}

	dailyTokenRate := float64(usage.Window7D.TotalTokens) / 7.0
	subscriptionBurn := "unknown"
	switch {
	case dailyTokenRate >= 10000:
		subscriptionBurn = "high"
	case dailyTokenRate >= 1000:
		subscriptionBurn = "moderate"
	case dailyTokenRate > 0:
		subscriptionBurn = "low"
	}
	confidence := "low"
	switch {
	case dailyTokenRate >= 10000:
		confidence = "high"
	case dailyTokenRate >= 1000:
		confidence = "medium"
	}
	sourceStr := latestHarnessUsageSource(info)
	if sourceStr == "" {
		sourceStr = "none"
	}
	freshnessTS := ""
	if ts := providerFreshnessTS(info); !ts.IsZero() {
		freshnessTS = ts.UTC().Format(time.RFC3339)
	}

	return &ProviderBurnEstimate{
		DailyTokenRate:   dailyTokenRate,
		SubscriptionBurn: subscriptionBurn,
		Source:           sourceStr,
		Confidence:       confidence,
		FreshnessTS:      freshnessTS,
	}
}

// ---- Utility functions ----

// listHarnessInfos returns the harness inventory via the agent service.
// Replaces the older in-package harness inventory.
func listHarnessInfos(ctx context.Context, workDir string) ([]agentlib.HarnessInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := agent.NewServiceFromWorkDir(workDir)
	if err != nil {
		return nil, err
	}
	return svc.ListHarnesses(ctx)
}

// findHarnessInfo locates a HarnessInfo in the slice by name.
func findHarnessInfo(infos []agentlib.HarnessInfo, name string) (agentlib.HarnessInfo, bool) {
	for i := range infos {
		if infos[i].Name == name {
			return infos[i], true
		}
	}
	return agentlib.HarnessInfo{}, false
}

// liveRouteStatusReport loads the current live route-status report from the
// Fizeau service for the current worktree.
func liveRouteStatusReport(ctx context.Context, workDir string) *agentlib.RouteStatusReport {
	svc, err := agent.NewServiceFromWorkDir(workDir)
	if err != nil {
		return nil
	}
	report, err := svc.RouteStatus(ctx)
	if err != nil {
		return nil
	}
	return report
}

// providerPerformanceFromRouteStatus derives recent provider performance from
// the live Fizeau route-status report, which replaces the old DDx metrics-store
// read model for current routing semantics.
func providerPerformanceFromRouteStatus(report *agentlib.RouteStatusReport, providerName string) ProviderPerformance {
	perf := ProviderPerformance{
		P50LatencyMS: -1,
		P95LatencyMS: -1,
		SuccessRate:  -1,
		SampleCount:  0,
		Window:       "7d",
	}
	if report == nil {
		return perf
	}
	var latencies []int
	var reliabilityTotal float64
	for _, route := range report.Routes {
		for _, candidate := range route.Candidates {
			if candidate.Provider != providerName {
				continue
			}
			perf.SampleCount++
			reliabilityTotal += candidate.ProviderReliabilityRate
			if candidate.RecentLatencyMS > 0 {
				latencies = append(latencies, int(candidate.RecentLatencyMS+0.5))
			}
		}
	}
	if perf.SampleCount < 3 {
		return perf
	}
	perf.SuccessRate = reliabilityTotal / float64(perf.SampleCount)
	if len(latencies) == 0 {
		return perf
	}
	sort.Ints(latencies)
	perf.P50LatencyMS = latencies[len(latencies)/2]
	p95Idx := int(float64(len(latencies)-1) * 0.95)
	perf.P95LatencyMS = latencies[p95Idx]
	return perf
}

// collectHarnessSignalSources builds the signal_sources array from the direct
// HarnessInfo fields.
func collectHarnessSignalSources(info agentlib.HarnessInfo) []string {
	seen := make(map[string]struct{})
	add := func(value string) {
		if value = normalizeHarnessSignalSource(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	if info.Account != nil {
		add(info.Account.Source)
	}
	if info.Quota != nil {
		add(info.Quota.Source)
	}
	for _, window := range info.UsageWindows {
		add(window.Source)
	}
	if len(seen) == 0 {
		return []string{"none"}
	}
	sources := make([]string, 0, len(seen))
	for source := range seen {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

var staleHarnessSignalSources = map[string]struct{}{
	"stats-cache":        {},
	"quota-snapshot":     {},
	"http-balance":       {},
	"http-models":        {},
	"recent-session-log": {},
	"ddx-metrics":        {},
	"routing-outcomes":   {},
	"burn-summaries":     {},
}

func normalizeHarnessSignalSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if isStaleHarnessSignalSource(value) {
		return ""
	}
	return value
}

func isStaleHarnessSignalSource(value string) bool {
	lower := strings.ToLower(value)
	if _, stale := staleHarnessSignalSources[lower]; stale {
		return true
	}
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		switch r {
		case '+', '/', ',', ';', '|':
			return true
		default:
			return false
		}
	})
	if len(tokens) <= 1 {
		return false
	}
	for _, token := range tokens {
		if _, stale := staleHarnessSignalSources[token]; !stale {
			return false
		}
	}
	return true
}

// providerFreshnessTS returns the latest direct capture timestamp available on
// the HarnessInfo.
func providerFreshnessTS(info agentlib.HarnessInfo) time.Time {
	var latest time.Time
	update := func(ts time.Time) {
		if ts.IsZero() {
			return
		}
		ts = ts.UTC()
		if ts.After(latest) {
			latest = ts
		}
	}
	if info.Account != nil {
		update(info.Account.CapturedAt)
	}
	if info.Quota != nil {
		update(info.Quota.CapturedAt)
	}
	for _, window := range info.UsageWindows {
		update(window.CapturedAt)
	}
	return latest
}

// providerStatusStrInfo maps a HarnessInfo to the API status string.
func providerStatusStrInfo(info agentlib.HarnessInfo) string {
	if info.Name == "" {
		return "unknown"
	}
	if info.Available {
		return "available"
	}
	return "unavailable"
}

// harnessAuthStateStr maps the direct HarnessInfo account fields to the API auth state string.
func harnessAuthStateStr(info agentlib.HarnessInfo) string {
	if info.Account != nil {
		if info.Account.Authenticated {
			return "authenticated"
		}
		if info.Account.Unauthenticated {
			return "unauthenticated"
		}
	}
	return "unknown"
}

// harnessRequestFitStr returns request_fit from the direct auto-routing flag.
func harnessRequestFitStr(info agentlib.HarnessInfo) string {
	if info.AutoRoutingEligible {
		return "capable"
	}
	return "unknown"
}

// harnessQuotaHeadroomStr maps the direct HarnessInfo quota state to the API enum.
func harnessQuotaHeadroomStr(info agentlib.HarnessInfo) string {
	if info.Quota == nil {
		return "unknown"
	}
	switch strings.ToLower(strings.TrimSpace(info.Quota.Status)) {
	case "ok", "stale":
		return "ok"
	case "blocked":
		return "blocked"
	default:
		return "unknown"
	}
}

// harnessDisplayName returns a human-readable display name for a harness.
func harnessDisplayName(name string) string {
	switch name {
	case "codex":
		return "Codex (OpenAI)"
	case "claude":
		return "Claude (Anthropic)"
	case "gemini":
		return "Gemini (Google)"
	case "opencode":
		return "OpenCode"
	case "agent":
		return "DDx Embedded Agent"
	case "pi":
		return "Pi"
	case "virtual":
		return "Virtual (Test)"
	case "openrouter":
		return "OpenRouter"
	case "lmstudio":
		return "LM Studio"
	default:
		return name
	}
}
