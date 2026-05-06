<bead-review>
  <bead id="ddx-b74e66f3" iter=1>
    <title>Design TD-036 prose quality pipeline</title>
    <description>
CONTEXT
This bead creates the technical design for the Prose Quality Support pipeline authorized by FEAT-027. The design should decide the implementation boundary for deterministic prose checks, including whether DDx shells out to Vale, embeds a small checker, or wraps a pluggable runner. The design must keep the first executable surface advisory and must define behavior when optional tooling is missing.

IN-SCOPE FILES
- docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md
- docs/helix/01-frame/features/FEAT-027-prose-quality-support.md only for depends_on/status updates if the local convention requires it

REQUIRED DESIGN CONTENT
- Checker boundary: Vale vs embedded checker vs wrapper, with install/runtime behavior and missing-tool behavior.
- Rule/vocabulary asset layout in the default DDx plugin.
- Config schema sketch for mode, severity, vocabulary accept/reject, includes/excludes, and advisory/blocking policy.
- CLI shape for ddx doc prose --changed and future ddx doc prose &lt;paths&gt;.
- Finding schema: file, line, rule id, severity, rationale, suggested edit.
- Fixture/golden test approach.
- Sequencing: skill/rules/doc command first; bead review integration later.

OUT-OF-SCOPE
- Implementing CLI code.
- Adding rule files.
- Wiring bead review.
    </description>
    <acceptance>
1. `test -f docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md` passes.
2. `rg -n "FEAT-027|checker boundary|Vale|embedded checker|missing|mode|severity|vocabulary|ddx doc prose --changed|file.*line.*rule|advisory|bead review" docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md` returns matches for the required design concepts.
3. The TD explicitly states findings are advisory by default and defines what happens when external checker dependencies are unavailable.
4. The TD includes a concrete fixture/golden test plan that can guide implementation beads.
5. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:docs, kind:design, prose-quality</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T072000-2ea693d9/manifest.json</file>
    <file>.ddx/executions/20260506T072000-2ea693d9/result.json</file>
  </changed-files>

  <governing>
    <ref id="FEAT-027" path="docs/helix/01-frame/features/FEAT-027-prose-quality-support.md" title="Feature: Prose Quality Support">
      <content>
<untrusted-data>
---
ddx:
  id: FEAT-027
  depends_on:
    - helix.prd
---
# Feature: Prose Quality Support

**ID:** FEAT-027
**Status:** Not Started
**Priority:** P2
**Owner:** DDx Team

## Overview

DDx Prose Quality Support defines deterministic, explainable prose checks for
governing artifacts and user-facing docs. The goal is clearer, more specific,
voice-preserving writing in DDx materials, not AI-detection evasion.

This feature introduces a default skill surface, a deterministic checker/rules
surface, and a docs command surface for reviewing changed prose. It produces
structural findings that can be reviewed, tracked, and later fed into review
workflow integration.

## Problem Statement

**Current situation:** Governing artifacts and documentation can drift into
vague, generic, high-level prose that reads fine on a first pass but weakens
document-driven development. When prose is too generic, maintainers spend more
time inferring intent than executing it.

**Pain points:**
- Technical docs hide concrete behavior behind broad claims and filler phrasing
- Planning docs lose actionable detail because the prose stops short of naming
  the actual decision, constraint, or boundary
- Public prose becomes polished but imprecise, which makes DDx harder to trust
  as a source of truth
- Reviewers have no deterministic prose-quality signal to point at when the
  problem is specific wording rather than missing requirements

**Desired outcome:** DDx can identify common AI-writing tropes and generic
prose patterns with deterministic rules, explain each finding, and suggest a
concrete edit without rewriting the author's voice away.

## Users and Content Modes

### Primary Users

- DDx maintainers
- Project maintainers using DDx governing artifacts
- Agents writing or reviewing docs

### Content Modes

Prose quality checks apply to three modes:

