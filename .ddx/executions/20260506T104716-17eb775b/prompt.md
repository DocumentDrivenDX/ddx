<bead-review>
  <bead id="ddx-d8522140" iter=1>
    <title>Add advisory ddx doc prose --changed command</title>
    <description>
CONTEXT
The first executable surface for Prose Quality Support is an advisory command over changed Markdown files. This should give users value without blocking ddx work, ddx bead review, or ddx doc validate by default.

IN-SCOPE FILES
- cli/cmd/doc*.go or existing doc command files
- cli/internal/docprose/ checker integration code
- config schema/types for prose settings if not already landed
- command tests under cli/cmd and checker tests under cli/internal/docprose

COMMAND BEHAVIOR
- Add `ddx doc prose --changed`.
- Optionally support explicit paths if TD-036 includes them, but changed Markdown is required.
- Findings include file, line, rule id, severity, rationale, and suggested edit.
- Exit behavior is advisory by default; do not break unrelated workflows on findings.
- Respect project config for mode, vocabulary, includes/excludes, and severity threshold.
- If an external checker is selected by TD-036 and missing, emit a clear advisory infrastructure message per the TD rather than panicking.

OUT-OF-SCOPE
- Bead review integration.
- CI blocking policy.
- AI-authorship scoring.
    </description>
    <acceptance>
1. `ddx doc prose --changed` exists in CLI help and runs against changed Markdown files in a git repo.
2. Command output includes file, line, rule id, severity, rationale, and suggested edit for fixture-backed findings.
3. Tests prove default findings are advisory and do not use a blocking exit code unless an explicit config/flag from TD-036 enables it.
4. Tests prove config vocabulary accept/reject and mode settings affect findings.
5. Missing external-checker behavior, if applicable, is covered by a test and produces a clear infrastructure diagnostic without panic.
6. `cd cli &amp;&amp; go test ./cmd ./internal/docprose/... -run "Prose|Doc"` passes.
7. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:cli, area:docs, kind:feature, prose-quality</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T103906-969b1e93/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T103906-969b1e93/manifest.json</file>
    <file>.ddx/executions/20260506T103906-969b1e93/result.json</file>
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

  <diff rev="d3b3fad86185de06a1cda0a977e9c31e7b8f7308">
<untrusted-data>
diff --git a/.ddx/executions/20260506T103906-969b1e93/checks/production-reachability.json b/.ddx/executions/20260506T103906-969b1e93/checks/production-reachability.json
new file mode 100644
index 000000000..246408be7
--- /dev/null
+++ b/.ddx/executions/20260506T103906-969b1e93/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T103906-969b1e93/manifest.json b/.ddx/executions/20260506T103906-969b1e93/manifest.json
new file mode 100644
index 000000000..2895a583a
--- /dev/null
+++ b/.ddx/executions/20260506T103906-969b1e93/manifest.json
@@ -0,0 +1,47 @@
+{
+  "attempt_id": "20260506T103906-969b1e93",
+  "bead_id": "ddx-d8522140",
+  "base_rev": "9bd04b8b0b9203ca25436e8d7be33094519fab59",
+  "created_at": "2026-05-06T10:39:08.497345027Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d8522140",
+    "title": "Add advisory ddx doc prose --changed command",
+    "description": "CONTEXT\nThe first executable surface for Prose Quality Support is an advisory command over changed Markdown files. This should give users value without blocking ddx work, ddx bead review, or ddx doc validate by default.\n\nIN-SCOPE FILES\n- cli/cmd/doc*.go or existing doc command files\n- cli/internal/docprose/ checker integration code\n- config schema/types for prose settings if not already landed\n- command tests under cli/cmd and checker tests under cli/internal/docprose\n\nCOMMAND BEHAVIOR\n- Add `ddx doc prose --changed`.\n- Optionally support explicit paths if TD-036 includes them, but changed Markdown is required.\n- Findings include file, line, rule id, severity, rationale, and suggested edit.\n- Exit behavior is advisory by default; do not break unrelated workflows on findings.\n- Respect project config for mode, vocabulary, includes/excludes, and severity threshold.\n- If an external checker is selected by TD-036 and missing, emit a clear advisory infrastructure message per the TD rather than panicking.\n\nOUT-OF-SCOPE\n- Bead review integration.\n- CI blocking policy.\n- AI-authorship scoring.",
+    "acceptance": "1. `ddx doc prose --changed` exists in CLI help and runs against changed Markdown files in a git repo.\n2. Command output includes file, line, rule id, severity, rationale, and suggested edit for fixture-backed findings.\n3. Tests prove default findings are advisory and do not use a blocking exit code unless an explicit config/flag from TD-036 enables it.\n4. Tests prove config vocabulary accept/reject and mode settings affect findings.\n5. Missing external-checker behavior, if applicable, is covered by a test and produces a clear infrastructure diagnostic without panic.\n6. `cd cli \u0026\u0026 go test ./cmd ./internal/docprose/... -run \"Prose|Doc\"` passes.\n7. `lefthook run pre-commit` passes.",
+    "parent": "ddx-ccda7a32",
+    "labels": [
+      "phase:2",
+      "area:cli",
+      "area:docs",
+      "kind:feature",
+      "prose-quality"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T10:39:06Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T10:39:06.040598225Z",
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
+    "dir": ".ddx/executions/20260506T103906-969b1e93",
+    "prompt": ".ddx/executions/20260506T103906-969b1e93/prompt.md",
+    "manifest": ".ddx/executions/20260506T103906-969b1e93/manifest.json",
+    "result": ".ddx/executions/20260506T103906-969b1e93/result.json",
+    "checks": ".ddx/executions/20260506T103906-969b1e93/checks.json",
+    "usage": ".ddx/executions/20260506T103906-969b1e93/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d8522140-20260506T103906-969b1e93"
+  },
+  "prompt_sha": "d3b2f61bef97f1d449eaa5f5dc67bd20a99baaf9053a029ed5e6962c86cb29e8"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T103906-969b1e93/result.json b/.ddx/executions/20260506T103906-969b1e93/result.json
new file mode 100644
index 000000000..f6aa9bea8
--- /dev/null
+++ b/.ddx/executions/20260506T103906-969b1e93/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-d8522140",
+  "attempt_id": "20260506T103906-969b1e93",
+  "base_rev": "9bd04b8b0b9203ca25436e8d7be33094519fab59",
+  "result_rev": "b4964d5bf020cffdcf25a6442440a629e5e9db5f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-5e28e2d9",
+  "duration_ms": 477676,
+  "tokens": 8581044,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T103906-969b1e93",
+  "prompt_file": ".ddx/executions/20260506T103906-969b1e93/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T103906-969b1e93/manifest.json",
+  "result_file": ".ddx/executions/20260506T103906-969b1e93/result.json",
+  "usage_file": ".ddx/executions/20260506T103906-969b1e93/usage.json",
+  "started_at": "2026-05-06T10:39:08.497779943Z",
+  "finished_at": "2026-05-06T10:47:06.174542461Z"
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
