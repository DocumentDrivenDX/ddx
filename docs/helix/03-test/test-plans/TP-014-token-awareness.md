---
ddx:
  id: TP-014
  depends_on:
    - FEAT-014
    - FEAT-006
    - FEAT-010
    - SD-014
    - TD-006
---
# Test Plan: Agent Usage, Cost, and Runtime Projections

**Design authority:** [`SD-014`](../../02-design/solution-designs/SD-014-token-awareness.md)
defines the Fizeau consumer boundary, canonical run projection, aggregation,
budget input, and negative architectural guards validated by this plan.

## Objective

Verify that DDx persists and aggregates only public resource facts returned by
Fizeau, keeps missing semantics unknown, and correlates those facts with
DDx-owned work and repository evidence. Tests must prove that DDx neither
implements a provider runtime nor uses concrete routing evidence to steer a
later request.

## Test Fixture

Unit and integration tests use a fake of the pinned public Fizeau service
contract:

```go
type fakeFizeau struct {
    ExecuteFn func(
        context.Context,
        fizeau.ServiceExecuteRequest,
    ) (<-chan fizeau.ServiceEvent, error)
    Requests []fizeau.ServiceExecuteRequest
}
```

The fake records the request and returns either:

- a public immediate error with no event stream; or
- opaque non-final events followed by a public final event carrying
  `fizeau.ServiceFinalData`.

The fixture does not emulate Claude, Codex, a provider, a catalog, routing,
fallback, native logs, quota, authentication, pricing, or a session/tool loop.
A final fixture may populate `Usage`, `CostUSD`, `DurationMS`, `Warnings`,
`SessionLogPath`, and `RoutingActual`. Repository side effects are produced by
a test callback in the temporary DDx worktree, not by a virtual harness.

## Layer 1: Public Contract Consumption

These tests are fast and in-process.

| ID | Test | What It Proves |
|---|---|---|
| F-01 | `TestFizeauUsageProjectionRoundTrip` | Every current public `Usage` dimension and its presence state survive the canonical run record |
| F-02 | `TestFizeauFinalResourceFieldsRoundTrip` | `CostUSD`, `DurationMS`, `Warnings`, opaque `SessionLogPath`, and audit-only `RoutingActual` survive persistence without provider reconstruction |
| F-03 | `TestFizeauImmediateErrorHasUnknownResourceProjection` | An immediate `Execute` error creates no invented final event and records usage/cost coverage as unknown |
| F-04 | `TestFizeauGenericFinalErrorDoesNotClassifyResources` | Generic final `Error` text cannot create cause, stage, quota, retryability, cost, or usage semantics |
| F-05 | `TestUnknownUsageAndCostRemainUnknown` | Missing usage dimensions and ambiguous/absent current cost are not coerced to known zero |
| F-06 | `TestExplicitZeroUsageRemainsKnownZero` | A public usage pointer containing zero remains distinguishable from a missing dimension |
| F-07 | `TestCurrentCostHasNoInventedClassOrProvenance` | A current `CostUSD` observation does not acquire provider-reported, estimated, subscription, local, or metered labels |
| F-08 | `TestWarningsRemainOutcomeData` | Warning code/message data round-trip but cannot become provider-health, auth, quota, or route state |
| F-09 | `TestSessionLogPathRemainsOpaque` | Missing, unreadable, or adversarial paths do not trigger file access or change usage/cost values |
| F-10 | `TestRoutingActualStoredInAuditEnvelopeOnly` | Returned concrete route facts are isolated from DDx request and policy state |
| F-11 | `TestDurationAndAttemptElapsedRetainDistinctAuthorities` | Fizeau `DurationMS` and DDx attempt elapsed time are recorded separately |
| F-12 | `TestStreamWithoutFinalHasUnknownResourceProjection` | A stream ending without valid final data is incomplete and does not fall back to logs or output parsing |
| F-13 | `TestUsageSourceFreshnessAndEvidenceRoundTrip` | Current public `Source`, `Fresh`, `CapturedAt`, and per-source usage/warning evidence persist without DDx inference |

Representative fixture:

```go
input, output, cacheRead := 1200, 300, 800
final := fizeau.ServiceFinalData{
    Status:     "success",
    DurationMS: 4200,
    Usage: &fizeau.ServiceFinalUsage{
        InputTokens:     &input,
        OutputTokens:    &output,
        CacheReadTokens: &cacheRead,
    },
    CostUSD:        0.17,
    Warnings:       []fizeau.ServiceFinalWarning{{Code: "partial-usage"}},
    SessionLogPath: "sessions/run-1.jsonl",
    RoutingActual: &fizeau.ServiceRoutingActual{
        Harness: "opaque-harness", Provider: "opaque-provider",
        Model: "opaque-model", Power: 7,
    },
}
```

