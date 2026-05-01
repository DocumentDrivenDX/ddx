package graphql

import (
	"context"
	"strings"
)

// RunsStateProvider is the optional sub-interface a StateProvider may
// implement to back the runs/run queries.
// Kept separate from StateProvider so test stubs that don't need runs
// continue to compile.
type RunsStateProvider interface {
	GetRunsGraphQL(projectID string, filter RunFilter) []*Run
	GetRunGraphQL(id string) (*Run, bool)
}

// RunFilter holds the optional filter args for Query.runs.
type RunFilter struct {
	Layer   *RunLayer
	BeadID  string
	Status  string
	Harness string
}

// Runs is the resolver for the runs field.
func (r *queryResolver) Runs(ctx context.Context, projectID *string, layer *RunLayer, beadID *string, status *string, harness *string, first *int, after *string) (*RunConnection, error) {
	provider, ok := r.State.(RunsStateProvider)
	if !ok {
		return emptyRunConnection(), nil
	}
	pid := ""
	if projectID != nil {
		pid = *projectID
	}
	filter := RunFilter{Layer: layer}
	if beadID != nil {
		filter.BeadID = *beadID
	}
	if status != nil {
		filter.Status = *status
	}
	if harness != nil {
		filter.Harness = *harness
	}
	all := provider.GetRunsGraphQL(pid, filter)
	return runConnectionFrom(all, first, after), nil
}

// Run is the resolver for the run field.
func (r *queryResolver) Run(ctx context.Context, id string) (*Run, error) {
	provider, ok := r.State.(RunsStateProvider)
	if !ok {
		return nil, nil
	}
	run, ok := provider.GetRunGraphQL(id)
	if !ok {
		return nil, nil
	}
	return run, nil
}

func emptyRunConnection() *RunConnection {
	return &RunConnection{Edges: []*RunEdge{}, PageInfo: &PageInfo{}, TotalCount: 0}
}

func runConnectionFrom(runs []*Run, first *int, after *string) *RunConnection {
	all := make([]*RunEdge, len(runs))
	ids := make([]string, len(runs))
	for i, r := range runs {
		all[i] = &RunEdge{Node: r, Cursor: encodeStableCursor(r.ID)}
		ids[i] = r.ID
	}
	startIdx, _ := stablePageBounds(ids, after, nil)
	slice := all[startIdx:]
	truncByFirst := false
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
		truncByFirst = true
	}
	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0,
		HasNextPage:     truncByFirst,
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}
	return &RunConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

// ApplyRunFilter applies the filter to a list of runs. Exported so state_runs.go can call it.
func ApplyRunFilter(in []*Run, filter RunFilter) []*Run {
	return applyRunFilter(in, filter)
}

// applyRunFilter applies the filter to a list of runs.
func applyRunFilter(in []*Run, filter RunFilter) []*Run {
	if len(in) == 0 {
		return in
	}
	if filter.Layer == nil && filter.BeadID == "" && filter.Status == "" && filter.Harness == "" {
		return in
	}
	out := in[:0:0]
	for _, r := range in {
		if filter.Layer != nil && r.Layer != *filter.Layer {
			continue
		}
		if filter.BeadID != "" {
			if r.BeadID == nil || *r.BeadID != filter.BeadID {
				continue
			}
		}
		if filter.Status != "" {
			if !strings.EqualFold(r.Status, filter.Status) {
				continue
			}
		}
		if filter.Harness != "" {
			if r.Harness == nil || *r.Harness != filter.Harness {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
