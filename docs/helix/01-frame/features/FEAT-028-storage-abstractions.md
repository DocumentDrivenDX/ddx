---
ddx:
  id: FEAT-028
  depends_on:
    - helix.prd
    - FEAT-002
    - FEAT-004
    - FEAT-005
    - FEAT-006
    - FEAT-009
---
# Feature: Storage Abstractions

**ID:** FEAT-028
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx persists state through direct filesystem writes scattered across many packages: `cli/internal/bead/`, `cli/internal/agent/`, `cli/internal/registry/`, `cli/internal/server/`, and others all call `os.WriteFile`, `os.MkdirAll`, and `filepath.Join` against `.ddx/<subtree>/...` paths chosen at the call site. The bead store is the only persistence surface that already sits behind a swappable backend (`bead.RawBackend` / `bead.Backend`); every other persistent state is open-coded.

This blocks two things:

1. Deploying `ddx-server` against non-local-FS storage (e.g. Unity Catalog Volumes for blobs, Lakebase Postgres for entity rows) — every direct FS call site is a port that must be rewritten.
2. Reasoning clearly about what state is durable, what is ephemeral, what is server-owned, and what is process-local — today the answer is "look at every package."

FEAT-028 defines **five storage abstractions** that cover everything ddx persists, names the interface for each, and identifies which existing call sites move behind each abstraction. The first concrete deliverable is the `BlobStore` interface and migration of execution evidence and externalized bead attachments behind it. Subsequent abstractions ship in follow-up beads, scoped one at a time so each migration is reviewable.

## Problem Statement

**Current situation:** Ten or more places under `.ddx/` are written to by direct filesystem calls scattered across packages. Migrating any of them to a non-FS backend requires touching every call site individually. There is no shared error model, no shared blob naming convention, no shared lifecycle (creation, listing, garbage collection, GC), and no test seam for backend swapping outside `bead/`.

**Pain points:**

- `BackendAxon` is the natural production-default for entity state in a multi-tenant server deployment, but the surrounding state (runs, plugin-dispatches, workers, evidence blobs, agent session logs) cannot move with it because no abstraction exists.
- Adding a Databricks-App deployment of `ddx-server` requires UC Volumes for blob storage and Lakebase Postgres (via Axon) for entity state. Both are dead-ends without abstractions to slot them into.
- The `.ddx/` directory layout is ddx's de-facto storage schema; it is undocumented and changes by accretion. New persistent state lands in new subdirectories with no contract.
- Tests that need to manipulate stored state mock filesystem calls or use `t.TempDir()` plus `testutils.NewFixtureRepo`, instead of swapping in a memory-backed implementation.

**Desired outcome:** Every persistent state surface in ddx is reachable through one of five named interfaces. Each interface has at least one local-FS implementation (the current behavior, refactored to live behind the interface) and is structured so additional implementations (UC Volumes, Lakebase, S3, in-memory for tests) slot in without touching call sites. The local-FS implementations remain the default for `ddx` CLI and for single-machine `ddx-server`; alternate implementations are configured at server bootstrap.

## The Five Abstractions

| # | Interface | Owns | Today's call sites | First non-FS backend |
|---|-----------|------|--------------------|----------------------|
| 1 | **EntityStore** | beads, bead events, run/attempt records, plugin-dispatches | `cli/internal/bead/` (already abstracted as `Backend`); `cli/internal/agent/` runs+dispatches | Axon-on-Lakebase |
| 2 | **BlobStore** | execution evidence (per-attempt files), externalized bead attachments, library packages, large agent outputs | `cli/internal/bead/attachments.go`, `.ddx/executions/<run-id>/` writers in `cli/internal/agent/execute_bead*.go`, `cli/internal/registry/`, `cli/internal/evidence/` | UC Volumes |
| 3 | **StreamStore** | agent session logs, metrics streams, server logs, **append-only mirror writers** (`executions_mirror.go`, `routing_metrics.go`) | `cli/internal/agentmetrics/`, `cli/internal/processmetrics/`, `cli/internal/attemptmetrics/`, `cli/internal/agent/executions_mirror.go`, `.ddx/agent-logs/` writers | Databricks Jobs / Volumes append targets |
| 4 | **ConfigStore** | project config, user config, persona bindings, harness routing | `cli/internal/config/` | Workspace settings (per-tenant) |
| 5 | **(no interface — local FS / process-local only)** | worker working trees, execution sandboxes, in-flight git worktrees, **worker disk projections** (`.ddx/workers/<id>/{spec,status}.json`, `worker.log`, `worker-events.jsonl`) | `cli/internal/agent/execute_bead*`, worker probe/reporting paths | n/a — authoritative worker execution state lives in the autonomous worker process and bead store; on-disk projections are diagnostic readback only |

