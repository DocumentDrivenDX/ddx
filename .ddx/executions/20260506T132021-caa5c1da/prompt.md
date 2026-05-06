<bead-review>
  <bead id="ddx-508a0297" iter=1>
    <title>Integrate prose findings into bead review as opt-in advisory output</title>
    <description>
CONTEXT
This is an explicit follow-up, not part of the first executable slice. Once `ddx doc prose --changed` has landed and advisory noise is known, DDx can expose prose findings inside bead review or a review hook.

IN-SCOPE FILES
- cli/cmd/bead_review*.go or existing review command files
- cli/internal/agent/review/lint hook code if TD-036 selects that path
- review tests using prose fixtures
- docs updates for review integration

REQUIRED BEHAVIOR
- Add an opt-in review surface such as `ddx bead review &lt;id&gt; --prose` or the exact command selected by TD-036.
- Findings remain advisory by default.
- Review output must distinguish prose-quality findings from acceptance/correctness findings.
- The integration must reuse the same checker/rule/config path as `ddx doc prose`.

OUT-OF-SCOPE
- Making prose findings globally blocking.
- Changing execute-loop selection or closure semantics based only on prose findings.
    </description>
    <acceptance>
1. A prose review integration command or flag exists as specified by TD-036 and is opt-in.
2. Tests prove review output includes prose findings separately from acceptance/correctness review findings.
3. Tests prove prose findings remain advisory by default and do not close/reopen/block beads on their own.
4. The integration reuses the same config/rule path as `ddx doc prose` rather than duplicating rule logic.
5. `cd cli &amp;&amp; go test ./cmd ./internal/agent/... ./internal/docprose/... -run "Prose|Review"` passes.
6. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:cli, area:agent, kind:feature, prose-quality, follow-up</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T131855-8f3bf99f/manifest.json</file>
    <file>.ddx/executions/20260506T131855-8f3bf99f/result.json</file>
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
4. opt-in bead review integration via `ddx bead review <id> --prose`,
   after the core command and result schema are stable

That sequencing keeps the first executable surface advisory and
deterministic before any review workflow starts consuming the findings.
It also means bead review wiring can reuse the same finding schema
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

  <diff rev="2896e75693ca3195ed723198fe6b15b3b7966e93">
