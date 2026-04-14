package server

// Provider availability and utilization endpoints — FEAT-002 §26-27.
// Field semantics, unknown-state rules, and the zero-fabrication contract are
// governed by FEAT-014 (dashboard read model).

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
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
	registry := agent.NewRegistry()
	runner := s.workers.buildAgentRunner(s.WorkingDir)

	statuses := registry.Discover()
	statusByName := make(map[string]agent.HarnessStatus, len(statuses))
	for _, st := range statuses {
		statusByName[st.Name] = st
	}

	metricsStore := metricsStoreFromRunner(runner, s.WorkingDir)
	outcomes, _ := metricsStore.ReadOutcomes()

	result := make([]ProviderSummary, 0, len(registry.Names()))
	for _, name := range registry.Names() {
		harness, _ := registry.Get(name)
		st := statusByName[name]
		signal := runner.LoadRoutingSignalSnapshot(name, now)
		result = append(result, buildProviderSummary(name, harness, st, signal, outcomes, now))
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
	registry := agent.NewRegistry()
	harness, ok := registry.Get(harnessName)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "harness not found"})
		return
	}

	now := time.Now().UTC()
	runner := s.workers.buildAgentRunner(s.WorkingDir)

	statuses := registry.Discover()
	var harnessStatus agent.HarnessStatus
	for _, st := range statuses {
		if st.Name == harnessName {
			harnessStatus = st
			break
		}
	}

	signal := runner.LoadRoutingSignalSnapshot(harnessName, now)
	metricsStore := metricsStoreFromRunner(runner, s.WorkingDir)
	outcomes, _ := metricsStore.ReadOutcomes()
	burnSummaries, _ := metricsStore.ReadBurnSummaries()

	detail := buildProviderDetail(harnessName, harness, harnessStatus, signal, outcomes, burnSummaries, now)
	writeJSON(w, http.StatusOK, detail)
}

// ---- MCP tool implementations ----

func (s *Server) mcpProviderList() mcpToolResult {
	now := time.Now().UTC()
	registry := agent.NewRegistry()
	runner := s.workers.buildAgentRunner(s.WorkingDir)

	statuses := registry.Discover()
	statusByName := make(map[string]agent.HarnessStatus, len(statuses))
	for _, st := range statuses {
		statusByName[st.Name] = st
	}

	metricsStore := metricsStoreFromRunner(runner, s.WorkingDir)
	outcomes, _ := metricsStore.ReadOutcomes()

	result := make([]ProviderSummary, 0, len(registry.Names()))
	for _, name := range registry.Names() {
		harness, _ := registry.Get(name)
		st := statusByName[name]
		signal := runner.LoadRoutingSignalSnapshot(name, now)
		result = append(result, buildProviderSummary(name, harness, st, signal, outcomes, now))
	}
	data, err := json.Marshal(result)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
	}
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpProviderShow(harnessName string) mcpToolResult {
	if harnessName == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "harness required"}}, IsError: true}
	}
	registry := agent.NewRegistry()
	harness, ok := registry.Get(harnessName)
	if !ok {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "harness not found: " + harnessName}}, IsError: true}
	}

	now := time.Now().UTC()
	runner := s.workers.buildAgentRunner(s.WorkingDir)

	statuses := registry.Discover()
	var harnessStatus agent.HarnessStatus
	for _, st := range statuses {
		if st.Name == harnessName {
			harnessStatus = st
			break
		}
	}

	signal := runner.LoadRoutingSignalSnapshot(harnessName, now)
	metricsStore := metricsStoreFromRunner(runner, s.WorkingDir)
	outcomes, _ := metricsStore.ReadOutcomes()
	burnSummaries, _ := metricsStore.ReadBurnSummaries()

	detail := buildProviderDetail(harnessName, harness, harnessStatus, signal, outcomes, burnSummaries, now)
	data, err := json.Marshal(detail)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "{}"}}}
	}
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

