package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// beadStore returns a bead.Store rooted at the per-request working directory
// (from ctx via WithWorkingDir, falling back to r.WorkingDir).
func (r *mutationResolver) beadStore(ctx context.Context) *bead.Store {
	return projectBeadStore(r.workingDir(ctx))
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

// beadMutationSelection is the common GraphQL field selection used to round-
// trip bead mutations through federation.
const beadMutationSelection = `{
  id
  title
  status
  priority
  issueType
  owner
  createdAt
  createdBy
  updatedAt
  labels
  projectID
  parent
  description
  acceptance
  notes
  dependencies {
    issueId
    dependsOnId
    type
    createdAt
    createdBy
    metadata
  }
}`

type beadMutationForwardEnvelope struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type beadMutationForwardResponse struct {
	Data struct {
		BeadCreate *Bead `json:"beadCreate,omitempty"`
		BeadUpdate *Bead `json:"beadUpdate,omitempty"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func (r *mutationResolver) projectIDForWorkingDir(workingDir string) (string, bool) {
	if r.State == nil || workingDir == "" {
		return "", false
	}
	for _, proj := range r.State.GetProjectSnapshots(false) {
		if proj.Path == workingDir {
			return proj.ID, true
		}
	}
	return "", false
}

// beadMutationOwner resolves the owning spoke for the current request's
// project. When the project is local or the request is not running with a
// federation provider, the second return value is nil and the caller should
// mutate the local store.
func (r *mutationResolver) beadMutationOwner(workingDir string) (projectID string, owner *federation.SpokeRecord, err error) {
	projectID, ok := r.projectIDForWorkingDir(workingDir)
	if !ok || r.Federation == nil {
		return projectID, nil, nil
	}

	registry := federation.NewRegistry()
	registry.Spokes = append(registry.Spokes, r.Federation.Spokes()...)
	owner, err = federation.RouteMutationToProjectOwner(registry, projectID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "multiple registered owners"):
			return projectID, nil, federation.ErrForwardMutationBroadcastLike
		case strings.Contains(err.Error(), "no registered spoke owns project"):
			return projectID, nil, federation.ErrForwardMutationMissingOwner
		default:
			return projectID, nil, err
		}
	}
	if owner == nil {
		return projectID, nil, nil
	}
	if strings.TrimSpace(owner.NodeID) == "" || strings.TrimSpace(owner.NodeID) == strings.TrimSpace(r.NodeID) {
		return projectID, nil, nil
	}
	return projectID, owner, nil
}

func beadMutationForwardQueryCreate() string {
	return "mutation BeadCreate($input: BeadInput!) { beadCreate(input: $input) " + beadMutationSelection + " }"
}

func beadMutationForwardQueryUpdate() string {
	return "mutation BeadUpdate($id: ID!, $input: BeadUpdateInput!) { beadUpdate(id: $id, input: $input) " + beadMutationSelection + " }"
}

func (r *mutationResolver) forwardBeadMutation(ctx context.Context, owner *federation.SpokeRecord, projectID, mutationName, query string, variables map[string]any) (*Bead, error) {
	if r.Federation == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	body, err := json.Marshal(beadMutationForwardEnvelope{
		Query:     query,
		Variables: variables,
	})
	if err != nil {
		return nil, fmt.Errorf("bead mutation forward: encode request: %w", err)
	}

	forwardPath := []string{}
	if nodeID := strings.TrimSpace(r.NodeID); nodeID != "" {
		forwardPath = append(forwardPath, nodeID)
	}
	if owner != nil && strings.TrimSpace(owner.NodeID) != "" {
		forwardPath = append(forwardPath, strings.TrimSpace(owner.NodeID))
	}

	resp, err := r.Federation.ForwardMutation(ctx, &federation.ForwardMutationRequest{
		OriginIdentity:       strings.TrimSpace(r.NodeID),
		ForwardingPath:       forwardPath,
		TargetNodeID:         strings.TrimSpace(owner.NodeID),
		TargetProjectID:      strings.TrimSpace(projectID),
		RequiredCapabilities: []string{"write"},
		Body:                 body,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("bead mutation forward: empty response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("bead mutation forward: spoke returned HTTP %d", resp.StatusCode)
	}

	var decoded beadMutationForwardResponse
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return nil, fmt.Errorf("bead mutation forward: decode response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		msgs := make([]string, 0, len(decoded.Errors))
		for _, e := range decoded.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("bead mutation forward: %s", strings.Join(msgs, "; "))
	}

	switch mutationName {
	case "beadCreate":
		if decoded.Data.BeadCreate == nil {
			return nil, fmt.Errorf("bead mutation forward: missing beadCreate payload")
		}
		return decoded.Data.BeadCreate, nil
	case "beadUpdate":
		if decoded.Data.BeadUpdate == nil {
			return nil, fmt.Errorf("bead mutation forward: missing beadUpdate payload")
		}
		return decoded.Data.BeadUpdate, nil
	default:
		return nil, fmt.Errorf("bead mutation forward: unknown mutation %q", mutationName)
	}
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

	projectID, owner, err := r.beadMutationOwner(r.workingDir(ctx))
	if err != nil {
		return nil, err
	}
	if owner != nil {
		return r.forwardBeadMutation(ctx, owner, projectID, "beadCreate", beadMutationForwardQueryCreate(), map[string]any{
			"input": input,
		})
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

	projectID, owner, routeErr := r.beadMutationOwner(r.workingDir(ctx))
	if routeErr != nil {
		return nil, routeErr
	}
	if owner != nil {
		return r.forwardBeadMutation(ctx, owner, projectID, "beadUpdate", beadMutationForwardQueryUpdate(), map[string]any{
			"id":    id,
			"input": input,
		})
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

	projectID, projectRoot, remote := beadLifecycleProjectPathFromSnapshot(ctx, r.Resolver, id)
	if remote {
		if forwarded, handled, err := r.forwardBeadLifecycleMutation(ctx, projectID, "beadReopen", "beadReopen", map[string]any{
			"id": id,
		}); err != nil {
			return nil, err
		} else if handled {
			return beadLifecycleSetProjectID(forwarded, projectID), nil
		}
	}

	store := projectBeadStore(projectRoot)
	if err := store.Reopen(id, "graphql bead reopen", ""); err != nil {
		return nil, err
	}
	if err := r.appendBeadLifecycleAuditEvent(ctx, store, id, "reopen", "graphql:beadReopen", map[string]string{
		"action": "reopen",
		"reason": "graphql bead reopen",
	}); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadLifecycleSetProjectID(beadModelFromBead(b), projectID), nil
}

// BeadApprove is the resolver for the beadApprove mutation.
// Transitions a proposed bead to open status (operator approval).
func (r *mutationResolver) BeadApprove(ctx context.Context, id string, note string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(note) == "" {
		return nil, beadLifecycleValidationError("BEAD_APPROVE_NOTE_REQUIRED", "note is required")
	}

	projectID, projectRoot, remote := beadLifecycleProjectPathFromSnapshot(ctx, r.Resolver, id)
	if remote {
		if forwarded, handled, err := r.forwardBeadLifecycleMutation(ctx, projectID, "beadApprove", "beadApprove", map[string]any{
			"id":   id,
			"note": note,
		}); err != nil {
			return nil, err
		} else if handled {
			return beadLifecycleSetProjectID(forwarded, projectID), nil
		}
	}

	store := projectBeadStore(projectRoot)
	if err := store.TransitionLifecycle(id, bead.StatusOpen, bead.LifecycleTransitionOptions{
		ManualReopen: true,
		Reason:       "graphql bead approve",
		Actor:        "operator",
		Source:       "graphql:beadApprove",
	}, nil); err != nil {
		return nil, err
	}

	if err := r.appendBeadLifecycleAuditEvent(ctx, store, id, "approve", "graphql:beadApprove", map[string]string{
		"action": "approve",
		"note":   note,
	}); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadLifecycleSetProjectID(beadModelFromBead(b), projectID), nil
}

// BeadCancel is the resolver for the beadCancel mutation.
// Cancels a bead from open, in_progress, blocked, or proposed status.
func (r *mutationResolver) BeadCancel(ctx context.Context, id string, reason string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, beadLifecycleValidationError("BEAD_CANCEL_REASON_REQUIRED", "reason is required")
	}

	projectID, projectRoot, remote := beadLifecycleProjectPathFromSnapshot(ctx, r.Resolver, id)
	if remote {
		if forwarded, handled, err := r.forwardBeadLifecycleMutation(ctx, projectID, "beadCancel", "beadCancel", map[string]any{
			"id":     id,
			"reason": reason,
		}); err != nil {
			return nil, err
		} else if handled {
			return beadLifecycleSetProjectID(forwarded, projectID), nil
		}
	}

	store := projectBeadStore(projectRoot)
	if err := store.TransitionLifecycle(id, bead.StatusCancelled, bead.LifecycleTransitionOptions{
		Reason: reason,
		Actor:  "operator",
		Source: "graphql:beadCancel",
	}, nil); err != nil {
		return nil, err
	}

	if err := r.appendBeadLifecycleAuditEvent(ctx, store, id, "cancel", "graphql:beadCancel", map[string]string{
		"action": "cancel",
		"reason": reason,
	}); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadLifecycleSetProjectID(beadModelFromBead(b), projectID), nil
}

// BeadBlock is the resolver for the beadBlock mutation.
// Blocks a bead with an external blocker reason.
func (r *mutationResolver) BeadBlock(ctx context.Context, id string, externalBlockerReason string) (*Bead, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(externalBlockerReason) == "" {
		return nil, beadLifecycleValidationError("BEAD_BLOCK_EXTERNAL_BLOCKER_REASON_REQUIRED", "externalBlockerReason is required")
	}

	projectID, projectRoot, remote := beadLifecycleProjectPathFromSnapshot(ctx, r.Resolver, id)
	if remote {
		if forwarded, handled, err := r.forwardBeadLifecycleMutation(ctx, projectID, "beadBlock", "beadBlock", map[string]any{
			"id":                    id,
			"externalBlockerReason": externalBlockerReason,
		}); err != nil {
			return nil, err
		} else if handled {
			return beadLifecycleSetProjectID(forwarded, projectID), nil
		}
	}

	store := projectBeadStore(projectRoot)
	if err := store.TransitionLifecycle(id, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: externalBlockerReason,
		Reason:                "graphql bead block",
		Actor:                 "operator",
		Source:                "graphql:beadBlock",
	}, nil); err != nil {
		return nil, err
	}

	if err := r.appendBeadLifecycleAuditEvent(ctx, store, id, "block", "graphql:beadBlock", map[string]string{
		"action":                  "block",
		"external_blocker_reason": externalBlockerReason,
	}); err != nil {
		return nil, err
	}

	b, err := store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return beadLifecycleSetProjectID(beadModelFromBead(b), projectID), nil
}
