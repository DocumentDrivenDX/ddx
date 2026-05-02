---
ddx:
  id: TD-030
  depends_on:
    - ADR-004
    - SD-004
    - FEAT-004
  status: draft
---
# Technical Design: Axon Bead-Tracker Backend

## Status

Draft. Specifies the integration between the DDx bead tracker
(FEAT-004 / SD-004 / ADR-004) and an external Axon entity-graph-relational
store as a pluggable storage backend behind the
`bead_tracker.backend` switch introduced in TD-027.

## Background

Source plan: `/tmp/ddx-axon-backend-plan.md`. That document captured the
field-by-field schema mapping, operation mapping, migration plan, and
open questions that fed the operator review. The locked decisions from
that review are baked into this TD; the deferred-default decisions are
adopted as v1 policy below. This TD does not relitigate those decisions
— it polishes them into an implementable design.

Axon is a separate Rust project (`/home/erik/Projects/axon`) exposing an
entity-graph-relational store over gRPC, REST, GraphQL, and MCP.
Backing stores are pluggable per deployment (PostgreSQL, SQLite,
FoundationDB, fjall) per Axon SPIKE-001. Axon ships an opinionated
`bead.rs` module with its own `BeadStatus` state machine; **DDx does
not consume that module** — DDx beads have a different status set
(`open`/`in_progress`/`closed`/`blocked` plus claim semantics), an
append-only events array, and bd/br JSONL interchange requirements that
do not match Axon's built-in shape. DDx instead defines its own
collection (`ddx_beads`) and JSON Schema and uses Axon as a generic
typed-entity store.

## Locked decisions

These are operator-approved and not negotiable inside this TD:

| # | Decision | Implication for design |
|---|---|---|
| D1 | **Deployment**: separate axon-server, operator-managed. | DDx does **not** spawn or supervise Axon. `ddx-server` has no responsibility for Axon's lifecycle. The operator runs Axon via systemd / docker-compose / etc. and configures the connection string in `.ddx/config.yaml`. `ddx doctor` gains an Axon-reachability diagnostic. |
| D2 | **Auth**: localhost-only by default; ts-net for remote. | Reuses ADR-006 (tsnet authentication). No new auth machinery in DDx. For remote Axon endpoints, DDx dials through a tsnet listener; for local Axon, plaintext gRPC on `127.0.0.1` is acceptable. |
| D3 | **Events storage**: separate `ddx_bead_events` collection with `event_of` links (Option B). | Two collections: `ddx_beads` (the bead row) and `ddx_bead_events` (one entity per event). Bead row stays light; events scale per-bead independently. |
| D4 | **Offline posture**: refuse all bead ops with a clear error when Axon is unreachable. | Axon is the source of truth. No local read-through cache, no WAL, no degraded-mode reads. `ddx bead *` and `ddx work` fail loudly with `axon unreachable at <addr>: <error>; check connectivity / config`. |

## v1 policy defaults (deferred decisions)

These can be revisited if implementation surfaces a problem; they are
not load-bearing for the architecture:

| Question | v1 policy | Rationale |
|---|---|---|
| Schema versioning | **Lazy migrate on read.** When DDx reads an entity at an older `schema_version`, it transforms in-memory, returns to the caller, and writes back at the new version on the next mutation. A future `ddx bead schema upgrade` command exists for breaking changes. | Avoids large eager-migrate batches; standard pattern. |
| `ddx bead ready` query | **Two-phase**: `QueryEntities(status=open)`, then per-candidate `Traverse(depends_on, outgoing)` to check whether each dep is closed. Benchmark on the 1100-bead archive scale (see Test plan §); if too slow, escalate to a server-side computed view (see Open Questions for Axon §1) or a derived `is_ready` index field maintained at write time. | Simplest correct implementation with a clear performance escalation path. |
| Backup / DR | **Operator-managed via Axon's own backing-store DR.** `ddx bead export` is the documented logical backup format. | Avoids DDx duplicating Axon's DR concerns. Aligns with operator-managed deployment (D1). |

## Architectural decision

DDx defines a `ddx_beads` collection (and a companion `ddx_bead_events`
collection per D3) in Axon, each with a DDx-controlled JSON Schema
registered via Axon `PutSchema`. Axon serves as a generic
entity-graph-relational store; bead semantics (state machine, claim
flow, ready-queue computation) live in `cli/internal/bead`. Axon's
opinionated `bead.rs` is informational only.

Rationale (from source plan, recorded for the record):

