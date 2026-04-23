package graphql

import (
	"context"
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
	GetProjectSnapshots(includeUnreachable bool) []*Project
	GetProjectSnapshotByID(id string) (*Project, bool)
	GetBeadSnapshots(status, label, projectID, search string) []BeadSnapshot
	// GetBeadSnapshotsForProject returns snapshots for a single registered
	// project without iterating any other projects' stores. Implementations
	// MUST open only the named project's bead store. The resolver for
	// Query.beadsByProject uses this path to avoid the N-project scan that
	// backs the cross-project GetBeadSnapshots call (ddx-9ce6842a).
	GetBeadSnapshotsForProject(projectID, status, label, search string) []BeadSnapshot
	GetBeadSnapshot(id string) (*BeadSnapshot, bool)

	// Worker queries
	GetWorkersGraphQL(projectID string) []*Worker
	GetWorkerGraphQL(id string) (*Worker, bool)
	GetWorkerLogGraphQL(id string) *WorkerLog
	GetWorkerProgressGraphQL(id string) []*PhaseTransition
	GetWorkerPromptGraphQL(id string) string

	// AgentSession queries
	GetAgentSessionsGraphQL(startedAfter, startedBefore *time.Time) []*AgentSession
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
	if strings.HasPrefix(id, "node-") {
		snap := r.State.GetNodeSnapshot()
		if snap.ID != id {
			return nil, nil
		}
		return nodeInfoFromSnapshot(snap), nil
	}
	if strings.HasPrefix(id, "proj-") {
		proj, ok := r.State.GetProjectSnapshotByID(id)
		if !ok {
			return nil, nil
		}
		return proj, nil
	}
	return nil, nil
}

// NodeInfo is the resolver for the nodeInfo field.
func (r *queryResolver) NodeInfo(ctx context.Context) (*NodeInfo, error) {
	snap := r.State.GetNodeSnapshot()
	return nodeInfoFromSnapshot(snap), nil
}

// Projects is the resolver for the projects field.
func (r *queryResolver) Projects(ctx context.Context, first *int, after *string, last *int, before *string, includeUnreachable *bool) (*ProjectConnection, error) {
	showAll := includeUnreachable != nil && *includeUnreachable
	projects := r.State.GetProjectSnapshots(showAll)

	// Build full edge list with stable ID-based cursors.
	all := make([]*ProjectEdge, len(projects))
	for i, p := range projects {
		all[i] = &ProjectEdge{
			Node:   p,
			Cursor: encodeStableCursor(p.ID),
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
