<bead-review>
  <bead id="ddx-293c2a4a" iter=1>
    <title>worker detail: metadata grid and sections use raw text-sm/text-xs not text-body-sm/text-label-caps</title>
    <description>
src/routes/nodes/[nodeId]/projects/[projectId]/workers/[workerId]/+page.svelte has pervasive raw font size usage outside the log area:

Lines 314, 378, 424, 455: section wrappers use text-sm on the container rather than applying semantic tokens to each element. This causes all child text to inherit text-sm (14px Tailwind) rather than the semantic 14px/400 (text-body-md) pairing.

Lines 351, 365: 'text-mono-code text-fg-muted' correct but missing the explicit font-mono-code pairing — uses text-mono-code size without font-mono-code family.

Lines 431: lifecycle event list uses text-xs (raw) for list items. Should be text-body-sm or text-label-caps depending on context.

Line 460: alert-caution banner uses text-xs — should be text-body-sm.

Lines 486, 495: pre elements inside tool call details have no explicit text token — they rely on inherited font size.

The entire file pattern is: container sets text-sm, individual elements get bare color tokens without paired size tokens. This creates drift from semantic expectations when the base text-sm is later changed.
    </description>
    <acceptance>
Sections drop container text-sm. Individual elements carry explicit text-body-sm/text-label-caps/text-mono-code tokens as appropriate. font-mono-code is always paired with text-mono-code. alert-caution text uses text-body-sm.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T143917-ab820a7e/manifest.json</file>
    <file>.ddx/executions/20260501T143917-ab820a7e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0b8bf5d16d8b9825f462fceb44a69afcfdaf50e0">
diff --git a/.ddx/executions/20260501T143917-ab820a7e/manifest.json b/.ddx/executions/20260501T143917-ab820a7e/manifest.json
new file mode 100644
index 00000000..bdaad2a4
--- /dev/null
+++ b/.ddx/executions/20260501T143917-ab820a7e/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260501T143917-ab820a7e",
+  "bead_id": "ddx-293c2a4a",
+  "base_rev": "9557e936e62ee9f9c80fd74d446d6afefa755b02",
+  "created_at": "2026-05-01T14:39:18.651841287Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-293c2a4a",
+    "title": "worker detail: metadata grid and sections use raw text-sm/text-xs not text-body-sm/text-label-caps",
+    "description": "src/routes/nodes/[nodeId]/projects/[projectId]/workers/[workerId]/+page.svelte has pervasive raw font size usage outside the log area:\n\nLines 314, 378, 424, 455: section wrappers use text-sm on the container rather than applying semantic tokens to each element. This causes all child text to inherit text-sm (14px Tailwind) rather than the semantic 14px/400 (text-body-md) pairing.\n\nLines 351, 365: 'text-mono-code text-fg-muted' correct but missing the explicit font-mono-code pairing — uses text-mono-code size without font-mono-code family.\n\nLines 431: lifecycle event list uses text-xs (raw) for list items. Should be text-body-sm or text-label-caps depending on context.\n\nLine 460: alert-caution banner uses text-xs — should be text-body-sm.\n\nLines 486, 495: pre elements inside tool call details have no explicit text token — they rely on inherited font size.\n\nThe entire file pattern is: container sets text-sm, individual elements get bare color tokens without paired size tokens. This creates drift from semantic expectations when the base text-sm is later changed.",
+    "acceptance": "Sections drop container text-sm. Individual elements carry explicit text-body-sm/text-label-caps/text-mono-code tokens as appropriate. font-mono-code is always paired with text-mono-code. alert-caution text uses text-body-sm.",
+    "labels": [
+      "area:ui",
+      "kind:design",
+      "design-tokens"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T14:39:17Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "360683",
+      "execute-loop-heartbeat-at": "2026-05-01T14:39:17.641478408Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T143917-ab820a7e",
+    "prompt": ".ddx/executions/20260501T143917-ab820a7e/prompt.md",
+    "manifest": ".ddx/executions/20260501T143917-ab820a7e/manifest.json",
+    "result": ".ddx/executions/20260501T143917-ab820a7e/result.json",
+    "checks": ".ddx/executions/20260501T143917-ab820a7e/checks.json",
+    "usage": ".ddx/executions/20260501T143917-ab820a7e/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-293c2a4a-20260501T143917-ab820a7e"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T143917-ab820a7e/result.json b/.ddx/executions/20260501T143917-ab820a7e/result.json
new file mode 100644
index 00000000..f352a735
--- /dev/null
+++ b/.ddx/executions/20260501T143917-ab820a7e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-293c2a4a",
+  "attempt_id": "20260501T143917-ab820a7e",
+  "base_rev": "9557e936e62ee9f9c80fd74d446d6afefa755b02",
+  "result_rev": "1f73c372b3eec8f6d1d8f286952db5c01c5cc97e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-4b700f81",
+  "duration_ms": 121146,
+  "tokens": 9172,
+  "cost_usd": 0.78868925,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T143917-ab820a7e",
+  "prompt_file": ".ddx/executions/20260501T143917-ab820a7e/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T143917-ab820a7e/manifest.json",
+  "result_file": ".ddx/executions/20260501T143917-ab820a7e/result.json",
+  "usage_file": ".ddx/executions/20260501T143917-ab820a7e/usage.json",
+  "started_at": "2026-05-01T14:39:18.65215812Z",
+  "finished_at": "2026-05-01T14:41:19.798549189Z"
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
