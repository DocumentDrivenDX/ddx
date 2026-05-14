---
name: bead-lifecycle
description: Assess readiness, score, classify, and refine ddx beads. Used by ddx try/work hooks before claim or dispatch, after failed attempts, and for operator-invoked refinement.
---

# Bead Lifecycle

Assess readiness, score, classify, and refine DDx beads using the repository's
bead-authoring rubric. This skill is intentionally prompt-only: it gives agents
a stable contract for bead readiness checks before claim or dispatch,
failed-attempt triage after dispatch, and operator-invoked bead refinement.

Invocation prompts MUST begin with one of:

```text
MODE: readiness
MODE: intake
MODE: lint
MODE: triage
MODE: refine
```

Read that first line and apply only the matching mode contract. Do not blend
mode outputs. Return only the requested structured output unless the caller asks
for explanatory prose.

Ground all scoring in `docs/helix/06-iterate/bead-authoring-template.md`, which
is canonical for the 8-criterion sufficient-sub-agent-prompt rubric.

`MODE: intake` is the compatibility name for readiness mode. Treat it exactly
like `MODE: readiness` unless the caller gives a narrower schema.

## READINESS MODE

Use readiness mode before a bead is claimed or dispatched. The input is bead
JSON plus any available queue context, dependencies, prior attempt summaries,
or cheap repository evidence. The goal is to decide whether the bead is a
tractable, self-contained unit of work for an agent before DDx spends an
implementation attempt.

Readiness mode answers a different question than infrastructure preflight.
Do not classify provider outages, quota exhaustion, missing harnesses,
transport failures, git index locks, worktree creation failures, ENOSPC, or
missing lifecycle automation as bead defects. Report those as system readiness
failures so DDx can pause, preflight, clean up, or fail open according to the
current policy.

Check these bead-readiness failure reasons when the evidence is available:

1. `too_large` — the bead bundles multiple independent implementation scopes,
   broad subsystem rewrites, or acceptance criteria that should be split into
   child beads.
2. `ambiguous_scope` — the requested behavior, ownership boundary, target file,
   or non-scope is unclear or contradictory.
3. `missing_root_cause_or_current_state` — a fix bead lacks file:line-grounded
   root cause, or a feature/docs bead lacks a concrete current-state anchor.
4. `missing_verification` — acceptance criteria lack named `Test*` symbols,
   unique `go test -run` filters, package-level `go test` commands, or
   `lefthook run pre-commit`, after legitimate waivers.
5. `missing_code_path_assertion` — introduced behavior has no wired assertion
   or reachable integration check, after legitimate waivers.
6. `missing_dependency_or_parent` — dependency, parent, spec-id, or external
   prerequisite information needed to execute safely is absent or inconsistent.
7. `hidden_external_blocker` — progress depends on credentials, service state,
   human decisions, generated artifacts, or upstream work that is not encoded
   as a dependency or blocker.
8. `already_satisfied_candidate` — cheap evidence strongly suggests the AC is
   already met and the attempt would be a no-op unless verification proves
   otherwise.

Return JSON only:

```json
{
  "classification": "ready|needs_refine|needs_split|operator_required|system_unready",
  "tractability": "tractable|too_large|ambiguous|blocked|unknown",
  "score": 0,
  "rationale": "brief evidence-grounded explanation",
  "readiness_checks": [
    {
      "reason": "too_large|ambiguous_scope|missing_root_cause_or_current_state|missing_verification|missing_code_path_assertion|missing_dependency_or_parent|hidden_external_blocker|already_satisfied_candidate|system_unready",
      "verdict": "pass|fail|unknown|waived",
      "evidence": "smallest durable evidence, preferably file:line or bead field",
      "checkable_before_attempt": true
    }
  ],
  "suggested_fixes": [
    {
      "target": "title|description|acceptance|labels|parent|deps|split|system",
      "fix": "specific amendment, split, or operator action"
    }
  ],
  "rewrite": {
    "changed_fields": ["description", "acceptance"],
    "description": "PROBLEM\n...\n\nROOT CAUSE\n...\n\nPROPOSED FIX\n...\n\nNON-SCOPE\n...",
    "acceptance": "1. TestFoo verifies the rewritten acceptance.\n2. cd cli && go test ./internal/agent/... -run TestFoo -count=1\n3. lefthook run pre-commit"
  },
  "suggested_child_beads": [
    {
      "title": "imperative child title",
      "description": "standalone PROBLEM / ROOT CAUSE or CURRENT STATE / PROPOSED FIX / NON-SCOPE summary",
      "acceptance": [
        "1. Named verification criterion."
      ],
      "labels": [
        "phase:*",
        "area:*",
        "kind:*"
      ],
      "parent": "ddx-id",
      "deps": [
        "ddx-id: why"
      ]
    }
  ],
  "waivers_applied": [
    {
      "reason": "doc-only|epic|deletion|rename",
      "criteria": [
        "c"
      ],
      "evidence": "why the waiver is legitimate"
    }
  ]
}
```

