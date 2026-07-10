package bead

import (
	"context"
	"io"
	"time"
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

// Backend is the high-level bead-tracker contract. It is the TD-027
// composition of the public storage sub-interfaces below, plus the existing
// mutable operations that the rest of the repo still calls directly.
type Backend interface {
	BeadInitializer
	BeadReader
	BeadLifecycle
	BeadEventReader
	BeadEventWriter
	BeadQueries
	BeadDependencyReader
	BeadDependencyWriter
	BeadArchive
	BeadInterchangeReader
	BeadInterchangeWriter

	Update(ctx context.Context, id string, mutate func(*Bead)) error
	Close(ctx context.Context, id string) error
	Claim(id, assignee string) error
	ClaimWithOptions(id, assignee, session, worktree string) error
	Unclaim(id string) error
}

// TD-027 foundation interfaces. These are the public storage contracts
// composed by Backend and ReadOnlyBackend.
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
	Apply(id string, op Operation) error
}

type BeadEventReader interface {
	Events(id string) ([]BeadEvent, error)
	EventsByKind(id, kind string) ([]BeadEvent, error)
}

type BeadEventWriter interface {
	AppendEvent(id string, event BeadEvent) error
}

type BeadQueries interface {
	List(status, label string, where map[string]string) ([]Bead, error)
	Ready() ([]Bead, error)
	Blocked() ([]Bead, error)
	ReadyExecutionBreakdown() (ReadyExecutionBreakdown, error)
	ProposedOperatorAttention() ([]Bead, error)
	NeedsHuman() ([]Bead, error)
	ExternalBlocked() ([]Bead, error)
	DependencyWaiting() ([]Bead, error)
	BlockedAll() ([]BlockedBead, error)
	Status() (*StatusCounts, error)
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

// LifecycleEvent is emitted by a LifecycleSubscriber when a bead is created
// or updated.
type LifecycleEvent struct {
	EventID   string
	BeadID    string
	Kind      string // "created", "status_changed", "updated"
	Summary   string
	Body      string
	Actor     string
	Timestamp time.Time
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
