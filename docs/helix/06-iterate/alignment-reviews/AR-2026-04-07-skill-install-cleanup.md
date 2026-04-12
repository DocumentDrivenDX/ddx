# Alignment Review: skill install cleanup

**Review Date**: 2026-04-07
**Scope**: ddx init + ddx install skill cleanup
**Status**: complete
**Review Epic**: ddx-c1cfcb40
**Primary Governing Artifact**: FEAT-015

## Scope and Governing Artifacts

### Scope

- ddx init bootstrap skill cleanup
- ddx install/plugin skill symlink cleanup + stale install file removal

### Governing Artifacts

- docs/helix/00-discover/product-vision.md
- docs/helix/01-frame/prd.md
- docs/helix/01-frame/features/FEAT-015-installation-architecture.md
- docs/helix/01-frame/features/FEAT-011-skills.md
- docs/helix/01-frame/features/FEAT-009-library-registry.md
- docs/helix/02-design/architecture.md
- docs/helix/03-test/test-plans/TP-015-onboarding-journey.md

## Intent Summary

- **Vision**: DDx provides plugin infrastructure and self-documenting onboarding for agent workflows, with skills and installs as first-class primitives.
- **Requirements**: PRD requires plugin install (`ddx install helix`) and agent-facing skills, plus self-documenting setup and onboarding.
- **Features / Stories**: FEAT-015 defines install architecture (init copies bootstrap skills; plugin installs create skill symlinks; idempotent behavior). FEAT-011 defines skill format and install expectations.
- **Architecture / ADRs**: architecture.md places install/search/verify in the registry engine and stresses deterministic, auditable install flows.
- **Technical Design**: No dedicated design doc for stale skill cleanup.
- **Test Plans**: TP-015 covers install and onboarding journey but not stale cleanup scenarios.
- **Implementation Plans**: None specific to this change.

## Planning Stack Findings

| Finding | Type | Evidence | Impact | Review Issue |
|---------|------|----------|--------|-------------|
| Stale skill cleanup semantics (removing obsolete ddx-* and plugin skill links) are not specified in FEAT-015/FEAT-011 acceptance criteria. | underspecified | FEAT-015 AC-003/AC-004; FEAT-011 Installation section | Behavior exists in code but no documented contract for removal or user expectations. | ddx-c92ff289 |
| Test plan lacks coverage for stale skill cleanup and stale install file removal. | missing-link | TP-015 TC-020/TC-021 | No automated verification of cleanup behavior. | ddx-3f6a7bc1 |

## Implementation Map

- **Topology**: CLI install logic in `cli/cmd/init.go`, `cli/cmd/install.go`, and registry installer in `cli/internal/registry/installer.go`.
- **Entry Points**: `ddx init`, `ddx install <plugin>`.
- **Test Surfaces**: Go unit tests in `cli/internal/registry/registry_test.go`; onboarding tests in TP-015.
- **Unplanned Areas**: Cleanup logic (`cleanupBootstrapSkills`, `pruneStaleSkillLinks`, `removeStaleFilesFromInstall`) added without spec or tests.

## Acceptance Criteria Status

| Story / Feature | Criterion | Test Reference | Status | Evidence |
|-----------------|-----------|----------------|--------|----------|
| FEAT-015 AC-003 | ddx init copies bootstrap skills as real files | TP-015 TC-021 | UNTESTED | init.go copies skills; no test run in this review. |
| FEAT-015 AC-004 | ddx install helix creates skill symlinks | TP-015 TC-020 | UNTESTED | installer.go symlinkSkills; no test run in this review. |

## Gap Register

