---
ddx:
  id: plan-2026-05-11-bead-backend-interfaces
---
# Bead Backend Interface Refinement (Pre-Axon)

Date: 2026-05-11
Status: Design locked — final version after multi-round review (self-review, opus, operator). Ready for bead breakdown.

## Why this exists

The current `cli/internal/bead/backend.go` declares two interfaces — `RawBackend` (4 whole-corpus methods) and `Backend` (22 high-level CRUD methods). `*Store` actually exposes **69 public methods**; the other ~47 are concrete-only. The Axon-backend production-readiness work (per `docs/plans/plan-2026-05-10-axon-only-architecture.md`) needs a clean, LSP-clean interface set; today's setup conflates JSONL implementation choices with the bead's contract.

This design refactors the interface layer to:

- Separate **storage primitives** from **workflow operations** (per CLAUDE.md's "Platform Services in CLI, Opinions in Workflows").
- Apply LSP rigorously: interfaces match substitutability classes; the read-only deployment shape becomes expressible.
- Open optimization paths for non-JSONL backends (Axon, Lakebase Postgres) via typed `Operation` values.
- Thread `context.Context` through every interface method so authz, cancellation, and tracing don't require a second cross-cutting refactor.
- Make ID generation pluggable as a separate strategy interface.
- Preserve `*Store`'s public surface so existing callers don't break.

## Locked decisions

| # | Decision |
|---|----------|
| 1 | **No breaking changes to `*Store` callers.** `*Store` retains all 69 concrete public methods. The new interfaces are additive; existing code keeps compiling. |
| 2 | **Option B**: Axon implements `Backend` directly, alongside `*Store`'s composition path over `RawBackend`. |
| 3 | **Storage vs. workflow separation.** Storage primitives live on `Backend` sub-interfaces. Workflow operations (heartbeat policy, lifecycle state-machine, cancellation, cooldown, queue ordering) live as helpers in `cli/internal/bead/ops/<concern>/` that invoke `Apply` with typed `Operation` values. |
| 4 | **LSP-driven splits**: read/write split applied wherever a planned backend class falls on one side but not the other (the read-only deployment per `plan-2026-05-10-read-only-deployment.md`). |
| 5 | **Typed `Operation` pattern** for mutations: `Apply(ctx, id, op Operation)` is the single mutation entry point on `BeadLifecycle`. Operations carry their own `Apply(*Bead) error` method; storage backends MAY type-switch for native optimization (Axon SQL UPDATE) and fall through to load-mutate-save for unknown ops. CAS semantics (Claim) work because `Operation.Apply` returns an error. |
| 6 | **Pluggable ID generation**: `IDGenerator` is a separate parallel interface, not on `Backend`. Storage backends validate ids via the package-level `bead.ValidateID` contract; generators produce conforming ids. |
| 7 | **Subscription is parallel, not on `Backend`.** `*WatcherHub` already implements `SubscribeLifecycle` and `*Store` doesn't; modeling it as a sibling interface matches existing structure and keeps `Backend`'s shape request/response. |
| 8 | **`context.Context` first param on every interface method**, with a package-level discipline doc enumerating allowed ctx values (cancellation/deadline, `WithIdentity`, `WithTrace`) and rejecting per-call options via ctx. |
| 9 | **`RawBackend` retained, shape unchanged**, docstring updated to warn new backends off the whole-corpus pattern. |

## Caller usage evidence (driver of the split)

Counts of `store.<Method>(` references across non-test callers in `cli/cmd/` + `cli/internal/{server,agent,agentmetrics,exec,escalation,processmetrics}/`:

| Top callers by method | | Verdict |
|---|---|---|
| Create (152), Get (145), Init (105), Events (74), AppendEvent (44), Update (18), EventsByKind (12), Claim (11) | high-frequency cluster | Real CRUD + events + claim path |
| `cli/cmd/work_focus.go`, `cli/internal/agent/preview_queue.go`, `cli/internal/server/graphql/resolver_beads.go` | `BeadQueries` only | Real ISP narrowing candidate |
| `cli/cmd/ac.go`, `cli/cmd/try.go`, `cli/cmd/agent_route_status.go`, `cli/internal/agentmetrics/loader.go` | `BeadReader` only | Real ISP narrowing candidate |
| `cli/internal/server/workers.go` | `BeadEventWriter` + claim ops | Multiple narrow interfaces |
| `cli/cmd/bead.go`, `cli/internal/server/graphql/resolver_mutation_beads.go` | Full Backend | The full-feature callers |

The interface boundaries below match these clusters.

## Architecture: storage vs. workflow

```
┌─ workflow helpers (cli/internal/bead/ops/) ──────────────────────────┐
│                                                                       │
│  ops/claim/        ops/cancel/      ops/cooldown/                     │
│  ops/lifecycle/    ops/queue/       (state-machine validation,        │
│                                      time defaults, etc.)             │
│                                                                       │
│  Each helper: takes BeadLifecycle (or smaller), composes a typed      │
│  Operation, calls Apply. Plus pure predicates over *Bead.             │
└───────────────────────────────────────────────────────────────────────┘
                              │ Apply(ctx, id, op Operation) error
                              ▼
┌─ storage primitives (cli/internal/bead/) ─────────────────────────────┐
│                                                                       │
│  Backend = BeadInitializer + BeadReader + BeadLifecycle               │
│          + BeadEventReader + BeadEventWriter                          │
│          + BeadQueries                                                │
│          + BeadDependencyReader + BeadDependencyWriter                │
│          + BeadArchive                                                │
│          + BeadInterchangeReader + BeadInterchangeWriter              │
│                                                                       │
│  Parallel (not on Backend):                                           │
│    LifecycleSubscriber  (implemented by *WatcherHub)                  │
│    IDGenerator          (RandomHexIDGenerator, SequentialIDGenerator) │
└───────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─ backend implementations ─────────────────────────────────────────────┐
│                                                                       │
│  *Store (composes over RawBackend: JSONLBackend, ExternalBackend)     │
│  *AxonStore (implements Backend directly — per-row Postgres ops)      │
│  Read-only backends (implement only the Reader sub-interfaces)        │
└───────────────────────────────────────────────────────────────────────┘
```

## The `Operation` pattern

```go
package bead

// Operation is a typed mutation applied to a bead. Backends MAY recognize
// specific operation types and execute them efficiently (e.g. a single
// SQL UPDATE on Axon); the default path is Get(id) → op.Apply(bead) → Save(bead).
// op.Apply is the canonical in-memory semantic definition.
//
// Operation.Apply returns an error: returning non-nil aborts the storage
// write. This is how compare-and-swap (CAS) semantics work — Claim, for
// example, returns ErrAlreadyClaimed from Apply if the bead is held by
// another worker, and the storage rolls back the save.
type Operation interface {
    Apply(b *Bead) error
}

// Named operations are plain value types. Each carries the data it needs.

// CRUD-ish ops
type SetStatus            struct{ Status string }
type AppendNotes          struct{ Notes string }

// Claim CAS
type ClaimOp              struct{ Owner, Session, Worktree string }
type UnclaimOp            struct{ RequireOwner string }  // empty = unconditional

// Claim liveness — high-frequency, optimization target for Postgres backends
type SetClaimHeartbeat    struct{ At time.Time }
type ClearClaimHeartbeat  struct{}

// Cancellation
type SetCancelRequested   struct{ At time.Time }
type ClearCancelRequested struct{}

// Cooldown
type SetCooldown          struct{ Until time.Time; Status, Detail, BaseRev string }
type ClearCooldownOp      struct{}
type IncrNoChangesCount   struct{}

// Queue ordering (operator actions)
type QueueSetTop          struct{}
type QueueSetPosition     struct{ Position int }
type QueueClearOp         struct{}

// Workflow-aware lifecycle transitions (validation lives in the helper, not the op)
type SetLifecycleStatus   struct{ Status string; Options LifecycleTransitionOptions }
type SetCloseEvidence     struct{ SessionID, CommitSHA string }
type ReopenOp             struct{ Reason, Notes string }
type ParkToProposedOp     struct{ Reason string }

// MutateFunc is the ad-hoc escape hatch — any mutation not covered by a
// named op uses this. Backends always fall through to the generic path
// for MutateFunc operations.
type MutateFunc func(*Bead) error
func (m MutateFunc) Apply(b *Bead) error { return m(b) }
```

Each named op has a method like:

```go
func (op SetClaimHeartbeat) Apply(b *Bead) error {
    b.Claim.LastHeartbeat = op.At
    return nil
}

func (op ClaimOp) Apply(b *Bead) error {
    if b.Claim.Owner != "" && b.Claim.Owner != op.Owner {
        return ErrAlreadyClaimed
    }
    b.Claim.Owner = op.Owner
    b.Claim.Session = op.Session
    b.Claim.Worktree = op.Worktree
    b.Claim.At = time.Now()
    return nil
}

func (op UnclaimOp) Apply(b *Bead) error {
    if op.RequireOwner != "" && b.Claim.Owner != op.RequireOwner {
        return ErrNotClaimedByOwner
    }
    b.Claim = Claim{}
    return nil
}

func (op IncrNoChangesCount) Apply(b *Bead) error {
    b.NoChangesCount++
    return nil
}
```

### How `Apply` flows through `*Store` over `RawBackend`

`Backend.Apply` is the contract every backend honors. The implementation question is: in the composition path (`*Store` over `RawBackend`), where does the per-op optimization live?

**`*Store.Apply` is the universal entry point.** It does NOT know about backend-specific optimizations directly. Instead, it does a type-assertion on the wrapped `RawBackend` for an optional `OperationApplier` interface, and delegates if present; otherwise it falls back to the generic load-mutate-save path.

```go
// OperationApplier is an OPTIONAL contract a RawBackend MAY satisfy to
// provide per-op optimization. JSONLBackend implements it so heartbeat
// writes go to the sidecar file instead of churning the corpus.
// ExternalBackend does not implement it and falls through to the generic
// path in *Store.Apply.
//
// This is the only "hidden capability" interface in the design. It is
// scoped to the composition layer (*Store + RawBackend) and is not
// exposed on the public Backend contract — callers always go through
// Backend.Apply, which transparently uses the fast path when available.
type OperationApplier interface {
    Apply(ctx context.Context, id string, op Operation) error
}

// *Store.Apply — the universal mutation entry on the composition path.
func (s *Store) Apply(ctx context.Context, id string, op Operation) error {
    if fast, ok := s.raw.(OperationApplier); ok {
        return fast.Apply(ctx, id, op)
    }
    return s.raw.WithLock(func() error {
        beads, err := s.raw.ReadAll()
        if err != nil { return err }
        b := findByID(beads, id)
        if b == nil { return ErrNotFound }
        if err := op.Apply(b); err != nil { return err }     // refused; skip save
        return s.raw.WriteAll(beads)
    })
}
```

JSONLBackend (implements `OperationApplier`, type-switches for hot ops, falls through for the rest):

```go
func (j *JSONLBackend) Apply(ctx context.Context, id string, op Operation) error {
    switch op := op.(type) {
    case SetClaimHeartbeat:
        // Sidecar optimization: write to claim_liveness.go's separate file,
        // do NOT touch beads.jsonl. High-frequency heartbeats are decoupled
        // from corpus churn.
        return j.writeClaimSidecar(id, op.At)
    case ClearClaimHeartbeat:
        return j.removeClaimSidecar(id)
    default:
        // Generic path: load corpus, op.Apply in-memory, save corpus.
        return j.WithLock(func() error {
            beads, err := j.ReadAll()
            if err != nil { return err }
            b := findByID(beads, id)
            if b == nil { return ErrNotFound }
            if err := op.Apply(b); err != nil { return err }
            return j.WriteAll(beads)
        })
    }
}
```

Axon (implements `Backend` directly — no `*Store` wrapper — type-switches for hot ops, falls through for the rest):

```go
func (a *AxonStore) Apply(ctx context.Context, id string, op Operation) error {
    switch op := op.(type) {
    case ClaimOp:
        res, err := a.db.Exec(ctx,
            `UPDATE beads
             SET claim_owner=$1, claim_session=$2, claim_worktree=$3, claim_at=NOW()
             WHERE id=$4 AND (claim_owner IS NULL OR claim_owner=$1)`,
            op.Owner, op.Session, op.Worktree, id)
        if rowsAffected, _ := res.RowsAffected(); rowsAffected == 0 {
            return ErrAlreadyClaimed
        }
        return err
    case SetClaimHeartbeat:
        _, err := a.db.Exec(ctx, `UPDATE beads SET claim_heartbeat=$1 WHERE id=$2`, op.At, id)
        return err
    case IncrNoChangesCount:
        _, err := a.db.Exec(ctx, `UPDATE beads SET no_changes_count = no_changes_count + 1 WHERE id=$1`, id)
        return err
    case SetStatus:
        _, err := a.db.Exec(ctx, `UPDATE beads SET status=$1 WHERE id=$2`, op.Status, id)
        return err
    default:
        // Generic path: load, op.Apply in-memory, save
        return a.genericApply(ctx, id, op)
    }
}
```

A read-only HTTP backend rejects all Apply calls (it's read-only).

## Pluggable ID generation

```go
package bead

// ID format constraints — the contract every backend enforces.
const (
    DefaultIDPrefix = "ddx-"
    MaxIDLength     = 64
    MinIDLength     = 8
)
var idCharsetRe = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// ValidateID is the canonical contract check. Every backend's Create
// calls it; pluggable generators must produce ids that pass it; external
// importers can pre-check before constructing a *Bead.
func ValidateID(id string) error {
    if len(id) < MinIDLength || len(id) > MaxIDLength {
        return fmt.Errorf("%w: length %d not in [%d, %d]", ErrInvalidID, len(id), MinIDLength, MaxIDLength)
    }
    if !idCharsetRe.MatchString(id) {
        return fmt.Errorf("%w: charset", ErrInvalidID)
    }
    return nil
}

// IDGenerator is the pluggable strategy. Implementations are pure or
// in-memory; they do not depend on storage state.
type IDGenerator interface {
    GenID(ctx context.Context) (string, error)
}

type RandomHexIDGenerator struct {
    Prefix string
    Bytes  int  // hex byte count; 4 → "ddx-12abcdef"
}

func (g RandomHexIDGenerator) GenID(ctx context.Context) (string, error) {
    buf := make([]byte, g.Bytes)
    if _, err := rand.Read(buf); err != nil { return "", err }
    id := g.Prefix + hex.EncodeToString(buf)
    if err := ValidateID(id); err != nil { return "", err }  // sanity
    return id, nil
}

// SequentialIDGenerator is for tests and fixtures.
type SequentialIDGenerator struct {
    Prefix  string
    counter atomic.Int64
}

func (g *SequentialIDGenerator) GenID(ctx context.Context) (string, error) {
    n := g.counter.Add(1)
    return fmt.Sprintf("%s%08x", g.Prefix, n), nil
}

// NewIDGenerator returns the package default.
func NewIDGenerator() IDGenerator {
    return RandomHexIDGenerator{Prefix: DefaultIDPrefix, Bytes: 4}
}
```

## Sub-interface taxonomy

### Storage interfaces composing `Backend` (11)

```go
package bead

// BeadInitializer prepares a backend to serve subsequent calls.
// Implementations MAY create local resources (JSONL: mkdir+touch file),
// open network connections (HTTP-backed), or be no-ops (in-memory test).
// Callers MUST call Init exactly once before any other method on the
// returned backend.
type BeadInitializer interface {
    Init(ctx context.Context) error
}

// BeadReader is the foundational read surface — corpus reads and
// per-bead lookups. Implemented by every backend, including read-only.
type BeadReader interface {
    ReadAll(ctx context.Context) ([]Bead, error)
    ReadAllFiltered(ctx context.Context, pred func(Bead) bool) ([]Bead, error)
    Get(ctx context.Context, id string) (*Bead, error)
    GetWithArchive(ctx context.Context, id string) (*Bead, error)
}

// BeadLifecycle is the mutation primitives — exactly two methods.
// Create is the new-bead primitive (no prior state to load).
// Apply is the universal mutation entry point — every mutation flows
// through it as a typed Operation.
type BeadLifecycle interface {
    Create(ctx context.Context, b *Bead) error                       // ErrInvalidID, ErrConflict
    Apply(ctx context.Context, id string, op Operation) error        // any op error propagates
}

// BeadEventReader reads the per-bead event log.
type BeadEventReader interface {
    Events(ctx context.Context, id string) ([]BeadEvent, error)
    EventsByKind(ctx context.Context, id, kind string) ([]BeadEvent, error)
}

// BeadEventWriter appends to the per-bead event log. High-volume — every
// state transition writes an event.
type BeadEventWriter interface {
    AppendEvent(ctx context.Context, id string, event BeadEvent) error
}

// BeadQueries provides derived views over the corpus. All pure reads.
type BeadQueries interface {
    List(ctx context.Context, status, label string, where map[string]string) ([]Bead, error)
    ListWithArchive(ctx context.Context, status, label string, where map[string]string) ([]Bead, error)
    Ready(ctx context.Context) ([]Bead, error)
    ReadyExecution(ctx context.Context) ([]Bead, error)
    ReadyExecutionBreakdown(ctx context.Context) (ReadyExecutionBreakdown, error)
    ProposedOperatorAttention(ctx context.Context) ([]Bead, error)
    NeedsHuman(ctx context.Context) ([]Bead, error)
    Blocked(ctx context.Context) ([]Bead, error)
    ExternalBlocked(ctx context.Context) ([]Bead, error)
    DependencyWaiting(ctx context.Context) ([]Bead, error)
    BlockedAll(ctx context.Context) ([]BlockedBead, error)
    Status(ctx context.Context) (*StatusCounts, error)
}

// BeadDependencyReader reads the dependency graph.
type BeadDependencyReader interface {
    DepTree(ctx context.Context, rootID string) (string, error)
}

// BeadDependencyWriter mutates the dependency graph.
type BeadDependencyWriter interface {
    DepAdd(ctx context.Context, id, depID string) error
    DepRemove(ctx context.Context, id, depID string) error
}

// BeadArchive is corpus-transition maintenance — splitting closed beads
// out of the active corpus into an archive collection. Genuine storage
// transition (rows move between collections); kept as a storage primitive.
type BeadArchive interface {
    Archive(ctx context.Context, policy ArchivePolicy) ([]string, error)
    ArchiveWithEvents(ctx context.Context, policy ArchivePolicy) ([]string, error)
    Migrate(ctx context.Context) (MigrateStats, error)
}

// BeadInterchangeReader exports the corpus to JSONL.
type BeadInterchangeReader interface {
    ExportTo(ctx context.Context, w io.Writer) error
    ExportToFile(ctx context.Context, path string) error
}

// BeadInterchangeWriter imports from JSONL.
type BeadInterchangeWriter interface {
    Import(ctx context.Context, source, filePath string) (int, error)
}
```

### Composite `Backend`

```go
// Backend is the full bead-tracker contract — the composition of all 11
// storage sub-interfaces. Existing callers depending on Backend continue
// to compile. *Store satisfies Backend via the existing 69 public methods
// (compile-time check: var _ Backend = (*Store)(nil)).
//
// New callers should depend on the smallest sub-interface they use (ISP).
// New backends should implement Backend directly; *Store's composition
// over RawBackend is preserved for JSONLBackend and ExternalBackend only.
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
}

// ReadOnlyBackend is the capability bundle a multi-tenant read-only
// deployment requires (per docs/plans/plan-2026-05-10-read-only-deployment.md).
// A backend implementing this WITHOUT the writer interfaces is a valid
// read-only backend.
type ReadOnlyBackend interface {
    BeadInitializer
    BeadReader
    BeadEventReader
    BeadQueries
    BeadDependencyReader
    BeadInterchangeReader
}
```

### Parallel interfaces (NOT on Backend)

```go
// LifecycleSubscriber broadcasts bead lifecycle change events. Distinct
// from Backend because the operation shape (long-running stream,
// fire-and-forget delivery, polling/push goroutines) is fundamentally
// different from request/response storage ops.
//
// *WatcherHub (cli/internal/bead/watcher.go) satisfies this today via
// per-project polling. A future AxonStore could implement it natively
// via GraphQL subscriptions or Postgres LISTEN/NOTIFY.
type LifecycleSubscriber interface {
    SubscribeLifecycle(ctx context.Context, projectID string) (<-chan LifecycleEvent, func(), error)
}

// IDGenerator is the pluggable id-allocation strategy. Implementations
// produce ids that satisfy ValidateID. Default: RandomHexIDGenerator with
// the "ddx-" prefix. SequentialIDGenerator for deterministic tests.
// Implementations are pure or in-memory; they do not depend on storage.
type IDGenerator interface {
    GenID(ctx context.Context) (string, error)
}
```

## Methods explicitly NOT on any sub-interface

These stay concrete on `*Store` only — backwards compat — but are NOT part of any interface contract:

- **Workflow wrappers** (~17 methods that delegate to `Apply(op)` in the helpers): `Heartbeat`, `TouchClaimHeartbeat`, `RemoveClaimHeartbeat`, `ClaimHeartbeatFresh`, `Claim`, `ClaimWithOptions`, `Unclaim`, `RequestCancel`, `IsCancelRequested`, `MarkCancelHonored`, `SetExecutionCooldown`, `ClearCooldowns`, `IncrNoChangesCount`, `TransitionLifecycle`, `SetLifecycleStatus`, `UpdateWithLifecycleStatus`, `CloseWithEvidence`, `AppendNotes`, `Reopen`, `ParkToProposed`, `Update`, `Close`, `QueueTop`, `QueueMove`, `QueueClear`, `GenID`.
- **Migration/admin** (one-time operations): `MigrateLifecycle`, `MigrateFromHelix`, `MigrateToAxon`, `MigrateDryRun`, `MigrateLifecycleDryRun`, `DetectLifecycleMigrationRequired`, `ReconcileLifecycleMetadata`.
- **Lifecycle schema marker** (bootstrap): `HasLifecycleSchemaMarker`, `WriteLifecycleSchemaMarker`, `LifecycleSchemaMarkerPath`. Axon handles schema versioning out-of-band via Lakebase migrations; these stay JSONL-only.
- **JSONL-internal**: `LoadEventsInline`.
- **Infrastructure**: `WithLock` (already on `RawBackend`).

New code uses the helper packages and the new sub-interfaces. Existing code keeps working unchanged via the concrete `*Store` methods.

## Helper packages

```
cli/internal/bead/ops/
  claim/
    liveness.go       // Touch, Clear, IsFresh(*Bead, maxAge), Acquire, Release
  cancel/
    cancel.go         // Request, MarkHonored, IsRequested(*Bead)
  cooldown/
    cooldown.go       // Set, ClearAll(filter), IncrNoChanges
  lifecycle/
    transition.go     // Transition (state-machine validation), Reopen, ParkToProposed, CloseWithEvidence, AppendNotes
  queue/
    queue.go          // Top, Move, Clear
```

Example:

```go
package claim

func Touch(ctx context.Context, l bead.BeadLifecycle, id string) error {
    return l.Apply(ctx, id, bead.SetClaimHeartbeat{At: time.Now()})
}

func IsFresh(b *bead.Bead, maxAge time.Duration) bool {
    return time.Since(b.Claim.LastHeartbeat) < maxAge
}

type AcquireOptions struct{ Owner, Session, Worktree string }

func Acquire(ctx context.Context, l bead.BeadLifecycle, id string, opts AcquireOptions) error {
    return l.Apply(ctx, id, bead.ClaimOp(opts))
}

func Release(ctx context.Context, l bead.BeadLifecycle, id string) error {
    return l.Apply(ctx, id, bead.UnclaimOp{})
}
```

```go
package lifecycle

// validTransitions encodes the HELIX state machine
var validTransitions = map[string][]string{ /* ... */ }

func Transition(ctx context.Context, l bead.BeadLifecycle, r bead.BeadReader, id, newStatus string) error {
    b, err := r.Get(ctx, id)
    if err != nil { return err }
    if !contains(validTransitions[b.Status], newStatus) {
        return fmt.Errorf("invalid transition %s → %s for %s", b.Status, newStatus, id)
    }
    return l.Apply(ctx, id, bead.SetLifecycleStatus{Status: newStatus})
}
```

## `context.Context` discipline

```go
// Package bead defines what is allowed on context.Context values passed
// to Backend methods. The set is closed:
//
//   - context.WithCancel / WithDeadline / WithTimeout from any caller
//   - bead.WithIdentity(ctx, Identity) — caller's authenticated identity
//   - bead.WithTrace(ctx, trace.Span) — OpenTelemetry span for the op
//
// NOT ALLOWED on ctx (anti-patterns):
//   - bead.Backend instance (use a function argument)
//   - per-call options like "include archived", "force refresh" — those
//     belong on method signatures (see GetWithArchive, ListWithArchive)
//   - mutable state, logger references, configuration
//
// A backend implementation that reads a context value outside this set
// is using the wrong door. Lint rule: ctx.Value() calls in this package
// must go through a typed accessor.
//
// Typed accessors:
//   func WithIdentity(ctx context.Context, id Identity) context.Context
//   func IdentityFromContext(ctx context.Context) (Identity, bool)
//   func WithTrace(ctx context.Context, span trace.Span) context.Context
//   func TraceFromContext(ctx context.Context) (trace.Span, bool)
```

## Sentinel errors

```go
package bead

var (
    ErrNotFound            = errors.New("bead: not found")
    ErrConflict            = errors.New("bead: id already exists")
    ErrInvalidID           = errors.New("bead: invalid id")
    ErrAlreadyClaimed      = errors.New("bead: already claimed by another owner")
    ErrNotClaimedByOwner   = errors.New("bead: not claimed by requesting owner")
    ErrUnsupported         = errors.New("bead: operation not supported by this backend")
)
```

Each `Backend` method documents which sentinels it returns. Callers discriminate via `errors.Is`.

## `RawBackend` retained with warning

```go
// RawBackend is the low-level whole-corpus storage contract used by
// JSONLBackend and ExternalBackend (bd/br). It exists for backends where
// corpus-shaped read/write is the natural granularity (single file with
// atomic rename + advisory lock).
//
// NEW BACKENDS SHOULD NOT IMPLEMENT RawBackend. The whole-corpus shape
// is wrong for any backend supporting per-row operations (Postgres, any
// structured store). New backends should implement Backend directly —
// see AxonStore as the reference example. *Store's composition over
// RawBackend is preserved only for the existing JSONL / External impls.
type RawBackend interface {
    Init() error
    ReadAll() ([]Bead, error)
    WriteAll(beads []Bead) error
    WithLock(fn func() error) error
}
```

## Acceptance criteria

1. New file `cli/internal/bead/interfaces.go` (or sub-interfaces split across files) declares the 11 storage sub-interfaces, the composite `Backend`, `ReadOnlyBackend`, the parallel `LifecycleSubscriber` and `IDGenerator`. All methods take `ctx context.Context` as first param.
2. New file `cli/internal/bead/operation.go` declares the `Operation` interface and ~20 named operation types, each implementing `Apply(*Bead) error`.
3. New file `cli/internal/bead/id.go` declares `ValidateID`, `RandomHexIDGenerator`, `SequentialIDGenerator`, `NewIDGenerator`, format constants.
4. New file `cli/internal/bead/context.go` declares `WithIdentity`/`IdentityFromContext`/`WithTrace`/`TraceFromContext` typed accessors.
5. New file `cli/internal/bead/errors.go` declares the sentinel errors.
6. `*Store` keeps all 69 concrete public methods. Each existing concrete method retains its current signature (NO ctx breaking changes to `*Store`). Compile-time assertions:
   ```go
   var _ Backend = (*Store)(nil)
   ```
   plus per-sub-interface assertions for `*Store`. **Routing verification test** `TestStore_HeartbeatRoutesThroughApply`: instrument an in-memory `RawBackend` that records every `Apply(...)` call against it; wrap in `*Store`; call `*Store.Heartbeat(ctx, id)`; assert exactly one recorded `Apply(ctx, id, SetClaimHeartbeat{...})` call. Same pattern for `*Store.Claim`/`Unclaim`/`RequestCancel`/`SetExecutionCooldown`/etc. — each concrete method must land on `*Store.Apply` (which delegates per below).

7. `*Store` gains a new ctx-aware `Apply(ctx, id, op Operation) error` method that **type-asserts the wrapped `RawBackend` to `OperationApplier`** (the optional interface defined in §"How Apply flows through *Store over RawBackend") and delegates if available; otherwise falls back to the generic `WithLock + ReadAll + op.Apply + WriteAll` path. Existing concrete methods (`Heartbeat`, `Claim`, `RequestCancel`, `TransitionLifecycle`, etc.) are internally rewritten to call `*Store.Apply` with the appropriate `Operation` value. Observable behavior unchanged. **JSONLBackend implements `OperationApplier`**: for `SetClaimHeartbeat` and `ClearClaimHeartbeat` it writes only the sidecar file (matching today's `claim_liveness.go` behavior); for all other ops it falls through to its generic load-mutate-save path. Test `TestJSONLBackend_Apply_SetClaimHeartbeat_UsesSidecar`: call `JSONLBackend.Apply(ctx, id, SetClaimHeartbeat{At: t})` and assert the sidecar file is touched while `beads.jsonl` is NOT modified (compare mtime). Combined with AC #6's routing test, the full chain `*Store.Heartbeat → *Store.Apply → JSONLBackend.Apply → sidecar` is verified end-to-end.
8. `*WatcherHub` satisfies `LifecycleSubscriber`; compile-time assertion added. Signature updated to take `ctx`.
9. Helper packages under `cli/internal/bead/ops/{claim,cancel,cooldown,lifecycle,queue}/` are created with the helpers and predicates listed above. Each helper has at least one test that exercises it against `*Store`.
10. `RawBackend` docstring updated with the warning text.
11. Caller migration: callers in `cli/cmd/`, `cli/internal/server/`, `cli/internal/agent/`, `cli/internal/escalation/`, `cli/internal/agentmetrics/`, `cli/internal/processmetrics/`, `cli/internal/exec/` that currently call `*Store` workflow methods (`Heartbeat`, `Claim`, `TransitionLifecycle`, etc.) are NOT modified — backwards compat. New code added during this work uses helpers + ctx-aware interfaces.
12. Conformance tests under `cli/internal/bead/` are parameterized to run against any `Backend` implementation. `*Store` (via JSONL and External RawBackends) passes. The suite is structured so AxonStore can join later.
13. `cd cli && go test ./...` is green.
14. `lefthook run pre-commit` passes.

Test names: `TestBackendConformance_*` (parameterized by backend), `TestOperation_*` (per-op semantics), `TestValidateID_*`, `TestRandomHexIDGenerator_*`, `TestSequentialIDGenerator_*`, `TestApply_ClaimCAS_PreventsDoubleClaim`, `TestApply_UnclaimWithOwner_RejectsOtherOwner`.

## Sequencing

This bead is the gate. It must land before downstream Axon beads can reopen.

After this bead:

1. **Reopen `ddx-9c5bca8f`** rescoped: implement `Backend` directly on `AxonStore` using per-row Postgres operations via `Apply`'s type-switch. AC: AxonStore passes the parameterized conformance suite.
2. **Reopen `ddx-29f02cf4`** rescoped: `NewStore(opts)` factory returns the right `Backend` impl from config; `WithAxonGraphQLTransport` constructed from config; required-capabilities validated at startup.
3. **Reopen `ddx-8bf23be0`** rescoped: reconcile `schema.graphql` with the per-row ops AxonStore actually invokes.
4. **Reopen `ddx-958b8fc3`** rescoped: parameterized conformance against an httptest-served Axon-shaped GraphQL endpoint.
5. **Reopen `ddx-8dd19492`** rescoped: subscription smoke test against `LifecycleSubscriber` natively implemented by Axon (when present); falls back to `*WatcherHub` polling.
6. **Reopen `ddx-8d747049`** (parent epic) — refresh context, drop `blocked-on-upstream` labels.
7. **New bead**: schema versioning + v0→v1 migration ladder (audit gap 4).
8. **New bead**: real JSONL→Postgres importer to replace `MigrateToAxon`'s current JSONL output (audit gap 6).
9. **New bead**: real-wire integration tests against an actual Axon/Postgres instance (audit gap 7).

## Risks

| Risk | Mitigation |
|------|------------|
| `*Store` concrete-method count grows large alongside the new interfaces (69 + new Apply + new helpers) | Acknowledged. The duplication is the cost of backwards compat. New code uses helpers; over time, concrete-method callers can be narrowed (separate, opportunistic work). |
| `Operation` type-switch in AxonStore drifts from named-op set (forgotten ops fall through to generic path) | A unit test enumerates all `Operation` types in the `bead` package via reflection and asserts AxonStore type-switches on each. New op without optimization = lint warning, not a deploy failure. |
| Heartbeat sidecar optimization (JSONL writes a separate file today) loses ground under the Apply pattern | `JSONLBackend.Apply` (via the optional `OperationApplier` interface) detects `SetClaimHeartbeat` and writes only the sidecar — same optimization, just driven by the typed op now. `*Store.Apply` delegates via type-assertion. Combined tests `TestStore_HeartbeatRoutesThroughApply` (routing) + `TestJSONLBackend_Apply_SetClaimHeartbeat_UsesSidecar` (sidecar) verify end-to-end. |
| Operation type evolution — adding a field to `SetClaimHeartbeat` later breaks backends type-switching on the old struct shape | Additive field changes are safe: backends type-switch on the struct type, not its field set; new fields are zero-valued when ignored. Removing or renaming fields IS breaking — operation types are public API once first shipped. Treat them like database schema: add freely, remove only with care. Documented in package doc on `cli/internal/bead/operation.go`. |
| New backend contributor doesn't know what operations to optimize | Canonical catalog lives in `cli/internal/bead/operation.go` package doc; `go doc bead` produces it. Conformance test `TestOperationCatalog_AxonStoreSwitchCoverage` (in the AxonStore bead `ddx-9c5bca8f`) enumerates via reflection and fails when AxonStore is missing a case — informative failure message points to the catalog. |
| Caller migration to ctx-aware interfaces is large scope | Out of scope for this bead. New code uses helpers. Existing callers continue with concrete `*Store` methods. A follow-up "caller narrowing" bead can happen incrementally. |
| Helper packages duplicate logic that should live in HELIX (workflow plugin) | Acknowledged. The helpers in `cli/internal/bead/ops/lifecycle/` encode HELIX state-machine rules. A future architectural improvement is to move lifecycle/ to the HELIX plugin and let `cli/internal/bead/ops/` hold only generic-bead workflow helpers. Out of scope here. |
| Adding `ctx` to interface methods while `*Store` keeps non-ctx signatures means `*Store` doesn't actually satisfy the new interfaces | Approach: add new ctx-aware methods to `*Store` as parallel wrappers (e.g. `*Store.GetCtx(ctx, id)` calls `*Store.Get(id)`). The new sub-interfaces are satisfied by the ctx-aware variants. Existing concrete methods remain for non-ctx callers. ~50 wrapper methods, mechanical. Alternative: rewrite `*Store` methods to take ctx and migrate the ~27 caller files in same bead. Decision: parallel-wrapper approach (mechanical, no caller breakage). |
| Operation discoverability — how do contributors know what ops exist? | All ops live in one file `cli/internal/bead/operation.go`. Package doc lists them. `go doc bead` shows the catalog. |

## Open questions

1. **`*Store` rewrites concrete workflow methods to call `Apply(op)` internally** (resolved): AC #6 requires the rewrite, plus AC #7 + the routing test verify it. Optimization lives only in `JSONLBackend.Apply` (via `OperationApplier`); concrete methods don't duplicate it.
2. **`SequentialIDGenerator` — does it belong in production or `testdata`?** Recommended: production code, since it's useful for fixtures and integration tests run from production binaries.
3. **Subscription failure mode** — when a backend can't subscribe, `SubscribeLifecycle` returns `(nil, nil, ErrUnsupported)`. Confirm callers handle this gracefully (fall back to polling vs. fail).
4. **Helper package scope** — `cli/internal/bead/ops/lifecycle/` encodes HELIX state-machine rules. Future architectural improvement: move this to the HELIX plugin so generic bead ops stay workflow-agnostic. Acknowledge here; defer.

## Bottom line

This refactor turns the bead backend from a fat 22-method interface (`Backend`) with 47 concrete-only `*Store` methods into a clean separation:

- **11 LSP-clean sub-interfaces** on `Backend`, all ctx-aware, focused on storage primitives.
- **A typed `Operation` pattern** for mutations that supports CAS via error returns and lets non-JSONL backends optimize via type-switch.
- **A pluggable `IDGenerator`** strategy with default and test implementations.
- **A parallel `LifecycleSubscriber`** matching the existing `*WatcherHub` structure.
- **Workflow helpers** in `cli/internal/bead/ops/<concern>/` for state-machine validation, heartbeat policy, cancellation, cooldown, queue ordering.
- **Zero breaking changes** to existing `*Store` callers.

This unblocks the Axon production-readiness work (reopen of ~6 closed beads + 3 new ones per `plan-2026-05-10-axon-only-architecture.md`) and gives the read-only deployment shape a clean interface set to declare its capability bundle against.