// ---- Build helpers ----

func buildProviderSummary(
	name string,
	harness agent.Harness,
	st agent.HarnessStatus,
	signal agent.RoutingSignalSnapshot,
	outcomes []agent.RoutingOutcome,
	now time.Time,
) ProviderSummary {
	harnessOutcomes := filterProviderOutcomes(outcomes, name, now, 7)
	perf := computeProviderPerformance(harnessOutcomes)
	sources := collectProviderSignalSources(signal, len(harnessOutcomes) > 0)

	var freshnessTS string
	if ts := providerFreshnessTS(signal, harnessOutcomes); !ts.IsZero() {
		freshnessTS = ts.UTC().Format(time.RFC3339)
	}

	return ProviderSummary{
		Harness:            name,
		DisplayName:        harnessDisplayName(name),
		Status:             providerStatusStr(st),
		AuthState:          providerAuthStateStr(signal),
		QuotaHeadroom:      providerQuotaHeadroomStr(signal),
		SignalSources:      sources,
		FreshnessTS:        freshnessTS,
		LastCheckedTS:      now.UTC().Format(time.RFC3339),
		RecentSuccessRate:  perf.SuccessRate,
		RecentLatencyP50MS: perf.P50LatencyMS,
		CostClass:          harnessCosCostClassStr(harness),
	}
}

func buildProviderDetail(
	name string,
	harness agent.Harness,
	st agent.HarnessStatus,
	signal agent.RoutingSignalSnapshot,
	outcomes []agent.RoutingOutcome,
	burnSummaries []agent.BurnSummary,
	now time.Time,
) ProviderDetail {
	outcomes7d := filterProviderOutcomes(outcomes, name, now, 7)
	outcomes30d := filterProviderOutcomes(outcomes, name, now, 30)
	perf := computeProviderPerformance(outcomes7d)
	sources := collectProviderSignalSources(signal, len(outcomes7d) > 0)

	var freshnessTS string
	if ts := providerFreshnessTS(signal, outcomes7d); !ts.IsZero() {
		freshnessTS = ts.UTC().Format(time.RFC3339)
	}

	models := buildModelQuotaList(harness, signal)
	historicalUsage := computeProviderHistoricalUsage(harness, signal, outcomes7d, outcomes30d)
	burnEstimate := computeProviderBurnEstimate(name, harness, burnSummaries, historicalUsage, signal)

	statusStr := providerStatusStr(st)
	return ProviderDetail{
		Harness:         name,
		DisplayName:     harnessDisplayName(name),
		Status:          statusStr,
		AuthState:       providerAuthStateStr(signal),
		Models:          models,
		HistoricalUsage: historicalUsage,
		BurnEstimate:    burnEstimate,
		RoutingSignals: ProviderRoutingSignals{
			Availability: statusStr,
			RequestFit:   providerRequestFitStr(st),
			CostEstimate: "unknown",
			Performance:  perf,
		},
		SignalSources: sources,
		FreshnessTS:   freshnessTS,
	}
}

