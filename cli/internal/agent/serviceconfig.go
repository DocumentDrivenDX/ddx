package agent

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	agentlib "github.com/DocumentDrivenDX/fizeau"
	// Import the configinit package for its init() side-effect: it triggers
	// agent's internal/config init which registers the config loader into
	// agentlib so that agentlib.New(ServiceOptions{ConfigPath:…}) can resolve
	// provider configuration without a separate adapter. configinit is the
	// public marker package exposed for this purpose after agent v0.5.0
	// moved internal/config out of the public surface.
	ddxconfig "github.com/DocumentDrivenDX/ddx/internal/config"
	_ "github.com/DocumentDrivenDX/fizeau/configinit"
)

// DefaultProviderRequestTimeout is the per-request wall-clock cap for
// providers. It guards against a provider that emits headers but then stalls
// entirely — a scenario the idle-read timeout below catches for streaming
// providers, but which also applies to non-streaming Chat calls where no body
// bytes arrive at all.
//
// To override per-project, set agent.endpoints.<name>.request_timeout_seconds
// in .ddx/config.yaml, or pass --request-timeout DURATION to execute-bead /
// execute-loop for one-off debugging.
const DefaultProviderRequestTimeout = 15 * time.Minute

// DefaultProviderIdleReadTimeout bounds the maximum idle gap between stream
// deltas. This is the primary stalled-TCP-socket defense: when no body bytes
// arrive for 5 continuous minutes the provider is assumed hung and the call
// fails.
const DefaultProviderIdleReadTimeout = 5 * time.Minute

// ResolveProviderRequestTimeout returns the effective wall-clock cap for a
// single Chat/ChatStream call. Resolution order:
//  1. override if > 0 (from --request-timeout CLI flag via ResolvedConfig)
//  2. agent.endpoints[n].request_timeout_seconds in .ddx/config.yaml for
//     the named provider endpoint
//  3. DefaultProviderRequestTimeout (15 min)
//
// The idle-read timeout (DefaultProviderIdleReadTimeout, 5 min) is a
// separate mechanism and is NOT affected by this function.
func ResolveProviderRequestTimeout(workDir, providerName, _ string, override time.Duration) time.Duration {
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

// NewServiceFromWorkDir constructs a FizeauService for the given DDx project.
// When .ddx/config.yaml contains agent.endpoints, those endpoint blocks are
// injected as the service config so routing is independent from global named
// provider profiles. Otherwise, ConfigPath preserves the upstream agent loader
// fallback for legacy .agent/global configuration.
func NewServiceFromWorkDir(workDir string) (agentlib.FizeauService, error) {
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
func NewStatusProbeServiceFromWorkDir(workDir string) (agentlib.FizeauService, error) {
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
	return newEndpointServiceConfigWithoutLiveFilter(cfg.Agent.Endpoints, workDir)
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

func (c *endpointServiceConfig) SessionLogDir() string {
	if c.workDir == "" {
		return ""
	}
	return filepath.Join(c.workDir, ".fizeau", "sessions")
}

func (c *endpointServiceConfig) RouteHealthPath(routeKey string) string {
	if c.workDir == "" {
		return ""
	}
	return filepath.Join(c.workDir, ".fizeau", "route-health-"+routeKey+".json")
}
