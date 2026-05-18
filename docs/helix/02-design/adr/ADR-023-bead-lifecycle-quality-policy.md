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
**Authors:** bead `ddx-9210f95a`

## Context

Reliability principle P7 ("BEAD = PROMPT") is planned in
`docs/helix/06-iterate/reliability-principles.md` by bead `ddx-06b77652`.
The principle says a bead's description and acceptance criteria must be enough
for a competent sub-agent to execute without hand-curation, but it does not
define the enforcement mechanism, waiver model, staged rollout, or recovery UX.

The quality gap is already visible in evidence:

- Audit 1, `.ddx/executions/20260503T195715-4725673a/bead-quality-audit-2026-05-03.md`,
  found repeated misses in root-cause detail, concrete test names, and
  self-contained scope.
- Audit 1 snapshot,
  `.ddx/executions/20260503T155638-bead57f0cb9e/bead-quality-audit-2026-05-03.md`,
  directly operationalizes P7 and records the first 20-bead scoring pass.
- Audit 2,
  `.ddx/executions/20260504T030000-audit2/bead-quality-audit-remaining-2026-05-04.md`,
  extended the scan across the remaining open queue and calls out evidence bead
  `ddx-f339c399`.
- `docs/helix/06-iterate/bead-authoring-template.md` is the canonical
  8-criterion authoring rubric and explicitly defines valid criterion skips for
  epic, doc-only, deletion, and rename beads.

Three existing feature specs already own the affected surfaces:

- FEAT-004 owns bead schema, validation hooks, labels, and evidence.
- FEAT-010 owns `ddx try`, `ddx work`, run records, and attempt outcomes.
- FEAT-011 owns the DDx skill packaging model that gives agents reusable
  workflow guidance.

## Decision

DDx adopts bead-lifecycle quality enforcement as policy over the existing
bead, execution, and skill surfaces. No new bead schema fields are introduced.
No new top-level FEAT is created.

### Policy

Every automated bead attempt has two quality checkpoints:

1. **Bead readiness assessment.** Before `ddx work` claims a bead or `ddx try`
   launches the implementation invocation, DDx evaluates whether the bead is
   tractable and actionable. It checks the bead description, acceptance
   criteria, labels, parent, dependency metadata, spec-id, prior attempt
   history, and the canonical rubric in
   `docs/helix/06-iterate/bead-authoring-template.md`. The implementation hook
   may still advertise `MODE: intake` for legacy compatibility only; the
   canonical product concept is bead readiness assessment.
2. **Post-attempt triage.** After an attempt finalizes, DDx triages the result
   against the same lifecycle quality policy so a low-quality prompt failure,
   missing rationale, empty review block, or structurally ambiguous outcome is
   classified and surfaced consistently.

The readiness checkpoint and the post-attempt triage checkpoint are distinct:
readiness decides whether a bead should be claimed or rewritten before
execution, lint/rubric scoring measures prompt quality inside the readiness
evaluation, and triage classifies the attempt after evidence exists.

Readiness treats `open` as the forward-progress lane. WARN-ONLY findings are
proceed-by-default findings unless they expose a hard operator-required parking
condition. See FEAT-010 "Forward-Progress Readiness Policy" for the complete
specification of how open/proposed status transitions interact with readiness
assessment.

Both checkpoints invoke the same nested bead-lifecycle workflow skill under the
`ddx` skill tree. The skill is responsible for translating the rubric into
agent-readable findings; DDx remains responsible for hook timing, evidence
placement, and outcome classification.

Post-attempt triage classifications feed
[`TD-031`](../technical-designs/TD-031-bead-state-machine.md). ADR-023 does not
define final queue mutation policy; TD-027 (state machine, transitions) and TD-031 (operational outcome → mapping) together remain the source of truth for
whether an attempt closes, stays open, moves to `status=proposed` for operator
triage, becomes externally blocked, is superseded, or receives a retry cooldown.

### Staged Rollout And Factory Mode

