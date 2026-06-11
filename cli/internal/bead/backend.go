package bead

import (
	"context"
	"io"
)

// RawBackend is the low-level storage contract — read/write the entire bead
// corpus and serialize concurrent rewrites. JSONLBackend and ExternalBackend
// implement it. New backends should implement Backend directly; RawBackend is
// retained for the JSONL/external composition path that Store uses internally.
// Higher-level operations (CRUD, claim, ready/blocked, dep ops, events,
// archive, JSONL interchange) live on the Backend interface below and are
// composed on top of a RawBackend by Store.
type RawBackend interface {
	Init() error
	ReadAll() ([]Bead, error)
	WriteAll(beads []Bead) error
	WithLock(fn func() error) error
}

// Backend is the high-level bead-tracker contract that callers in
// cli/cmd/bead_*.go and the agent loop will eventually program against. It
// covers everything the bead description for ddx-bbdd7564 calls out: CRUD,
// claim, list/ready/blocked, dep ops, events append, archive split, and
// JSONL export/import.
//
// *Store satisfies this interface; the JSONL implementation lives in store.go
// and delegates per-write serialization to the configured RawBackend
// (JSONLBackend by default, ExternalBackend when DDX_BEAD_BACKEND or the
// .ddx/config.yaml beads.backend field selects bd/br).
//
// chaos_test.go and other conformance suites should program against this
// interface so additional backends can be exercised by the same tests.
type Backend interface {
	// Foundational
	Init(ctx context.Context) error
	ReadAll(ctx context.Context) ([]Bead, error)

	// CRUD
	Create(ctx context.Context, b *Bead) error
	Get(ctx context.Context, id string) (*Bead, error)
	Update(ctx context.Context, id string, mutate func(*Bead)) error
	Close(ctx context.Context, id string) error

	// Claim
	Claim(id, assignee string) error
	ClaimWithOptions(id, assignee, session, worktree string) error
	Unclaim(id string) error

	// Query
	List(status, label string, where map[string]string) ([]Bead, error)
	Ready() ([]Bead, error)
	Blocked() ([]Bead, error)

	// Dep ops
	DepAdd(ctx context.Context, id, depID string) error
	DepRemove(ctx context.Context, id, depID string) error
	DepTree(ctx context.Context, rootID string) (string, error)

	// Events
	AppendEvent(id string, event BeadEvent) error
	Events(id string) ([]BeadEvent, error)

	// Archive split
	Archive(policy ArchivePolicy) ([]string, error)
	Migrate() (MigrateStats, error)

	// JSONL interchange
	Import(source, filePath string) (int, error)
	ExportTo(w io.Writer) error
}

// TD-027 foundation interfaces. These are additive and intentionally do not
// change the legacy Store-backed interface above in this bead slice.
type BeadInitializer interface {
	Init(ctx context.Context) error
}

type BeadReader interface {
	ReadAll(ctx context.Context) ([]Bead, error)
	ReadAllFiltered(ctx context.Context, pred func(Bead) bool) ([]Bead, error)
	Get(ctx context.Context, id string) (*Bead, error)
}

type BeadLifecycle interface {
	Create(ctx context.Context, b *Bead) error
	Apply(ctx context.Context, id string, op Operation) error
}

type BeadEventReader interface {
	Events(ctx context.Context, id string) ([]BeadEvent, error)
	EventsByKind(ctx context.Context, id, kind string) ([]BeadEvent, error)
}

type BeadEventWriter interface {
	AppendEvent(ctx context.Context, id string, event BeadEvent) error
}

type BeadQueries interface {
	List(ctx context.Context, status, label string, where map[string]string) ([]Bead, error)
	Ready(ctx context.Context) ([]Bead, error)
	Blocked(ctx context.Context) ([]Bead, error)
	ReadyExecutionBreakdown(ctx context.Context) (ReadyExecutionBreakdown, error)
	ProposedOperatorAttention(ctx context.Context) ([]Bead, error)
	NeedsHuman(ctx context.Context) ([]Bead, error)
	ExternalBlocked(ctx context.Context) ([]Bead, error)
	DependencyWaiting(ctx context.Context) ([]Bead, error)
	BlockedAll(ctx context.Context) ([]BlockedBead, error)
	Status(ctx context.Context) (*StatusCounts, error)
}

type BeadDependencyReader interface {
	DepTree(ctx context.Context, rootID string) (string, error)
}

type BeadDependencyWriter interface {
	DepAdd(ctx context.Context, id, depID string) error
	DepRemove(ctx context.Context, id, depID string) error
}

type BeadArchive interface {
	Archive(ctx context.Context, policy ArchivePolicy) ([]string, error)
	Migrate(ctx context.Context) (MigrateStats, error)
}

type BeadInterchangeReader interface {
	ExportTo(ctx context.Context, w io.Writer) error
}

type BeadInterchangeWriter interface {
	Import(ctx context.Context, source, filePath string) (int, error)
}

type ReadOnlyBackend interface {
	BeadInitializer
	BeadReader
	BeadEventReader
	BeadQueries
	BeadDependencyReader
	BeadInterchangeReader
}

// LifecycleSubscriber is the parallel subscription surface from TD-027.
type LifecycleSubscriber interface {
	SubscribeLifecycle(ctx context.Context, projectID string) (<-chan LifecycleEvent, func(), error)
}

// OperationApplier is an optional fast path for RawBackend implementations
// that can preserve specialized mutation behavior.
type OperationApplier interface {
	Apply(ctx context.Context, id string, op Operation) error
}

// BackendType constants
const (
	BackendJSONL = "jsonl"
	BackendBD    = "bd"
	BackendBR    = "br"
)

const DefaultCollection = "beads"
