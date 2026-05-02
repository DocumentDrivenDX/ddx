<bead-review>
  <bead id="ddx-a2255401" iter=1>
    <title>B4a: Amend FEAT-022 with static-prompt minimum-prompt rule</title>
    <description>
Amend FEAT-022 per /tmp/story-12-final.md 'Spec changes' section. Add the following under Functional / Shared evidence primitives: 'Static-prompt minimum-prompt rule. Every static instruction string DDx embeds in an LLM prompt MUST declare its load-bearing guardrails inline as a Go comment list above the constant, one bullet per guardrail with a pointer to the failure mode it prevents. Edits MUST preserve every listed guardrail. New guardrails MUST be added to the list. A regression test MUST assert each listed guardrail still appears in the rendered prompt. This rule applies to STATIC prompt bodies; dynamic evidence egress is covered by the §5–§7 cap primitives and is unaffected.' Closing sentence (per codex) prevents implying static-comment guardrails replace dynamic-egress caps. Should land after B2 so the rule reflects implementation.
    </description>
    <acceptance>
AC8 (FEAT-022 portion): FEAT-022 spec amended with the minimum-prompt rule paragraph exactly as specified, including the closing sentence about dynamic egress. Linked from the affected constants if cross-references exist.
    </acceptance>
    <labels>phase:2, story:12, tier:cheap</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T122805-11bad270/manifest.json</file>
    <file>.ddx/executions/20260502T122805-11bad270/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="954326d3a8f16ff4956ec04a7204d1385a1c3238">
diff --git a/.ddx/executions/20260502T122805-11bad270/manifest.json b/.ddx/executions/20260502T122805-11bad270/manifest.json
new file mode 100644
index 00000000..7dddcffd
--- /dev/null
+++ b/.ddx/executions/20260502T122805-11bad270/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T122805-11bad270",
+  "bead_id": "ddx-a2255401",
+  "base_rev": "af88c35eeec53c4d6219231c3477956ad5d7390c",
+  "created_at": "2026-05-02T12:28:06.960096756Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a2255401",
+    "title": "B4a: Amend FEAT-022 with static-prompt minimum-prompt rule",
+    "description": "Amend FEAT-022 per /tmp/story-12-final.md 'Spec changes' section. Add the following under Functional / Shared evidence primitives: 'Static-prompt minimum-prompt rule. Every static instruction string DDx embeds in an LLM prompt MUST declare its load-bearing guardrails inline as a Go comment list above the constant, one bullet per guardrail with a pointer to the failure mode it prevents. Edits MUST preserve every listed guardrail. New guardrails MUST be added to the list. A regression test MUST assert each listed guardrail still appears in the rendered prompt. This rule applies to STATIC prompt bodies; dynamic evidence egress is covered by the §5–§7 cap primitives and is unaffected.' Closing sentence (per codex) prevents implying static-comment guardrails replace dynamic-egress caps. Should land after B2 so the rule reflects implementation.",
+    "acceptance": "AC8 (FEAT-022 portion): FEAT-022 spec amended with the minimum-prompt rule paragraph exactly as specified, including the closing sentence about dynamic egress. Linked from the affected constants if cross-references exist.",
+    "parent": "ddx-a61bf8ee",
+    "labels": [
+      "phase:2",
+      "story:12",
+      "tier:cheap"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T12:28:05Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1724970",
+      "execute-loop-heartbeat-at": "2026-05-02T12:28:05.693941588Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T122805-11bad270",
+    "prompt": ".ddx/executions/20260502T122805-11bad270/prompt.md",
+    "manifest": ".ddx/executions/20260502T122805-11bad270/manifest.json",
+    "result": ".ddx/executions/20260502T122805-11bad270/result.json",
+    "checks": ".ddx/executions/20260502T122805-11bad270/checks.json",
+    "usage": ".ddx/executions/20260502T122805-11bad270/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a2255401-20260502T122805-11bad270"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T122805-11bad270/result.json b/.ddx/executions/20260502T122805-11bad270/result.json
new file mode 100644
index 00000000..ffd5361d
--- /dev/null
+++ b/.ddx/executions/20260502T122805-11bad270/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a2255401",
+  "attempt_id": "20260502T122805-11bad270",
+  "base_rev": "af88c35eeec53c4d6219231c3477956ad5d7390c",
+  "result_rev": "3a015fbcb4e5f462a3f60d3805b73993346a4980",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-1bd76e03",
+  "duration_ms": 45576,
+  "tokens": 2198,
+  "cost_usd": 0.37272025000000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T122805-11bad270",
+  "prompt_file": ".ddx/executions/20260502T122805-11bad270/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T122805-11bad270/manifest.json",
+  "result_file": ".ddx/executions/20260502T122805-11bad270/result.json",
+  "usage_file": ".ddx/executions/20260502T122805-11bad270/usage.json",
+  "started_at": "2026-05-02T12:28:06.960404214Z",
+  "finished_at": "2026-05-02T12:28:52.53689589Z"
+}
\ No newline at end of file
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
