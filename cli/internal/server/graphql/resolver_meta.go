package graphql

import (
	"context"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/persona"
)

// Personas is the resolver for the personas field.
func (r *queryResolver) Personas(ctx context.Context, projectID *string) ([]*Persona, error) {
	loader := persona.NewPersonaLoader(r.personaProjectRoot(ctx, projectID))
	ps, err := loader.ListPersonas()
	if err != nil {
		return nil, fmt.Errorf("loading personas: %w", err)
	}
	gql := make([]*Persona, len(ps))
	for i, p := range ps {
		gql[i] = personaToGQL(p)
	}
	return gql, nil
}

// Persona is the resolver for the persona field.
func (r *queryResolver) Persona(ctx context.Context, name string, projectID *string) (*Persona, error) {
	loader := persona.NewPersonaLoader(r.personaProjectRoot(ctx, projectID))
	p, err := loader.LoadPersona(name)
	if err != nil {
		return nil, nil //nolint:nilerr // not-found is represented as null
	}
	return personaToGQL(p), nil
}

// personaProjectRoot resolves the working directory for persona loading.
// When a project ID is provided, it resolves via projectRoot; otherwise
// falls back to the server's working directory.
func (r *queryResolver) personaProjectRoot(ctx context.Context, projectID *string) string {
	if projectID != nil && *projectID != "" {
		return r.projectRoot(ctx, *projectID)
	}
	return r.workingDir(ctx)
}

// PersonaByRole is the resolver for the personaByRole field.
func (r *queryResolver) PersonaByRole(ctx context.Context, role string) (*Persona, error) {
	loader := persona.NewPersonaLoader(r.workingDir(ctx))
	ps, err := loader.FindByRole(role)
	if err != nil || len(ps) == 0 {
		return nil, nil
	}
	return personaToGQL(ps[0]), nil
}

// Coordinators is the resolver for the coordinators field.
func (r *queryResolver) Coordinators(ctx context.Context) ([]*CoordinatorMetricsEntry, error) {
	return r.State.GetCoordinatorsGraphQL(), nil
}

// CoordinatorMetricsByProject is the resolver for the coordinatorMetricsByProject field.
func (r *queryResolver) CoordinatorMetricsByProject(ctx context.Context, projectRoot string) (*CoordinatorMetrics, error) {
	return r.State.GetCoordinatorMetricsByProjectGraphQL(projectRoot), nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func personaToGQL(p *persona.Persona) *Persona {
	source := p.Source
	if source == "" {
		source = persona.SourceLibrary
	}
	var filePath *string
	if p.FilePath != "" {
		fp := p.FilePath
		filePath = &fp
	}
	return &Persona{
		ID:          "persona-" + p.Name,
		Name:        p.Name,
		Roles:       p.Roles,
		Description: p.Description,
		Tags:        p.Tags,
		Content:     p.Content,
		Body:        p.Content,
		Source:      source,
		Bindings:    []*PersonaBinding{},
		FilePath:    filePath,
	}
}

func personaConnectionFrom(personas []*Persona, first *int, after *string, last *int, before *string) *PersonaConnection {
	all := make([]*PersonaEdge, len(personas))
	ids := make([]string, len(personas))
	for i, p := range personas {
		all[i] = &PersonaEdge{Node: p, Cursor: encodeStableCursor(p.ID)}
		ids[i] = p.ID
	}
	startIdx, endIdx := stablePageBounds(ids, after, before)
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