Assertions read only the public final struct. No fixture contains provider
stdout, provider JSONL, a provider cache, a price list, or catalog data.

## Layer 2: DDx Evidence Correlation

These tests use temporary git repositories and the Fizeau contract fake.

| ID | Test | What It Proves |
|---|---|---|
| E-01 | `TestAttemptRuntimeMetricsLinkCanonicalRuns` | An attempt links each child run and its public resource projection |
| E-02 | `TestRunMetricsPreserveAuthorityLabels` | Fizeau duration/resource fields remain separate from DDx revisions, checks, review, and landing facts |
| E-03 | `TestSuccessfulFizeauRunCanFailRepositoryGate` | Public operation success does not override a failing DDx repository gate |
| E-04 | `TestFailedFizeauRunRetainsReportedUsage` | Resource fields survive a failed operation or failed DDx work outcome |
| E-05 | `TestResultRevisionDoesNotDependOnSessionLog` | DDx derives repository evidence from the worktree and git, never native-log contents |
| E-06 | `TestAgentBackedRolesShareUsageAccounting` | Implementer, reviewer, repair, reframer, decomposer, and artifact runs use the same canonical projection |
| E-07 | `TestLegacyUsageMigrationPreservesUnknowns` | Bounded legacy reads are labeled `legacy` and do not fabricate missing dimensions |

Temporary-repository tests capture base revision, result revision, changed
paths, gate results, review verdict, and landing/preservation disposition.
Changing only those DDx facts must not mutate the copied Fizeau projection;
changing only the public projection must not rewrite repository evidence.

## Layer 3: Aggregation And Budget Inputs

| ID | Test | What It Proves |
|---|---|---|
| A-01 | `TestRunMetricsAggregateKnownUsageByDDxDimensions` | Known usage aggregates by run, attempt, bead, role, artifact, drain, project, and time window |
| A-02 | `TestRunMetricsReportUnknownCoverage` | Missing usage/cost increments explicit unknown counts and is not silently dropped |
| A-03 | `TestRunMetricsAggregateIsOfflineAndDeterministic` | The same canonical records and filters produce identical results without external acquisition |
| A-04 | `TestRunMetricsDoNotGroupByRoutingActual` | Harness/provider/model/fallback/failure-class values cannot become an aggregate dimension |
| A-05 | `TestRunMetricsKeepFizeauAndDDxDurationsSeparate` | Operation duration and attempt elapsed time are never summed as one metric |
| A-06 | `TestBudgetAccountingUsesTypedFizeauCost` | The budget ledger consumes established current cost observations and reports ambiguous/unknown coverage |
| A-07 | `TestBudgetNeverEstimatesCostFromUsageOrRoute` | Token counts and concrete route identity cannot synthesize a price |
| A-08 | `TestBudgetIncludesEveryAgentBackedRole` | Review, repair, recovery, and artifact-generation operations contribute under the same rules |
| A-09 | `TestRunMetricsTenThousandRecordsUnderBudget` | A 10,000-record, 30-day aggregate completes under 500 ms on the reference test machine |

`A-03` instruments all external boundaries. The test fails if aggregation
attempts a network request, provider process, Fizeau diagnostics/catalog call,
native-log read, provider-cache read, or pricing lookup.

Cost assertions are deliberately conservative. Current `CostUSD` does not
authorize assertions about cost class, provenance, estimate status, or a known
free run when zero/omission is ambiguous. Those cases remain unknown until a
future pinned contract represents them explicitly.

## Layer 4: Route-Neutral Retry And Review

| ID | Test | What It Proves |
|---|---|---|
| P-01 | `TestRetryAndReviewChangeMinPowerOnly` | Stronger review intent, or a distinct new DDx attempt after capability-sensitive evidence, raises only abstract `MinPower` |
| P-02 | `TestOperatorRuntimeConstraintsPassThroughUnchanged` | Operator `MaxPower`, harness, provider, model, and public `Policy` are copied exactly to every new operation; no `Profile` field is invented |
| P-03 | `TestDDxDoesNotOriginateConcreteRoute` | Without operator pins, concrete request fields remain unset regardless of prior usage/cost/outcome |
| P-04 | `TestConcreteRoutingActualCannotAffectDDxPolicy` | Varying concrete harness/provider/model route facts cannot change retry, escalation, review strength, warnings, budget, or queue decisions |
| P-04a | `TestReturnedAbstractPowerOnlyRaisesCapabilityAttemptOrReview` | Returned abstract `Power` may inform only a stronger review or a distinct new attempt after capability-sensitive DDx evidence; it never selects a route or escalates infrastructure failure |
| P-05 | `TestUsageAndCostCannotChooseRoute` | Usage, cost, duration, and warnings cannot select a harness/provider/model or query a catalog |
| P-06 | `TestConstraintConflictReturnsToOperator` | A raised `MinPower` that conflicts with an immutable operator constraint is not repaired by relaxing or replacing the constraint |
| P-07 | `TestStrongerReviewDoesNotCompareConcreteRoutes` | Reviewer strength is expressed by `MinPower`; implementer/reviewer route identity is not compared or diversified by DDx |
| P-08 | `TestInfrastructureFailuresNeverRaiseMinPower` | Transport, route, quota, authentication, setup, operator-action, and generic failures keep the abstract power unchanged |

