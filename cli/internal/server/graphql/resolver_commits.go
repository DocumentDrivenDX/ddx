package graphql

import (
	"context"
	"fmt"
)

// Commits is the resolver for the commits field.
func (r *queryResolver) Commits(ctx context.Context, projectID string, first *int, after *string, last *int, before *string, since *string, author *string) (*CommitConnection, error) {
	if r.State == nil {
		return nil, fmt.Errorf("state provider not configured")
	}

	sinceStr := ""
	if since != nil {
		sinceStr = *since
	}
	authorStr := ""
	if author != nil {
		authorStr = *author
	}

	snaps, err := r.State.GetProjectCommits(projectID, sinceStr, authorStr)
	if err != nil {
		return nil, err
	}

	// Build full edge list with opaque cursors.
	all := make([]*CommitEdge, len(snaps))
	for i, s := range snaps {
		all[i] = &CommitEdge{
			Node:   commitFromSnapshot(s),
			Cursor: encodeCursor(i),
		}
	}

	// Apply window: start after `after` cursor, end before `before` cursor.
	startIdx := 0
	if after != nil {
		if idx, ok := decodeCursor(*after); ok {
			startIdx = idx + 1
		}
	}
	endIdx := len(all)
	if before != nil {
		if idx, ok := decodeCursor(*before); ok && idx < endIdx {
			endIdx = idx
		}
	}
	if startIdx > endIdx {
		startIdx = endIdx
	}

	windowSize := endIdx - startIdx
	slice := all[startIdx:endIdx]
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
	}
	if last != nil && *last >= 0 && *last < len(slice) {
		slice = slice[len(slice)-*last:]
	}

	pageInfo := &PageInfo{
		// HasPreviousPage: there are edges before the window (after cursor) or
		// last-truncation left edges at the start of the window.
		HasPreviousPage: startIdx > 0 || (last != nil && *last >= 0 && *last < windowSize),
		// HasNextPage: there are edges after the window (before cursor) or
		// first-truncation left edges at the end of the window.
		HasNextPage: endIdx < len(all) || (first != nil && *first >= 0 && *first < windowSize),
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}

	return &CommitConnection{
		Edges:      slice,
		PageInfo:   pageInfo,
		TotalCount: len(all),
	}, nil
}

func commitFromSnapshot(s CommitSnapshot) *Commit {
	c := &Commit{
		Sha:      s.SHA,
		ShortSha: s.ShortSHA,
		Author:   s.Author,
		Date:     s.Date,
		Subject:  s.Subject,
		BeadRefs: s.BeadRefs,
	}
	if s.Body != "" {
		c.Body = &s.Body
	}
	return c
}
