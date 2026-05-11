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
| 1 | **EntityStore** | beads, bead events, run/attempt records, plugin-dispatches, worker registrations | `cli/internal/bead/` (already abstracted as `Backend`); `cli/internal/agent/` runs+dispatches; `cli/internal/server/` workers | Axon-on-Lakebase |
| 2 | **BlobStore** | execution evidence, externalized bead attachments, library packages, large agent outputs | `cli/internal/bead/attachments.go`, `cli/internal/agent/executions_mirror.go`, `cli/internal/registry/`, `cli/internal/evidence/` | UC Volumes |
| 3 | **StreamStore** | agent session logs, metrics streams, server logs | `cli/internal/agentmetrics/`, `cli/internal/processmetrics/`, `cli/internal/attemptmetrics/`, `.ddx/agent-logs/` writers | Databricks Jobs / Volumes append targets |
| 4 | **ConfigStore** | project config, user config, persona bindings, harness routing | `cli/internal/config/` | Workspace settings (per-tenant) |
| 5 | **(no interface — keep ephemeral)** | worker working trees, execution sandboxes, in-flight git worktrees | `cli/internal/agent/execute_bead*`, server worker spawn | n/a — stays local FS, never persisted to server |

The fifth row is explicit: ephemeral process state does not get an abstraction. It stays where it is, on local disk, and the server-deployment story handles it by running execution out-of-process (workstation or Databricks Job) rather than inside the server.

## Scope

### In Scope (v1)

- **Define `BlobStore` interface** in a new `cli/internal/blob/` package. Methods, error semantics, naming/key conventions, content-addressed vs. caller-keyed semantics — all decided in this spec.
- **Implement `LocalFSBlob`** as the default backend, mirroring current on-disk layouts under `.ddx/attachments/`, `.ddx/executions/`, `.ddx/plugins/`. Behavior-equivalent to today's direct writes.
- **Migrate two call-site clusters behind `BlobStore`:**
  - Externalized bead attachments (`cli/internal/bead/attachments.go`).
  - Execution evidence (`cli/internal/agent/executions_mirror.go` and the `.ddx/executions/<run-id>/` write paths inside `agent_execute_bead.go`).
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
    // Implementations should make Put atomic-on-success — partial writes
    // must not be observable to a concurrent Get.
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
5. **No transactional grouping of operations.** Deferred. If future work needs "either both blobs are written or neither," wrap at a higher layer.
6. **No encryption / compression / checksumming inside the interface.** Backends may implement these internally; not part of the contract.

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
5. `cli/internal/bead/attachments.go` writes externalized bead-event attachments through a `blob.Store` injected at construction. The on-disk layout under `.ddx/attachments/<bead-id>/` is preserved exactly when `LocalFSBlob` is the configured backend (verified by a test that asserts file paths match the pre-migration layout).
6. `cli/internal/agent/executions_mirror.go` and the `.ddx/executions/<run-id>/` writers in `cli/internal/agent/execute_bead*.go` write through a `blob.Store`. On-disk layout preserved when `LocalFSBlob` is configured.
7. No new top-level `.ddx/` subdirectory is introduced by this work. No existing layout changes.
8. `cd cli && go test ./...` is green.
9. `lefthook run pre-commit` passes.

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
| Conformance suite under-specifies behavior, two backends diverge silently | Low | Medium | Suite covers the contract methods + concurrency; expand as new backends surface gaps |
| Streaming semantics for large blobs fail under `LocalFSBlob` due to fsync timing | Low | Low | Local-FS Put writes to a temp file and renames atomically; standard pattern |
| Scope creep into StreamStore or ConfigStore design before BlobStore proves out | Medium | Medium | This spec is explicit about deferral; reviewers reject scope additions |
| Axon-as-default and BlobStore work entangle | Low | Medium | Sequenced separately: BlobStore lands first, Axon-as-default is its own FEAT-004 update |