For `P-04`, run the same DDx input twice while changing only the fake's
`RoutingActual`. Compare serialized follow-up requests and DDx decisions; they
must be identical. The audit subrecord may differ.

## Layer 5: Structural Negative Guards

Structural tests inspect imports, production call graphs, configuration keys,
and command/API registration. They fail on prohibited DDx ownership.

| ID | Test | What It Proves |
|---|---|---|
| S-01 | `TestUsageConsumerHasNoProviderParserOrNativeStoreDependency` | Consumer code has no provider-output, native-log, cache, hook, or transcript parser |
| S-02 | `TestUsageConsumerHasNoCatalogPricingOrRouteDependency` | Consumer/metrics code has no model price table, Fizeau catalog client, route helper, or fallback helper |
| S-03 | `TestUsageConsumerHasNoConcreteHarnessRegistryArgs` | DDx does not add Claude/Codex/Gemini/OpenCode output flags or invocation rules |
| S-04 | `TestUsageConsumerHasNoQuotaAuthOrProviderHealthProbe` | DDx does not probe provider quota, auth, rate limit, billing, or health state |
| S-05 | `TestDDxHasNoProviderStatusProxySurface` | No DDx provider dashboard, `/api/providers`, catalog cache, or provider-status proxy is registered |
| S-06 | `TestRoutingActualCannotReachPolicyPackages` | Policy, budget, retry, review, queue, and status packages cannot import/read the concrete audit projection |
| S-07 | `TestMetricsCommandReadsCanonicalRunsOnly` | `ddx runs metrics` depends on the FEAT-010 run store, not Fizeau diagnostics or provider sources |
| S-08 | `TestFutureResourceFieldsRemainContractGated` | Cost provenance/class/currency, stable session identity, general cause/stage, final retry time, and continuation cannot appear without an explicit pinned-contract gate |

The negative checks should target stable package boundaries or forbidden
dependencies, not brittle prose searches alone. A small source scan may
supplement import/call-graph checks for forbidden command/API registration.

## Layer 6: Optional Real-Fizeau Conformance

These tests use a configured Fizeau service and skip when unavailable. They do
not connect DDx directly to a provider or concrete harness.

| ID | Test | What It Proves |
|---|---|---|
| I-01 | `TestIntegration_UsageProjectionThroughFizeau` | One public final projection reaches the canonical DDx run record end to end |
| I-02 | `TestIntegration_ImmediateFizeauErrorHasNoInventedUsage` | An immediate service error remains an incomplete resource projection |
| I-03 | `TestIntegration_SessionLogReferenceRemainsOpaque` | Real opaque session-log references are recorded without being read by metrics |

Fizeau's provider adapters, usage acquisition, routing, native logs, and
process behavior are verified in Fizeau's CONTRACT-003 conformance suite.
DDx integration tests verify only the consumer boundary.

## Future-Contract Cases

Tests for stable session or cost provenance beyond current usage source
evidence, generalized currency, cost class, estimate status, general terminal
cause/stage, final-event `RetryAfter`, or continuation must remain skipped or
absent until CONTRACT-003 publishes the field and DDx pins a compatible
version. A test fixture must not add a shadow DDx field to simulate an upstream
contract that does not exist.

When a future contract lands, migration tests must prove old records retain
unknown values and no backfill reads `RoutingActual`, provider logs, or caches.

## Pass Criteria

The plan passes when:

1. every named non-integration test above passes;
2. optional real-Fizeau tests pass when the service is configured and otherwise
   skip with an explicit reason;
3. aggregation is deterministic, offline, and reports unknown coverage;
4. structural guards prove the absence of provider-runtime, catalog, pricing,
   status-proxy, and route-policy dependencies; and
5. repository gates are green.

Commands:

```bash
cd cli && go test ./internal/agent/... ./internal/agentmetrics/... ./cmd/...
lefthook run pre-commit
```

During staged implementation, package paths may be narrowed to the final
FEAT-010 owners, but the named tests and boundary assertions remain mandatory.
