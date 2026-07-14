---
ddx:
  id: FEAT-014
  depends_on:
    - helix.prd
    - FEAT-006
    - FEAT-010
---
# Feature: Agent Usage, Cost, and Runtime Projections

**ID:** FEAT-014
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

FEAT-014 defines how DDx records and aggregates usage, cost, and runtime facts
returned by Fizeau. DDx is the work tracker and git-aware executor. Fizeau is
the complete agent runtime and the exclusive owner of concrete harness,
provider, model, catalog, routing, native-log, authentication, quota, and usage
acquisition behavior.

DDx consumes the public final usage, cost, duration, warning, and actual-route
fields Fizeau exposes. It stores those fields beside DDx-owned request, bead,
worktree, repository-gate, and landing evidence so operators can audit a run
and enforce cost budgets. DDx does not reconstruct a Fizeau projection by invoking a
provider CLI, discovering or parsing a native session log, reading a provider
cache, maintaining a model price table, or querying a route catalog.

This boundary applies uniformly to `ddx run`, `ddx try`, `ddx work`, review and
recovery runs, and agent-backed artifact generation. All of those operations
use the same typed usage/cost fields and the same accounting rules.

## Problem Statement

DDx needs durable answers to operational questions:

- How many input, cached-input, and output tokens did a run consume?
- What billable cost or cost class did Fizeau report?
- How much known cost has a bead, attempt, review cycle, drain, or artifact
  generation accumulated?
- Which values are known, unknown, estimated upstream, or unavailable?
- Which Fizeau session and DDx repository result produced the evidence?

The previous FEAT-014 mixed those questions with concrete runtime concerns. It
assigned DDx direct Codex and Claude output parsers, provider-native log and
cache readers, authentication and quota probes, catalog normalization, model
pricing, provider comparison, and route-selection policy. That design created
a second, incomplete harness runtime inside DDx and contradicted the Fizeau
boundary in FEAT-006.

The desired outcome is a single authority chain:

1. DDx constructs a Fizeau request from work facts, abstract `MinPower`,
   and any opaque operator-owned constraints.
2. Fizeau selects and runs the concrete route, acquires usage/cost/auth/quota
   signals, and returns its public immediate error or final event.
3. DDx records the available public final fields with DDx-owned repository evidence.
4. DDx aggregates usage and cost for audit, metrics, and budget decisions.
5. DDx may use a higher `MinPower` for stronger-review intent or for a distinct
   new attempt after capability-sensitive evidence; infrastructure, transport,
   route, quota, authentication, setup, operator-action, and generic failures
   never raise power. DDx never chooses or steers a concrete route.

## Normative Ownership Boundary

| Concern | Normative owner | DDx behavior |
|---|---|---|
| Harness/provider/model discovery and catalog | Fizeau | No catalog dependency, query, cache, normalization, or candidate ranking |
| Concrete route selection, fallback, and provider retry | Fizeau | Sends no DDx-originated concrete route; consumes the public operation outcome |
| Provider-native output, session logs, local caches, billing APIs, auth, quota, and rate-limit signals | Fizeau | Never discovers, reads, parses, probes, or reconstructs them |
| Per-session token, cost, cost-class, freshness, and provenance projection | Fizeau | Decodes only fields defined by the public Fizeau contract |
| Bead, attempt, worktree, repository gates, result revision, landing, and durable DDx evidence | DDx | Records and correlates these facts with its request ID and opaque Fizeau `SessionLogPath` |
| Per-run, per-attempt, per-bead, per-drain, and per-artifact usage/cost aggregation | DDx | Aggregates typed Fizeau projections without route interpretation |
| Retry and review capability escalation | DDx for new-attempt/review policy; Fizeau for routing | DDx may raise only abstract `MinPower`, and only for stronger review intent or a distinct new attempt after capability-sensitive evidence |
| `MaxPower`, harness, provider, model, or public Fizeau `Policy` constraint | Operator | DDx forwards the supplied value unchanged and never originates, infers, ranks, loosens, rewrites, removes, or branches on it; current v0.14.50 has no per-request `Profile` field |

