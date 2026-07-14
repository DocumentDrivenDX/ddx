---
ddx:
  id: SD-014
  depends_on:
    - FEAT-014
    - FEAT-006
    - FEAT-010
    - ADR-001
    - ADR-024
---
# Solution Design: Agent Usage, Cost, and Runtime Projections

## Purpose

DDx records the resource facts that Fizeau returns for an agent operation and
correlates them with DDx-owned work and repository evidence. Fizeau is the full
agent runtime. It owns harnesses, providers, models, concrete routing,
fallback, native logs, quota and authentication signals, and usage acquisition.
DDx is the work tracker and git-aware executor. It does not recreate any part
of the runtime to fill gaps in Fizeau's public result.

This design is authoritative for TP-014. It replaces the former design based
on Claude/Codex registry arguments, provider-output parsing, provider caches,
quota probes, and a DDx model-price table.

## Scope

In scope:

- decoding current public Fizeau `Execute` immediate errors and final data;
- persisting `Usage`, `CostUSD`, `DurationMS`, `Warnings`, an opaque
  `SessionLogPath`, and audit-only `RoutingActual` beside a DDx run;
- correlating those fields with bead, attempt, role, worktree, revision, gate,
  review, landing, and preservation evidence;
- deterministic aggregation by DDx work dimensions;
- budget accounting from authoritative values that the pinned contract can
  represent; and
- route-neutral retries and stronger reviews through abstract `MinPower`.

Out of scope:

- provider or harness command-line arguments and output formats;
- provider stdout/stderr, native session logs, caches, billing APIs, status
  lines, hooks, credentials, quota, or rate-limit inspection;
- model pricing, provider catalogs, availability dashboards, provider APIs,
  route comparison, route efficacy, or route selection;
- inferring missing usage, cost, cost provenance/class/currency, terminal cause,
  or terminal stage beyond the public fields; and
- choosing or changing a harness, provider, model, public `Policy`, or
  operator-owned `MaxPower`.

## Ownership Boundary

| Concern | Owner | DDx behavior |
|---|---|---|
| Harness/provider/model discovery and concrete routing | Fizeau | Sends no DDx-originated concrete route and does not query a catalog |
| Provider invocation, tool/session loop, fallback, native logs, auth, quota, and usage acquisition | Fizeau | Consumes only the public service result |
| Explicit harness/provider/model/public `Policy` pins and `MaxPower` | Operator | Forwards each value unchanged; never originates, relaxes, rewrites, or removes it; current v0.14.50 has no per-request `Profile` |
| Retry and review strength | DDx | May raise abstract `MinPower` for stronger review intent, or on a distinct new DDx attempt after capability-sensitive evidence; never for infrastructure/route failure |
| Beads, attempts, worktrees, revisions, gates, review, and landing | DDx | Records and evaluates repository evidence |
| Per-operation resource projection | Fizeau | DDx preserves current public fields without reconstruction |
| Work-level usage/cost aggregation and budget stops | DDx | Aggregates public per-run values by DDx-owned dimensions |

The invariant is:

> DDx may ask for more capability by raising `MinPower`. Fizeau decides which
> harness, provider, and model satisfy the request.

`RoutingActual` is evidence of what happened. It is not a DDx policy input.

## Current Fizeau Contract

The design targets the pinned public in-process contract:

```go
Execute(
    ctx context.Context,
    req fizeau.ServiceExecuteRequest,
) (<-chan fizeau.ServiceEvent, error)
```

`Execute` can fail before it returns an event stream. DDx persists that public
immediate error as the operation outcome. It does not invent final data, parse
the error message for provider state, or try another concrete route. The
current public `*fizeau.NoViableProviderForNow` type may provide a
`RetryAfter`; DDx may use that value only as the queue-level waiting signal
defined by FEAT-006 and FEAT-010.

When an event stream is returned, DDx waits for the public final event and
decodes `ServiceFinalData`. FEAT-006 owns complete final-outcome handling. This
design consumes these resource and audit fields:

