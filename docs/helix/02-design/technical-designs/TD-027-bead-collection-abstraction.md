---
ddx:
  id: TD-027
  depends_on:
    - ADR-004
    - SD-004
    - FEAT-004
---
# Technical Design: Bead-Backed Collection Abstraction

## Purpose

This design picks the concrete implementation choices needed to land
ADR-004 ("Use Bead-Backed Collections for DDx Runtime Storage") on top
of the existing single-file `.ddx/beads.jsonl` tracker described in
SD-004 and required by FEAT-004.

ADR-004 commits DDx to a model of named bead-backed collections with
sidecar attachments. SD-004 describes the existing single-file store and
its locking, repair, and append-only evidence semantics. FEAT-004 sets
the user-visible behavior of the active tracker. This TD specifies:

(a) the collection registry shape,
(b) the archival trigger policy and parameters,
(c) the attachment storage layout,
(d) the migration semantics for the existing 5.4 MB `beads.jsonl`,
(e) the read-path semantics across active plus archive, and
(f) the bd/br interchange compatibility for the archive collection.

The decisions below are the defaults DDx will ship. Anything outside
this list — for example new collections beyond `beads`, `beads-archive`,
`exec-runs`, and `agent-sessions` — is out of scope for this TD and
should be handled in a follow-up TD that depends on this one.

## (a) Collection Registry

### Registry Shape

A collection is a named logical store backed by one of the existing
bead backends. The registry is an in-process Go map seeded at startup
from a small declarative table. It is not a user-editable file; users
do not name new collections, and DDx does not auto-discover collections
from disk.

```go
type CollectionID string

type CollectionSpec struct {
    ID              CollectionID // "beads", "beads-archive", ...
    DefaultBackend  BackendKind  // jsonl | bd | br
    JSONLPath       string       // path under .ddx/ for the jsonl backend
    QueueSemantics  bool         // true for "beads" only
    ArchivePartner  CollectionID // "beads-archive" for "beads", "" otherwise
    Attachments     bool         // true for collections that allow sidecars
}
```

Concrete shipping registry:

| ID               | JSONL path                       | QueueSemantics | ArchivePartner | Attachments |
|------------------|----------------------------------|----------------|----------------|-------------|
| `beads`          | `.ddx/beads.jsonl`               | yes            | `beads-archive`| no          |
| `beads-archive`  | `.ddx/beads-archive.jsonl`       | no             | (none)         | no          |
| `exec-runs`      | `.ddx/exec-runs.jsonl`           | no             | (none)         | yes         |
| `agent-sessions` | `.ddx/agent-sessions.jsonl`      | no             | (none)         | yes         |

### Store API

The current `beads.Store` API is generalized so the file path is no
longer hardcoded. The minimum surface is:

```go
type Store interface {
    Collection(id CollectionID) Collection
}

type Collection interface {
    List(ctx context.Context) ([]Bead, error)
    Get(ctx context.Context, id string) (Bead, error)
    Append(ctx context.Context, b Bead) error
    Update(ctx context.Context, b Bead) error
    Delete(ctx context.Context, id string) error
    Lock(ctx context.Context) (Unlocker, error)
}
```

`Lock` is per-collection so that archiving from `beads` to
`beads-archive` can hold both locks in a fixed order (`beads` first,
then `beads-archive`) to avoid deadlocks.

The existing locking, atomic temp-file rename, malformed-record repair,
and `Extra` preservation behaviors from SD-004 carry over unchanged on
a per-collection basis. Each JSONL collection has its own `.lock`,
`.tmp`, and `.bak` files alongside its primary file.

### Backend Selection

`bd` and `br` backends already accept a collection name through their
own CLIs. The DDx external-backend adapter passes the collection ID
through to those CLIs. For backends that do not support multiple named
collections, the adapter returns a clear error when a non-default
collection is requested rather than silently merging records.

## (b) Archival Trigger Policy

### Goal

Keep the active queue small enough that `ddx bead list` and the SD-004
read path stay under the 100 ms / 10,000 bead target stated in SD-004,
without forcing operators to think about archival.

### Trigger

Archival runs on demand through `ddx bead archive` and opportunistically
after any close-causing mutation when the active collection exceeds a
threshold. There is no background daemon.

### Defaults

