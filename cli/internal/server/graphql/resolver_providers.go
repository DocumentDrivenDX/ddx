package graphql

import (
	"context"
	"fmt"
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
	svc, err := agent.NewServiceFromWorkDir(r.WorkingDir)
	if err != nil {
		// If agent config is missing, return empty list rather than an error.
		return []*ProviderStatus{}, nil //nolint:nilerr
	}

	providers, err := svc.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing providers: %w", err)
	}

	results := make([]*ProviderStatus, 0, len(providers))
	for _, p := range providers {
		url := p.BaseURL
		if url == "" {
			url = "(api)"
		}
		ps := &ProviderStatus{
			Name:         p.Name,
			ProviderType: p.Type,
			BaseURL:      url,
			Model:        p.DefaultModel,
			Status:       p.Status,
			ModelCount:   p.ModelCount,
			IsDefault:    p.IsDefault,
		}
		if p.CooldownState != nil && !p.CooldownState.Until.IsZero() {
			s := p.CooldownState.Until.UTC().Format(time.RFC3339)
			ps.CooldownUntil = &s
		}
		results = append(results, ps)
	}

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