- `Usage`
- `CostUSD`
- `DurationMS`
- `Warnings`
- `SessionLogPath`
- `RoutingActual`

`Status`, `ExitCode`, the generic `Error`, and `FinalText` remain part of the
same final envelope and are stored by the FEAT-006/FEAT-010 consumer. The
generic final `Error` is opaque, unclassified evidence. Resource accounting
must not parse it to infer cause, stage, provider health, quota, retryability,
or missing cost.

Non-final service events are not a usage source. DDx may forward their public
display projection while live, but it does not interpret provider, tool,
transcript, routing, or token details from them.

## Canonical Run Projection

The FEAT-010 layer-1 run record is the canonical DDx store. Its resource
projection contains:

```text
fizeau_resource:
  duration_ms
  usage                 # public ServiceFinalData.Usage, presence preserved
  cost_usd              # public ServiceFinalData.CostUSD observation
  warnings              # public ServiceFinalData.Warnings, verbatim
  session_log_path      # opaque string; never opened by accounting code
  routing_actual        # isolated audit-only subrecord
  projection_contract   # pinned Fizeau contract/version identifier

ddx_evidence:
  run_id, parent_run_id, bead_id, attempt_id, role
  requested_min_power
  operator_passthrough  # exact MaxPower/pins supplied by the operator
  base_revision, result_revision
  repository_checks, review_verdict
  landing_or_preservation_disposition
```

The persisted representation must preserve presence separately from a numeric
value wherever the public type supports presence. A missing usage projection
or missing token dimension is unknown, not zero. An explicitly reported zero
token dimension is a known zero.

The current `CostUSD` field does not publish cost class, currency metadata,
source provenance, estimate provenance, or an explicit known/unknown marker.
DDx records the returned field but does not attach those missing semantics. In
particular, it must not label a cost as provider-reported, estimated,
subscription, local, or metered. If the current wire representation cannot
distinguish an omitted amount from an explicit zero, the accounting
completeness state is unknown; DDx must not upgrade that zero to a known free
run. A future pinned contract may remove that ambiguity.

`DurationMS` is Fizeau's operation duration. DDx attempt elapsed time is a
separate DDx-owned fact and may span more than one Fizeau operation plus git,
test, review, and landing work.

`Warnings` are copied as public outcome data. DDx does not turn warning text
into quota, authentication, provider-health, or routing state.

`SessionLogPath` is an opaque reference. DDx may render the reference or attach
it to evidence, but usage, cost, retry, review, and replay code must not open
the path.

`RoutingActual` is stored in a separately typed audit subrecord. DDx may show
the concrete harness/provider/model facts only on that run's audit detail;
they cannot drive aggregation, filters, budgets, warnings, or queue policy.
The one policy-bearing field is the returned abstract `Power`: together with
capability-sensitive DDx outcome evidence, it may establish a higher
`MinPower` for a distinct new attempt or stronger review. It never authorizes
concrete route selection, and infrastructure/route/quota/authentication/setup/
operator-action/generic failures still cannot raise power.

## Immediate Errors And Incomplete Runs

An immediate `Execute` error has no final resource projection. DDx still
creates a canonical run record containing:

- the DDx request and correlation facts;
- the exact operator passthrough envelope;
- the immediate public error identity/evidence;
- DDx repository state known at the time; and
- explicit unknown usage and cost coverage.

A stream that ends without a valid final event is also incomplete. DDx does
not scan captured output or `SessionLogPath` to recover resource values.

Operation success and DDx work success remain separate. A successful Fizeau
final event can still correspond to a failed repository gate or a review block.
Conversely, resource fields are retained even when the operation or DDx work
outcome fails.

## Aggregation

`ddx runs metrics` reads canonical run records only. It supports deterministic
grouping by DDx-owned dimensions such as project, run, attempt, bead, role,
artifact, drain, and time window. It reports:

