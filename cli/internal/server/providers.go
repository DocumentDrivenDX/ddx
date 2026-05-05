package server

// Provider availability and utilization endpoints — FEAT-002 §26-27.
// Field semantics, unknown-state rules, and the zero-fabrication contract are
// governed by FEAT-014 (dashboard read model).

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/DocumentDrivenDX/fizeau"
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
		signal := signalFromHarnessInfo(info, now)
		result = append(result, buildProviderSummary(info, signal, report, now))
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
	signal := signalFromHarnessInfo(info, now)
	report := liveRouteStatusReport(r.Context(), s.WorkingDir)

	detail := buildProviderDetail(info, signal, report, now)
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
		signal := signalFromHarnessInfo(info, now)
		result = append(result, buildProviderSummary(info, signal, report, now))
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
	signal := signalFromHarnessInfo(info, now)
	report := liveRouteStatusReport(context.Background(), s.WorkingDir)

	detail := buildProviderDetail(info, signal, report, now)
	data, err := json.Marshal(detail)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("{}")}}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

// ---- Build helpers ----

// signalFromHarnessInfo translates upstream HarnessInfo into the
// ddx-local RoutingSignalSnapshot shape used by buildProviderSummary /
// buildProviderDetail. This is the vocabulary translation shim introduced
// when the DDx-side provider-native parsers were retired (ddx-7bc0c8d5):
// upstream quota.Status values "ok|stale|unavailable" map to the existing
// "fresh|stale|unknown" vocabulary that the GraphQL/REST surface exposes so
// the SvelteKit frontend renders identically.
func signalFromHarnessInfo(info agentlib.HarnessInfo, now time.Time) agent.RoutingSignalSnapshot {
	snap := agent.RoutingSignalSnapshot{Provider: info.Name}

	if info.Account != nil && (info.Account.Email != "" || info.Account.PlanType != "" || info.Account.OrgName != "") {
		snap.Account = &agent.AccountInfo{
			Email:    info.Account.Email,
			PlanType: info.Account.PlanType,
			OrgName:  info.Account.OrgName,
		}
	}

	if info.Quota == nil {
		snap.CurrentQuota = agent.QuotaSignal{
			Source: agent.SignalSourceMetadata{
				Provider:  info.Name,
				Kind:      "docs-only",
				Freshness: "unknown",
			},
			State: "unknown",
		}
		snap.Source = snap.CurrentQuota.Source
		return snap
	}

	// Translate upstream quota.Status (ok|stale|unavailable|unauthenticated|unknown)
	// into the existing ddx vocabulary.
	state := "unknown"
	switch info.Quota.Status {
	case "ok":
		state = "ok"
	case "stale":
		state = "ok" // stale-but-present data still counts as headroom.
	case "unavailable", "unauthenticated":
		state = "unknown"
	}

	// Promote worst non-extra window to "blocked" if any window is blocked.
	var usedPercent int
	var windowMinutes int
	var resetsAt string
	var translatedWindows []agent.QuotaWindow
	for _, w := range info.Quota.Windows {
		qw := agent.QuotaWindow{
			Name:          w.Name,
			LimitID:       w.LimitID,
			WindowMinutes: w.WindowMinutes,
			UsedPercent:   w.UsedPercent,
			ResetsAt:      w.ResetsAt,
			ResetsAtUnix:  w.ResetsAtUnix,
			State:         w.State,
		}
		translatedWindows = append(translatedWindows, qw)
		if w.LimitID == "extra" {
			continue
		}
		if w.State == "blocked" {
			state = "blocked"
			resetsAt = w.ResetsAt
		}
		if w.UsedPercent > float64(usedPercent) {
			usedPercent = int(w.UsedPercent + 0.5)
			windowMinutes = w.WindowMinutes
		}
	}
	snap.QuotaWindows = translatedWindows

	freshness := "fresh"
	if !info.Quota.Fresh {
		freshness = "stale"
	}
	if info.Quota.Status == "unavailable" || info.Quota.Status == "unauthenticated" {
		freshness = "unknown"
	}

	kind := agent.NormalizeSignalSourceKind(info.Quota.Source)

	var ageSeconds int64
	if !info.Quota.CapturedAt.IsZero() {
		if age := now.UTC().Sub(info.Quota.CapturedAt.UTC()); age > 0 {
			ageSeconds = int64(age.Seconds())
		}
	}

	meta := agent.SignalSourceMetadata{
		Provider:   info.Name,
		Kind:       kind,
		ObservedAt: info.Quota.CapturedAt.UTC(),
		Freshness:  freshness,
		AgeSeconds: ageSeconds,
	}
	snap.Source = meta
	snap.CurrentQuota = agent.QuotaSignal{
		Source:        meta,
		State:         state,
		UsedPercent:   usedPercent,
		WindowMinutes: windowMinutes,
		ResetsAt:      resetsAt,
	}

	for _, u := range info.UsageWindows {
		snap.HistoricalUsage.InputTokens += u.InputTokens
		snap.HistoricalUsage.OutputTokens += u.OutputTokens
		snap.HistoricalUsage.TotalTokens += u.TotalTokens
	}
	snap.HistoricalUsage.Source = meta

	return snap
}

