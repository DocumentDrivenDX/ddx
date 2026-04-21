---
ddx:
  id: TP-020
  depends_on:
    - FEAT-001
    - FEAT-006
---
# Test Plan: Agent Routing and Catalog Resolution

> **Migrated 2026-04-21.** The candidate planning, ranking, and harness-state
> probing test cases this plan used to cover (`TestAgentRoute*`,
> `TestAgentRouteTP020AcceptanceGroups`, the `catalog_test.go` BuildCandidatePlans
> matrix) tested a DDx-side routing planner (`cli/internal/agent/routing.go`)
> that was deleted in commit `00481155` once upstream
> `github.com/DocumentDrivenDX/agent` v0.8.0 shipped `DdxAgent.ResolveRoute`
> (agent-1a486c2e). Those tests now live in the upstream agent repo; DDx is a
> thin consumer via `svc.ResolveRoute` with coverage at the call-site level
> (`TestAgentRouteRequestFromProfile` / `TestAgentRunProfileFlag*` in cmd/).
>
> This plan is retained as a historical record of the DDx-side routing
> coverage. Any new DDx-side routing test case belongs in this file only if
> it exercises DDx-specific behavior (legacy script/virtual/HTTP dispatch in
> runner.go), not general routing semantics.

## Scope

Validate DDx intent-first agent routing, shared-catalog projection across
harness surfaces, harness-state-aware candidate planning, and the embedded
runtime boundary.

## Test Cases

### Request Normalization

- `TestAgentRouteRequestFromProfile`
- `TestAgentRouteRequestFromModelRef`
- `TestAgentRouteRequestFromExactPinFallback`
- `TestAgentRouteRequestHarnessOverrideWins`

### Catalog Projection

- `TestAgentRoutingResolvesProfileAcrossHarnessSurfaces`
- `TestAgentRoutingResolvesQwen3ToEmbeddedOnly`
- `TestAgentRoutingRejectsUnknownModelRef`
- `TestAgentRoutingSurfacesDeprecatedReplacementWarning`

### Candidate Planning and Rejection

- `TestAgentRoutingRejectsHarnessWithoutSurfaceMapping`
- `TestAgentRoutingRejectsUnsupportedEffort`
- `TestAgentRoutingRejectsPolicyRestrictedHarness`
- `TestAgentRoutingRejectsQuotaBlockedHarness`
- `TestAgentRoutingRejectsUnreachableHarness`

### Ranking

- `TestAgentRoutingCheapPrefersLowestCostHealthyCandidate`
- `TestAgentRoutingFastPrefersFastestViableCandidate`
- `TestAgentRoutingSmartPrefersHighestQualityCandidate`
- `TestAgentRoutingPrefersLocalWhenOtherwiseEquivalent`
- `TestAgentRoutingUsesStableTieBreaker`

### Embedded Boundary

- `TestAgentRoutingSelectsEmbeddedWithoutInspectingProviderDetails`
- `TestAgentRoutingPassesResolvedIntentToEmbeddedHarness`
- `TestAgentRoutingDoesNotDuplicateEmbeddedBackendPoolStrategy`

### CLI and Config

- `TestAgentRunProfileFlagRoutesWithoutHarness`
- `TestAgentRunModelRefFlagRoutesWithoutHarness`
- `TestAgentConfigDefaultProfileUsedWhenNoExplicitSelector`
- `TestAgentConfigForcedHarnessBypassesAutomaticSelection`

### Capabilities and Doctor

- `TestAgentCapabilitiesShowsEffectiveProfileMappings`
- `TestAgentCapabilitiesShowsExactPinSupport`
- `TestAgentDoctorReportsRoutingRelevantHarnessState`
- `TestAgentDoctorReportsEmbeddedDefaultBackendRoutability`

## Fixtures

- catalog fixture with:
  - embedded-only `qwen3`
  - shared `cheap`, `fast`, `smart`
  - deprecated alias with replacement
- harness-state fixture matrix covering:
  - healthy local embedded
  - healthy cloud codex
  - degraded cloud claude
  - quota-blocked harness
  - unreachable harness

## Exit Criteria

- DDx can route by `profile` or `model` without requiring `--harness`.
- Exact model refs such as `qwen3` deterministically choose the embedded path
  when only that surface exists.
- Candidate rejection and ranking are deterministic and test-covered.
- DDx capability and doctor surfaces expose routing-relevant state rather than
  only static harness defaults.
- The embedded runtime boundary is preserved: DDx chooses the harness; embedded
  `ddx-agent` chooses provider/backend details.