- **Technical** prose: feature specs, design docs, reference docs, and command
  descriptions
- **Planning** prose: beads, plans, roadmaps, and implementation notes
- **Public** prose: website copy, release notes, and externally visible docs

The rules may vary by mode, but they share the same deterministic output shape
and advisory default behavior.

## Product Surfaces

### 1. Default Skill

DDx ships a default skill surface that can guide agents toward concise,
specific prose and surface the prose-quality workflow when appropriate. The
skill is advisory: it helps authors improve prose, but it does not change the
meaning of the authored artifact.

### 2. Deterministic Checker and Rules

DDx defines prose-quality checks as deterministic rules over text. The checker
does not attempt to classify human vs. AI authorship and is not an AI detector.
It evaluates observed text against named rules and explains each match.

### 3. `ddx doc prose --changed`

DDx adds a docs surface for checking changed prose only. The command is meant
for pre-review and pre-merge use cases where maintainers want a focused,
diff-based advisory report instead of a full repository scan.

### 4. Later Review Integration

The feature reserves room for later review integration so prose findings can be
surfaced in review workflows. This feature does not define that integration
boundary beyond naming it as the next consumer of the same deterministic
finding format.

## Requirements

### Functional

1. **Deterministic rule evaluation** — prose checks must run as rule-based
   analysis over the selected text and return repeatable results for the same
   input.
2. **Mode awareness** — the checker must support technical, planning, and
   public prose modes, with rule application appropriate to the selected mode.
3. **Advisory default** — prose findings are advisory by default and do not
   block docs operations unless a later policy explicitly opts into blocking.
4. **Structural findings** — each finding must include:
   - file path
   - line or line range
   - rule id
   - severity
   - rationale
   - suggested edit
5. **Project vocabulary** — findings and suggestions must preserve project
   terminology when possible instead of rewriting terms into generic language.
6. **Changed-only review surface** — `ddx doc prose --changed` evaluates only
   changed prose by default and reports findings for the touched lines.
7. **Explainable output** — the checker must describe why a rule fired using
   concrete textual evidence from the input.
8. **Later review compatibility** — the finding format must be stable enough to
   be consumed by future review integration without changing the core rule
   model.

### Measurable Acceptance Criteria

The feature is considered successful when it can produce deterministic,
command-verifiable findings with the following structure for a changed prose
sample:

- `file`: the path of the changed document
- `line` or `line_range`: the affected location
- `rule_id`: a stable deterministic identifier
- `severity`: advisory severity value
- `rationale`: a short explanation tied to the observed text
- `suggested_edit`: a concrete replacement or rewrite suggestion

The feature must also be able to flag generic prose patterns such as vague
claims, filler transitions, and uncoupled abstractions while preserving the
document's own vocabulary and intended voice.

Findings must be structural rather than subjective. Each result must trace to a
specific file and line span, and the rationale must explain the triggered rule
in terms of observed text instead of a broad style judgment.

## Non-Goals

- No AI detector
- No detector bypass
- No default blocking behavior
- No automatic rewriting that strips authorial voice
- No implementation of the checker in this feature
- No CLI flag design beyond naming the deterministic prose check surface
- No plugin asset additions

## Rule Model

The prose-quality checker should reason in terms of named rules, not opaque
scores. A rule may detect one or more common prose tropes, but each emitted
finding must remain explainable and reviewable on its own.

### Example Rule Families

- Generic claim without specific subject or consequence
- Overly abstract sentence that omits the concrete artifact or action
- Repetition of empty emphasis phrases
- Passive or indirect phrasing where the responsible actor is known
- Voice drift that replaces project-specific terminology with generic wording

These families are examples of the target shape, not a final implementation
catalog.

## Command and Skill Boundaries

- The default skill helps authors and agents notice prose issues during writing
  and review
- The deterministic checker owns the actual finding generation
- `ddx doc prose --changed` is the primary command surface for reviewing only
  changed prose