The following invariant is load-bearing:

> DDx can ask Fizeau for more capability. DDx cannot tell Fizeau which concrete
> harness, provider, or model should supply it.

Concrete harness/provider/model values returned by Fizeau are opaque audit
facts. DDx may persist and render them only on that exact run's audit detail.
It must not expose route-keyed list filters or use them to compute `MinPower`, select review
pairing, rank candidates, estimate prices, decide fallback, or otherwise affect
a future execution request.

## Current Public Fields And Future Projection Gate

CONTRACT-003 owns the wire schema. Current v0.14.50 exposes final
`ServiceFinalData` fields `Status`, `ExitCode`, opaque generic `Error`,
`FinalText`, `DurationMS`, `Usage`, `Warnings`, `CostUSD`, `SessionLogPath`, and
`RoutingActual`. `ServiceFinalUsage` currently exposes optional input, output,
cache-read, cache-write, cache, reasoning, and total token counts plus `Source`,
`Fresh`, `CapturedAt`, and per-source evidence. Warnings may also carry source
evidence. DDx preserves those public usage-provenance fields. The final payload
does not expose a general typed cause/stage, `RetryAfter`, cost class/currency/
cost provenance, continuation capability, or process-tree disposition. Missing
current fields remain unknown.

The current and future concepts are separated below.

### Session and terminal fields

- stable Fizeau session/run reference distinct from `SessionLogPath`
- machine-readable terminal cause and Fizeau-owned stage
- start time and finish time (current final data exposes elapsed `DurationMS`)
- abstract actual power (current audit-only `RoutingActual.Power`)
- optional concrete harness/provider/model/route facts for opaque audit only
- any terminal facts beyond the public final fields above

### Usage fields

- input tokens
- cache-read, cache-write, and aggregate cache tokens
- output tokens
- reasoning tokens
- total tokens
- current `Source`, `Fresh`, `CapturedAt`, and per-source evidence
- any additional typed units added by a versioned Fizeau contract

### Cost fields

- current `CostUSD` scalar observation, whose zero/omission ambiguity is
  preserved;
- whether cost is known
- amount and currency when known
- cost class, such as metered, subscription, local, or unknown
- whether the amount is provider-reported or estimated by Fizeau

Only `CostUSD` exists in v0.14.50. The known marker, generalized currency, cost
class, and cost provenance are future-gated.

Absence is not zero. Current usage pointers distinguish missing from explicit
zero. Current scalar `CostUSD` does not reliably distinguish omission from an
explicit zero and supplies no cost class, currency, or provenance, so DDx must
not label a zero as a known free/local/subscription run. DDx never fills a gap
from a concrete model name, built-in price table, native provider log, or
provider cache.

## DDx Run and Attempt Record

Each FEAT-010 layer-1 run record stores:

- DDx request and correlation facts: run id, parent run id, bead/attempt id,
  role, timestamps, requested `MinPower`, permissions, effort, and timeout;
- operator-owned `MaxPower` or concrete constraints, if supplied, as an opaque
  immutable passthrough envelope;
- DDx request correlation, opaque Fizeau `SessionLogPath`, and available public
  final fields;
- opaque Fizeau artifact references rather than copied or normalized native
  transcript bodies; and
- DDx-owned links to the base revision, result revision, checks, review verdict,
  landing/preservation result, and evidence bundle.

Layer-2 and layer-3 records reference their child layer-1 records and aggregate
their typed usage/cost facts. They do not create a second provider session
schema. A review, repair, reframer, decomposer, or artifact-generation run uses
the same fields and is distinguished by DDx-owned role/correlation metadata.
Review accounting does not include implementer/reviewer route comparison or a
`review-pairing-degraded` policy signal.

