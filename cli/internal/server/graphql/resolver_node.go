package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// NodeStateSnapshot holds node identity data for resolver consumption.
type NodeStateSnapshot struct {
	ID        string
	Name      string
	StartedAt time.Time
	LastSeen  time.Time
}

// ProjectSnapshot holds project data for resolver consumption.
type ProjectSnapshot struct {
	ID           string
	Name         string
	Path         string
	GitRemote    string
	RegisteredAt time.Time
	LastSeen     time.Time
	Unreachable  bool
	TombstonedAt *time.Time
}

// BeadDependencySnapshot holds dependency data for resolver consumption.
type BeadDependencySnapshot struct {
	IssueID     string
	DependsOnID string
	Type        string
	CreatedAt   string
	CreatedBy   string
	Metadata    string
}

// BeadSnapshot holds bead data for resolver consumption.
type BeadSnapshot struct {
	ProjectID    string
	ID           string
	Title        string
	Status       string
	Priority     int
	IssueType    string
	Owner        string
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    time.Time
	Labels       []string
	Parent       string
	Description  string
	Acceptance   string
	Notes        string
	Dependencies []BeadDependencySnapshot
}

// StateProvider is the minimal interface the node/projects resolvers need.
type StateProvider interface {
	GetNodeSnapshot() NodeStateSnapshot
	GetProjectSnapshots(includeUnreachable bool) []ProjectSnapshot
	GetProjectSnapshotByID(id string) (ProjectSnapshot, bool)
	GetBeadSnapshots(status, label, projectID string) []BeadSnapshot

	// Worker queries
	GetWorkersGraphQL(projectID string) []*Worker
	GetWorkerGraphQL(id string) (*Worker, bool)
	GetWorkerLogGraphQL(id string) *WorkerLog
	GetWorkerProgressGraphQL(id string) []*PhaseTransition
	GetWorkerPromptGraphQL(id string) string

	// AgentSession queries
	GetAgentSessionsGraphQL() []*AgentSession
	GetAgentSessionGraphQL(id string) (*AgentSession, bool)

	// Exec queries
	GetExecDefinitionsGraphQL(artifactID string) []*ExecutionDefinition
	GetExecDefinitionGraphQL(id string) (*ExecutionDefinition, bool)
	GetExecRunsGraphQL(artifactID, definitionID string) []*ExecutionRun
	GetExecRunGraphQL(id string) (*ExecutionRun, bool)
	GetExecRunLogGraphQL(runID string) *ExecutionRunLog

	// Coordinator queries
	GetCoordinatorsGraphQL() []*CoordinatorMetricsEntry
	GetCoordinatorMetricsByProjectGraphQL(projectRoot string) *CoordinatorMetrics
}

// Node is the resolver for the node(id: ID!) field (Relay lookup by global ID).
func (r *queryResolver) Node(ctx context.Context, id string) (Node, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	if strings.HasPrefix(id, "node-") {
		snap := r.State.GetNodeSnapshot()
		if snap.ID != id {
			return nil, nil
		}
		return nodeInfoFromSnapshot(snap), nil
	}
	if strings.HasPrefix(id, "proj-") {
		snap, ok := r.State.GetProjectSnapshotByID(id)
		if !ok {
			return nil, nil
		}
		return projectFromSnapshot(snap), nil
	}
	return nil, nil
}

// NodeInfo is the resolver for the nodeInfo field.
func (r *queryResolver) NodeInfo(ctx context.Context) (*NodeInfo, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	snap := r.State.GetNodeSnapshot()
	return nodeInfoFromSnapshot(snap), nil
}

// Projects is the resolver for the projects field.
func (r *queryResolver) Projects(ctx context.Context, first *int, after *string, last *int, before *string, includeUnreachable *bool) (*ProjectConnection, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}
	showAll := includeUnreachable != nil && *includeUnreachable
	snaps := r.State.GetProjectSnapshots(showAll)

	// Build full edge list with stable ID-based cursors.
	all := make([]*ProjectEdge, len(snaps))
	for i, s := range snaps {
		all[i] = &ProjectEdge{
			Node:   projectFromSnapshot(s),
			Cursor: encodeStableCursor(s.ID),
		}
	}

	// Apply window: start after `after` cursor, end before `before` cursor.
	startIdx := 0
	if after != nil {
		if afterID, ok := decodeStableCursor(*after); ok {
			for i, e := range all {
				if e.Node.ID == afterID {
					startIdx = i + 1
					break
				}
			}
		}
	}
	endIdx := len(all)
	if before != nil {
		if beforeID, ok := decodeStableCursor(*before); ok {
			for i, e := range all {
				if e.Node.ID == beforeID {
					endIdx = i
					break
				}
			}
		}
	}
	if startIdx > endIdx {
		startIdx = endIdx
	}

	slice := all[startIdx:endIdx]
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
	}
	if last != nil && *last >= 0 && *last < len(slice) {
		slice = slice[len(slice)-*last:]
	}

	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0,
		HasNextPage:     endIdx < len(all),
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}

	return &ProjectConnection{
		Edges:      slice,
		PageInfo:   pageInfo,
		TotalCount: len(all),
	}, nil
}

func nodeInfoFromSnapshot(s NodeStateSnapshot) *NodeInfo {
	return &NodeInfo{
		ID:        s.ID,
		Name:      s.Name,
		StartedAt: s.StartedAt.UTC().Format(time.RFC3339),
		LastSeen:  s.LastSeen.UTC().Format(time.RFC3339),
	}
}

func projectFromSnapshot(s ProjectSnapshot) *Project {
	p := &Project{
		ID:           s.ID,
		Name:         s.Name,
		Path:         s.Path,
		RegisteredAt: s.RegisteredAt.UTC().Format(time.RFC3339),
		LastSeen:     s.LastSeen.UTC().Format(time.RFC3339),
	}
	if s.GitRemote != "" {
		p.GitRemote = &s.GitRemote
	}
	if s.Unreachable {
		b := true
		p.Unreachable = &b
	}
	if s.TombstonedAt != nil {
		ts := s.TombstonedAt.UTC().Format(time.RFC3339)
		p.TombstonedAt = &ts
	}
	return p
}