- Review integration is a later consumer of the same structured findings

## Out of Scope

- Detector scoring heuristics that try to infer authorship
- Content transformation that rewrites style by default
- Blocking docs operations by default
- Choosing the final low-level implementation boundary beyond naming
  deterministic prose checks
- CLI flags or plugin assets beyond the prose review surface
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="af17ac8744b2da96fc2dfe6b6e980fa7846f6df4">
<untrusted-data>
diff --git a/.ddx/executions/20260506T072000-2ea693d9/manifest.json b/.ddx/executions/20260506T072000-2ea693d9/manifest.json
new file mode 100644
index 00000000..eb8f6501
--- /dev/null
+++ b/.ddx/executions/20260506T072000-2ea693d9/manifest.json
@@ -0,0 +1,168 @@
+{
+  "attempt_id": "20260506T072000-2ea693d9",
+  "bead_id": "ddx-b74e66f3",
+  "base_rev": "647f0d8f2ffc6d9c4402efc3e66ce98bdf7f5f49",
+  "created_at": "2026-05-06T07:20:03.695931473Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b74e66f3",
+    "title": "Design TD-036 prose quality pipeline",
+    "description": "CONTEXT\nThis bead creates the technical design for the Prose Quality Support pipeline authorized by FEAT-027. The design should decide the implementation boundary for deterministic prose checks, including whether DDx shells out to Vale, embeds a small checker, or wraps a pluggable runner. The design must keep the first executable surface advisory and must define behavior when optional tooling is missing.\n\nIN-SCOPE FILES\n- docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md\n- docs/helix/01-frame/features/FEAT-027-prose-quality-support.md only for depends_on/status updates if the local convention requires it\n\nREQUIRED DESIGN CONTENT\n- Checker boundary: Vale vs embedded checker vs wrapper, with install/runtime behavior and missing-tool behavior.\n- Rule/vocabulary asset layout in the default DDx plugin.\n- Config schema sketch for mode, severity, vocabulary accept/reject, includes/excludes, and advisory/blocking policy.\n- CLI shape for ddx doc prose --changed and future ddx doc prose \u003cpaths\u003e.\n- Finding schema: file, line, rule id, severity, rationale, suggested edit.\n- Fixture/golden test approach.\n- Sequencing: skill/rules/doc command first; bead review integration later.\n\nOUT-OF-SCOPE\n- Implementing CLI code.\n- Adding rule files.\n- Wiring bead review.",
+    "acceptance": "1. `test -f docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md` passes.\n2. `rg -n \"FEAT-027|checker boundary|Vale|embedded checker|missing|mode|severity|vocabulary|ddx doc prose --changed|file.*line.*rule|advisory|bead review\" docs/helix/02-design/technical-designs/TD-036-prose-quality-pipeline.md` returns matches for the required design concepts.\n3. The TD explicitly states findings are advisory by default and defines what happens when external checker dependencies are unavailable.\n4. The TD includes a concrete fixture/golden test plan that can guide implementation beads.\n5. `lefthook run pre-commit` passes.",
+    "parent": "ddx-ccda7a32",
+    "labels": [
+      "phase:2",
+      "area:docs",
+      "kind:design",
+      "prose-quality"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T07:20:00Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T01:17:48.789598653Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T011623-d4bc3a9a\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":643863,\"output_tokens\":9834,\"total_tokens\":653697,\"cost_usd\":0,\"duration_ms\":83149,\"exit_code\":0}",
+          "created_at": "2026-05-06T01:17:49.012390298Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=653697 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T01:17:54.867016395Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=67b84e89484c7006d568faa16f473bc548d7a70e\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T21:23:00-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=16327\noutput_bytes=0\nelapsed_ms=4174",
+          "created_at": "2026-05-06T01:18:00.157408093Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=67b84e89484c7006d568faa16f473bc548d7a70e\nbase_rev=22065f8767f822ba4d06978919c222746cb4ca7d",
+          "created_at": "2026-05-06T01:18:00.365274578Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T03:09:50.102049688Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T030847-d857bfae\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":336113,\"output_tokens\":4678,\"total_tokens\":340791,\"cost_usd\":0,\"duration_ms\":60555,\"exit_code\":0}",
+          "created_at": "2026-05-06T03:09:50.315289289Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=340791 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T03:09:57.315659895Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: unparseable\nattempt_count=1\nresult_rev=350222aa0bda780d32de41431cb8466cca342899\n\nreviewer: review-error: unparseable: reviewer output: unparseable JSON verdict: no JSON object found (raw output 66 bytes; see .ddx/executions/20260506T030958-b5c06c90)\nharness=claude\nmodel=opus\ninput_bytes=18654\noutput_bytes=66\nelapsed_ms=48807",
+          "created_at": "2026-05-06T03:10:49.460453173Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: unparseable"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=350222aa0bda780d32de41431cb8466cca342899\nbase_rev=4de60a8534463f8b0b088816070619e50396fbd4",
+          "created_at": "2026-05-06T03:10:49.658746496Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T05:53:35.551026836Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T055240-b918e978\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":398015,\"output_tokens\":3622,\"total_tokens\":401637,\"cost_usd\":0,\"duration_ms\":52553,\"exit_code\":0}",
+          "created_at": "2026-05-06T05:53:35.762560634Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=401637 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T05:53:41.700045802Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=166c80739a7bb62ee6399bb3f56a66b13d5088a3\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T01:58:47-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=20974\noutput_bytes=0\nelapsed_ms=4076",
+          "created_at": "2026-05-06T05:53:47.396003291Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=166c80739a7bb62ee6399bb3f56a66b13d5088a3\nbase_rev=f002007c2d1ffe64f98854b220758c92023a85ee",
+          "created_at": "2026-05-06T05:53:47.595743062Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T07:20:00.914916777Z",
+      "spec-id": "FEAT-027"
+    }
+  },
+  "governing": [
+    {
+      "id": "FEAT-027",
+      "path": "docs/helix/01-frame/features/FEAT-027-prose-quality-support.md",
+      "title": "Feature: Prose Quality Support"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260506T072000-2ea693d9",
+    "prompt": ".ddx/executions/20260506T072000-2ea693d9/prompt.md",
+    "manifest": ".ddx/executions/20260506T072000-2ea693d9/manifest.json",
+    "result": ".ddx/executions/20260506T072000-2ea693d9/result.json",
+    "checks": ".ddx/executions/20260506T072000-2ea693d9/checks.json",
+    "usage": ".ddx/executions/20260506T072000-2ea693d9/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b74e66f3-20260506T072000-2ea693d9"
+  },
+  "prompt_sha": "dd6e9d0731322530df865c60e1d11d64984b28c4baecc18e4c2c18df2f9831d9"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T072000-2ea693d9/result.json b/.ddx/executions/20260506T072000-2ea693d9/result.json
new file mode 100644
index 00000000..c02fb3dd
--- /dev/null
+++ b/.ddx/executions/20260506T072000-2ea693d9/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-b74e66f3",
+  "attempt_id": "20260506T072000-2ea693d9",
+  "base_rev": "647f0d8f2ffc6d9c4402efc3e66ce98bdf7f5f49",
+  "result_rev": "aa9ae829d25ecc6bae25e9786d0025e8d76bc330",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-4493b7b6",
+  "duration_ms": 95479,
+  "tokens": 615507,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T072000-2ea693d9",
+  "prompt_file": ".ddx/executions/20260506T072000-2ea693d9/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T072000-2ea693d9/manifest.json",
+  "result_file": ".ddx/executions/20260506T072000-2ea693d9/result.json",
+  "usage_file": ".ddx/executions/20260506T072000-2ea693d9/usage.json",
+  "started_at": "2026-05-06T07:20:03.696332306Z",
+  "finished_at": "2026-05-06T07:21:39.17618493Z"
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
