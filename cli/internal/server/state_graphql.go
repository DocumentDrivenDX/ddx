package server

import (
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// GetNodeSnapshot implements ddxgraphql.StateProvider.
func (s *ServerState) GetNodeSnapshot() ddxgraphql.NodeStateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ddxgraphql.NodeStateSnapshot{
		ID:        s.Node.ID,
		Name:      s.Node.Name,
		StartedAt: s.Node.StartedAt,
		LastSeen:  s.Node.LastSeen,
	}
}

// GetProjectSnapshots implements ddxgraphql.StateProvider.
func (s *ServerState) GetProjectSnapshots(includeUnreachable bool) []ddxgraphql.ProjectSnapshot {
	entries := s.GetProjects(includeUnreachable)
	snaps := make([]ddxgraphql.ProjectSnapshot, len(entries))
	for i, e := range entries {
		snaps[i] = projectEntryToSnapshot(e)
	}
	return snaps
}

// GetProjectSnapshotByID implements ddxgraphql.StateProvider.
func (s *ServerState) GetProjectSnapshotByID(id string) (ddxgraphql.ProjectSnapshot, bool) {
	entry, ok := s.GetProjectByID(id)
	if !ok {
		return ddxgraphql.ProjectSnapshot{}, false
	}
	return projectEntryToSnapshot(entry), true
}

func projectEntryToSnapshot(e ProjectEntry) ddxgraphql.ProjectSnapshot {
	return ddxgraphql.ProjectSnapshot{
		ID:           e.ID,
		Name:         e.Name,
		Path:         e.Path,
		GitRemote:    e.GitRemote,
		RegisteredAt: e.RegisteredAt,
		LastSeen:     e.LastSeen,
		Unreachable:  e.Unreachable,
		TombstonedAt: e.TombstonedAt,
	}
}
