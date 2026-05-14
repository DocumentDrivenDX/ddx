---
ddx:
  id: AR-2026-05-06-failed-attempt-causes
  depends_on:
    - ADR-023
    - FEAT-004
---
# Alignment Review: Recent Failed Attempt Causes and Readiness Checkability

## Review Metadata

**Review Date**: 2026-05-06  
**Scope**: recent `ddx work` failures drawn from `.ddx/beads.jsonl` and matching execution bundles under `.ddx/executions/`  
**Status**: complete  
**Evidence window**: tracker rows created between `2026-04-10T22:20:09Z` and `2026-05-06T07:43:38Z`, with sampled execution directories from `.ddx/executions/20260425T181621-f0cb3b0d` through `.ddx/executions/20260506T074122-298ef08b`

## What happened

Recent attempts failed for several different reasons, but the evidence is split across tracker events, execution bundles, and the work's own logging split between pre-claim intake and pre-dispatch lint paths. The result is that operators see a mix of:

- provider and transport failures
- routing/preflight failures
- no_changes and close-policy failures
- review verdict failures
- decomposition/readiness failures
- ambiguous failures that need manual investigation

The relevant split in the worker loop is visible in `cli/internal/agent/execute_bead_loop.go:670` for pre-claim intake failures and `cli/internal/agent/execute_bead_loop.go:732-751` for pre-dispatch lint failures. Those two surfaces do not currently produce a single operator-facing failure taxonomy.

## Evidence Samples

| Bead | Observed snippet | Estimated cause | Bucket |
|---|---|---|---|
| `ddx-d6730314` | `reviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted` | The review provider was unavailable at dispatch time. | System readiness |
| `ddx-d6730314` | `The required verification gate ... is failing in pre-existing internal/server and internal/server/graphql retry-threshold tests` | The bead could not be proven green without unrelated fixes. | Post-attempt |
| `ddx-d7aca866` | `This bead is an epic ... and the execution tree is already at decomposition depth 2, so no further child-bead split is allowed in this pass.` | The bead was not claim-ready because it exceeded the allowed decomposition depth. | Bead readiness |
| `ddx-d778886c` | `agent made no commits` / `iteration_limit` | The attempt stopped without producing a commit or a valid no_changes record. | Close-policy |
| `ddx-ffda4fb5` | `checkpointing dirty worktree: git stash: error: could not write index` | The worktree was not in a claimable/repairable state. | System readiness |
| `ddx-ffda4fb5` | `rebase failed` / `land_conflict` | Merge choreography failed after the attempt had already started. | Post-attempt |
| `ddx-d7778715` | `GraphQL schema/resolver implementation is missing ...` and later `REVIEW:BLOCK` | The review found the implementation incomplete. | Post-attempt |
| `ddx-d758f207` | `reviewer output: unparseable JSON verdict: no JSON object found` | The reviewer returned malformed output. | Unknown |

## Buckets

### Bead Readiness

This bucket covers failures that bead-lifecycle can usually check before claim, because they are visible from bead metadata, description quality, dependency structure, or queueing constraints.

Examples:

- `ddx-d7aca866` was explicitly rejected as an epic that was already at decomposition depth 2.

Can bead-lifecycle check before claim? **Yes, mostly**.

Owning follow-up surfaces:

- `docs/helix/06-iterate/bead-authoring-template.md`
- `docs/helix/06-iterate/reliability-principles.md`
- `cli/internal/agent/execute_bead_loop.go`

### System Readiness

This bucket covers failures that depend on runtime environment health, provider availability, git/worktree state, or local resource contention.

Examples:

- `ddx-d6730314` transport failure: `claude quota-exhausted`
- `ddx-ffda4fb5` dirty worktree / index lock
- `ddx-d7778715` provider or execution failures that surface as infra-level blocks

Can bead-lifecycle check before claim? **Not reliably**. The loop can probe some conditions, but it cannot guarantee provider availability, git lock state, or local resource contention before dispatch.

Owning follow-up surfaces:

- `cli/internal/agent/execute_bead_loop.go`
- provider/routing code in the agent execution path
- workspace lock / checkout management around execution worktrees

### Post-Attempt

This bucket covers failures that happen after claim and execution have already started: review verdicts, land conflicts, test failures, and other attempt-specific outcomes.

Examples:

- `ddx-d6730314` pre-existing failing tests blocked proving the AC
- `ddx-ffda4fb5` `land_conflict` after the attempt had run
- `ddx-d7778715` `REVIEW:BLOCK`

Can bead-lifecycle check before claim? **No**. These outcomes depend on the code that ran, the reviewer, or the merge result.

Owning follow-up surfaces:

- `cli/internal/agent/execute_bead.go`
- `cli/internal/agent/execute_bead_loop.go`
- review and merge policy docs under `docs/helix/01-frame/`

### Close-Policy

This bucket covers failures where the attempt ends without satisfying DDx's close contract: missing `no_changes_rationale.txt`, agent exit without a commit, or policy-driven closure issues.

Examples:

- `ddx-d778886c` `agent made no commits`
- `ddx-d6730314` `no_changes_needs_investigation`

Can bead-lifecycle check before claim? **No**. These are end-of-attempt policy checks, not pre-claim readiness checks.

Owning follow-up surfaces:

- `cli/internal/agent/execute_bead_loop.go`
- `cli/cmd/bead.go`
- `docs/helix/01-frame/features/FEAT-019-agent-evaluation.md`

### Unknown

This bucket is for failures whose snippets are too malformed or too underspecified to classify confidently.

Examples:

- `ddx-d758f207` `reviewer output: unparseable JSON verdict: no JSON object found`

Can bead-lifecycle check before claim? **No**. Unknown failures are, by definition, not safely pre-classified.

Owning follow-up surfaces:

- execution telemetry normalization in the agent loop
- reviewer transport/schema validation
- the execution-bundle schema under `.ddx/executions/`

## Readiness Checkability Summary

| Bucket | Can bead-lifecycle check before claim? | Why |
|---|---|---|
| Bead readiness | Yes, mostly | The necessary signals are already in bead metadata, dependency edges, and authoring quality. |
| System readiness | No, not reliably | Provider availability, git locks, and resource contention are runtime conditions. |
| Post-attempt | No | These outcomes only exist after a claim has already run. |
| Close-policy | No | Closure policy is enforced at the end of the attempt, not at claim time. |
| Unknown | No | The evidence is too malformed or incomplete to pre-classify safely. |

## Follow-up Surfaces

- Bead readiness should be tightened in `docs/helix/06-iterate/bead-authoring-template.md` and the claim path in `cli/internal/agent/execute_bead_loop.go`.
- System readiness needs clearer health/routing boundaries in the agent execution loop and provider selection path.
- Post-attempt reporting needs a single, durable failure taxonomy that merges the intake and pre-dispatch logs instead of leaving them split across terminal output and tracker events.
- Close-policy issues need explicit end-of-attempt validation around commit/no_changes/rationale handling.
- Unknown failures need a narrower telemetry schema so malformed reviewer output is surfaced as a first-class category instead of collapsing into a generic stop.

## Closing Notes

- Documentation-only waiver: this report adds no generated references, so no new Go test is required.
- This review is intentionally descriptive only; it does not change tracker state, Go code, or live worker behavior.
- Verification gate: `lefthook run pre-commit` passes.
