---
ddx:
  id: TD-040-cross-repo-blocker-recheck
  depends_on:
    - TD-031
    - TD-024
  related:
    - ddx-3b5ab8aa
    - ddx-438d7ee3
    - ddx-3fc5e579
    - ddx-a851b60b
---
# Technical Design: Structured Cross-Repo Blocker Recheck

## Purpose

This design specifies how DDx represents and rechecks a bead that is blocked on work in another repo.

The goal is not to invent a new lifecycle status. DDx already has the right persisted state for this problem:

- `StatusBlocked` requires an explicit external blocker reason in `cli/internal/bead/lifecycle.go:97-124`.
- `LifecycleBucketBlockedExternal` already classifies those beads in `cli/internal/bead/lifecycle.go:176-206`.
- `Store.ExternalBlocked()` already enumerates them in `cli/internal/bead/store.go:2557-2574`.

The gap is narrower: the blocker reason is free text, and reopening is still manual. This TD adds a structured cross-repo reference and a mechanical recheck path without replacing the existing external-blocker primitive.

## Problem Statement

Operators need to say "this bead is blocked on bead X in repo Y" and have DDx reopen it when that remote bead closes.

Today, that intent is encoded only as prose in `ExternalBlockerReason`, so:

- the blocker target is not machine-readable,
- the source repo cannot be checked automatically,
- and reopening depends on operator memory.

That works for one-off cases, but it does not scale across repeated cross-repo blockers or compactions.

## Existing Primitive

The current external-blocker path already does the right coarse-grained thing:

- `ValidateLifecycleTransition()` requires a non-empty `ExternalBlockerReason` when transitioning to `StatusBlocked`.
- `applyLifecycleTransitionMetadata()` stores that reason under `ExtraLifecycleExternalBlockerReason` and clears it when the bead leaves blocked state.
- `EvaluateLifecycleQueue()` maps `StatusBlocked` to `LifecycleBucketBlockedExternal`.

That means this design should layer on top of the current `blocked` semantics, not replace them.

## Proposed Shape

### Structured ref field

Store an additional machine-readable ref in `Bead.Extra` alongside the existing reason:

```json
{"repo":"<known-repos key>","bead":"<bead-id>"}
```

The ref is intentionally small:

- `repo` names the configured repo alias, not a filesystem path.
- `bead` names the target bead inside that repo.

The reason remains required and human-facing. The ref is an enrichment, not a substitute.

### Backward compatibility

Free-text-only blockers remain valid.

If a bead has:

- only `ExternalBlockerReason`, DDx keeps the current manual-reopen behavior.
- a structured ref plus `ExternalBlockerReason`, DDx can recheck it mechanically.

No new lifecycle status is introduced, and no existing blocked bead changes meaning.

## Recheck Contract

The recheck operation is a pure read-then-maybe-reopen flow:

1. Enumerate `Store.ExternalBlocked()`.
2. For each bead, inspect `ExtraLifecycleCrossRepoBlockerRef` if present.
3. Resolve the referenced repo using the loaded known-repos config.
4. Read the referenced bead's status.
5. If the target bead is `closed`, reopen the blocked bead using the existing manual-reopen transition path.
6. If the target bead is not closed, leave the bead blocked.
7. If the target cannot be resolved, leave the bead blocked and report why.

The reopening step must reuse the current blocked-to-open mechanics so the lifecycle rules stay centralized in `cli/internal/bead/lifecycle.go`.

## Local Repo Resolution

For known-repos entries backed by a local path:

- open a store rooted at the target repo's `.ddx/` directory,
- read the target bead by ID,
- and inspect its persisted status.

This is the low-risk path and should be the first implementation target.

If the path is missing, unreadable, or does not contain the target bead, the recheck must not mutate the blocked bead. It should report a specific unresolvable reason and exit cleanly.

## Remote / Federation Resolution

Known-repos entries that point at a node/project pair need a read-only federated lookup.

The design constraint here is simple:

- resolve the target bead's status through the existing read path,
- never issue a write/forward call to the remote side,
- and treat offline or degraded remote availability as a transient failure, not a false reopen.

If the federation read path cannot prove the target bead is closed, the blocked bead stays blocked.

## Reopen Behavior

When the target bead is confirmed closed, DDx reopens the blocked bead using the same lifecycle transition machinery used for manual operator reopen.

The reopen should:

- preserve lifecycle invariants,
- clear the structured blocker ref,
- clear the external blocker reason,
- and append an event that records the source repo, source bead, and observed remote status.

This ensures the reopened bead no longer looks externally blocked and does not immediately re-enter the same recheck path.

## Failure Modes

The recheck must be conservative. It should only reopen on positive confirmation that the source bead is closed.

Failure modes and required behavior:

- malformed structured ref: reject at write time, not at recheck time,
- unknown known-repos key: leave blocked and report a specific unresolvable reason,
- local repo path missing: leave blocked and report a transient or permanent resolution failure depending on the cause,
- target bead missing: leave blocked,
- remote offline or stale: leave blocked and retry later,
- remote degraded without the capability needed to read bead status: leave blocked,
- target not closed: leave blocked.

The important rule is that a failed lookup never changes the bead's lifecycle state.

## Command and Automation Boundary

The first concrete surface for this design is a standalone recheck command:

- `ddx bead recheck-blockers`

That command is the operator-visible entry point and the easiest way to verify the contract in tests.

The shared recheck logic should be library-first so a future drain-loop or pre-claim hook can call the same function without duplicating resolution or reopen logic.

## Compatibility Notes

This design intentionally preserves the current user experience for free-text blockers.

That means:

- existing blocked beads continue to show up in the blocked queue,
- operators can still reopen them manually,
- and existing `ExternalBlockerReason` data stays valid.

Structured refs only add precision and automation for the cases where the blocker target is actually knowable.

## Implementation Beads

This design is decomposed into build beads so implementation can proceed in small, testable increments:

- `ddx-438d7ee3` adds the structured blocker ref field and validation.
- `ddx-3fc5e579` implements the local-repo recheck path and CLI command.
- `ddx-a851b60b` extends the recheck path to federation-backed repos.

Those beads are the execution plan; this document is the contract they implement.

## Non-Goals

- No new lifecycle statuses.
- No replacement for `StatusBlocked` or `ExternalBlockerReason`.
- No automatic cross-repo write propagation.
- No cross-repo cycle detection in this design.

