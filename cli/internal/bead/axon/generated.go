package axon

import (
	"context"
	"errors"
	"time"
)

// Transport is the minimal GraphQL execution surface used by the client.
type Transport interface {
	Query(ctx context.Context, query string, variables map[string]any, response any) error
}

// Client is a small typed wrapper around a GraphQL transport.
type Client struct {
	transport Transport
}

// NewClient wires a transport into the client surface.
func NewClient(transport Transport) *Client {
	return &Client{transport: transport}
}

// Dependency mirrors a bead dependency edge in the GraphQL layer.
type Dependency struct {
	IssueID     string `json:"issueID"`
	DependsOnID string `json:"dependsOnID"`
	Type        string `json:"type"`
	CreatedAt   string `json:"createdAt,omitempty"`
	CreatedBy   string `json:"createdBy,omitempty"`
	Metadata    string `json:"metadata,omitempty"`
}

// DependencyInput mirrors Dependency for mutations.
type DependencyInput = Dependency

// Bead mirrors the bead tracker row stored in Axon.
type Bead struct {
	Version      int            `json:"version,omitempty"`
	ID           string         `json:"id,omitempty"`
	Title        string         `json:"title"`
	Status       string         `json:"status"`
	Priority     int            `json:"priority"`
	IssueType    string         `json:"issueType"`
	Owner        string         `json:"owner,omitempty"`
	CreatedAt    time.Time      `json:"createdAt,omitempty"`
	CreatedBy    string         `json:"createdBy,omitempty"`
	UpdatedAt    time.Time      `json:"updatedAt,omitempty"`
	Labels       []string       `json:"labels,omitempty"`
	Parent       string         `json:"parent,omitempty"`
	Description  string         `json:"description,omitempty"`
	Acceptance   string         `json:"acceptance,omitempty"`
	Notes        string         `json:"notes,omitempty"`
	Dependencies []Dependency   `json:"dependencies,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

// BeadInput matches the GraphQL mutation payload for create/update.
type BeadInput struct {
	ID           string            `json:"id,omitempty"`
	Title        string            `json:"title"`
	Status       string            `json:"status"`
	Priority     int               `json:"priority"`
	IssueType    string            `json:"issueType"`
	Owner        string            `json:"owner,omitempty"`
	CreatedAt    time.Time         `json:"createdAt,omitempty"`
	CreatedBy    string            `json:"createdBy,omitempty"`
	UpdatedAt    time.Time         `json:"updatedAt,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	Parent       string            `json:"parent,omitempty"`
	Description  string            `json:"description,omitempty"`
	Acceptance   string            `json:"acceptance,omitempty"`
	Notes        string            `json:"notes,omitempty"`
	Dependencies []DependencyInput `json:"dependencies,omitempty"`
	Extra        map[string]any    `json:"extra,omitempty"`
}

// BeadEvent mirrors a bead event entity in the ddx_bead_events collection.
type BeadEvent struct {
	ID        string    `json:"id,omitempty"`
	Version   int       `json:"version,omitempty"`
	EventOf   string    `json:"eventOf,omitempty"`
	Index     int       `json:"index,omitempty"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary,omitempty"`
	Body      string    `json:"body,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

// LifecycleEvent is the local lifecycle payload used by the subscription
// adapter tests. It stays in this package so the client package does not need
// to import cli/internal/bead.
type LifecycleEvent struct {
	EventID   string    `json:"eventID"`
	BeadID    string    `json:"beadID"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Body      string    `json:"body"`
	Actor     string    `json:"actor"`
	Timestamp time.Time `json:"timestamp"`
}