**On row 5 (workers):** ADR-022 makes workers autonomous. Authoritative worker
execution state lives in the worker process plus the bead store; the server and
hub consume derived reports. The `.ddx/workers/<id>/` directory is a
**diagnostic projection** for other clients to read, not a portable
cross-node authority record. Worker disk projections therefore stay local-FS:
they are ephemeral with the process and meaningful only as local diagnostics or
as source material for best-effort reporting.

**On row 5 (execution out-of-process):** managed nodes (ADR-028 / FEAT-029) do
not make worktrees portable. A hub command may ask a managed node to start or
stop local work, but execution still happens on that node with local worktrees.
Any future Databricks-App or remote-execution deployment must specify how
worktrees and sandboxes are created on the execution host. FEAT-028 does not
introduce or fix that deployment model; it only declines to abstract worktrees
as general durable storage.

## Scope

### In Scope (v1)

- **Define `BlobStore` interface** in a new `cli/internal/blob/` package. Methods, error semantics, naming/key conventions, content-addressed vs. caller-keyed semantics — all decided in this spec.
- **Implement `LocalFSBlob`** as the default backend, mirroring current on-disk layouts under `.ddx/attachments/`, `.ddx/executions/`, `.ddx/plugins/`. Behavior-equivalent to today's direct writes.
- **Migrate two call-site clusters behind `BlobStore`:**
  - Externalized bead attachments (`cli/internal/bead/attachments.go`) — write-once sidecars, fits BlobStore.
  - Per-attempt execution evidence files written under `.ddx/executions/<run-id>/` by `cli/internal/agent/execute_bead*.go` (e.g. `manifest.json`, `result.json`, `prompt.md`, captured agent output) — write-once-per-attempt, fits BlobStore. **`cli/internal/agent/executions_mirror.go` is explicitly NOT in v1**: its primary writes (`mirror.log`, `mirror-index.jsonl`) are append-only and belong to StreamStore (deferred). It stays as direct FS calls until StreamStore lands.
- **Add a `MemoryBlob` implementation** for tests, replacing the per-test `t.TempDir` blob write/read fixtures where they exist.
- **Document the five abstractions** in this spec as the canonical taxonomy. Subsequent state-storage features reference this spec rather than inventing their own categorization.

### Out of Scope (deferred to follow-up FEAT updates or beads)

- `StreamStore`, `ConfigStore` interfaces — defined here as placeholders, full design deferred until a concrete migration target needs them.
- Migrating `cli/internal/registry/` (library package install) behind `BlobStore` — registry has its own manifest-plus-content shape that needs separate consideration.
- Migrating `cli/internal/agentmetrics/` behind `StreamStore` — deferred to the StreamStore feature.
- Promoting `BackendAxon` to production-default for entity state — covered by a FEAT-004 update, not here.
- New entity collections in Axon for runs, plugin-dispatches, workers — covered by FEAT-004 and FEAT-006 updates once Axon-as-default lands.
- Any UC Volumes / Lakebase implementation — gated on confirming the Databricks-App deployment direction. This spec only ensures the abstractions are in place to host them later.
- Garbage collection or quota enforcement on `BlobStore` — out of scope for v1.

### Non-Goals

- This spec does not change runtime behavior for any existing user. Local-FS layouts and lifetimes are preserved bit-for-bit by `LocalFSBlob`.
- This spec does not introduce new commands or surface area on the CLI.