The canonical store is the FEAT-010 run substrate. An attempt evidence bundle
may project the same fields into `result.json`, but that projection must be
derived from the canonical run record and must not become an independent usage
authority.

## Requirements

### Functional

1. **Fizeau-only acquisition.** DDx MUST obtain current-run usage, cost,
   concrete route audit facts, auth/quota outcomes, and runtime provenance only
   from typed public Fizeau projections. Production DDx code MUST NOT integrate
   directly with Claude, Codex, Gemini, OpenCode, or another provider runtime.

2. **No provider-native parsing.** DDx MUST NOT discover or parse provider
   stdout/stderr, native session JSONL, local provider caches, status-line
   output, hook output, billing responses, rate-limit headers, or provider
   process state. Opaque Fizeau artifact references may be retained without
   opening or interpreting their contents.

3. **No catalog or routing dependency.** DDx MUST NOT query, cache, normalize,
   or interpret the Fizeau model/harness/provider catalog. DDx usage, budget,
   retry, review, queue, and status code MUST NOT call a route-selection helper
   or use a concrete route fact to affect execution.

4. **Abstract escalation only.** A stronger-review request may raise
   `MinPower` by review intent. A failed implementation may raise it only on a
   distinct new DDx attempt after capability-sensitive evidence. Infrastructure,
   transport, route, quota, authentication, setup, operator-action, and generic
   failures MUST keep power unchanged. DDx MUST NOT derive a power change from
   a model catalog or concrete model/provider identity. Returned abstract
   actual-power may inform the new floor only under that same evidence rule.

5. **Immutable operator constraints.** `MaxPower`, harness, provider, model,
   and public Fizeau `Policy` constraints are operator-owned inputs. DDx MUST
   forward them byte-for-byte unchanged across retries and reviews. DDx MUST
   NOT originate them or branch on their presence or value. Fizeau owns
   compatibility validation and returns its public immediate error or final
   outcome when a request cannot be satisfied.

6. **Public-field persistence.** Every completed layer-1 run MUST persist the
   available public Fizeau usage/cost/final fields, usage presence and public
   provenance/freshness/source evidence, DDx request correlation, and opaque
   `SessionLogPath` in the FEAT-010 run record. Cost provenance/class and a
   stable session ID remain unknown until a future pinned contract exposes them.

7. **DDx runtime facts.** Every layer-2 attempt MUST retain DDx-owned elapsed
   time, base revision, result revision when present, repository-check result,
   landing/preservation disposition, and links to every child layer-1 run.
   These facts remain distinct from Fizeau runtime facts.

8. **Aggregation.** DDx MUST aggregate known usage and cost by run, attempt,
   bead, role, artifact, work drain, project, and time window. Unknown values
   MUST be counted separately and MUST NOT be coerced to zero or silently
   excluded from completeness reporting.

9. **Budget accounting.** FEAT-010 budget and stop policies MUST consume the
   public FEAT-014 `CostUSD` observation with explicit unknown coverage. A cost
   class or generalized currency may participate only after a pinned public
   contract exposes it. Implementation, review, repair,
   reframer, decomposer, and agent-backed artifact-generation runs contribute
   under the same rules. FEAT-014 defines accounting semantics; FEAT-010 and
   ADR-024 define when a budget stops new DDx work.

10. **No DDx price inference.** DDx MUST NOT maintain a concrete model pricing
    table or estimate cost from a model identity and token count. An estimated
    amount is acceptable only when Fizeau marks it as an upstream estimate with
    explicit provenance.

11. **Audit-only concrete facts.** DDx MAY display Fizeau-returned
    harness/provider/model facts only on the exact run's audit detail. Those fields
    MUST be stored separately from DDx policy state and MUST NOT feed retry,
    review tier, reviewer pairing, candidate ranking, budget rate selection, or
    queue decisions.

