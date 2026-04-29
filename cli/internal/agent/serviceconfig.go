package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	agentlib "github.com/DocumentDrivenDX/agent"
	// Import the configinit package for its init() side-effect: it triggers
	// agent's internal/config init which registers the config loader into
	// agentlib so that agentlib.New(ServiceOptions{ConfigPath:…}) can resolve
	// provider configuration without a separate adapter. configinit is the
	// public marker package exposed for this purpose after agent v0.5.0
	// moved internal/config out of the public surface.
	_ "github.com/DocumentDrivenDX/agent/configinit"
	ddxconfig "github.com/DocumentDrivenDX/ddx/internal/config"
)

// DefaultProviderRequestTimeout is the per-request wall-clock cap for
// standard (non-thinking) providers. It guards against a provider that
// emits headers but then stalls entirely — a scenario the idle-read
// timeout below catches for streaming providers, but which also applies
// to non-streaming Chat calls where no body bytes arrive at all.
//
// Relationship between the two timeouts:
//   - DefaultProviderIdleReadTimeout (5 min): fires when no stream delta
//     arrives for 5 continuous minutes. This is the primary defense against
//     stalled TCP sockets on streaming providers.
//   - DefaultProviderRequestTimeout (15 min): wall-clock cap on the entire
//     Chat/ChatStream call. This is a secondary backstop for non-streaming
//     paths and should NOT be the primary tool for limiting thinking models,
//     since those can legitimately spend >15 min on reasoning before the
//     first delta. Use DefaultThinkingModelProviderRequestTimeout (60 min)
//     for models known to do extended chain-of-thought reasoning.
//
// To override per-project, set agent.endpoints.<name>.request_timeout_seconds
// in .ddx/config.yaml, or pass --request-timeout DURATION to execute-bead /
// execute-loop for one-off debugging.
const DefaultProviderRequestTimeout = 15 * time.Minute

// DefaultThinkingModelProviderRequestTimeout is the per-request wall-clock
// cap for known thinking / chain-of-thought reasoning models (e.g.
// qwen3.x, deepseek-r1, o1, o3). These models can spend 5–20+ minutes on
// internal reasoning before emitting the first response token, so the
// standard 15-minute cap would kill legitimate requests. The idle-read
// timeout still applies — a truly stalled stream fires after 5 min of
// silence regardless of this cap.
const DefaultThinkingModelProviderRequestTimeout = 60 * time.Minute

// DefaultProviderIdleReadTimeout bounds the maximum idle gap between stream
// deltas. This is the primary stalled-TCP-socket defense: when no body bytes
// arrive for 5 continuous minutes the provider is assumed hung and the call
// fails. Thinking models that are actively streaming reasoning tokens will
// NOT hit this timeout — they only risk DefaultThinkingModelProviderRequestTimeout
// (60 min) if a single call exceeds the wall-clock cap.
const DefaultProviderIdleReadTimeout = 5 * time.Minute

// thinkingModelPrefixes is the set of model-name substrings (lowercased)
// that identify thinking / chain-of-thought reasoning models requiring a
// longer per-request wall-clock cap. Extend this list when a new model
// class needs more than 15 minutes to produce its first response token.
var thinkingModelPrefixes = []string{
	"qwen3",      // qwen3.x series (e.g. qwen3.6-35b-a3b)
	"qwen-r1",    // Qwen R1 series
	"deepseek-r", // deepseek-r1, deepseek-r2, deepseek-reasoner
	"deepseek/r", // deepseek/r1 (openrouter format)
	"o1-",        // OpenAI o1 series (o1-mini, o1-preview)
	"o1-pro",     // OpenAI o1-pro
	"o3",         // OpenAI o3 / o3-mini
	"thinking",   // any model with "thinking" in the name
}

// isThinkingModel reports whether model is a known thinking/reasoning
// model that requires a longer per-request timeout than the standard 15 min.
func isThinkingModel(model string) bool {
	low := strings.ToLower(model)
	for _, prefix := range thinkingModelPrefixes {
		if strings.Contains(low, prefix) {
			return true
		}
	}
	return false
}

// ResolveProviderRequestTimeout returns the effective wall-clock cap for a
// single Chat/ChatStream call. Resolution order:
//  1. override if > 0 (from --request-timeout CLI flag via ResolvedConfig)
//  2. agent.endpoints[n].request_timeout_seconds in .ddx/config.yaml for
//     the named provider endpoint
//  3. DefaultThinkingModelProviderRequestTimeout (60 min) when model is a
//     known thinking/reasoning model
//  4. DefaultProviderRequestTimeout (15 min) for all other models
//
// The idle-read timeout (DefaultProviderIdleReadTimeout, 5 min) is a
// separate mechanism and is NOT affected by this function.
func ResolveProviderRequestTimeout(workDir, providerName, model string, override time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	if workDir != "" && providerName != "" {
		if cfg, err := ddxconfig.LoadWithWorkingDir(workDir); err == nil {
			if t := endpointRequestTimeout(cfg, providerName); t > 0 {
				return t
			}
		}
	}
	if isThinkingModel(model) {
		return DefaultThinkingModelProviderRequestTimeout
	}
	return DefaultProviderRequestTimeout
}

