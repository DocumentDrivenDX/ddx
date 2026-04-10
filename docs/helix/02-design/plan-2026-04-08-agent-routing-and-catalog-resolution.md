---
ddx:
  id: PLAN-2026-04-08-AGENT-ROUTING-AND-CATALOG-RESOLUTION
  depends_on:
    - FEAT-001
    - FEAT-006
---
# Design Plan: DDx Agent Routing and Catalog Resolution

**Date**: 2026-04-08
**Status**: Superseded — content reconciled into FEAT-006 and SD-015 (2026-04-09)
**Refinement Rounds**: 1

## Problem Statement

DDx currently exposes agent dispatch primarily through harness selection:

- `ddx agent run --harness codex`
- `ddx agent run --harness claude`
- `ddx agent run --harness agent`

That works for explicit comparisons and debugging, but it is the wrong default
user model. The real user asks are closer to:

- "run this cheaply"
- "use a smart model"
- "run this on qwen3 with high effort"

At the same time, DDx now sits above two different routing layers:

1. **Cross-harness routing in DDx** — choose among codex, claude, opencode,
   embedded `ddx-agent`, and future harnesses.
2. **Provider/backend routing inside embedded `ddx-agent`** — once DDx selects
   the embedded harness, `ddx-agent` may still choose among one or more
   providers or backend pools for the resolved model.

The current specs say DDx owns harness orchestration while `ddx-agent` owns the
shared model catalog and provider/backend orchestration. What is still missing
is one precise DDx routing contract that explains:

- what the user specifies
- what DDx resolves automatically
- how DDx evaluates harness health and restrictions
- how exact model asks interact with the embedded harness
- where DDx stops and `ddx-agent` begins

## Goals

- Make `ddx agent run` intent-first for normal use.
- Keep `--harness` available as an explicit override, not the primary UX.
- Use the shared `ddx-agent` model catalog as the source of truth for aliases,
  profiles, canonical targets, and deprecations.
- Make DDx routing deterministic, explainable, and testable.
- Preserve a strict boundary: DDx chooses the harness; embedded `ddx-agent`
  chooses the provider/backend.

## Non-Goals

- Implement the routing feature in this plan.
- Introduce automatic task classification or "smart AI picks the best model"
  heuristics beyond the explicit rules below.
- Duplicate `ddx-agent` provider or backend-pool logic in DDx.
- Add request-level failover across `ddx-agent` providers from DDx.

## User-Facing Routing Model

### Primary Selectors

Normal users should select one or more of:

- `--profile cheap|fast|smart`
- `--model <ref-or-exact>`
- `--effort low|medium|high`
- `--permissions safe|supervised|unrestricted`

### Explicit Override

Advanced users may also pass:

- `--harness <name>`

`--harness` remains the escape hatch for benchmarking, replay, debugging, and
forced dispatch. It should not be required for common intent-first use.

### User Experience Direction

Preferred:

```bash
ddx agent run -p "fix the failing test" --profile cheap
ddx agent run -p "review this diff" --profile smart
ddx agent run -p "implement this" --model qwen3 --effort high
```

Still supported:

```bash
ddx agent run -p "implement this" --harness codex
ddx agent run -p "implement this" --harness embedded --model qwen3
```

## Normalized Request Model

Before routing, DDx should normalize every invocation into one routing request:

- `profile`
- `model_ref`
- `model_pin`
- `effort`
- `permissions`
- `harness_override`
- future policy hints such as `prefer_local`, `avoid_cloud`, or `max_cost_usd`

Normalization rules:

- `--model` first attempts catalog resolution as a logical reference.
- If catalog resolution fails, `--model` is treated as an explicit concrete pin.
- `--profile` is a shared policy reference, not a harness-specific shortcut.
- `--harness` does not change the meaning of `--profile` or `--model`; it only
  constrains which harnesses may be considered.

## Routing Precedence

DDx must apply these precedence rules in order:

1. If `--harness` is set, DDx only evaluates that harness.
2. Else if `--model` is set:
   - attempt to resolve it through the shared model catalog for each harness
     surface
   - if resolution succeeds for a harness, treat it as a logical model request
   - if resolution fails for every harness surface, treat the value as an exact
     pin and only consider harnesses that can accept raw pins
3. Else if `--profile` is set, resolve the profile through the shared model
   catalog and evaluate all harnesses that have a mapping for the target surface
   and can satisfy the remaining constraints.
4. Else fall back to the configured default profile.
5. If no profile is configured, fall back to the configured default harness and
   its configured/default model.

`--model` is therefore the strongest model selector. `--profile` is the normal
policy selector. `--harness` is the strongest dispatch override.

## Shared Catalog Contract

DDx should consume the `ddx-agent` shared model catalog as the source of truth
for:

- aliases such as `qwen3`, `gpt5`, `sonnet`, `opus`
- shared profiles such as `cheap`, `fast`, `smart`
- canonical targets and replacements
- per-surface concrete model strings

DDx must not keep its own hardcoded concrete production model tables except as
temporary fallback while migrating to the catalog.

DDx should resolve requests against multiple surfaces:

- embedded `ddx-agent` OpenAI-compatible surface
- embedded `ddx-agent` Anthropic surface
- Codex surface
- Claude Code surface
- future surfaces as added by the catalog

If a requested ref maps only to an embedded `ddx-agent` surface, then only the
embedded harness can satisfy it.

Example:

- User asks for `--model qwen3`
- Catalog has a mapping for embedded `ddx-agent` OpenAI-compatible surface
- Catalog has no `codex` or `claude-code` mapping for that ref
- DDx produces exactly one viable candidate: `embedded`
- DDx selects the embedded harness
- Embedded `ddx-agent` then resolves its own backend/provider internally

This overlap is intentional and must remain explicit in docs and code.

## Harness Capability and State Model

DDx routing must consider both static capabilities and dynamic state.

### Static Capability Metadata

Each harness should expose at least:

- harness name
- surface name used for model-catalog resolution
- whether exact model pins are supported
- supported effort values
- supported permission modes
- local/cloud classification
- rough cost class or pricing metadata when known

### Dynamic State Metadata

Each harness should also have routable runtime state:

- installed
- reachable
- authenticated
- quota/credits OK
- policy-restricted or disabled
- healthy / degraded / unavailable
- last checked time

The current `ddx agent doctor` connectivity model is a useful starting point
but is too coarse for routing decisions. DDx should define a structured status
record with TTLs so routing does not need to synchronously probe every harness
on every invocation.

## Candidate Planning

DDx should evaluate routing by producing one candidate plan per eligible
harness.

Each candidate plan should answer:

- `harness`
- `surface`
- `requested_ref`
- `resolved_canonical_target`
- `resolved_concrete_model`
- `supports_effort`
- `supports_permissions`
- `installed`
- `reachable`
- `authenticated`
- `quota_ok`
- `cost_class`
- `estimated_cost_usd` when available
- `rejection_reason` if not runnable
- `score`

This planning step is important because "automatic" routing must still be
auditable. DDx should be able to explain why a harness was selected or rejected.

## Candidate Rejection Rules

A candidate must be rejected when any of these are true:

- the harness cannot project the requested profile or model to its catalog
  surface
- the harness does not support the requested effort
- the harness does not support the requested permission model
- the harness is not installed
- the harness is installed but not reachable
- the harness lacks required auth or quota
- the harness is disabled by config or policy
- the harness cannot accept an exact raw pin when the request bypasses the
  catalog

Rejected candidates should remain inspectable for `ddx agent doctor`,
`capabilities`, and future explain/debug modes.

## Candidate Ranking Rules

Among viable candidates, DDx should rank by:

1. Exactness of match to the requested profile/model.
2. Constraint satisfaction confidence:
   - healthy over degraded
   - authenticated/quota-ok over unknown state
3. Cost intent:
   - `cheap` prefers the lowest-cost viable candidate
   - `fast` prefers the fastest viable candidate within acceptable cost bounds
   - `smart` prefers the highest-quality viable candidate
