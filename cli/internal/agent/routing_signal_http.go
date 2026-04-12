package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	openrouterSourceKind = "http-balance"
	lmstudioSourceKind   = "http-models"
)

// EndpointStatus reports availability of a single HTTP inference endpoint.
type EndpointStatus struct {
	URL       string   `json:"url"`
	Available bool     `json:"available"`
	Models    []string `json:"models,omitempty"`
	LatencyMS int      `json:"latency_ms,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// ProbeOpenRouterBalance queries the OpenRouter /v1/auth/key endpoint for balance info.
// Returns a RoutingSignalSnapshot with current quota state.
func ProbeOpenRouterBalance(timeout time.Duration) RoutingSignalSnapshot {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	now := time.Now().UTC()
	unknown := RoutingSignalSnapshot{
		Provider: "openrouter",
		Source: SignalSourceMetadata{
			Provider:  "openrouter",
			Kind:      openrouterSourceKind,
			Freshness: "unknown",
		},
		CurrentQuota: QuotaSignal{
			Source: SignalSourceMetadata{
				Provider:  "openrouter",
				Kind:      openrouterSourceKind,
				Freshness: "unknown",
			},
			State: "unknown",
		},
	}

	apiKey, baseURL := resolveOpenRouterCredentials()
	if apiKey == "" {
		unknown.Source.Notes = "OPENROUTER_API_KEY not set and no key found in ~/.config/agent/config.yaml"
		unknown.CurrentQuota.Source.Notes = unknown.Source.Notes
		return unknown
	}
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	keyURL := strings.TrimRight(baseURL, "/") + "/auth/key"
	req, err := http.NewRequest("GET", keyURL, nil)
	if err != nil {
		unknown.Source.Notes = err.Error()
		return unknown
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Do(req)
	latencyMS := int(time.Since(start).Milliseconds())
	if err != nil {
		unknown.Source.Notes = err.Error()
		unknown.Source.AgeSeconds = 0
		return unknown
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		unknown.Source.Notes = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return unknown
	}

	var parsed struct {
		Data struct {
			Label          *string  `json:"label"`
			Usage          float64  `json:"usage"`
			UsageMonthly   float64  `json:"usage_monthly"`
			IsFreeTier     bool     `json:"is_free_tier"`
			Limit          *float64 `json:"limit"`
			LimitReset     string   `json:"limit_reset"`
			LimitRemaining *float64 `json:"limit_remaining"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		unknown.Source.Notes = "parse error: " + err.Error()
		return unknown
	}

	d := parsed.Data
	// Use monthly usage if available (more relevant for monthly limit).
	usage := d.UsageMonthly
	if usage == 0 {
		usage = d.Usage
	}

	state := "ok"
	notes := ""
	var usedPct int
	var resetsAt string

	if d.Limit != nil && *d.Limit > 0 {
		// Credit-limited account.
		remaining := *d.Limit - usage
		if d.LimitRemaining != nil {
			remaining = *d.LimitRemaining
		}
		usedPct = int(usage / *d.Limit * 100)
		if usedPct > 100 {
			usedPct = 100
		}
		if remaining <= 0 {
			state = "blocked"
			notes = fmt.Sprintf("$%.2f limit exhausted (%.0f%% used, resets %s)", *d.Limit, float64(usedPct), d.LimitReset)
		} else {
			if usedPct >= 80 {
				state = "ok" // still ok but approaching limit
			}
			notes = fmt.Sprintf("$%.2f used of $%.2f limit ($%.2f remaining, resets %s)", usage, *d.Limit, remaining, d.LimitReset)
		}
		resetsAt = d.LimitReset
	} else {
		// Pay-as-you-go or free tier — no cap.
		notes = fmt.Sprintf("$%.4f used (no credit limit)", usage)
	}

	label := ""
	if d.Label != nil {
		label = *d.Label
	}

	meta := SignalSourceMetadata{
		Provider:   "openrouter",
		Kind:       openrouterSourceKind,
		ObservedAt: now,
		Freshness:  "fresh",
		AgeSeconds: 0,
		Basis:      keyURL,
		Notes:      notes,
	}
	_ = latencyMS

	var windows []QuotaWindow
	if d.Limit != nil && *d.Limit > 0 {
		windows = []QuotaWindow{{
			Name:        d.LimitReset,
			LimitID:     "credit",
			UsedPercent: float64(usedPct),
			ResetsAt:    resetsAt,
			State:       state,
		}}
	}

	return RoutingSignalSnapshot{
		Provider: "openrouter",
		Source:   meta,
		Account: &AccountInfo{
			PlanType: planTypeFromOpenRouter(d.IsFreeTier, label),
		},
		CurrentQuota: QuotaSignal{
			Source:      meta,
			State:       state,
			UsedPercent: usedPct,
			ResetsAt:    resetsAt,
		},
		QuotaWindows: windows,
	}
}

func planTypeFromOpenRouter(isFreeTier bool, label string) string {
	if isFreeTier {
		return "free"
	}
	if label != "" {
		return "paid"
	}
	return "paid"
}