| Parameter                      | Default                                |
|--------------------------------|----------------------------------------|
| `archive.enabled`              | `true`                                 |
| `archive.min_age`              | `30d` since `closed_at`                |
| `archive.min_active_count`     | `2000` records in `beads`              |
| `archive.batch_size`           | `500` per opportunistic pass           |
| `archive.statuses`             | `closed`, `wont_fix`, `superseded`     |
| `archive.opportunistic`        | `true` (run after close mutations)     |
| `archive.preserve_dependencies`| `true` (keep edges resolvable)         |

A bead is eligible to archive when its `status` is in
`archive.statuses`, its `closed_at` (or `updated_at` fallback) is older
than `archive.min_age`, and no open bead in `beads` lists it as a
dependency. The dependency check prevents archiving a closed bead that
an open bead still references; archived beads remain readable, but
queue derivation in SD-004 should not have to load `beads-archive` to
decide `ready`/`blocked`.

### Configuration

Defaults live in `config.go` and may be overridden through `.ddx.yml`:

```yaml
archive:
  enabled: true
  min_age: 30d
  min_active_count: 2000
  batch_size: 500
  statuses: [closed, wont_fix, superseded]
  opportunistic: true
```

### Mutation Sequence

1. Acquire `beads` lock.
2. Acquire `beads-archive` lock.
3. Read both snapshots.
4. Select up to `batch_size` eligible records from `beads`.
5. Append the selected records to `beads-archive` with an added
   `archived_at` timestamp in `Extra`.
6. Remove the selected records from `beads`.
7. Atomic temp-file rename for `beads-archive` first, then for `beads`.
   This ordering means a crash mid-archive leaves a record in both
   collections rather than nowhere, and the read path in (e) hides the
   duplicate behind active-wins precedence.
8. Release locks in reverse order.

## (c) Attachment Storage Layout

### Layout

Attachments are sidecar files under a per-collection directory keyed by
record ID:

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

Concrete defaults:

- The active `beads` collection does not use sidecar attachments.
  Evidence stays in `Extra["events"]` per SD-004, because that history
  is small and is referenced often by queue tooling.
- `exec-runs` and `agent-sessions` use sidecars by default for prompt,
  response, stdout, stderr, and structured result blobs.
- `beads-archive` does not introduce new attachments. Records moved
  from `beads` carry their existing `Extra["events"]` history inline.

### Reference Format

Attachment references are stored under a reserved
`Extra["attachments"]` array on the record:

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

`path` is repository-relative under `.ddx/attachments/`. `sha256` and
`size` are recorded at write time and verified on read. The reference
key `attachments` is DDx-specific and lives in preserved extras as
required by ADR-004; it does not rename or shadow any bd/br field.

### Write Algorithm

1. Write the attachment to a temp path under
   `.ddx/attachments/<collection>/<id>/.<name>.tmp`.
2. `fsync`, then rename to the final path.
3. Compute and record `sha256` and `size`.
4. Append the reference to `Extra["attachments"]`.
5. Persist the record through the normal collection write path so the
   reference becomes durable atomically with the temp-file rename of
   the collection JSONL file.

### Garbage Collection

Attachments are removed only when their owning record is removed. A
`ddx bead gc` command (out of scope for this TD beyond the contract)
walks `.ddx/attachments/<collection>/` and deletes directories whose
record IDs no longer exist in the collection. There is no time-based
attachment expiration.

## (d) Migration Plan for Existing `beads.jsonl`

The current `.ddx/beads.jsonl` is 5.4 MB and 1,172 lines. The
migration must be safe to run on an in-place repository checkout.

### Steps

1. **Compatibility shim first.** The new collection registry treats
   `.ddx/beads.jsonl` as the JSONL path for the `beads` collection. No
   file move is required to keep the active tracker working. Existing
   readers and writers continue to operate against the same path.

2. **Idempotent create-on-write for archive.** The
   `.ddx/beads-archive.jsonl` file is created lazily on the first
   archive operation. Absence of the file is treated as an empty
   archive collection.

3. **One-shot backfill command.** `ddx bead migrate-archive` performs
   the initial archival pass:
   - Acquire both locks.
   - Make a backup copy of `.ddx/beads.jsonl` to
     `.ddx/beads.jsonl.pre-archive.bak`.
   - Move all records that match the archival policy in (b) using the
     normal mutation sequence.
   - Print a summary of moved/retained counts.
   This command is opt-in. Users who never run it keep the current
   single-file behavior.