- Preserves the DDx schema (FEAT-004 / SD-004) without forcing Axon's
  status model onto DDx.
- Maintains bd/br interchange compatibility — `ddx bead export | bd
  import` round-trip is codified in
  `cli/internal/bead/schema_compat_test.go`.
- Lets HELIX and other plugins extend the bead `Extra` map without
  coordinating with Axon's `bead.rs` schema.
- DDx ships per-collection schemas as artifacts under
  `cli/internal/bead/schema/` and pushes them via `PutSchema` on first
  use.

## Wire transport

Use **gRPC**. DDx generates a Go client from `axon.proto` via
`protoc-gen-go` + `protoc-gen-go-grpc`. The generated client lives at
`cli/internal/bead/axon/pb/` with a `make gen-axon-client` target;
`axon.proto` is checked in alongside its sha256 sum so consumer/producer
drift surfaces in CI. REST and MCP are not used; they remain available
fallbacks if gRPC adoption proves operationally heavy.

Axon version pin: the build pins a specific Axon release tag (e.g.
`axon@v0.x.y`) and tracks Axon's CHANGELOG. Axon is pre-release; wire
protocol and schema model are not yet frozen, so the pin is mandatory.

## Schema mapping

DDx bead record (per `cli/internal/bead/marshal.go` and
`cli/internal/bead/schema/bead-record.schema.json`) → Axon entity in
collection `ddx_beads`:

| DDx bead field | Axon mapping |
|---|---|
| `id` | Axon `EntityId` (string; preserves the existing `ddx-<8hex>` form) |
| `title` | `data_json.title` |
| `description` | `data_json.description` |
| `acceptance` | `data_json.acceptance` |
| `issue_type` (task/bug/epic/chore) | `data_json.issue_type` (string enum) |
| `status` (open/in_progress/closed/blocked) | `data_json.status` (string enum) — validated by the registered JSON Schema; **not** routed through Axon's `TransitionLifecycle` (DDx owns the state machine) |
| `priority`, `labels`, `created_at`, `updated_at`, `assignee`, `claimed_at`, `claimed_pid` | `data_json.*` (verbatim field-for-field) |
| `parent` | Axon **link** of type `parent`, child → parent entity |
| `dependencies` | Axon **links** of type `depends_on`, this → each dependency target |
| `events` | per D3, **Option B**: each event is its own entity in `ddx_bead_events`, linked back to the bead via an `event_of` link. The bead row's `data_json` does **not** carry the events array. Export materialises the inline events array for bd/br interchange (see Operation mapping). |
| `Extra` (HELIX/plugin custom fields) | `data_json.extra` with `additionalProperties: true` at that path; preserves round-trip |
| `version` (optimistic concurrency) | `EntityProto.version` from Axon (returned on every Get; sent as `expected_version` on Update) |

### Schema artifacts

Two new schema files, both under `cli/internal/bead/schema/`:

- `bead-record-axon.schema.json` — extends `bead-record.schema.json`,
  registered as the schema for collection `ddx_beads`.
- `bead-event-axon.schema.json` — schema for collection
  `ddx_bead_events`. Fields: `event_id`, `at`, `kind`, `payload`, plus
  `event_of` link target.

Both schemas carry an explicit `schema_version` (starts at `1`).
Schema upgrades push a new version via `PutSchema`; live entities
migrate lazily on read per the v1 policy.

## Operation mapping

| DDx operation | Axon RPC(s) |
|---|---|
| `ddx bead create` | `CreateEntity(collection=ddx_beads)` — wrapped in `CommitTransaction` if the same call also creates dep links |
| `ddx bead show <id>` | `GetEntity` for the bead row + `Traverse(depends_on, outgoing)` + `Traverse(parent, outgoing)` + `Traverse(event_of, incoming)` and `GetEntity` per event entity. May escalate to a single server-side aggregation if Axon adds one — see Open Questions for Axon §1. |
| `ddx bead update` | `UpdateEntity` with `expected_version` (OCC) |
| `ddx bead close` | `UpdateEntity` (status → closed) — may include the closing event-write in the same `CommitTransaction` |
| `ddx bead claim` | `UpdateEntity` (status → in_progress + `claimed_at` + `claimed_pid`) — atomic via OCC: on version mismatch, the claim lost the race and the caller retries (or surfaces "claimed by another worker") |
| `ddx bead ready` | per v1 policy: `QueryEntities(status=open)` then per-candidate `Traverse(depends_on, outgoing)` checking dependency status; escalation paths documented above |
| `ddx bead blocked` | symmetric to ready: query `status=open` then filter to those with at least one non-closed dependency |
| `ddx bead dep add/remove` | `CreateLink` / `DeleteLink` (link type `depends_on`) |
| `ddx bead dep tree` | `Traverse(depends_on, outgoing, transitive)` |
| `ddx bead evidence add` | `CreateEntity(collection=ddx_bead_events)` + `CreateLink(event_of)` in one `CommitTransaction` |
| `ddx bead export` | iterate `QueryEntities(collection=ddx_beads)`, fan-out to events via `Traverse(event_of, incoming)`, materialise the inline events array, marshal to bd/br JSONL — must round-trip per FEAT-004 + `schema_compat_test.go` |
| `ddx bead import` | reverse: parse JSONL, split into bead row + event entities, write each bead inside a transaction |

