<bead-review>
  <bead id="ddx-8977b68b" iter=1>
    <title>docs: align bead readiness terminology across governing specs</title>
    <description>
PROBLEM
Governing specs mix "complexity gate," "pre-claim intake," "pre-dispatch lint," and readiness language, so DDx has no stable product concept for the pre-claim decision about whether a bead is tractable and actionable.

ROOT CAUSE WITH FILE:LINE
- `docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md:50-67` defines the hook as "Pre-claim intake" and describes actionability, scope, and decomposition without naming bead readiness as the canonical concept.
- `docs/helix/01-frame/features/FEAT-010-task-execution.md:112-145` repeats the intake terminology inside task execution lifecycle text.
- `docs/helix/01-frame/features/FEAT-011-skills.md:142-143` describes bead-lifecycle as lint/triage, omitting readiness.

PROPOSED FIX
Amend ADR-023, FEAT-010, FEAT-004, FEAT-011, and TD-031 review notes so "bead readiness assessment" is the canonical product concept. Keep `MODE: intake` and existing intake event names only as compatibility details. Distinguish readiness, lint/rubric scoring, and post-attempt triage.

NON-SCOPE
Do not edit skills, Go code, generated docs, or tests in this bead.
    </description>
    <acceptance>
1. ADR-023 defines bead readiness assessment as the canonical pre-claim decision about tractability/actionability.
2. FEAT-010 distinguishes readiness, lint/rubric scoring, and post-attempt triage.
3. FEAT-004 states readiness uses existing bead metadata and does not add schema fields.
4. FEAT-011 states bead-lifecycle owns readiness, lint/rubric scoring, triage, and refine guidance.
5. TD-031 is reviewed and either updated for readiness/triage queue actions or explicitly noted unchanged.
6. Stale terms are absent from normative spec language except in explicitly labeled legacy/migration notes.
7. Documentation-only waiver is stated in closing notes; no new Go test is required unless generated references are edited.
8. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:docs, area:specs, kind:doc, reliability, bead-quality, adr:023, spec:FEAT-004, spec:FEAT-010, spec:FEAT-011, spec:TD-031</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T172936-452214ec/manifest.json</file>
    <file>.ddx/executions/20260506T172936-452214ec/result.json</file>
  </changed-files>

  <governing>
    <ref id="ADR-023" path="docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md" title="ADR-023: Bead Lifecycle Quality Policy">
      <content>
<untrusted-data>
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
   may still advertise `MODE: intake` for legacy compatibility, but the
   product concept is bead readiness assessment.
2. **Post-attempt triage.** After an attempt finalizes, DDx triages the result
   against the same lifecycle quality policy so a low-quality prompt failure,
   missing rationale, empty review block, or structurally ambiguous outcome is
   classified and surfaced consistently.

