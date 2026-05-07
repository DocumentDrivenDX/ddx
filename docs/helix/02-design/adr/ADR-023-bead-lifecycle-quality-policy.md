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

Both checkpoints invoke the same nested bead-lifecycle workflow skill under the
`ddx` skill tree. The skill is responsible for translating the rubric into
agent-readable findings; DDx remains responsible for hook timing, evidence
placement, and outcome classification.

Post-attempt triage classifications feed
[`TD-031`](../technical-designs/TD-031-bead-state-machine.md). ADR-023 does not
define final queue mutation policy; TD-031 remains the source of truth for
whether an attempt closes, stays open for human triage, becomes blocked, is
superseded, or receives a retry cooldown.

### Staged Rollout And Factory Mode

The default rollout is WARN-ONLY.

- WARN-ONLY mode reports the lint score, missing criteria, waiver matches, and
  suggested remediation, then proceeds with dispatch.
- BLOCK mode is opt-in until the queue baseline confirms that ordinary
  execution-ready beads can consistently satisfy the rubric.
- BLOCK mode may stop only on a valid low lint score after applicable rubric
  skips and label waivers are applied. It must not stop on hook crashes,
  missing skill files, transient filesystem errors, or malformed lint output.
- Reliable factory mode is BLOCK mode plus default adversarial review. In that
  mode, poorly specified work is improved, decomposed, or blocked before claim
  rather than allowed to consume implementation attempts.

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
- In WARN-ONLY mode, all valid lint results proceed after reporting.
- In BLOCK mode, only a valid post-waiver lint result below the configured
  threshold blocks dispatch.
- Post-attempt triage failures never erase the attempt result. They annotate the
  report and evidence so retry policy and operators can distinguish "agent made
  a bad attempt" from "quality infrastructure failed."

### Safe Improvement, Decomposition, And Recovery UX

Bead readiness assessment may update a bead before execution only when the
update is intent-preserving and grounded in durable context. Safe improvements
include normalizing the description into the authoring template, adding
discovered file:line evidence, adding an obvious subsystem test command, and
wiring deterministic labels, parent, or dependencies. Readiness must block for
human input instead of inventing acceptance criteria, changing scope, choosing
between conflicting requirements, or guessing a missing governing artifact.

If readiness finds the bead too broad, it decomposes before claim. Every parent AC
must map to at least one child AC or be explicitly marked `needs_human` or
`non_scope`; token-overlap metrics are heuristics, not proof of preservation.
The parent is blocked/decomposed with child ids and the AC map in evidence.

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

## Failure Taxonomy

Failures that occur during bead claim, attempt, and post-attempt triage fall
into three distinct classes. Every owner surface must correctly classify a
failure before applying retry, close, block, or triage policy.

### Bead-Readiness Reasons

A bead-readiness reason indicates a deficiency in the bead description,
acceptance criteria, or metadata — not in the execution substrate.
`BeadReadinessHook` detects these reasons before claim and before the
implementation worktree is created.

| Reason | Meaning |
|---|---|
| `too_large` | The bead spans unrelated subsystems, has more than ~6 ACs, or would require changes across more than ~5 files in unrelated packages. |
| `ambiguous_scope` | The description names conflicting or underspecified requirements such that a competent agent cannot safely pick a file to edit without operator clarification. |
| `missing_root_cause_or_current_state` | The description lacks a `file:line` pointer or equivalent current-state evidence that anchors the proposed change. |
| `missing_verification` | No AC names a test function, `go test -run` filter, or other verifiable command that confirms the fix. |
| `missing_code_path_assertion` | ACs exist but none wire to a deterministic assertion (test, lint rule, or schema guard) that would fail if the fix regressed. |
| `missing_dependency_or_parent` | A stated or implied predecessor bead or spec-id is absent from the dependency graph or the bead's labels. |
| `hidden_external_blocker` | The work cannot proceed until an out-of-repo condition (API access, upstream release, human decision) is met that the bead does not mention. |
| `already_satisfied_candidate` | Pre-claim inspection suggests the bead's ACs are already met by the current codebase without any change. |

### System-Readiness Reasons

A system-readiness reason indicates that the execution substrate — not the
bead — is unfit for a claim or worktree creation. System-readiness failures
must not be classified as bead defects. DDx records the infrastructure failure
and, in WARN-ONLY mode, proceeds; in BLOCK mode, it stops without penalizing
the bead.

| Reason class | Examples |
|---|---|
| Provider / quota | No configured AI provider, quota exhausted, API auth failure |
| Transport | Network unreachable, TLS error, upstream 5xx |
| Missing harness | Named harness absent from `.ddx/config.yaml` or harness binary not on PATH |
| ENOSPC | Temporary worktree root or durable evidence root out of bytes or inodes |
| Git lock | `.git/index.lock`, ref lock held by another process, or `git worktree add` fails due to concurrent operation |
| Worktree / evidence write failure | `git worktree add` or evidence directory creation fails for reasons other than ENOSPC or lock |

### Post-Attempt Reasons

`PostAttemptTriageHook` produces a post-attempt reason after the attempt has
produced its owned evidence. These reasons feed retry reporting and operator
UX; they do not replace the outcome taxonomy in TD-031 §5.

| Reason | Meaning |
|---|---|
| `tests_red` | The attempt committed a change but one or more required tests fail against the committed state. |
| `merge_conflict` | The worktree result could not be merged to the base branch due to conflicting changes. |
| `review_block` | An adversarial reviewer returned a BLOCKING finding that the implementation did not address. |
| `no_changes_unverified` | The attempt produced no commit; a `verification_command` was supplied but failed or could not run. |
| `no_changes_unjustified` | The attempt produced no commit without structured rationale sufficient to prove satisfaction or a durable blocker. |
| `already_satisfied` | The attempt confirmed (via a passing `verification_command`) that the bead's ACs are already met. |

### Classification Invariant

Infrastructure and system-readiness failures must not be classified as bead
defects. A quota failure, transport error, git lock, or ENOSPC classified
as a bead-readiness problem would incorrectly block or downgrade the bead.
Both `BeadReadinessHook` and `PostAttemptTriageHook` must select from the
correct class before applying any policy action.

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