12. **No provider/status proxy.** DDx MUST NOT proxy, cache, or persist Fizeau's
    provider catalog, quota/headroom, health, or route-candidate read models.
    DDx MAY link to a configured Fizeau diagnostics surface and MAY display the
    per-run public `RoutingActual` audit fields. It MUST NOT synthesize missing
    provider status from attempt metrics or native logs.

13. **Unified command surface.** `ddx runs metrics` MUST report DDx-recorded
    usage and cost aggregates with time-window, bead, role, and machine-readable
    output options. It reads FEAT-010 run records; it does not query provider
    runtimes or Fizeau catalogs. Fizeau-owned provider diagnostics remain a
    Fizeau surface.

14. **Legacy-record migration.** Historical DDx usage rows may be read through
    a bounded migration adapter. Their source MUST be labeled `legacy`, unknown
    values MUST remain unknown, and they MUST never become routing input.
    Migration MUST NOT introduce a provider-native log reader into the current
    execution path.

### Non-Functional

- **Boundary enforcement:** An automated structural test MUST fail if the DDx
  Fizeau consumer imports a concrete harness adapter, provider-log parser,
  native provider-store reader, model pricing table, route catalog, or
  route-selection helper.
- **No fabrication:** Unknown usage, cost, quota, auth, or provenance remains
  unknown at storage, API, CLI, and UI layers.
- **Determinism:** Recomputing aggregates from the same run records produces
  identical totals and identical unknown counts.
- **Performance:** Aggregating 10,000 canonical run records for a 30-day window
  completes in under 500 ms on the reference test machine; this query performs
  no network, provider-runtime, native-log, or catalog access.
- **Privacy:** DDx stores only the typed projection and opaque artifact
  references. It does not copy provider transcripts or credentials into usage
  records.
- **Forward compatibility:** Unknown fields in versioned Fizeau projections
  round-trip without causing DDx to infer semantics or reject the run record.

## Operator Surfaces

### Run usage and cost

`ddx runs metrics` is the DDx-owned usage surface. At minimum it reports:

- run/attempt/bead counts;
- known input, cached-input, output, and total tokens;
- known/established `CostUSD` subtotal under the current contract, plus
  currency and cost-class groups only when a future pinned contract supplies
  them;
- count of runs with unknown usage or unknown cost;
- role and time-window breakdowns; and
- links to the underlying DDx run and opaque Fizeau `SessionLogPath` values.

Human output renders unknown as `—` and includes an explicit incomplete-data
count. Machine-readable output uses an explicit known/unknown representation,
not `0`, an empty string, or an undocumented sentinel.

### Provider and routing status

Provider availability, authentication, quota/headroom, model catalogs, and
route candidates are Fizeau-owned observability. DDx does not expose or proxy
that information. FEAT-008 provides an external handoff to Fizeau diagnostics
and renders only per-run public terminal audit facts. DDx attempt metrics never
augment or override Fizeau's routing state.

## User Stories

### US-140: Developer Inspects Run Usage

**As a** developer reviewing an agent-backed run
**I want** to see the usage and cost Fizeau reported beside DDx repository
evidence
**So that** I can audit resource consumption without DDx parsing provider logs

**Acceptance Criteria:**

- Given Fizeau returns public usage/cost fields, when the layer-1 run is
  persisted, then every known field and its source plus the opaque
  `SessionLogPath` round-trip through the canonical run record.
- Given Fizeau reports an unknown cost, when I inspect the run, then cost is
  shown as unknown rather than zero or a DDx estimate.
- Given an opaque native-session-log path is present, then DDx stores the
  reference without opening or interpreting the target.

### US-141: Operator Enforces a Work Budget

**As an** operator running a bounded drain
**I want** every agent-backed operation to contribute to one cost ledger
**So that** DDx can stop new work at the configured budget without choosing a
provider or model

**Acceptance Criteria:**

- Given implementation, review, repair, and artifact-generation child runs,
  when DDx computes the drain cost, then all known Fizeau-reported billable
  amounts are included under the same accounting rules.
