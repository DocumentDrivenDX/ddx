---
ddx:
  id: plan-2026-05-11-bead-backend-interfaces
---
# Bead Backend Interface Refinement (Pre-Axon)

Date: 2026-05-11
Status: Draft — gates the Axon backend rebuild and the broader storage-abstractions work

## Why this exists

The current `cli/internal/bead/backend.go` declares two interfaces:

- **`RawBackend`** (4 methods) — whole-corpus `Init/ReadAll/WriteAll/WithLock`.
- **`Backend`** (22 methods) — high-level CRUD, claim, query, dep ops, events, archive, interchange.

Three concrete problems block the Axon production-readiness work captured in `docs/plans/plan-2026-05-10-axon-only-architecture.md`:

1. **`Backend` is incomplete.** `*Store` exposes **69 public methods** (verified via `grep -E "^func \(s \*Store\) [A-Z]"`); `Backend` declares 22. The other ~47 are concrete-only on `*Store`. They include real caller surface like `Heartbeat`, `TouchClaimHeartbeat`, `ClaimHeartbeatFresh`, `Status`, `BlockedAll`, `ListWithArchive`, `GetWithArchive`, `ReadAllFiltered`, `EventsByKind`, `ExternalBlocked`, `DependencyWaiting`, `ProposedOperatorAttention`, `NeedsHuman`, `ReadyExecution`, `ReadyExecutionBreakdown`, `CloseWithEvidence`, `AppendNotes`, `Reopen`, `ParkToProposed`, `TransitionLifecycle`, `SetLifecycleStatus`, `UpdateWithLifecycleStatus`, `RequestCancel`, `IsCancelRequested`, `MarkCancelHonored`, `SetExecutionCooldown`, `ClearCooldowns`, `IncrNoChangesCount`, `GenID`, `QueueClear`, `QueueMove`, `QueueTop`, `ArchiveWithEvents`, `ExportToFile`. Plus a migration family (`MigrateLifecycle`, `MigrateFromHelix`, `MigrateToAxon`, `MigrateDryRun`, `MigrateLifecycleDryRun`, `DetectLifecycleMigrationRequired`, `ReconcileLifecycleMetadata`) and lifecycle-schema-marker bootstrap (`HasLifecycleSchemaMarker`, `WriteLifecycleSchemaMarker`, `LifecycleSchemaMarkerPath`) that are intentionally not steady-state. Real callers in `cli/cmd/` and `cli/internal/` reach for the steady-state extras every day. The "any backend can be swapped" promise is unenforceable because the interface doesn't declare what callers use.
2. **`RawBackend` granularity is wrong for non-JSONL backends.** The whole-corpus `ReadAll → mutate → WriteAll` pattern is fine for a single JSONL file (lock + atomic rename = correctness) and catastrophic for Postgres (no per-row UPSERT, no optimistic concurrency, no indexes used). Today's `AxonBackend.WriteAll` ships the entire corpus on every mutation.
3. **No interface seam for "Axon implements `Backend` directly."** `*Store` is the only `Backend` implementation. To let an Axon-native implementation slot in alongside (per Option B in the prior turn), `Backend` has to be the contract that callers depend on — not `*Store` concretely.

## Locked decisions (from prior conversation)

- **Option B**: Axon implements `Backend` directly, alongside `*Store`'s composition path. Both satisfy the same contract; callers don't notice. `JSONLBackend` keeps `RawBackend` and feeds `*Store` as today.
- **Promote `*Store` extras onto interface(s)**, driven by what callers actually use. Whether Axon can support every extra is a separate, longer-term question.
- **Preserve existing direct interfaces.** No breaking changes to `Backend` or `RawBackend` signatures. All work is additive.
- **BlobStore stays in DDx (FEAT-028).** Axon-as-blob-backend is a future option, not a constraint here.
- **Apply SOLID, especially Interface Segregation.** Don't make every caller depend on the full `Backend` surface. Split by responsibility into cohesive sub-interfaces.

## Caller usage data

