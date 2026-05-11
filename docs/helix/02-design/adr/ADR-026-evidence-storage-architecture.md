---
ddx:
  id: ADR-026
  depends_on:
    - TD-031
    - SD-025
---
# ADR-026: Evidence Storage Architecture (3-Tier)

**Status:** Accepted
**Date:** 2026-05-10
**Authors:** bead `ddx-7e9e5e1a`

## 1. Context

DDx accumulates per-attempt evidence (manifest.json, prompt.md, result.json,
reviewer-stream.log, agent stream logs, worker state) without a documented
retention or storage architecture. Before this ADR the implementation was
inconsistent:

- `.ddx/executions/` was tracked in git via explicit un-ignore rules.
- `.ddx/agent-logs/` was gitignored but unbounded on local disk.
- `.ddx/workers/` had no gitignore entry and no active tracking.

A cleanup audit found 12,096 tracked execution files and ~860 MB of local
agent-log data. The absence of a formal design made it impossible to reason
about repo bloat, machine-local retention, or cross-bead analytics without
fanning out full-directory reads.

The design must balance three requirements:

- **Reconstruct the past**: operators and analytics tools need enough durable
  evidence to understand bead outcomes, model/prompt/harness performance, and
  failure patterns across many attempts.
- **Avoid repo bloat**: large binary blobs, raw model-stream JSON, and
  redundant per-attempt copies must not accumulate indefinitely in git.
- **Support cross-bead queries**: aggregate analytics (model performance, prompt
  efficacy, harness comparisons) require a queryable, structured, durable table
  rather than a per-directory scatter.

## 2. Decision

DDx adopts a three-tier evidence storage architecture:

| Tier | Location | Durable? | In git? | Bounded? |
|------|----------|----------|---------|----------|
| 1. Git-tracked structured state | `.ddx/beads.jsonl`, `.ddx/attachments/`, `.ddx/metrics/` | Yes | Yes | Yes — by design |
| 2. Local disk ephemeral evidence | `.ddx/executions/`, `.ddx/agent-logs/`, `.ddx/workers/` | Machine-local | No | Yes — by retention policy |
| 3. External archive (operator-configured) | Mirror backend (local dir or remote) | Operator-managed | No | Operator-configured |

Tier 1 is the single source of truth for bead lifecycle, outcome events, and
aggregate analytics. Tier 2 holds the full evidence bundle for recent attempts
on the local machine. Tier 3 is an optional, operator-configured mirror of
Tier 2 content for long-term retention or cross-machine access.

## 3. Principles

### P1 — Git holds durable, small, structured state

Git tracks only:

- **Bead rows** (`.ddx/beads.jsonl`) — active bead lifecycle state; ~2–5 KB
  per bead at steady state.
- **Closed-bead events** (`.ddx/attachments/<bead-id>/events.jsonl`) — the
  append-only lifecycle event stream for each bead, including outcome
  classifications and review verdicts. Event vocabulary is defined in TD-027 §13 (the controlled list); outcome → event firing mapping is in TD-031 §2.
- **Aggregate per-attempt metrics** (`.ddx/metrics/attempts.jsonl`) — one
  denormalized row per bead attempt with structured fields for cross-bead
  analytics queries. ~300 bytes per row.
- **Code, docs, ADRs, TDs, FEATs** — all governed artifact types.

Git MUST NOT track raw model streams, full prompt bodies, large result blobs,
or any file whose size scales with the number of retries or the token count of
an invocation.

**Rationale:** Git is optimized for text-sized, append-friendly structured
state. Forcing large binary or repetitive evidence into git creates unbounded
repo growth and degrades clone times for all contributors and CI runners.

### P2 — Local disk holds detailed, ephemeral, retention-bounded evidence

The following paths are per-machine, never committed:

- **`.ddx/executions/<attempt-id>/`** — full evidence bundle per attempt:
  `prompt.md`, `manifest.json`, `result.json`, `usage.json`, `checks.json`,
  `reviewer-stream.log`, `decisions.md`, and any other execution artifacts
  produced by SD-025 layer-1/layer-2 records. This is the canonical source for
  deep-dive post-mortems on recent attempts.