- Given some child runs have unknown cost, then the budget view reports the
  known subtotal and unknown-run count; it does not claim the subtotal is a
  complete total.
- Given the FEAT-010 cost stop fires, then no route, provider, or model failure
  is synthesized.

### US-142: Operator Preserves Routing Authority

**As an** operator supplying an explicit runtime constraint
**I want** DDx to pass it through without interpretation
**So that** Fizeau remains the sole concrete routing authority

**Acceptance Criteria:**

- Given I supply `MaxPower`, harness, provider, model, or public `Policy`, then
  every retry and review request contains the exact same operator value.
- Given DDx raises `MinPower`, then no concrete constraint is added, removed,
  normalized, or rewritten.
- Given Fizeau returns a public incompatibility error, then DDx records it and
  requests operator action without inspecting or substituting the constraint.

### US-143: Analyst Compares DDx Work Costs

**As an** analyst evaluating execution economics
**I want** deterministic aggregates by bead, role, artifact, and time window
**So that** I can compare DDx work without turning historical route facts into
execution policy

**Acceptance Criteria:**

- Given the same canonical run corpus, two aggregate reads return the same
  totals and unknown counts.
- Given a historical run record contains concrete route audit facts, its exact
  run detail may display them, but list filters, aggregates, and execution
  requests cannot consume them.
- Given 10,000 fixture run records, a 30-day aggregate completes within the
  stated performance bound without catalog or provider access.

### US-144: Operator Distinguishes Runtime and Repository Evidence

**As an** operator auditing an execution
**I want** Fizeau projections and DDx-owned facts labeled by their authority
**So that** I do not mistake DDx aggregation for provider or routing evidence

**Acceptance Criteria:**

- Given a usage, cost, actual-power, or concrete route audit fact came from the
  public Fizeau final data, then the run view labels it as Fizeau-returned and
  shows reported/estimated provenance only when the public field supplies it;
  current cost provenance remains unknown.
- Given elapsed attempt time, base/result revisions, checks, or landing state
  came from DDx, then the run view labels it as DDx-owned evidence.
- Given no authoritative value exists, then the view reports unknown and does
  not substitute a DDx estimate or a value inferred from another field.

### US-145: Bead-Attempt Metrics Are Captured Automatically

**As a** developer reviewing bead execution history
**I want** runtime metrics recorded for every attempt without manual
instrumentation
**So that** attempts remain comparable and cost remains visible

**Acceptance Criteria:**

- Given `ddx try` completes through Fizeau, then its attempt record links every
  child run and records DDx elapsed time, base revision, result revision when
  present, checks, and landing/preservation disposition.
- Given child runs expose typed usage/cost projections, then attempt and bead
  summaries aggregate those projections without reading a provider transcript.
- Given an attempt has no trustworthy usage projection, then the record remains
  valid and explicitly reports unknown usage.

## Test Strategy

Tests use a contract fake implementing the public Fizeau interface. Fixtures
contain typed Fizeau projections, not Claude/Codex/Gemini output or native log
formats.

| Invariant | Required test |
|---|---|
| Typed usage/cost fields persist without loss | `TestFizeauUsageProjectionRoundTrip` |
| DDx does not parse provider output or native stores | `TestUsageConsumerHasNoProviderParserOrNativeStoreDependency` |
| DDx has no catalog, pricing-table, or route-helper dependency | `TestUsageConsumerHasNoCatalogPricingOrRouteDependency` |
| Retry/review changes abstract capability only | `TestRetryAndReviewChangeMinPowerOnly` |
| Operator `MaxPower` and concrete pins remain immutable | `TestOperatorRuntimeConstraintsPassThroughUnchanged` |
| Concrete route facts are audit-only | `TestConcreteRouteFactsCannotAffectDDxPolicy` |
| Unknown usage and cost are not coerced | `TestUnknownUsageAndCostRemainUnknown` |
| All agent-backed roles share one ledger | `TestAgentBackedRolesShareUsageAccounting` |
| Budget consumes typed cost and reports unknown coverage | `TestBudgetAccountingUsesTypedFizeauCost` |
| Attempt metrics link repository and child-run evidence | `TestAttemptRuntimeMetricsLinkCanonicalRuns` |
| Fizeau and DDx facts retain distinct authority labels | `TestRunMetricsPreserveAuthorityLabels` |
| Aggregate path performs no external acquisition | `TestRunMetricsAggregateIsOfflineAndDeterministic` |

