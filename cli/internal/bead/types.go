package bead

import (
	"fmt"
	"strings"
	"time"
)

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
	// StatusBlocked marks a bead that is not eligible for agent dispatch —
	// either decomposed into child beads by the triage gate or parked for
	// human intervention after a triage-overflow event. Blocked beads are
	// excluded from ReadyExecution and will not be auto-dispatched.
	StatusBlocked = "blocked"
	// StatusProposed marks an operator-prompt bead awaiting approval. It is
	// excluded from execute-loop drain until transitioned to open via the
	// approval flow, or to cancelled when the operator declines it.
	StatusProposed = "proposed"
	// StatusCancelled marks a bead that was rejected before any execution
	// took place (e.g. an operator-prompt bead the user chose not to run).
	StatusCancelled = "cancelled"
)

// IssueType constants
const (
	// IssueTypeOperatorPrompt is the bead type used for operator-submitted
	// prompts that the execute-loop runs as instructions.
	IssueTypeOperatorPrompt = "operator-prompt"
)

// Default labels and other defaults for operator-prompt beads.
const (
	OperatorPromptLabelKind   = "kind:operator-prompt"
	OperatorPromptLabelSource = "source:web-ui"
	// OperatorPromptDefaultAcceptance is the auto-AC stub used when an
	// operator submits a prompt without explicit acceptance criteria. The
	// structural AC verifier is skipped for operator-prompt beads, so this
	// stub stands as a human-readable contract only.
	OperatorPromptDefaultAcceptance = "Agent must produce a diff or no_changes rationale; the prompt body is the contract."
)

// IsValidStatusTransition reports whether a bead may move from `from` to `to`
// under the documented state machine. Self-transitions (from == to) and
// transitions to/from the empty string (used by Create defaults) are rejected;
// callers that want to seed a fresh status should set b.Status directly before
// validateBead runs.
//
// The matrix below is the normative implementation of TD-031 §3 (Transition
// Matrix). `closed` and `cancelled` are terminal — re-opening a closed bead
// is not a transition; it is filing a follow-up bead with `replaces` set.
//
// Allowed transitions:
//
//	proposed    → open, cancelled
//	open        → in_progress, blocked, cancelled
//	in_progress → open, closed, blocked
//	blocked     → open, cancelled
//	closed      → (terminal)
//	cancelled   → (terminal)
func IsValidStatusTransition(from, to string) bool {
	if from == "" || to == "" || from == to {
		return false
	}
	allowed := map[string]map[string]bool{
		StatusProposed:   {StatusOpen: true, StatusCancelled: true},
		StatusOpen:       {StatusInProgress: true, StatusBlocked: true, StatusCancelled: true},
		StatusInProgress: {StatusOpen: true, StatusClosed: true, StatusBlocked: true},
		StatusBlocked:    {StatusOpen: true, StatusCancelled: true},
		StatusClosed:     {},
		StatusCancelled:  {},
	}
	if next, ok := allowed[from]; ok {
		return next[to]
	}
	return false
}

// OperatorPromptMutationGuard enforces the no-self-mutation rule from
// Story 15: an operator-prompt bead's execution may not create, edit, or
// close another operator-prompt bead. The guard returns nil when the
// mutation is allowed and a non-nil error when it must be rejected.
//
// actorIssueType is the issue_type of the bead currently being executed
// (empty when no operator-prompt context is active). targetIssueType is
// the issue_type of the bead about to be mutated.
//
// Allow/deny matrix:
//
//	actor=""               , target=*                → allow
//	actor="task"           , target=*                → allow
//	actor="operator-prompt", target!="operator-prompt" → allow
//	actor="operator-prompt", target=="operator-prompt" → deny
func OperatorPromptMutationGuard(actorIssueType, targetIssueType string) error {
	if actorIssueType != IssueTypeOperatorPrompt {
		return nil
	}
	if targetIssueType != IssueTypeOperatorPrompt {
		return nil
	}
	return fmt.Errorf("bead: operator-prompt bead may not mutate another operator-prompt bead")
}

// NewOperatorPromptBead constructs a fresh operator-prompt bead from a raw
// prompt string, applying the Story 15 template defaults: title is the first
// non-empty line of the prompt, full prompt body is preserved verbatim in the
// description, default labels are kind:operator-prompt + source:web-ui, the
// status starts in `proposed` (approval flow), priority defaults to the
// caller-supplied tier (clamped to MinPriority..MaxPriority), and the
// acceptance field carries the auto-AC stub.
//
// The returned bead is not persisted; callers feed it to Store.Create which
// will assign the ID and CreatedAt/UpdatedAt timestamps.
func NewOperatorPromptBead(prompt string, defaultTier int) *Bead {
	body := strings.TrimSpace(prompt)
	title := body
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		title = strings.TrimSpace(body[:i])
	}
	if title == "" {
		title = "(empty operator prompt)"
	}
	tier := defaultTier
	if tier < MinPriority {
		tier = MinPriority
	}
	if tier > MaxPriority {
		tier = MaxPriority
	}
	return &Bead{
		Title:       title,
		IssueType:   IssueTypeOperatorPrompt,
		Status:      StatusProposed,
		Priority:    tier,
		Labels:      []string{OperatorPromptLabelKind, OperatorPromptLabelSource},
		Description: body,
		Acceptance:  OperatorPromptDefaultAcceptance,
	}
}

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

// Blocker kinds surfaced through BlockedAll. These strings are part of the
// external DDx contract (HELIX reads them to decide how to handle a blocker).
const (
	BlockerKindDependency    = "dependency"
	BlockerKindRetryCooldown = "retry-cooldown"
)

// Blocker describes why an open bead is currently not runnable. Either
// unclosed dependencies exist, or an execute-loop cooldown has parked the
// bead until NextEligibleAt.
type Blocker struct {
	Kind           string   `json:"kind"`
	NextEligibleAt string   `json:"next_eligible_at,omitempty"`
	UnclosedDepIDs []string `json:"unclosed_dep_ids,omitempty"`
	LastStatus     string   `json:"last_status,omitempty"`
	LastDetail     string   `json:"last_detail,omitempty"`
}

// BlockedBead pairs a bead with its blocker classification.
type BlockedBead struct {
	Bead
	Blocker Blocker `json:"blocker"`
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
