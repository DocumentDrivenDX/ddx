<bead-review>
  <bead id="ddx-a6e65634" iter=1>
    <title>Document DDx Prose Quality Support</title>
    <description>
CONTEXT
After the skill, rules, and advisory CLI command exist, DDx needs user-facing documentation that explains Prose Quality Support without overclaiming. The docs must be clear that DDx provides quality guidance and deterministic checks, not AI detection.

IN-SCOPE FILES
- docs/prose-quality.md or the docs path chosen by TD-036
- README.md or docs index only for a concise link
- CHANGELOG.md if project convention requires an entry for user-facing CLI behavior

REQUIRED DOC CONTENT
- What Prose Quality Support does and does not do.
- How to run `ddx doc prose --changed`.
- Example finding output.
- Config examples for mode, vocabulary accept/reject, severity, includes/excludes, and advisory/blocking policy if supported.
- Guidance on when to use the human-writing-support skill.
- Explicit statement: not an AI detector and not detector bypass.

OUT-OF-SCOPE
- New checker behavior.
- Review integration.
    </description>
    <acceptance>
1. User docs for Prose Quality Support exist and include `ddx doc prose --changed`, config examples, and example findings.
2. `rg -n "not an AI detector|detector bypass|advisory|vocabulary|technical|planning|public|human-writing-support" docs README.md CHANGELOG.md` returns matches in appropriate user-facing docs.
3. Docs explain that technical lists, headings, and project terminology are legitimate and can be configured rather than blanket-suppressed.
4. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:docs, kind:docs, prose-quality</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T112816-fb3e1761/manifest.json</file>
    <file>.ddx/executions/20260506T112816-fb3e1761/result.json</file>
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

  <diff rev="968aff466755408ff8a32b9e7c24dd3a02b66ce5">
<untrusted-data>
diff --git a/.ddx/executions/20260506T112816-fb3e1761/manifest.json b/.ddx/executions/20260506T112816-fb3e1761/manifest.json
new file mode 100644
index 000000000..2a213d0ff
--- /dev/null
+++ b/.ddx/executions/20260506T112816-fb3e1761/manifest.json
@@ -0,0 +1,46 @@
+{
+  "attempt_id": "20260506T112816-fb3e1761",
+  "bead_id": "ddx-a6e65634",
+  "base_rev": "75184cd7a8a87945d84d5b1b88ae15fbb37ac4f7",
+  "created_at": "2026-05-06T11:28:18.485063518Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a6e65634",
+    "title": "Document DDx Prose Quality Support",
+    "description": "CONTEXT\nAfter the skill, rules, and advisory CLI command exist, DDx needs user-facing documentation that explains Prose Quality Support without overclaiming. The docs must be clear that DDx provides quality guidance and deterministic checks, not AI detection.\n\nIN-SCOPE FILES\n- docs/prose-quality.md or the docs path chosen by TD-036\n- README.md or docs index only for a concise link\n- CHANGELOG.md if project convention requires an entry for user-facing CLI behavior\n\nREQUIRED DOC CONTENT\n- What Prose Quality Support does and does not do.\n- How to run `ddx doc prose --changed`.\n- Example finding output.\n- Config examples for mode, vocabulary accept/reject, severity, includes/excludes, and advisory/blocking policy if supported.\n- Guidance on when to use the human-writing-support skill.\n- Explicit statement: not an AI detector and not detector bypass.\n\nOUT-OF-SCOPE\n- New checker behavior.\n- Review integration.",
+    "acceptance": "1. User docs for Prose Quality Support exist and include `ddx doc prose --changed`, config examples, and example findings.\n2. `rg -n \"not an AI detector|detector bypass|advisory|vocabulary|technical|planning|public|human-writing-support\" docs README.md CHANGELOG.md` returns matches in appropriate user-facing docs.\n3. Docs explain that technical lists, headings, and project terminology are legitimate and can be configured rather than blanket-suppressed.\n4. `lefthook run pre-commit` passes.",
+    "parent": "ddx-ccda7a32",
+    "labels": [
+      "phase:2",
+      "area:docs",
+      "kind:docs",
+      "prose-quality"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T11:28:16Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T11:28:16.009815719Z",
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
+    "dir": ".ddx/executions/20260506T112816-fb3e1761",
+    "prompt": ".ddx/executions/20260506T112816-fb3e1761/prompt.md",
+    "manifest": ".ddx/executions/20260506T112816-fb3e1761/manifest.json",
+    "result": ".ddx/executions/20260506T112816-fb3e1761/result.json",
+    "checks": ".ddx/executions/20260506T112816-fb3e1761/checks.json",
+    "usage": ".ddx/executions/20260506T112816-fb3e1761/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a6e65634-20260506T112816-fb3e1761"
+  },
+  "prompt_sha": "2c781753eb9e388622b9495f9a23fd1f00b88d58b2f29da805984fa23ab55af8"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T112816-fb3e1761/result.json b/.ddx/executions/20260506T112816-fb3e1761/result.json
new file mode 100644
index 000000000..1de92eed1
--- /dev/null
+++ b/.ddx/executions/20260506T112816-fb3e1761/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-a6e65634",
+  "attempt_id": "20260506T112816-fb3e1761",
+  "base_rev": "75184cd7a8a87945d84d5b1b88ae15fbb37ac4f7",
+  "result_rev": "6d3c96b7d0b7720ee3aa0b8f28223d5e27d0e444",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-2b238845",
+  "duration_ms": 69107,
+  "tokens": 816039,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T112816-fb3e1761",
+  "prompt_file": ".ddx/executions/20260506T112816-fb3e1761/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T112816-fb3e1761/manifest.json",
+  "result_file": ".ddx/executions/20260506T112816-fb3e1761/result.json",
+  "usage_file": ".ddx/executions/20260506T112816-fb3e1761/usage.json",
+  "started_at": "2026-05-06T11:28:18.485359477Z",
+  "finished_at": "2026-05-06T11:29:27.593166089Z"
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
