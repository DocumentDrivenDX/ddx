---
ddx:
  id: AR-2026-04-04-exec-model
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-005
    - FEAT-006
    - FEAT-010
    - ADR-001
---
# Alignment Review: Execution Model (FEAT-010)

**Review Date**: 2026-04-04
**Scope**: execution model / metric substrate
**Status**: complete
**Review Epic**: `ddx-aa2d938a`
**Primary Governing Artifact**: `FEAT-010`

## Scope and Governing Artifacts

### Scope

- Generic execution substrate for `ddx exec`
- Metric convenience surface and storage shape
- Run-history durability, concurrency, and compatibility policy

### Governing Artifacts

- `docs/helix/01-frame/features/FEAT-001-cli.md`
- `docs/helix/01-frame/features/FEAT-005-artifacts.md`
- `docs/helix/01-frame/features/FEAT-006-agent-service.md`
- `docs/helix/01-frame/features/FEAT-010-executions.md`
- `docs/helix/02-design/architecture.md`
- `docs/helix/02-design/solution-designs/SD-005-metric-runtime-history.md`
- `docs/helix/02-design/technical-designs/TD-005-metric-runtime-history.md`
- `docs/helix/03-test/test-plans/TP-005-metric-runtime-history.md`
- `cli/cmd/metric.go`
- `cli/internal/metric/store.go`
- `cli/internal/metric/store_test.go`
- `cli/cmd/metric_acceptance_test.go`

## Intent Summary

- **Vision**: DDx keeps runtime evidence separate from declarative artifacts and exposes reusable, workflow-agnostic execution records.
- **Requirements**: execution definitions and immutable runs should live in repo-local storage, support raw logs plus structured results, and remain projection-friendly for specializations like metrics.
- **Features / Stories**: FEAT-001 already exposes `ddx exec` command shapes; FEAT-010 makes `ddx exec` the owning substrate and makes `ddx metric` only an optional wrapper.
- **Architecture / ADRs**: the architecture explicitly names `internal/exec` as artifact-generic and says metrics are projections over the shared execution store.
- **Technical Design**: TD-005 defines `.ddx/exec/definitions/` and `.ddx/exec/runs/` and explicitly rejects a separate `.ddx/metrics/` runtime store.
- **Test Plans**: TP-005 requires concurrency-safe writes, append-only history, and run-history inspection over the generic execution store.
- **Implementation Plans**: no repo plan currently reconciles the code to the FEAT-010 substrate; the current code still reflects the older metric-specific implementation.

## Planning Stack Findings

| Finding | Type | Evidence | Impact | Review Issue |
|---------|------|----------|--------|-------------|
| FEAT-010, architecture, SD-005, TD-005, and TP-005 agree on a generic execution substrate with metric projections layered on top. | aligned | `docs/helix/01-frame/features/FEAT-010-executions.md:64-77`, `docs/helix/02-design/architecture.md:121-140`, `docs/helix/02-design/solution-designs/SD-005-metric-runtime-history.md`, `docs/helix/02-design/technical-designs/TD-005-metric-runtime-history.md`, `docs/helix/03-test/test-plans/TP-005-metric-runtime-history.md` | The governing stack is coherent; the gap is in implementation, not requirements. | `ddx-f02e6f0e` |
| The current metric implementation owns a separate command surface and a separate `.ddx/metrics/` store. | divergent | `cli/cmd/metric.go:13-163`, `cli/internal/metric/store.go:24-38`, `cli/internal/metric/store.go:74-143`, `cli/internal/metric/store.go:192-303` | This is the wrong runtime shape for FEAT-010 and will continue to drift unless it is re-layered over `ddx exec`. | `ddx-f02e6f0e` |

## Implementation Map

- **Topology**: `cli/cmd/metric.go` is a dedicated Cobra subtree; `cli/internal/metric` owns the runtime/store implementation; there is no `cli/internal/exec` package yet.
- **Entry Points**: `newMetricCommand`, `metricStore`, `Validate`, `Run`, `Compare`, `History`, and `Trend`.
- **Test Surfaces**: `cli/internal/metric/store_test.go` and `cli/cmd/metric_acceptance_test.go` validate the metric-specific store and CLI, but they do not exercise a generic `ddx exec` surface.
- **Unplanned Areas**: `.ddx/metrics/definitions/` and `.ddx/metrics/history.jsonl` are treated as authoritative runtime storage even though FEAT-010 says that storage should live under `.ddx/exec/`.

## Acceptance Criteria Status

