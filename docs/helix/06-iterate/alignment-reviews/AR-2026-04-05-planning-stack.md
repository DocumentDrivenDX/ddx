---
ddx:
  id: AR-2026-04-05-planning-stack
  depends_on:
    - helix.prd
    - product-vision
    - FEAT-001
    - FEAT-002
    - FEAT-003
    - FEAT-004
    - FEAT-005
    - FEAT-006
    - FEAT-007
    - FEAT-008
    - FEAT-009
    - FEAT-010
    - FEAT-011
    - FEAT-012
    - FEAT-013
    - FEAT-014
---

> **Historical** — describes the pre-2026-04-14 React stack. Current stack: see ADR-002 v2.
# Alignment Review: Planning Stack and Governance

**Review Date**: 2026-04-05
**Scope**: Planning stack traceability and internal consistency
**Status**: complete
**Review Epic**: `hx-6d163089`
**Primary Governing Artifact**: `helix.prd`
**Tracker Issue**: `hx-0fceabd3`

## Scope and Governing Artifacts

### Scope

- Vision → PRD traceability
- PRD → feature spec coverage
- Feature spec → design artifact linkage
- Design artifacts → test plan coverage
- Test plans → implementation presence

### Governing Artifacts Inspected

- `docs/helix/00-discover/product-vision.md` (id: `product-vision`)
- `docs/helix/01-frame/prd.md` (id: `helix.prd`, v3.5.0)
- `docs/helix/01-frame/features/FEAT-001` through `FEAT-014`
- `docs/helix/02-design/architecture.md`
- `docs/helix/02-design/adr/ADR-001` through `ADR-005`
- `docs/helix/02-design/solution-designs/SD-004`, `SD-005`, `SD-006`, `SD-007`, `SD-014`
- `docs/helix/02-design/technical-designs/TD-004`, `TD-005`, `TD-006`, `TD-010`
- `docs/helix/03-test/test-plans/TP-004`, `TP-005`, `TP-006`, `TP-007`, `TP-010`, `TP-014`, `TP-015`

## Traceability Matrix

| Feature | In PRD list | Has FEAT doc | Has SD/TD | Has TP | Status |
|---------|-------------|-------------|-----------|--------|--------|
| FEAT-001 CLI | ✓ | ✓ | via architecture.md | TP-015 (e2e) | ALIGNED |
| FEAT-002 Server | ✓ | ✓ | — | TP-015 (e2e) | PARTIAL — no dedicated SD/TD |
| FEAT-003 Website | body only | ✓ | via ADR-002 | Hugo build | PARTIAL |
| FEAT-004 Beads | ✓ | ✓ | SD-004, TD-004 | TP-004 | ALIGNED |
| FEAT-005 Artifacts | body only | ✓ | via architecture.md | TP-007 | ALIGNED |
| FEAT-006 Agent | ✓ | ✓ | SD-006, TD-006 | TP-006 | ALIGNED |
| FEAT-007 Doc Graph | ✓ | ✓ | via architecture.md | TP-007 | ALIGNED |
| FEAT-008 Web UI | body only | ✓ | via ADR-002/005 | — | INCOMPLETE |
| FEAT-009 Registry | body only | ✓ | via ADR-003 | — | INCOMPLETE |
| FEAT-010 Executions | ✓ | ✓ | SD-005, TD-010 | TP-010, TP-005 | ALIGNED |
| FEAT-011 Skills | ✓ | ✓ | — | — | PLANNING ONLY |
| FEAT-012 Git Awareness | ✓ | ✓ | — | — | PLANNING ONLY |
| FEAT-013 Multi-agent | ✓ | ✓ | — | — | PLANNING ONLY |
| FEAT-014 Token Awareness | not in PRD list | ✓ | SD-014 | TP-014 | PARTIAL — missing from PRD feature list |

## Planning Stack Findings

| Finding | Type | Evidence | Impact |
|---------|------|----------|--------|
| Vision, PRD, and all 14 feature specs exist with `ddx:` frontmatter and valid IDs. | resolved / aligned | All files checked | Stack is navigable top-to-bottom |
| PRD feature reference list covers FEAT-001, FEAT-002, FEAT-004, FEAT-006, FEAT-007, FEAT-010–013 but omits FEAT-003, FEAT-005, FEAT-008, FEAT-009, FEAT-014 from explicit listing. FEAT-003/005/008/009 are described in the PRD body; FEAT-014 is absent entirely. | partial | `docs/helix/01-frame/prd.md` lines 33–43; absence of `FEAT-014` anywhere in PRD text | Minor traceability gap: a reader browsing the PRD feature index cannot find token awareness; it is only discoverable via FEAT-006 or the plan doc |
| FEAT-002 (server) has no dedicated SD or TD. The server surface is partially covered by architecture.md and ADR-002, but no solution design document exists for the HTTP/MCP contract. | gap | `docs/helix/02-design/solution-designs/` — no SD-002; `docs/helix/02-design/technical-designs/` — no TD-002 | Medium: test plans and acceptance criteria for FEAT-002 cannot anchor to a concrete design document |
| FEAT-008 (web UI) has no test plan. The framework decision is in ADR-002/ADR-005 but there is no TP for UI acceptance. | gap | Absence of `TP-008-*` in `docs/helix/03-test/test-plans/` | Low: covered by existing tracking issue `hx-c8b3104a` |
| FEAT-009 (registry) has no test plan or design document. | gap | Absence of `SD-009-*`, `TD-009-*`, `TP-009-*` | Low: covered by existing tracking issue `hx-6b1ddf04` |
| FEAT-011, FEAT-012, FEAT-013 have no SD/TD/TP. They are planning-only with feature spec + PRD reference, which is correct for unstarted features. | expected | Feature status fields: "Not Started" | No action needed until implementation begins |
| ADR-004 (bead-backed runtime storage) and ADR-005 (local-first beads UI) correctly flow down to SD-004, SD-005, TD-010, and the exec model. No ADR contradictions found. | aligned | ADR-004 → SD-005, TD-010; ADR-005 → SD-004 | Stack is internally consistent |

## Gap Register

| Area | Classification | Evidence | Resolution Direction | Existing Issue |
|------|----------------|----------|----------------------|----------------|
| FEAT-014 missing from PRD feature list | PARTIAL | `prd.md` lines 33–43 | Add FEAT-014 reference to PRD feature list section | none — create below |
| FEAT-002 server: no dedicated SD/TD | PARTIAL | No SD-002 or TD-002 in design dirs | Create SD-002 when server contract work resumes | none (low urgency) |
| FEAT-008 web UI: no TP | INCOMPLETE | No TP-008 | Create TP when UI implementation begins | `hx-c8b3104a` |
| FEAT-009 registry: no SD/TD/TP | INCOMPLETE | No SD/TD/TP for FEAT-009 | Create when registry implementation begins | `hx-6b1ddf04` |

## Execution Issues Generated

| Issue | Type | Goal |
|-------|------|------|
| `hx-prd-feat014` (create below) | task | Add FEAT-014 to PRD feature list |

## Issue Coverage

| Gap | Covering Issue | Status |
|-----|----------------|--------|
| FEAT-008 missing test plan | `hx-c8b3104a` | covered (tracked) |
| FEAT-009 missing design docs | `hx-6b1ddf04` | covered (tracked) |
| FEAT-014 not in PRD | new issue | to create |

## Open Decisions

| Decision | Why Open | Recommended Owner |
|----------|----------|-------------------|
| None | — | — |
