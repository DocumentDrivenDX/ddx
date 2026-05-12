---
ddx:
  id: TD-027
  depends_on:
    - ADR-004
    - SD-004
    - FEAT-004
    - TD-004
  related:
    - TD-031
---
# Technical Design: Bead Storage System and Lifecycle

## Purpose

This is the canonical technical design for the bead storage system and lifecycle. It specifies:

1. The persisted bead status enumeration and transition matrix (§1–§4).
2. The storage interface taxonomy — sub-interfaces, the `Operation` pattern, ID generation, context discipline, sentinel errors (§5–§10).
3. The Bead data model — field enumeration, universal invariants, JSONL wire format, schema-versioning policy, operation-vs-field-mutation rules (§11).
4. Bead semantics — claim resolution, event log, workflow helper packages (§12–§14).
5. Collection registry, archival policy, attachment layout, migration plan, read-path semantics, bd/br interchange compatibility (§15–§20).
6. Module-boundary architecture that makes bypass physically impossible after caller migration completes (§21).
7. Process and maintenance — future-change rule, acceptance criteria, risks, open questions (§22–§25).

**Scope:** bead storage system and lifecycle. **Out of scope:** how the drain loop / executor consumes the bead lifecycle (outcome→state mapping, hygiene-bead contracts, worker state, auto-recovery dispatch policy). That operational contract lives in [TD-031: Drain-Loop Operational Contract over Beads](TD-031-bead-state-machine.md).

**Related docs:**
- [FEAT-004](../../01-frame/features/FEAT-004-beads.md) — feature scope; user-visible behavior.
- [ADR-004](../adr/ADR-004-bead-backed-runtime-storage.md) — decision: use bead-backed collections.
- [SD-004](../solution-designs/SD-004-beads-tracker.md) — runtime/storage behavior; file layout.
- [TD-004](TD-004-beads-claims-evidence.md) — execution-evidence subsystem (tightened from earlier scope; claims content folded into §12 here).
- [TD-031](TD-031-bead-state-machine.md) — drain-loop operational contract over beads.

## Critical Constraint (per ADR-004)

The bead-record envelope must remain compatible with the bd/br interchange contract. The persisted `status` enum is **fixed** at the bd/br canonical six values:

```
open, in_progress, closed, blocked, proposed, cancelled
```

DDx-specific execution semantics live in **labels**, **events**, or the preserved-extras **Extra** map — never in new statuses. Adding a new persisted status requires upstream bd/br coordination plus an ADR-004 amendment. This TD does not authorize any such addition.

---

# Part I — Bead State

## 1. Persisted Status Enumeration

The persisted bead status is exactly:

```
open | in_progress | closed | blocked | proposed | cancelled
```

This is the bd/br-canonical set; it matches `cli/internal/bead/schema/bead-record.schema.json`.

Plain-English semantics:

- `open` — accepted active work. No claim is active. Includes work waiting on dependencies; dependency waiting is derived from the dependency DAG while persisted status remains `open`.
- `in_progress` — the bead has an active claim (`assignee`, `claimed-at`, `claimed-pid` populated). Drain loop or operator has taken ownership.
- `closed` — terminal satisfied work. A `closed` dependency satisfies downstream beads. The reason for closure is encoded in a closing event and/or label; the status itself does not carry the reason.
- `blocked` — accepted work paused by a rare external, recheckable blocker. Dependency waits are not `status=blocked`; they are derived from unsatisfied dependencies while the bead remains `open`.
- `proposed` — operator decision required. Proposed beads are not autonomous-work eligible until an operator accepts, rewrites, splits, waives, or cancels them.
- `cancelled` — terminal not-doing. Distinct from `closed`: `cancelled` does not satisfy dependents unless a later explicit dependency policy says otherwise.

## 2. Transition Matrix

| From → To | Allowed? | Driver | Event fired |
|---|---|---|---|
| `proposed` → `open` | yes | operator acceptance (`ddx bead update --status open`) | `triaged` |
| `proposed` → `cancelled` | yes | operator | `cancelled` |
| `open` → `proposed` | yes | readiness or operator found missing decision input | `triage-ambiguous` / `review-manual-required` |
| `open` → `in_progress` | yes | drain loop or operator (claim) | `claimed` |
| `open` → `blocked` | yes | operator or auto-triage found an external recheckable blocker | `blocked` |
| `open` → `cancelled` | yes | operator | `cancelled` |
| `in_progress` → `open` | yes | unclaim (operator or stale-claim sweep) | `unclaimed` |
| `in_progress` → `closed` | yes | drain loop (on merge/already-satisfied) or operator | `closed-merged` / `closed-already-satisfied` |
| `in_progress` → `proposed` | yes | non-automatable review/readiness finding or exhausted repair/review budget | `review-block` / `review-manual-required` |
| `in_progress` → `blocked` | yes | drain loop or operator found an external recheckable blocker | `blocked` |
| `blocked` → `open` | yes | operator (block resolved) | `unblocked` |
| `blocked` → `cancelled` | yes | operator | `cancelled` |
| `closed` → * | no | — | — (closed is terminal) |
| `cancelled` → * | no | — | — (cancelled is terminal) |

Closed and cancelled are terminal. `closed` satisfies dependency edges; `cancelled` does not satisfy dependency edges unless a future dependency policy explicitly defines an exception. Re-opening a closed bead is not a transition; it is filing a follow-up bead with `replaces` set.

`triaged` is the operator-acceptance signal for `proposed → open`. Readiness idempotency uses that signal to avoid re-parking the same bead for the same rule or finding unless prompt-relevant fields changed or the operator explicitly requests re-triage.

### 2.1 Derived Queue Buckets

Persisted status is the sole DDx-owned lifecycle field. Queue buckets are computed read models over status, dependency edges, claim metadata, and preserved Extra fields:

- `execution-ready` — `status=open`, no active claim, every dependency is `closed`, and no execution-suppressing metadata is present.
- `dependency-waiting` — `status=open` with at least one dependency that is not `closed`; this is derived waiting, not `status=blocked`.
- `operator-review` — `status=proposed`; these beads require an operator decision before autonomous execution.
- `externally-blocked` — `status=blocked`; the blocker must be external and recheckable.
- `active` — `status=in_progress`; claim metadata names the current owner.
- `terminal-satisfied` — `status=closed`; this satisfies dependency edges.
- `terminal-not-doing` — `status=cancelled`; this does not satisfy dependency edges unless a future dependency policy explicitly says otherwise.

## 3. Category Taxonomy

Every state-machine name observed in DDx falls into exactly one of the following categories. The category determines where the name is allowed to live.

| Category | Storage location | Lifecycle | Owner |
|---|---|---|---|
| Persisted bead status | `status` field on the bead record | Mutated by atomic snapshot rewrite | Bead store, locked to bd/br set |
| Derived queue category | Computed on read from status + deps + preserved metadata | Never persisted | Queue derivation code (`ddx bead ready/blocked/status`) |
| Event kind | Append-only entry in `Extra["events"][].kind` | Append-only; explains state, does not control lifecycle | Drain loop, agent service, CLI |
| Terminal phase | A persisted `closed` status plus a closing event/label that names *why* | Mutated once on close | Drain loop / CLI |
| Claim metadata | `assignee`, `claimed-at`, `claimed-pid` fields (preserved extras) | Set on claim, cleared on unclaim, expired by triage | Claim resolution path (§12) |
| Label | Entry in the `labels` array | Explains or filters state; never controls lifecycle | Anyone with `ddx bead update` |
| Extra metadata field | Arbitrary key under preserved extras | Explains or filters state; never controls lifecycle | Subsystem owning that key |
| Worker state | In-memory state of the drain process | Lives only for the worker's lifetime | Drain loop process (see TD-031) |

A name MUST NOT span categories. If a name today appears as both a label and a derived queue category, reconciliation removes the duplicate usage.

## 4. Naming-Role Decision Matrix

Every name observed in code, schema, docs, or persisted data is assigned a single category. Names not in the persisted-status set MUST NOT appear as `status` values.