### Explicit deferred-keys list

`BlobStore` is **not** the storage path for the following keys/paths in v1. They stay as direct FS calls, are written by the existing packages, and migrate later (each behind its own bead, when its call site warrants it):

- `attachments/` and `executions/<run-id>/<file>` (per-attempt evidence files) — **migrating in v1** (above).
- `agent-logs/` (`*.jsonl` per session) — StreamStore-shaped, deferred.
- `executions/mirror-index.jsonl`, `executions/<run-id>/mirror.log` — StreamStore-shaped, deferred.
- `metrics/`, `processmetrics/`, `attemptmetrics/` writers — StreamStore-shaped, deferred.
- `plugins/`, `skills/`, `library/` (registry-installed content) — needs registry-aware design, deferred.
- `workers/<id>/` — diagnostic projection, no abstraction (row 5 above).
- `server/state.json`, `server/tls/`, `lifecycle-schema.json` — server-internal, no abstraction.
- `backups/`, `beads.backup.jsonl` — bead-store maintenance artifacts, follow EntityStore not BlobStore.
- `axon/` (Axon snapshot files when JSONL fallback is in effect) — Axon-internal, follows EntityStore.
- `run-state/`, `run-state.json` — coordination state, EntityStore-shaped, deferred.
- `plugin-dispatches/` — EntityStore-shaped, deferred.

If a v1 migration appears to require touching any path on this list, that is a sign the migration scope has crept and needs to stop, not that the deferral list is wrong.

## BlobStore Interface (v1)

The interface is intentionally small. Anything not justified by an existing call site is out.

```go
// Package blob defines the BlobStore abstraction for byte-blob storage
// (execution evidence, bead attachments, future library content).
package blob

import (
    "context"
    "io"
)

// Key is a hierarchical, slash-separated identifier for a blob.
// Keys are caller-supplied (not content-addressed) and stable across
// reads and writes. Examples:
//   "attachments/ddx-a827d146/events.jsonl"
//   "executions/20260511T030206-bca671a4/result.json"
type Key string

// Store is the abstraction every persistent byte-blob in ddx flows through.
// Implementations must be safe for concurrent use.
type Store interface {
    // Put writes the entire blob at key, overwriting any previous value.
    // Put MUST be both atomic-on-success and durable-on-return:
    //   - Atomic: a concurrent Get either sees the full prior value or
    //     the full new value, never a partial write.
    //   - Durable: when Put returns nil, the blob has survived a process
    //     or host crash. For LocalFSBlob this means fsync of the data
    //     file AND fsync of its parent directory before returning.
    //
    // Crash-safety in callers (e.g. bead attachment externalization in
    // attachments.go) depends on this guarantee — a Put that returns
    // before durability is established can permanently lose data when
    // the caller proceeds to clear the in-memory copy.
    Put(ctx context.Context, key Key, r io.Reader) error

    // Get returns a reader for the blob at key. The caller must Close.
    // Returns ErrNotFound (errors.Is) when the key does not exist.
    Get(ctx context.Context, key Key) (io.ReadCloser, error)

    // Stat returns metadata for the blob at key without fetching its body.
    // Returns ErrNotFound when the key does not exist.
    Stat(ctx context.Context, key Key) (Info, error)

    // List enumerates keys with the given prefix. Order is unspecified.
    // Implementations should stream rather than buffer for large prefixes.
    List(ctx context.Context, prefix Key) ([]Key, error)

    // Delete removes the blob at key. Deleting a missing key is not an error.
    Delete(ctx context.Context, key Key) error
}

// Info is the metadata Stat returns. Size and ModTime are required; ETag
// is optional and may be empty for backends that don't surface one.
type Info struct {
    Key     Key
    Size    int64
    ModTime time.Time
    ETag    string
}

// ErrNotFound is returned (or wrapped) by Get and Stat when a key does not exist.
var ErrNotFound = errors.New("blob: not found")
```

### Design choices the interface bakes in

