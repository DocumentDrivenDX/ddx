package agent

import (
	"context"
	"sync"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"

	"github.com/DocumentDrivenDX/ddx/internal/agent/providerstatus"
)

// DiscoveredProvider holds the result of probing one configured provider.
type DiscoveredProvider struct {
	Name   string
	Models []string
	Config agentconfig.ProviderConfig
}

// DiscoveryResult holds discovered models across all configured providers.
type DiscoveryResult struct {
	Providers []DiscoveredProvider
	CachedAt  time.Time
}

// ProvidersForModel returns all discovered providers that advertise the given model ID.
func (d *DiscoveryResult) ProvidersForModel(model string) []DiscoveredProvider {
	if d == nil {
		return nil
	}
	var out []DiscoveredProvider
	for _, p := range d.Providers {
		for _, m := range p.Models {
			if m == model {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// discoveryCache is a process-scoped cache for discovery results.
var discoveryCache struct {
	mu     sync.Mutex
	result *DiscoveryResult
	ttl    time.Duration
}

func init() {
	discoveryCache.ttl = 30 * time.Second
}

// DiscoverProviderModels probes all configured providers and returns their
// advertised model lists. Results are cached per-process with a 30s TTL.
// Providers that fail to respond are silently skipped (partial failure).
func DiscoverProviderModels(workDir string, timeout time.Duration) (*DiscoveryResult, error) {
	discoveryCache.mu.Lock()
	if discoveryCache.result != nil && time.Since(discoveryCache.result.CachedAt) < discoveryCache.ttl {
		cached := discoveryCache.result
		discoveryCache.mu.Unlock()
		return cached, nil
	}
	discoveryCache.mu.Unlock()

	cfg, err := agentconfig.Load(workDir)
	if err != nil {
		return &DiscoveryResult{CachedAt: time.Now()}, nil
	}
	if cfg == nil || len(cfg.ProviderNames()) == 0 {
		return &DiscoveryResult{CachedAt: time.Now()}, nil
	}

	var providers []DiscoveredProvider
	for _, name := range cfg.ProviderNames() {
		pc := cfg.Providers[name]
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		r := providerstatus.Probe(ctx, pc)
		cancel()
		if !r.Reachable || len(r.Models) == 0 {
			continue
		}
		providers = append(providers, DiscoveredProvider{
			Name:   name,
			Models: r.Models,
			Config: pc,
		})
	}

	result := &DiscoveryResult{
		Providers: providers,
		CachedAt:  time.Now(),
	}

	discoveryCache.mu.Lock()
	discoveryCache.result = result
	discoveryCache.mu.Unlock()

	return result, nil
}

// InvalidateDiscoveryCache clears the cached discovery result. Useful for tests.
func InvalidateDiscoveryCache() {
	discoveryCache.mu.Lock()
	discoveryCache.result = nil
	discoveryCache.mu.Unlock()
}