func buildModelQuotaList(harness agent.Harness, signal agent.RoutingSignalSnapshot) []ProviderModelQuota {
	models := harness.Models
	if len(models) == 0 && harness.DefaultModel != "" {
		models = []string{harness.DefaultModel}
	}
	if len(models) == 0 {
		return []ProviderModelQuota{}
	}

	quotaState := providerQuotaHeadroomStr(signal)
	sourceEnum := signalSourceAPIEnum(signal.Source.Kind)
	var sourceNote string
	if quotaState == "unknown" && harness.Name == "claude" {
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
	harness agent.Harness,
	signal agent.RoutingSignalSnapshot,
	outcomes7d []agent.RoutingOutcome,
	outcomes30d []agent.RoutingOutcome,
) *ProviderHistoricalUsage {
	window7d := usageWindowFromSignalOrOutcomes(harness, signal, outcomes7d)
	window30d := usageWindowFromOutcomes(harness, outcomes30d)
	if window7d == nil && window30d == nil {
		return nil
	}
	return &ProviderHistoricalUsage{
		Window7D:  window7d,
		Window30D: window30d,
	}
}

func usageWindowFromSignalOrOutcomes(
	harness agent.Harness,
	signal agent.RoutingSignalSnapshot,
	outcomes []agent.RoutingOutcome,
) *ProviderUsageWindow {
	// Prefer provider-native signal if it has token data.
	if signal.HistoricalUsage.TotalTokens > 0 || signal.HistoricalUsage.InputTokens > 0 {
		w := &ProviderUsageWindow{
			InputTokens:  signal.HistoricalUsage.InputTokens,
			OutputTokens: signal.HistoricalUsage.OutputTokens,
			TotalTokens:  signal.HistoricalUsage.TotalTokens,
		}
		if harness.IsSubscription || harness.IsLocal {
			w.CostUSD = 0
			if harness.IsSubscription {
				w.CostNote = "subscription plan; per-token cost not billed"
			}
		} else {
			w.CostUSD = -1
		}
		return w
	}
	return usageWindowFromOutcomes(harness, outcomes)
}

func usageWindowFromOutcomes(harness agent.Harness, outcomes []agent.RoutingOutcome) *ProviderUsageWindow {
	if len(outcomes) == 0 {
		return nil
	}
	var inTok, outTok int
	var costUSD float64
	hasCost := false
	for _, o := range outcomes {
		inTok += o.InputTokens
		outTok += o.OutputTokens
		if o.CostUSD > 0 {
			costUSD += o.CostUSD
			hasCost = true
		}
	}
	if inTok == 0 && outTok == 0 {
		return nil
	}
	w := &ProviderUsageWindow{
		InputTokens:  inTok,
		OutputTokens: outTok,
		TotalTokens:  inTok + outTok,
	}
	if harness.IsSubscription || harness.IsLocal {
		w.CostUSD = 0
		if harness.IsSubscription {
			w.CostNote = "subscription plan; per-token cost not billed"
		}
	} else if hasCost {
		w.CostUSD = costUSD
	} else {
		w.CostUSD = -1
	}
	return w
}

func computeProviderBurnEstimate(
	name string,
	harness agent.Harness,
	burnSummaries []agent.BurnSummary,
	usage *ProviderHistoricalUsage,
	signal agent.RoutingSignalSnapshot,
) *ProviderBurnEstimate {
	if !harness.IsSubscription {
		return nil
	}

	// Find the most recent burn summary for this harness.
	var latestBurn *agent.BurnSummary
	for i := range burnSummaries {
		if burnSummaries[i].Harness == name {
			if latestBurn == nil || burnSummaries[i].ObservedAt.After(latestBurn.ObservedAt) {
				latestBurn = &burnSummaries[i]
			}
		}
	}

	// Derive daily token rate from 7d window (DDx-observed or provider-native).
	dailyTokenRate := -1.0
	if usage != nil && usage.Window7D != nil && usage.Window7D.TotalTokens > 0 {
		dailyTokenRate = float64(usage.Window7D.TotalTokens) / 7.0
	}

	// Require at least one data source before emitting a burn estimate.
	if latestBurn == nil && dailyTokenRate < 0 {
		return nil
	}

	// Determine source attribution.
	sourceEnum := signalSourceAPIEnum(signal.Source.Kind)
	var sourceStr string
	if latestBurn != nil && dailyTokenRate >= 0 {
		if sourceEnum != "none" {
			sourceStr = sourceEnum + "+ddx-metrics"
		} else {
			sourceStr = "ddx-metrics"
		}
	} else if latestBurn != nil {
		if sourceEnum != "none" {
			sourceStr = sourceEnum
		} else {
			sourceStr = "ddx-metrics"
		}
	} else {
		sourceStr = "ddx-metrics"
	}

	subscriptionBurn := "unknown"
	confidence := "low"
	var freshnessTS string

	if latestBurn != nil {
		switch {
		case latestBurn.BurnIndex >= 0.8:
			subscriptionBurn = "high"
		case latestBurn.BurnIndex >= 0.5:
			subscriptionBurn = "moderate"
		case latestBurn.BurnIndex >= 0.1:
			subscriptionBurn = "low"
		}
		switch {
		case latestBurn.Confidence >= 0.8:
			confidence = "high"
		case latestBurn.Confidence >= 0.5:
			confidence = "medium"
		}
		if !latestBurn.ObservedAt.IsZero() {
			freshnessTS = latestBurn.ObservedAt.UTC().Format(time.RFC3339)
		}
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

// metricsStoreFromRunner returns a RoutingMetricsStore using the runner's session log dir.
func metricsStoreFromRunner(runner *agent.Runner, workingDir string) *agent.RoutingMetricsStore {
	logDir := runner.Config.SessionLogDir
	if logDir == "" {
		logDir = agent.DefaultLogDir
	}
	if !filepath.IsAbs(logDir) {
		logDir = filepath.Join(workingDir, logDir)
	}
	return agent.NewRoutingMetricsStore(logDir)
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

// collectProviderSignalSources builds the signal_sources array from the snapshot and metrics presence.
// Defined values per FEAT-014: native-session-jsonl, stats-cache, ddx-metrics, none.
func collectProviderSignalSources(signal agent.RoutingSignalSnapshot, hasMetrics bool) []string {
	seen := map[string]bool{}
	result := []string{}
	if signal.Source.Kind != "" {
		if enum := signalSourceAPIEnum(signal.Source.Kind); enum != "none" {
			seen[enum] = true
			result = append(result, enum)
		}
	}
	if hasMetrics && !seen["ddx-metrics"] {
		result = append(result, "ddx-metrics")
		seen["ddx-metrics"] = true
	}
	if len(result) == 0 {
		result = []string{"none"}
	}
	return result
}

// providerFreshnessTS returns the oldest contributing signal timestamp.
// Returns zero time when no signals exist (caller omits the field per FEAT-014).
func providerFreshnessTS(signal agent.RoutingSignalSnapshot, outcomes []agent.RoutingOutcome) time.Time {
	var oldest time.Time
	if !signal.Source.ObservedAt.IsZero() {
		oldest = signal.Source.ObservedAt
	}
	for _, o := range outcomes {
		if oldest.IsZero() || o.ObservedAt.Before(oldest) {
			oldest = o.ObservedAt
		}
	}
	return oldest
}

// providerStatusStr maps a HarnessStatus to the API status string.
func providerStatusStr(st agent.HarnessStatus) string {
	if st.Name == "" {
		return "unknown"
	}
	if st.Available {
		return "available"
	}
	return "unavailable"
}

// providerRequestFitStr returns request_fit from harness availability.
func providerRequestFitStr(st agent.HarnessStatus) string {
	if st.Available {
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

// harnessCosCostClassStr maps a Harness to the API cost_class string.
func harnessCosCostClassStr(harness agent.Harness) string {
	if harness.IsSubscription {
		return "subscription"
	}
	if harness.IsLocal {
		return "local"
	}
	if harness.CostClass != "" {
		return harness.CostClass
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
// Defined values: native-session-jsonl, stats-cache, ddx-metrics, none.
func signalSourceAPIEnum(kind string) string {
	switch kind {
	case "native-session-jsonl":
		return "native-session-jsonl"
	case "stats-cache", "quota-snapshot":
		return "stats-cache"
	case "http-balance", "http-models":
		return "stats-cache" // provider-reported via live HTTP API
	case "recent-session-log":
		return "ddx-metrics"
	default:
		return "none"
	}
}
