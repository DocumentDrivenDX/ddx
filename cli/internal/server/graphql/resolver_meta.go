package graphql

import (
	"context"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/persona"
)

// Personas is the resolver for the personas field.
func (r *queryResolver) Personas(ctx context.Context, first *int, after *string, last *int, before *string) (*PersonaConnection, error) {
	loader := persona.NewPersonaLoader(r.WorkingDir)
	ps, err := loader.ListPersonas()
	if err != nil {
		return nil, fmt.Errorf("loading personas: %w", err)
	}
	gql := make([]*Persona, len(ps))
	for i, p := range ps {
		gql[i] = personaToGQL(p)
	}
	return personaConnectionFrom(gql, first, after, last, before), nil
}

// Persona is the resolver for the persona field.
func (r *queryResolver) Persona(ctx context.Context, name string) (*Persona, error) {
	loader := persona.NewPersonaLoader(r.WorkingDir)
	p, err := loader.LoadPersona(name)
	if err != nil {
		return nil, nil //nolint:nilerr // not-found is represented as null
	}
	return personaToGQL(p), nil
}

// PersonaByRole is the resolver for the personaByRole field.
func (r *queryResolver) PersonaByRole(ctx context.Context, role string) (*Persona, error) {
	loader := persona.NewPersonaLoader(r.WorkingDir)
	ps, err := loader.FindByRole(role)
	if err != nil || len(ps) == 0 {
		return nil, nil
	}
	return personaToGQL(ps[0]), nil
}

// Coordinators is the resolver for the coordinators field.
func (r *queryResolver) Coordinators(ctx context.Context) ([]*CoordinatorMetricsEntry, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	return r.State.GetCoordinatorsGraphQL(), nil
}

// CoordinatorMetricsByProject is the resolver for the coordinatorMetricsByProject field.
func (r *queryResolver) CoordinatorMetricsByProject(ctx context.Context, projectRoot string) (*CoordinatorMetrics, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	return r.State.GetCoordinatorMetricsByProjectGraphQL(projectRoot), nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func personaToGQL(p *persona.Persona) *Persona {
	return &Persona{
		ID:          "persona-" + p.Name,
		Name:        p.Name,
		Roles:       p.Roles,
		Description: p.Description,
		Tags:        p.Tags,
		Content:     p.Content,
	}
}

func personaConnectionFrom(personas []*Persona, first *int, after *string, last *int, before *string) *PersonaConnection {
	all := make([]*PersonaEdge, len(personas))
	for i, p := range personas {
		all[i] = &PersonaEdge{Node: p, Cursor: encodeCursor(i)}
	}
	startIdx, endIdx := pageBounds(len(all), after, before)
	slice := all[startIdx:endIdx]
	truncByFirst, truncByLast := false, false
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
		truncByFirst = true
	}
	if last != nil && *last >= 0 && *last < len(slice) {
		slice = slice[len(slice)-*last:]
		truncByLast = true
	}
	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0 || truncByLast,
		HasNextPage:     endIdx < len(all) || truncByFirst,
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}
	return &PersonaConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}
