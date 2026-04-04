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
    - ADR-004
    - TD-010
    - TD-005
    - TP-010
    - TP-005
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
- `docs/helix/02-design/adr/ADR-004-bead-backed-runtime-storage.md`
- `docs/helix/02-design/solution-designs/SD-005-metric-runtime-history.md`
- `docs/helix/02-design/technical-designs/TD-010-executions.md`
- `docs/helix/02-design/technical-designs/TD-005-metric-runtime-history.md`
- `docs/helix/03-test/test-plans/TP-010-executions.md`
- `docs/helix/03-test/test-plans/TP-005-metric-runtime-history.md`
- `cli/cmd/exec.go`
- `cli/internal/exec/store.go`
- `cli/internal/exec/bead_runtime.go`
- `cli/cmd/metric.go`
- `cli/internal/metric/store.go`
- `cli/internal/metric/exec_bridge.go`
- `cli/internal/metric/store_test.go`
- `cli/internal/exec/store_test.go`
- `cli/cmd/exec_acceptance_test.go`
- `cli/cmd/metric_acceptance_test.go`

## Intent Summary

- **Vision**: DDx keeps runtime evidence separate from declarative artifacts and exposes reusable, workflow-agnostic execution records.
- **Requirements**: execution definitions and immutable runs should live in repo-local storage, support raw logs plus structured results, and remain projection-friendly for specializations like metrics.
- **Features / Stories**: FEAT-001 already exposes `ddx exec` command shapes; FEAT-010 makes `ddx exec` the owning substrate and makes `ddx metric` only an optional wrapper.
- **Architecture / ADRs**: the architecture and ADR-004 explicitly define bead-backed collections plus sidecar attachments as the runtime storage pattern.
- **Technical Design**: TD-010 defines generic `exec-definitions` and `exec-runs` collections; TD-005 layers metric projections on top and rejects a separate `.ddx/metrics/` runtime store.
- **Test Plans**: TP-010 covers generic execution persistence and compatibility; TP-005 covers metric projection behavior over that substrate.
- **Implementation Plans**: the reopened exec epic now tracks storage hardening and migration remediation on top of the generic substrate already in flight.

## Planning Stack Findings

| Finding | Type | Evidence | Impact | Review Issue |
|---------|------|----------|--------|-------------|
| FEAT-010, ADR-004, TD-010, TD-005, TP-010, and TP-005 now agree on bead-backed execution collections with metric projections layered on top. | aligned | `docs/helix/01-frame/features/FEAT-010-executions.md`, `docs/helix/02-design/adr/ADR-004-bead-backed-runtime-storage.md`, `docs/helix/02-design/technical-designs/TD-010-executions.md`, `docs/helix/02-design/technical-designs/TD-005-metric-runtime-history.md`, `docs/helix/03-test/test-plans/TP-010-executions.md`, `docs/helix/03-test/test-plans/TP-005-metric-runtime-history.md` | The planning stack is coherent; storage and inspection boundaries are no longer underspecified. | `ddx-facb7aa5` |
| The implementation now has a real `ddx exec` substrate and metric projection layer, but the `agent` executor path in `ddx exec` is still unimplemented. | partial | `cli/cmd/exec.go`, `cli/internal/exec/store.go`, `cli/cmd/metric.go`, `cli/internal/metric/exec_bridge.go` | Generic command execution is aligned; FEAT-010 executor coverage is still incomplete. | `ddx-53048c6c` |
| Storage hardening work remains open for collection migration cleanup, external-backend adapter tests, and CI schema enforcement. | partial | `cli/internal/exec/store.go`, `cli/internal/bead/backend_external.go`, `cli/internal/bead/schema_validation_test.go`, `.github/workflows/ci.yml` | The storage model is correct, but backend and CI guards are not fully closed. | `ddx-f9121a4c`, `ddx-21fda5a4`, `ddx-105e7afc` |

## Implementation Map

- **Topology**: `cli/cmd/exec.go` fronts `cli/internal/exec`; `cli/cmd/metric.go` and `cli/internal/metric/exec_bridge.go` project metric behavior over that substrate.
- **Entry Points**: `newExecCommand`, `ListDefinitions`, `ShowDefinition`, `Validate`, `Run`, `Log`, `Result`, `History`, and the metric wrapper commands.
- **Test Surfaces**: `cli/internal/exec/store_test.go`, `cli/cmd/exec_acceptance_test.go`, `cli/internal/metric/store_test.go`, and `cli/cmd/metric_acceptance_test.go` now cover both the generic execution path and the metric projection.
- **Unplanned Areas**: `agent` executor delegation inside `ddx exec` is still missing, and the remaining storage-hardening tasks are tracked separately.

## Acceptance Criteria Status