4. Local over cloud when cost and fit are otherwise equivalent.
5. Stable tie-breaker preference order.

DDx must not silently choose a cloud harness when an equivalent healthy local
embedded path exists for a `cheap` ask unless the config explicitly prefers
otherwise.

## Embedded Harness Boundary

The embedded harness should be user-facing as `embedded`, with an internal
canonical identity such as `ddx-agent`.

DDx owns:

- selecting `embedded` versus `codex`, `claude`, `opencode`, and others
- projecting model refs and profiles across harness surfaces
- deciding whether the embedded harness is the best viable candidate

Embedded `ddx-agent` owns:

- selecting provider type
- selecting backend pool
- resolving provider/backend strategy such as `round-robin` or
  `first-available`
- exact provider auth and transport details

DDx must not inspect or replicate embedded provider-level backend decisions
beyond what is needed for coarse cost/availability routing.

## `ddx agent capabilities` Direction

`ddx agent capabilities` should evolve from "show configured model defaults" to
"show effective routing capability for this harness."

For each harness, capabilities should eventually expose:

- supported effort values
- exact-pin support
- resolved current profile mappings visible on that harness surface
- deprecated aliases and their replacements
- whether the harness is currently healthy and authenticated
- whether the harness is local or cloud-backed

For the embedded harness, capabilities should expose DDx-visible routing
information, not the full internal provider pool state. Provider/backend pool
inspection belongs in `ddx-agent` tooling, not DDx.

## `ddx agent doctor` Direction

`ddx agent doctor` should distinguish:

- installed but unusable
- usable but quota-restricted
- usable but degraded
- healthy and routable

For the embedded harness, doctor should report:

- embedded runtime available
- shared catalog load status
- embedded routing config present or missing
- whether at least one backend/provider can satisfy default routing

DDx doctor should not attempt to fully replace `ddx-agent` backend/provider
diagnostics.

## Config Direction

The normal DDx config path should evolve toward intent-first defaults:

```yaml
agent:
  profile: cheap
  harness: ""
  model: ""
  permissions: supervised
```

Interpretation:

- `profile` is the default routing intent
- `harness` is optional and empty by default
- `model` is an optional forced default model ref or pin

Advanced harness-specific configuration may still exist, but the top-level
story should favor profile/model intent over harness naming.

## Required Spec Evolution Before Build Work

This design is not an implementation shortcut. DDx must evolve the governing
spec stack before starting the implementation bead.

Required evolution order:

1. Update `FEAT-006` so the agent service no longer treats harness selection as
   the primary UX and instead defines intent-first routing, structured harness
   state, and the embedded-harness boundary.
2. Update `FEAT-001` CLI surface definitions so `ddx agent run`,
   `capabilities`, and `doctor` reflect the new routing model.
3. Update `architecture.md` so the execution-layer description explicitly
   distinguishes:
   - DDx harness routing
   - shared model-catalog consumption
   - embedded `ddx-agent` backend/provider routing
4. Create or update a DDx solution-design artifact if FEAT-006 becomes too
   broad to carry the routing contract alone.
5. Only after those artifacts converge should a build bead be created or
   resumed.

Implementation must therefore be blocked on specification evolution.

## Implementation Shape After Spec Evolution

The likely implementation sequence is:

1. Add catalog-backed routing request normalization.
2. Add harness static capability descriptors and dynamic status cache.
3. Add candidate planning and ranking.
4. Add intent-first CLI/config surfaces.
5. Migrate `capabilities` and `doctor` output to the new model.
6. Remove duplicated hardcoded model-family policy from DDx.

## Open Questions

- Should DDx expose a user-visible `--explain-routing` or debug output mode for
  rejected candidates?
- How much coarse pricing metadata should DDx own locally versus delegating to
  `ddx-agent` or harness-specific adapters?
- Should `embedded` remain an alias only, or also be the canonical persisted
  harness name in DDx logs?
