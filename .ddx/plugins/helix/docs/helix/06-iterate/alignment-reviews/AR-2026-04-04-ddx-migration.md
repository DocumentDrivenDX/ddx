# Alignment Review: DDx Migration

**Date:** 2026-04-04
**Scope:** DDx bead delegation, tracker migration, spec alignment
**Reviewer:** Claude Opus 4.6

## Summary

This review covers the migration of the HELIX tracker from a self-contained
bash implementation (`scripts/tracker.sh`, ~962 lines) to a thin adapter
delegating to `ddx bead` (~200 lines). The review checks spec/code alignment
after the migration.

## Findings

### ALIGNED

| Area | Evidence |
|------|----------|
| PRD Dependencies | PRD correctly describes DDx bead + DDx agent as dependencies |
| FEAT-002 Tracker Model | Accurately describes ddx bead delegation with HELIX validation hooks |
| DDX.md Tooling | Correctly describes beads, ADRs, HELIX layering |
| scripts/tracker.sh | Delegates all storage to ddx bead; HELIX validation preserved |
| scripts/helix field names | Uses bd-compatible field names (issue_type, owner, dependencies) |
| NEXT_ACTION vocabulary | Uses BUILD/DESIGN (not IMPLEMENT/PLAN) everywhere |
| Skill symlinks | helix-implement → helix-build, helix-plan → helix-design |
| SKILL.md frontmatter | All 16 skills parse valid YAML |
| Test suite | 133/133 pass with ddx bead backend |
| helix status | Implemented with run-state persistence and JSON output |

### FIXED (was STALE)

| Area | What Changed |
|------|-------------|
| tracker_dir default | Was `.ddx/`, now `.helix/` per FEAT-002 and README |
| 10+ normative docs | Updated from `.helix/issues.jsonl` path references |
| TRACKER.md refs | Removed from all normative files (file deleted) |
| ADR-003 (Go rewrite) | Deleted — applies to DDx, not HELIX |
| Export default | No longer hardcodes `.beads/issues.jsonl` |

### REMAINING DRIFT

| Area | Status | Notes |
|------|--------|-------|
| workflow.yml comment | Low | Has `# - workflows/TRACKER.md` comment — cosmetic |
| TD-002 line numbers | Low | Design doc line refs to tracker.sh are stale — expected for historical docs |
| TP-002 evidence chain | Low | Test plan references are generic — pre-existing, not migration-related |

### OPEN WORK

6 issues remain in tracker (all P2/P3):
- `hx-0d27eca7` P2: Condense codex stderr output
- `hx-7d00b06f` P2: Queue-drain check regression test
- `hx-bf99e0ee` P2: Deduplicate implementation blocker notes
- `hx-44a5dbfe` P3: Deduplicate review evidence notes
- `hx-407ed8b8` P2: helix start/stop for background operation
- `hx-8447a41c` P2: Refresh implementation plan

None are migration-related. All are incremental improvements.

## Conclusion

The DDx migration is complete and aligned:
- **Storage**: delegated to `ddx bead` via thin adapter
- **Format**: bd-compatible (issue_type, owner, dependencies, created_at, updated_at)
- **Specs**: FEAT-002, PRD, DDX.md all reflect the new architecture
- **Tests**: 133/133 pass through the adapter
- **Vocabulary**: BUILD/DESIGN throughout (not IMPLEMENT/PLAN)

No blocking drift remains. The 6 open issues are backlog quality work.

ALIGNMENT_STATUS: ALIGNED
