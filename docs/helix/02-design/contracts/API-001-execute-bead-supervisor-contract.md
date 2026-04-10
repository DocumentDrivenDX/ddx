---
ddx:
  id: API-001
  depends_on:
    - FEAT-004
    - FEAT-006
    - FEAT-010
    - FEAT-012
    - SD-012
    - SD-019
    - TD-010
---
# API/Interface Contract: Execute-Bead Supervisor

## Purpose

This contract defines the first shipped supervision loop for
`ddx agent execute-bead` style work. It is intentionally single-project scoped
and consumes DDx's generic execution readiness and execution-result surfaces
rather than any HELIX-specific hidden policy.

The supervisor contract covers:

- one project context at a time
- bead claim and selection flow
- structural execution validation
- isolated worktree execution
- observation of execute-bead result states
- operator controls and loop observability

## Boundary

DDx owns the generic substrate:

- bead storage and claim state
- execution-definition validation and run recording
- git worktree mechanics
- hidden-ref preservation of non-landed iterations

HELIX owns the workflow policy:

- which beads are eligible for the loop
- when the loop should run
- retry policy after a failure
- what operator-facing automation wraps the loop

`ddx agent execute-bead` remains the single owner of required execution
document resolution, required post-run checks, merge-eligibility evaluation,
and land/preserve mechanics. The supervisor only orchestrates queue selection
and records the result states emitted by that command.

The first release does not require multi-project discovery, cross-project
scheduling, or multi-host coordination. A worker instance binds to exactly one
project context for the duration of a loop.

## Single-Project State Machine

The supervision loop is a bounded state machine that repeats until the queue is
drained, the operator stops it, or a fatal project error occurs.

1. Resolve one project context from the current repository, explicit selector,
   or `ddx server` project binding.
2. Resolve the effective base revision and the governing execution contract
   snapshot that will govern this iteration.
3. Read the ready bead set for that project and choose the best candidate using
   the generic execution-ready validator against that resolved base snapshot.
4. Claim the chosen bead atomically.
5. Create an isolated execution worktree from the resolved base.
6. Run `ddx agent execute-bead` against the bead in that worktree and capture
   its reported result state.
7. Classify the outcome reported by `execute-bead`:
   - structural validation failure before launch
   - execution failure
   - post-run check failure
   - land conflict after a successful attempt
   - success
8. Remove the execution worktree.
9. Continue scanning the same project queue.

The loop must never infer readiness from HELIX-specific hidden policy. It uses
the shared validator output and the explicit contract snapshot as its source of
truth.

## Validation And Retry Semantics

Structural validation happens before any irreversible execution step.

- If a bead fails structural validation, the supervisor records the blocker,
  unclaims the bead, and leaves it open for later correction.
- If execution starts and later fails, `execute-bead` preserves the iteration
  under a hidden ref using the documented naming scheme, and the supervisor
  records that result state.
- If post-run required checks fail, `execute-bead` preserves the iteration
  under a hidden ref, attaches the failure reason to the result record, and
  the supervisor records that result state.
- If a rebase or fast-forward land fails after a successful run, `execute-bead`
  preserves the iteration and the preserved iteration remains the canonical
  evidence for that attempt.

Retry surface:

- operators or workflow tooling fix the governing docs, bead content, or
  environment
- the bead is unclaimed and made ready again
- the next loop pass may claim it and create a new iteration

Previous preserved refs remain immutable. A retry does not rewrite or reuse the
prior attempt.

## Land And Preserve Semantics

Success is only complete after the result is landed by rebase plus
fast-forward.

- rebase the execution branch onto the latest target tip
- fast-forward the target branch when the rebase result is clean
- reset the worker worktree to the updated branch tip after a successful land
- preserve the iteration under a hidden ref when the result cannot be landed or
  `--no-merge` semantics apply

These mechanics are owned by `ddx agent execute-bead`; the supervisor observes
the resulting landed or preserved state rather than performing them itself.

The preserved ref is the durable evidence for any non-landed attempt. It keeps
the exact iteration commit, associated metadata, and inspection trail intact.

## Minimal Control Surface

The shipped supervisor needs only a small operator surface:

- project selector or single-project default binding
- one-shot mode for a single claim-and-run cycle
- continuous loop mode with a poll interval
- worktree root or worktree naming root
- target branch selection
- explicit stop / shutdown handling

The contract deliberately does not couple these controls to any future
multi-project scheduler. A later multi-project server may host several
single-project workers, but each worker still obeys this contract one project
at a time.

## Observability

The supervisor should emit enough state to answer:

- which project is being scanned
- which bead was selected and why
- whether claim, validation, execution, or land failed
- which base revision and result revision were used
- where the active worktree lives
- what hidden ref was created for a preserved attempt
- how long the iteration took

At minimum, the loop should expose:

- current project
- current bead ID
- current state machine step
- last success timestamp
- last failure reason
- current worktree path
- current preserve ref, if any

## Acceptance Notes

- The contract stays single-project scoped for the first release.
- The loop consumes DDx execution validation instead of HELIX-hidden policy.
- Non-landed attempts are preserved by `execute-bead`, not rewritten by the
  supervisor.
- Successful attempts land by rebase plus fast-forward inside `execute-bead`
  and then reset the worker worktree to the new tip.
- The contract remains valid when later `ddx server` work uses project-scoped
  worker pools.
