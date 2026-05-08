package graphql

import (
	"context"
	"time"
)

// ExecutionFilter mirrors the optional filter args of Query.executions in a
// single struct so the StateProvider implementation can decode them once.
type ExecutionFilter struct {
	BeadID  string
	Verdict string
	Harness string
	Since   *time.Time
	Until   *time.Time
	Search  string
}

// ExecutionsStateProvider is the optional sub-interface a StateProvider may
// implement to back the executions/execution/executionToolCalls queries.
// Kept separate from StateProvider so test stubs that don't need executions
// continue to compile.
type ExecutionsStateProvider interface {
	GetExecutionsGraphQL(projectID string, filter ExecutionFilter) []*Execution
	GetExecutionGraphQL(id string) (*Execution, bool)
	GetExecutionToolCallsGraphQL(id string) []*ExecutionToolCall
}

// Executions is the resolver for the executions field.
func (r *queryResolver) Executions(ctx context.Context, projectID string, first *int, after *string, last *int, before *string, beadID *string, verdict *string, harness *string, since *string, until *string, search *string) (*ExecutionConnection, error) {
	provider, ok := r.State.(ExecutionsStateProvider)
	if !ok {
		return emptyExecutionConnection(), nil
	}
	filter := ExecutionFilter{}
	if beadID != nil {
		filter.BeadID = *beadID
	}
	if verdict != nil {
		filter.Verdict = *verdict
	}
	if harness != nil {
		filter.Harness = *harness
	}
	if search != nil {
		filter.Search = *search
	}
	if since != nil && *since != "" {
		if t, err := time.Parse(time.RFC3339, *since); err == nil {
			filter.Since = &t
		}
	}
	if until != nil && *until != "" {
		if t, err := time.Parse(time.RFC3339, *until); err == nil {
			filter.Until = &t
		}
	}
	all := provider.GetExecutionsGraphQL(projectID, filter)
	return executionConnectionFrom(all, first, after, last, before), nil
}

// ExecutionBySessionID is the resolver for the executionBySessionId field.
func (r *queryResolver) ExecutionBySessionID(ctx context.Context, projectID string, sessionID string) (*Execution, error) {
	provider, ok := r.State.(ExecutionsStateProvider)
	if !ok || projectID == "" || sessionID == "" {
		return nil, nil
	}
	all := provider.GetExecutionsGraphQL(projectID, ExecutionFilter{})
	for _, e := range all {
		if e.SessionID != nil && *e.SessionID == sessionID {
			return e, nil
		}
	}
	return nil, nil
}

// ExecutionByResultRev is the resolver for the executionByResultRev field.
func (r *queryResolver) ExecutionByResultRev(ctx context.Context, projectID string, sha string) (*Execution, error) {
	provider, ok := r.State.(ExecutionsStateProvider)
	if !ok || projectID == "" || sha == "" {
		return nil, nil
	}
	all := provider.GetExecutionsGraphQL(projectID, ExecutionFilter{})
	for _, e := range all {
		if e.ResultRev != nil && *e.ResultRev == sha {
			return e, nil
		}
	}
	return nil, nil
}

// Execution is the resolver for the execution field.
func (r *queryResolver) Execution(ctx context.Context, id string) (*Execution, error) {
	provider, ok := r.State.(ExecutionsStateProvider)
	if !ok {
		return nil, nil
	}
	exec, ok := provider.GetExecutionGraphQL(id)
	if !ok {
		return nil, nil
	}
	return exec, nil
}

// ExecutionToolCalls is the resolver for the executionToolCalls field.
func (r *queryResolver) ExecutionToolCalls(ctx context.Context, id string, first *int, after *string) (*ExecutionToolCallConnection, error) {
	provider, ok := r.State.(ExecutionsStateProvider)
	if !ok {
		return emptyToolCallConnection(), nil
	}
	calls := provider.GetExecutionToolCallsGraphQL(id)
	return toolCallConnectionFrom(calls, first, after), nil
}

func emptyExecutionConnection() *ExecutionConnection {
	return &ExecutionConnection{Edges: []*ExecutionEdge{}, PageInfo: &PageInfo{}, TotalCount: 0}
}

func emptyToolCallConnection() *ExecutionToolCallConnection {
	return &ExecutionToolCallConnection{Edges: []*ExecutionToolCallEdge{}, PageInfo: &PageInfo{}, TotalCount: 0}
}

func executionConnectionFrom(execs []*Execution, first *int, after *string, last *int, before *string) *ExecutionConnection {
	all := make([]*ExecutionEdge, len(execs))
	ids := make([]string, len(execs))
	for i, e := range execs {
		all[i] = &ExecutionEdge{Node: e, Cursor: encodeStableCursor(e.ID)}
		ids[i] = e.ID
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
	return &ExecutionConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func toolCallConnectionFrom(calls []*ExecutionToolCall, first *int, after *string) *ExecutionToolCallConnection {
	all := make([]*ExecutionToolCallEdge, len(calls))
	ids := make([]string, len(calls))
	for i, c := range calls {
		all[i] = &ExecutionToolCallEdge{Node: c, Cursor: encodeStableCursor(c.ID)}
		ids[i] = c.ID
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
	return &ExecutionToolCallConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}
