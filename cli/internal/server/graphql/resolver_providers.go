package graphql

import (
	"context"
	"fmt"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
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
	svc, err := agent.NewServiceFromWorkDir(r.WorkingDir)
	if err != nil {
		return nil, nil //nolint:nilerr
	}

	dec, err := svc.ResolveRoute(ctx, agentlib.RouteRequest{})
	if err != nil {
		// No healthy candidate — return empty status rather than an error.
		return &DefaultRouteStatus{}, nil //nolint:nilerr
	}

	result := &DefaultRouteStatus{
		ModelRef: dec.Model,
	}
	if dec.Provider != "" {
		p := dec.Provider
		result.ResolvedProvider = &p
	}
	if dec.Model != "" {
		m := dec.Model
		result.ResolvedModel = &m
	}
	return result, nil
}
