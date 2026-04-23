package graphql

import (
	"context"
	"time"
)

// Workers is the resolver for the workers field.
func (r *queryResolver) Workers(ctx context.Context, first *int, after *string, last *int, before *string) (*WorkerConnection, error) {
	workers := r.State.GetWorkersGraphQL("")
	return workerConnectionFrom(workers, first, after, last, before), nil
}

// WorkersByProject is the resolver for the workersByProject field.
func (r *queryResolver) WorkersByProject(ctx context.Context, projectID string, first *int, after *string, last *int, before *string) (*WorkerConnection, error) {
	workers := r.State.GetWorkersGraphQL(projectID)
	return workerConnectionFrom(workers, first, after, last, before), nil
}

// Worker is the resolver for the worker field.
func (r *queryResolver) Worker(ctx context.Context, id string) (*Worker, error) {
	w, ok := r.State.GetWorkerGraphQL(id)
	if !ok {
		return nil, nil
	}
	return w, nil
}

// WorkerProgress is the resolver for the workerProgress field.
func (r *queryResolver) WorkerProgress(ctx context.Context, workerID string) ([]*PhaseTransition, error) {
	return r.State.GetWorkerProgressGraphQL(workerID), nil
}

// WorkerLog is the resolver for the workerLog field.
func (r *queryResolver) WorkerLog(ctx context.Context, workerID string) (*WorkerLog, error) {
	return r.State.GetWorkerLogGraphQL(workerID), nil
}

// WorkerPrompt is the resolver for the workerPrompt field.
func (r *queryResolver) WorkerPrompt(ctx context.Context, workerID string) (string, error) {
	return r.State.GetWorkerPromptGraphQL(workerID), nil
}

// AgentSessions is the resolver for the agentSessions field.
func (r *queryResolver) AgentSessions(ctx context.Context, first *int, after *string, last *int, before *string, startedAfter *string, startedBefore *string) (*AgentSessionConnection, error) {
	var afterTime *time.Time
	var beforeTime *time.Time
	if startedAfter != nil && *startedAfter != "" {
		if parsed, err := time.Parse(time.RFC3339, *startedAfter); err == nil {
			afterTime = &parsed
		}
	}
	if startedBefore != nil && *startedBefore != "" {
		if parsed, err := time.Parse(time.RFC3339, *startedBefore); err == nil {
			beforeTime = &parsed
		}
	}
	sessions := r.State.GetAgentSessionsGraphQL(afterTime, beforeTime)
	return agentSessionConnectionFrom(sessions, first, after, last, before), nil
}

// AgentSession is the resolver for the agentSession field.
func (r *queryResolver) AgentSession(ctx context.Context, id string) (*AgentSession, error) {
	s, ok := r.State.GetAgentSessionGraphQL(id)
	if !ok {
		return nil, nil
	}
	return s, nil
}

// ExecDefinitions is the resolver for the execDefinitions field.
func (r *queryResolver) ExecDefinitions(ctx context.Context, first *int, after *string, last *int, before *string, artifactID *string) (*ExecutionDefinitionConnection, error) {
	artifactIDVal := ""
	if artifactID != nil {
		artifactIDVal = *artifactID
	}
	defs := r.State.GetExecDefinitionsGraphQL(artifactIDVal)
	return execDefinitionConnectionFrom(defs, first, after, last, before), nil
}

// ExecDefinition is the resolver for the execDefinition field.
func (r *queryResolver) ExecDefinition(ctx context.Context, id string) (*ExecutionDefinition, error) {
	d, ok := r.State.GetExecDefinitionGraphQL(id)
	if !ok {
		return nil, nil
	}
	return d, nil
}

// ExecRuns is the resolver for the execRuns field.
func (r *queryResolver) ExecRuns(ctx context.Context, first *int, after *string, last *int, before *string, artifactID *string, definitionID *string) (*ExecutionRunConnection, error) {
	artifactIDVal := ""
	if artifactID != nil {
		artifactIDVal = *artifactID
	}
	definitionIDVal := ""
	if definitionID != nil {
		definitionIDVal = *definitionID
	}
	runs := r.State.GetExecRunsGraphQL(artifactIDVal, definitionIDVal)
	return execRunConnectionFrom(runs, first, after, last, before), nil
}

// ExecRun is the resolver for the execRun field.
func (r *queryResolver) ExecRun(ctx context.Context, id string) (*ExecutionRun, error) {
	run, ok := r.State.GetExecRunGraphQL(id)
	if !ok {
		return nil, nil
	}
	return run, nil
}

// ExecRunLog is the resolver for the execRunLog field.
func (r *queryResolver) ExecRunLog(ctx context.Context, runID string) (*ExecutionRunLog, error) {
	return r.State.GetExecRunLogGraphQL(runID), nil
}

// ─── connection helpers ───────────────────────────────────────────────────────

// stablePageBounds computes the [startIdx, endIdx) slice window using stable
// ID-based cursors. ids must contain the stable key for each edge in order.
func stablePageBounds(ids []string, after, before *string) (startIdx, endIdx int) {
	startIdx = 0
	endIdx = len(ids)
	if after != nil {
		if afterID, ok := decodeStableCursor(*after); ok {
			for i, id := range ids {
				if id == afterID {
					startIdx = i + 1
					break
				}
			}
		}
	}
	if before != nil {
		if beforeID, ok := decodeStableCursor(*before); ok {
			for i, id := range ids {
				if id == beforeID {
					endIdx = i
					break
				}
			}
		}
	}
	if startIdx > endIdx {
		startIdx = endIdx
	}
	return startIdx, endIdx
}

func workerConnectionFrom(workers []*Worker, first *int, after *string, last *int, before *string) *WorkerConnection {
	all := make([]*WorkerEdge, len(workers))
	ids := make([]string, len(workers))
	for i, w := range workers {
		all[i] = &WorkerEdge{Node: w, Cursor: encodeStableCursor(w.ID)}
		ids[i] = w.ID
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
	return &WorkerConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func agentSessionConnectionFrom(sessions []*AgentSession, first *int, after *string, last *int, before *string) *AgentSessionConnection {
	all := make([]*AgentSessionEdge, len(sessions))
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		all[i] = &AgentSessionEdge{Node: s, Cursor: encodeStableCursor(s.ID)}
		ids[i] = s.ID
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
	return &AgentSessionConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func execDefinitionConnectionFrom(defs []*ExecutionDefinition, first *int, after *string, last *int, before *string) *ExecutionDefinitionConnection {
	all := make([]*ExecutionDefinitionEdge, len(defs))
	ids := make([]string, len(defs))
	for i, d := range defs {
		all[i] = &ExecutionDefinitionEdge{Node: d, Cursor: encodeStableCursor(d.ID)}
		ids[i] = d.ID
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
	return &ExecutionDefinitionConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func execRunConnectionFrom(runs []*ExecutionRun, first *int, after *string, last *int, before *string) *ExecutionRunConnection {
	all := make([]*ExecutionRunEdge, len(runs))
	ids := make([]string, len(runs))
	for i, r := range runs {
		all[i] = &ExecutionRunEdge{Node: r, Cursor: encodeStableCursor(r.ID)}
		ids[i] = r.ID
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
	return &ExecutionRunConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}
