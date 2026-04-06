---
ddx:
  id: AR-2026-04-06-evolution
  depends_on:
    - helix.prd
    - product-vision
---
# Alignment Review: Post-Evolution Reconciliation

**Date:** 2026-04-06
**Scope:** Full repo — vision and PRD evolution
**Status:** Complete
**Epic:** ddx-24eefcfe

## Scope and Governing Artifacts

This review reconciles the planning stack after the vision and PRD were evolved
to incorporate strategic design discussion outcomes. The evolution added:

- Core thesis, differentiators, and design philosophy to the product vision
- Six new PRD goals (10-14 primary, 1 secondary)
- Four new feature references (FEAT-015 through FEAT-018)
- Expanded problem statement, risks, non-goals, and success criteria

**Governing artifacts reviewed:**

| Artifact | ID | Status |
|----------|-----|--------|
| Product Vision | product-vision | Updated to v2.0.0 |
| PRD | helix.prd | Updated to v4.0.0 |
| FEAT-001 CLI | FEAT-001 | In Progress |
| FEAT-002 Server | FEAT-002 | Not Started |
| FEAT-004 Beads | FEAT-004 | Complete |
| FEAT-005 Artifacts | FEAT-005 | In Progress |
| FEAT-006 Agent | FEAT-006 | Not Started (spec) |
| FEAT-007 Doc Graph | FEAT-007 | Not Started (spec) |
| FEAT-008 Web UI | FEAT-008 | Not Started |
| FEAT-009 Registry | FEAT-009 | Not Started |
| FEAT-010 Executions | FEAT-010 | In Progress |
| FEAT-011 Skills | FEAT-011 | Not Started |
| FEAT-012 Git Awareness | FEAT-012 | Not Started (spec) |
| FEAT-013 Multi-Agent | FEAT-013 | Framing |
| FEAT-014 Token Awareness | FEAT-014 | In Progress |
| FEAT-015 Feedback Loops | — | No spec file |
| FEAT-016 Measurement | — | No spec file |
| FEAT-017 Adversarial Review | — | No spec file |
| FEAT-018 Plugin API | — | No spec file |

## Intent Summary

**Vision (v2.0.0):** DDx is a toolkit/platform for document-driven agentic
software development. Unopinionated about methodology — plugins bring opinions.
Core thesis: documentation as abstraction produces better agent-written
software. Key differentiators vs vibe coding, code-only tools, and traditional
DDD. Design philosophy: multi-directional iteration, human-agent control
slider, self-documenting workflows, platform-not-opinions.

**PRD (v4.0.0):** Three artifacts (CLI, server, website). 14 primary goals
(up from 9), 3 secondary goals (up from 2). New capabilities: feedback loops,
measurement, adversarial review, plugin API stability. Explicit boundary:
DDx provides metric collection hooks and feedback infrastructure; plugins
define what to measure and how to act on feedback.

## Planning Stack Findings

| Finding | Classification | Evidence | Resolution |
|---------|---------------|----------|------------|
| FEAT-015 through FEAT-018 referenced in PRD but no spec files exist | UNDERSPECIFIED | PRD lines 50-57; no files in features/ | Write spec files |
| FEAT-016 (measurement) overlaps with FEAT-010 metric projections | UNDERSPECIFIED | FEAT-010 "domain-specific projections (metrics)"; PRD goal 12 "bead lifecycle metrics, cost tracking" | Clarify boundary: FEAT-010 owns exec-based metrics, FEAT-016 owns bead lifecycle and cost metrics |
| principles.md uses `dun:` frontmatter, not `ddx:` | STALE_PLAN | `docs/helix/01-frame/principles.md:2` — `dun: id: helix.workflow.principles` | Migrate to ddx: prefix |
| Vision lacks `depends_on` in frontmatter (nothing upstream) | ALIGNED | Vision is the root artifact — no upstream dependency expected | No action |
| PRD `depends_on: product-vision` correctly references vision | ALIGNED | `docs/helix/01-frame/prd.md:4` | No action |
| All FEAT-001 through FEAT-014 have spec files | ALIGNED | 14 files in `features/` | No action |
| Vision platform-vs-plugin boundary table matches PRD non-goals | ALIGNED | Vision "Platform Services, Not Opinions" table; PRD non-goals | No action |
| Optimization loop ownership unresolved | UNDERSPECIFIED | Evolution prompt §5 open question; PRD risk "resolve before plugin API v1" | Needs ADR |
| Prior alignment gaps (server mutations, CI, demos) still open | INCOMPLETE | AR-2026-04-05-repo.md execution issues | Carry forward |
| FEAT-XXX (installation architecture) has no assigned number | INCOMPLETE | `FEAT-XXX-installation-architecture.md` | Assign number |

## Gap Register

### New Gaps from Evolution

| ID | Gap | Classification | Resolution Direction | Priority |
|----|-----|---------------|---------------------|----------|
| G1 | FEAT-015 spec file missing | UNDERSPECIFIED | plan-to-code | P1 |
| G2 | FEAT-016 spec file missing; boundary with FEAT-010 unclear | UNDERSPECIFIED | plan-to-code | P1 |
| G3 | FEAT-017 spec file missing; no implementation | UNDERSPECIFIED | plan-to-code | P1 |
| G4 | FEAT-018 spec file missing; no formal plugin API | UNDERSPECIFIED | plan-to-code | P1 |
| G5 | Optimization loop ownership unresolved (DDx vs plugin) | UNDERSPECIFIED | decision-needed | P2 |
| G6 | principles.md uses `dun:` frontmatter prefix | STALE_PLAN | code-to-plan | P2 |
| G7 | FEAT-XXX needs a real number | INCOMPLETE | plan-to-code | P2 |