Counts of `store.<Method>(` references across `cli/cmd/` + `cli/internal/{server,agent,agentmetrics,exec,escalation,processmetrics}/` (non-test):

| Method | Internal-pkg usage | Method | Internal-pkg usage |
|--------|---|--------|---|
| `Create` | 152 | `WriteAll` | 5 |
| `Get` | 145 | `SetExecutionCooldown` | 4 |
| `Init` | 105 | `Status` | 3 |
| `Events` | 74 | `ReadyExecution` | 3 |
| `AppendEvent` | 44 | `Ready` | 2 |
| `Update` | 18 | `MarkCancelHonored` | 2 |
| `EventsByKind` | 12 | `List` | 2 |
| `Claim` | 11 | `ExternalBlocked` | 2 |
| `UpdateWithLifecycleStatus` | 9 | `DepTree` | 2 |
| `ReadAll` | 8 | `DependencyWaiting` | 2 |
| `Unclaim` | 6 | `CloseWithEvidence` | 2 |
| `ClaimWithOptions` | 2 | `RequestCancel`/`Reopen`/`IsCancelRequested`/`DepRemove`/`DepAdd` | 1 each |

Plus `Close`, which appears 52 times in `cli/cmd/` but is overcounted (other types have `.Close()`); real bead-store `Close` is a substantial caller set.

Two takeaways:

- **CRUD + events dominate** (Get/Create/Init/Events/AppendEvent/Update/EventsByKind = ~560 of ~620 calls). Whatever interface a caller programs against, it almost always needs these.
- **The "long tail" extras are low-frequency but cohesive in pairs/triples**: cancel methods together, cooldown methods together, lifecycle methods together. These are exactly the cases ISP is designed for — give each cohesive group its own interface, let callers depend on the smallest set they need.

## Proposed sub-interface taxonomy

12 cohesive interfaces, grouped by concern, accounting for **all** of `*Store`'s 69 public methods (with explicit non-interface bucket for the migration/bootstrap family). Names are domain-shaped (`BeadCore`, `BeadEvents`) rather than role-shaped (`BeadReader`, `BeadWriter`) to keep the surface count manageable while still allowing ISP-style narrow dependencies.