### Concurrency model

Axon's `expected_version` (OCC) plus `CommitTransaction` (ACID over
batched ops) replace **both** of the current JSONL backend's
serialization mechanisms:

- the `flock` on `.ddx/beads.jsonl` (acquired via
  `JSONLBackend.WithLock` per `cli/internal/bead/backend_jsonl.go:74`)
- the git tracker-commit lock that motivated bead `ddx-da11a34a` (the
  v1 patch in `cli/internal/bead/lock.go` serializing
  `git add .ddx/beads.jsonl && git commit`)

Multi-worker concurrency falls out for free — each DDx worker is just
another gRPC client. The `git add .ddx/beads.jsonl && git commit` step
retires entirely under the Axon backend: bead state lives in Axon, not
on disk under git.

### Multi-machine story (Story 14 prerequisite)

FEAT-026 / Story 14 (per ADR-007 federation topology) needs a single
authoritative bead store that multiple machines can read and mutate
without git as the synchronisation channel. The JSONL backend cannot
satisfy that contract — it relies on a shared filesystem and a single
git remote as the merge point, and ADR-021 already calls out that the
Axon path "gets the right answer for free" for the multi-node story.
This TD is the prerequisite work: once `bead_tracker.backend: axon` is
the configured backend, every DDx worker on every machine speaks gRPC
to the same Axon endpoint and OCC handles cross-machine races
identically to single-machine cross-process races. No new federation
machinery is needed inside DDx; FEAT-026 reduces to "operators run one
Axon and point all `.ddx/config.yaml` files at it" plus the auth story
in D2 (localhost / tsnet via ADR-006).

## Deployment and configuration

`.ddx/config.yaml` schema additions:

```yaml
bead_tracker:
  backend: axon            # one of: jsonl (current default), bd, axon
  axon:
    address: localhost:50051     # gRPC endpoint (host:port)
    database: ddx-default        # Axon database name
    namespace: project-<name>    # one Axon namespace per DDx project
    collection: ddx_beads        # default; configurable
    events_collection: ddx_bead_events
    schema_version: 1
    auth:
      mode: localhost            # 'localhost' | 'tsnet'
      tsnet:                     # only when mode=tsnet, per ADR-006
        hostname: ddx-<project>
        # remaining tsnet fields per ADR-006
```

`auth.mode: localhost` requires `address` to resolve to a loopback
host; DDx refuses to dial non-loopback addresses without `mode: tsnet`
(matches the security stance in `concerns.md`).

### `ddx doctor` diagnostic

`ddx doctor` adds a `bead-tracker` section. When `backend: axon` is
configured, the diagnostic:

1. Resolves `address` and dials the gRPC endpoint with a 2s timeout.
2. Calls `DescribeCollection(ddx_beads)` and `DescribeCollection(ddx_bead_events)`.
3. Verifies that the registered schema version matches the configured
   `schema_version`.
4. Surfaces actionable errors: unreachable, collection missing, schema
   missing, schema version mismatch.

Failure of the doctor check does **not** automatically initialize Axon
state — operators run `ddx bead migrate --to=axon` (see Migration plan)
which is responsible for collection/schema bring-up.

## Migration plan

Implements bead `ddx-155204fd` (B_MIGRATE). Command:
`ddx bead migrate --to=axon`.

1. **Pre-flight**: dial Axon; verify or `PutSchema` for both
   collections; `CreateCollection` if absent.
2. **Read source**: `.ddx/beads.jsonl`, `.ddx/beads-archive.jsonl`,
   and `.ddx/attachments/*` (externalised events from
   `ddx bead migrate` under the JSONL backend, per ADR-004).
