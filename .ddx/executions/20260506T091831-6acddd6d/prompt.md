<bead-review>
  <bead id="ddx-f546d275" iter=1>
    <title>Ship human-writing-support skill in the default DDx plugin</title>
    <description>
CONTEXT
DDx should guide agents while they write or edit prose, separate from deterministic post-write checks. This bead ships a default-plugin skill named human-writing-support that adapts the stronger advisory guidance from human-writing-style approaches to DDx: preserve voice, prefer specific detail, avoid generic AI-like tropes, and do not trade away technical precision.

IN-SCOPE FILES
- library/.agents/skills/ddx/human-writing-support/SKILL.md or the default-plugin skill path established by FEAT-011/TD-036
- skills/ddx/human-writing-support/SKILL.md and .agents/.claude materialized copies only if this repo maintains generated checked-in mirrors
- skill validation/eval fixtures under the existing skills validation surface

REQUIRED SKILL BEHAVIOR
- Trigger for writing, rewriting, editing, or reviewing DDx prose, specs, ADRs, beads, docs, website copy, and public prose.
- Explicitly says it is not an AI detector or detector-bypass tool.
- Preserves user voice and technical precision.
- Applies different expectations for technical, planning, and public prose.
- Encourages concrete/checkable detail over generic importance language.
- Directs agents to run deterministic prose checks when available.

OUT-OF-SCOPE
- CLI checker implementation.
- Rule pack implementation.
- Bead review integration.
    </description>
    <acceptance>
1. The default DDx plugin includes a human-writing-support skill with valid agentskills.io frontmatter and a trigger description under the project convention limit.
2. `rg -n "not an AI detector|detector bypass|specific|voice|technical|planning|public|ddx doc prose"` against the skill file returns matches for required guidance.
3. Existing skill validation command passes, or if this repo uses EXEC-011, the focused validation for shipped skills passes.
4. The skill does not require agents to remove legitimate technical lists, headings, or DDx terminology.
5. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:skills, area:plugins, kind:feature, prose-quality</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T091604-645ceb67/manifest.json</file>
    <file>.ddx/executions/20260506T091604-645ceb67/result.json</file>
  </changed-files>

  <governing>
    <ref id="TD-036" path="docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md" title="Technical Design: Prose Quality Pipeline">
      <content>
<untrusted-data>
---
ddx:
  id: TD-036
  depends_on:
    - FEAT-027
  status: draft
---
# Technical Design: Prose Quality Pipeline

## Status

Draft. This TD closes the implementation-boundary gap left open by
FEAT-027 by deciding how DDx evaluates prose deterministically, how the
default plugin packages the assets, and how the first command surface
behaves when optional tooling is unavailable.

## Why This TD Exists

FEAT-027 defines the product problem and the required output shape for
deterministic prose checks, but it intentionally stops short of naming
the execution boundary. That leaves three open questions for the first
implementation bead:

- whether DDx should shell out to Vale
- whether DDx should embed a small checker
- whether DDx should wrap a pluggable runner and treat external tooling
  as optional

This TD answers the checker boundary question so the implementation
beads can start from a stable contract instead of inventing their own
architecture.

## Decision

DDx should ship a pluggable runner wrapper whose canonical first runner
is an embedded checker.

- The deterministic prose logic lives in DDx, not in Vale.
- Vale is an optional compatibility runner, not the required core.
- The wrapper is what lets DDx keep the first surface advisory, select a
  runner based on configuration, and degrade gracefully when external
  tooling is missing.

That boundary keeps the product contract stable:

- the checker is deterministic and explainable
- the first executable surface does not depend on a third-party install
- optional tooling can be added later without changing the finding
  schema

## Runtime Behavior

### Default path

The default path is the embedded checker. It runs in-process and does not
require Vale or any other external binary.

### Install behavior

The default DDx plugin should ship the embedded checker assets and their
rules/vocabulary/fixture tree. It should not require the user to install Vale
for the first supported surface to run.

### Optional runner path

The wrapper may delegate to Vale when a project explicitly selects that
runner. That path is compatibility-oriented and must produce the same
finding schema as the embedded checker.

### Missing-tool behavior

If the selected optional runner is unavailable, DDx must not turn the
prose check into a hard failure by default.

- In `policy: advisory`, DDx falls back to the embedded checker when it
  can, or reports a single advisory diagnostic that the optional runner
  was unavailable.
- In `policy: blocking`, DDx still prefers to run the embedded checker
  so the user gets concrete findings; an unavailable optional runner is
  reported as an execution diagnostic, not as a prose finding.
- When fallback is possible, the command should still return the
  embedded checker findings and keep the runner-missing diagnostic
  separate from the finding stream.

The important rule is that missing optional tooling never erases the
document analysis path. It only changes whether DDx can use the selected
runner implementation. The first executable surface stays advisory by
default even when the runner is optional: the user still gets findings,
and missing-tool state is surfaced separately as an execution diagnostic
instead of suppressing the scan.

## Default Plugin Asset Layout

