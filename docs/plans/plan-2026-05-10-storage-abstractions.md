---
ddx:
  id: plan-2026-05-10-storage-abstractions
---
# Storage Abstractions: Taxonomy + BlobStore-First Sequencing

Date: 2026-05-10
Status: Spec drafted as FEAT-028 (`docs/helix/01-frame/features/FEAT-028-storage-abstractions.md`); post-review fixes applied; ready for bead breakdown pending user approval

## Why this exists

DDx persists state via `os.WriteFile` calls scattered across many packages. The bead store (`cli/internal/bead/Backend`) is the only persistence surface that's already abstracted. Every other write path is open-coded — attachments, executions, runs, workers, metrics, agent logs, plugin dispatches, etc.

This blocks two things:

1. Deploying `ddx-server` against non-local-FS storage (UC Volumes for blobs, Lakebase Postgres for entity rows) — every direct FS call site is a port that must be rewritten.
2. Reasoning clearly about what state is durable, what is ephemeral, what is server-owned, and what is process-local — today the answer is "look at every package."

## The five-abstraction taxonomy

| # | Abstraction | Owns | First non-FS backend |
|---|---|---|---|
| 1 | **EntityStore** | beads, bead events, run/attempt records, plugin-dispatches | Axon-on-Lakebase |
| 2 | **BlobStore** | execution evidence (per-attempt files), externalized bead attachments, library packages | UC Volumes |
| 3 | **StreamStore** | agent session logs, metrics streams, server logs, append-only mirror writers | Volumes append targets |
| 4 | **ConfigStore** | project config, user config, persona bindings, harness routing | Workspace settings |
| 5 | **(no abstraction)** | worker working trees, execution sandboxes, worker disk projections | n/a — local FS only; authoritative state lives in server-process memory |

Row 5 is explicit and load-bearing: server-process state stays local. Worker on-disk files (`.ddx/workers/<id>/{spec,status}.json`) are diagnostic projection — the server (`cli/internal/server/workers.go:191`) keeps authoritative state in `workerHandle` structs in memory and never reads disk back.

## Sequencing

FEAT-028 v1 ships **BlobStore only** with two call-site migrations:

1. Define `cli/internal/blob/` package — `Store` interface, `LocalFSBlob`, `MemoryBlob`, conformance test suite. Pure additive.
2. Migrate `cli/internal/bead/attachments.go` behind `blob.Store`. Verify on-disk layout unchanged.
3. Migrate per-attempt evidence writers in `cli/internal/agent/execute_bead*.go` behind `blob.Store`. Verify on-disk layout unchanged.
4. (Optional) retrofit existing `t.TempDir()` callers to use `MemoryBlob`.

Subsequent abstractions ship as separate FEAT updates / beads, in priority order driven by the **read-only deployment plan** (`plan-2026-05-10-read-only-deployment.md`):

- EntityStore on Axon for `beads` + `bead_events` (FEAT-004 update — gated on Axon production-readiness work).
- New EntityStore collection `attempts` covering execution outcomes + costs (high-value: unlocks all metrics commands).
- ConfigStore for project/user scopes.
- StreamStore for agent log + usage + routing-decisions readers.
- PackageRegistry abstraction for library content browse (largest deferred scope).

## Multi-model review (opus)

Five must-fix issues caught and applied to FEAT-028:

1. `executions_mirror.go` is append-only, doesn't fit BlobStore — pulled out of v1 (StreamStore-shaped).
2. Durability semantics added to `Put` contract (fsync file + parent dir before return) — load-bearing for `attachments.go` crash safety.
3. Multi-blob write discipline added: manifest-last ordering, foreign-key stability, orphan-blob problem acknowledged with deferred GC for non-FS backends.
4. Workers split out of EntityStore — moved to row 5; authoritative state is in-process memory in `workers.go:191`, on-disk is diagnostic projection only.
5. Honestly noted that "execution out-of-process" rationale is aspirational — today's server runs `ExecuteBead` in-process at `workers.go:748,783`; resolving this is a hard prerequisite for Databricks-App execution but is out of scope for FEAT-028.

Plus secondary fixes: Axon-on-Lakebase risk row added; permissions decision documented (LocalFS = `0o644`/`0o755`); explicit deferred-keys list so the "no scope creep" AC is self-checking.

## Open questions

- Does FEAT-028 v1 ship as a single bead or four sequenced beads? Spec says four. Single might be cleaner if migrations are tightly coupled; four lets bead-quality review happen at each step.
- The Axon-on-Lakebase prerequisite list (per axon backend audit, `plan-2026-05-10-axon-only-architecture.md`) is substantial — does FEAT-004 update happen in parallel with FEAT-028 v1, or sequentially? Sequential is safer.