// ProbeLMStudioEndpoints checks if LM Studio HTTP endpoints respond to /v1/models.
// Returns one EndpointStatus per endpoint.
func ProbeLMStudioEndpoints(endpoints []string, timeout time.Duration) []EndpointStatus {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	results := make([]EndpointStatus, len(endpoints))

	type result struct {
		idx    int
		status EndpointStatus
	}
	ch := make(chan result, len(endpoints))

	for i, ep := range endpoints {
		go func(idx int, baseURL string) {
			base := strings.TrimRight(baseURL, "/")
			// Strip trailing /v1 so we can re-append /v1/models cleanly.
			base = strings.TrimSuffix(base, "/v1")
			url := base + "/v1/models"
			start := time.Now()
			resp, err := client.Get(url)
			latencyMS := int(time.Since(start).Milliseconds())
			if err != nil {
				ch <- result{idx, EndpointStatus{
					URL:   baseURL,
					Error: err.Error(),
				}}
				return
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 200 {
				ch <- result{idx, EndpointStatus{
					URL:       baseURL,
					Error:     fmt.Sprintf("HTTP %d", resp.StatusCode),
					LatencyMS: latencyMS,
				}}
				return
			}
			models := parseModelsResponse(body)
			ch <- result{idx, EndpointStatus{
				URL:       baseURL,
				Available: true,
				Models:    models,
				LatencyMS: latencyMS,
			}}
		}(i, ep)
	}

	for range endpoints {
		r := <-ch
		results[r.idx] = r.status
	}
	return results
}

func parseModelsResponse(body []byte) []string {
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// resolveOpenRouterCredentials returns the API key and base URL for OpenRouter.
// Priority: env var, then ~/.config/agent/config.yaml.
func resolveOpenRouterCredentials() (apiKey, baseURL string) {
	if key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); key != "" {
		return key, "https://openrouter.ai/api/v1"
	}
	return readOpenRouterFromAgentConfig()
}

// readOpenRouterFromAgentConfig reads openrouter credentials from ~/.config/agent/config.yaml.
// The config structure is: providers.openrouter.{api_key,base_url}
func readOpenRouterFromAgentConfig() (apiKey, baseURL string) {
	providers := readAgentConfigProviders()
	if or, ok := providers["openrouter"]; ok {
		if k, ok := or["api_key"].(string); ok {
			apiKey = strings.TrimSpace(k)
		}
		if u, ok := or["base_url"].(string); ok {
			baseURL = strings.TrimSpace(u)
		}
	}
	return apiKey, baseURL
}

// ReadLMStudioEndpointsFromAgentConfig returns base_url values for all providers
// in ~/.config/agent/config.yaml that look like local LM Studio instances
// (type openai-compat, base_url not pointing to known cloud providers).
func ReadLMStudioEndpointsFromAgentConfig() []string {
	cloudHosts := []string{"openai.com", "anthropic.com", "openrouter.ai", "cohere.com", "mistral.ai", "groq.com", "together.ai"}
	providers := readAgentConfigProviders()
	var endpoints []string
	for _, p := range providers {
		t, _ := p["type"].(string)
		u, _ := p["base_url"].(string)
		if t != "openai-compat" || u == "" {
			continue
		}
		isCloud := false
		for _, h := range cloudHosts {
			if strings.Contains(u, h) {
				isCloud = true
				break
			}
		}
		if !isCloud {
			endpoints = append(endpoints, u)
		}
	}
	return endpoints
}

// readAgentConfigProviders reads the providers map from ~/.config/agent/config.yaml.
func readAgentConfigProviders() map[string]map[string]any {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "agent", "config.yaml"))
	if err != nil {
		return nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	rawProviders, ok := raw["providers"].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]map[string]any, len(rawProviders))
	for name, v := range rawProviders {
		if m, ok := v.(map[string]any); ok {
			out[name] = m
		}
	}
	return out
}

// BuildLMStudioSignal converts endpoint probe results into a RoutingSignalSnapshot.
func BuildLMStudioSignal(presetName string, statuses []EndpointStatus, now time.Time) RoutingSignalSnapshot {
	available := 0
	for _, s := range statuses {
		if s.Available {
			available++
		}
	}

	overallState := "blocked"
	if available > 0 {
		overallState = "ok"
	}
	if len(statuses) == 0 {
		overallState = "unknown"
	}

	windows := make([]QuotaWindow, len(statuses))
	for i, s := range statuses {
		name := endpointShortName(s.URL)
		state := "blocked"
		resetsAt := s.Error
		if s.Available {
			state = "ok"
			if len(s.Models) > 0 {
				resetsAt = strings.Join(s.Models, ", ")
			} else {
				resetsAt = ""
			}
		}
		windows[i] = QuotaWindow{
			Name:     name,
			LimitID:  s.URL,
			State:    state,
			ResetsAt: resetsAt,
		}
	}

	meta := SignalSourceMetadata{
		Provider:   "lmstudio",
		Kind:       lmstudioSourceKind,
		ObservedAt: now,
		Freshness:  "fresh",
		Basis:      presetName,
		Notes:      fmt.Sprintf("%d/%d endpoints available", available, len(statuses)),
	}

	return RoutingSignalSnapshot{
		Provider: "lmstudio",
		Source:   meta,
		CurrentQuota: QuotaSignal{
			Source: meta,
			State:  overallState,
		},
		QuotaWindows: windows,
	}
}

func endpointShortName(url string) string {
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimSuffix(url, "/v1")
	url = strings.TrimSuffix(url, "/")
	return url
}