4. **No schema rewrite.** Existing records are not rewritten to add
   new fields. The `attachments` extra appears only on new
   `exec-runs` and `agent-sessions` records.

5. **Rollback.** `ddx bead migrate-archive --rollback` re-merges
   `beads-archive.jsonl` into `beads.jsonl` under both locks, atomic
   temp-file rename, and removes the archive file when empty.

6. **Doc audit.** The migration command is documented in the bead
   command reference. `ddx doc audit` should pass after this TD lands
   because the new TD declares its dependencies and no existing artifact
   needs to change its `depends_on`.

### Risk Mitigation

- The default `archive.min_active_count` of 2000 means existing
  installations under that count will not be archived opportunistically
  on close. They keep behaving like today's single-file tracker until
  they cross the threshold or run `ddx bead migrate-archive`.
- The pre-archive backup file is never removed automatically.

## (e) Read-Path Semantics Across Active and Archive

### Default View

Queue commands defined by SD-004 (`list`, `ready`, `blocked`, `status`)
read only the `beads` collection. They do not load `beads-archive`.
This preserves the SD-004 100 ms target as the active set stays bounded
and matches user expectation that "what's on the queue" is the active
queue.

### Merged View

`ddx bead show <id>`, `ddx bead history`, and any explicit
`--include-archive` flag use a merged view:

1. Look up the bead in `beads` first.
2. Fall back to `beads-archive` if not found.
3. For listing operations with `--include-archive`, lazily concatenate
   the two snapshots and de-duplicate by `id` with active-wins
   precedence (so a record present in both collections after an
   interrupted archive is reported as the active copy).

### Lazy Loading

The archive collection is opened only when a merged view is requested.
The store keeps per-collection snapshots cached for the lifetime of a
single command invocation; commands do not share snapshots across
invocations.

### Deletion Semantics

`ddx bead delete <id>` removes from the collection that currently holds
the record. If the record is in both collections (post-crash), the
delete removes it from both under both locks before returning success.

### Dependency Resolution

When the queue derivation in SD-004 needs to display a closed
dependency that has been archived, it reads only the dependency's
`status` from `beads-archive` on demand and caches it in memory for the
remainder of the command. It never promotes archived beads back into
the active snapshot.

## (f) bd/br Interchange Compatibility for the Archive Collection

### Constraints from ADR-004

ADR-004 requires that bd/br interchange continue to work through
`list --json` for reads and `import --from jsonl --replace -` for
writes, and that the bead envelope keeps its shared field names.

### Decision

The archive collection participates in interchange on the same terms
as the active collection.

- `ddx bead export --collection beads-archive` emits one bead-record
  JSON object per line, identical schema to active beads.
- `ddx bead import --collection beads-archive --from jsonl -` accepts
  the same shape.
- `archived_at` is stored in `Extra` and round-trips as an unknown
  field for bd/br, exactly like other DDx-specific fields. It is not
  promoted to a top-level field.
- The `attachments` extra also round-trips. bd/br do not interpret it,
  but they do not strip preserved extras, so references survive an
  export/import cycle.

### Adapter Behavior

For backends that support multiple named collections, the DDx adapter
passes the collection ID through. For backends that do not, the
adapter returns an error when the user asks for a non-default
collection rather than silently merging records into the default
collection. This is the same rule as (a).

### Schema Compatibility Test

`schema_compat_test.go` is extended with an "archive round-trip" case:
take a record from the active collection, archive it, export it,
re-import it into a fresh `beads-archive` collection, and verify
field-for-field equality including preserved extras.

## Validation

These checks must pass after this TD is implemented:

- `go test ./internal/bead/...`
- `go test ./cmd -run 'TestBeadCollection|TestArchive|TestAttachment' -count=1`
- `ddx doc audit` shows TD-027 with no broken edges.
- Manual: run `ddx bead migrate-archive` on a copy of an existing
  repository checkout and confirm queue commands produce the same
  active snapshot they did before migration.

## Non-Goals

- Adding collections beyond `beads`, `beads-archive`, `exec-runs`, and
  `agent-sessions`. Future collections require a follow-up TD.
- Real-time replication between collections.
- Cross-repository archive sharing.
- Background or scheduled archival; archival only runs on user-initiated
  commands or as an opportunistic step inside an already-mutating
  command.