<untrusted-data>
diff --git a/.ddx/executions/20260506T131855-8f3bf99f/manifest.json b/.ddx/executions/20260506T131855-8f3bf99f/manifest.json
new file mode 100644
index 000000000..ca1bd6e19
--- /dev/null
+++ b/.ddx/executions/20260506T131855-8f3bf99f/manifest.json
@@ -0,0 +1,170 @@
+{
+  "attempt_id": "20260506T131855-8f3bf99f",
+  "bead_id": "ddx-508a0297",
+  "base_rev": "b36844eecff032396df8e6d912721f666fd45320",
+  "created_at": "2026-05-06T13:18:57.58250937Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-508a0297",
+    "title": "Integrate prose findings into bead review as opt-in advisory output",
+    "description": "CONTEXT\nThis is an explicit follow-up, not part of the first executable slice. Once `ddx doc prose --changed` has landed and advisory noise is known, DDx can expose prose findings inside bead review or a review hook.\n\nIN-SCOPE FILES\n- cli/cmd/bead_review*.go or existing review command files\n- cli/internal/agent/review/lint hook code if TD-036 selects that path\n- review tests using prose fixtures\n- docs updates for review integration\n\nREQUIRED BEHAVIOR\n- Add an opt-in review surface such as `ddx bead review \u003cid\u003e --prose` or the exact command selected by TD-036.\n- Findings remain advisory by default.\n- Review output must distinguish prose-quality findings from acceptance/correctness findings.\n- The integration must reuse the same checker/rule/config path as `ddx doc prose`.\n\nOUT-OF-SCOPE\n- Making prose findings globally blocking.\n- Changing execute-loop selection or closure semantics based only on prose findings.",
+    "acceptance": "1. A prose review integration command or flag exists as specified by TD-036 and is opt-in.\n2. Tests prove review output includes prose findings separately from acceptance/correctness review findings.\n3. Tests prove prose findings remain advisory by default and do not close/reopen/block beads on their own.\n4. The integration reuses the same config/rule path as `ddx doc prose` rather than duplicating rule logic.\n5. `cd cli \u0026\u0026 go test ./cmd ./internal/agent/... ./internal/docprose/... -run \"Prose|Review\"` passes.\n6. `lefthook run pre-commit` passes.",
+    "parent": "ddx-ccda7a32",
+    "labels": [
+      "phase:2",
+      "area:cli",
+      "area:agent",
+      "kind:feature",
+      "prose-quality",
+      "follow-up"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T13:18:55Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T12:04:05.480618541Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T115251-15c8c0c3\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":10594004,\"output_tokens\":48553,\"total_tokens\":10642557,\"cost_usd\":0,\"duration_ms\":671192,\"exit_code\":0}",
+          "created_at": "2026-05-06T12:04:05.706692395Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=10642557 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T12:04:12.516448676Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=0fa2725138b65913443e992207cfd19826594c73\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T08:09:17-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=18007\noutput_bytes=0\nelapsed_ms=4205",
+          "created_at": "2026-05-06T12:04:17.847064784Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=0fa2725138b65913443e992207cfd19826594c73\nbase_rev=3ea39b5e4b986fdeb30349006bd982b58ed359b3",
+          "created_at": "2026-05-06T12:04:18.059022856Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T12:32:17.114167263Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T123100-43dc9cf9\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":809445,\"output_tokens\":5954,\"total_tokens\":815399,\"cost_usd\":0,\"duration_ms\":73997,\"exit_code\":0}",
+          "created_at": "2026-05-06T12:32:17.329138811Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=815399 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T12:32:24.008327979Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=f37d450aa3abbbc81d597fde23c5d69cf67403dc\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T08:37:29-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=19799\noutput_bytes=0\nelapsed_ms=4112",
+          "created_at": "2026-05-06T12:32:29.503933575Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=f37d450aa3abbbc81d597fde23c5d69cf67403dc\nbase_rev=bf7fb75d733a8fc5d153c38718362f33c9e0c641",
+          "created_at": "2026-05-06T12:32:29.708176463Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T12:55:35.971339327Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T125411-935f98b3\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":805994,\"output_tokens\":7368,\"total_tokens\":813362,\"cost_usd\":0,\"duration_ms\":81893,\"exit_code\":0}",
+          "created_at": "2026-05-06T12:55:36.190450153Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=813362 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T12:55:42.784415424Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=c68856e2a61634727e76586be76272cdb92dd178\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T09:00:48-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=22100\noutput_bytes=0\nelapsed_ms=4117",
+          "created_at": "2026-05-06T12:55:48.1839836Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=c68856e2a61634727e76586be76272cdb92dd178\nbase_rev=1fc58364ea2e3e1ace597b31d5ba37e89ba45729",
+          "created_at": "2026-05-06T12:55:48.415649667Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T13:18:55.107183949Z",
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
+    "dir": ".ddx/executions/20260506T131855-8f3bf99f",
+    "prompt": ".ddx/executions/20260506T131855-8f3bf99f/prompt.md",
+    "manifest": ".ddx/executions/20260506T131855-8f3bf99f/manifest.json",
+    "result": ".ddx/executions/20260506T131855-8f3bf99f/result.json",
+    "checks": ".ddx/executions/20260506T131855-8f3bf99f/checks.json",
+    "usage": ".ddx/executions/20260506T131855-8f3bf99f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-508a0297-20260506T131855-8f3bf99f"
+  },
+  "prompt_sha": "085387784c6a748ffc5142cf93092f9fb64fa8a2ec058d1b80a9de8cb162a8d5"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T131855-8f3bf99f/result.json b/.ddx/executions/20260506T131855-8f3bf99f/result.json
new file mode 100644
index 000000000..42a9205cb
--- /dev/null
+++ b/.ddx/executions/20260506T131855-8f3bf99f/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-508a0297",
+  "attempt_id": "20260506T131855-8f3bf99f",
+  "base_rev": "b36844eecff032396df8e6d912721f666fd45320",
+  "result_rev": "dbcfcc9daa49c080095d7827be9508954af9489c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-2defc0b0",
+  "duration_ms": 75760,
+  "tokens": 839089,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T131855-8f3bf99f",
+  "prompt_file": ".ddx/executions/20260506T131855-8f3bf99f/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T131855-8f3bf99f/manifest.json",
+  "result_file": ".ddx/executions/20260506T131855-8f3bf99f/result.json",
+  "usage_file": ".ddx/executions/20260506T131855-8f3bf99f/usage.json",
+  "started_at": "2026-05-06T13:18:57.583000078Z",
+  "finished_at": "2026-05-06T13:20:13.343534301Z"
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