- sums for each known public usage dimension;
- known `CostUSD` subtotal where the current record establishes an amount;
- operation duration and separate DDx attempt elapsed time;
- total run count; and
- unknown/incomplete usage and cost counts.

Unknown values are not converted to zero and are not silently dropped. A
subtotal accompanied by unknown runs is labeled incomplete. Values with
different units or future currencies are never combined without an explicit
upstream contract.

The aggregate path performs no network call, provider invocation, Fizeau
catalog request, diagnostics request, native-log access, or price lookup. The
same records and filters produce the same totals and unknown counts.

`RoutingActual` is intentionally excluded from aggregate grouping, list
filters, and all quality, efficacy, and policy calculations. It is readable
only on the exact run's audit detail.

## Budget Accounting

FEAT-010 owns when a budget stops new work. FEAT-014 supplies only the
resource ledger:

1. Every agent-backed role records the same public Fizeau projection.
2. Known current cost observations contribute to the known subtotal.
3. Ambiguous or absent cost observations increment unknown coverage.
4. DDx reports both before applying the configured budget policy.

DDx never estimates missing cost from token counts or `RoutingActual`. It never
uses a model price table. Cost class, provenance, estimate status, and currency
beyond the current `CostUSD` field remain unknown until a compatible Fizeau
contract explicitly supplies them.

## Retry, Review, And Operator Constraints

Resource accounting cannot select a route. Stronger review intent may raise
abstract `MinPower`. A failed implementation may raise it only on a distinct
new DDx attempt after capability-sensitive evidence under ADR-024.
Infrastructure, transport, route, quota, authentication, setup,
operator-action, and generic failures keep power unchanged. No floor may be
derived from concrete `RoutingActual` route identity, a model price, quota, or
a catalog. Returned abstract `RoutingActual.Power` may inform only the stronger
review/distinct capability-sensitive-attempt rule above.

Operator-provided `MaxPower`, harness, provider, model, and public `Policy` are
immutable passthrough fields. A new operation copies them exactly. If a raised
`MinPower` conflicts with an operator constraint, Fizeau returns the public
outcome and DDx requests operator action. DDx does not loosen the constraint or
substitute a route.

## Future Contract Gate

The following are unknown under the current contract and must not appear as
implemented DDx semantics:

- stable session or cost provenance beyond the current usage source evidence
  and opaque `SessionLogPath`;
- cost source, currency generalization, cost class, or estimate status;
- a general typed terminal cause or stage;
- final-event `RetryAfter`;
- usage/cost freshness guarantees not present in the pinned contract; and
- continuation or active-session query semantics.

DDx may add these fields only after CONTRACT-003 publishes them and DDx pins a
compatible Fizeau release. Migration must preserve old unknown values rather
than backfilling them from concrete route identity or native logs.

## Structural Boundaries

The implementation must have automated negative guards proving that the DDx
Fizeau consumer and metrics packages do not depend on:

- Claude, Codex, Gemini, OpenCode, or other concrete harness adapters;
- provider-output or native-session-log parsers;
- provider cache, quota, auth, billing, or status readers;
- a model price table;
- a Fizeau provider/catalog/status proxy;
- route selection, fallback, ranking, comparison, or efficacy helpers; or
- `RoutingActual` from any DDx policy package.

Tests use a fake of the public Fizeau service contract. They supply immediate
errors or public final events and verify DDx records only the contract fields
plus its own repository evidence.

## Legacy Data

A bounded migration adapter may read old DDx activity rows. It labels their
source `legacy`, preserves unknown dimensions, and never makes them execution
policy input. The adapter must not add a provider-native parser to the current
run path. New writes use only the canonical FEAT-010 run record.

## Verification

TP-014 defines the named contract, persistence, aggregation, budget,
route-neutrality, and structural tests. Required repository gates are:

```bash
cd cli && go test ./internal/agent/... ./internal/agentmetrics/... ./cmd/...
lefthook run pre-commit
```