Validation commands:

```bash
cd cli && go test ./internal/agent/... ./internal/agentmetrics/... ./cmd/...
lefthook run pre-commit
```

Package paths may be narrowed to the final owning packages when the FEAT-010
run substrate lands, but every named test remains mandatory. TP-014 and SD-014
must be revised to this boundary before implementation work that cites them is
execution-ready; their former direct-provider parsers, native-log adapters,
model-pricing table, and DDx routing tests are migration debt, not acceptance
authority.

## Dependencies and Consumers

- **CONTRACT-003 (upstream Fizeau):** owns current public final fields, future
  projection extensions, provider-native acquisition, concrete catalogs, and
  routing behavior.
- **FEAT-006:** owns the DDx request/terminal consumer boundary and opaque
  passthrough envelope.
- **FEAT-010:** owns canonical run records, attempt/drain lifecycle, retry
  scheduling, and budget-stop policy.
- **FEAT-002 and FEAT-008:** may expose DDx usage aggregates, per-run public
  route audit facts, and an external Fizeau diagnostics handoff; they do not
  acquire, proxy, cache, or synthesize provider signals.
- **FEAT-016:** consumes canonical FEAT-014 aggregates for bead- and
  feature-level process metrics.
- **ADR-024:** may consume FEAT-014 cost fields for limits; it must not use
  route facts, provider identity, or model identity as DDx policy input.

## Implementation Strategy

### Phase 1 — Contract gate and deletion inventory

- Consume v0.14.50's existing public fields without inventing missing semantics;
  pin a later CONTRACT-003 revision before implementing any required richer
  usage/cost/runtime projection or provenance semantics.
- Inventory and mark for deletion every DDx provider-output parser, native-log
  or provider-cache reader, auth/quota probe, concrete model pricing table,
  catalog query, and route-selection consumer.
- Add the structural boundary tests before changing runtime behavior.

### Phase 2 — Canonical typed consumer

- Decode the public Fizeau projection once at the FEAT-006 boundary.
- Persist it in the FEAT-010 layer-1 record with opaque artifact references.
- Preserve operator-owned `MaxPower` and concrete constraints byte-for-byte;
  ensure retry/review code can change only `MinPower` under the policy above.

### Phase 3 — Aggregation and budget observability

- Implement deterministic offline aggregates over canonical run records.
- Wire FEAT-010 cost stops to typed amount/cost-class fields and explicit
  unknown coverage.
- Project attempt, bead, drain, and artifact-generation summaries without a
  second usage store.

### Phase 4 — Surface migration and legacy retirement

- Point CLI, server, and UI usage views at canonical aggregates.
- Replace DDx provider/status projections with an external handoff to Fizeau
  diagnostics and per-run terminal audit fields.
- Migrate historical DDx rows with explicit `legacy` provenance, then remove
  all current-path provider parsing and routing-signal acquisition code.

## Out of Scope

- Concrete harness/provider/model selection, ranking, fallback, or review
  pairing
- Fizeau model, provider, or harness catalogs
- Direct Claude, Codex, Gemini, OpenCode, or native-runtime integration in DDx
- Provider-native log, cache, auth, quota, billing, rate-limit, or process-state
  discovery and parsing
- DDx-maintained concrete model pricing tables or provider cost estimation
- Provider transcript storage or a DDx-native session schema
- Provider catalog, health, quota, route-candidate, or dashboard proxy semantics
- Workflow-specific interpretation of usage or cost beyond FEAT-010 budgets
