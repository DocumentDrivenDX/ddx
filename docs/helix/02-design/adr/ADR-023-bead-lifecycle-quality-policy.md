---
ddx:
  id: ADR-023
  depends_on:
    - FEAT-004
    - FEAT-010
    - FEAT-011
---
# ADR-023: Bead Lifecycle Quality Policy

**Status:** Proposed (Accepted after operator review)
**Date:** 2026-05-04
**Authors:** Bead `ddx-9210f95a`

## Context

Reliability principle P7 ("BEAD = PROMPT") from `ddx-06b77652` says a bead's
description and acceptance criteria must be sufficient for a competent
sub-agent to execute without hand-curation. The canonical operational rubric is
`docs/helix/06-iterate/bead-authoring-template.md`.

Two audits show that this is not yet a durable property of the queue:

- `.ddx/executions/20260503T155638-bead57f0cb9e/bead-quality-audit-2026-05-03.md`
  found only 1 of 20 representative open beads scored 8/8.
- `.ddx/executions/20260504T030000-audit2/bead-quality-audit-remaining-2026-05-04.md`
  continued the audit across 108 remaining open beads and found 13 scored 8/8.

Evidence bead `ddx-f339c399` is the implementation evidence stream for turning
those audit findings into lifecycle guardrails. This ADR records the policy so
FEAT-004, FEAT-010, and FEAT-011 can amend their surfaces consistently.

## Policy

DDx will enforce bead authoring quality at two lifecycle points:

1. **Pre-dispatch lint.** Before `ddx try` starts an agent attempt, DDx runs a
   bead-lifecycle workflow skill against the candidate bead. The hook scores
   the bead against the rubric in
   `docs/helix/06-iterate/bead-authoring-template.md` and emits missing fields,
   criterion verdicts, and suggested repair commands.
2. **Post-attempt triage.** After each attempt, DDx runs the same
   bead-lifecycle workflow skill over the attempt evidence. The triage pass
   classifies whether the outcome reflects task quality, prompt quality,
   infrastructure failure, deterministic setup failure, or operator action.

The policy is intentionally one skill in two places, not two divergent
implementations. Pre-dispatch lint and post-attempt triage must share criterion
definitions, waiver semantics, and output vocabulary.

## Staged Rollout

The default rollout is **WARN-ONLY**. A low score prints diagnostics and records
ephemeral evidence, but dispatch proceeds. This allows existing queues to be
baselined without turning historical debt into an immediate work stoppage.

Projects may opt into **BLOCK** mode after their baseline is understood. In
BLOCK mode, a valid low lint score blocks `ddx try` and any `ddx work` iteration
that would dispatch that bead. BLOCK mode is a project or invocation policy; it
is not encoded in bead rows.

Promotion from WARN-ONLY to BLOCK should happen only after operators confirm:

- the waiver rules below cover legitimate doc, epic, deletion, and rename beads;
- the suggested `ddx bead update` recovery commands are usable;
- the hook's infrastructure failure behavior matches the fail-open rule.

## Waiver Model

The rubric has type-level waivers for bead classes where a criterion does not
apply. `docs/helix/06-iterate/bead-authoring-template.md` is canonical:

- doc-only beads may omit criterion (c) specific test names and criterion (d)
  wired-in code-path assertions, but must still name `lefthook run pre-commit`
  and be sufficient as standalone prompts;
- epic beads may satisfy criterion (c) and (d) through children, but still need
  clear aggregate test gates and explicit dependencies;
- pure deletion or rename beads may omit criterion (d), but must cite the
  target file:line and assert behavior preservation where applicable.

Rare per-bead exceptions are stored as labels in the existing bead label set:
`lint-waiver:<criterion>`, for example `lint-waiver:c`. The label waives one
criterion for one bead; it does not suppress the rest of the report.

Operators may force dispatch with `--force --reason <text>`. A force does not
silently bypass lint. DDx records an execution event containing the reason, the
criterion failures that were overridden, the actor, and the hook mode.

This ADR deliberately adds no new bead schema fields. Waivers use labels; lint
output is ephemeral attempt evidence under the execution directory.

## Fail-Open Behavior

Lifecycle hooks follow reliability principle P1: infrastructure failures do not
block useful work. If the hook binary, skill package, model invocation, evidence
write, or runtime environment fails, DDx proceeds and records the hook failure.

Only a successfully computed low lint score can block dispatch, and only when
BLOCK mode is active. WARN-ONLY mode never blocks. A hook crash is not a low
score; it is an infrastructure failure and therefore fail-open.

Post-attempt triage is also fail-open. If triage cannot run, the underlying
attempt outcome remains available to the existing retry and close policy. The
report should mark triage as unavailable rather than inventing a classification.

## Recovery UX

When BLOCK mode rejects dispatch, the operator-facing output must include:

- the bead id, title, type, and current score;
- each missing criterion by letter and short name;
- the exact missing fields or weak fields, such as absent ROOT CAUSE file:line,
  absent `Test*` name, absent `cd cli && go test ...`, or absent
  `lefthook run pre-commit`;
- suggested `ddx bead update` commands for description, acceptance, and labels;
- the waiver options that apply to the bead type, if any;
- the `--force --reason` form for break-glass dispatch.

Example output should be command-oriented, not advisory prose:

```text
ddx try ddx-12345678 blocked by bead-quality lint: score 5/8
missing:
  (b) ROOT CAUSE WITH FILE:LINE
  (c) AC names specific Test*
  (e) lefthook run pre-commit
repair:
  ddx bead update ddx-12345678 --description-file /path/to/description.md
  ddx bead update ddx-12345678 --acceptance-file /path/to/acceptance.txt
override:
  ddx try ddx-12345678 --force --reason "operator accepted doc-only exception"
```

The same diagnostics are persisted in the attempt evidence bundle for later
audit. This keeps recovery local to the operator while preserving the audit
trail needed to evaluate whether the policy is improving queue quality.

## Consequences

- FEAT-004 owns the bead-side authoring quality rubric, label-based waivers,
  and the decision not to add schema fields.
- FEAT-010 owns hook insertion points in `ddx try` / `ddx work`, attempt
  evidence capture, and `OutcomeReason` reporting beside `Disrupted`.
- FEAT-011 owns the skill packaging model: the root `ddx` skill plus nested
  workflow skills that can be invoked by lifecycle hooks.
- Queue cleanup can be staged without blocking all historical work on day one.
- A force path exists, but every force is explicit and auditable.

## Alternatives Considered

- **Create a new FEAT for bead quality enforcement.** Rejected. FEAT-004,
  FEAT-010, and FEAT-011 already own the storage, execution, and skill
  surfaces; a separate FEAT would duplicate their contracts.
- **Add schema fields for lint status or waivers.** Rejected. The score is
  derived evidence and may change with the rubric. Labels are sufficient for
  durable waivers, and execution evidence is sufficient for per-attempt output.
- **Block immediately by default.** Rejected. The audits show substantial
  existing queue debt. WARN-ONLY first gives operators a baseline and avoids
  turning a documentation cleanup into an execution outage.

## References

- `docs/helix/06-iterate/bead-authoring-template.md`
- `docs/helix/06-iterate/reliability-principles.md` / `ddx-06b77652`
- `.ddx/executions/20260503T155638-bead57f0cb9e/bead-quality-audit-2026-05-03.md`
- `.ddx/executions/20260504T030000-audit2/bead-quality-audit-remaining-2026-05-04.md`
- `ddx-f339c399` evidence bead
- FEAT-004, FEAT-010, FEAT-011 amendments that cross-link this ADR
