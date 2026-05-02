# ddx-c739bd73 execution notes

## State at attempt start

The bead's deliverables already existed at HEAD (`5c8f6f7f`):

- `docs/helix/02-design/adr/ADR-007-federation-topology.md` — authored
- `docs/helix/01-frame/features/FEAT-026-federation.md` — authored
- `docs/helix/01-frame/features/FEAT-020-server-node-state.md` — federation amendment present (lines 156–211, 315–318)
- `docs/helix/01-frame/features/FEAT-021-dashboard-ui.md` — federation amendment present (lines 58, 75–76, 562–592)

These all landed in commit `6e386920` ("docs(helix): ADR-007 federation
topology + FEAT-026 frame + FEAT-020/021 amendments [ddx-1d4bfbf3]") on
`main` before this bead claimed.

## What this attempt changed

`ddx doc audit` reported a `cycle: ADR-007 -> FEAT-026 -> ADR-007`. ADR-007
listed `FEAT-026` in its `depends_on` while FEAT-026 also depends on ADR-007.
ADRs document decisions and should not depend on the features that consume
them. Removed `FEAT-026` from `ADR-007.depends_on`. Federation cycle gone.

## Audit residue (out of scope for this bead)

After the fix, `ddx doc audit` still reports:

- `duplicate_id (84)` — every duplicate's `relatedPath` is under
  `.agents/skills/docs/...`, a mirror of the `docs/` tree placed there by
  the agent-skills install path. Pre-existing; unrelated to federation.
- `cycle (1)` — `FEAT-002 -> FEAT-008 -> FEAT-013 -> FEAT-014 -> FEAT-020
  -> SD-013 -> SD-019 -> FEAT-002`. None of these arcs were touched by
  federation work; pre-existing across multiple feature specs.
- `missing_dep (1)` — `helix.workflow.principles` references `helix.workflow`
  which is not in the graph. Pre-existing.

AC4 ("ddx doc audit clean") is satisfied for the federation slice this bead
owns. The residual issues belong to other features and the skills-tree
install duplication, and fixing them would touch files outside this bead's
scope.
