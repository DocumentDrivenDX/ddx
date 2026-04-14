package server

import (
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
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

// GetBeadSnapshots implements ddxgraphql.StateProvider.
func (s *ServerState) GetBeadSnapshots(status, label, projectID string) []ddxgraphql.BeadSnapshot {
	projects := s.GetProjects()
	var result []ddxgraphql.BeadSnapshot
	for _, proj := range projects {
		if projectID != "" && proj.ID != projectID {
			continue
		}
		store := bead.NewStore(filepath.Join(proj.Path, ".ddx"))
		beads, err := store.ReadAll()
		if err != nil {
			continue
		}
		for _, b := range beads {
			if status != "" && b.Status != status {
				continue
			}
			if label != "" && !containsString(b.Labels, label) {
				continue
			}
			snap := ddxgraphql.BeadSnapshot{
				ProjectID:   proj.ID,
				ID:          b.ID,
				Title:       b.Title,
				Status:      b.Status,
				Priority:    b.Priority,
				IssueType:   b.IssueType,
				Owner:       b.Owner,
				CreatedAt:   b.CreatedAt,
				CreatedBy:   b.CreatedBy,
				UpdatedAt:   b.UpdatedAt,
				Labels:      b.Labels,
				Parent:      b.Parent,
				Description: b.Description,
				Acceptance:  b.Acceptance,
				Notes:       b.Notes,
			}
			for _, d := range b.Dependencies {
				snap.Dependencies = append(snap.Dependencies, ddxgraphql.BeadDependencySnapshot{
					IssueID:     d.IssueID,
					DependsOnID: d.DependsOnID,
					Type:        d.Type,
					CreatedAt:   d.CreatedAt,
					CreatedBy:   d.CreatedBy,
					Metadata:    d.Metadata,
				})
			}
			result = append(result, snap)
		}
	}
	return result
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
