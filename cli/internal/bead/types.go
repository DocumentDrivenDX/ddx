package bead

import "time"

// Bead represents a portable work item with metadata.
// Unknown fields from external sources are preserved in Extra.
type Bead struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Labels      []string  `json:"labels"`
	Parent      string    `json:"parent,omitempty"`
	Description string    `json:"description,omitempty"`
	Acceptance  string    `json:"acceptance,omitempty"`
	Deps        []string  `json:"deps"`
	Assignee    string    `json:"assignee,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`

	// Extra holds unknown fields for round-trip preservation.
	// Workflow-specific fields (e.g. HELIX spec-id, execution-eligible)
	// are stored here and written back on save.
	Extra map[string]any `json:"-"`
}

// Status constants
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusClosed     = "closed"
)

// Default values
const (
	DefaultType     = "task"
	DefaultStatus   = StatusOpen
	DefaultPriority = 2
	DefaultPrefix   = "bx"
	MinPriority     = 0
	MaxPriority     = 4
)

// StatusCounts holds aggregate counts for a bead store.
type StatusCounts struct {
	Open    int `json:"open"`
	Closed  int `json:"closed"`
	Blocked int `json:"blocked"`
	Ready   int `json:"ready"`
	Total   int `json:"total"`
}