| Name | Category | Rationale |
|---|---|---|
| `open` | Persisted status | bd/br canonical; queue-eligible default. |
| `in_progress` | Persisted status | bd/br canonical; implies an active claim. |
| `closed` | Persisted status | bd/br canonical; terminal-success path. |
| `blocked` | Persisted status | bd/br canonical; accepted work paused by an external recheckable blocker. |
| `proposed` | Persisted status | bd/br canonical; operator decision required before autonomous work. |
| `cancelled` | Persisted status | bd/br canonical; terminal not-doing path that does not satisfy dependents. |
| `done` | Removed alias | Historical alias of `closed`. Not a persisted status. |
| `pending` / `waiting` | Derived queue category | Used by queue derivation to mean "open AND has unmet deps". Never persisted. |
| `ready` | Derived queue category | "open AND no unmet deps AND not claimed". Computed; never persisted. |
| `review` | Terminal phase / event | A *phase of work*, not a status. Implemented as the `review-block` / `review-pass` event pair plus review evidence. |
| `needs_human` | Legacy/backcompat label | Migration-only signal formerly used for operator intervention. New routing uses `status=proposed`. |
| `needs_investigation` | Legacy/backcompat label | Migration-only signal; new routing uses `status=proposed` when operator action is required. |
| `blocked-on-upstream:<id>` | Label | Parameterized label naming an external upstream blocker. Distinct from derived dependency waiting and from the `blocked` status. |
| `decomposed` | Label | Set by drain on decomposition outcomes; pairs with `Extra["children"]`. |
| `triage` | Label | Set by drain on triage outcomes. |
| `idle` / `draining` / `paused-quota` / `paused-rate-limit` / `exiting` | Worker state | Lives in the drain loop process (see TD-031). Not on the bead. |

---

# Part II — Storage Architecture

## 5. Storage vs. Workflow Separation

Per CLAUDE.md's "Platform Services in CLI, Opinions in Workflows": storage primitives live on `Backend` sub-interfaces; workflow operations (heartbeat policy, lifecycle state-machine validation, cancellation, cooldown, queue ordering) live as helpers in `cli/internal/bead/ops/<concern>/` that invoke `Apply` with typed `Operation` values.

```
┌─ workflow helpers (cli/internal/bead/ops/) ──────────────────────────┐
│  ops/claim/        ops/cancel/      ops/cooldown/                     │
│  ops/lifecycle/    ops/queue/                                          │
│  Each helper: takes BeadLifecycle (or smaller), composes a typed      │
│  Operation, calls Apply. Plus pure predicates over *Bead.             │
└───────────────────────────────────────────────────────────────────────┘
                              │ Apply(ctx, id, op Operation) error
                              ▼
┌─ storage primitives (cli/internal/bead/) ─────────────────────────────┐
│  Backend = BeadInitializer + BeadReader + BeadLifecycle               │
│          + BeadEventReader + BeadEventWriter                          │
│          + BeadQueries                                                │
│          + BeadDependencyReader + BeadDependencyWriter                │
│          + BeadArchive                                                │
│          + BeadInterchangeReader + BeadInterchangeWriter              │
│  Parallel (not on Backend):                                           │
│    LifecycleSubscriber  (implemented by *WatcherHub)                  │
│    IDGenerator          (RandomHexIDGenerator, SequentialIDGenerator) │
└───────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─ backend implementations (cli/internal/bead/internal/storage/) ───────┐
│  *Store (composes over RawBackend: JSONLBackend, ExternalBackend)     │
│  *AxonStore (implements Backend directly — per-row Postgres ops)      │
│  Read-only backends (implement only the Reader sub-interfaces)        │
└───────────────────────────────────────────────────────────────────────┘
```

## 6. Storage Sub-Interface Taxonomy

Eleven ctx-aware sub-interfaces compose `Backend`. The split is LSP-driven: read/write are separated wherever a planned backend class falls on one side but not the other (specifically, the read-only deployment shape).

```go
package bead

type BeadInitializer interface {
    Init(ctx context.Context) error
}

type BeadReader interface {
    ReadAll(ctx context.Context) ([]Bead, error)
    ReadAllFiltered(ctx context.Context, pred func(Bead) bool) ([]Bead, error)
    Get(ctx context.Context, id string) (*Bead, error)
    GetWithArchive(ctx context.Context, id string) (*Bead, error)
}

type BeadLifecycle interface {
    Create(ctx context.Context, b *Bead) error              // ErrInvalidID, ErrConflict
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

type BeadDependencyReader interface {
    DepTree(ctx context.Context, rootID string) (string, error)
}

type BeadDependencyWriter interface {
    DepAdd(ctx context.Context, id, depID string) error
    DepRemove(ctx context.Context, id, depID string) error
}

type BeadArchive interface {
    Archive(ctx context.Context, policy ArchivePolicy) ([]string, error)
    ArchiveWithEvents(ctx context.Context, policy ArchivePolicy) ([]string, error)
    Migrate(ctx context.Context) (MigrateStats, error)
}

type BeadInterchangeReader interface {
    ExportTo(ctx context.Context, w io.Writer) error
    ExportToFile(ctx context.Context, path string) error
}

type BeadInterchangeWriter interface {
    Import(ctx context.Context, source, filePath string) (int, error)
}
```

### 6.1 Composite Backend

```go
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

type ReadOnlyBackend interface {
    BeadInitializer
    BeadReader
    BeadEventReader
    BeadQueries
    BeadDependencyReader
    BeadInterchangeReader
}
```

### 6.2 Parallel Interfaces (NOT on Backend)

```go
type LifecycleSubscriber interface {
    SubscribeLifecycle(ctx context.Context, projectID string) (<-chan LifecycleEvent, func(), error)
}

type IDGenerator interface {
    GenID(ctx context.Context) (string, error)
}
```

## 7. Operation Pattern

```go
// Operation is a typed mutation applied to a bead. Backends MAY recognize
// specific operation types and execute them efficiently; the default path
// is Get(id) → op.Apply(bead) → Save(bead). op.Apply returns an error;
// returning non-nil aborts the storage write (CAS semantics).
type Operation interface {
    Apply(b *Bead) error
}
```

### 7.1 Named Operation Types

```go
// CRUD-ish ops
type SetStatus            struct{ Status string }
type AppendNotes          struct{ Notes string }

// Claim CAS
type ClaimOp              struct{ Owner, Session, Worktree string }
type UnclaimOp            struct{ RequireOwner string }

// Claim liveness
type SetClaimHeartbeat    struct{ At time.Time }
type ClearClaimHeartbeat  struct{}

// Cancellation
type SetCancelRequested   struct{ At time.Time }
type ClearCancelRequested struct{}
type MarkCancelHonoredOp  struct{ At time.Time }

// Cooldown
type SetCooldown          struct{ Until time.Time; Status, Detail, BaseRev string }
type ClearCooldownOp      struct{}
type IncrNoChangesCount   struct{}

// Queue ordering
type QueueSetTop          struct{}
type QueueSetPosition     struct{ Position int }
type QueueClearOp         struct{}

// Workflow-aware lifecycle transitions (validation in helper, not op)
type SetLifecycleStatus   struct{ Status string; Options LifecycleTransitionOptions }
type SetCloseEvidence     struct{ SessionID, CommitSHA string }
type ReopenOp             struct{ Reason, Notes string }
type ParkToProposedOp     struct{ Reason string }

// Ad-hoc escape hatch
type MutateFunc func(*Bead) error
func (m MutateFunc) Apply(b *Bead) error { return m(b) }
```

Selected Apply implementations:

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

### 7.2 How Apply flows through `*Store` over `RawBackend`

`*Store.Apply` is the universal entry point on the composition path. It type-asserts the wrapped `RawBackend` for an optional `OperationApplier` interface, and delegates if present; otherwise it falls back to the generic load-mutate-save path.

```go
// OperationApplier is an OPTIONAL contract a RawBackend MAY satisfy to
// provide per-op optimization. JSONLBackend implements it so heartbeat
// writes go to the sidecar file instead of churning the corpus.
type OperationApplier interface {
    Apply(ctx context.Context, id string, op Operation) error
}

func (s *Store) Apply(ctx context.Context, id string, op Operation) error {
    if fast, ok := s.raw.(OperationApplier); ok {
        return fast.Apply(ctx, id, op)
    }
    return s.raw.WithLock(func() error {
        beads, err := s.raw.ReadAll()
        if err != nil { return err }
        b := findByID(beads, id)
        if b == nil { return ErrNotFound }
        if err := op.Apply(b); err != nil { return err }
        return s.raw.WriteAll(beads)
    })
}
```

JSONLBackend implements `OperationApplier`, type-switching for hot ops:

