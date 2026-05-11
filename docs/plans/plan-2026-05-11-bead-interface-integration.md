---
ddx:
  id: plan-2026-05-11-bead-interface-integration
  status: superseded
  superseded_by: TD-027
---
# Bead Interface Refactor — Integration Plan — SUPERSEDED

> **SUPERSEDED 2026-05-11 by TD-027 (Bead Architecture).**
>
> This integration plan catalogued side-doors and legacy paths, specified compile/test/process integrity mechanisms, and described the rollout sequencing for the bead interface refactor.
>
> All normative content has moved into TD-027 (`docs/helix/02-design/technical-designs/TD-027-bead-collection-abstraction.md`):
>
> - Side-door catalog → TD-027 §21 (Module Boundary)
> - Legacy path catalog → TD-027 §21
> - Compile-time / test-time / process-time integrity layers → TD-027 §28 (Acceptance Criteria) + §27 (Future-Change Process)
> - Caller migration as Stage 2 → TD-027 §21
> - Boundary lockdown as Stage 3 (physical impossibility via Go `internal/`) → TD-027 §21
> - Allowlist + lint rules → TD-027 §21 side-door catalog (transitional measures; obsolete after Stage 3)
>
> The follow-up beads `ddx-900a8d38` (WatcherHub injection) and `ddx-e91a45c0` (lint suite) reference TD-027 as their canonical acceptance criteria source. Do not edit this plan; amend TD-027 directly.

[Original plan content preserved in git history.]
