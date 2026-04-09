package bead

import "time"

// Bead represents a portable work item with metadata.
// The schema matches bd/br JSONL format for interchange compatibility.
// Unknown fields from external sources are preserved in Extra.
type Bead struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  int       `json:"priority"`
	IssueType string    `json:"issue_type"`
	Owner     string    `json:"owner,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`

	// Optional fields (bd-compatible)
	Labels      []string `json:"labels,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	Description string   `json:"description,omitempty"`
	Acceptance  string   `json:"acceptance,omitempty"`
	Notes       string   `json:"notes,omitempty"`

	// Dependencies use bd-compatible format
	Dependencies []Dependency `json:"dependencies,omitempty"`

	// Extra holds unknown fields for round-trip preservation.
	// Workflow-specific fields (e.g. HELIX spec-id, execution-eligible)
	// are stored here and written back on save.
	Extra map[string]any `json:"-"`
}

// Dependency represents a link between two beads (bd-compatible format).
type Dependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"` // "blocks", "related", etc.
	CreatedAt   string `json:"created_at,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
	Metadata    string `json:"metadata,omitempty"`
}

// BeadEvent records append-only execution evidence.
type BeadEvent struct {
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary,omitempty"`
	Body      string    `json:"body,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source,omitempty"`
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
	DefaultPrefix   = "bx" // used only when repo name detection fails
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

// DepIDs returns a flat list of dependency IDs for this bead.
func (b *Bead) DepIDs() []string {
	var ids []string
	for _, d := range b.Dependencies {
		ids = append(ids, d.DependsOnID)
	}
	return ids
}

// HasDep returns true if the bead depends on the given ID.
func (b *Bead) HasDep(id string) bool {
	for _, d := range b.Dependencies {
		if d.DependsOnID == id {
			return true
		}
	}
	return false
}

// AddDep adds a dependency if it doesn't already exist.
func (b *Bead) AddDep(depID, depType string) {
	for _, d := range b.Dependencies {
		if d.DependsOnID == depID {
			return // already exists
		}
	}
	b.Dependencies = append(b.Dependencies, Dependency{
		IssueID:     b.ID,
		DependsOnID: depID,
		Type:        depType,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	})
}

// RemoveDep removes a dependency by target ID.
func (b *Bead) RemoveDep(depID string) {
	var filtered []Dependency
	for _, d := range b.Dependencies {
		if d.DependsOnID != depID {
			filtered = append(filtered, d)
		}
	}
	b.Dependencies = filtered
}