| Story / Feature | Criterion | Test Reference | Status | Evidence |
|-----------------|-----------|----------------|--------|----------|
| US-090 / FEAT-010 | Run a metric-backed execution and retain raw logs plus structured result data. | `cli/internal/exec/store_test.go`, `cli/cmd/exec_acceptance_test.go`, `cli/cmd/metric_acceptance_test.go` | PARTIAL | Command-backed exec runs and metric projections exist; `agent` executor support is still pending. |
| US-091 / FEAT-010 | Preserve ordered execution history and inspect run status, logs, structured result, and provenance. | `cli/internal/exec/store_test.go`, `cli/cmd/exec_acceptance_test.go` | PARTIAL | Generic history and inspection exist, but storage-hardening follow-ons are still open. |
| US-092 / FEAT-010 | Query execution history by artifact ID and retain agent-session linkage when applicable. | `cli/internal/exec/store_test.go` | PARTIAL | Artifact filtering exists; agent-session linkage is blocked on `agent` executor support inside `ddx exec`. |
| US-093 / FEAT-010 | Optional metric convenience commands resolve through `ddx exec` without a separate authoritative `.ddx/metrics/` store. | `cli/cmd/metric_acceptance_test.go`, `cli/internal/metric/store_test.go` | IMPLEMENTED | The metric surface now resolves through the exec store and bridges to generic execution records. |
| US-094 / FEAT-010 | Define a migration or backward-compatible policy for older specialized runtime data. | `cli/internal/exec/store_test.go` | PARTIAL | Legacy `.ddx/exec/` fallback reads exist; final migration cleanup and hardening remain open. |

## Gap Register

| Area | Classification | Planning Evidence | Implementation Evidence | Resolution Direction | Issue |
|------|----------------|-------------------|------------------------|----------------------|-------|
| Execution model / storage hardening | PARTIAL | FEAT-010, ADR-004, TD-010, TD-005, TP-010, TP-005 | `cli/cmd/exec.go`, `cli/internal/exec/store.go`, `cli/internal/metric/store.go`, `cli/internal/bead/backend_external.go` | close remaining build/test hardening tasks | `ddx-f9121a4c`, `ddx-21fda5a4`, `ddx-105e7afc` |
| Executor coverage | PARTIAL | FEAT-010, FEAT-006 | `cli/internal/exec/store.go` | implement `agent` executor delegation through `ddx agent` | none |

### Quality Findings

| Area | Dimension | Concern | Severity | Resolution | Issue |
|------|-----------|---------|----------|------------|-------|
| Exec executor coverage | completeness | FEAT-010 defines `agent` as a first-class executor kind, but `ddx exec run` still rejects it explicitly. | medium | tracked implementation gap | `ddx-53048c6c` |
| Runtime storage hardening | maintainability | Collection-backed storage is in place, but external backend tests and CI schema enforcement are still being closed. | medium | tracked follow-on | `ddx-21fda5a4`, `ddx-105e7afc` |

## Traceability Matrix

| Vision | Requirement | Feature/Story | Arch/ADR | Design | Tests | Impl Plan | Code Status | Classification |
|--------|-------------|---------------|----------|--------|-------|-----------|-------------|----------------|
| Runtime evidence stays reusable and artifact-linked | Generic execution substrate with immutable runs | FEAT-010 / US-090-US-094 | `architecture.md`, `ADR-004` | `TD-010`, `SD-005`, `TD-005` | `TP-010`, `TP-005` | `ddx-facb7aa5` and child tasks | Generic exec substrate exists; remaining gaps are executor coverage and storage hardening | PARTIAL |

## Execution Issues Generated

| Issue ID | Type | Labels | Goal | Dependencies | Verification |
|----------|------|--------|------|--------------|-------------|
| `ddx-f9121a4c` | task | `helix`, `phase:build`, `kind:implementation`, `area:exec`, `area:beads` | Finish migrating exec and metric metadata onto bead-backed runtime collections | none | Exec/metric stores use named collections and explicit migration behavior |
| `ddx-53048c6c` | task | `helix`, `phase:build`, `kind:implementation`, `area:exec`, `area:agent` | Implement agent executor delegation from `ddx exec` through `ddx agent` | none | Delegated exec runs retain agent-session linkage and coherent inspection behavior |
| `ddx-21fda5a4` | task | `helix`, `phase:build`, `kind:testing`, `area:beads` | Harden external backend adapter coverage for runtime collections | none | Adapter tests assert logical collection propagation without prescribing backend layout |
| `ddx-105e7afc` | task | `helix`, `phase:build`, `kind:testing`, `area:beads` | Enforce bead-record schema validation in the real CI path | none | CI fails deterministically on bead envelope drift |

## Issue Coverage

| Gap / Criterion | Covering Issue | Status |
|-----------------|----------------|--------|
| Collection-backed storage migration hardening | `ddx-f9121a4c` | covered |
| `agent` executor inside `ddx exec` | `ddx-53048c6c` | covered |
| External backend collection-boundary tests | `ddx-21fda5a4` | covered |
| CI schema enforcement for bead-backed runtime records | `ddx-105e7afc` | covered |

## Execution Order

1. Finish collection-backed exec/metric storage hardening in `ddx-f9121a4c`.
2. Implement agent executor delegation in `ddx-53048c6c`.
3. Add external backend collection-boundary coverage in `ddx-21fda5a4`.
4. Wire schema validation into the authoritative CI path in `ddx-105e7afc`.

**Critical Path**: complete the collection-backed storage hardening first, then land agent delegation on the stabilized substrate. | **Parallel**: adapter tests and CI schema wiring can run independently. | **Blockers**: none beyond the tracked implementation tasks.

## Open Decisions

| Decision | Why Open | Governing Artifacts | Recommended Owner |
|----------|----------|---------------------|-------------------|
| How `ddx exec` should delegate the `agent` executor kind to `ddx agent` while preserving one run/session identity | FEAT-010 requires `agent` executor support, but the current implementation still returns a not-yet-implemented error | `FEAT-010`, `FEAT-006`, `TD-010`, `TD-006` | `ddx-53048c6c` |