Use the exact readiness classifications: `ready`, `needs_refine`,
`needs_split`, `operator_required`, and `system_unready`.

Legacy migration aliases only: older plans, beads, or session transcripts may
mention `safely_refinable`, `rewritten`, or `needs_human`. Treat
`safely_refinable` and `rewritten` as historical signals for `needs_refine`;
treat `needs_human` as a historical signal for `operator_required`. Never emit
those aliases in new readiness output.

`suggested_fixes` are advisory diagnostics for the author or operator. They
describe what should improve, but DDx must not treat them as a machine-applied
tracker patch. `rewrite` is the machine-
consumable replacement contract DDx may apply before claim when classification
is `needs_refine`.
`rewrite.changed_fields` is required whenever `rewrite` is present, and every
field listed there must also be present in `rewrite`.
`rewrite.description` / `rewrite.acceptance` must be strings, not arrays, so
the hook can write them directly through supported bead update commands.

Use `ready` only when the bead is tractable and the sufficient-prompt rubric
passes after legitimate waivers. Use `needs_refine` when targeted metadata or
AC edits can make the bead ready without changing intent. Use `needs_split`
when child beads are required. Use `operator_required` when intent, scope, or
external state cannot be safely inferred and autonomous execution should pause
for `status=proposed`. Use `system_unready` only when the readiness assessment
itself cannot run or the provided context proves an infrastructure blocker
rather than a bead defect.

In `MODE: intake`, emit a `rewrite` object only when the bead is not executable
as written but can be made executable by a narrow, semantics-preserving
metadata/AC rewrite. If a bead is executable as written, classify it as `ready`
even when the prose could be cleaner. Put non-blocking prompt quality
improvements in `suggested_fixes` only. Use `operator_required` only for actual
ambiguity, missing prerequisites, hidden external blockers, or unsafe scope
choices that prevent an implementation attempt.

`readiness_checks` MUST be a JSON array. It may be empty, and every entry MUST
be an object with `reason`, `verdict`, `evidence`, and
`checkable_before_attempt`. It must not be an object or string.

Legacy migration input aliases only: older records or prompts may mention `needs_human`.
Treat that as a one-way migration signal for
`operator_required` / `status=proposed`; never emit it as a current lifecycle
classification.

## LINT MODE

Use lint mode before dispatch. The input is bead JSON: title, type, labels,
parent, deps, description, acceptance criteria, and any custom fields available.

Rubric, scored one point each after applying waivers:

1. Title is one-line scope clarity: imperative, names subsystem and change.
2. Description has PROBLEM, ROOT CAUSE or CURRENT STATE with file:line when
   applicable, PROPOSED FIX, and NON-SCOPE.
3. Acceptance criteria are numbered, verifiable, and name specific `Test*`
   symbols or a unique `go test -run` filter unless waived.
4. Acceptance criteria include a wired-in assertion for introduced code paths
   unless waived.
5. Acceptance criteria include both a `cd cli && go test ./<pkg>/...` command
   and `lefthook run pre-commit`.
6. Labels include phase, area, kind, and cross-reference facets.
7. Parent is explicit and dependencies are either listed or explicitly stated
   as "No deps."
8. The bead reads as a sufficient sub-agent prompt: a competent agent with only
   the bead body can pick files, edit scope, and verification commands without
   asking.

Apply the rubric first, then apply any waiver from the waiver table only when
the bead type or labels clearly justify it. Do not use waivers to excuse vague
or missing context.

Return JSON only:

```json
{
  "score": 0,
  "rationale": "brief evidence-grounded explanation",
  "suggested_fixes": [
    {
      "criterion": "title|description|acceptance|code_path_assertion|verification_gate|labels|parent_deps|sufficient_prompt",
      "fix": "specific amendment to make"
    }
  ],
  "waivers_applied": [
    {
      "criterion": "c|d|implementation",
      "waiver": "doc-only",
      "reason": "why this bead qualifies"
    }
  ]
}
```