func buildProviderSummary(
	info agentlib.HarnessInfo,
	signal agent.RoutingSignalSnapshot,
	report *agentlib.RouteStatusReport,
	now time.Time,
) ProviderSummary {
	perf := providerPerformanceFromRouteStatus(report, info.Name)
	sources := collectProviderSignalSources(signal)

	var freshnessTS string
	if ts := providerFreshnessTS(signal); !ts.IsZero() {
		freshnessTS = ts.UTC().Format(time.RFC3339)
	}

	return ProviderSummary{
		Harness:            info.Name,
		DisplayName:        harnessDisplayName(info.Name),
		Status:             providerStatusStrInfo(info),
		AuthState:          providerAuthStateStr(signal),
		QuotaHeadroom:      providerQuotaHeadroomStr(signal),
		SignalSources:      sources,
		FreshnessTS:        freshnessTS,
		LastCheckedTS:      now.UTC().Format(time.RFC3339),
		RecentSuccessRate:  perf.SuccessRate,
		RecentLatencyP50MS: perf.P50LatencyMS,
		CostClass:          harnessCosCostClassStr(info),
	}
}

func buildProviderDetail(
	info agentlib.HarnessInfo,
	signal agent.RoutingSignalSnapshot,
	report *agentlib.RouteStatusReport,
	now time.Time,
) ProviderDetail {
	perf := providerPerformanceFromRouteStatus(report, info.Name)
	sources := collectProviderSignalSources(signal)

	var freshnessTS string
	if ts := providerFreshnessTS(signal); !ts.IsZero() {
		freshnessTS = ts.UTC().Format(time.RFC3339)
	}

	models := buildModelQuotaList(info, signal)
	historicalUsage := computeProviderHistoricalUsage(info, signal)
	burnEstimate := computeProviderBurnEstimate(info, historicalUsage, signal)

	statusStr := providerStatusStrInfo(info)
	return ProviderDetail{
		Harness:         info.Name,
		DisplayName:     harnessDisplayName(info.Name),
		Status:          statusStr,
		AuthState:       providerAuthStateStr(signal),
		Models:          models,
		HistoricalUsage: historicalUsage,
		BurnEstimate:    burnEstimate,
		RoutingSignals: ProviderRoutingSignals{
			Availability: statusStr,
			RequestFit:   providerRequestFitStrInfo(info),
			CostEstimate: "unknown",
			Performance:  perf,
		},
		SignalSources: sources,
		FreshnessTS:   freshnessTS,
	}
}

