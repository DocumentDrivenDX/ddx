package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// beadStore returns a bead.Store rooted at the per-request working directory
// (from ctx via WithWorkingDir, falling back to r.WorkingDir).
func (r *mutationResolver) beadStore(ctx context.Context) *bead.Store {
	return bead.NewStore(ddxroot.JoinProject(r.workingDir(ctx)))
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
	if err := store.Create(ctx, b); err != nil {
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
	mutate := func(b *bead.Bead) error {
		if input.Title != nil {
			b.Title = *input.Title
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
		return nil
	}
	var err error
	if input.Status != nil {
		err = store.UpdateWithLifecycleStatus(id, *input.Status, bead.LifecycleTransitionOptions{
			Reason: "graphql bead update",
			Source: "graphql:beadUpdate",
		}, mutate)
	} else {
		err = store.Update(ctx, id, func(b *bead.Bead) {
			_ = mutate(b)
		})
	}
	if err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
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

	b, err := store.Get(ctx, id)
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

	b, err := store.Get(ctx, id)
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
	if err := store.Reopen(id, "graphql bead reopen", ""); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// BeadApprove is the resolver for the beadApprove mutation.
// Transitions a proposed bead to open status (operator approval).
func (r *mutationResolver) BeadApprove(ctx context.Context, id string, note string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(note) == "" {
		return nil, fmt.Errorf("note is required")
	}

	store := r.beadStore(ctx)
	if err := store.TransitionLifecycle(id, bead.StatusOpen, bead.LifecycleTransitionOptions{
		ManualReopen: true,
		Reason:       "graphql bead approve",
		Actor:        "operator",
		Source:       "graphql:beadApprove",
	}, nil); err != nil {
		return nil, err
	}

	if err := store.AppendEvent(id, bead.BeadEvent{
		Kind:    "human-resolution",
		Summary: "approve",
		Body:    note,
		Actor:   "operator",
		Source:  "graphql",
	}); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// BeadCancel is the resolver for the beadCancel mutation.
// Cancels a bead from open, in_progress, blocked, or proposed status.
func (r *mutationResolver) BeadCancel(ctx context.Context, id string, reason string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("reason is required")
	}

	store := r.beadStore(ctx)
	if err := store.TransitionLifecycle(id, bead.StatusCancelled, bead.LifecycleTransitionOptions{
		Reason: reason,
		Actor:  "operator",
		Source: "graphql:beadCancel",
	}, nil); err != nil {
		return nil, err
	}

	if err := store.AppendEvent(id, bead.BeadEvent{
		Kind:    "human-resolution",
		Summary: "cancel",
		Body:    reason,
		Actor:   "operator",
		Source:  "graphql",
	}); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// BeadBlock is the resolver for the beadBlock mutation.
// Blocks a bead with an external blocker reason.
func (r *mutationResolver) BeadBlock(ctx context.Context, id string, externalBlockerReason string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(externalBlockerReason) == "" {
		return nil, fmt.Errorf("externalBlockerReason is required")
	}

	store := r.beadStore(ctx)
	if err := store.TransitionLifecycle(id, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: externalBlockerReason,
		Reason:                "graphql bead block",
		Actor:                 "operator",
		Source:                "graphql:beadBlock",
	}, nil); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}