`LintResult.rationale` is a single string summary. Put per-criterion details in
`suggested_fixes` or `waivers_applied`; do not return an array for
`rationale`. Valid waiver values include `"doc-only"`, `"epic"`, and
`"deletion"`.

`score` is the number of pass or waived criteria after legitimate waivers. Use
integer scores from 0 through 8.

## TRIAGE MODE

Use triage mode after an attempt ends without straightforward success. The input
is the bead, an outcome event, and a relevant session log excerpt. Classify the
failure and recommend the next queue action.

Triage mode is advisory. It classifies evidence and recommends a TD-031 action
category; final queue mutation semantics are owned by
`docs/helix/02-design/technical-designs/TD-031-bead-state-machine.md` and the
`ddx try` / `ddx work` implementation. Do not invent persisted statuses or
queue mutation rules in skill output.

Valid classifications:

- `already_satisfied` — repository already meets the bead AC.
- `no_changes_unverified` — no-change evidence included verification, but it
  failed or could not run.
- `no_changes_unjustified` — no-change evidence lacks enough structured
  rationale to prove satisfaction or a durable blocker.
- `decomposed` — parent/epic/container work was decomposed or should be made
  execution-ineligible for ordinary queue execution.
- `blocked` — a hard external precondition prevents progress.
- `superseded` — work has been replaced by another bead or artifact.
- `routing` — model/provider/harness selection or capability mismatch.
- `quota` — rate limit, spend cap, or usage ceiling.
- `transport` — network, API, subprocess, serialization, or connector failure.
- `tests_red` — implementation exists but verification failed.
- `merge_conflict` — landing failed due to git conflicts.
- `review_block` — reviewer found blocking issues or requested changes.
- `timeout` — attempt exceeded time or idle limits.
- `recoverable` — transient infrastructure or time-based condition can plausibly
  succeed by retrying the same bead later.

Lifecycle interpretation:

- Operator-required outcomes are `status=proposed` in TD-031. Classify them
  with the narrow evidence class above and state the proposed/operator-required
  reason in `rationale`.
- Autonomous smart-agent investigation remains `status=open` and should
  continue through retry policy when the evidence suggests a stronger route can
  make progress.
- Hard external recheckable blockers are `status=blocked`; ordinary dependency
  waits are derived queue state, not `status=blocked`.
- Satisfied work is `status=closed`; intentionally not-doing work is
  `status=cancelled`.

Valid recommended actions:

- `close_already_satisfied`
- `release_claim_retry`
- `release_claim_mark_blocked`
- `release_claim_mark_superseded`
- `release_claim_wait_retry`
- `close_decomposed_or_mark_execution_ineligible`

Legacy migration input aliases only: historical triage may contain `needs_investigation` or `release_claim_needs_investigation`. Treat them as
one-way migration signals for proposed/operator-required review or autonomous
smart retry, depending on the evidence. Never emit them in new output.

Prefer the narrowest classification supported by the evidence. If the log shows
both a vague bead and a tool timeout, classify the first event that explains why
work could not be completed reliably.

Do not collapse recurring system failures into bead-quality failures. Recent
attempt evidence should be classified as follows:

- Provider exhaustion, quota ceilings, missing viable provider, missing harness,
  or transport errors are `routing`, `quota`, or `transport`.
- ENOSPC, failed worktree creation, evidence write failures, and git index lock
  contention are `recoverable` infrastructure failures unless the log proves a
  deterministic repository defect.
- A pre-execute checkpoint or pre-commit failure before implementation starts
  is `recoverable` or `no_changes_unjustified` unless the failing test output
  identifies a bead-owned code regression.
- `no_changes` with passing verification is `already_satisfied`; `no_changes`
  without enough proof is `no_changes_unjustified`.
- Red tests after the worker changed code are `tests_red`.

Return JSON only:

```json
{
  "classification": "already_satisfied|no_changes_unverified|no_changes_unjustified|decomposed|blocked|superseded|routing|quota|transport|tests_red|merge_conflict|review_block|timeout|recoverable",
  "recommended_action": "close_already_satisfied|release_claim_retry|release_claim_mark_blocked|release_claim_mark_superseded|release_claim_wait_retry|close_decomposed_or_mark_execution_ineligible",
  "rationale": "brief evidence-grounded explanation",
  "suggested_amendments": [
    {
      "target": "title|description|acceptance|labels|parent|deps",
      "amendment": "specific proposed change"
    }
  ],
  "suggested_followup_beads": [
    {
      "title": "imperative child or follow-up title",
      "description": "standalone problem/root-cause/proposed-fix/non-scope summary",
      "acceptance": [
        "numbered AC line with named verification"
      ],
      "labels": [
        "phase:N",
        "area:*",
        "kind:*"
      ],
      "parent": "ddx-id or empty when unknown",
      "deps": [
        "ddx-id: why"
      ]
    }
  ]
}
```