The prose-quality assets belong in the default DDx plugin, not in a
project-specific check scaffold.

Proposed source layout:

- `library/checks/prose-quality/check.yaml`
- `library/checks/prose-quality/rules/`
- `library/checks/prose-quality/vocabulary/`
- `library/checks/prose-quality/fixtures/`

Installed layout:

- `.ddx/plugins/ddx/checks/prose-quality/check.yaml`
- `.ddx/plugins/ddx/checks/prose-quality/rules/`
- `.ddx/plugins/ddx/checks/prose-quality/vocabulary/`
- `.ddx/plugins/ddx/checks/prose-quality/fixtures/`

Layout rules:

- `rules/` stores named rule definitions grouped by mode.
- `vocabulary/` stores project terminology that the checker should
  preserve or prefer.
- `fixtures/` stores golden inputs and expected findings for regression
  tests.
- `check.yaml` wires the default command invocation and the runner
  selection defaults.

The layout is intentionally check-shaped rather than skill-shaped. The
skill can point authors at the workflow, but the asset tree owns the
deterministic rule definitions.

## Config Schema Sketch

The config schema needs to expose the policy knobs without making the
first release overfit to one runner.

```yaml
prose:
  mode: technical | planning | public
  severity: advisory | warning | blocking
  policy: advisory | blocking
  runner: embedded | vale | auto
  includes:
    - docs/helix/**
  excludes:
    - "**/*.generated.md"
  vocabulary:
    accept:
      - DDx
      - bead
      - execution
    reject:
      - system
      - solution
      - seamless
```

Semantics:

- `mode` selects the rule pack.
- `severity` is the severity attached to emitted findings.
- `policy` controls whether findings are advisory by default or can be
  elevated to blocking later.
- `runner` selects the implementation boundary.
- `includes` and `excludes` define the text selection scope.
- `vocabulary.accept` preserves project terms and domain terms.
- `vocabulary.reject` names generic substitutes the checker should flag
  when they replace project vocabulary.

`policy: advisory` is the default. That is the product-level default
required by FEAT-027 and the default the first executable surface must
honor.

## CLI Shape

The CLI surface is intentionally small.

### Primary command

`ddx doc prose --changed`

This is the first supported surface. It checks changed prose only and is
the default entry point for pre-review and pre-merge usage.

### Future direct-path command

`ddx doc prose <paths>`

This is the future explicit-path form. It should accept one or more
paths, reuse the same engine, and allow a caller to target a fixed set of
documents without relying on the diff.

### Shared behavior

Both forms must:

- load the same rule pack and vocabulary assets
- emit the same finding schema
- respect the same advisory/blocking policy
- preserve the same mode-specific rule selection

The only difference is how the input set is selected.

## Finding Schema

Findings must be structural and machine-readable. The canonical fields
are:

- `file`
- `line` or `line_range`
- `rule_id`
- `severity`
- `rationale`
- `suggested_edit`

Each finding therefore carries file, line, and rule identifiers together
with an explanation and a concrete edit suggestion.

Implementation may add helper fields such as `mode`, `snippet`, or
`runner`, but these core fields must remain stable.

Rules for each field:

- `file` is the relative path of the affected document.
- `line` or `line_range` identifies the touched text span.
- `rule_id` is a stable deterministic identifier, not a prose label.
- `severity` reflects the configured policy and the rule’s native
  impact.
- `rationale` must explain why the rule fired using observed text.
- `suggested_edit` must propose a concrete rewrite, replacement, or
  deletion.

The FEAT-027 principle applies here too: the output must be specific
enough that a later review consumer can reuse it without changing the
rule model.

## Fixture And Golden Test Plan

The first implementation bead should be guided by fixture-driven golden
tests rather than ad hoc assertions.

Recommended fixture set:

- one technical sample with vague claims and uncoupled abstractions
- one planning sample with generic roadmap language
- one public sample with voice drift and filler phrases
- one sample with accepted project vocabulary that must be preserved
- one sample with an unavailable optional runner

Recommended test shape:

- `TestProseCheckerChangedMode`
- `TestProseCheckerPathMode`
- `TestProseCheckerFindingSchema`
- `TestProseCheckerVocabularyPreservation`
- `TestProseCheckerMissingRunnerFallsBackOrReportsAdvisory`

Golden assertions should lock down:

- the chosen `rule_id`
- the affected line span
- the advisory default behavior
- the suggested edit text
- the missing-tool diagnostic text

The fixtures should be stable text files and JSON expectations so that a
future runner swap does not invalidate the acceptance corpus.

## Sequencing

The rollout should be staged in this order:

1. skill and rule assets
2. deterministic `ddx doc prose --changed`
3. direct-path `ddx doc prose <paths>`
4. bead review integration later, after the core command and result
   schema are stable

That sequencing keeps the first executable surface advisory and
deterministic before any review workflow starts consuming the findings.
It also means later bead review wiring can reuse the same finding schema
without re-litigating the checker boundary or the missing-tool contract.

## Non-Scope