// endpointRequestTimeout looks up the request_timeout_seconds for the named
// provider in cfg.Agent.Endpoints. Returns 0 if not configured or not found.
func endpointRequestTimeout(cfg *ddxconfig.Config, providerName string) time.Duration {
	if cfg == nil || cfg.Agent == nil {
		return 0
	}
	for i, ep := range cfg.Agent.Endpoints {
		if ep.RequestTimeoutSeconds <= 0 {
			continue
		}
		name, _, err := endpointProviderEntry(ep, i)
		if err != nil {
			continue
		}
		if name == providerName {
			return time.Duration(ep.RequestTimeoutSeconds) * time.Second
		}
	}
	return 0
}

// NewServiceFromWorkDir constructs a DdxAgent for the given DDx project.
// When .ddx/config.yaml contains agent.endpoints, those endpoint blocks are
// injected as the service config so routing is independent from global named
// provider profiles. Otherwise, ConfigPath preserves the upstream agent loader
// fallback for legacy .agent/global configuration.
func NewServiceFromWorkDir(workDir string) (agentlib.DdxAgent, error) {
	opts := agentlib.ServiceOptions{
		ConfigPath: filepath.Join(workDir, "config.yaml"),
	}
	sc, err := serviceConfigFromDDxEndpoints(workDir)
	if err != nil {
		return nil, err
	}
	if sc != nil {
		opts.ServiceConfig = sc
	}
	return agentlib.New(opts)
}

// NewStatusProbeServiceFromWorkDir constructs a service for status surfaces
// without pre-filtering .ddx agent endpoints by /models reachability. The
// returned service still probes when ListProviders is called, but unreachable
// configured endpoints remain present in the result as unreachable rows.
func NewStatusProbeServiceFromWorkDir(workDir string) (agentlib.DdxAgent, error) {
	opts := agentlib.ServiceOptions{
		ConfigPath: filepath.Join(workDir, "config.yaml"),
	}
	sc, err := serviceConfigFromDDxEndpointsNoFilter(workDir)
	if err != nil {
		return nil, err
	}
	if sc != nil {
		opts.ServiceConfig = sc
	}
	return agentlib.New(opts)
}

type endpointServiceConfig struct {
	providers   map[string]agentlib.ServiceProviderEntry
	names       []string
	defaultName string
	workDir     string
}

func serviceConfigFromDDxEndpoints(workDir string) (agentlib.ServiceConfig, error) {
	cfg, err := ddxconfig.LoadWithWorkingDir(workDir)
	if err != nil {
		return nil, err
	}
	if cfg.Agent == nil || len(cfg.Agent.Endpoints) == 0 {
		return nil, nil
	}
	return newEndpointServiceConfig(context.Background(), cfg.Agent.Endpoints, workDir)
}

func serviceConfigFromDDxEndpointsNoFilter(workDir string) (agentlib.ServiceConfig, error) {
	cfg, err := ddxconfig.LoadWithWorkingDir(workDir)
	if err != nil {
		return nil, err
	}
	if cfg.Agent == nil || len(cfg.Agent.Endpoints) == 0 {
		return nil, nil
	}
	return newEndpointServiceConfigWithoutLiveFilter(cfg.Agent.Endpoints, workDir)
}

// ConfiguredProviderSnapshots returns endpoint-provider rows from .ddx config
// without probing their /models endpoints. It is used by UI surfaces that must
// first-paint from last-known or configured state and refresh live probes
// asynchronously.
func ConfiguredProviderSnapshots(workDir string) ([]agentlib.ProviderInfo, bool, error) {
	cfg, err := ddxconfig.LoadWithWorkingDir(workDir)
	if err != nil {
		return nil, false, err
	}
	if cfg.Agent == nil || len(cfg.Agent.Endpoints) == 0 {
		return nil, false, nil
	}
	out := make([]agentlib.ProviderInfo, 0, len(cfg.Agent.Endpoints))
	for i, endpoint := range cfg.Agent.Endpoints {
		name, entry, err := endpointProviderEntry(endpoint, i)
		if err != nil {
			return nil, true, err
		}
		out = append(out, agentlib.ProviderInfo{
			Name:         name,
			Type:         strings.ToLower(strings.TrimSpace(entry.Type)),
			BaseURL:      entry.BaseURL,
			Endpoints:    append([]agentlib.ServiceProviderEndpoint(nil), entry.Endpoints...),
			Status:       "unknown",
			DefaultModel: entry.Model,
			IsDefault:    i == 0,
		})
	}
	return out, true, nil
}