- **`.ddx/agent-logs/agent-*.jsonl`** — raw model-stream output. Useful for
  token-level debugging but large and redundant with structured fields in the
  attempt bundle.
- **`.ddx/workers/worker-*/`** — per-worker runtime state (heartbeats, claim
  metadata, in-progress records). This state is volatile by design; workers
  reconstruct it on restart.

**Default retention:** 90 days from the attempt's `finished_at` timestamp.
Attempts with no `finished_at` (interrupted runs) age from their `started_at`.

**Operator override:** The retention window is configurable in
`.ddx/config.yaml` under `evidence.local_retention_days`. Setting it to `0`
disables automatic pruning (operator assumes responsibility for disk use).

**Rationale:** Detailed evidence is needed for debugging recent failures but
has diminishing value over time. Per-machine storage avoids git bloat and
avoids the latency of round-tripping large blobs through a remote for every
operator who clones the repo.

### P3 — External archive is operator-configured, not built in

DDx provides a mirror hook point but does not mandate a backend. The default
configuration has no external archive.

The hook fires after an attempt completes and the Tier 2 bundle is written.
Operators configure the backend in `.ddx/config.yaml` under `evidence.archive`:

```yaml
evidence:
  archive:
    backend: local-dir      # or: s3 (deferred)
    path: /mnt/evidence
```

**Initial backends:**

- `local-dir` — copies the attempt bundle to an operator-specified directory
  path, preserving the `<attempt-id>/` subdirectory structure. Suitable for
  NAS mounts, shared team volumes, and local backup paths.
- `s3` — deferred; defined as a forward-compatible extension point in this ADR
  but not implemented in the initial delivery.

When no archive is configured, attempt bundles are subject only to the local
retention window (P2).

**Rationale:** Operator environments vary too widely to choose a single
archival backend. Embedding S3 credentials or a particular cloud vendor in DDx
defaults would couple the platform to infrastructure choices that are
legitimately project-specific.

### P4 — Aggregate metrics are denormalized, queryable, and durable

`.ddx/metrics/attempts.jsonl` is the analytics table. Each row is one
bead-attempt with structured fields intended for cross-bead queries:

```json
{
  "attempt_id": "20260510T214516-d550ba49",
  "bead_id": "ddx-7e9e5e1a",
  "started_at": "2026-05-10T21:45:16Z",
  "finished_at": "2026-05-10T22:01:44Z",
  "outcome": "closed-merged",
  "harness": "claude",
  "model": "claude-sonnet-4-6",
  "provider": "anthropic",
  "power": 72,
  "input_tokens": 14200,
  "output_tokens": 3100,
  "cost_usd": 0.042,
  "review_outcome": "approve",
  "attempt_index": 1,
  "repair_cycle": 0,
  "spec_id": "ADR-026"
}
```

This file is tracked in git, so the analytics table survives machine resets and
is available to all contributors without local Tier 2 data.

**Rationale:** Scattering per-attempt metrics across `executions/<id>/` dirs
makes aggregate queries expensive and brittle. A denormalized JSONL table
enables `ddx jq` queries over attempt outcomes, model comparisons, harness
performance, and prompt efficacy without fan-out reads.

## 4. Per-Bead Git Footprint

Under this architecture, the tracked git cost for a typical bead with three
attempts is:

| Artifact | Size estimate |
|----------|--------------|
| Bead row in `.ddx/beads.jsonl` | ~2–5 KB |
| `.ddx/attachments/<bead-id>/events.jsonl` | ~1–3 KB |
| 3 rows in `.ddx/metrics/attempts.jsonl` | ~900 bytes |
| **Total tracked per 3-attempt bead** | **~4–9 KB** |

The current state before this ADR stores the full execution bundle in git:
~95 KB per 3-attempt bead (dominated by `manifest.json`, `result.json`, and
`reviewer-stream.log`). This ADR achieves a **~10–20x reduction** in per-bead
git footprint.

