# Recent Failed Attempt Causes And Readiness Checkability

Evidence window: `2026-05-05T01:31:09Z` through `2026-05-06T15:21:21Z`.
Sources sampled: `.ddx/beads.jsonl` event streams plus
`.ddx/executions/<attempt-id>/result.json` and
`.ddx/executions/<attempt-id>/manifest.json` for the attempts below.

This report groups the observed failures into the five buckets required by the
bead acceptance. The point is not perfect root-cause proof; it is a durable,
operator-facing estimate of what kind of failure happened and whether
bead-lifecycle readiness can catch it before claim.

## Bucket Summary

| Bucket | Example bead / attempt | Observed snippet | Can readiness check before claim? | Owning follow-up surface |
|---|---|---|---|---|
| Bead readiness | `ddx-9c5bca8f` / `20260506T092126-fa2440e8` | `The Axon backend boundary ... are already present in the current worktree` | Yes. This is the class bead readiness is supposed to catch early. | `cli/internal/agent/execute_bead_loop.go` pre-claim intake / readiness lint; `docs/helix/06-iterate/reliability-principles.md` and ADR-023 policy. |
| System readiness | `ddx-4ab66676` / `20260506T154152-4a9c452c`; `ddx-d0c656b0` / `20260506T160123-7b468ece` | `No space left on device`, `.git/index.lock` exists, `review-error: transport`, `claude quota-exhausted` | Mixed. Local disk and stale git-lock conditions are checkable before claim; provider quota exhaustion is only heuristically visible. | Host-resource preflight and git-lock checks in `cli/internal/agent/execute_bead_loop.go` plus the review transport path in the execute loop. |
| Post-attempt | `ddx-9f20a4bd` / `20260506T103413-b9f566cd` | `TestGraphQLPerfMatrix_Baseline ... p95=57.28ms > 50ms budget` | No. This only shows up after the targeted build/test run or checkpoint. | Post-attempt triage in `cli/internal/agent/execute_bead_loop.go` and the test budget surface in `cli/internal/server/perf`. |
| Close-policy | `ddx-8d747049` / `20260505T160236-8db426ba` | `(rationale absent)` and `agent exited without a commit or no_changes_rationale.txt` | No. This is a closure-contract failure after the attempt, not a claim-time readiness issue. | `cli/internal/agent/execute_bead_loop.go` close path, the no_changes contract, and tracker-close policy. |
| Unknown | `ddx-ff7c8ec9` / `20260505T012605-cd23d1fd` | `review-error: unparseable JSON verdict: no JSON object found` | No. The error is real, but the root cause is not recoverable from the payload alone. | Reviewer contract / parser boundary in the review pipeline. |

## Bucket Notes

### Bead Readiness

Representative failure: `ddx-9c5bca8f` at `20260506T092126-fa2440e8`.
The attempt returned `task_no_changes` because the targeted Axon backend and
its tests were already present in-tree. That is a true bead-readiness signal:
the work was already done, so the loop should be able to recognize that before
claiming the bead.

Readiness verdict: yes, this bucket is the one bead-lifecycle readiness should
own before claim.

### System Readiness

Representative failures:

- `ddx-4ab66676` at `20260506T154152-4a9c452c` hit `No space left on device`
  while creating the isolated worktree and execution artifacts.
- `ddx-4ab66676` in the same attempt also surfaced `.git/index.lock` contention
  in the pre-execute checkpoint.
- `ddx-d0c656b0` at `20260506T160123-7b468ece` later failed review with
  `review-error: transport` and `claude quota-exhausted`.

Readiness verdict: mixed. Disk-space and stale-lock problems are pre-claim
checks. Provider quota exhaustion is only partially checkable because it
depends on external provider state, not just local repository state.

### Post-Attempt

Representative failure: `ddx-9f20a4bd` at `20260506T103413-b9f566cd`.
The attempt reached verification, then failed because the package-wide command
hit `TestGraphQLPerfMatrix_Baseline` above budget. That is a post-attempt
problem: the code path is only visible after the test or checkpoint runs.

Readiness verdict: no, not before claim. Bead-lifecycle can only detect this
after a targeted verification pass.

### Close-Policy

Representative failure: `ddx-8d747049` at `20260505T160236-8db426ba`.
The loop had work to do, but the attempt ended with `agent exited without a
commit or no_changes_rationale.txt`. That is not a code defect; it is a
closure-contract failure.

Readiness verdict: no. The only reliable detector is the post-attempt close
path, because the issue is about missing final evidence, not claim-time state.

### Unknown

Representative failure: `ddx-ff7c8ec9` at `20260505T012605-cd23d1fd`.
The reviewer emitted `review-error: unparseable JSON verdict: no JSON object
found`. The payload proves something went wrong, but not enough to classify it
as provider, host, bead readiness, or close-policy.

Readiness verdict: no. Unknowns remain a post-hoc investigation bucket until
the review contract or parser boundary records a more specific cause.

## Closing Notes

Documentation-only waiver: this report adds analysis only. No new Go test is
required because it does not add generated references or executable code.

The next improvement surface is the bead-lifecycle readiness and triage
taxonomy: local host checks, provider/reviewer transport classification, and
close-contract outcomes should be surfaced as separate operator-facing events
instead of being left to ad hoc event inspection.
