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

// DefaultProviderRequestTimeout bounds a single Chat / ChatStream call.
// Defeats RC4 of ddx-0a651925: a stalled TCP socket that has delivered
// headers but stopped emitting body bytes would otherwise pin a goroutine
// until the outer wall-clock (3h) frees it.
const DefaultProviderRequestTimeout = 15 * time.Minute

// DefaultProviderIdleReadTimeout bounds the maximum idle gap between stream
// deltas. Used by service callers to bound idle reads on streaming providers.
const DefaultProviderIdleReadTimeout = 5 * time.Minute

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