Use an empty `suggested_followup_beads` array when no child or follow-up bead is
needed. Suggested follow-up beads must be execution-ready drafts, not vague
reminders.

## REFINE MODE

Use refine mode when an operator asks to amend a bead before retry. The input is
the bead and optionally a prior triage output. Produce a YAML diff describing
only recommended tracker amendments. Do not run `ddx bead update`; this mode is
advisory unless the caller separately asks you to mutate the tracker.

Return YAML only:

```yaml
title:
  from: "current title"
  to: "refined imperative title"
description:
  add:
    - section: "PROBLEM"
      text: "standalone text to add"
    - section: "ROOT CAUSE"
      text: "file:line-grounded root cause or CURRENT STATE for features"
  replace:
    - from: "ambiguous existing sentence"
      to: "specific replacement"
acceptance:
  add:
    - "N. TestSpecificName verifies the behavior."
    - "N+1. cd cli && go test ./internal/pkg/... passes."
    - "N+2. lefthook run pre-commit passes."
  remove:
    - "vague or duplicate AC line"
labels:
  add:
    - "area:subsystem"
    - "kind:fix"
  remove:
    - "misleading-label"
parent:
  from: "old-parent-or-empty"
  to: "new-parent-or-empty"
deps:
  add:
    - "ddx-id: why this dependency matters"
  remove:
    - "ddx-id: why it is not a true dependency"
notes:
  - "short explanation of any waiver or non-obvious judgment"
```

Equivalent JSON output contract for callers that request JSON instead of YAML:

```json
{
  "title": {
    "from": "current title",
    "to": "refined imperative title"
  },
  "description": {
    "add": [
      {
        "section": "PROBLEM|ROOT CAUSE|CURRENT STATE|PROPOSED FIX|NON-SCOPE",
        "text": "standalone text to add"
      }
    ],
    "replace": [
      {
        "from": "ambiguous existing sentence",
        "to": "specific replacement"
      }
    ]
  },
  "acceptance": {
    "add": [
      "N. TestSpecificName verifies the behavior.",
      "N+1. cd cli && go test ./internal/pkg/... passes.",
      "N+2. lefthook run pre-commit passes."
    ],
    "remove": [
      "vague or duplicate AC line"
    ]
  },
  "labels": {
    "add": [
      "area:subsystem",
      "kind:fix"
    ],
    "remove": [
      "misleading-label"
    ]
  },
  "parent": {
    "from": "old-parent-or-empty",
    "to": "new-parent-or-empty"
  },
  "deps": {
    "add": [
      "ddx-id: why this dependency matters"
    ],
    "remove": [
      "ddx-id: why it is not a true dependency"
    ]
  },
  "notes": [
    "short explanation of any waiver or non-obvious judgment"
  ]
}
```

Omit YAML keys that have no proposed changes. Keep replacements specific enough
that an operator can translate them directly into `ddx bead update` and
`ddx bead dep` commands.

## WAIVER TABLE

Rubric-first, label override second: score the bead against all eight criteria,
then apply these waivers only when the bead type, labels, and content make the
waiver defensible.

| Bead type or label | Criterion skip | Conditions |
|---|---|---|
| `kind:doc`, `kind:docs`, or doc-only scope | Criterion (c), criterion (d) | The bead changes documentation only, names the doc path, includes `lefthook run pre-commit`, and remains sufficient for a documentation agent. |
| `type: epic` or `kind:epic` | Specific test-name part of criterion (c), criterion (d), concrete-implementation expectation | The bead is an aggregate container, lists child scope or decomposition criteria, includes parent/deps status, and names the collective verification gate expected from children. |
| `kind:deletion`, `kind:rename`, `kind:cleanup`, or deletion/rename scope | Criterion (d) | The bead cites the target file:line, states behavior preservation or removal intent, and acceptance criteria verify no stale references remain. |

Never waive criterion (h). A bead must still be a sufficient prompt for its
actual type.

## Examples

Curated examples live in `examples/`. Use them as calibration cases for lint,
triage, and refine output shape:

- `code-bug-lint.json`
- `feature-lint.json`
- `doc-only-waiver.json`
- `epic-waiver.json`
- `deletion-rename-waiver.json`
- `no-op-investigation-triage.json`
- `upstream-external-triage.json`