- No CLI implementation in this TD
- No rule file content in this TD
- No Vale packaging requirement
- No bead review wiring in this TD
- No automatic prose rewriting
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="ebfc9c3174ad37054a5e561ccad3ab613fccb43b">
<untrusted-data>
diff --git a/.ddx/executions/20260506T091604-645ceb67/manifest.json b/.ddx/executions/20260506T091604-645ceb67/manifest.json
new file mode 100644
index 000000000..97b4a7436
--- /dev/null
+++ b/.ddx/executions/20260506T091604-645ceb67/manifest.json
@@ -0,0 +1,47 @@
+{
+  "attempt_id": "20260506T091604-645ceb67",
+  "bead_id": "ddx-f546d275",
+  "base_rev": "09a70357e1364a9f9908799b9d4ecb42b6b7c6f7",
+  "created_at": "2026-05-06T09:16:07.782012852Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-f546d275",
+    "title": "Ship human-writing-support skill in the default DDx plugin",
+    "description": "CONTEXT\nDDx should guide agents while they write or edit prose, separate from deterministic post-write checks. This bead ships a default-plugin skill named human-writing-support that adapts the stronger advisory guidance from human-writing-style approaches to DDx: preserve voice, prefer specific detail, avoid generic AI-like tropes, and do not trade away technical precision.\n\nIN-SCOPE FILES\n- library/.agents/skills/ddx/human-writing-support/SKILL.md or the default-plugin skill path established by FEAT-011/TD-036\n- skills/ddx/human-writing-support/SKILL.md and .agents/.claude materialized copies only if this repo maintains generated checked-in mirrors\n- skill validation/eval fixtures under the existing skills validation surface\n\nREQUIRED SKILL BEHAVIOR\n- Trigger for writing, rewriting, editing, or reviewing DDx prose, specs, ADRs, beads, docs, website copy, and public prose.\n- Explicitly says it is not an AI detector or detector-bypass tool.\n- Preserves user voice and technical precision.\n- Applies different expectations for technical, planning, and public prose.\n- Encourages concrete/checkable detail over generic importance language.\n- Directs agents to run deterministic prose checks when available.\n\nOUT-OF-SCOPE\n- CLI checker implementation.\n- Rule pack implementation.\n- Bead review integration.",
+    "acceptance": "1. The default DDx plugin includes a human-writing-support skill with valid agentskills.io frontmatter and a trigger description under the project convention limit.\n2. `rg -n \"not an AI detector|detector bypass|specific|voice|technical|planning|public|ddx doc prose\"` against the skill file returns matches for required guidance.\n3. Existing skill validation command passes, or if this repo uses EXEC-011, the focused validation for shipped skills passes.\n4. The skill does not require agents to remove legitimate technical lists, headings, or DDx terminology.\n5. `lefthook run pre-commit` passes.",
+    "parent": "ddx-ccda7a32",
+    "labels": [
+      "phase:2",
+      "area:skills",
+      "area:plugins",
+      "kind:feature",
+      "prose-quality"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T09:16:04Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T09:16:04.916189786Z",
+      "spec-id": "TD-036"
+    }
+  },
+  "governing": [
+    {
+      "id": "TD-036",
+      "path": "docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md",
+      "title": "Technical Design: Prose Quality Pipeline"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260506T091604-645ceb67",
+    "prompt": ".ddx/executions/20260506T091604-645ceb67/prompt.md",
+    "manifest": ".ddx/executions/20260506T091604-645ceb67/manifest.json",
+    "result": ".ddx/executions/20260506T091604-645ceb67/result.json",
+    "checks": ".ddx/executions/20260506T091604-645ceb67/checks.json",
+    "usage": ".ddx/executions/20260506T091604-645ceb67/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-f546d275-20260506T091604-645ceb67"
+  },
+  "prompt_sha": "398138cf8a6ede4f3666cac6e09c30c1a1dd39a7c0478f6d16bc8b98edbc6f5b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T091604-645ceb67/result.json b/.ddx/executions/20260506T091604-645ceb67/result.json
new file mode 100644
index 000000000..175f374b6
--- /dev/null
+++ b/.ddx/executions/20260506T091604-645ceb67/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-f546d275",
+  "attempt_id": "20260506T091604-645ceb67",
+  "base_rev": "09a70357e1364a9f9908799b9d4ecb42b6b7c6f7",
+  "result_rev": "241947588c425f47931f19ed98b679e8e6116873",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-f250ef6e",
+  "duration_ms": 136455,
+  "tokens": 1808800,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T091604-645ceb67",
+  "prompt_file": ".ddx/executions/20260506T091604-645ceb67/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T091604-645ceb67/manifest.json",
+  "result_file": ".ddx/executions/20260506T091604-645ceb67/result.json",
+  "usage_file": ".ddx/executions/20260506T091604-645ceb67/usage.json",
+  "started_at": "2026-05-06T09:16:07.782317352Z",
+  "finished_at": "2026-05-06T09:18:24.237831172Z"
+}
\ No newline at end of file
</untrusted-data>
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
