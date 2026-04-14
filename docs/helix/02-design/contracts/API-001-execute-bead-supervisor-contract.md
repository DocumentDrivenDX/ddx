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
- one ordinary executable bead at a time
- bead claim and selection flow
- structural execution validation
- delegated execute-bead execution
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

The first shipped supervisor is intentionally **not** the epic worker. Epic
execution uses a separate worker mode because it needs a persistent branch,
worktree, and merge-commit landing contract.

`ddx agent execute-bead` remains the single owner of required execution
document resolution, required post-run checks, merge-eligibility evaluation,
and land/preserve mechanics, including creation and cleanup of its isolated
worktree. The supervisor only orchestrates queue selection, invokes that
command from the project root, and records the result states emitted by that
command.

The command requires a reproducible git base revision, not a pristine root
checkout. Tracked root changes may be checkpointed into an immutable base
commit before launch; ignored runtime scratch must not block the attempt.

The first release does not require multi-project discovery, cross-project
scheduling, or multi-host coordination. A worker instance binds to exactly one
project context for the duration of a loop.

Each attempt also has a tracked execution-evidence bundle under
`.ddx/executions/<attempt-id>/`. The supervisor may read and surface those
artifacts, but it does not reinterpret them as generic `exec-runs`
attachments; the bundle is the canonical tracked evidence for one
`execute-bead` attempt.

The bundle contents and write order are owned by `execute-bead` and
specified in FEAT-006 §"Execute-Bead Evidence Bundle". At minimum, the
bundle contains `prompt.md`, `manifest.json`, and `result.json`, and is
committed alongside the iteration (landed or preserved). The supervisor
must not treat the absence of any of those files as a normal post-run
state.

The prompt delivered to the agent for each attempt is compiled by the
**execute-bead prompt rationalizer** from bead fields and resolved governing
references (see FEAT-006 §"Prompt Rationalizer Contract"). The rationalizer
writes `prompt.md` to the bundle before the agent runs; this file is the
authoritative record of exactly what the agent received. The supervisor does
not author or modify the prompt.

Each iteration commit (landed or preserved under a hidden ref) carries the
canonical Git trailer set defined in FEAT-006 §"Canonical Git trailers":
`Ddx-Attempt-Id`, `Ddx-Worker-Id`, `Ddx-Harness`, `Ddx-Model`, and
`Ddx-Result-Status`. The supervisor relies on `Ddx-Attempt-Id` and
`Ddx-Result-Status` when projecting its observability surface from commit
history, and treats the trailer set as authoritative alongside the
supervisor-visible `status` field emitted by `execute-bead`.

## Single-Project State Machine

The supervision loop is a bounded state machine that repeats until the queue is
drained, the operator stops it, or a fatal project error occurs.

1. Resolve one project context from the current repository, explicit selector,
   or `ddx server` project binding.
2. Resolve the effective base revision and the governing execution contract
   snapshot that will govern this iteration.
3. Read the ready bead set for that project and order candidates by the
   supervisor's queue policy for that project context.
   - Ready non-epic beads are ordered ahead of ready epics at the same
     priority.
   - Open epics are excluded from this worker's launch set by default.
4. Run the generic execution-ready validator against the ordered candidate set
   to filter structural ineligible beads against the resolved base snapshot.
5. Claim the first validated bead atomically.
6. Run `ddx agent execute-bead` against the bead from the project root and
   capture its documented result schema.
7. Classify the outcome reported by `execute-bead` from the documented
   supervisor-visible `status` field:
   - structural validation failure before launch
   - execution failure
   - post-run check failure
   - land conflict after a successful attempt
   - success
8. Continue scanning the same project queue.

The loop must never infer readiness from HELIX-specific hidden policy. It uses
the explicit queue ordering policy, the shared validator output, and the
documented result schema as its source of truth.

## Epic Boundary

Epics are first-class tracker items, but they are not consumed by the ordinary
single-ticket supervisor.

- The single-ticket supervisor drains executable child beads and other
  non-epic work.
- A separate epic-scoped worker owns one persistent epic branch and worktree.
- That worker executes child beads sequentially inside the epic worktree,
  commits each child result on the epic branch, and merges the completed epic
  to the target branch with a regular merge commit.

The epic worker's branch, worktree, child-execution, merge-gate, and final
merge-commit contract is defined in API-002 (Epic Worker Surfaces) and the
"Epic Execution Workflow" section of FEAT-006. The single-ticket supervisor
must observe these boundary rules even though it does not operate on epics
itself:

