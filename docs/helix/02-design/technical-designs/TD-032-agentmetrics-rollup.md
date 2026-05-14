---
ddx:
  id: TD-032
  depends_on:
    - FEAT-010
    - SD-005
    - TD-005
  status: draft
  review:
    self_hash: 746e97c6dbf8a0608847e03ff9b0274068d5e3c31a8daa160ce695ca139090d6
    deps:
      FEAT-010: 63227355a2dd6f3b84d54c6ef88fd45741ce3dca73d47f25cff6f10e6356be05
      SD-005: 43475ce198e0a7629ca57a4a795515c6f29afee0552ca2bbb70a309845617170
      TD-005: d183026903343d47b9f12b2715cc251b0bd69006d522d27bb45f79f5c7d1f774
    reviewed_at: "2026-05-05T00:54:42Z"
---
# Technical Design: Agent Metrics Rollup Engine

## Purpose

This design defines how Story 11 / `agentMetrics(window, groupBy)` should
collect, aggregate, and cache execution outcomes for the agent-metrics
surface. The v1 implementation is intentionally simple: scan the existing
attempt corpus, compute the rollup in memory, and cache the result by a
revision fingerprint. A persistent materialized rollup backend is only
justified once the corpus grows to roughly 5,000 attempts per workspace and
the ad-hoc scan stops being cheap enough.

## Current State

The code already has the v1 scan-and-cache shape:

- `cli/internal/agentmetrics/loader.go:20-49` scans the FEAT-010 run store
  first, then `.ddx/executions/*/result.json`, dedupes by `AttemptID`, and
  returns attempts sorted by `StartedAt`.
- `cli/internal/agentmetrics/types.go:21-44` defines the normalized attempt
  projection used by the rollup path: `attempt_id`, `bead_id`, routing
  metadata, status, outcome, bucket, cost, duration, tokens, and source.
- `cli/internal/server/graphql/resolver_agent_metrics.go:41-74` computes the
  GraphQL rollup response and memoizes it per
  `(workingDir, window, groupBy)`.
- `cli/internal/server/graphql/resolver_agent_metrics.go:231-245` hashes the
  underlying data sources into a revision fingerprint so the cache survives
  archive moves and invalidates on corpus change.

There is no separate persisted rollup collection yet. The v1 path is an
ad-hoc scan with a revision-keyed cache.

## Trigger Threshold

The trigger for introducing a heavier rollup engine is a workspace with about
5,000 deduped attempts. That threshold is measured after source dedupe, not by
raw filesystem entries.

Policy:

- Below the threshold, keep the scan-and-cache implementation as the source of
  truth.
- At or above the threshold, treat the scan cost as a capacity problem and
  evaluate a materialized rollup store or incremental maintenance path.
- The trigger is advisory for architecture, not a runtime error. Hitting the
  threshold must not break queries; it only means the current implementation is
  carrying load it was not designed to own long term.

## Schema

The rollup engine has two schemas: the normalized attempt input schema and the
aggregated output schema.

### Normalized attempt schema

Each input row to the rollup engine is an `agentmetrics.Attempt`:

```json
{
  "attempt_id": "try-2026-05-03T06:26:25Z",
  "bead_id": "ddx-1234abcd",
  "harness": "claude",
  "provider": "openai",
  "model": "gpt-5.5",
  "route": "claude/gpt-5.5",
  "status": "closed",
  "outcome": "merged",
  "bucket": "successful",
  "cost_usd": 0.42,
  "duration_ms": 3812,
  "exit_code": 0,
  "started_at": "2026-05-03T06:20:00Z",
  "finished_at": "2026-05-03T06:26:25Z",
  "input_tokens": 1140,
  "output_tokens": 621,
  "source": "run-store"
}
```

Schema rules:

- `attempt_id` is the dedupe key.
- `source` is one of `run-store` or `bundle`.
- `route` is the canonical `harness/model` label; an empty model collapses to
  the harness name only.
- `bucket` is derived from status, with `already_satisfied` counted as a
  success per Story 11.

### Aggregated output schema

The rollup response is a revision-scoped envelope containing row aggregates:

```json
{
  "window": "W7D",
  "group_by": "ROUTE",
  "revision": "sha256:...",
  "rows": [
    {
      "key": "claude/gpt-5.5",
      "attempts": 48,
      "successes": 31,
      "success_rate": 0.6458333333,
      "mean_duration_ms": 3821.4,
      "p50_duration_ms": 3510,
      "p95_duration_ms": 6201,
      "mean_cost_usd": 0.38,
      "mean_input_tokens": 1099.2,
      "mean_output_tokens": 616.4,
      "effective_cost_per_success_usd": 0.5883870968,
      "last_seen_at": "2026-05-03T06:26:25Z"
    }
  ]
}
```

Schema rules:

- `window` is the query window enum (`24h`, `7d`, or `30d`).
- `group_by` is one of `MODEL`, `HARNESS`, `PROVIDER`, or `ROUTE`.
- `revision` is the fingerprint of the underlying source corpus used to build
  the response.
- `rows[].key` is the axis label for the selected grouping.
- `rows[].effective_cost_per_success_usd` is omitted when a bucket has zero
  successes.
- `rows[].last_seen_at` is omitted when the bucket has no timestamps.

## Invalidation Strategy

The v1 cache is revision-keyed, not time-keyed.

Cache identity:

- workspace path
- query window
- group-by axis
- corpus fingerprint

Fingerprint inputs:

- `.ddx/exec/runs/` directory listing and file stats
- `.ddx/executions/` `result.json` payload locations and file stats
- `.ddx/beads.jsonl` file stats
- `.ddx/beads-archive.jsonl` file stats

Invalidation rules:

- Any add, remove, or modification in those inputs flips the fingerprint and
  invalidates the cached rollup.
- A cache hit is valid only when the stored fingerprint matches the current
  fingerprint exactly.
- No TTL is required in v1; freshness follows corpus revision, not elapsed
  time.
- No background invalidator is required in v1; cache rebuild happens lazily on
  the next query.

## Rollup Behavior

1. Load the normalized attempt set.
2. Drop attempts that fall outside the selected window.
3. Group the remaining attempts by the selected axis.
4. Compute count, success rate, mean duration, p50/p95 duration, mean cost,
   mean token counts, effective cost per success, and last-seen timestamp.
5. Sort rows by attempts descending, then key ascending.
6. Cache the result under the current fingerprint and return it.

This keeps the v1 implementation deterministic and easy to reason about while
preserving a clean upgrade path to a persistent rollup store once the 5,000
attempt threshold is no longer acceptable.

## Non-Goals

- No persisted `agentmetrics_rollups.jsonl` or equivalent store in v1.
- No background compaction daemon.
- No change to the GraphQL surface beyond the existing `agentMetrics`
  response shape.
- No attempt to optimize the legacy `.ddx/executions/` fallback before the
  scan/caching path is proven to be the bottleneck.

## Operating Notes

- The scan path remains authoritative for v1. The cache is an accelerator, not
  a separate source of truth.
- The trigger threshold is a design signal, not a hard runtime guardrail.
- Any future materialized-rollup design must preserve the current aggregate
  schema so GraphQL clients do not need a response rewrite when the storage
  strategy changes.
