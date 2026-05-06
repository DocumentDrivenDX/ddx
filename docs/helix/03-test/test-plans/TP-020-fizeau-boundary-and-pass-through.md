---
ddx:
  id: TP-020
  depends_on:
    - FEAT-001
    - FEAT-006
---
# Test Plan: DDx Boundary Over Fizeau Routing

> **Supersedes the old DDx-side routing planner plan.** DDx no longer owns
> model normalization, fuzzy matching, candidate ranking, or route selection.
> Those behaviors live in Fizeau. This plan now only covers DDx's boundary
> guarantees: pass strings through unchanged, never normalize them locally, and
> never route on model catalog details.

## Scope

Validate that DDx forwards operator-supplied routing constraints unchanged and
does not perform any DDx-side model matching, catalog ranking, or route
selection.

## Test Cases

### Raw Constraint Passthrough

- `TestRunForwardsModelStringUnchanged`
- `TestRunForwardsHarnessAndProviderUnchanged`
- `TestTryForwardsModelStringUnchanged`
- `TestWorkForwardsPassthroughPinsUnchanged`

### No DDx-Side Normalization

- `TestRunDoesNotNormalizeModelRef`
- `TestTryDoesNotNormalizeModelRef`
- `TestRunPreservesLiteralModelStringQwen36`
- `TestTryPreservesLiteralModelStringQwen36`

### No DDx-Side Routing

- `TestWorkDoesNotChooseRouteFromModel`
- `TestRunDoesNotFallbackToDifferentModel`
- `TestRunDoesNotInspectCatalogForRouteSelection`
- `TestTryDoesNotFuzzyMatchProviderOrModel`

### Config Boundary

- `TestConfigSeparatesExecutionAndRoutingDefaults`
- `TestConfigRejectsAgentRoutingDefaults`
- `TestConfigLeavesFizeauRoutingToUpstream`

## Fixtures

- operator input fixture with literal model strings such as `qwen36`,
  `Qwen3.6-35B-A3B`, and `vendor/qwen36`
- passthrough-pins fixture covering `--harness`, `--provider`, and `--model`
- execution-config fixture covering DDx-owned queue, retry, and power knobs

## Exit Criteria

- DDx test coverage proves raw passthrough strings survive unchanged from CLI
  parsing to execution request construction.
- DDx test coverage proves no local normalization, fuzzy matching, or route
  selection occurs in `run`, `try`, or `work`.
- Fizeau routing behavior is covered in Fizeau-owned tests, not in DDx.