func newEndpointServiceConfig(ctx context.Context, endpoints []ddxconfig.AgentEndpoint, workDir string) (*endpointServiceConfig, error) {
	sc := &endpointServiceConfig{
		providers: make(map[string]agentlib.ServiceProviderEntry),
		workDir:   workDir,
	}
	for i, endpoint := range endpoints {
		name, entry, err := endpointProviderEntry(endpoint, i)
		if err != nil {
			return nil, err
		}
		if !endpointHasLiveModels(ctx, entry.BaseURL, entry.APIKey) {
			continue
		}
		sc.providers[name] = entry
		sc.names = append(sc.names, name)
		if sc.defaultName == "" {
			sc.defaultName = name
		}
	}
	return sc, nil
}

func newEndpointServiceConfigWithoutLiveFilter(endpoints []ddxconfig.AgentEndpoint, workDir string) (*endpointServiceConfig, error) {
	sc := &endpointServiceConfig{
		providers: make(map[string]agentlib.ServiceProviderEntry),
		workDir:   workDir,
	}
	for i, endpoint := range endpoints {
		name, entry, err := endpointProviderEntry(endpoint, i)
		if err != nil {
			return nil, err
		}
		sc.providers[name] = entry
		sc.names = append(sc.names, name)
		if sc.defaultName == "" {
			sc.defaultName = name
		}
	}
	return sc, nil
}

func endpointProviderEntry(endpoint ddxconfig.AgentEndpoint, index int) (string, agentlib.ServiceProviderEntry, error) {
	providerType := strings.ToLower(strings.TrimSpace(endpoint.Type))
	baseURL := strings.TrimSpace(endpoint.BaseURL)
	if baseURL == "" {
		if endpoint.Host == "" || endpoint.Port == 0 {
			return "", agentlib.ServiceProviderEntry{}, fmt.Errorf("agent.endpoints[%d]: base_url or host+port is required", index)
		}
		baseURL = fmt.Sprintf("http://%s:%d/v1", endpoint.Host, endpoint.Port)
	}
	if providerType == "" {
		providerType = inferEndpointProviderType(baseURL, endpoint.Port)
	}
	if providerType == "" {
		return "", agentlib.ServiceProviderEntry{}, fmt.Errorf("agent.endpoints[%d]: type is required when it cannot be inferred", index)
	}

	name := endpointProviderName(providerType, baseURL, endpoint, index)
	return name, agentlib.ServiceProviderEntry{
		Type:    providerType,
		BaseURL: baseURL,
		APIKey:  endpoint.APIKey,
	}, nil
}

func inferEndpointProviderType(baseURL string, port int) string {
	low := strings.ToLower(baseURL)
	switch {
	case strings.Contains(low, "openrouter.ai"):
		return "openrouter"
	case strings.Contains(low, "openai.com"):
		return "openai"
	case strings.Contains(low, ":1235") || port == 1235:
		return "omlx"
	case strings.Contains(low, ":11434") || port == 11434:
		return "ollama"
	case strings.Contains(low, ":1234") || port == 1234:
		return "lmstudio"
	default:
		return ""
	}
}

func endpointProviderName(providerType, baseURL string, endpoint ddxconfig.AgentEndpoint, index int) string {
	host := endpoint.Host
	port := endpoint.Port
	if u, err := url.Parse(baseURL); err == nil {
		if host == "" {
			host = u.Hostname()
		}
		if port == 0 {
			if p := u.Port(); p != "" {
				if n, err := strconv.Atoi(p); err == nil {
					port = n
				}
			}
		}
	}
	parts := []string{providerType}
	if host != "" {
		parts = append(parts, host)
	}
	if port != 0 {
		parts = append(parts, strconv.Itoa(port))
	}
	if len(parts) == 1 {
		parts = append(parts, strconv.Itoa(index+1))
	}
	return sanitizeEndpointName(strings.Join(parts, "-"))
}

func sanitizeEndpointName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		keep := unicode.IsLetter(r) || unicode.IsDigit(r)
		if keep {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "endpoint"
	}
	return out
}

func endpointHasLiveModels(ctx context.Context, baseURL, apiKey string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return false
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false
	}
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) != "" {
			return true
		}
	}
	return false
}

func (c *endpointServiceConfig) ProviderNames() []string {
	return append([]string(nil), c.names...)
}

func (c *endpointServiceConfig) DefaultProviderName() string {
	return c.defaultName
}

func (c *endpointServiceConfig) Provider(name string) (agentlib.ServiceProviderEntry, bool) {
	entry, ok := c.providers[name]
	return entry, ok
}

func (c *endpointServiceConfig) ModelRouteNames() []string {
	return nil
}

func (c *endpointServiceConfig) ModelRouteCandidates(string) []string {
	return nil
}

func (c *endpointServiceConfig) ModelRouteConfig(string) agentlib.ServiceModelRouteConfig {
	return agentlib.ServiceModelRouteConfig{}
}

func (c *endpointServiceConfig) HealthCooldown() time.Duration {
	return 0
}

func (c *endpointServiceConfig) WorkDir() string {
	return c.workDir
}