1. **Caller-supplied keys, not content addressing.** Today's `.ddx/` paths are caller-keyed (`attachments/<bead-id>/events.jsonl`, `executions/<run-id>/result.json`). Content addressing would force every existing call site to track an out-of-band manifest. Defer it.
2. **Whole-blob `Put` and reader-returning `Get`, no append, no random access.** Externalized attachments are written once. Execution evidence files are written once per attempt. Where append semantics are actually needed (agent session logs, metrics) the call site belongs in `StreamStore`, not here.
3. **`io.Reader`/`io.ReadCloser`, not `[]byte`.** Avoid forcing all blobs through memory; UC Volumes and S3 implementations want streaming.
4. **`context.Context` on every method.** Required for cancellation when backends are network-bound. Local-FS implementation ignores it.
5. **No transactional grouping of operations.** Deferred. If future work needs "either both blobs are written or neither," wrap at a higher layer. See "Multi-blob write discipline" below for the pattern callers MUST follow in the meantime.
6. **No encryption / compression / checksumming inside the interface.** Backends may implement these internally; not part of the contract.
7. **Permissions are not in the interface.** `LocalFSBlob` writes files at `0o644` and creates directories at `0o755`, matching today's default behavior under `.ddx/` for these collections. Server-internal state (`server/state.json` at `0o600`, TLS material) is **not** a `BlobStore` collection — it stays as direct FS calls in row 5. If a future BlobStore caller needs different file modes per collection, the right fix is per-collection backend configuration, not a `mode` parameter on `Put` (which would leak FS-isms into the interface).

### Multi-blob write discipline (normative for callers)

`BlobStore` provides no transactional grouping. Callers that write multiple blobs as part of one logical operation (e.g. a single attempt writing `prompt.md`, `result.json`, captured agent output, and a `manifest.json`) MUST follow this discipline:

1. **Manifest written last.** The blob that names or references the others (`manifest.json`, or whatever the EntityStore row points at) is written **after** all referenced blobs have returned successful `Put`. This guarantees that any reader that sees the manifest will see all referenced blobs.
2. **Foreign key stability.** When an EntityStore row references a `BlobStore` key (or key prefix), the row uses the same stable identifier the blob keys are derived from (e.g. `run-id`). The EntityStore row is also written last, after all blobs and after the manifest.
3. **Orphan blobs are acknowledged.** A crash between blob writes and manifest write leaves orphan blobs at the prefix. On `LocalFSBlob` this is invisible (the prefix is on local disk, not billed). On a remote backend (UC Volumes, S3) orphans accumulate as billable garbage. **A garbage-collection sweep that lists blob prefixes without a corresponding EntityStore row is required for non-FS backends and is explicitly out of scope for v1** — it must be designed when the first non-FS backend lands. v1 ships with the orphan problem present-but-deferred.

### Key naming conventions (v1, normative)

- Keys are slash-separated, ASCII, no leading slash, no trailing slash, no `..` segments.
- The first segment is the **collection**: `attachments`, `executions`, future collections live alongside.
- Within a collection, the second segment is the **owner ID** (bead ID, run ID).
- Subsequent segments are **resource paths** within the owner.
- Implementations may map keys to different on-disk or remote layouts; callers must not assume key=path.

## Acceptance Criteria

