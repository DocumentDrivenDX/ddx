---
name: helix-align
description: Create or claim the governing alignment bead, then run the HELIX alignment prompt. Use when the user wants `helix align` behavior or when functionality changes imply upstream spec or design reconciliation.
argument-hint: "[scope]"
---

# Align

Use this skill for repo-wide or area-scoped reconciliation reviews.
It is a convenience entrypoint to the stored alignment prompt plus tracker
workflow, not a separate execution surface.

This skill depends on the shared HELIX workflow library under `.ddx/plugins/helix/workflows/`.
If the shared resources are unavailable, treat the HELIX package as incomplete
and stop rather than guessing from code alone.

## When To Use

- the user asks for an alignment review, reconciliation, traceability audit, or drift analysis
- the project uses HELIX artifacts or a similar planning stack
- the user wants deterministic follow-up work in the tracker (`ddx bead`)
- the review should produce one durable consolidated report plus ephemeral review/execution issues

## Startup

Reference docs:

- `.ddx/plugins/helix/workflows/actions/reconcile-alignment.md`
- `.ddx/plugins/helix/workflows/templates/alignment-review.md`
- Tracker conventions: `ddx bead --help` (DDx FEAT-004)

## Core Rules

- Review top-down, not code-first.
- Planning intent comes from canonical artifacts, not from implementation.
- Use the HELIX authority order from the references.
- Use the built-in tracker only: `ddx bead` issues, parents, dependencies, `spec-id`, and labels.
- Before writing the report or filing follow-on issues, create or claim the
  governing `kind:planning,action:align` bead for this pass.
- Create or reconcile one review epic plus one review issue per functional area.
- Create execution issues only after the consolidated report exists.
- If follow-on work has real ordering constraints, encode them with parents and
  `ddx bead dep add` rather than prose.

## Follow-Up Bead Policy

Before the alignment review closes, **every gap that is not classified
ALIGNED** must have at least one corresponding execution bead in the tracker.
Do not close the review with prose recommendations that have no corresponding
bead — the ready queue is the only durable hand-off mechanism between review
and execution.

See Phase 7 (Execution Issues) and Issue Coverage Verification in
`.ddx/plugins/helix/workflows/actions/reconcile-alignment.md` for the exact rules and format.

## Output Model

Produce:

1. Governing `kind:planning,action:align` bead in the tracker
2. Review epic in the tracker
3. Review issues in the tracker
4. Durable report at `docs/helix/06-iterate/alignment-reviews/AR-YYYY-MM-DD[-scope].md`
5. Execution issues for every non-ALIGNED gap (filed before the review closes)

## Required Evidence

Every non-trivial finding must cite:

- planning evidence
- implementation evidence
- a classification
- a recommended resolution direction

Use these references as needed:

- [review-flow.md](references/review-flow.md)
- [alignment-report.md](references/alignment-report.md)
- [tracker.md](references/tracker.md)
