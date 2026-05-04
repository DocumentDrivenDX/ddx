---
name: bead-lifecycle
description: Score, classify, and refine ddx beads. Used by ddx try hooks pre-dispatch (lint mode) and post-attempt (triage mode); operator-invocable for refine mode.
---

# Bead Lifecycle

Score, classify, and refine DDx beads using the repository's bead-authoring
rubric. This skill is intentionally prompt-only: it gives agents a stable
contract for bead quality checks before dispatch, failed-attempt triage after
dispatch, and operator-invoked bead refinement.

Invocation prompts MUST begin with one of:

```text
MODE: lint
MODE: triage
MODE: refine
```

Read that first line and apply only the matching mode contract. Do not blend
mode outputs. Return only the requested structured output unless the caller asks
for explanatory prose.

Ground all scoring in `docs/helix/06-iterate/bead-authoring-template.md`, which
is canonical for the 8-criterion sufficient-sub-agent-prompt rubric.

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

Valid classifications:

- `already_satisfied` — repository already meets the bead AC.
- `ambiguous` — bead text lacks enough detail for a reliable retry.
- `investigated_no_path` — investigation found no viable implementation path.
- `decomposed` — bead is too large and should be replaced by children.
- `operator_input_needed` — human decision or external secret/access is needed.
- `routing` — model/provider/harness selection or capability mismatch.
- `quota` — rate limit, spend cap, or usage ceiling.
- `transport` — network, API, subprocess, serialization, or connector failure.
- `tests_red` — implementation exists but verification failed.
- `merge_conflict` — landing failed due to git conflicts.
- `review_block` — reviewer found blocking issues or requested changes.
- `timeout` — attempt exceeded time or idle limits.

Valid recommended actions:

- `refine_bead_and_retry`
- `retry_with_more_context`
- `file_children_and_supersede`
- `escalate_to_operator`
- `close_as_already_done`
- `wait_and_retry`
- `give_up`

Prefer the narrowest classification supported by the evidence. If the log shows
both a vague bead and a tool timeout, classify the first event that explains why
work could not be completed reliably.

Return JSON only:

```json
{
  "classification": "already_satisfied|ambiguous|investigated_no_path|decomposed|operator_input_needed|routing|quota|transport|tests_red|merge_conflict|review_block|timeout",
  "recommended_action": "refine_bead_and_retry|retry_with_more_context|file_children_and_supersede|escalate_to_operator|close_as_already_done|wait_and_retry|give_up",
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
