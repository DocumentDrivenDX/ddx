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

- a profile such as `cheap`, `default`, `fast`, or `smart`
- a model such as `qwen3.6` or `claude-sonnet-4-20250514`
- an effort level such as `high`

The four profiles are escalation ladders, not single-tier selectors. Per
FEAT-006 §"Profile Semantics": `default` = local first, escalate to cloud;
`cheap` = local only; `fast` = cloud-fast, skip local; `smart` = cloud-
high-quality, no escalation. This SD specifies how the routing layer
evaluates candidates within one tier; tier escalation itself is a
control-loop concern layered on top (see FEAT-006 requirement 3c).

DDx then decides which harness should execute the request. Once DDx chooses
the embedded harness, the embedded `ddx-agent` runtime chooses its own
provider/backend internally.

This design keeps the boundary explicit:

- DDx owns cross-harness routing and guardrails
- embedded `ddx-agent` owns shared model-catalog data and provider/backend
  selection inside the embedded runtime

## Routing Inputs

Users express intent through CLI flags. DDx normalizes these into a
`RouteRequest` for internal processing:

- `Model` — the user's requested model string (may be exact or a prefix)
- `Profile` — a routing profile (`cheap`, `default`, `fast`, `smart`)
- `Tier` — the currently-evaluated tier within the profile's ladder
  (`cheap`, `standard`, `fast`, `smart`). For single-tier profiles (`cheap`,
  `smart`), Tier equals Profile. For ladder profiles (`default`, `fast`),
  Tier advances on escalation (see FEAT-006 §"Profile Semantics").
- `Provider` — a provider preference or target
- `Effort` — effort level
- `Permissions` — permission mode
- `HarnessOverride` — explicit harness bypass

Internally, after model resolution, the request carries:

- `ResolvedModel` — the concrete model ID after fuzzy matching
- `ModelRef` — when the resolved model came from a catalog alias
- `ModelPin` — when the resolved model is an exact concrete string

The `ModelRef`/`ModelPin` distinction is an implementation detail for catalog
attribution, not a user-facing concept. Users always specify `--model`.

## Resolution Order

The four routing modes, in precedence order:

### 1. Harness Override (`--harness H`)

Skip all routing. Evaluate only harness H. Any `--model` value becomes a
pass-through string to the harness binary — DDx does not validate whether the
harness can serve it. This is the debugging/override escape hatch.

### 2. Explicit Model (`--model X`)

The user knows what model they want. DDx finds the best provider for it.

#### Step 1: Resolve the model name

Build a unified model pool from all sources: the shared catalog (aliases,
canonical targets) and live provider discovery (`/v1/models` on all
configured providers). This produces a flat set of known model IDs across
all providers.

Match X against the pool:

- **Exact match**: X equals a known model ID → use it.
- **Prefix match**: X is a prefix of one or more known model IDs (e.g.
  `qwen3.6` matches `qwen3.6-35b-a3b` and `qwen3.6-27b`). Select the
  model with the **shortest suffix after the prefix** — i.e. the model ID
  closest in length to X. This is more predictable than edit distance:
  `qwen3.6` → `qwen3.6-27b` (suffix `-27b`, length 4) wins over
  `qwen3.6-35b-a3b` (suffix `-35b-a3b`, length 8). On equal suffix
  length, prefer catalog entries over discovered-only models, then
  alphabetically first as a stable tiebreaker. When multiple models tie
  after all tiebreakers, DDx emits a warning listing the tied candidates
  and picks the first.
- **No match**: error clearly:
  `model "X" is not in the catalog and no discovered provider serves it`.

When a fuzzy (non-exact) match is used, DDx prints the resolved model name
so the operator sees what was selected:
`model "qwen3.6" resolved to "qwen3.6-27b" (fuzzy match)`.

When the resolved model has deprecation metadata in the catalog, DDx prints
a warning: `model "X" is deprecated; use "Y" instead`. Resolution proceeds
with the deprecated model — the warning is informational, not blocking.

After this step there is a single resolved model ID.

#### Step 2: Find candidate providers

Identify all providers that can serve the resolved model ID. Sources:

- Live provider discovery: providers whose `/v1/models` list includes the
  resolved model.
- Catalog surface mappings: harnesses whose catalog surface maps to the
  resolved model.

Each provider+model pair becomes a candidate plan.

If no provider can serve the resolved model, error:
`model "X" resolved but no available provider serves it`.

#### Step 3: Pick the best provider

Rank candidates by standard scoring (cost class, locality, health,
historical success rate). If `--provider P` is set, prefer P but do not
require it — if P does not serve the model, route to a provider that does.

