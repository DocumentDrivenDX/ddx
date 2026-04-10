---
ddx:
  id: SD-015
  depends_on:
    - FEAT-001
    - FEAT-006
---
# Solution Design: Agent Routing and Catalog Resolution

## Overview

DDx should route ordinary agent requests by intent and normalized routing
signals, not by harness name alone.
Users primarily express:

- a profile such as `cheap`, `fast`, or `smart`
- a model ref or exact pin such as `qwen3` or `claude-sonnet-4-20250514`
- an effort level such as `high`

DDx then decides which harness should execute the request. Once DDx chooses
the embedded harness, the embedded `ddx-agent` runtime chooses its own
provider/backend internally.

This design keeps the boundary explicit:

- DDx owns cross-harness routing and guardrails
- embedded `ddx-agent` owns shared model-catalog data and provider/backend
  selection inside the embedded runtime

## Routing Inputs

DDx normalizes every agent invocation into a `RouteRequest`:

- `Profile`
- `ModelRef`
- `ModelPin`
- `Effort`
- `Permissions`
- `HarnessOverride`

Interpretation:

- `ModelRef` is a logical catalog ref or alias that should be projected onto
  harness-specific surfaces
- `ModelPin` is an exact concrete model string that bypasses catalog policy
- `HarnessOverride` constrains routing to one harness only

## Resolution Order

1. If `HarnessOverride` is set, evaluate only that harness.
2. Else if `ModelRef` is present, attempt shared-catalog resolution on every
   harness surface.
3. Else if `Profile` is present, resolve the profile and evaluate all harnesses
   that can satisfy it.
4. Else use the configured default profile.
5. If no profile is configured, fall back to the configured default harness.

If a `--model` value fails catalog resolution for all harness surfaces, DDx
reinterprets it as an exact `ModelPin`.

## Shared Catalog Use

DDx consumes the shared embedded-runtime catalog for:

- aliases
- shared profiles
- canonical targets
- deprecation/replacement metadata
- surface-specific concrete model strings

DDx does not own concrete production model tables except as temporary fallback
during migration.

### Surfaces

Initial DDx routing should recognize at least:

- embedded OpenAI-compatible surface
- embedded Anthropic surface
- Codex surface
- Claude Code surface

This means a request such as `qwen3` can legitimately resolve only to the
embedded harness.

## Candidate Planning

DDx evaluates one `CandidatePlan` per harness.

Each plan records:

- `Harness`
- `Surface`
- `RequestedRef`
- `CanonicalTarget`
- `ConcreteModel`
- `SupportsEffort`
- `SupportsPermissions`
- `Installed`
- `Reachable`
- `Authenticated`
- `QuotaState`
- `SignalFreshness`
- `CostClass`
- `EstimatedCostUSD`
- `PerformanceMetrics`
- `RejectReason`
- `Score`

The selected plan should be explainable from this record.

## Harness Capability Model

### Static Capability

Each harness publishes:

- catalog surface
- exact-pin support
- supported effort values
- supported permission modes
- local/cloud classification
- cost class or pricing metadata when known

### Dynamic State

Each harness also has runtime state:

- installed
- reachable
- authenticated
- quota or headroom state: `ok`, `blocked`, or `unknown`
- policy-restricted
- healthy / degraded / unavailable
- last checked timestamp
- signal source and freshness

This state should be cached with TTLs rather than fully reprobed on every run.

## Routing Signal Model

DDx routes using a normalized model composed from multiple signal families:

- **Capability** — whether the harness can satisfy the requested profile,
  model, effort, and permission mode
- **Availability** — installed, reachable, authenticated, policy-allowed
- **Quota/headroom** — current provider limit state when a trustworthy source is
  available; otherwise `unknown`
- **Cost** — provider-reported cost where available, otherwise DDx-owned cost
  estimate or coarse cost class
- **Performance** — minimal DDx-observed metrics such as recent latency and
  recent success/failure
- **Freshness** — when each dynamic signal was last observed

Signal ownership is intentionally split:

- **Provider-native sources** own transcripts, rich session history, and
  current quota/headroom when available
- **DDx** owns only the normalized view and the minimal observed metrics needed
  to compare harnesses at dispatch time
- **Embedded `ddx-agent`** owns its runtime telemetry; DDx consumes references
  and derived metrics rather than re-implementing runtime logging

### Minimal DDx-Owned Routing Metrics

DDx keeps only compact routing facts needed to rank harnesses. It does not
store provider transcripts, provider session stores, or embedded-runtime log
bodies as part of routing state.

#### `RoutingOutcome`

One bounded sample per DDx-observed invocation, used to summarize reliability
and latency.

- `harness`
- `surface`
- `observed_at`
- `success`
- `latency_ms`
- `input_tokens` when available
- `output_tokens` when available
- `cost_usd` when available
- `native_session_id`, `native_log_ref`, `trace_id`, or `span_id` when the
  source provides a reference instead of a body

#### `QuotaSnapshot`

One bounded sample per live-probe or cached quota read, used to model headroom
and subscription pressure.

- `harness`
- `surface`
- `source`
- `observed_at`
- `quota_state`
- `used_percent` when available
- `window_minutes` when available
- `resets_at` when available
- `sample_kind`
  - `native-log`
  - `async-probe`
  - `cache`