```go
package bead

// BeadCore is the foundational CRUD + ID generation that nearly every
// caller needs. ReadAllFiltered + GetWithArchive + ListWithArchive variants
// are part of the core read surface (callers depend on them widely).
type BeadCore interface {
    Init() error
    GenID() (string, error)
    ReadAll() ([]Bead, error)
    ReadAllFiltered(pred func(Bead) bool) ([]Bead, error)
    Get(id string) (*Bead, error)
    GetWithArchive(id string) (*Bead, error)
    Create(b *Bead) error
    Update(id string, mutate func(*Bead)) error
    Close(id string) error
}

// BeadEvents is the event log per bead. High-volume usage; nearly every
// caller that does a state transition appends an event.
type BeadEvents interface {
    AppendEvent(id string, event BeadEvent) error
    Events(id string) ([]BeadEvent, error)
    EventsByKind(id, kind string) ([]BeadEvent, error)
}

// BeadLifecycle is the transition surface that enforces lifecycle gates
// and writes provenance (closing commit, evidence). Distinct from
// BeadCore.Update because callers don't always need lifecycle semantics.
type BeadLifecycle interface {
    TransitionLifecycle(id, status string, opts LifecycleTransitionOptions, mutate func(*Bead) error) error
    SetLifecycleStatus(id, status string, opts LifecycleTransitionOptions) error
    UpdateWithLifecycleStatus(id, status string, opts LifecycleTransitionOptions, mutate func(*Bead) error) error
    CloseWithEvidence(id, sessionID, commitSHA string) error
    AppendNotes(id, notes string) error
    Reopen(id, reason, notes string) error
    ParkToProposed(id, reason string) error
}

// BeadClaiming is worker assignment + liveness. Heartbeat freshness primitives
// (Touch/Remove/Fresh) are bundled here because they're operationally part
// of the same claim-lifetime concern.
type BeadClaiming interface {
    Claim(id, assignee string) error
    ClaimWithOptions(id, assignee, session, worktree string) error
    Unclaim(id string) error
    Heartbeat(id string) error
    TouchClaimHeartbeat(id string) error
    RemoveClaimHeartbeat(id string) error
    ClaimHeartbeatFresh(id string, maxAge time.Duration) (bool, error)
}

// BeadCancellation is cooperative cancellation signaling.
type BeadCancellation interface {
    RequestCancel(id string) (bool, error)
    IsCancelRequested(id string) (bool, error)
    MarkCancelHonored(id string) error
}

// BeadCooldown is retry/backoff state for failed attempts.
type BeadCooldown interface {
    SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error
    ClearCooldowns(filter func(Bead) bool) (int, error)
    IncrNoChangesCount(id string) (int, error)
}

// BeadQueries is read-only filtering and aggregation. Backends with native
// query support (Axon/Postgres) implement directly; whole-corpus backends
// can implement by scanning ReadAll() + filtering in memory.
type BeadQueries interface {
    List(status, label string, where map[string]string) ([]Bead, error)
    ListWithArchive(status, label string, where map[string]string) ([]Bead, error)
    Ready() ([]Bead, error)
    ReadyExecution() ([]Bead, error)
    ReadyExecutionBreakdown() (ReadyExecutionBreakdown, error)
    ProposedOperatorAttention() ([]Bead, error)
    NeedsHuman() ([]Bead, error)
    Blocked() ([]Bead, error)
    ExternalBlocked() ([]Bead, error)
    DependencyWaiting() ([]Bead, error)
    BlockedAll() ([]BlockedBead, error)
    Status() (*StatusCounts, error)
}

// BeadDependencies is the dep-graph surface.
type BeadDependencies interface {
    DepAdd(id, depID string) error
    DepRemove(id, depID string) error
    DepTree(rootID string) (string, error)
}

// BeadQueue is operator-driven queue manipulation — pinning, reordering,
// clearing. Distinct from BeadQueries (which derives readiness) because
// these are explicit operator actions that mutate queue ordering metadata.
type BeadQueue interface {
    QueueTop(id string) error
    QueueMove(id string, position int) error
    QueueClear() error
}

// BeadArchive is operational maintenance — splitting closed beads
// out of the active corpus. ArchiveWithEvents preserves event history.
type BeadArchive interface {
    Archive(policy ArchivePolicy) ([]string, error)
    ArchiveWithEvents(policy ArchivePolicy) ([]string, error)
    Migrate() (MigrateStats, error)
}

// BeadInterchange is JSONL import/export. Useful for backups, dev
// workflows, cross-backend migration.
type BeadInterchange interface {
    Import(source, filePath string) (int, error)
    ExportTo(w io.Writer) error
    ExportToFile(path string) error
}

// BeadSubscription is push-based change notification. JSONLBackend
// satisfies this via the existing polling WatcherHub
// (cli/internal/bead/watcher.go); Axon will satisfy it natively via
// GraphQL subscriptions on ddx_beads/ddx_bead_events change events.
//
// The signature is derived from WatcherHub.SubscribeLifecycle's existing
// shape (cli/internal/bead/watcher.go:54). Callers (the GraphQL
// beadLifecycle subscription resolver in cli/internal/server/) currently
// depend on WatcherHub concretely; promoting to an interface lets Axon
// substitute a native push transport without changing the resolver.
type BeadSubscription interface {
    SubscribeLifecycle() (events <-chan LifecycleEvent, cancel func())
}
```

### Methods intentionally NOT on any sub-interface

The following `*Store` methods stay as concrete `*Store` calls (not on any backend interface), with rationale:

- **Lifecycle-schema-marker bootstrap** (`HasLifecycleSchemaMarker`, `WriteLifecycleSchemaMarker`, `LifecycleSchemaMarkerPath`) — one-time bootstrap probe, not steady-state. Callers either don't need it or are operator-tooling.
- **Migration family** (`MigrateLifecycle`, `MigrateFromHelix`, `MigrateToAxon`, `MigrateDryRun`, `MigrateLifecycleDryRun`, `DetectLifecycleMigrationRequired`, `ReconcileLifecycleMetadata`) — admin/one-time operations. Cohesive but not needed by application callers; live as concrete `*Store` methods. If multiple backends ever need to expose migration, lift to a `BeadMigration` interface then.
- **`LoadEventsInline`** — JSONL-internal optimization for re-inlining externalized events on read; not a backend-portable concept.
- **`WithLock`** — already on `RawBackend`; infrastructure-level locking primitive, not bead-as-entity.

These exclusions are explicit so future contributors aren't ambiguous about whether they should be on the interface.

## The composite `Backend` (backwards-compatible)

`Backend` becomes the union of all 11 sub-interfaces. Existing callers that depend on `Backend` keep compiling unchanged because every method they need is still there — just sourced from a sub-interface.

```go
// Backend is the full bead-tracker contract. Composition of the
// sub-interfaces above. Existing callers (and the conformance suite)
// continue to depend on Backend; new callers should depend on the
// smallest sub-interface they actually use (ISP).
type Backend interface {
    BeadCore
    BeadEvents
    BeadLifecycle
    BeadClaiming
    BeadCancellation
    BeadCooldown
    BeadQueries
    BeadDependencies
    BeadQueue
    BeadArchive
    BeadInterchange
    BeadSubscription
}
```