3. **Per bead**, in one `CommitTransaction`:
   - `CreateEntity(collection=ddx_beads)` with the mapped data
   - `CreateLink(depends_on)` for each dependency
   - `CreateLink(parent)` if applicable
   - For each event: `CreateEntity(collection=ddx_bead_events)` +
     `CreateLink(event_of)`
4. **Verify**: pre-migration bead count = post-migration count; sample
   N=20 random beads and hash-compare `data_json` plus traversed link
   sets.
5. **On success**: rename `.ddx/beads.jsonl` →
   `.ddx/beads.jsonl.pre-axon-migration.bak` (retained for the rollback
   window; not deleted).
6. **Switch** `bead_tracker.backend: axon` in `.ddx/config.yaml`.

The migration is idempotent: re-running on a partially or fully
migrated store is a no-op via OCC version checks plus presence checks
on the target entity ids.

## Test plan

1. **Backend conformance**: parameterise the existing
   `cli/internal/bead/chaos_test.go` suite (10+ chaos tests) by backend
   selector (`jsonl`, `axon`); the Axon-backed runs must pass identically.
2. **Unit/integration** under `cli/internal/bead/axon/`:
   - `integration_test.go`: round-trip create → get → update → claim →
     close → re-read against a real Axon test instance
   - concurrent-claim race: two goroutines race a claim on the same
     bead; exactly one wins; the loser observes a version mismatch
   - dependency add/remove with `Traverse` verification
   - export → bd-shape JSONL → import round-trip; assert byte-equal
     after a re-export (per `schema_compat_test.go`)
3. **End-to-end**: spawn 2 DDx workers against a shared Axon, queue 50
   trivial beads, both drain to closure with no contention errors —
   replaces the workaround in `ddx-da11a34a`.
4. **Performance benchmark**: queue ops over the existing 1100-bead
   archive-scale fixture; assert parity-or-better against the JSONL
   backend on `bead ready`, `bead show`, and `bead create`. Failure
   here triggers the v1-policy escalation path for `bead ready`
   (server-side view or derived index).
5. **Doctor diagnostic**: `ddx doctor` surfaces each of the four
   failure modes (unreachable, missing collection, missing schema,
   schema version mismatch) with the expected message.
6. **Offline posture (D4)**: with Axon stopped, every `ddx bead *`
   subcommand and `ddx work` exit non-zero with the
   `axon unreachable at <addr>` message; no local writes occur.

## Backend interface (Go)

The current `cli/internal/bead/backend.go` interface is too coarse for
Axon — `WriteAll(beads []Bead)` cannot reasonably be implemented
against a remote entity store, and `WithLock` assumes a process-local
mutex around an in-memory rewrite. This TD replaces the existing
interface with a per-entity surface that both backends can satisfy.
Existing JSONL semantics are preserved by a thin adapter that wraps
the existing `ReadAll`/`WriteAll`/`flock` pair.