**The default readiness mode is WARN-ONLY.** This is a load-bearing default
that ensures beads proceed to execution by default (open is the forward-progress
lane) while producing diagnostic quality signals. See FEAT-010
"Forward-Progress Readiness Policy" for how WARN-ONLY interacts with
`status=open` and `status=proposed`.

- **WARN-ONLY mode** reports the lint score, missing criteria, waiver matches,
  and suggested remediation, then proceeds with dispatch. Recoverable rubric
  gaps, lint noise, uncertain model judgment, and readiness infrastructure
  failures are proceed findings, not hard parking findings. Readiness warnings
  are observable but non-blocking unless a hard operator-required condition
  exists.
- **BLOCK mode** is opt-in until the queue baseline confirms that ordinary
  execution-ready beads can consistently satisfy the rubric. In BLOCK mode,
  readiness may stop dispatch on a valid low lint score, but it still honors
  the operator-promotion non-regression clause (see below).
- **BLOCK mode enforcement:** BLOCK mode may stop only on a valid low lint
  score after applicable rubric skips and label waivers are applied, or on a
  hard operator-required blocker. It must not stop on hook crashes, missing
  skill files, transient filesystem errors, or malformed lint output.
- **Reliable factory mode** is BLOCK mode plus default adversarial review. In
  that mode, poorly specified work is improved, decomposed, or blocked before
  claim rather than allowed to consume implementation attempts.

This makes the policy measurable before it becomes a dispatch gate and avoids
freezing the queue while legacy beads are being retrofitted.

### Waiver Model

Waivers exist at two levels:

- **Rubric-level by bead type.** The skips documented in
  `docs/helix/06-iterate/bead-authoring-template.md` are built into lint:
  doc-only beads may omit criterion (c) and (d), epic beads may satisfy those
  criteria through children, and pure deletion or rename beads may omit
  criterion (d) while preserving behavior.
- **Per-bead labels.** Rare exceptions use labels of the form
  `lint-waiver:<criterion>`, for example `lint-waiver:c`. Labels are the
  durable waiver store because FEAT-004 already defines labels as free-form
  bead metadata and no schema change is warranted.

Operator override uses `--force --reason <text>`. The override records an event
in the bead evidence stream naming the criterion, mode, actor, reason, and lint
summary. It does not silently bypass the hook and it does not mutate the rubric.

### Fail-Open Behavior

The lifecycle hooks follow reliability principle P1's fail-open posture for
infrastructure failures.

- If the hook cannot run, the workflow skill is missing, output cannot be
  parsed, or evidence cannot be written, DDx records the infrastructure failure
  and proceeds.
- In WARN-ONLY mode, `ddx work` may skip model-backed lint scoring after the
  readiness pass so advisory quality checks do not stall execution or spam bead
  evidence. Any valid lint result that is produced proceeds after reporting.
- In BLOCK mode, only a valid post-waiver lint result below the configured
  threshold blocks dispatch.
- Post-attempt triage failures never erase the attempt result. They annotate the
  report and evidence so retry policy and operators can distinguish "agent made
  a bad attempt" from "quality infrastructure failed."

### Safe Improvement, Decomposition, And Recovery UX

Bead readiness assessment may update a bead before execution only when the
update is intent-preserving and grounded in durable context. Safe improvements
include validated replacement rewrites of the description, normalizing the bead
body into the authoring template, adding discovered file:line evidence, adding
an obvious subsystem test command, and wiring deterministic labels, parent, or
dependencies.

Validated replacement is preferred over append-only amendment when replacement
would make the bead a better implementation prompt. Prompt fitness is part of
the quality policy: readiness should leave the next agent enough durable context
to execute without dragging along stale, duplicated, or chat-shaped noise. A
one-line vague bead may need to expand into a standalone task; a noisy bead may
need to shrink. The original bead text remains auditable through the readiness
event and before/after hashes; it does not have to be embedded verbatim in the
replacement prompt body.