## 5. Local Retention

**Default:** 90 days from `finished_at` (or `started_at` for interrupted runs).

**Operator override:**

```yaml
# .ddx/config.yaml
evidence:
  local_retention_days: 90   # set to 0 to disable automatic pruning
```

The pruning command is `ddx evidence prune` (implemented in a sister bead).
Pruning is idempotent and safe to re-run. It removes only files under
`.ddx/executions/`, `.ddx/agent-logs/`, and `.ddx/workers/` that are older
than the configured retention window and are not referenced by a bead in an
active (`in_progress`) state.

Pruning never touches Tier 1 (git-tracked) artifacts. It operates only on
local Tier 2 paths.

## 6. External Archive Interface

The archive hook fires synchronously after the Tier 2 bundle is written and
before the Tier 1 metrics row is appended. If the archive backend fails, DDx
logs a warning but does not fail the attempt — archival is best-effort.

The hook receives:

- `attempt_id` — the subdirectory name under `.ddx/executions/`
- `bundle_path` — absolute path to `.ddx/executions/<attempt-id>/`
- `bead_id` — the bead being executed
- `outcome` — terminal outcome string

No built-in backend is mandated. The initial `local-dir` backend is the
reference implementation. Remote backends (S3, GCS, etc.) are extension points
for future delivery; they must not be assumed present in any code that reads
Tier 1 metrics.

## 7. Cross-Machine Considerations

`.ddx/metrics/attempts.jsonl` carries a **single-machine assumption** in the
initial implementation: rows are appended by the machine that ran the attempt.
When a bead is executed on machine A, machine B does not see the row until A
commits and B pulls.

This is acceptable because:

- The queue drain loop is single-machine today (one `ddx work` process per
  project).
- Metrics rows are small and fast to pull on the next `git pull`.
- The analytics use case is primarily retrospective (post-drain queries), not
  real-time cross-machine aggregation.

**Federation-time partitioning:** When DDx federation (ADR-007) enables
multi-machine drain, `attempts.jsonl` will need to be partitioned by machine
or node to avoid write conflicts. The partition key will be `host` (already a
field in SD-025 record shapes). The metrics extraction bead (filed as a sister
bead) should emit `host` from the start so federation-time partitioning is a
file-split operation rather than a schema change.

The `attempts.jsonl` format is append-only. Rows are never edited in place.
Correction means appending a new row with a `corrects` reference to the prior
row's `attempt_id`.

## 8. Migration Plan

This ADR is the design anchor. Implementation is distributed across sister beads
filed concurrently:

- **B29 — Retention implementation:** implement `ddx evidence prune` and the
  90-day default; wire to post-drain cleanup hook.
- **B30 — Gitignore flip:** remove un-ignore rules that track
  `.ddx/executions/` and `.ddx/workers/` in git; add canonical gitignore
  entries for all Tier 2 paths.
- **B31 — Mirror backend:** implement `local-dir` archive backend and the hook
  that fires post-attempt.
- **Metrics extraction:** implement the metrics writer that appends a row to
  `.ddx/metrics/attempts.jsonl` at attempt close; retroactively populate from
  existing execution bundles where fields are available.
- **Analyze CLI:** implement `ddx analyze` or `ddx metrics` commands for
  cross-bead jq queries over `attempts.jsonl`.

The sister beads depend on this ADR (`ddx-7e9e5e1a`) for the design contract.
B29, B30, B31, and metrics extraction may proceed in parallel once this ADR
is merged.

## References

- `docs/helix/02-design/technical-designs/TD-027-bead-collection-abstraction.md` §13 + TD-031-bead-state-machine.md §2
  (event vocabulary)
- `docs/helix/02-design/solution-designs/SD-025-task-execution-lifecycle.md`
  (execution lifecycle layer model and record shapes)
- `docs/helix/02-design/adr/ADR-007-federation-topology.md` (forward pointer
  for federation-time partitioning)