```go
func (j *JSONLBackend) Apply(ctx context.Context, id string, op Operation) error {
    switch op := op.(type) {
    case SetClaimHeartbeat:
        return j.writeClaimSidecar(id, op.At)
    case ClearClaimHeartbeat:
        return j.removeClaimSidecar(id)
    default:
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

Axon implements `Backend` directly (no `*Store` wrapper):

```go
func (a *AxonStore) Apply(ctx context.Context, id string, op Operation) error {
    switch op := op.(type) {
    case ClaimOp:
        res, err := a.db.Exec(ctx,
            `UPDATE beads SET claim_owner=$1, claim_session=$2, claim_worktree=$3, claim_at=NOW()
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
        return a.genericApply(ctx, id, op)
    }
}
```

A read-only HTTP backend rejects all Apply calls — returns `ErrUnsupported`.

## 8. ID Generation

```go
package bead

const (
    DefaultIDPrefix = "ddx-"
    MaxIDLength     = 64
    MinIDLength     = 8
)
var idCharsetRe = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

func ValidateID(id string) error {
    if len(id) < MinIDLength || len(id) > MaxIDLength {
        return fmt.Errorf("%w: length %d not in [%d, %d]", ErrInvalidID, len(id), MinIDLength, MaxIDLength)
    }
    if !idCharsetRe.MatchString(id) {
        return fmt.Errorf("%w: charset", ErrInvalidID)
    }
    return nil
}

type IDGenerator interface {
    GenID(ctx context.Context) (string, error)
}

type RandomHexIDGenerator struct {
    Prefix string
    Bytes  int  // 4 → "ddx-12abcdef"
}

func (g RandomHexIDGenerator) GenID(ctx context.Context) (string, error) {
    buf := make([]byte, g.Bytes)
    if _, err := rand.Read(buf); err != nil { return "", err }
    id := g.Prefix + hex.EncodeToString(buf)
    if err := ValidateID(id); err != nil { return "", err }
    return id, nil
}

type SequentialIDGenerator struct {
    Prefix  string
    counter atomic.Int64
}
func (g *SequentialIDGenerator) GenID(ctx context.Context) (string, error) {
    n := g.counter.Add(1)
    return fmt.Sprintf("%s%08x", g.Prefix, n), nil
}

func NewIDGenerator() IDGenerator {
    return RandomHexIDGenerator{Prefix: DefaultIDPrefix, Bytes: 4}
}
```

## 9. Context Discipline

Every `Backend` method takes `ctx context.Context` as first parameter. The set of allowed ctx values is **closed**:

```go
// Allowed on ctx:
//   - context.WithCancel / WithDeadline / WithTimeout from any caller
//   - bead.WithIdentity(ctx, Identity) — caller's authenticated identity
//   - bead.WithTrace(ctx, trace.Span) — OpenTelemetry span
//
// NOT allowed on ctx (anti-patterns):
//   - bead.Backend instance (use a function argument)
//   - per-call options ("include archived", "force refresh") — belong on method signatures
//   - mutable state, logger references, configuration
//
// Typed accessors (these are the only public ctx APIs in the bead package):
func WithIdentity(ctx context.Context, id Identity) context.Context
func IdentityFromContext(ctx context.Context) (Identity, bool)
func WithTrace(ctx context.Context, span trace.Span) context.Context
func TraceFromContext(ctx context.Context) (trace.Span, bool)
```

A backend implementation that reads a context value outside this set is using the wrong door. Lint rule: `ctx.Value()` calls in `cli/internal/bead/` must go through a typed accessor.

## 10. Sentinel Errors

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

---

# Part III — Bead Data Model

## 11. Bead Data Model

The bead is the unit of identity in the storage system. This section enumerates its fields, normative invariants, JSONL wire format, schema-versioning policy, and the Operation × field-mutation rules.

### 11.1 Field Enumeration

**Required at Create:**

| Field | Type | Semantics |
|---|---|---|
| `id` | string | Conforms to `bead.ValidateID` (§8). Immutable post-create. |
| `title` | string | Imperative summary. Max `MaxIDLength * 4` bytes (256). Non-empty. |
| `type` | string | One of `{task, bug, epic, chore}`. |
| `status` | string | One of the six values in §1. Default `open`. |
| `created_at` | RFC3339Nano UTC | Set by the storage at Create. Immutable post-create. |
| `schema_version` | int | Currently `1`. Set by storage. Immutable post-create except via migration (§11.4). |

**Optional fields (top-level):**

| Field | Type | Semantics |
|---|---|---|
| `description` | string | Canonical body content for executors (drain loop reads this). Max `64 KiB`. |
| `acceptance` | string | Numbered acceptance criteria. Max `16 KiB`. |
| `notes` | string | Append-only free-form notes; new content appended via `AppendNotes` op. |
| `labels` | []string | Each entry matches `/^[a-z0-9:_-]+$/`; distinct; ordered. |
| `parent` | string | Parent bead id (conforms to `ValidateID`). |
| `deps` | []string | Dependency bead ids; each conforms to `ValidateID`; distinct. |
| `priority` | int | `[0, 4]` where 0 is highest. Default 2. |
| `assignee` | string | Convenience copy of `claim.owner`; cleared on unclaim. |
| `updated_at` | RFC3339Nano UTC | Set by storage on every mutation. |
| `closed_at` | RFC3339Nano UTC | Set when `status` becomes `closed`. |

**Sub-struct `claim`** (claim metadata; see §12 for semantics):

| Field | Type |
|---|---|
| `claim.owner` | string |
| `claim.session` | string |
| `claim.worktree` | string |
| `claim.at` | RFC3339Nano UTC |
| `claim.last_heartbeat` | RFC3339Nano UTC (may live in sidecar file for JSONL backends) |

**Sub-struct `cancel`** (cooperative cancellation signal):

| Field | Type |
|---|---|
| `cancel.requested` | bool |
| `cancel.requested_at` | RFC3339Nano UTC |
| `cancel.honored` | bool |
| `cancel.honored_at` | RFC3339Nano UTC |

**Sub-struct `cooldown`** (retry/backoff metadata; queue derivation respects this):

| Field | Type |
|---|---|
| `cooldown.until` | RFC3339Nano UTC |
| `cooldown.status` | string (last-status code) |
| `cooldown.detail` | string |
| `cooldown.base_rev` | string (git SHA at cooldown time) |

**Other top-level fields:**

| Field | Type | Semantics |
|---|---|---|
| `no_changes_count` | int | Number of consecutive `no_changes_*` outcomes. ≥ 0. |
| `events` | []BeadEvent | Inline event log (append-only). May be externalized to attachments (§17) when large. |
| `extra` | map[string]any | Preserved extras per ADR-004 (workflow-specific keys; bd/br interchange compatible). |

**Sub-struct `BeadEvent`** (one entry in the event log):

| Field | Type |
|---|---|
| `event_id` | string (unique within bead) |
| `kind` | string (from §13 event vocabulary) |
| `actor` | string |
| `created_at` | RFC3339Nano UTC |
| `summary` | string |
| `body` | string (free-form; may contain structured JSON) |
| `source` | string (e.g. "ddx bead evidence add", "drain loop") |

### 11.2 Universal Invariants

These hold across every backend implementation. Conformance tests assert them.

1. **Identity immutability**: `id`, `created_at`, and `schema_version` are immutable post-create. No Operation may mutate them.
2. **Claim consistency**: `claim.owner` empty ↔ `claim.session` empty ↔ `claim.worktree` empty (all-or-nothing).
3. **Claim timestamp**: `claim.at` present ↔ `claim.owner` present.
4. **Cancel consistency**: `cancel.requested_at` present ↔ `cancel.requested == true`. `cancel.honored_at` present ↔ `cancel.honored == true`.
5. **Cancel ordering**: if `cancel.honored == true` then `cancel.requested == true` and `cancel.honored_at ≥ cancel.requested_at`.
6. **Cooldown semantics**: `cooldown.until` in the past → cooldown is no-op for queue derivation (consumer ignores; doesn't auto-clear).
7. **Event append-only**: existing entries in `events` are never modified or removed. New entries always appended at the end. `event_id` is unique within a bead.
8. **Event timestamps**: `events[i].created_at ≤ events[i+1].created_at` (monotonic per bead). If a backend cannot guarantee monotonicity natively, the storage layer enforces by assigning timestamps at append time.
9. **Status terminality**: `closed` and `cancelled` are terminal — no Operation transitions them to a non-terminal status. Reopening files a new bead with `replaces` set.
10. **`in_progress` ↔ claim**: persisted convention is that `status=in_progress` implies a non-empty claim, and `status=open` implies no claim. Stale-claim sweep enforces this when it observes drift.
11. **Dependency identifiers**: every id in `deps` conforms to `ValidateID`. Storage does not enforce that the referenced bead exists (deferred references are allowed).
12. **Label uniqueness**: `labels` contains no duplicates.

### 11.3 JSONL Wire Format

For JSONL-backed collections, the on-disk representation is:

- **One bead per line.** UTF-8 encoded. No BOM. Line terminator `\n` (LF). Final line MUST end with `\n`.
- **Each line is a valid JSON object.** Parsers MUST accept any JSON-compliant whitespace within the object, but writers SHOULD emit compact (no inter-key whitespace) form.
- **Field ordering**: alphabetical by field name, **except `id` is always first**. Sub-objects (claim, cancel, cooldown) follow their parent's alphabetical position; their internal fields are alphabetical.
- **Null/empty omission**: fields with zero values (empty string, zero int, false bool, nil time, empty array, empty map, all-zero sub-struct) are omitted from the JSON object (Go's `omitempty` convention).
- **Timestamps**: encoded as RFC3339Nano UTC strings (e.g. `"2026-05-11T17:30:00.123456789Z"`). Always UTC; never includes a timezone offset.
- **Booleans, integers, strings**: natural JSON encoding.
- **Sub-structs**: emitted as nested JSON objects; omitted entirely when all fields are zero-valued.
- **Arrays**: emitted as JSON arrays; omitted entirely when empty.
- **`extra` map**: emitted as a JSON object preserving unknown keys verbatim per ADR-004. Field order within `extra` is alphabetical.
- **Unknown field preservation**: readers MUST preserve unknown top-level fields and unknown `extra` keys verbatim across a read-write cycle (bd/br interchange contract).

Backends other than JSONL (e.g. Axon-Postgres) MAY use different wire formats internally. The JSONL form is the canonical interchange shape; `BeadInterchangeReader.ExportTo` produces it; `BeadInterchangeWriter.Import` consumes it. Every backend MUST round-trip a bead through JSONL export+import without semantic loss (verified by `schema_compat_test.go`).

### 11.4 Schema-Versioning Policy

The `schema_version` field is a positive integer; current value is `1`. The version increments only on **breaking** changes.

**Additive changes** (do NOT bump `schema_version`):

- Adding a new optional field (top-level or in `extra`).
- Adding a new sub-struct field within `claim`, `cancel`, `cooldown`.
- Adding a new event `kind` to the controlled vocabulary (§13).
- Adding a new label convention.

Readers MUST preserve unknown fields and keep operating. Writers MAY emit the new field freely.

**Breaking changes** (DO bump `schema_version`):

- Renaming a field.
- Removing a field.
- Changing a field's type.
- Changing the semantic meaning of an existing field value.
- Changing the JSONL wire-format conventions (ordering, encoding, omission rules).

Adding a new persisted **status value** is NOT a schema-version bump alone — it requires an ADR-004 amendment (the bd/br canonical set is fixed; see Critical Constraint at top).

**Migration ladder** (when `schema_version` changes):

1. **Absence is v1.** Existing beads in `.ddx/beads.jsonl` that predate the `schema_version` field SHOULD be treated as `schema_version=1` on read. Backends MUST set `schema_version=1` on the next write to such records.
2. Migration is **lazy on read** until Axon's FEAT-017 (schema evolution) ships in upstream Axon. When a backend reads a record at an older `schema_version`, it transforms in-memory, returns to the caller, and writes back at the new version on the next mutation.
3. A `ddx bead schema upgrade` command exists for eager migration during breaking changes.
4. Migration code lives in `cli/internal/bead/migrate.go` (or backend-specific equivalents). Each migration is a function `func migrateVN_VN_plus_1(b *Bead) error`.
5. Once Axon FEAT-017 lands, DDx switches to Axon-native schema migration via `putSchema`.

### 11.5 Operation × Field-Mutation Rules

The following table is normative: it specifies which Operations are permitted to mutate which fields. Operations not listed for a field MUST NOT mutate it. Custom code paths MUST go through `MutateFunc` (the escape hatch) rather than introducing field mutations directly.

| Field | Operations that may mutate it |
|---|---|
| `id` | NONE (immutable; assigned at Create) |
| `created_at` | NONE (immutable; assigned at Create) |
| `schema_version` | NONE except internal migration |
| `title` | `MutateFunc` only |
| `type` | `MutateFunc` only |
| `status` | `SetStatus`, `SetLifecycleStatus`, `ClaimOp` (→ in_progress), `UnclaimOp` (→ open if status was in_progress), `ReopenOp` (→ open), `ParkToProposedOp` (→ proposed) |
| `description` | `MutateFunc` only |
| `acceptance` | `MutateFunc` only |
| `notes` | `AppendNotes`, `MutateFunc` |
| `labels` | `MutateFunc` only |
| `parent` | `MutateFunc` only |
| `deps` | (set/cleared via `BeadDependencyWriter.DepAdd`/`DepRemove`, which take their own atomic path; not Operation-driven) |
| `priority` | `MutateFunc` only |
| `assignee` | `ClaimOp` (set to `owner`), `UnclaimOp` (clear) |
| `claim.owner`, `claim.session`, `claim.worktree`, `claim.at` | `ClaimOp` (set), `UnclaimOp` (clear) |
| `claim.last_heartbeat` | `SetClaimHeartbeat`, `ClearClaimHeartbeat` |
| `cancel.requested`, `cancel.requested_at` | `SetCancelRequested`, `ClearCancelRequested` |
| `cancel.honored`, `cancel.honored_at` | `MarkCancelHonoredOp` |
| `cooldown.*` | `SetCooldown`, `ClearCooldownOp` |
| `no_changes_count` | `IncrNoChangesCount` (drain-loop-driven; field policy described in TD-031); operator-cleared via `MutateFunc` |
| `events` | `BeadEventWriter.AppendEvent` (append-only) |
| `extra.*` | `MutateFunc` (specific keys per their owning subsystem — e.g. `extra.execute-loop-retry-after` is mutated by `SetCooldown`, `extra.children` by decomposition flows) |
| `closed_at` | Storage layer sets automatically when `status` transitions to `closed` |
| `updated_at` | Storage layer sets automatically on every mutation |

Operations are stable public API. Adding a new named Operation is additive (no version bump). Removing or changing the shape of an existing Operation is a breaking change requiring schema-version coordination.

### 11.6 Bead Go Struct (reference)

The canonical Go representation lives in `cli/internal/bead/bead.go`. The struct definition is the implementation source of truth; this TD describes the contract the struct must honor. Conformance tests assert that the struct's JSON-tagged fields, types, and `omitempty` markers match this specification.

---

# Part IV — Bead Semantics

## 12. Claim Semantics

Claim is metadata, not status. The persisted-status convention is that `in_progress` implies a claim is held; `open` implies no claim. Claim fields live in `claim.*` (§11.1).

### 12.1 Claim Resolution (atomic)

Claiming a bead performs one atomic snapshot rewrite under the store lock:

1. Load the bead snapshot.
2. Resolve the assignee in this order:
   1. explicit CLI `--assignee`
   2. runtime caller identity
   3. `ddx`
3. Set `status=in_progress`.
4. Record `claim.owner`, `claim.at`, optionally `claim.session` and `claim.worktree`.
5. Rewrite the snapshot atomically.

Unclaiming performs the inverse mutation:

1. Load the bead snapshot.
2. Set `status=open`.
3. Clear all `claim.*` fields.
4. Rewrite the snapshot atomically.

Claim metadata is advisory in the storage layer. `ClaimOp.Apply` returns `ErrAlreadyClaimed` if it observes a different owner; this is the CAS check.

### 12.2 Worker Shutdown and Interruption

- A worker that receives graceful shutdown while no terminal mutation has been applied MUST release any active claim it owns before exiting. Status returns to `open`; claim cleared; an `unclaimed` event records `reason=worker_shutdown`.
- If a terminal mutation (`closed`, `cancelled`, or `blocked`) was already applied, shutdown MUST NOT undo it. Best-effort worker-disconnect telemetry may be emitted, but bead state is authoritative.
- Mechanical interruption (ctx cancel, SIGTERM, SIGINT) before terminal mutation: worker preserves available evidence, appends a structured interruption event, releases the claim, leaves the bead re-claimable unless an explicit blocker or cooldown was recorded.
- Ungraceful worker death may strand an `in_progress` bead. The stale-claim sweep (drain-loop policy; see TD-031) is the recovery path.
- Cooldown is NOT a shutdown-cleanup mechanism. A stopped or interrupted worker may set `execute-loop-retry-after` (cooldown) only when the recorded outcome is a retryable time-based condition.

### 12.3 Stale Claim Recovery (storage contract; drain-loop policy in TD-031)

A claim is *stale* when `claim.at` is older than the configured stale threshold AND the bead is still `in_progress`. Storage exposes the field; the drain loop owns the policy for declaring staleness and dispatching recovery (see TD-031 §6.2 TriageContract for the canonical policy).

When the drain loop releases a stale claim, it MUST:

1. Append an `auto-triage` (or `unclaimed` with `reason=stale_claim`) event recording prior `claim.owner` and `claim.at`.
2. Move the bead from `in_progress → open`.
3. Clear the claim fields.

Storage does not delete or modify prior claim metadata as part of stale recovery — the event log retains the history.

## 13. Event Vocabulary

Events are append-only entries in `events` (§11.1). The `kind` field uses a closed controlled vocabulary; consumers (drain loop, server, CLI, MCP) read and write only these kinds.

### Lifecycle events (storage-level state transitions)

- `triaged` — `proposed → open` (operator acceptance signal)
- `claimed` — claim acquired
- `unclaimed` — claim released
- `blocked` — moved to `blocked`
- `unblocked` — moved off `blocked`
- `closed-merged` — closed after a merge
- `closed-already-satisfied` — closed because work was already done
- `closed` — operator-driven close (catch-all)
- `cancelled` — moved to `cancelled`
- `lifecycle_reconciled` — operator/system reconciled a stale lifecycle state

### Drain-outcome events (no_changes family)

- `no_changes_verified` — attempt produced no commit; verification command passed.
- `no_changes_unverified` — attempt produced no commit; verification command failed or could not run.
- `no_changes_unjustified` — attempt produced no commit without structured rationale.
- `no_changes_needs_investigation` — legacy/backcompat for operator triage.
- `no_changes_decomposed` — agent decomposed the bead instead of changing files.
- `no_changes_blocked` — agent declared no_changes with a justified external blocker.
- `no_changes_recoverable` — transient cause; retry plausibly succeeds.

### Drain-control events

- `drain-paused-quota` — drain paused on quota exhaustion.
- `drain-resumed-quota`
- `rate-limit-retry` — single attempt retried after rate-limit response.
- `lock-contention` — store-lock contention observed and handled.

### Review and triage events

- `review-block` — reviewer raised a BLOCKING finding.
- `review-pass` — reviewer cleared the change.
- `review-request-changes` — reviewer returned structured `REQUEST_CHANGES`.
- `review-fixable-gap` — reviewer found a repairable gap.
- `review-too-large` — reviewer/readiness found bead/result too broad.
- `review-error` — reviewer invocation failed or returned no parseable verdict.
- `review-manual-required` — exhausted automatic recovery; operator action required.
- `triage-decomposed` — readiness/review decomposed parent into children.
- `triage-overflow` — decomposition reached the depth cap.
- `triage-rewritten` / `intake-rewritten` — readiness applied a validated rewrite.
- `triage-ambiguous` — readiness could not safely clarify.
- `auto-triage` — triage path mutated labels/status.

### Candidate-cycle events (FEAT-010)

- `candidate-pinned`
- `candidate-checks-failed`
- `repair-cycle-started`
- `repair-cycle-exhausted`
- `approved-land-conflict`
- `final-result-landed`

### Auto-recovery events (ADR-024)

- `reframe-applied`
- `decompose-applied`
- `auto-recovery-failed`
- `per-bead-budget-exhausted`

### Push-outcome events

- `push-failed` — commit created, push rejected.
- `push-conflict` — push rejected because remote advanced.

Each event SHOULD include `kind`, `actor`, `created_at`, and a `body` (free-form, may carry structured JSON). Drain-related events SHOULD include the `attempt-id` of the relevant execution attempt in the body or in a structured extra field.

The **operational mapping** from drain-loop outcomes (e.g. `review_pass`, `push_failed`) to which events are appended and which transitions fire lives in TD-031 §2 (Outcome → Label/Event/Extra Mapping). New event kinds require amending §13 here in the same PR that introduces them, plus the mapping table in TD-031 if drain-loop-driven.

## 14. Workflow Helper Packages

Workflow operations live in `cli/internal/bead/ops/<concern>/`. Each helper takes a `BeadLifecycle` (or smaller sub-interface) and composes a typed `Operation`, calling `Apply`.

```
cli/internal/bead/ops/
  claim/              # liveness + heartbeat policy
    liveness.go       # Touch, Clear, IsFresh(*Bead, maxAge), Acquire, Release
  cancel/
    cancel.go         # Request, MarkHonored, IsRequested(*Bead)
  cooldown/
    cooldown.go       # Set, ClearAll(filter), IncrNoChanges
  lifecycle/          # state-machine ops with HELIX-aware validation
    transition.go     # Transition, Reopen, ParkToProposed, CloseWithEvidence, AppendNotes
  queue/              # operator-driven queue ordering
    queue.go          # Top, Move, Clear
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
```

```go
package lifecycle

var validTransitions = map[string][]string{ /* per §2 */ }

func Transition(ctx context.Context, l bead.BeadLifecycle, r bead.BeadReader, id, newStatus string) error {
    b, err := r.Get(ctx, id)
    if err != nil { return err }
    if !contains(validTransitions[b.Status], newStatus) {
        return fmt.Errorf("invalid transition %s → %s for %s", b.Status, newStatus, id)
    }
    return l.Apply(ctx, id, bead.SetLifecycleStatus{Status: newStatus})
}
```

The `lifecycle/` helper encodes HELIX-flavored state-machine validation today. A future architectural improvement is to move that policy to the HELIX plugin so generic bead ops stay workflow-agnostic; deferred.

---

# Part V — Collections, Storage, and Migration

## 15. Collection Registry

A collection is a named logical store backed by one of the bead backends. The registry is an in-process Go map seeded at startup from a small declarative table.

```go
type CollectionID string

type CollectionSpec struct {
    ID              CollectionID
    DefaultBackend  BackendKind  // jsonl | bd | br | axon
    JSONLPath       string
    QueueSemantics  bool
    ArchivePartner  CollectionID
    Attachments     bool
}
```

Concrete shipping registry:

| ID               | JSONL path                       | QueueSemantics | ArchivePartner | Attachments |
|------------------|----------------------------------|----------------|----------------|-------------|
| `beads`          | `.ddx/beads.jsonl`               | yes            | `beads-archive`| no          |
| `beads-archive`  | `.ddx/beads-archive.jsonl`       | no             | (none)         | no          |
| `exec-runs`      | `.ddx/exec-runs.jsonl`           | no             | (none)         | yes         |
| `agent-sessions` | `.ddx/agent-sessions.jsonl`      | no             | (none)         | yes         |

### Backend Selection

`bd` and `br` accept a collection name via their own CLIs; the external-backend adapter passes the collection ID through. Backends that do not support multiple named collections return a clear error when a non-default collection is requested rather than silently merging records.

## 16. Archival Trigger Policy

### Goal

Keep the active queue small enough that `ddx bead list` and the read path stay under the 100 ms / 10,000 bead target stated in SD-004, without forcing operators to think about archival.

### Trigger

Archival runs on demand through `ddx bead archive` and opportunistically after any close-causing mutation when the active collection exceeds a threshold. No background daemon.

### Defaults

| Parameter                      | Default                                |
|--------------------------------|----------------------------------------|
| `archive.enabled`              | `true`                                 |
| `archive.min_age`              | `30d` since `closed_at`                |
| `archive.min_active_count`     | `2000` records in `beads`              |
| `archive.batch_size`           | `500` per opportunistic pass           |
| `archive.statuses`             | `closed`, `cancelled`                  |
| `archive.opportunistic`        | `true`                                 |
| `archive.preserve_dependencies`| `true`                                 |

A bead is eligible to archive when its `status` is in `archive.statuses`, its `closed_at` (or `updated_at` fallback) is older than `archive.min_age`, and no open bead in `beads` lists it as a dependency. Terminal reason names such as `wont_fix` or `superseded` are labels or `extra` metadata, not statuses; they can explain why a bead is terminal but do not belong in `archive.statuses`.

### Configuration

```yaml
archive:
  enabled: true
  min_age: 30d
  min_active_count: 2000
  batch_size: 500
  statuses: [closed, cancelled]
  opportunistic: true
```

### Mutation Sequence

1. Acquire `beads` lock.
2. Acquire `beads-archive` lock.
3. Read both snapshots.
4. Select up to `batch_size` eligible records from `beads`.
5. Append the selected records to `beads-archive` with an added `archived_at` timestamp in `Extra`.
6. Remove the selected records from `beads`.
7. Atomic temp-file rename for `beads-archive` first, then for `beads`.
8. Release locks in reverse order.

## 17. Attachment Storage Layout

### Layout

```
.ddx/attachments/
  <collection-id>/
    <record-id>/
      events.jsonl
      prompt.txt
      response.txt
      stdout.log
      <name>.<ext>
```

Defaults:

- `beads` does not use sidecar attachments; evidence stays in `events`.
- `exec-runs` and `agent-sessions` use sidecars for prompt/response/stdout/stderr/results.
- `beads-archive` does not introduce new attachments; archived records carry their existing `events` history inline.

### Reference Format

```json
{
  "attachments": [
    {
      "name": "prompt",
      "path": "exec-runs/run-2026-05-01-abc/prompt.txt",
      "media_type": "text/plain",
      "sha256": "…",
      "size": 12345
    }
  ]
}
```

`path` is repository-relative under `.ddx/attachments/`. `sha256` and `size` are recorded at write time and verified on read. `attachments` is a DDx-specific extras key per ADR-004.

### Write Algorithm

1. Write attachment to temp path under `.ddx/attachments/<collection>/<id>/.<name>.tmp`.
2. `fsync`, then rename to final path.
3. Compute and record `sha256` and `size`.
4. Append reference to `extra.attachments`.
5. Persist record through normal collection write path.

### Garbage Collection

`ddx bead gc` walks `.ddx/attachments/<collection>/` and deletes directories whose record IDs no longer exist in the collection. No time-based expiration.

### Event Externalization

For active beads with very large event histories, `events` may be externalized to a sidecar `events.jsonl` under the bead's attachment directory. The bead record then carries `extra.events_attachment` pointing to the sidecar; the inline `events` array is empty. Reading reconstructs the events from the sidecar; appending goes to the sidecar directly. Externalization is a backend-internal optimization; consumers see the same logical event log either way.

## 18. Migration Plan for Existing `beads.jsonl`

### Steps

1. **Compatibility shim first.** Collection registry treats `.ddx/beads.jsonl` as the JSONL path for the `beads` collection; no file move.
2. **Idempotent create-on-write for archive.** `.ddx/beads-archive.jsonl` is created lazily on first archive operation.
3. **One-shot backfill command.** `ddx bead migrate-archive` performs the initial archival pass: backup → move eligible records → print summary.
4. **No schema rewrite.** Existing records are not rewritten on migration.
5. **Rollback.** `ddx bead migrate-archive --rollback` re-merges `beads-archive.jsonl` into `beads.jsonl`.

### Risk Mitigation

- Default `archive.min_active_count` of 2000 means small installations keep single-file behavior until explicitly migrated.
- Pre-archive backup file is never removed automatically.

### One-Way Lifecycle Migration and Startup Gate

The transition from label-owned lifecycle to status-owned lifecycle is one-way. Legacy/backcompat labels and pseudo-statuses (`needs_human`, `triage:needs-investigation`, `needs_investigation`) are migration input only. Normal runtime MUST NOT maintain compatibility lanes.

DDx startup MUST refuse normal operation when the active project bead queue contains unmigrated lifecycle state. The preflight scans the active queue before ordinary commands load queue views. It fails closed when it finds:

- open or in-progress beads carrying legacy lifecycle labels;
- non-canonical pseudo-status values outside the six persisted statuses;
- legacy `extra` fields that still control routing instead of preserving historical evidence.

Allowed bypass: `ddx help`, `ddx version`, `ddx doctor` and other read-only diagnostics, `ddx bead migrate --lifecycle --dry-run`, `ddx bead migrate --lifecycle --apply`.

The startup error output MUST include:

1. counts of legacy lifecycle labels by name;
2. counts of non-canonical pseudo-status values by name;
3. the first few affected bead IDs;
4. the exact commands:
   ```bash
   ddx bead migrate --lifecycle --dry-run
   ddx bead migrate --lifecycle --apply
   ```
5. the rollback instruction: use git rollback if the migration result is wrong.

## 19. Read-Path Semantics Across Active and Archive

### Default View

`list`, `ready`, `blocked`, `status` read only the `beads` collection. They do not load `beads-archive`.

### Merged View

`ddx bead show <id>`, `ddx bead history`, and `--include-archive`:

1. Look up in `beads` first.
2. Fall back to `beads-archive` if not found.
3. Listing with `--include-archive` lazily concatenates with active-wins precedence.

### Lazy Loading

The archive collection opens only when a merged view is requested. Per-collection snapshots cache for one command invocation; not shared across invocations.

### Deletion Semantics

`ddx bead delete <id>` removes from the collection holding the record. Records in both collections (post-crash) are removed from both under both locks.

### Dependency Resolution Across Archive

Queue derivation reads only the dependency's `status` from `beads-archive` on demand and caches it for the command. Archived beads are never promoted back into the active snapshot.

## 20. bd/br Interchange Compatibility

The archive collection participates in interchange on the same terms as the active collection.

- `ddx bead export --collection beads-archive` emits one bead-record JSON object per line, identical schema to active beads.
- `ddx bead import --collection beads-archive --from jsonl -` accepts the same shape.
- `archived_at` lives in `extra` and round-trips as an unknown field for bd/br.
- `attachments` extra round-trips; bd/br don't strip preserved extras.

Backends supporting multiple named collections pass the collection ID through; backends without multi-collection support return an error rather than merging.

`schema_compat_test.go` includes an "archive round-trip" case: archive a record, export it, re-import into a fresh `beads-archive`, verify field-for-field equality including preserved extras.

---

# Part VI — Module Boundary

## 21. Physical-Impossibility Enforcement via Go `internal/`

End-state directory layout uses Go's `internal/` rule to make bypass of the `Backend` interface **physically impossible** — not lint-prevented, not review-prevented, but a build failure if any package outside `cli/internal/bead/` attempts to import the concrete storage implementations.

### End-State Directory Layout

```
cli/internal/bead/
  bead.go            ← Bead struct (public — the data type)
  backend.go         ← Backend interface + 11 sub-interfaces (public)
  operation.go       ← Operation interface + ~20 named ops (public)
  errors.go          ← sentinel errors (public)
  id.go              ← ValidateID + IDGenerator (public)
  context.go         ← typed ctx accessors (public)
  factory.go         ← NewStore(opts) → Backend  (public — sole construction path)
  ops/               ← helper packages (public)
    claim/ cancel/ cooldown/ lifecycle/ queue/
  internal/          ← physically inaccessible from outside cli/internal/bead/
    storage/
      store.go             ← *Store (concrete; constructed only via factory)
      raw.go               ← RawBackend interface
      operation_applier.go ← OperationApplier optional interface
      backend_jsonl.go
      backend_external.go
      backend_axon.go      ← future
      claim_liveness.go    ← sidecar logic
      attachments.go
      archive.go
      migrate.go
    lifecycle/
      watcher_hub.go       ← *WatcherHub (factory returns LifecycleSubscriber)
```

After this layout: `cli/cmd/`, `cli/internal/agent/`, `cli/internal/server/`, anything else — **cannot** import `cli/internal/bead/internal/...`. They can only see what `cli/internal/bead/` exports.

### Per-Concern Bead Structure

The refactor lands as **atomic per-concern beads**, not three monolithic stages. Each bead is a closed loop: add the new abstraction for one concern (helpers + named Operations + sub-interface methods, all ctx-aware), migrate the callers that touch that concern, remove the corresponding `*Store` methods. After each bead, the tree compiles and tests pass — there's no transitional half-state where the interfaces exist but callers ignore them.

```
Phase 1: Foundation (gate)
    F (ddx-c6317784)    Operation pattern + IDGenerator + sentinels + ctx
                        accessors + foundational sub-interfaces
                        (Initializer/Reader/Lifecycle{Create+Apply}; EventR/W
                        declarations only)
                        + *Store.Apply + JSONLBackend.OperationApplier
                        + ctx threading on non-event foundational *Store methods

Phase 2: Per-concern beads (parallel-safe; any order after F)
    L (ddx-bca628fa)    Lifecycle helpers + ops; remove 7 *Store methods
    C (ddx-e1c743d3)    Claim + Heartbeat helpers (sidecar-preserved) + ops;
                        remove 7 *Store methods
    E (ddx-f39b41b3)    Events — ctx on *Store; satisfy BeadEventReader/Writer
    X (ddx-aed7c7ab)    Cancel + Cooldown helpers + ops; remove 6 *Store methods
    Q (ddx-bc07270f)    BeadQueries — ctx on 12 *Store methods; opportunistic narrowing
    D (ddx-3af1c1a6)    Dependencies (ctx) + Queue helpers + ops; remove 3
                        *Store queue methods
    A (ddx-71d3a2de)    Archive + Interchange — ctx; satisfy BeadArchive +
                        BeadInterchangeR/W
    MIG (ddx-0ab16765)  Migration tooling factory — Migrator interface +
                        NewMigrator + remove 7 *Store migration methods

Phase 3: Supporting + Axon cluster
    WH (ddx-900a8d38)   WatcherHub takes BeadReader factory
    LINT (ddx-e91a45c0) 5-analyzer lint suite + CI gate
    SR (ddx-8bf23be0)   Schema reconciliation (axon/schema.graphql vs invoked ops)
    AX (ddx-9c5bca8f)   AxonStore implements Backend directly (per-row Postgres ops)
    CF (ddx-29f02cf4)   Config-driven factory + capability validation
    CONF (ddx-958b8fc3) Parameterized conformance against both *Store and AxonStore
    SS (ddx-8dd19492)   Subscription smoke test against LifecycleSubscriber

Phase 4: Final lockdown
    BL (ddx-74452926)   Caller narrowing + relocate concretes to
                        internal/storage/ + Backend composite + compile-time
                        enforcement of physical impossibility

Phase 5: Production deployment (downstream)
    IMP (ddx-53df5e2f)  JSONL→Postgres importer (replaces legacy MigrateToAxon)
    WIRE (ddx-c479688b) Real-wire Axon integration tests against actual Postgres
```

### Why the per-concern boundary?

Each per-concern bead is **atomic** by design:

1. **Add** the new abstraction for one concern (helpers + typed `Operation` values + interface methods, all ctx-aware).
2. **Migrate** every caller that touches that concern from `*Store.<methodname>(args)` to `ops/<concern>.Func(ctx, ...)`.
3. **Remove** the corresponding `*Store` concrete methods.
4. Tree compiles, tests pass, `*Store` has fewer methods than before.

After a per-concern bead lands, no caller anywhere in the codebase calls the old `*Store` methods for that concern — because those methods don't exist. **No transitional state where some callers use the old way and others use the new way.**

`context.Context` threading happens incrementally and locally: when bead C migrates the claim-related callers, those caller functions get ctx threading for claim ops in the same change. Each caller file ends up touched in 3-6 different per-concern beads, each time small and focused.

### What BL (the final bead) actually guarantees

BL only succeeds if every per-concern bead has completed its migration. The mechanism: relocating concrete `*Store`/`JSONLBackend`/etc. to `cli/internal/bead/internal/storage/` makes them unreachable from outside `cli/internal/bead/` (Go's `internal/` rule). The build fails if any caller still references the concrete type. That failure surfaces the missed migration immediately — there's no silent regression path.

After BL: physically impossible bypass enforced by the Go compiler.

### Side-Door Catalog

| # | Side-door | Closure | Bead |
|---|---|---|---|
| S1 | Direct `*Store` method calls for workflow concerns | Method removal + caller migration in per-concern bead | L, C, X, D, MIG |
| S2 | Direct file I/O against `beads.jsonl` outside bead package | Audit + documented allowlist (`bead_doctor`, `sync`, git ops are legitimate) | F (audit), LINT (enforce) |
| S3 | `RawBackend` retained — new backends pick wrong shape | Docstring warning at F; `internal/` placement at BL | F (warning), BL (lockdown) |
| S4 | `Operation` catalog drift in optimizing backends | `TestOperationCatalog_AxonStoreSwitchCoverage` enumerates via reflection | AX |
| S5 | `*WatcherHub` self-constructs `*Store` | WatcherHub takes `BeadReader` factory | WH |
| S6 | Tests bypass factory via direct `NewStore` | Acceptable for backend-internal tests; conformance suite for cross-backend behavior | doc-only |
| S7 | `DDX_AXON_EXPERIMENTAL` legacy env var | Remove in AxonStore bead | AX |
| S8 | Operation type discoverability | Package doc on `operation.go` + catalog test | F + AX |
| S9 | Untyped `ctx.Value(...)` god-object risk | Typed accessors required; documented in §9 | F |
| S10 | `*bead.Store` field types in caller structs persist after per-concern beads | Narrowing to `bead.Backend` (or sub-interface) + `internal/` relocation makes concrete type unreachable | BL |
| S11 | Migration tooling unreachable post-lockdown | `Migrator` factory exposed at public `cli/internal/bead/` boundary | MIG |

### Legacy Path Catalog

| # | Legacy path | Removal plan |
|---|---|---|
| L1 | `AxonBackend` (whole-corpus path) | Remove after `AxonStore` ships and passes conformance |
| L2 | `*Store` workflow methods (Heartbeat, Claim, etc.) | Removed by the relevant per-concern bead after callers migrate to `ops/<concern>/` helpers |
| L3 | `RawBackend` interface | Kept; placed inside `internal/storage/` after Phase 4 |
| L4 | `DDX_AXON_EXPERIMENTAL` env var | Removed in AxonStore bead |
| L5 | `MigrateToAxon` writing JSONL | Replaced by real Postgres importer post-AxonStore |
| L6 | Lifecycle schema marker methods | Kept JSONL-only; not promoted to interface |
| L7 | Concrete-only Migrate*/Reconcile* methods | Case-by-case |

### Lint Rules (transitional vs. permanent)

Five Go analyzers under `cli/tools/lints/` (filed as bead `ddx-e91a45c0` / LINT) enforce boundary invariants:

- `direct-bead-jsonl-io` — flags new file I/O against `beads.jsonl` outside allowlist. **Permanent.**
- `no-new-rawbackend-impls` — flags new types implementing `RawBackend` outside allowlist. **Permanent.**
- `typed-context-accessors-only` — flags untyped `ctx.Value` in `cli/internal/bead/`. **Permanent.**
- `concrete-store-methods` — flags new files holding `*bead.Store` as struct field. **Transitional** — removed at BL (Go's `internal/` rule enforces the same property at the compiler level).
- `no-internal-store-construction` — flags `bead.NewStore(...)` calls outside test packages and factory. **Transitional** — removed at BL.

Through Phase 2 (per-concern beads) and Phase 3 (Axon cluster), all five analyzers run in CI. After BL completes, the lint suite drops the two transitional analyzers.

---

# Part VII — Process and Maintenance

## 22. Future-Change Process

> Any bead that introduces or changes a label, an event kind, an outcome → state mapping, claim handling, or worker-state semantics MUST cite the relevant TD section that authorizes the change. If no section authorizes it, the TD is amended in the same PR (or a parent bead) before the dependent work lands.
>
> Adding a new persisted bead status is **not** authorized by TD-027 or TD-031. It requires upstream bd/br coordination AND an amendment to ADR-004.

In practice:

- A new event kind: amend TD-027 §13 in the same PR.
- A new outcome → state mapping: amend TD-031 §2 in the same PR.
- A new label: amend TD-027 §4.
- A new worker state: amend TD-031 §3.
- A new operation type: add to TD-027 §7.1 (additive; no schema-version bump).
- A new persisted status: file an ADR-004 amendment first.

CI guard: a sibling check verifies any change to `bead-record.schema.json` or to the persisted-status enum touches ADR-004 + TD-027 in the same commit.

## 23. Acceptance Criteria

Acceptance criteria are partitioned by phase. Each bead's own description lists its specific AC; this section gives the workstream-level invariants and points at the relevant beads. The workstream is "done" only when every phase's beads have completed.

Every bead in every phase must satisfy these baseline gates: `cd cli && go test ./...` is green; `lefthook run pre-commit` passes. These are not restated per bead.

### 23.1 Phase 1 — Foundation

**`F` (ddx-c6317784):** see bead description for the 15-item AC list.

Key cross-bead invariants after F lands:

- `cli/internal/bead/operation.go`, `id.go`, `errors.go`, `context.go` exist.
- Foundational sub-interfaces (`BeadInitializer`, `BeadReader`, `BeadLifecycle{Create+Apply}`, `BeadEventReader`, `BeadEventWriter`) declared. `*Store` satisfies `BeadInitializer`, `BeadReader`, and `BeadLifecycle`; event interface satisfaction is deferred to E.
- `*Store.Apply(ctx, id, op Operation) error` exists and type-asserts to `OperationApplier`.
- `JSONLBackend` implements `OperationApplier` with empty switch + generic fallback.
- ctx threaded through `*Store`'s non-event foundational read/write methods.
- Bead data model invariants (§11.2) asserted via `TestBeadDataModel_InvariantsHold`.

### 23.2 Phase 2 — Per-concern beads

Each per-concern bead is atomic: it adds its helpers + `Operation` types + ctx-aware interface satisfaction, migrates the relevant callers, and removes the corresponding `*Store` methods.

**Per-concern bead AC** (specific test names and method removal lists in each bead's description):

- **`L` (ddx-bca628fa)** — Lifecycle helpers in `ops/lifecycle/`; removes 7 `*Store` methods (`TransitionLifecycle`, `SetLifecycleStatus`, `UpdateWithLifecycleStatus`, `CloseWithEvidence`, `AppendNotes`, `Reopen`, `ParkToProposed`).
- **`C` (ddx-e1c743d3)** — Claim/Heartbeat helpers in `ops/claim/` with sidecar optimization preserved via `JSONLBackend.Apply` switch; removes 7 `*Store` methods.
- **`E` (ddx-f39b41b3)** — Event log ctx threading; satisfies `BeadEventReader`/`Writer`; signature-only.
- **`X` (ddx-aed7c7ab)** — Cancel + Cooldown helpers; removes 6 `*Store` methods.
- **`Q` (ddx-bc07270f)** — BeadQueries ctx threading on 12 `*Store` methods; opportunistic caller narrowing.
- **`D` (ddx-3af1c1a6)** — Dependencies ctx + Queue helpers; removes 3 `*Store` queue methods.
- **`A` (ddx-71d3a2de)** — Archive + Interchange ctx threading; satisfies `BeadArchive`/`BeadInterchange{Reader,Writer}`.
- **`MIG` (ddx-0ab16765)** — Migrator interface + factory; removes 7 `*Store` migration methods.

**Cross-bead invariant** after Phase 2 completes: `*Store`'s workflow-method surface is gone — only foundational read/write/event/query/dep/archive/interchange methods remain, all ctx-aware.

### 23.3 Phase 3 — Supporting + Axon cluster

- **`WH` (ddx-900a8d38)** — `WatcherHub` takes `BeadReader` factory; verified by `TestWatcherHub_UsesProvidedFactory`.
- **`LINT` (ddx-e91a45c0)** — 5 Go analyzers + CI gate; allowlists committed; per-analyzer positive/negative/precision tests.
- **`SR` (ddx-8bf23be0)** — `axon/schema.graphql` reconciled with the per-row ops AxonStore invokes.
- **`AX` (ddx-9c5bca8f)** — AxonStore implements `Backend` directly via per-row Postgres ops; passes the parameterized conformance suite (with httptest-served Axon endpoint). Includes `TestOperationCatalog_AxonStoreSwitchCoverage` (reflection asserts every `Operation` type is handled by the type-switch, with explicit annotation when generic fallback is acceptable).
- **`CF` (ddx-29f02cf4)** — `NewStore(opts) (Backend, error)` factory; capability validation at startup; mismatch returns error.
- **`CONF` (ddx-958b8fc3)** — parameterized conformance suite runs identical tests against both `*Store` (JSONL + External) and `AxonStore` (httptest mock); behavior parity verified.
- **`SS` (ddx-8dd19492)** — `LifecycleSubscriber` end-to-end smoke test; AxonStore native subscriptions when available, polling fallback otherwise.

### 23.4 Phase 4 — Boundary Lockdown

**`BL` (ddx-74452926):** see bead description for the 10-item AC list.

Key cross-bead invariants after BL lands:

- Every caller in scope has its `*bead.Store` field types narrowed to `bead.Backend` (or sub-interface), with documented allowlist exceptions (`bead_doctor.go`, file-level paths in `bead.go`, `sync.go`).
- `cli/internal/bead/internal/storage/` and `cli/internal/bead/internal/lifecycle/` host the concrete implementations; reachable only from `cli/internal/bead/`.
- `cli/internal/bead/factory.go` is the sole public construction path; exports `NewStore`, `NewLifecycleSubscriber`, `NewMigrator`.
- `Backend` composite + `ReadOnlyBackend` declared; compile-time `var _ Backend = (*storage.Store)(nil)` asserts satisfaction.
- `TestModuleBoundary_NoInternalImportsOutsideBead` (AST-based) asserts no package outside `cli/internal/bead/` imports `internal/storage/` or `internal/lifecycle/`.
- Lint suite drops the two transitional analyzers (`concrete-store-methods`, `no-internal-store-construction`); Go's `internal/` rule enforces the same property at the compiler level.
- `cd cli && go build ./...` succeeds — confirms Phase 2 fully completed.

### 23.5 Phase 5 — Production deployment (downstream)

- **`IMP` (ddx-53df5e2f)** — JSONL→Postgres importer; replaces legacy `MigrateToAxon` JSONL-writer; supports dry-run/apply/verify; idempotent on re-run; preserves field fidelity + event ordering + attachments.
- **`WIRE` (ddx-c479688b)** — real-wire Axon integration tests against actual Postgres; build-tagged `integration`; CI workflow `integration-axon.yml` provisioned; tests covering CAS atomicity under concurrency, native heartbeat UPDATE, transaction isolation, NULL/JSONB handling, timestamp precision, schema_version defaults.

### Workstream completion criterion

The workstream is "done" when:

1. All 19 active beads (Phases 1-5) are closed with their per-bead AC satisfied.
2. The conformance suite passes against both `*Store` (JSONL) and `AxonStore` (httptest + real-wire).
3. `cd cli && go build ./...` succeeds (BL gate).
4. `cd cli && go test ./...` is green.
5. `lefthook run pre-commit` passes.
6. `TestModuleBoundary_NoInternalImportsOutsideBead` passes (physical impossibility verified).

After completion, the gating bead `ddx-c6317784` (F) chain is unwound; the workstream parent `ddx-8d747049` (axon backend prototype umbrella) can close.

## 24. Risks

| Risk | Mitigation |
|------|------------|
| Gating bead lands incomplete; *Store concrete-method rewrite is missed | `TestStore_HeartbeatRoutesThroughApply` (and equivalents) use instrumented `RawBackend` to verify routing. Verifiable, not "review verifies." |
| Heartbeat sidecar optimization loses ground under Apply pattern | `JSONLBackend.Apply` (via `OperationApplier`) type-switches `SetClaimHeartbeat` to sidecar-only write. `*Store.Apply` delegates via type-assertion. Combined tests verify end-to-end. |
| Operation type evolution — adding a field to an op later breaks backends type-switching on the old shape | Additive field changes are safe (backends type-switch on struct type; new fields zero-valued when ignored). Removing/renaming is breaking — operation types are public API once shipped. Treat them like database schema. |
| New backend contributor doesn't know what operations to optimize | Canonical catalog lives in `cli/internal/bead/operation.go` package doc. Conformance test `TestOperationCatalog_AxonStoreSwitchCoverage` enumerates via reflection. |
| Bead data model invariants drift from implementation | `TestBeadDataModel_InvariantsHold` asserts §11.2 invariants on every backend in the conformance suite. |
| Schema-version migrations land breaking changes without ADR-004 update | CI guard checks that `bead-record.schema.json` and persisted-status enum changes touch ADR-004 + TD-027 in the same commit (per §22). |
| Caller migration to ctx-aware interfaces is large scope | Phase 2 is split into per-concern beads with focused caller sets and concrete method-removal checks. |
| Module-boundary lockdown breaks compilation when caller migration is incomplete | That's the point. Phase 4 only succeeds if Phase 2 fully completed. |
| AxonStore implementation finds operation patterns we didn't anticipate | The Operation catalog can be extended additively. AxonStore conformance suite catches behavior drift. |
| Drain-loop content drift (TD-031 references events/labels added without updating TD-027) | §22 makes this a normative process rule; CI guard for schema changes; reviewers check cross-doc consistency. |

## 25. Open Questions

1. **Subscription failure mode** — when a backend can't subscribe, `SubscribeLifecycle` returns `(nil, nil, ErrUnsupported)`. Callers fall back to polling.
2. **`lifecycle/` helper package scope** — encodes HELIX state-machine rules. Future architectural improvement: move to HELIX plugin so generic bead ops stay workflow-agnostic. Deferred.
3. **Schema-version v1 → v2 migration** — when the first breaking change lands, the migration ladder is exercised. No v2 currently planned.
4. **Event externalization threshold** (§17) — when does a bead's `events` array get pushed to a sidecar? Default: keep inline; externalize only on explicit operator command or when total bead size exceeds a configured threshold. Threshold value TBD on first observation of large-event beads in production.
