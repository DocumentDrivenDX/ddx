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
  "classification": "ready|needs_refine|needs_split|needs_human|system_unready",
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

Use `ready` only when the bead is tractable and the sufficient-prompt rubric
passes after legitimate waivers. Use `needs_refine` when targeted metadata or
AC edits can make the bead ready without changing intent. Use `needs_split`
when child beads are required. Use `needs_human` when intent, scope, or
external state cannot be safely inferred. Use `system_unready` only when the
readiness assessment itself cannot run or the provided context proves an
infrastructure blocker rather than a bead defect.

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
  "rationale": [
    {
      "criterion": "a|b|c|d|e|f|g|h",
      "verdict": "pass|fail|waived",
      "reason": "brief evidence-grounded reason"
    }
  ],
  "suggested_fixes": [
    {
      "criterion": "a|b|c|d|e|f|g|h",
      "fix": "specific amendment to make"
    }
  ],
  "waivers_applied": [
    {
      "criterion": "c|d|implementation",
      "waiver": "doc-only|epic|deletion",
      "reason": "why this bead qualifies"
    }
  ]
}
```

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
- `needs_investigation` — evidence is ambiguous or asks for operator triage.
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

Valid recommended actions:

- `close_already_satisfied`
- `release_claim_retry`
- `release_claim_needs_investigation`
- `release_claim_mark_blocked`
- `release_claim_mark_superseded`
- `release_claim_wait_retry`
- `close_decomposed_or_mark_execution_ineligible`

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
  is `recoverable` or `needs_investigation` unless the failing test output
  identifies a bead-owned code regression.
- `no_changes` with passing verification is `already_satisfied`; `no_changes`
  without enough proof is `no_changes_unjustified`.
- Red tests after the worker changed code are `tests_red`.

Return JSON only:

```json
{
  "classification": "already_satisfied|no_changes_unverified|no_changes_unjustified|needs_investigation|decomposed|blocked|superseded|routing|quota|transport|tests_red|merge_conflict|review_block|timeout|recoverable",
  "recommended_action": "close_already_satisfied|release_claim_retry|release_claim_needs_investigation|release_claim_mark_blocked|release_claim_mark_superseded|release_claim_wait_retry|close_decomposed_or_mark_execution_ineligible",
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
