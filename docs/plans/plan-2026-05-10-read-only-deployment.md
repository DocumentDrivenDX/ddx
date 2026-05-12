---
ddx:
  id: plan-2026-05-10-read-only-deployment
---
# Read-Only Deployment: Minimum Storage Surface for "BlobStore + AxonStore Only"

Date: 2026-05-10
Status: Multi-model reviewed (opus); concrete sequencing identified.

## Question being answered

> DDx should be able to run in a read-only state with BlobStore + AxonStore as the only backends. What interfaces are missing for that to work?

## Verdict

A **minimum viable read-only deployment** ships in this order:

1. **BlobStore (FEAT-028 v1)** — already drafted.
2. **EntityStore on Axon for `beads` + `bead_events`** — FEAT-004 update, gated on Axon production-readiness work (per `plan-2026-05-10-axon-only-architecture.md`).
3. **EntityStore `attempts` collection (new)** — derived from `.ddx/metrics/attempts.jsonl` rows + `result.json` content via BlobStore pointer. **Highest single-addition value** for cost-tier observability.
4. **Thin read-only ConfigStore** — project + user scopes.

After those four, the **working command set**: full bead inspection, full execution-evidence inspection, all metrics commands, persona/agent config introspection, and `ddx doctor`/`ddx status` working in degraded-but-honest mode.

**Still broken without StreamStore**: `legacy agent log` (full history), `legacy agent usage`, `legacy agent route-status` recent-decisions tail.

**Still broken without library-registry abstraction**: `ddx list`, `ddx prompts`, `ddx skills`, `ddx installed`, `ddx search`, `ddx outdated`, `ddx verify`.

**Permanently out of scope** for read-only: every `init/create/update/run/work/try/install/sync/checkpoint/cleanup` command — these are write-side.

## Why this matters

The user framed it as: *"ddx should be able to run in a read-only state with BlobStore + AxonStore as the only backends."* That's the test for whether the storage abstractions are complete enough — if a read-only deployment with just those two backends doesn't function, the abstraction set is missing pieces. This plan identifies exactly those pieces.

## Read-only command catalog

(Full catalog with file:line citations available via the agent transcript; key categories below.)

| Category | Examples | Reads from |
|---|---|---|
| Bead query | `ddx bead show/list/ready/blocked/status/dep tree/export/doctor/lint/routing/queue/replay/evidence list` | `.ddx/beads.jsonl`, `.ddx/beads-archive.jsonl`, `.ddx/attachments/` |
| Execution-derived metrics | `ddx bead metrics`, `legacy agent metrics tier-success/review-outcomes/cost-efficiency` | `.ddx/executions/*/result.json`, `.ddx/metrics/attempts.jsonl` |
| Agent inspection | `legacy agent log`, `legacy agent list/capabilities/doctor/workers/providers/models/route-status/usage/executions fetch` | `.ddx/agent-logs/`, `.ddx/workers/`, `.ddx/executions/`, config |
| Library browse | `ddx list`, `ddx prompts/persona/skills`, `ddx installed/search/outdated/verify`, `ddx exec list/show` | `.ddx/plugins/`, `.ddx/skills/`, `library/` |
| Project state | `ddx status`, `ddx doctor`, `ddx config` (read), `ddx jq`, `ddx log` | mixed |

## Coverage map

**Already served by today's `bead.Backend`** (would map to Axon-EntityStore directly): all bead query commands.

**Will be served by FEAT-028 v1 BlobStore** (after migration): `ddx bead evidence list/show`, all `result.json`/`manifest.json` readers, `legacy agent executions fetch`.

**Need new EntityStore collections in Axon**:

- **`attempts`** — schema: `attempt_id` (PK), `run_id`, `bead_id`, `worker_id`, `harness`, `model`, `outcome` (merged|preserved|error|task_failed), `failure_mode`, `cost_usd`, `duration_ms`, `started_at`, `finished_at`, `requested_harness`, `requested_model`, `result_blob_key` (BlobStore pointer), `prompt_blob_key`, `manifest_blob_key`. Unlocks: `bead_metrics`, all `agent metrics *`, `agent log --bead` outcome resolution, `agent workers` (server API path).
- **`run_state`** — for in-flight runs; read-only deployment can probably skip entirely.
- **`plugin_dispatches`** — graphql resolver consumes; no CLI read path uses directly.

**Workers**: server-process in-memory today; on-disk `.ddx/workers/<id>/{spec,status}.json` is diagnostic projection. `legacy agent workers` already does HTTP-first fetch from server (`agent_workers.go:307`). In remote read-only deployment, command MUST go through server API; local-disk fallback is meaningless. Not a new EntityStore collection — a transport question.

**Need StreamStore (deferred in FEAT-028)** for: `legacy agent log` (`.ddx/agent-logs/agent-*.jsonl` + per-session `mirror.log`), `legacy agent usage`, `legacy agent route-status` (recent decisions from `routing-outcomes.jsonl`), `legacy agent metrics` (also reads `.ddx/metrics/attempts.jsonl` via `cli/internal/attemptmetrics/`).

**Need ConfigStore** for: `ddx config` (read), `ddx persona --list/--show` (bindings), every `legacy agent providers/models/route-status/list/capabilities/doctor` command (loads `.ddx/config.yaml`), `ddx status`, `ddx doctor`.

**Need library-registry abstraction**: every `ddx list/prompts/skills/installed/search/outdated/verify/exec list-show`. Either treat installed packages as a BlobStore prefix (and `List`) — works mechanically but loses manifest semantics — or add a proper `PackageRegistry` (`ListInstalled() []PackageRef`, `Manifest(pkg) Manifest`, `OpenFile(pkg, path) io.Reader`). Cleaner. FEAT-028 calls this out as deferred.

## Minimal interfaces (read-only subset)

```go
type ReadOnlyEntityStore interface {
    Get(ctx, collection, id) (Row, error)
    List(ctx, collection, filter Filter) ([]Row, error)
    ListEvents(ctx, collection, ownerID, since time.Time) ([]Event, error)
}

type ReadOnlyBlobStore interface {
    Get(ctx, key) (io.ReadCloser, error)
    Stat(ctx, key) (Info, error)
    List(ctx, prefix) ([]Key, error)
}
```

Read-only doesn't need Create/Update/Close/Claim — much smaller surface than full `Backend`.

## Degraded commands

- `ddx doctor` — does writability checks (`cli/cmd/doctor.go:449`). Needs a `--read-only` mode to skip writability gracefully.
- `ddx status` — git modification check (`--changes`, `--diff`) implicitly requires a git working tree.
- `legacy agent doctor` — config probes work; harness probes (which try to invoke agents) fail.
- `legacy agent workers` — works through server HTTP API; local `.ddx/executions/*/manifest.json` scan path is dead.
- `ddx config` (read flags) — works through ConfigStore.

## Implications

This plan is a **completeness test for FEAT-028 + the Axon EntityStore work**. If the answer to "does ddx run read-only with just BlobStore + AxonStore" is "no, you also need StreamStore and a registry abstraction," then those abstractions need their own FEAT specs to ship before a Databricks-App read-only deployment is feasible.

That's not a problem with FEAT-028 — it's an expansion of the roadmap. FEAT-028 v1 (BlobStore) is the right starting point. After it lands, the next two pieces of work are the Axon EntityStore migration (FEAT-004 update) and the new `attempts` collection design.

## Open questions

- Is "remote read-only with degraded library browse" acceptable for v1, or is library browse table-stakes? If table-stakes, the registry abstraction work front-loads.
- Should the `attempts` collection schema be designed alongside FEAT-028 v1 (so BlobStore migration of `result.json` writers can write the EntityStore row at the same time), or sequenced strictly after?
- StreamStore design: probably worth a stub spec (StreamStore = deferred FEAT) so callers know what shape to expect, even if the implementation lands later.
