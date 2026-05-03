package graphql

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// beadStore returns a bead.Store rooted at the per-request working directory
// (from ctx via WithWorkingDir, falling back to r.WorkingDir).
func (r *mutationResolver) beadStore(ctx context.Context) *bead.Store {
	return bead.NewStore(filepath.Join(r.workingDir(ctx), ".ddx"))
}

// beadModelFromBead converts a bead.Bead to the GraphQL Bead model.
func beadModelFromBead(b *bead.Bead) *Bead {
	gql := &Bead{
		ID:        b.ID,
		Title:     b.Title,
		Status:    b.Status,
		Priority:  b.Priority,
		IssueType: b.IssueType,
		CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: b.UpdatedAt.UTC().Format(time.RFC3339),
		Labels:    b.Labels,
	}
	if b.Owner != "" {
		gql.Owner = &b.Owner
	}
	if b.CreatedBy != "" {
		gql.CreatedBy = &b.CreatedBy
	}
	if b.Parent != "" {
		gql.Parent = &b.Parent
	}
	if b.Description != "" {
		gql.Description = &b.Description
	}
	if b.Acceptance != "" {
		gql.Acceptance = &b.Acceptance
	}
	if b.Notes != "" {
		gql.Notes = &b.Notes
	}
	for _, d := range b.Dependencies {
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
		gql.Dependencies = append(gql.Dependencies, dep)
	}
	return gql
}

// BeadCreate is the resolver for the beadCreate mutation.
func (r *mutationResolver) BeadCreate(ctx context.Context, input BeadInput) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if input.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	b := &bead.Bead{
		Title: input.Title,
	}
	if input.Status != nil {
		b.Status = *input.Status
	}
	if input.Priority != nil {
		b.Priority = *input.Priority
	}
	if input.IssueType != nil {
		b.IssueType = *input.IssueType
	}
	if input.Labels != nil {
		b.Labels = input.Labels
	}
	if input.Parent != nil {
		b.Parent = *input.Parent
	}
	if input.Description != nil {
		b.Description = *input.Description
	}
	if input.Acceptance != nil {
		b.Acceptance = *input.Acceptance
	}
	if input.Notes != nil {
		b.Notes = *input.Notes
	}

	store := r.beadStore(ctx)
	if err := store.Create(b); err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// BeadUpdate is the resolver for the beadUpdate mutation.
func (r *mutationResolver) BeadUpdate(ctx context.Context, id string, input BeadUpdateInput) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}

	store := r.beadStore(ctx)
	err := store.Update(id, func(b *bead.Bead) {
		if input.Title != nil {
			b.Title = *input.Title
		}
		if input.Status != nil {
			b.Status = *input.Status
		}
		if input.Priority != nil {
			b.Priority = *input.Priority
		}
		if input.IssueType != nil {
			b.IssueType = *input.IssueType
		}
		if input.Labels != nil {
			b.Labels = input.Labels
		}
		if input.Parent != nil {
			b.Parent = *input.Parent
		}
		if input.Description != nil {
			b.Description = *input.Description
		}
		if input.Acceptance != nil {
			b.Acceptance = *input.Acceptance
		}
		if input.Notes != nil {
			b.Notes = *input.Notes
		}
	})
	if err != nil {
		return nil, err
	}

	b, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// BeadClaim is the resolver for the beadClaim mutation.
func (r *mutationResolver) BeadClaim(ctx context.Context, id string, assignee string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}

	store := r.beadStore(ctx)
	if err := store.Claim(id, assignee); err != nil {
		return nil, err
	}

	b, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// BeadUnclaim is the resolver for the beadUnclaim mutation.
func (r *mutationResolver) BeadUnclaim(ctx context.Context, id string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}

	store := r.beadStore(ctx)
	if err := store.Unclaim(id); err != nil {
		return nil, err
	}

	b, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// BeadReopen is the resolver for the beadReopen mutation.
func (r *mutationResolver) BeadReopen(ctx context.Context, id string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}

	store := r.beadStore(ctx)
	err := store.Update(id, func(b *bead.Bead) {
		b.Status = bead.StatusOpen
		b.Owner = ""
	})
	if err != nil {
		return nil, err
	}

	b, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}
