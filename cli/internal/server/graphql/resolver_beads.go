package graphql

import (
	"context"
	"time"
)

// Beads is the resolver for the beads field.
func (r *queryResolver) Beads(ctx context.Context, first *int, after *string, last *int, before *string, status *string, label *string, projectID *string) (*BeadConnection, error) {
	statusVal := ""
	if status != nil {
		statusVal = *status
	}
	labelVal := ""
	if label != nil {
		labelVal = *label
	}
	projectIDVal := ""
	if projectID != nil {
		projectIDVal = *projectID
	}
	snaps := r.State.GetBeadSnapshots(statusVal, labelVal, projectIDVal, "")
	return beadConnectionFromSnapshots(snaps, first, after, last, before), nil
}

// BeadsByProject is the resolver for the beadsByProject field.
func (r *queryResolver) BeadsByProject(ctx context.Context, projectID string, first *int, after *string, last *int, before *string, status *string, label *string, search *string) (*BeadConnection, error) {
	statusVal := ""
	if status != nil {
		statusVal = *status
	}
	labelVal := ""
	if label != nil {
		labelVal = *label
	}
	searchVal := ""
	if search != nil {
		searchVal = *search
	}
	snaps := r.State.GetBeadSnapshots(statusVal, labelVal, projectID, searchVal)
	return beadConnectionFromSnapshots(snaps, first, after, last, before), nil
}

func beadConnectionFromSnapshots(snaps []BeadSnapshot, first *int, after *string, last *int, before *string) *BeadConnection {
	// Build full edge list with stable ID-based cursors.
	all := make([]*BeadEdge, len(snaps))
	for i, s := range snaps {
		all[i] = &BeadEdge{
			Node:   beadFromSnapshot(s),
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
	truncatedByFirst := false
	truncatedByLast := false
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
		truncatedByFirst = true
	}
	if last != nil && *last >= 0 && *last < len(slice) {
		slice = slice[len(slice)-*last:]
		truncatedByLast = true
	}

	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0 || truncatedByLast,
		HasNextPage:     endIdx < len(all) || truncatedByFirst,
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}

	return &BeadConnection{
		Edges:      slice,
		PageInfo:   pageInfo,
		TotalCount: len(all),
	}
}

func beadFromSnapshot(s BeadSnapshot) *Bead {
	b := &Bead{
		ID:        s.ID,
		Title:     s.Title,
		Status:    s.Status,
		Priority:  s.Priority,
		IssueType: s.IssueType,
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: s.UpdatedAt.UTC().Format(time.RFC3339),
		Labels:    s.Labels,
	}
	if s.ProjectID != "" {
		b.ProjectID = &s.ProjectID
	}
	if s.Owner != "" {
		b.Owner = &s.Owner
	}
	if s.CreatedBy != "" {
		b.CreatedBy = &s.CreatedBy
	}
	if s.Parent != "" {
		b.Parent = &s.Parent
	}
	if s.Description != "" {
		b.Description = &s.Description
	}
	if s.Acceptance != "" {
		b.Acceptance = &s.Acceptance
	}
	if s.Notes != "" {
		b.Notes = &s.Notes
	}
	for _, d := range s.Dependencies {
		dep := &Dependency{
			IssueID:     d.IssueID,
			DependsOnID: d.DependsOnID,
			Type:        d.Type,
		}
		if d.CreatedAt != "" {
			dep.CreatedAt = &d.CreatedAt
		}
		if d.CreatedBy != "" {
			dep.CreatedBy = &d.CreatedBy
		}
		if d.Metadata != "" {
			dep.Metadata = &d.Metadata
		}
		b.Dependencies = append(b.Dependencies, dep)
	}
	return b
}