The validator proves preservation from durable anchors rather than asking
whether the new bead is merely "in the spirit" of the old one. Acceptance
criteria, non-scope, governing artifact references, dependencies, named
files/tests, and still-valid root-cause evidence are explicit commitments and
must be preserved or replaced with current durable evidence. If the old bead
contains contradictions, stale anchors, or missing product choices, readiness may
remove the confusing wording only when the replacement clearly marks the
unresolved part as operator-required (`status=proposed`) or preserves it as an
explicit constraint.
Readiness must block for human input instead of inventing acceptance criteria,
changing scope, choosing between conflicting requirements, deleting unresolved
constraints, or guessing a missing governing artifact.

### Operator-Promotion Non-Regression Clause

Operator promotion via `triaged` event is a durable override signal. When an
operator moves a bead from `status=proposed` back to `status=open`, they are
explicitly accepting the bead as actionable despite readiness concerns. This
acceptance is a hard policy decision that readiness must honor.

**Non-regression requirement:** Readiness must not move an operator-promoted bead
back to `status=proposed` for the same rule or finding unless one of the
following holds:

1. A prompt-relevant field has changed materially (description rewritten,
   acceptance criteria added/modified, labels/parent/deps changed), **or**
2. The operator explicitly requests re-triage via `--force --reason`.

This clause prevents readiness from recreating a proposed→open→proposed loop
after an operator has made an explicit acceptance decision. The original rule
that triggered the `proposed` parking may remain a readiness warning; it simply
must not automatically park the bead again without explicit operator re-request.
The durable record of the operator decision is the `triaged` event and the bead
transitioning to `status=open`.

If readiness finds the bead too broad, it decomposes before claim. Every parent AC
must map to at least one child AC or be explicitly marked `operator_required` or
`non_scope`; token-overlap metrics are heuristics, not proof of preservation.
The parent remains `status=open` with child dependency edges when decomposition
succeeds, or moves to `status=proposed` when the split would be lossy.

When BLOCK mode stops dispatch, the operator-facing output must be actionable.
It prints:

- bead id, score, mode, and blocked criteria
- the missing fields or malformed sections in plain language
- the waiver labels already applied and the labels that would waive each
  failed criterion
- suggested `ddx bead update` commands for adding description, acceptance,
  labels, parent, notes, or dependency metadata
- the `--force --reason` form for exceptional dispatch, with the note that an
  evidence event will be recorded

Recovery output must not require reading the lint implementation or an
out-of-band report to fix an ordinary authoring problem.

## Consequences

- FEAT-004 owns the label-based waiver storage and the no-new-schema rule.
- FEAT-010 owns hook insertion in `ExecuteBeadLoopRuntime`, attempt
  classification, and the `OutcomeReason` report field.
- FEAT-011 owns nested workflow-skill packaging so the same rubric guidance can
  be reused by lint, triage, review, breakdown, replay, and benchmark flows.
- Legacy beads can continue running in WARN-ONLY while the queue is baselined.
- Once BLOCK mode is enabled, dispatch quality becomes explicit and auditable
  instead of depending on operator memory.

## Non-Goals

- A new FEAT for bead quality; the existing feature specs own the affected
  surfaces.
- The separate `in_progress` eligibility bug.
- Cross-project skill packaging, which remains deferred to FEAT-015.
- New fields in `.ddx/beads.jsonl`; waivers use labels and evidence events.

## References

- `ddx-06b77652` — RELIABILITY-PRINCIPLES bead for P7.
- `ddx-f339c399` — evidence bead called out by Audit 2.
- `docs/helix/06-iterate/bead-authoring-template.md` — canonical rubric and
  criterion-skip policy.
- `.ddx/executions/20260503T195715-4725673a/bead-quality-audit-2026-05-03.md`
  — Audit 1.
- `.ddx/executions/20260503T155638-bead57f0cb9e/bead-quality-audit-2026-05-03.md`
  — Audit 1 scoring snapshot.
- `.ddx/executions/20260504T030000-audit2/bead-quality-audit-remaining-2026-05-04.md`
  — Audit 2.
- `docs/helix/01-frame/features/FEAT-004-beads.md` — bead metadata,
  validation, labels, and evidence.
- `docs/helix/01-frame/features/FEAT-010-task-execution.md` — try/work lifecycle
  and attempt outcomes.
- `docs/helix/01-frame/features/FEAT-011-skills.md` — DDx skill packaging.