`*Store` already implements every method on these interfaces (it's where they came from). Compile-time check `var _ Backend = (*Store)(nil)` enforces this. Adding the assertion is the entire `*Store`-side change — no logic touched.

`RawBackend` is unchanged in shape, but its docstring is updated to warn off new backends:

```go
// RawBackend is the low-level whole-corpus storage contract used by
// JSONLBackend and ExternalBackend (bd/br). It exists for backends
// where corpus-shaped read/write is the natural granularity (single
// file with atomic rename + advisory lock).
//
// NEW BACKENDS SHOULD NOT IMPLEMENT RawBackend. The whole-corpus shape
// is wrong for any backend that supports per-row operations (Postgres,
// any structured store). New backends should implement Backend
// directly — see AxonStore as the reference example. Composition of
// *Store over RawBackend is preserved only for the existing JSONL/
// External implementations.
type RawBackend interface { ... }
```

This stops the next contributor from copy-pasting `AxonBackend`'s mistake of forcing per-row work through whole-corpus `WriteAll`.

## Axon under Option B

Axon implements `Backend` directly. Concretely:

1. `cli/internal/bead/axon_backend.go` (rename optional) grows to satisfy the full `Backend` interface — not by composing `*Store` over `RawBackend`, but by mapping each sub-interface's methods to per-row Axon GraphQL operations (UPSERT/DELETE per row, queries with filters, event appends as separate entity inserts).
2. `cli/internal/bead/store.go`'s `NewStore(...)` factory returns either:
   - `*Store{Raw: JSONLBackend, ...}` (default, today's path) — satisfies `Backend` via the composition layer.
   - `*AxonStore{...}` (or whatever the type is named) when `bead.backend: axon` — satisfies `Backend` via direct per-row implementation.
   Both return values are typed as `Backend`. Callers don't notice.
3. `*AxonStore` declares which sub-interfaces it implements. If Axon long-term cannot support a sub-interface (e.g., `BeadCancellation` because Axon's schema doesn't model cancel-request state), capability negotiation happens **at the factory, against config, at startup** — NOT via runtime type-assertions at call sites. The codebase has zero runtime capability type-asserts today; introducing them would be foreign and would produce surprise errors deep in execution paths.

Concretely, the factory pattern:

```go
// NewStore returns a Backend impl chosen by config. Required capabilities
// are validated against the chosen backend; missing ones fail at startup
// with a clear message rather than at first use.
func NewStore(opts StoreOptions) (Backend, error) {
    backend := constructBackend(opts) // *Store or *AxonStore depending on config
    var missing []string
    for _, cap := range opts.RequiredCapabilities {
        if !cap.SatisfiedBy(backend) {
            missing = append(missing, cap.Name)
        }
    }
    if len(missing) > 0 {
        return nil, fmt.Errorf("backend %q lacks required capabilities: %v",
            opts.BackendName, missing)
    }
    return backend, nil
}
```

`opts.RequiredCapabilities` defaults to "all" for the standard `ddx` CLI (full-feature workstation use). A read-only deployment declares a subset; Axon-backed server declares whatever the production deployment needs. Mismatch fails the server boot with a useful message, not the 47th `ddx bead claim` of the day.

This pattern fits the codebase: backend selection is already config-driven at the factory (`cli/internal/bead/store.go:89-129`); we're extending that selection logic with capability assertions, not introducing a new pattern.

The capability question — *can* Axon model heartbeats, cancellation, cooldown? — is deferred. The interface design lets us answer "yes" or "no" per sub-interface as we go, without changing the contract. That's exactly the SOLID/Open-Closed property we want.

## Caller migration — be honest about the scope

No caller has to change *to keep compiling*. Every existing call site holds `*bead.Store` concretely (`cli/cmd/`, `cli/internal/server/`, `cli/internal/agent/` — verified: zero interface-typed bead variables in production code today). The composite `Backend` covers everything `*Store` exposes; the compile-time `var _ Backend = (*Store)(nil)` assertion enforces it.

**But the caller-side ISP payoff is deferred.** Sub-interfaces buy nothing for callers that hold `*Store` concretely. The immediate payoff is **Axon-side**: it lets the new Axon implementation declare exactly which sub-interfaces it satisfies, and lets the factory validate at startup. Caller-side narrowing (e.g. converting `ddx bead list` to depend on `BeadQueries` only) is **its own follow-on work**, not part of this bead.

This is the right scope for the gating bead because:

- The Axon work needs the sub-interfaces to declare against; that's the load-bearing dependency for the downstream bead chain.
- Caller migration is mechanical and incremental — it can happen file-by-file as callers are touched for other reasons. It's the kind of migration that benefits from being unconstrained by a single bead's scope.
- Trying to migrate ~27 caller files (the production callers from prior turn's audit) inside one bead would dramatically expand the change surface and review burden, which is exactly the kind of thing the prior plan-review feedback warned against.

**Follow-on work** (file separately, not blocking Axon):

- **Caller-narrowing pass**: convert `cli/cmd/bead_*.go` and the `cli/internal/server/graphql/resolver_*.go` callsites to depend on the narrow sub-interface they actually use. Mechanical, file-by-file.
- **Test conformance**: the existing `cli/internal/bead/backend_conformance_test.go` and `chaos_test.go` keep testing against `Backend`. Per-sub-interface conformance suites get added when a backend (Axon) declares a partial capability set.

## What this enables (the load-bearing payoff)

- **Axon can be implemented per-row.** No diff-engine inside `WriteAll`. The interface granularity matches the storage shape.
- **`bead.backend: axon` config selection becomes real.** `NewStore(...)` factory returns the right `Backend` impl from config; existing callers work unchanged. (Closed bead `ddx-29f02cf4`.)
- **Conformance suites can run against multiple backends** declaring matching capability sets. (Closed bead `ddx-958b8fc3`.)
- **Read-only deployment shape** (`docs/plans/plan-2026-05-10-read-only-deployment.md`) becomes expressible as a smaller interface: a read-only Backend impl declares only `BeadCore` (Get/ReadAll), `BeadEvents` (Events/EventsByKind), `BeadQueries`, and `BeadDependencies` (DepTree only). The Create/Update/Close/Claim/etc. sub-interfaces are simply not implemented; callers that need them fail at construction.
- **Future BlobStore-on-Axon** (if Axon exposes a blob interface later) is unrelated — `BlobStore` per FEAT-028 is its own thing in DDx, with `LocalFS` today and `UC Volumes` as a candidate future backend. This design note is purely about bead entity storage.

## Risks / open questions

- **Method placement vs. signatures.** Every method placed on a sub-interface above was verified present on `*Store` via `grep -E "^func \(s \*Store\) [A-Z]"`. Signatures need to be confirmed line-by-line during the implementation bead (the prototypes above are designer's transcription, not extracted from source). Any mismatches surface during compile of the `var _ Backend = (*Store)(nil)` assertion.
- **Bulk operations are deferred.** `BeadCore` has no `BatchCreate`/`BatchUpdate`. Axon could support these efficiently; today the interface doesn't allow expressing it. Add only when a concrete caller needs it. Documented here so downstream beads don't quietly assume bulk semantics.
- **Transactions / atomicity across operations are deferred.** No `WithTransaction(fn)` method. Today's `WithLock` (on `RawBackend`) gives JSONL its serialized rewrites; Axon-via-Backend would need its own transactional pattern (per-row UPSERTs are individually atomic, but multi-row consistency is on the caller). Documented here so downstream Axon work makes a deliberate choice rather than inheriting a default.
- **Pagination on `BeadQueries` is deferred.** `List`/`Ready`/etc. return full slices; that's fine for current scale (per-project bead counts are bounded) but breaks if multi-project federation hits this surface. Add `ListPaged(opts) ([]Bead, cursor, error)` when a concrete caller hits the limit.
- **Authentication/authorization context propagation is deferred.** No `context.Context` on the interface methods today; this design keeps that. A multi-tenant Databricks-App deployment will need ctx-aware methods to propagate identity. Added as a future-update note rather than a v1 surface change because adding `ctx` to every method is a breaking change that should be evaluated against actual auth requirements, not pre-emptively.
- **Conformance test matrix complexity.** 12 sub-interfaces × N backends is real cost. Mitigation: parameterize the existing `backend_conformance_test.go` by sub-interface; backends that don't implement a given sub-interface are skipped in that subtest. JSONLBackend implements all 12; AxonBackend will declare its set. Most pairs are present, so the matrix is sparse-but-mostly-full.

## Sequencing

1. **This bead (the interface refinement)**: add the 12 sub-interface declarations + the composite `Backend` redefinition + the updated `RawBackend` docstring warning new backends off + the `var _ Backend = (*Store)(nil)` compile-time check + per-sub-interface `var _ BeadCore = (*Store)(nil)` (etc.) checks + promote `WatcherHub` to satisfy `BeadSubscription` (`SubscribeLifecycle` already has the right shape — just add the interface declaration). Pure declarations + one compile-time satisfies-check on the watcher. Zero runtime behavior change. Conformance suite continues to pass against `*Store`. **No `cli/cmd/` or `cli/internal/` caller changes** (caller-narrowing pass is its own follow-on, see "Caller migration" above).
2. **Reopen `ddx-9c5bca8f`** rescoped: implement `Backend` directly on Axon (or on `AxonStore`-shaped type), mapping each sub-interface to per-row Axon ops. Per-sub-interface conformance is the AC.
3. **Reopen `ddx-29f02cf4`** rescoped: `NewStore(...)` factory returns the right `Backend` impl from config; `WithAxonGraphQLTransport` is constructed from config.
4. **Reopen `ddx-8bf23be0`**: reconcile `schema.graphql` with the per-entity ops the generated client exposes (so the GraphQL surface ddx invokes actually parses against what's published).
5. **Reopen `ddx-958b8fc3`**: parameterize the conformance suite across backends, with per-sub-interface declared capability matching.
6. **Reopen `ddx-8dd19492`**: subscription smoke test against the ops AxonBackend actually invokes.
7. **New beads**:
   - Schema versioning + v0→v1 migration ladder (audit gap 4).
   - Real JSONL→Postgres importer to replace today's JSONL-writing `MigrateToAxon` (audit gap 6).
   - Real-wire integration tests against an actual Axon/Postgres instance (audit gap 7).

The interface refinement bead is the gate. Nothing else can land until the sub-interfaces exist as the contract Axon implements against.
