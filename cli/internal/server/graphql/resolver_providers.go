package graphql

import (
	"context"
	"sync"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/providerstatus"
)

const (
	gqlProviderProbeTimeout = 3 * time.Second
)

// ProviderStatuses is the resolver for the providerStatuses field.
// It mirrors the output of `ddx agent providers`, probing each configured
// provider for live connectivity and returning their status.
func (r *queryResolver) ProviderStatuses(ctx context.Context) ([]*ProviderStatus, error) {
	cfg, err := agentconfig.Load(r.WorkingDir)
	if err != nil {
		// If agent config is missing, return empty list rather than an error.
		return []*ProviderStatus{}, nil //nolint:nilerr
	}

	names := cfg.ProviderNames()
	if len(names) == 0 {
		return []*ProviderStatus{}, nil
	}

	defName := cfg.DefaultName()
	healthSnap := agent.GlobalProviderHealth.Snapshot()

	// Probe all providers in parallel.
	results := make([]*ProviderStatus, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(idx int, provName string) {
			defer wg.Done()
			pc := cfg.Providers[provName]

			probeCtx, cancel := context.WithTimeout(ctx, gqlProviderProbeTimeout)
			probeResult := providerstatus.Probe(probeCtx, pc)
			cancel()

			url := pc.BaseURL
			if url == "" {
				url = "(api)"
			}

			ps := &ProviderStatus{
				Name:         provName,
				ProviderType: pc.Type,
				BaseURL:      url,
				Model:        pc.Model,
				Status:       probeResult.Message,
				ModelCount:   len(probeResult.Models),
				IsDefault:    provName == defName,
			}
			if until, ok := healthSnap[provName]; ok {
				s := until.UTC().Format(time.RFC3339)
				ps.CooldownUntil = &s
			}
			results[idx] = ps
		}(i, name)
	}
	wg.Wait()

	return results, nil
}

// DefaultRouteStatus is the resolver for the defaultRouteStatus field.
// It shows which provider/model the default model-ref resolves to, mirroring
// the Visibility 6 route-status data for the default profile.
func (r *queryResolver) DefaultRouteStatus(ctx context.Context) (*DefaultRouteStatus, error) {
	cfg, err := agentconfig.Load(r.WorkingDir)
	if err != nil {
		return nil, nil //nolint:nilerr
	}

	// Determine the default model-ref key. Prefer DefaultModelRef; fall back to DefaultModel.
	modelRef := cfg.Routing.DefaultModelRef
	if modelRef == "" {
		modelRef = cfg.Routing.DefaultModel
	}
	if modelRef == "" {
		// No default configured — return a minimal status with empty modelRef.
		return &DefaultRouteStatus{ModelRef: ""}, nil
	}

	route, ok := cfg.GetModelRoute(modelRef)
	if !ok {
		return &DefaultRouteStatus{ModelRef: modelRef}, nil
	}

	strategy := route.Strategy
	if strategy == "" {
		strategy = "first-available"
	}

	// Evaluate candidates to find the first healthy one.
	// We probe in order and take the first reachable candidate.
	var resolvedProvider, resolvedModel *string
	for _, candidate := range route.Candidates {
		pc, exists := cfg.GetProvider(candidate.Provider)
		if !exists {
			continue
		}

		model := candidate.Model
		if model == "" {
			model = pc.Model
		}

		probeCtx, cancel := context.WithTimeout(ctx, gqlProviderProbeTimeout)
		result := providerstatus.Probe(probeCtx, pc)
		cancel()

		if result.Reachable {
			p := candidate.Provider
			m := model
			resolvedProvider = &p
			resolvedModel = &m
			break
		}
	}

	s := strategy
	return &DefaultRouteStatus{
		ModelRef:         modelRef,
		ResolvedProvider: resolvedProvider,
		ResolvedModel:    resolvedModel,
		Strategy:         &s,
	}, nil
}
