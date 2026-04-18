package agent

import (
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	agentconfig "github.com/DocumentDrivenDX/agent/config"
)

// ServiceConfigAdapter wraps a loaded *agentconfig.Config so it satisfies
// the agentlib.ServiceConfig interface defined by CONTRACT-003. Used by
// every DDx command that constructs an agentlib.DdxAgent.
type ServiceConfigAdapter struct {
	Config  *agentconfig.Config
	BaseDir string
}

func (a *ServiceConfigAdapter) ProviderNames() []string {
	if a.Config == nil {
		return nil
	}
	return a.Config.ProviderNames()
}

func (a *ServiceConfigAdapter) DefaultProviderName() string {
	if a.Config == nil {
		return ""
	}
	return a.Config.DefaultName()
}

func (a *ServiceConfigAdapter) Provider(name string) (agentlib.ServiceProviderEntry, bool) {
	if a.Config == nil {
		return agentlib.ServiceProviderEntry{}, false
	}
	pc, ok := a.Config.Providers[name]
	if !ok {
		return agentlib.ServiceProviderEntry{}, false
	}
	return agentlib.ServiceProviderEntry{
		Type:    pc.Type,
		BaseURL: pc.BaseURL,
		APIKey:  pc.APIKey,
		Model:   pc.Model,
	}, true
}

func (a *ServiceConfigAdapter) ModelRouteNames() []string {
	if a.Config == nil {
		return nil
	}
	names := make([]string, 0, len(a.Config.ModelRoutes))
	for name := range a.Config.ModelRoutes {
		names = append(names, name)
	}
	return names
}

func (a *ServiceConfigAdapter) ModelRouteCandidates(routeName string) []string {
	if a.Config == nil {
		return nil
	}
	route, ok := a.Config.ModelRoutes[routeName]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(route.Candidates))
	for _, c := range route.Candidates {
		out = append(out, c.Provider)
	}
	return out
}

func (a *ServiceConfigAdapter) ModelRouteConfig(routeName string) agentlib.ServiceModelRouteConfig {
	if a.Config == nil {
		return agentlib.ServiceModelRouteConfig{}
	}
	route, ok := a.Config.ModelRoutes[routeName]
	if !ok {
		return agentlib.ServiceModelRouteConfig{}
	}
	candidates := make([]agentlib.ServiceRouteCandidateEntry, 0, len(route.Candidates))
	for _, c := range route.Candidates {
		candidates = append(candidates, agentlib.ServiceRouteCandidateEntry{
			Provider: c.Provider,
			Model:    c.Model,
			Priority: c.Priority,
		})
	}
	return agentlib.ServiceModelRouteConfig{
		Strategy:   route.Strategy,
		Candidates: candidates,
	}
}

func (a *ServiceConfigAdapter) HealthCooldown() time.Duration {
	if a.Config == nil || a.Config.Routing.HealthCooldown == "" {
		return 0
	}
	d, err := time.ParseDuration(a.Config.Routing.HealthCooldown)
	if err != nil {
		return 0
	}
	return d
}

func (a *ServiceConfigAdapter) WorkDir() string {
	return a.BaseDir
}

// NewServiceFromWorkDir loads the agent config from workDir, wraps it, and
// returns a DdxAgent ready for use. Used by every DDx command that needs to
// invoke the agent service.
func NewServiceFromWorkDir(workDir string) (agentlib.DdxAgent, error) {
	cfg, err := agentconfig.Load(workDir)
	if err != nil {
		return nil, err
	}
	adapter := &ServiceConfigAdapter{Config: cfg, BaseDir: workDir}
	return agentlib.New(agentlib.ServiceOptions{ServiceConfig: adapter})
}