| Area | Classification | Planning Evidence | Implementation Evidence | Resolution Direction | Issue |
|------|----------------|-------------------|------------------------|----------------------|-------|
| ddx init bootstrap skill cleanup | UNDERSPECIFIED | FEAT-015 AC-003 (no cleanup contract), FEAT-011 Installation | `cleanupBootstrapSkills` in `cli/cmd/init.go` (lines ~382-422) removes ddx-* skill dirs not in bootstrap list. | plan-to-code | ddx-e44ee3c8 |
| plugin install stale skill cleanup + stale file removal | UNDERSPECIFIED | FEAT-015 AC-004 + idempotent requirement (no stale removal specifics); TP-015 lacks coverage | `pruneStaleSkillLinks` in `cli/internal/registry/installer.go` (lines ~324-438) and `removeStaleFilesFromInstall` in `cli/cmd/install.go` (lines ~148-176). | plan-to-code | ddx-e44ee3c8 |

### Quality Findings

| Area | Dimension | Concern | Severity | Resolution | Issue |
|------|-----------|---------|----------|------------|-------|
| install cleanup | robustness | Cleanup logic has no dedicated tests (registry + init/install). | medium | quality-improvement | ddx-52df44b1 |

## Traceability Matrix

| Vision | Requirement | Feature/Story | Arch/ADR | Design | Tests | Impl Plan | Code Status | Classification |
|--------|-------------|---------------|----------|--------|-------|-----------|-------------|----------------|
| Plugin infrastructure + self-documenting onboarding | PRD plugin install + skills | FEAT-015 AC-003/AC-004 | architecture.md (registry install/search) | none | TP-015 TC-020/TC-021 | none | Cleanup added but not specified/tested | UNDERSPECIFIED |
| Agent-facing skills | PRD skills requirement | FEAT-011 Installation | architecture.md | none | TP-015 (indirect) | none | ddx init cleans ddx-* skills without documented contract | UNDERSPECIFIED |

## Review Issue Summary

- ddx-c92ff289 — ddx init bootstrap skills cleanup
- ddx-3f6a7bc1 — plugin install stale skill cleanup

## Execution Issues Generated

| Issue ID | Type | Labels | Goal | Dependencies | Verification |
|----------|------|--------|------|--------------|--------------|
| ddx-e44ee3c8 | task | helix,phase:build,area:cli,kind:docs | Document stale skill cleanup expectations in FEAT-015/FEAT-011 | none | Docs updated, acceptance criteria added |
| ddx-52df44b1 | task | helix,phase:build,area:cli,kind:test | Add tests for cleanup behaviors | ddx-e44ee3c8 (docs clarify expected behavior) | Tests pass / updated TP-015 |

## Issue Coverage

| Gap / Criterion | Covering Issue | Status |
|-----------------|----------------|--------|
| ddx init stale ddx-* skill cleanup undocumented | ddx-e44ee3c8 | covered |
| plugin install stale skill symlink cleanup undocumented | ddx-e44ee3c8 | covered |
| cleanup logic untested | ddx-52df44b1 | covered |

## Execution Order

1. ddx-e44ee3c8 — document cleanup contract and acceptance criteria
2. ddx-52df44b1 — add tests / update TP-015

**Critical Path**: ddx-e44ee3c8 → ddx-52df44b1 | **Parallel**: none | **Blockers**: none

## Open Decisions

| Decision | Why Open | Governing Artifacts | Recommended Owner |
|----------|----------|---------------------|-------------------|
| Should cleanup remove only ddx-* skills or be driven by an explicit allowlist in config? | Behavior is currently implicit in code without a documented contract. | FEAT-015, FEAT-011 | DDx CLI owner |

## Queue Health and Exhaustion Assessment

- Ready queue currently non-empty (9 ready beads, including review and agent-related work). Cleanup follow-ups add 2 build tasks.

## Measurement Results

- Acceptance: Alignment report published, gaps classified, execution issues created.
- Status: PASS

## Follow-On Beads Created

- ddx-e44ee3c8
- ddx-52df44b1

ALIGN_STATUS: COMPLETE
GAPS_FOUND: 2
EXECUTION_ISSUES_CREATED: 2
MEASURE_STATUS: PASS
BEAD_ID: ddx-1e363751
FOLLOW_ON_CREATED: 2