// ChangeEvent mirrors the subscription payload returned by Axon.
type ChangeEvent struct {
	EventID   string    `json:"eventID"`
	BeadID    string    `json:"beadID"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Body      string    `json:"body"`
	Actor     string    `json:"actor"`
	Timestamp time.Time `json:"timestamp"`
}

// ChangeEventFromLifecycle converts a lifecycle event into the GraphQL shape.
func ChangeEventFromLifecycle(src LifecycleEvent) ChangeEvent {
	return ChangeEvent(src)
}

// ToLifecycleEvent converts the GraphQL payload into the local lifecycle shape.
func (c ChangeEvent) ToLifecycleEvent() LifecycleEvent {
	return LifecycleEvent(c)
}

// Response envelopes.
type GetBeadResponse struct {
	DDXBead *Bead `json:"ddxBead,omitempty"`
}

type ListBeadsResponse struct {
	DDXBeads []Bead `json:"ddxBeads,omitempty"`
}

type CreateBeadResponse struct {
	CreateEntity *Bead `json:"createEntity,omitempty"`
}

type UpdateBeadResponse struct {
	UpdateEntity *Bead `json:"updateEntity,omitempty"`
}

type ListBeadEventsResponse struct {
	DDXBeadEvents []BeadEvent `json:"ddxBeadEvents,omitempty"`
}

type CreateBeadEventResponse struct {
	CreateEntity *BeadEvent `json:"createEntity,omitempty"`
}

type LinkResult struct {
	OK bool `json:"ok"`
}

type CreateLinkResponse struct {
	CreateLink *LinkResult `json:"createLink,omitempty"`
}

type DeleteLinkResponse struct {
	DeleteLink *LinkResult `json:"deleteLink,omitempty"`
}

const (
	getBeadQuery                  = `query GetBead($id: ID!) { ddxBead(id: $id) { ...BeadFields } }`
	listBeadsQuery                = `query ListBeads { ddxBeads { ...BeadFields } }`
	createBeadMutation            = `mutation CreateBead($input: BeadInput!) { createEntity(input: $input) { ...BeadFields } }`
	updateBeadMutation            = `mutation UpdateBead($id: ID!, $expectedVersion: Int!, $input: BeadInput!) { updateEntity(id: $id, expectedVersion: $expectedVersion, input: $input) { ...BeadFields } }`
	listBeadEventsQuery           = `query ListBeadEvents { ddxBeadEvents { ...BeadEventFields } }`
	createBeadEventMutation       = `mutation CreateBeadEvent($input: BeadEventInput!) { createEntity(input: $input) { ...BeadEventFields } }`
	createDependencyLinkMutation  = `mutation CreateDependencyLink($from: ID!, $to: ID!) { createLink(from: $from, to: $to, type: "depends_on") { ok } }`
	deleteDependencyLinkMutation  = `mutation DeleteDependencyLink($from: ID!, $to: ID!) { deleteLink(from: $from, to: $to, type: "depends_on") { ok } }`
	changeEventsSubscriptionQuery = `subscription ChangeEvents($projectID: ID!) { changeEvents(projectID: $projectID) { ...ChangeEventFields } }`
)

func (c *Client) query(ctx context.Context, query string, variables map[string]any, response any) error {
	if c == nil || c.transport == nil {
		return errors.New("axon: transport is nil")
	}
	return c.transport.Query(ctx, query, variables, response)
}

// GetBead loads one bead by ID.
func (c *Client) GetBead(ctx context.Context, id string) (*Bead, error) {
	var resp GetBeadResponse
	if err := c.query(ctx, getBeadQuery, map[string]any{"id": id}, &resp); err != nil {
		return nil, err
	}
	return resp.DDXBead, nil
}

// ListBeads loads the bead collection.
func (c *Client) ListBeads(ctx context.Context) ([]Bead, error) {
	var resp ListBeadsResponse
	if err := c.query(ctx, listBeadsQuery, nil, &resp); err != nil {
		return nil, err
	}
	return resp.DDXBeads, nil
}

// CreateBead inserts a new bead and returns the materialized row.
func (c *Client) CreateBead(ctx context.Context, input BeadInput) (*Bead, error) {
	var resp CreateBeadResponse
	if err := c.query(ctx, createBeadMutation, map[string]any{"input": input}, &resp); err != nil {
		return nil, err
	}
	return resp.CreateEntity, nil
}

// UpdateBead updates an existing bead with optimistic concurrency control.
func (c *Client) UpdateBead(ctx context.Context, id string, expectedVersion int, input BeadInput) (*Bead, error) {
	var resp UpdateBeadResponse
	vars := map[string]any{"id": id, "expectedVersion": expectedVersion, "input": input}
	if err := c.query(ctx, updateBeadMutation, vars, &resp); err != nil {
		return nil, err
	}
	return resp.UpdateEntity, nil
}

// ListBeadEvents loads the event entity collection.
func (c *Client) ListBeadEvents(ctx context.Context) ([]BeadEvent, error) {
	var resp ListBeadEventsResponse
	if err := c.query(ctx, listBeadEventsQuery, nil, &resp); err != nil {
		return nil, err
	}
	return resp.DDXBeadEvents, nil
}

// CreateBeadEvent appends one bead event entity.
func (c *Client) CreateBeadEvent(ctx context.Context, input BeadEvent) (*BeadEvent, error) {
	var resp CreateBeadEventResponse
	if err := c.query(ctx, createBeadEventMutation, map[string]any{"input": input}, &resp); err != nil {
		return nil, err
	}
	return resp.CreateEntity, nil
}

// CreateDependencyLink writes a depends_on link between two beads.
func (c *Client) CreateDependencyLink(ctx context.Context, fromID, toID string) error {
	var resp CreateLinkResponse
	vars := map[string]any{"from": fromID, "to": toID}
	if err := c.query(ctx, createDependencyLinkMutation, vars, &resp); err != nil {
		return err
	}
	if resp.CreateLink != nil && !resp.CreateLink.OK {
		return errors.New("axon: create dependency link rejected")
	}
	return nil
}

// DeleteDependencyLink removes a depends_on link between two beads.
func (c *Client) DeleteDependencyLink(ctx context.Context, fromID, toID string) error {
	var resp DeleteLinkResponse
	vars := map[string]any{"from": fromID, "to": toID}
	if err := c.query(ctx, deleteDependencyLinkMutation, vars, &resp); err != nil {
		return err
	}
	if resp.DeleteLink != nil && !resp.DeleteLink.OK {
		return errors.New("axon: delete dependency link rejected")
	}
	return nil
}

// cloneStringAnyMap copies a map[string]any without aliasing the backing store.
func cloneStringAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
