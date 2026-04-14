package graphql

import (
	"context"
	"fmt"
)

// Workers is the resolver for the workers field.
func (r *queryResolver) Workers(ctx context.Context, first *int, after *string, last *int, before *string) (*WorkerConnection, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	workers := r.State.GetWorkersGraphQL("")
	return workerConnectionFrom(workers, first, after, last, before), nil
}

// WorkersByProject is the resolver for the workersByProject field.
func (r *queryResolver) WorkersByProject(ctx context.Context, projectID string, first *int, after *string, last *int, before *string) (*WorkerConnection, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	workers := r.State.GetWorkersGraphQL(projectID)
	return workerConnectionFrom(workers, first, after, last, before), nil
}

// Worker is the resolver for the worker field.
func (r *queryResolver) Worker(ctx context.Context, id string) (*Worker, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	w, ok := r.State.GetWorkerGraphQL(id)
	if !ok {
		return nil, nil
	}
	return w, nil
}

// WorkerProgress is the resolver for the workerProgress field.
func (r *queryResolver) WorkerProgress(ctx context.Context, workerID string) ([]*PhaseTransition, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	return r.State.GetWorkerProgressGraphQL(workerID), nil
}

// WorkerLog is the resolver for the workerLog field.
func (r *queryResolver) WorkerLog(ctx context.Context, workerID string) (*WorkerLog, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	return r.State.GetWorkerLogGraphQL(workerID), nil
}

// WorkerPrompt is the resolver for the workerPrompt field.
func (r *queryResolver) WorkerPrompt(ctx context.Context, workerID string) (string, error) {
	if r.State == nil {
		return "", fmt.Errorf("state provider not configured")
	}
	return r.State.GetWorkerPromptGraphQL(workerID), nil
}

// AgentSessions is the resolver for the agentSessions field.
func (r *queryResolver) AgentSessions(ctx context.Context, first *int, after *string, last *int, before *string) (*AgentSessionConnection, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	sessions := r.State.GetAgentSessionsGraphQL()
	return agentSessionConnectionFrom(sessions, first, after, last, before), nil
}

// AgentSession is the resolver for the agentSession field.
func (r *queryResolver) AgentSession(ctx context.Context, id string) (*AgentSession, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	s, ok := r.State.GetAgentSessionGraphQL(id)
	if !ok {
		return nil, nil
	}
	return s, nil
}

// ExecDefinitions is the resolver for the execDefinitions field.
func (r *queryResolver) ExecDefinitions(ctx context.Context, first *int, after *string, last *int, before *string, artifactID *string) (*ExecutionDefinitionConnection, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	artifactIDVal := ""
	if artifactID != nil {
		artifactIDVal = *artifactID
	}
	defs := r.State.GetExecDefinitionsGraphQL(artifactIDVal)
	return execDefinitionConnectionFrom(defs, first, after, last, before), nil
}

// ExecDefinition is the resolver for the execDefinition field.
func (r *queryResolver) ExecDefinition(ctx context.Context, id string) (*ExecutionDefinition, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	d, ok := r.State.GetExecDefinitionGraphQL(id)
	if !ok {
		return nil, nil
	}
	return d, nil
}

// ExecRuns is the resolver for the execRuns field.
func (r *queryResolver) ExecRuns(ctx context.Context, first *int, after *string, last *int, before *string, artifactID *string, definitionID *string) (*ExecutionRunConnection, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
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
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	run, ok := r.State.GetExecRunGraphQL(id)
	if !ok {
		return nil, nil
	}
	return run, nil
}

// ExecRunLog is the resolver for the execRunLog field.
func (r *queryResolver) ExecRunLog(ctx context.Context, runID string) (*ExecutionRunLog, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	return r.State.GetExecRunLogGraphQL(runID), nil
}

// ─── connection helpers ───────────────────────────────────────────────────────

// pageBounds computes the [start, end) slice indices for Relay-style cursor pagination.
func pageBounds(total int, after, before *string) (startIdx, endIdx int) {
	startIdx = 0
	endIdx = total
	if after != nil {
		if idx, ok := decodeCursor(*after); ok {
			startIdx = idx + 1
		}
	}
	if before != nil {
		if idx, ok := decodeCursor(*before); ok && idx < endIdx {
			endIdx = idx
		}
	}
	if startIdx > endIdx {
		startIdx = endIdx
	}
	return startIdx, endIdx
}

func workerConnectionFrom(workers []*Worker, first *int, after *string, last *int, before *string) *WorkerConnection {
	all := make([]*WorkerEdge, len(workers))
	for i, w := range workers {
		all[i] = &WorkerEdge{Node: w, Cursor: encodeCursor(i)}
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
	return &WorkerConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func agentSessionConnectionFrom(sessions []*AgentSession, first *int, after *string, last *int, before *string) *AgentSessionConnection {
	all := make([]*AgentSessionEdge, len(sessions))
	for i, s := range sessions {
		all[i] = &AgentSessionEdge{Node: s, Cursor: encodeCursor(i)}
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
	return &AgentSessionConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func execDefinitionConnectionFrom(defs []*ExecutionDefinition, first *int, after *string, last *int, before *string) *ExecutionDefinitionConnection {
	all := make([]*ExecutionDefinitionEdge, len(defs))
	for i, d := range defs {
		all[i] = &ExecutionDefinitionEdge{Node: d, Cursor: encodeCursor(i)}
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
	return &ExecutionDefinitionConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func execRunConnectionFrom(runs []*ExecutionRun, first *int, after *string, last *int, before *string) *ExecutionRunConnection {
	all := make([]*ExecutionRunEdge, len(runs))
	for i, r := range runs {
		all[i] = &ExecutionRunEdge{Node: r, Cursor: encodeCursor(i)}
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
	return &ExecutionRunConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}
