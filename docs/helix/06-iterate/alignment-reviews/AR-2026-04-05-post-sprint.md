---
ddx:
  id: AR-2026-04-05
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-009
    - FEAT-012
---
# Alignment Review: Post-Sprint Reconciliation

**Date:** 2026-04-05
**Scope:** Full repository — specs vs implementation after release readiness sprint
**Reviewer:** Claude Opus 4.6

## Critical Findings

### 1. Spec status fields are stale

| Spec | Status in Doc | Actual State |
|------|--------------|-------------|
| FEAT-009 (Registry) | Not Started | Core commands operational |
| FEAT-012 (Git Awareness) | Not Started | checkpoint + history commands live |
| FEAT-001 Doc Graph section | not started | All commands implemented |

**Resolution:** Update status fields to match reality.

### 2. PRD feature index is incomplete

PRD v3.5.0 lists FEAT-001, 002, 004, 006, 007, 010, 011, 012, 013, 014 but
omits FEAT-003 (website), FEAT-005 (artifacts), FEAT-008 (web UI),
FEAT-009 (registry) — all of which have spec files.

**Resolution:** Add missing FEATs to PRD index.

### 3. Deprecated commands not fully removed

`ddx mcp` is marked deprecated in FEAT-001 but fully functional in the binary.
`ddx update` is soft-deprecated (redirect message) but still accessible.

**Resolution:** Remove `mcp` command entirely. Make `update` a hard redirect
to `ddx install`.

### 4. Extra commands not in spec

Commands exist in the binary but aren't documented in FEAT-001:
- `ddx bead evidence`
- `ddx agent condense`
- `ddx exec define`
- `ddx metric` (top-level)
- `ddx doc migrate`

**Resolution:** Add to FEAT-001 or remove if not intended.

## Non-Critical Findings

### 5. Persona uses flags not subcommands

FEAT-001 says `ddx persona list/show/bind` (subcommands) but the binary uses
`ddx persona --list/--show/--bind` (flags). The flag-based interface works
but doesn't match the spec.

**Classification:** Cosmetic drift — low priority.

### 6. Missing spec'd commands

- `ddx verify` (FEAT-009 item 41) — not implemented
- `ddx outdated` (FEAT-009 item 9) — not implemented

**Classification:** Spec describes future work. Not a drift — just not done yet.

## Recommendations

1. **Update spec statuses** — mechanical, one pass through all FEAT docs
2. **Fix PRD feature index** — add FEAT-003, 005, 008, 009
3. **Remove ddx mcp** — delete the command code
4. **Document extra commands** — add exec define, metric, doc migrate to FEAT-001
