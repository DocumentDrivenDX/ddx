---
ddx:
  id: plan-2026-05-11-bead-backend-interfaces
  status: superseded
  superseded_by: TD-027
---
# Bead Backend Interface Refinement (Pre-Axon) — SUPERSEDED

> **SUPERSEDED 2026-05-11 by TD-027 (Bead Architecture).**
>
> This plan captured the design exploration for the bead backend interface refactor — 11 sub-interfaces composing `Backend`, the `Operation` pattern with `OperationApplier` optional fast-path, pluggable `IDGenerator`, parallel `LifecycleSubscriber`, `context.Context` discipline, sentinel errors, module boundary via Go's `internal/` rule, and three-stage path (interface refactor → caller migration → boundary lockdown).
>
> All normative content has moved into TD-027 (`docs/helix/02-design/technical-designs/TD-027-bead-collection-abstraction.md`):
>
> - Storage sub-interfaces → TD-027 §6
> - Composite Backend + ReadOnlyBackend → TD-027 §6.1
> - Parallel LifecycleSubscriber + IDGenerator → TD-027 §6.2 + §8
> - Operation pattern → TD-027 §7
> - OperationApplier optional fast-path → TD-027 §7.2
> - Context discipline → TD-027 §9
> - Sentinel errors → TD-027 §10
> - Module boundary (internal/ + three-stage path) → TD-027 §21
> - Storage-vs-workflow separation + helper packages → TD-027 §5 + §14
>
> The gating bead `ddx-c6317784` and its dependents (`ddx-9c5bca8f`, `ddx-29f02cf4`, `ddx-8bf23be0`, `ddx-958b8fc3`, `ddx-8dd19492`, `ddx-900a8d38`, `ddx-e91a45c0`) reference TD-027 as their canonical acceptance criteria source. Do not edit this plan; amend TD-027 directly.

[Original plan content preserved in git history.]