| Story / Feature | Criterion | Test Reference | Status | Evidence |
|-----------------|-----------|----------------|--------|----------|
| US-090 / FEAT-010 | Run a metric-backed execution and retain raw logs plus structured result data. | `cli/internal/metric/store_test.go:31-61`, `cli/cmd/metric_acceptance_test.go:38-56` | UNIMPLEMENTED | The tests exercise a metric-only command, not `ddx exec run <definition-id>`. |
| US-091 / FEAT-010 | Preserve ordered execution history and inspect run status, logs, structured result, and provenance. | `cli/internal/metric/store_test.go:63-111` | UNIMPLEMENTED | History exists only for the metric-specific store; no generic run bundle or run inspector exists yet. |
| US-092 / FEAT-010 | Query execution history by artifact ID and retain agent-session linkage when applicable. | none | UNIMPLEMENTED | No `ddx exec history` implementation or generic run model exists. |
| US-093 / FEAT-010 | Optional metric convenience commands resolve through `ddx exec` without a separate authoritative `.ddx/metrics/` store. | `cli/cmd/metric_acceptance_test.go:38-85` | UNIMPLEMENTED | The command surface exists, but it resolves through `internal/metric` and still writes to `.ddx/metrics/`. |
| US-094 / FEAT-010 | Define a migration or backward-compatible policy for older specialized runtime data. | none | UNIMPLEMENTED | No migration or compatibility story is implemented for the metric store. |

## Gap Register

| Area | Classification | Planning Evidence | Implementation Evidence | Resolution Direction | Issue |
|------|----------------|-------------------|------------------------|----------------------|-------|
| Execution model / metric runtime substrate | DIVERGENT | FEAT-010, architecture, SD-005, TD-005, TP-005 | `cli/cmd/metric.go`, `cli/internal/metric/store.go`, `cli/internal/metric/store_test.go`, `cli/cmd/metric_acceptance_test.go` | code-to-plan | `ddx-f02e6f0e` |

### Quality Findings

| Area | Dimension | Concern | Severity | Resolution | Issue |
|------|-----------|---------|----------|------------|-------|
| Metric runtime store | maintainability | The separate metric store duplicates the eventual exec responsibilities, which makes the codebase easier to ship short-term but harder to reconcile safely with FEAT-010. | medium | quality-improvement | `ddx-6fe3e041` |

## Traceability Matrix

| Vision | Requirement | Feature/Story | Arch/ADR | Design | Tests | Impl Plan | Code Status | Classification |
|--------|-------------|---------------|----------|--------|-------|-----------|-------------|----------------|
| Runtime evidence stays reusable and artifact-linked | Generic execution substrate with immutable runs | FEAT-010 / US-090-US-094 | `architecture.md`, ADR-001 | `SD-005`, `TD-005` | `TP-005` | none specific | Metric-specific store and CLI exist instead | DIVERGENT |

## Execution Issues Generated

| Issue ID | Type | Labels | Goal | Dependencies | Verification |
|----------|------|--------|------|--------------|-------------|
| `ddx-966e5349` | task | `helix`, `phase:build`, `kind:implementation`, `area:exec` | Implement the generic `ddx exec` substrate under `.ddx/exec/` | none | New exec store, definition/run commands, atomic writes, and concurrency tests |
| `ddx-6fe3e041` | task | `helix`, `phase:build`, `kind:implementation`, `area:exec` | Re-layer metrics as a projection over `ddx exec` | `ddx-966e5349` | Metric convenience surface delegates to exec; no authoritative `.ddx/metrics/` store remains |

## Issue Coverage

| Gap / Criterion | Covering Issue | Status |
|-----------------|----------------|--------|
| Generic exec substrate absent | `ddx-966e5349` | covered |
| Metric surface still forks storage | `ddx-6fe3e041` | covered |
| FEAT-010 migration/compatibility policy not implemented | `ddx-6fe3e041` | covered |

## Execution Order

1. Implement the generic `ddx exec` substrate in `ddx-966e5349`.
2. Re-layer the metric surface over that substrate in `ddx-6fe3e041`.

**Critical Path**: build the shared execution substrate first, then move the metric projection onto it. | **Parallel**: none for the core exec path | **Blockers**: none identified.

## Open Decisions

| Decision | Why Open | Governing Artifacts | Recommended Owner |
|----------|----------|---------------------|-------------------|
| Exact migration strategy for historical `.ddx/metrics` data | FEAT-010 allows migration or explicit backward-compatible policy, but the implementation path is not yet coded | `FEAT-010`, `SD-005`, `TD-005` | `ddx-6fe3e041` |