### Carried-Forward Gaps (from AR-2026-04-05)

| ID | Gap | Classification | Resolution Direction | Priority |
|----|-----|---------------|---------------------|----------|
| C1 | Server mutation endpoints (FEAT-002) | INCOMPLETE | code-to-plan | P1 |
| C2 | MCP write tools (bead create/update, doc changed) | INCOMPLETE | code-to-plan | P1 |
| C3 | Website demos and recordings (FEAT-003) | INCOMPLETE | code-to-plan | P1 |
| C4 | CI pipeline (GitHub Actions) | INCOMPLETE | code-to-plan | P0 |
| C5 | Web UI mutations (FEAT-008) | INCOMPLETE | code-to-plan | P1 |
| C6 | SQLite-WASM for beads UI (ADR-005) | INCOMPLETE | code-to-plan | P2 |
| C7 | Auth package tests | INCOMPLETE | code-to-plan | P2 |

## Traceability Matrix

| Vision Concept | PRD Goal | Feature Spec | Implementation |
|----------------|----------|-------------|----------------|
| Core thesis | — | — | Encoded in all specs |
| Artifact management | Goal 1 | FEAT-005, FEAT-007 | ✓ In progress |
| Plugin infrastructure | Goal 5, 14 | FEAT-009, FEAT-018 | ✓ Registry exists; API unspecified |
| Bead tracker | Goal 2 | FEAT-004 | ✓ Complete |
| Agent execution | Goal 2 | FEAT-006 | ✓ Implemented |
| Artifact templates | — | Plugin concern | ✓ In library/ |
| Multi-directional iteration | — | FEAT-007 (staleness) | ✓ Doc graph supports any-direction |
| Human-agent control slider | — | FEAT-006 (permissions) | ✓ safe/supervised/unrestricted |
| Self-documenting workflows | Goal S3 | — | ✗ No spec or implementation |
| Platform not opinions | Non-goals | All FEATs | ✓ Consistent boundary |
| Feedback loops | Goal 11 | FEAT-015 | ✗ No spec, no implementation |
| Measurement | Goal 12 | FEAT-016, FEAT-010 | ◐ Exec metrics exist; bead/cost metrics missing |
| Adversarial review | Goal 13 | FEAT-017 | ✗ No spec; quorum exists but not adversarial |
| Plugin API stability | Goal 14 | FEAT-018 | ✗ No spec; minimal API surface |

## Execution Issues

Filed under epic ddx-24eefcfe. See tracker for current state.

| Bead ID | Title | Priority | Labels |
|---------|-------|----------|--------|
| ddx-5751caf8 | Write FEAT-015 spec: feedback loop infrastructure | P1 | helix,phase:frame,kind:documentation,area:platform |
| ddx-05320bcb | Write FEAT-016 spec: measurement and metrics (clarify FEAT-010 boundary) | P1 | helix,phase:frame,kind:documentation,area:platform |
| ddx-063341d6 | Write FEAT-017 spec: adversarial review infrastructure | P1 | helix,phase:frame,kind:documentation,area:platform |
| ddx-436f9803 | Write FEAT-018 spec: plugin API stability | P1 | helix,phase:frame,kind:documentation,area:platform |
| ddx-b5b55387 | ADR: optimization loop ownership (DDx vs plugin) | P2 | helix,phase:design,kind:decision,area:platform |
| ddx-29ae9e8d | Migrate principles.md frontmatter from dun: to ddx: | P2 | helix,phase:iterate,kind:chore,area:docs |
| ddx-8de1dc56 | Assign number to FEAT-XXX installation architecture | P2 | helix,phase:frame,kind:chore,area:docs |

## Issue Coverage Verification

| Gap ID | Classification | Execution Bead | Covered |
|--------|---------------|----------------|---------|
| G1 | UNDERSPECIFIED | ddx-5751caf8 | ✓ |
| G2 | UNDERSPECIFIED | ddx-05320bcb | ✓ |
| G3 | UNDERSPECIFIED | ddx-063341d6 | ✓ |
| G4 | UNDERSPECIFIED | ddx-436f9803 | ✓ |
| G5 | UNDERSPECIFIED | ddx-b5b55387 | ✓ |
| G6 | STALE_PLAN | ddx-29ae9e8d | ✓ |
| G7 | INCOMPLETE | ddx-8de1dc56 | ✓ |
| C1-C7 | INCOMPLETE | carried from AR-2026-04-05 | ✓ (existing beads) |

All non-ALIGNED gaps have corresponding execution beads.

## Execution Order and Critical Path

**Wave 1 — Spec writing (parallel, no blockers):**
- G1: FEAT-015 spec
- G2: FEAT-016 spec
- G3: FEAT-017 spec
- G4: FEAT-018 spec

**Wave 2 — Design decisions (after Wave 1):**
- G5: Optimization loop ADR (depends on FEAT-016 and FEAT-018 scoping)

**Wave 3 — Housekeeping (independent):**
- G6: Frontmatter migration
- G7: FEAT-XXX numbering

**Critical path:** G1-G4 (spec writing) → G5 (ADR) → implementation.
The four spec files are the highest priority deliverables from this evolution.
Carried-forward gaps (C1-C7) continue on their own existing execution paths.