The resolved model string (the actual model ID, not the user's prefix)
becomes the `ConcreteModel` on the selected candidate plan.

This replaces the previous behavior where uncataloged models silently became
exact pins routed to whichever harness won scoring — which typically sent local
models to cloud harnesses that couldn't serve them (see SD-023 §"`--model`
must discover or error, not fall back").

#### `--model-ref` variant

`--model-ref R` is an explicit catalog-only override. It skips fuzzy matching
and live discovery entirely — R must resolve through the catalog on at least
one harness surface, or DDx errors. This exists for scripts and automation
that want deterministic catalog resolution without discovery variability.
`--model-ref` follows mode 2's steps 2-3 (find providers, pick best) but
replaces step 1 with strict catalog lookup.

### 3. Profile (`--profile cheap|default|fast|smart`)

Catalog-driven autopilot with tier escalation. The profile names a ladder
(see FEAT-006 §"Profile Semantics"); routing walks the ladder tier by tier.
Within each tier, resolve the tier's model ref through the shared catalog for
each harness surface and evaluate all harnesses that can satisfy it. This is
the primary path for automated execution (execute-loop, cost-tiered
escalation).

Profile routing uses the **catalog only**, not the unified discovery pool.
Uncataloged models discovered via live probing are never auto-selected by
profile routing — they must be explicitly requested via `--model`.

### 4. Default

Use the configured default profile (`agent.profile` in config). If no profile
is configured, fall back to the configured default harness.

### Precedence

When multiple flags are set, the lowest-numbered mode wins:

- `--harness` (mode 1) always wins — overrides everything.
- `--model` (mode 2) takes precedence over `--profile` (mode 3) and
  `--provider` alone (mode 5). If `--model` and `--profile` are both set,
  `--profile` is ignored.
- `--provider` alone (mode 5) takes precedence over default (mode 4).
- When `--provider` is combined with `--model` or `--profile`, it acts as
  a soft preference modifier, not its own mode (see "Provider as Modifier").

### 5. Provider Targeting (`--provider P` alone)

When `--provider P` is set without `--model` or `--profile`, DDx discovers
the models available on provider P and selects the best one. Selection
prefers models that appear in the catalog (known cost class, quality tier)
over uncataloged models. Among cataloged models, prefer the highest-quality
tier available. Among uncataloged models with no ranking data, prefer
alphabetically first as a stable tiebreaker. If P has no available models
or P is not a configured provider, error:
`provider "P" has no available models` or `unknown provider "P"`.
No fuzzy matching on provider names — P must match exactly.

### Provider as Modifier

When `--provider P` is combined with `--model` or `--profile`:

**With `--model X`** (mode 2): soft preference. After resolving X to a
concrete model (step 1), prefer P when ranking candidates. If P does not
serve the resolved model, route to a provider that does. For example,
`--model qwen3.6 --provider vidar` routes to bragi if bragi has a qwen3.6
match and vidar does not.

**With `--profile`** (mode 3): soft preference. Prefer P when multiple
candidates satisfy the profile. If P cannot serve the profile, route to a
provider that can.

**Harness name collision**: if P exactly matches a registered harness name
(e.g. "agent", "claude", "codex") **and `--model` is not set**, P is
treated as a `HarnessOverride` (mode 1). When `--model` is set, P is
always treated as a soft provider preference — even if P names a harness —
because the user's intent is "this model on this provider," not "use this
harness." DDx warns on collision:
`provider "claude" shadows harness "claude"; treating as provider preference
because --model is set`.

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

## Live Provider Discovery

DDx discovers available models from configured providers at routing time.
Discovery results are a peer of catalog data — both feed the unified model
pool used by `--model` resolution (mode 2) and `--provider` targeting.

### How it works

1. Load provider configuration via `agentconfig.Load()` (the ddx-agent
   public API, per SD-023 boundary rules).
2. For each configured provider, call the `/v1/models` discovery endpoint
   (via `provider/openai.DiscoverModels`) to get the current model list.
3. Merge discovered models with catalog entries into a unified pool. Each
   entry records the model ID and which provider(s) can serve it.
4. Map each provider to its harness surface (LM Studio endpoints →
   `embedded-openai` surface → `lmstudio` or `agent` harness).
5. Feed provider+model combinations into candidate planning.

### Caching

Discovery results may be cached with a short TTL (e.g. 30 seconds) to avoid
redundant `/v1/models` calls within a single routing decision. The cache is
per-process, not persisted.

### Partial failure

When some providers time out or fail during discovery, DDx proceeds with the
providers that responded. Unreachable providers are excluded from the model
pool for this routing decision. If all providers fail, discovery returns an
empty pool and the error path in mode 2 step 1 fires. DDx does not block
routing waiting for slow providers — use a short per-provider timeout (e.g.
5 seconds) and move on.

### Assumptions for uncataloged models

Models discovered via live probing but absent from the catalog are assumed to
be high-cost and low-performance for scoring purposes. This prevents them
from being auto-selected by profile routing (cheap/standard/smart) — they
only win when explicitly requested via `--model`. Adding the model to the
catalog with proper cost class and surface mappings is the path to automatic
routing.

### Boundary

DDx calls ddx-agent's public Go APIs for discovery (per SD-023). DDx does
not maintain its own provider tables, parse stdout, or duplicate discovery
logic.

## Candidate Planning

For profile routing (mode 3), DDx evaluates one `CandidatePlan` per harness
— harnesses are the unit of selection and each maps to at most one catalog
surface.

For explicit model routing (mode 2) and provider targeting, DDx may evaluate
multiple candidates for the same harness when that harness backs multiple
providers. For example, the `agent` harness with three LM Studio endpoints
(vidar, bragi, grendel) may produce three candidates if all three advertise
the requested model. Each candidate records its provider endpoint so scoring
can differentiate by locality, health, and historical success rate.

`CanonicalTarget` is the stable attribution key for downstream routing
metrics. When a request resolves only to an exact concrete model pin, DDx
records that concrete model in the same attribution key space so observations
for different resolved models stay separate.

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

> **Implementation note**: the full signal model below is the target design.
> For MVP, only capability, availability, and cost class are required for
> routing decisions. Quota/headroom, burn summaries, and signal freshness
> are deferrable — they improve routing quality but are not blocking for
> the core resolution order and live discovery changes.

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

For MVP, only `RoutingOutcome` (harness, success, latency, cost) is required.
Detailed schemas for `QuotaSnapshot`, `BurnSummary`, freshness/retention
rules, and source precedence are documented in **Appendix A** below and are
deferred until quota-aware routing is implemented.

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
   - `standard` prefers a balanced cost/quality tradeoff
   - `smart` prefers highest-quality viable candidate
4. DDx-observed performance and reliability
5. local over cloud when otherwise equivalent
6. stable tie-breaker order

## Embedded Runtime Boundary

When DDx selects the embedded harness for **profile routing** (mode 3):

- DDx passes the resolved profile/model intent into the embedded runtime
- DDx does not select a concrete provider/backend itself
- embedded `ddx-agent` resolves backend pools, provider type, and strategy

When DDx selects the embedded harness for **explicit model routing** (mode 2)
or **provider targeting**:

- DDx selects which provider endpoint to target (e.g. vidar vs bragi) based
  on discovery and scoring — this is cross-provider routing, not backend-pool
  selection
- DDx passes the resolved model and target provider to the embedded runtime
- The embedded runtime handles the actual connection, authentication, and
  request execution against that provider

The distinction: DDx chooses "which provider endpoint" (cross-provider),
the embedded runtime chooses "how to talk to it" (backend-pool internals).
DDx must never duplicate backend-pool logic (retry strategies, connection
pooling, streaming protocols).

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

---

## Appendix A: Deferred Signal Schemas

> These schemas are the target design for quota-aware and burn-rate routing.
> They are not required for MVP routing (resolution order + live discovery).

### `RoutingOutcome`

One bounded sample per DDx-observed invocation.

- `harness`, `surface`, `canonical_target`
- `observed_at`, `success`, `latency_ms`
- `input_tokens`, `output_tokens`, `cost_usd` (when available)
- `native_session_id`, `native_log_ref`, `trace_id`, `span_id` (when available)

### `QuotaSnapshot`

One bounded sample per live-probe or cached quota read.

- `harness`, `surface`, `canonical_target`, `source`
- `observed_at`, `quota_state`, `used_percent`, `window_minutes`, `resets_at`
- `sample_kind`: `native-log` | `async-probe` | `cache`

### `BurnSummary`

One derived record per harness/surface/target group.

- `harness`, `surface`, `canonical_target`
- `observed_at`, `burn_index` (relative unitless), `trend`, `confidence`
- `basis` (which snapshots/observations fed the score)

### Freshness And Retention

- Outcome: rolling window of 50 samples or 7 days per canonical target.
- Quota snapshots: 30 days or one billing window; older compacted into burn.
- Burn calculations restart after quota reset.

### Source Precedence

- Codex: native session JSONL → unknown (no PTY default)
- Claude: `~/.claude/stats-cache.json` for history; stable non-PTY for live
- Embedded agent: DDx-observed performance metrics only
- DDx never stores provider prompt/response bodies in routing metrics
