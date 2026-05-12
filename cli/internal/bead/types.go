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
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	Priority      int       `json:"priority"`
	IssueType     string    `json:"issue_type"`
	SchemaVersion int       `json:"schema_version"`
	Owner         string    `json:"owner,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`

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
	// StatusBlocked marks an explicit external blocker that can be rechecked.
	// Dependency waits are derived from dependency status and do not persist as
	// blocked.
	StatusBlocked = "blocked"
	// StatusProposed marks work awaiting operator approval or clarification.
	// It is outside worker dispatch until transitioned to open or cancelled.
	StatusProposed = "proposed"
	// StatusCancelled marks work that will not run and does not satisfy
	// dependents.
	StatusCancelled = "cancelled"
)

// CanonicalStatuses is the single source of truth for the persisted bead
// status enumeration. It mirrors the bd/br canonical set documented in
// TD-031 §2 and the JSON Schema enum at
// cli/internal/bead/schema/bead-record.schema.json. The CI guard tests in
// sync_test.go assert that schema, TD doc, and Go source agree with this
// list; adding or removing a value here without updating the schema and TD
// will fail CI.
var CanonicalStatuses = []string{
	StatusOpen,
	StatusInProgress,
	StatusClosed,
	StatusBlocked,
	StatusProposed,
	StatusCancelled,
}

// IsCanonicalStatus reports whether s is one of the persisted bead statuses
// in CanonicalStatuses.
func IsCanonicalStatus(s string) bool {
	for _, c := range CanonicalStatuses {
		if c == s {
			return true
		}
	}
	return false
}

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
	DefaultType          = "task"
	DefaultStatus        = StatusOpen
	DefaultPriority      = 2
	DefaultPrefix        = "bx" // used only when repo name detection fails
	CurrentSchemaVersion = 1
	MinPriority          = 0
	MaxPriority          = 4
)

// StatusCounts holds aggregate counts for a bead store.
type StatusCounts struct {
	Open              int `json:"open"`
	InProgress        int `json:"in_progress"`
	Closed            int `json:"closed"`
	Blocked           int `json:"blocked"`
	Proposed          int `json:"proposed"`
	Cancelled         int `json:"cancelled"`
	Ready             int `json:"ready"`
	NeedsHuman        int `json:"needs_human"`
	WorkerReady       int `json:"worker_ready"`
	DependencyWaiting int `json:"dependency_waiting"`
	ExternalBlocked   int `json:"external_blocked"`
	OperatorAttention int `json:"operator_attention"`
	Total             int `json:"total"`
}

// Blocker kinds surfaced through BlockedAll. These strings are part of the
// external DDx contract (HELIX reads them to decide how to handle a blocker).
const (
	BlockerKindDependency         = "dependency"
	BlockerKindBlockedStatus      = "blocked-status"
	BlockerKindRetryCooldown      = "retry-cooldown"
	BlockerKindNeedsInvestigation = "needs-investigation"
	BlockerKindOperatorAttention  = "operator-attention"
	BlockerKindNotEligible        = "not-execution-eligible"
	BlockerKindSuperseded         = "superseded"
	BlockerKindEpicOnly           = "epic-only"
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
	Reason         string   `json:"reason,omitempty"`
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