The readiness checkpoint and the post-attempt triage checkpoint are distinct:
readiness decides whether a bead should be claimed or rewritten before
execution, lint/rubric scoring measures prompt quality, and triage classifies
the attempt after evidence exists.

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
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="eb595a6c37fd343c547290949fb39d1d14f9a1d7">
<untrusted-data>
diff --git a/.ddx/executions/20260506T172936-452214ec/manifest.json b/.ddx/executions/20260506T172936-452214ec/manifest.json
new file mode 100644
index 000000000..5b7f19980
--- /dev/null
+++ b/.ddx/executions/20260506T172936-452214ec/manifest.json
@@ -0,0 +1,159 @@
+{
+  "attempt_id": "20260506T172936-452214ec",
+  "bead_id": "ddx-8977b68b",
+  "base_rev": "f0a1399c96d5c47e6df60decd034556a4505f1b2",
+  "created_at": "2026-05-06T17:29:39.397181477Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-8977b68b",
+    "title": "docs: align bead readiness terminology across governing specs",
+    "description": "PROBLEM\nGoverning specs mix \"complexity gate,\" \"pre-claim intake,\" \"pre-dispatch lint,\" and readiness language, so DDx has no stable product concept for the pre-claim decision about whether a bead is tractable and actionable.\n\nROOT CAUSE WITH FILE:LINE\n- `docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md:50-67` defines the hook as \"Pre-claim intake\" and describes actionability, scope, and decomposition without naming bead readiness as the canonical concept.\n- `docs/helix/01-frame/features/FEAT-010-task-execution.md:112-145` repeats the intake terminology inside task execution lifecycle text.\n- `docs/helix/01-frame/features/FEAT-011-skills.md:142-143` describes bead-lifecycle as lint/triage, omitting readiness.\n\nPROPOSED FIX\nAmend ADR-023, FEAT-010, FEAT-004, FEAT-011, and TD-031 review notes so \"bead readiness assessment\" is the canonical product concept. Keep `MODE: intake` and existing intake event names only as compatibility details. Distinguish readiness, lint/rubric scoring, and post-attempt triage.\n\nNON-SCOPE\nDo not edit skills, Go code, generated docs, or tests in this bead.",
+    "acceptance": "1. ADR-023 defines bead readiness assessment as the canonical pre-claim decision about tractability/actionability.\n2. FEAT-010 distinguishes readiness, lint/rubric scoring, and post-attempt triage.\n3. FEAT-004 states readiness uses existing bead metadata and does not add schema fields.\n4. FEAT-011 states bead-lifecycle owns readiness, lint/rubric scoring, triage, and refine guidance.\n5. TD-031 is reviewed and either updated for readiness/triage queue actions or explicitly noted unchanged.\n6. Stale terms are absent from normative spec language except in explicitly labeled legacy/migration notes.\n7. Documentation-only waiver is stated in closing notes; no new Go test is required unless generated references are edited.\n8. `lefthook run pre-commit` passes.",
+    "parent": "ddx-a9d130d0",
+    "labels": [
+      "phase:2",
+      "area:docs",
+      "area:specs",
+      "kind:doc",
+      "reliability",
+      "bead-quality",
+      "adr:023",
+      "spec:FEAT-004",
+      "spec:FEAT-010",
+      "spec:FEAT-011",
+      "spec:TD-031"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T17:29:36Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "960259",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"\",\"score\":0,\"suggested_fixes\":null,\"waivers_applied\":null,\"warning\":\"lint hook: missing-harness\"}",
+          "created_at": "2026-05-06T16:49:55.510950653Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "warning score=0"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-06T16:52:29.038647316Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T164955-75f475db\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":2233689,\"output_tokens\":15574,\"total_tokens\":2249263,\"cost_usd\":0,\"duration_ms\":150267,\"exit_code\":0}",
+          "created_at": "2026-05-06T16:52:29.260382617Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2249263 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T16:52:38.13543996Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=f9b834a1096833892a87d0ed3f9c91d4e84a2801\n\nreview-group: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T12:57:43-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=14770\noutput_bytes=0\nelapsed_ms=4100",
+          "created_at": "2026-05-06T16:52:43.327040911Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "pre-close review: review-error: transport\nagent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T12:57:43-04:00)\nresult_rev=f9b834a1096833892a87d0ed3f9c91d4e84a2801\nbase_rev=24b83b3cec5220c2fe4fd428934b4b75f4b0ed3a",
+          "created_at": "2026-05-06T16:52:43.535602348Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_malfunction"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"\",\"score\":0,\"suggested_fixes\":null,\"waivers_applied\":null,\"warning\":\"lint hook: missing-harness\"}",
+          "created_at": "2026-05-06T16:58:34.787810106Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "warning score=0"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-06T17:00:18.509506986Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T165835-ee9e61c2\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":933664,\"output_tokens\":10501,\"total_tokens\":944165,\"cost_usd\":0,\"duration_ms\":100390,\"exit_code\":0}",
+          "created_at": "2026-05-06T17:00:18.730155293Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=944165 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T17:00:27.00883491Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=bd118b7af65b10795b0794169bd42c145a8451a6\n\nreview-group: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T13:05:30-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=14768\noutput_bytes=0\nelapsed_ms=2186",
+          "created_at": "2026-05-06T17:00:30.314456804Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "pre-close review: review-error: transport\nagent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T13:05:30-04:00)\nresult_rev=bd118b7af65b10795b0794169bd42c145a8451a6\nbase_rev=f47e6a5dbc6dc4d72a3fd4c1cc6939060de50c9d",
+          "created_at": "2026-05-06T17:00:30.531628947Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_malfunction"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"\",\"score\":0,\"suggested_fixes\":null,\"waivers_applied\":null,\"warning\":\"lint hook: missing-harness\"}",
+          "created_at": "2026-05-06T17:29:35.826162086Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "warning score=0"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T17:29:36.218370773Z",
+      "spec-id": "ADR-023"
+    }
+  },
+  "governing": [
+    {
+      "id": "ADR-023",
+      "path": "docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md",
+      "title": "ADR-023: Bead Lifecycle Quality Policy"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260506T172936-452214ec",
+    "prompt": ".ddx/executions/20260506T172936-452214ec/prompt.md",
+    "manifest": ".ddx/executions/20260506T172936-452214ec/manifest.json",
+    "result": ".ddx/executions/20260506T172936-452214ec/result.json",
+    "checks": ".ddx/executions/20260506T172936-452214ec/checks.json",
+    "usage": ".ddx/executions/20260506T172936-452214ec/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8977b68b-20260506T172936-452214ec"
+  },
+  "prompt_sha": "ff064ba3148a9c2c5cf07a74c05c58308be9d8a676755bcd794ae2aa5be73501"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T172936-452214ec/result.json b/.ddx/executions/20260506T172936-452214ec/result.json
new file mode 100644
index 000000000..6f4421e47
--- /dev/null
+++ b/.ddx/executions/20260506T172936-452214ec/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-8977b68b",
+  "attempt_id": "20260506T172936-452214ec",
+  "base_rev": "f0a1399c96d5c47e6df60decd034556a4505f1b2",
+  "result_rev": "df3bc577cc4583955d3df4bee29c4f40e1622ad0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-e685abc6",
+  "duration_ms": 88836,
+  "tokens": 854213,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T172936-452214ec",
+  "prompt_file": ".ddx/executions/20260506T172936-452214ec/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T172936-452214ec/manifest.json",
+  "result_file": ".ddx/executions/20260506T172936-452214ec/result.json",
+  "usage_file": ".ddx/executions/20260506T172936-452214ec/usage.json",
+  "started_at": "2026-05-06T17:29:39.397659185Z",
+  "finished_at": "2026-05-06T17:31:08.2345077Z"
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