1. `cli/internal/blob/` package exists, defining `blob.Store`, `blob.Key`, `blob.Info`, `blob.ErrNotFound` matching the signatures above.
2. `cli/internal/blob/localfs.go` implements `blob.Store` against a configurable root directory, defaulting to `.ddx/` resolved from the project root via the existing `config.NewConfig` lookup.
3. `cli/internal/blob/memory.go` implements `blob.Store` as an in-memory map for tests.
4. A conformance test suite (`cli/internal/blob/conformance_test.go`) validates that any `blob.Store` implementation passes the same behavioral tests (Put-then-Get round-trip, Stat metadata accuracy, List prefix enumeration, Delete idempotency, ErrNotFound on missing keys, concurrent Put safety). Both `LocalFSBlob` and `MemoryBlob` pass it. Test functions are named `TestBlobStoreConformance_*`.
5. `cli/internal/bead/attachments.go` writes externalized bead-event attachments through a `blob.Store` injected at construction. The on-disk layout under `.ddx/attachments/<bead-id>/` is preserved exactly when `LocalFSBlob` is the configured backend (verified by a test that asserts file paths match the pre-migration layout). The pre-existing crash-safety property (sidecar durably written before inline events are cleared from the bead record) is preserved by the durability guarantee in `BlobStore.Put`.
6. The per-attempt evidence writers in `cli/internal/agent/execute_bead*.go` (writing `.ddx/executions/<run-id>/<file>` for `manifest.json`, `result.json`, `prompt.md`, captured agent output) write through a `blob.Store`. On-disk layout preserved when `LocalFSBlob` is configured. **`cli/internal/agent/executions_mirror.go` is NOT migrated in v1** — its `mirror.log` and `mirror-index.jsonl` writers are append-only and stay as direct FS calls until StreamStore lands.
7. The multi-blob write discipline (manifest written last) is enforced in the migrated evidence writers — verified by a test that injects a failing `Put` for a non-manifest blob and asserts the manifest is never written.
8. No new top-level `.ddx/` subdirectory is introduced by this work. No existing layout changes for migrated paths.
9. `cd cli && go test ./...` is green.
10. `lefthook run pre-commit` passes.

## Sequencing

These ship as separate beads, in this order:

1. **Bead 1 (this FEAT seeds it)**: define `cli/internal/blob/` package — interface, `LocalFSBlob`, `MemoryBlob`, conformance test suite. No call-site migration yet. Pure additive.
2. **Bead 2**: migrate `cli/internal/bead/attachments.go` behind `blob.Store`. Verify on-disk layout unchanged.
3. **Bead 3**: migrate execution-evidence writers in `cli/internal/agent/` behind `blob.Store`. Verify on-disk layout unchanged.
4. **Bead 4** (optional, deferrable): retrofit existing tests that currently use `t.TempDir()` for blob round-trips to use `MemoryBlob` instead — grep `t.TempDir()` callers in the migrated packages.

After all four ship, the `BlobStore` abstraction is in place and the next storage-abstraction work (`StreamStore`, registry-on-BlobStore, Axon-as-entity-default) can be picked up independently.

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Migration changes on-disk layout subtly, breaking external tools that read `.ddx/` directly | Medium | High | Bead AC explicitly require pre/post layout assertion; `LocalFSBlob` is byte-for-byte equivalent |
| Conformance suite under-specifies behavior, two backends diverge silently | Low | Medium | Suite covers the contract methods + concurrency + durability; expand as new backends surface gaps |
| Streaming semantics for large blobs fail under `LocalFSBlob` due to fsync timing | Low | Low | Local-FS Put writes to a temp file, fsyncs file + parent dir, renames atomically; standard pattern |
| Scope creep into StreamStore or ConfigStore design before BlobStore proves out | Medium | Medium | This spec is explicit about deferral; reviewers reject scope additions |
| Axon-as-default and BlobStore work entangle | Low | Medium | Sequenced separately: BlobStore lands first, Axon-as-default is its own FEAT-004 update |
| Axon-on-Lakebase claimed as production-default but not actually viable | Medium | High | `BackendAxon` today is JSONL + optional GraphQL passthrough (`cli/internal/bead/axon_backend.go:37-39,541`), with no JSONL→Postgres migration scripts and no schema-version loader. Production-default requires both, plus identity-passthrough validation against Lakebase. Tracked as a prerequisite in the FEAT-004 update; FEAT-028 does not assume any of this is done. |
| Orphan blob accumulation on non-FS backends after partial-write crashes | Medium | Medium | v1 ships with the orphan problem present-but-deferred (see "Multi-blob write discipline"); GC sweep is required before the first non-FS backend ships. Documented in the deferred-keys list and called out in scope. |
| Caller of multi-blob write violates "manifest written last" discipline | Medium | High | Discipline is normative in spec text and verified by an AC #7 test that injects `Put` failures and asserts manifest is not written. Reviewers must check new BlobStore callers for compliance. |