```go
// cli/internal/bead/backend.go
package bead

import "context"

// Backend is the storage contract for the bead tracker. Both the
// JSONL backend (cli/internal/bead/backend_jsonl.go) and the axon
// backend (cli/internal/bead/axon/) implement this surface. The
// chaos tests under cli/internal/bead/ run against any registered
// Backend via newTestStore(t, BackendName).
type Backend interface {
    // Init prepares storage (create JSONL file + lock dir; or for
    // axon: ensure collections exist and registered schema is current).
    Init(ctx context.Context) error

    // Get returns one bead with its OCC version. ErrNotFound for missing.
    Get(ctx context.Context, id string) (Bead, Version, error)

    // List returns all beads in the tracker collection (open + in_progress
    // + closed not yet archived). Order is unspecified.
    List(ctx context.Context) ([]Bead, error)

    // Create persists a new bead. Bead.ID may be empty; backend assigns
    // a ddx-<8hex> id and returns the assigned id. Fails if id is set
    // and already exists.
    Create(ctx context.Context, b Bead) (string, Version, error)

    // Update applies mutate to the bead identified by id under
    // optimistic concurrency control: read at version V, apply mutate,
    // write iff stored version == V. ErrVersionMismatch on conflict;
    // caller retries. Used by claim, status transitions, and any
    // field edit. Backend MUST serialize concurrent Update calls on
    // the same id (jsonl: via flock; axon: via expected_version).
    Update(ctx context.Context, id string, mutate func(*Bead) error) error

    // AppendEvent records an immutable audit event linked to a bead.
    // For jsonl: appends to the inline events array under the same
    // lock as the bead row. For axon: CreateEntity in
    // ddx_bead_events + CreateLink(event_of) in one CommitTransaction.
    AppendEvent(ctx context.Context, beadID string, ev Event) error

    // AddDep / RemoveDep maintain the depends_on graph. For jsonl:
    // mutate Dependencies on the bead row. For axon: CreateLink /
    // DeleteLink with type=depends_on.
    AddDep(ctx context.Context, fromID, toID string) error
    RemoveDep(ctx context.Context, fromID, toID string) error

    // Ready returns beads with Status==open whose every dependency
    // is Status==closed. Implementations are free to compute this
    // server-side; the JSONL implementation does it client-side.
    Ready(ctx context.Context) ([]Bead, error)

    // Close is sugar for Update(id, b -> b.Status = closed) plus
    // AppendEvent(closed). Provided so the backend can fuse both
    // writes into a single transaction (axon) or single locked
    // rewrite (jsonl).
    Close(ctx context.Context, id string) error

    // Export streams beads in bd/br JSONL interchange shape (events
    // materialised inline). Round-trip with Import is verified by
    // schema_compat_test.go.
    Export(ctx context.Context, w BeadWriter) error
    Import(ctx context.Context, r BeadReader) error
}

// Version is an opaque OCC token (jsonl: monotonic int from the
// in-memory snapshot; axon: EntityProto.version).
type Version uint64

var (
    ErrNotFound        = errors.New("bead: not found")
    ErrVersionMismatch = errors.New("bead: version mismatch (concurrent update)")
    ErrBackendOffline  = errors.New("bead: backend unreachable")
)
```

The `BackendType` constants in `backend.go` extend with
`BackendAxon = "axon"`. The factory in `cli/internal/bead/registry.go`
selects the implementation from `bead_tracker.backend` in
`.ddx/config.yaml`. The existing `JSONLBackend` becomes an internal
adapter behind this interface; call sites in `cli/internal/bead/` and
`cli/cmd/bead_*.go` migrate to the new methods (mechanical change —
the operations already exist, just on `Store` rather than `Backend`).

## Chaos contract the new backend must satisfy

The contract under `cli/internal/bead/chaos_test.go` is the
parameterised conformance suite for any `Backend` implementation. The
tests already operate on a `Store` value via `newTestStore(t)`; this TD
parameterises that helper by backend selector
(`newTestStore(t, BackendJSONL)` / `newTestStore(t, BackendAxon)`) and
both runs MUST pass identically. The following invariants are the
contract surface:

| Test | Invariant the backend must guarantee |
|---|---|
| `TestChaos_ConcurrentAppendSafety` | 100 concurrent `Create` calls from 10 goroutines all succeed and all 100 beads are present afterwards with unique ids. No duplicate ids, no lost writes, no torn rows. |
| `TestChaos_AtomicStatusTransitions` | 5 goroutines × 20 cycles of `Update(status)` on the same bead leave the bead with a valid status (`open` / `in_progress` / `closed`) and the store fully readable. Transient `ErrVersionMismatch` is acceptable (caller's retry concern); corruption is not. |
| `TestChaos_ConcurrentCloseAndAppend` | A `Close(idA)` racing a `Create(B)` never loses either op: B exists with the right title; A's row remains parseable. Reproduces the original JSONL bug; required of every backend. |
| `TestChaos_ConcurrentCloseNotLost` | After a `Close(idA)` racing a `Create(B)` completes, `Get(idA).Status == closed` deterministically. The backend MUST serialize the read-modify-write of A (jsonl: flock; axon: OCC + retry on mismatch). The "close is never lost" invariant is the strict regression test for the JSONL bug and the acceptance criterion for axon. |
| `TestChaos_JSONLRoundTripIntegrity` | Renamed to `TestChaos_RoundTripIntegrity` once parameterised. Beads with unicode, emoji, embedded JSON-injection bytes, tabs, multi-line descriptions, and 200/1000/500-char field maxima survive `Create → ReadAll` field-for-field (Title, Description, Notes, Priority, Status, Labels). Validates that the wire format and schema preserve all `Bead` fields without truncation or escape-sequence loss. |
| `TestChaos_*` (all of the above) | No partial writes are visible to a concurrent reader. For jsonl this is the `writeAtomicFile` + `flock` guarantee; for axon this is the `CommitTransaction` ACID guarantee. |

Companion suites that the axon backend must also pass once
parameterised: `chaos2_test.go`, `property_test.go`, `fuzz_test.go`,
`merge_test.go`, and `schema_compat_test.go` (the bd/br JSONL
round-trip). `schema_compat_test.go` is the strictest
interoperability check — `Export` then `Import` then `Export` must
produce byte-equal output regardless of backend.

## Archive policy under axon

ADR-004 specifies that closed-bead events are externalised to
`.ddx/attachments/` and that eligible closed beads move to
`.ddx/beads-archive.jsonl` to keep the active tracker file small. Under
the axon backend the externalisation problem disappears: events are
already separate entities in `ddx_bead_events` (per D3), so the active
`ddx_beads` row never carries the events array on the wire. The
"archive" distinction collapses to a soft index: `ddx bead migrate`
under the axon backend is a no-op for archive-split purposes; closed
beads remain in the same `ddx_beads` collection, distinguished only by
`status=closed`. Queries that historically scanned only the active
file (`bead list`, `bead ready`) become `QueryEntities` calls with a
`status` filter and pay nothing for closed-bead volume. The
attachments mechanism remains the canonical location for non-event
artifacts (large logs, screenshots) referenced from event payloads;
nothing in this TD changes the attachments contract.

| Current pain (JSONL backend) | Axon backend fix |
|---|---|
| Multi-worker `.git/index.lock` contention (bead `ddx-da11a34a`) | No JSONL writes, no git tracker commits — gone. |
| `beads.jsonl > 5 MB` lefthook complications | No JSONL on disk; growth is an Axon-side concern. |
| ADR-004 archive split + attachment store complexity | Subsumed by the two-collection model + per-event entities. |
| Multi-machine sync (today is git-based per FEAT-023) | Axon server is the source of truth; workers are clients. Federation falls out. |
| Implicit schema enforcement | Axon validates per-entity against the registered JSON Schema. |
| Audit trail is event-array-on-bead | Events are first-class entities; Axon's per-entity immutable audit (`QueryAuditByEntity`) augments. |
| OCC is flock-only | OCC via `expected_version` end-to-end. |

## Out of scope

- The git tracker-commit lock issue (`ddx-da11a34a`) becomes moot under
  the Axon backend, but the lock-fix bead remains relevant for the
  JSONL backend's lifetime as the fallback path.
- Axon's pre-release status: wire protocol, data model, and SDK surface
  are not frozen. DDx pins a specific Axon version and tracks the
  CHANGELOG.
- Axon's Go SDK does not exist; DDx generates its own from `axon.proto`.

## Open questions for the Axon team

These are filed as cross-repo coordination items; each can land as an
issue against the Axon repo. They do not block this TD from shipping —
the v1 policy defaults are sufficient until benchmarks or operator
reports surface a concrete need.

1. **Server-side aggregation for the ready-queue traversal pattern.**
   Is there (or could there be) an Axon RPC or computed-view that
   answers "all entities in collection C with status=S whose outgoing
   links of type T all point to entities with status ∈ {S'}" in a
   single round-trip? The v1 policy uses two-phase client-side
   traversal; a server-side answer would unlock larger archives.
2. **ts-net alignment with ADR-006.** Does Axon's auth/transport story
   support a Tailscale tsnet listener directly, or does DDx need to
   wrap the gRPC dial through its own tsnet socket? Either is workable;
   the answer determines whether ADR-006's tsnet integration extends
   transparently or requires adapter code on the DDx side.

## References

- Background plan: `/tmp/ddx-axon-backend-plan.md`
- ADR-004: Bead-Backed Runtime Storage
- ADR-006: Tailscale tsnet Authentication
- SD-004: Beads Tracker
- FEAT-004: Beads
- TD-027: Bead-Backed Collection Abstraction (defines the
  `bead_tracker.backend` switch)
- ADR-007: Federation Topology
- ADR-021: Operator-Prompt Beads Web Write Path (multi-node story note)
- FEAT-026 / Story 14: multi-machine federation prerequisite
- Lock-contention v1 patch: bead `ddx-da11a34a` (`cli/internal/bead/lock.go`)
- Backend interface today: `cli/internal/bead/backend.go`,
  `cli/internal/bead/backend_jsonl.go`
- Conformance suite: `cli/internal/bead/chaos_test.go`,
  `cli/internal/bead/schema_compat_test.go`
- Axon `axon.proto`: `crates/axon-server/proto/axon.proto`
- Axon `bead.rs` (informational, not consumed):
  `crates/axon-api/src/bead.rs`