#### `BurnSummary`

One derived record per harness/surface pair, used to compare providers without
direct billing APIs.

- `harness`
- `surface`
- `observed_at`
- `burn_index` as a relative, unitless score
- `trend`
- `confidence`
- `basis` describing which snapshot deltas and token/cost observations fed the
  score

#### Freshness And Retention

- Outcome samples are fresh for one routing TTL after observation; older
  samples remain inspectable but are demoted behind fresher data.
- Keep a rolling window of the most recent 50 outcome samples or 7 days of
  samples per harness surface, whichever is smaller.
- Keep quota snapshots for 30 days or one billing window, whichever is
  smaller, and compact older snapshots into the burn summary rather than
  retaining them as raw routing inputs.
- Snapshot freshness is source-specific: provider-native logs are fresh until
  the next known provider write, async probes are fresh until their source TTL,
  and cached values are only as fresh as their cache timestamp.
- Burn calculations restart after a quota reset and use only post-reset
  snapshots for the next accumulation interval.

#### Boundary And Exclusions

- DDx never stores provider prompt/response bodies or full native session
  transcripts in routing metrics.
- DDx never re-implements embedded `ddx-agent` session logs or OTEL storage.
- When embedded telemetry is available, DDx stores only references and
  derived facts such as `session_id`, `trace_id`, `span_id`, and the routing
  outcome samples above.
- Routing logic consumes those references and derived metrics; it does not
  need the underlying transcript or log payloads to rank harnesses.

### Source Precedence

- **Codex current quota/headroom** should come from native Codex session JSONL
  when persistence is enabled. If the log is missing or unreadable, treat the
  value as `unknown` rather than fabricating a headroom state. PTY `/status`
  automation is not the default design.
- **Claude historical usage** should come from `~/.claude/stats-cache.json`.
  That cache is the stable source for account-wide usage history, but not for
  current quota/headroom.
- **Claude current quota/headroom** should use a stable non-PTY source if one
  exists. PTY automation is an explicit fallback of last resort and, if used,
  should update an async snapshot cache rather than block routing on inline
  terminal scraping.
- **embedded `ddx-agent` telemetry** should contribute DDx-observed
  performance, reliability, and provenance metrics, not provider quota state.
- **Performance metrics** should come from DDx-observed runs, including async
  snapshot history when DDx must actively sample a live quota source.

## Candidate Rejection Rules

A candidate must be rejected when any of these are true:

- the harness cannot project the requested profile or model to its catalog surface
- the harness does not support the requested effort level
- the harness does not support the requested permission mode
- the harness is not installed
- the harness is installed but not reachable
- the harness lacks required auth
- the harness is explicitly quota-blocked
- the harness is disabled by config or policy
- the harness cannot accept an exact raw pin when the request bypasses the catalog

Rejected candidates remain inspectable via `ddx agent doctor`, `capabilities`,
and future explain/debug modes.

## Ranking Rules

Valid candidates are ranked by:

1. exactness of model/profile match
2. health and confidence of current state
3. freshness and quality of current routing signals
3. intent:
   - `cheap` prefers lowest-cost viable candidate
   - `fast` prefers fastest viable candidate within acceptable cost bounds
   - `smart` prefers highest-quality viable candidate
4. DDx-observed performance and reliability
5. local over cloud when otherwise equivalent
6. stable tie-breaker order

## Embedded Runtime Boundary

When DDx selects the embedded harness:

- DDx passes the resolved profile/model intent into the embedded runtime
- DDx does not select a concrete provider/backend itself
- embedded `ddx-agent` resolves backend pools, provider type, and strategy

Therefore DDx must never duplicate embedded backend-pool logic.

## CLI and Config Direction

### CLI

Preferred:

```bash
ddx agent run --profile cheap --prompt task.md
ddx agent run --model qwen3 --effort high --prompt task.md
```

Advanced override:

```bash
ddx agent run --harness codex --prompt task.md
```

### Config

```yaml
agent:
  profile: cheap
  harness: ""
  model: ""
  permissions: supervised
```

`harness` remains optional and mostly for operator override or debugging.

## `capabilities` and `doctor`

`ddx agent capabilities <harness>` should evolve to show:

- reasoning levels
- exact-pin support
- effective profile/model mappings
- deprecation/replacement warnings

`ddx agent doctor` should evolve to report:

- installed
- reachable
- authenticated
- quota/headroom state
- degraded vs healthy
- source and freshness for dynamic signals
- whether the embedded harness has at least one viable backend for default
  routing

## Implementation Notes

- The current hardcoded DDx model tables in the agent package are transitional
  only.
- Cross-harness routing belongs in DDx.
- Provider/backend selection belongs in embedded `ddx-agent`.
- DDx must not suppress native persistence for external harnesses by default,
  because native provider stores are part of the routing signal surface.
- Exact-model asks such as `qwen3` must be handled without special cases once
  the shared catalog projections are in place.

## Open Questions

- Should DDx expose a user-visible `--explain-routing` or debug output mode for
  rejected candidates?
- How much coarse pricing metadata should DDx own locally versus delegating to
  `ddx-agent` or harness-specific adapters?
- What stable non-PTY current-quota source, if any, can DDx use for Claude
  Code?
- Should `embedded` remain an alias only, or also be the canonical persisted
  harness name in DDx logs?