func buildModelQuotaList(info agentlib.HarnessInfo, signal agent.RoutingSignalSnapshot) []ProviderModelQuota {
	models := harnessDefaultModels(info.Name)
	if len(models) == 0 {
		return []ProviderModelQuota{}
	}

	quotaState := providerQuotaHeadroomStr(signal)
	sourceEnum := signalSourceAPIEnum(signal.Source.Kind)
	var sourceNote string
	if quotaState == "unknown" && info.Name == "claude" {
		sourceNote = "no stable non-PTY quota source confirmed"
	}

	result := make([]ProviderModelQuota, 0, len(models))
	for _, m := range models {
		result = append(result, ProviderModelQuota{
			Model:         m,
			QuotaHeadroom: quotaState,
			Source:        sourceEnum,
			SourceNote:    sourceNote,
		})
	}
	return result
}

func computeProviderHistoricalUsage(
	info agentlib.HarnessInfo,
	signal agent.RoutingSignalSnapshot,
) *ProviderHistoricalUsage {
	window7d := usageWindowFromLiveWindows(info, "7d")
	window30d := usageWindowFromLiveWindows(info, "30d")
	if window7d == nil && window30d == nil && signal.HistoricalUsage.TotalTokens == 0 && signal.HistoricalUsage.InputTokens == 0 && signal.HistoricalUsage.OutputTokens == 0 {
		return nil
	}
	if window7d == nil {
		window7d = usageWindowFromSignal(info, signal)
	}
	if window30d == nil {
		window30d = usageWindowFromSignal(info, signal)
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
		if info.IsSubscription {
			result.CostUSD = 0
			result.CostNote = "subscription plan; per-token cost not billed"
		} else if info.IsLocal {
			result.CostUSD = 0
		} else if result.CostUSD == 0 {
			result.CostUSD = -1
		}
		return result
	}
	return nil
}

func usageWindowFromSignal(info agentlib.HarnessInfo, signal agent.RoutingSignalSnapshot) *ProviderUsageWindow {
	if signal.HistoricalUsage.TotalTokens == 0 && signal.HistoricalUsage.InputTokens == 0 && signal.HistoricalUsage.OutputTokens == 0 {
		return nil
	}
	w := &ProviderUsageWindow{
		InputTokens:  signal.HistoricalUsage.InputTokens,
		OutputTokens: signal.HistoricalUsage.OutputTokens,
		TotalTokens:  signal.HistoricalUsage.TotalTokens,
	}
	if info.IsSubscription || info.IsLocal {
		w.CostUSD = 0
		if info.IsSubscription {
			w.CostNote = "subscription plan; per-token cost not billed"
		}
	} else {
		w.CostUSD = -1
	}
	return w
}