1. **No epic claims.** This supervisor must never claim an epic bead. Ready
   epics are filtered out of its candidate set even when they pass the
   structural execution validator.
2. **Child beads during an active epic.** When a child bead belongs to an
   epic that is currently being executed by an epic worker, this supervisor
   must not claim that child. The epic worker is the authoritative consumer
   of in-flight epic children. The supervisor treats such children as
   already claimed by the epic worker.
3. **Child beads for idle epics.** Children of epics that have no active
   epic worker remain ineligible for this supervisor by default. Promoting
   an orphaned child to the single-ticket queue is a workflow-tool decision,
   not a supervisor decision.
4. **Child close semantics.** When the epic worker closes a child mid-epic,
   the child's result commit lives on the epic branch (`ddx/epics/<epic-id>`)
   and has not yet reached the target branch. This supervisor must treat
   such a child as "closed on epic" rather than "landed on target" when
   computing readiness for its own queue — downstream beads that depend on a
   child closed on epic are not yet eligible for single-ticket execution on
   the target branch until the epic itself merges.
5. **Final landing is not this supervisor's job.** Landing the completed
   epic with a `--no-ff` merge commit is owned by the epic worker. This
   supervisor does not evaluate epic merge gates, does not rebase the epic
   branch, and does not perform the final merge.

This contract therefore remains the single-ticket worker contract even after
epic execution is introduced elsewhere in the system.

## Execute-Bead Result Schema

The supervisor consumes only the documented result envelope emitted by
`ddx agent execute-bead`:

- `status`: one of `structural_validation_failed`, `execution_failed`,
  `post_run_check_failed`, `land_conflict`, or `success`
- `detail`: optional operator-facing text for logging and diagnostics

The supervisor must not infer state from free-form reason strings. It uses the
`status` field for control flow and may surface `detail` for observability.

`execute-bead` result classification is based on the managed worktree outcome,
not solely on agent-authored commits. If the agent leaves tracked file edits
without creating commits, `execute-bead` synthesizes the result commit and
then evaluates land/preserve semantics. Only a truly clean managed worktree may
be reported as `no_changes`.

## Validation And Retry Semantics

Structural validation happens before any irreversible execution step.

- If a bead fails structural validation, the supervisor records the blocker,
  unclaims the bead, and leaves it open for later correction.
- If execution starts and later fails, `execute-bead` preserves the iteration
  under a hidden ref using the documented naming scheme, and the supervisor
  records that result `status`.
- If post-run required checks fail, `execute-bead` preserves the iteration
  under a hidden ref, sets the documented failure `status`, and the supervisor
  records that result `status`.
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

- fetch origin before the rebase step so the local target tip reflects the
  latest remote state
- rebase the execution branch onto the latest target tip
- push `--ff-only` to origin; the remote's atomic ref-update is the
  serialization point for concurrent coordinators on other machines
- fast-forward the local target branch to match after a successful push
- reset the worker worktree to the updated branch tip after a successful land
- preserve the iteration under a hidden ref when the result cannot be landed or
  `--no-merge` semantics apply

On push rejection (another coordinator landed first), the losing coordinator
preserves the iteration under `refs/ddx/iterations/` and unclaims the bead
without force-pushing. The multi-machine coordinator topology and the full
conflict and divergence recovery contract are specified in SD-020.

These mechanics are owned by `ddx agent execute-bead`; the supervisor observes
the resulting landed or preserved state rather than performing them itself.

The preserved ref is the durable evidence for any non-landed attempt. It keeps
the exact iteration commit, associated metadata, and inspection trail intact.

## Minimal Control Surface

The shipped supervisor needs only a small operator surface:

- project selector or single-project default binding
- one-shot mode for a single claim-and-run cycle
- continuous loop mode with a poll interval
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
- last failure `status`
- current worktree path
- current execution bundle path
- current preserve ref, if any

## Acceptance Notes

- The contract stays single-project scoped for the first release.
- The loop consumes DDx execution validation instead of HELIX-hidden policy.
- The supervisor does not create or remove the execution worktree; that
  lifecycle remains inside `execute-bead`.
- Non-landed attempts are preserved by `execute-bead`, not rewritten by the
  supervisor.
- Successful attempts land by rebase plus fast-forward inside `execute-bead`
  and then reset the worker worktree to the new tip.
- The contract remains valid when later `ddx server` work uses project-scoped
  worker pools.
