---
ddx:
  id: AR-2026-04-04-agent-beads
  depends_on:
    - AR-2026-04-04
    - FEAT-004
    - FEAT-006
---

> **Historical** — describes the pre-2026-04-14 React stack. Current stack: see ADR-002 v2.
# Alignment Review: Agent Harness & Bead Storage

**Date:** 2026-04-04
**Scope:** Agent service (FEAT-006) + Bead tracker (FEAT-004) post-refactor
**Reviewer:** Claude Opus 4.6
**Prior review:** AR-2026-04-04

## Prior Findings Status

| Finding | Status |
|---------|--------|
| F-001: Artifacts lack frontmatter | **RESOLVED** — all 15 have ddx: frontmatter |
| F-002: Vision/PRD no IDs | **RESOLVED** |
| F-004: No architecture doc | OPEN (deferred) |
| F-005: No test plans | OPEN (deferred) |
| F-006: FEAT-001 stale | **RESOLVED** — updated |
| F-007: FEAT-002 stale | **RESOLVED** — updated |
| F-008: Orphaned artifact pkg | **RESOLVED** — removed |

## Agent Service (FEAT-006) — Post-Refactor

### Architecture

| Component | Status | Notes |
|-----------|--------|-------|
| Executor interface | Clean | `Executor` + `OSExecutor` + `LookPathFunc`. Full testability seam. |
| Registry | Clean | Data-driven harness definitions. No name-based switches in runner. |
| Runner | Clean | Decomposed: resolveHarness/resolvePrompt/resolveModel/resolveTimeout/BuildArgs/processResult |
| Quorum | Clean | Parallel execution, threshold logic, strategy evaluation |
| Token extraction | Clean | `Harness.TokenPattern` field, no harness-name dispatch |
| Effort handling | Clean | `Harness.EffortFormat` field, format string applied generically |
| Session logging | Clean | JSONL append, best-effort |
| CLI commands | Clean | run/list/doctor/log all wired up |

### Test Coverage

| Area | Tests | Coverage |
|------|-------|---------|
| Registry (builtin, get, preference, discover) | 4 | Good |
| Arg construction (codex, claude, gemini, all flags) | 7 | Good |
| Runner with mock (basic, stdin, promptfile, model, exit codes) | 7 | Good |
| Token extraction | 3 | Adequate |
| Session logging | 1 | Adequate |
| Quorum (threshold, consensus, multi-harness) | 4 | Good |
| Integration (real codex, real claude) | 2 | Critical path covered |
| **Total** | **28** | |

### Remaining Gap

**F-009: Config not loaded from .ddx/config.yaml** (severity: medium)
- `agent_cmd.go:43` still has `TODO: load agent config`
- Default harness, model overrides, timeout from project config are ignored
- **Resolution:** Wire up when config refactor happens. Low urgency — CLI flags override everything.

## Bead Tracker (FEAT-004) — Post Path Fix

### Storage

| Aspect | Spec (FEAT-004) | Implementation | Aligned? |
|--------|----------------|----------------|----------|
| Default dir | `.beads` | `.beads` | YES |
| Default file | `.beads/issues.jsonl` | `.beads/issues.jsonl` | YES |
| Lock dir | `.beads/issues.lock/` | `.beads/issues.lock/` | YES (just fixed) |
| ID prefix | Auto from repo name | `detectPrefix()` → repo name | YES |
| Schema | bd-compatible | `issue_type`, `owner`, `created_at`, `dependencies` | YES |
| `--set key=value` | Supported | `--set` with type routing for known fields | YES |
| bd/br backend | Configurable | `DDX_BEAD_BACKEND` env var | YES |

### Finding

**F-010: Bead `--set` routes known fields to struct, unknown to Extra** (severity: info, not a bug)
- `bead.go:270` — `--set issue_type=epic` sets `b.IssueType`, not `b.Extra["issue_type"]`
- This is intentional (from the linter fix) but should be documented
- Known routed fields: `parent`, `description`, `notes`, `acceptance`, `issue_type`
- Booleans and numbers in `--set` are parsed for proper JSON typing
- **No action needed** — this is correct behavior

## Schema Consistency

| Store | Schema | Compatible? |
|-------|--------|-------------|
| `.helix/issues.jsonl` (HELIX tracker) | `dependencies`, `issue_type`, `owner`, `created_at` | YES |
| `.beads/issues.jsonl` (DDx beads) | Same schema | YES |
| bd export | Same schema | YES (locked by schema_compat_test.go) |

## Summary

The agent service and bead tracker are well-aligned with their specs after the refactors. The Executor interface provides full testability. The bead storage path now matches bd convention. Schema compatibility is locked with tests.

**Open items from this + prior review:**
- F-004: No architecture document (deferred)
- F-005: No test plans (deferred)
- F-009: Agent config not loaded from .ddx/config.yaml (low priority)