func computeProviderBurnEstimate(
	info agentlib.HarnessInfo,
	usage *ProviderHistoricalUsage,
	signal agent.RoutingSignalSnapshot,
) *ProviderBurnEstimate {
	if !info.IsSubscription {
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
	sourceStr := signalSourceAPIEnum(signal.Source.Kind)
	if sourceStr == "" {
		sourceStr = "none"
	}
	freshnessTS := ""
	if !signal.Source.ObservedAt.IsZero() {
		freshnessTS = signal.Source.ObservedAt.UTC().Format(time.RFC3339)
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

// harnessDefaultModels returns the well-known default model(s) for a harness.
// Mirrors the historical in-package default model fields used by
// the provider detail endpoint. Empty slice means "no default model published".
func harnessDefaultModels(name string) []string {
	switch name {
	case "codex":
		return []string{"gpt-5.4"}
	case "claude":
		return []string{"claude-sonnet-4-6"}
	case "virtual":
		return []string{"recorded"}
	default:
		return nil
	}
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

// filterProviderOutcomes returns outcomes for a given harness within the last windowDays days.
func filterProviderOutcomes(outcomes []agent.RoutingOutcome, harnessName string, now time.Time, windowDays int) []agent.RoutingOutcome {
	cutoff := now.Add(-time.Duration(windowDays) * 24 * time.Hour)
	result := make([]agent.RoutingOutcome, 0)
	for _, o := range outcomes {
		if o.Harness == harnessName && o.ObservedAt.After(cutoff) {
			result = append(result, o)
		}
	}
	return result
}

// computeProviderPerformance computes latency percentiles and success rate from outcomes.
// Returns -1 sentinels when sample_count < 3 per FEAT-014.
func computeProviderPerformance(outcomes []agent.RoutingOutcome) ProviderPerformance {
	perf := ProviderPerformance{
		P50LatencyMS: -1,
		P95LatencyMS: -1,
		SuccessRate:  -1,
		SampleCount:  len(outcomes),
		Window:       "7d",
	}
	if len(outcomes) < 3 {
		return perf
	}
	var successCount int
	latencies := make([]int, 0, len(outcomes))
	for _, o := range outcomes {
		if o.Success {
			successCount++
		}
		latencies = append(latencies, o.LatencyMS)
	}
	perf.SuccessRate = float64(successCount) / float64(len(outcomes))
	sort.Ints(latencies)
	perf.P50LatencyMS = latencies[len(latencies)/2]
	p95Idx := int(float64(len(latencies)-1) * 0.95)
	perf.P95LatencyMS = latencies[p95Idx]
	return perf
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

// collectProviderSignalSources builds the signal_sources array from the live
// snapshot. Non-live DDx cache labels are normalized away before this point.
func collectProviderSignalSources(signal agent.RoutingSignalSnapshot) []string {
	if enum := signalSourceAPIEnum(signal.Source.Kind); enum != "none" {
		return []string{enum}
	}
	return []string{"none"}
}

// providerFreshnessTS returns the contributing signal timestamp.
// Returns zero time when no signals exist (caller omits the field per FEAT-014).
func providerFreshnessTS(signal agent.RoutingSignalSnapshot) time.Time {
	return signal.Source.ObservedAt
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

// providerRequestFitStrInfo returns request_fit from harness availability.
func providerRequestFitStrInfo(info agentlib.HarnessInfo) string {
	if info.Available {
		return "capable"
	}
	return "unknown"
}

// providerAuthStateStr infers auth state from signal source and quota data.
// Per FEAT-014: probe fails or is not implemented → "unknown".
func providerAuthStateStr(signal agent.RoutingSignalSnapshot) string {
	// A non-trivial quota state means the harness responded with real data → authenticated.
	state := signal.CurrentQuota.State
	if state == "ok" || state == "blocked" {
		return "authenticated"
	}
	// A live signal from a native source (not docs-only/unknown) → authenticated.
	kind := signal.Source.Kind
	if kind != "" && kind != "docs-only" && kind != "unknown" && signal.Source.Freshness != "unknown" {
		return "authenticated"
	}
	return "unknown"
}

// providerQuotaHeadroomStr maps CurrentQuota.State to the API enum.
// Returns "unknown" when no trustworthy live source exists per FEAT-014.
func providerQuotaHeadroomStr(signal agent.RoutingSignalSnapshot) string {
	switch signal.CurrentQuota.State {
	case "ok":
		return "ok"
	case "blocked":
		return "blocked"
	default:
		return "unknown"
	}
}

// harnessCosCostClassStr maps a HarnessInfo to the API cost_class string.
func harnessCosCostClassStr(info agentlib.HarnessInfo) string {
	if info.IsSubscription {
		return "subscription"
	}
	if info.IsLocal {
		return "local"
	}
	if info.CostClass != "" {
		return info.CostClass
	}
	return "unknown"
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

// signalSourceAPIEnum maps an internal source kind to the FEAT-014 API enum value.
// Non-live DDx-local cache labels are normalized away by the shared agent helper.
func signalSourceAPIEnum(kind string) string {
	if normalized := agent.NormalizeSignalSourceKind(kind); normalized != "" {
		return normalized
	}
	return "none"
}
