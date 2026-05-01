<bead-review>
  <bead id="ddx-91a4e480" iter=1>
    <title>docs: extend product-vision.md Physics #4 with context rot and the ralph loop</title>
    <description/>
    <acceptance>
Principle 4 in product-vision.md includes the sentence: 'Quality degrades as the context window fills.' Nothing else added.
    </acceptance>
    <labels>area:docs, bounded-context</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T075114-00d7aee2/manifest.json</file>
    <file>.ddx/executions/20260501T075114-00d7aee2/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="bd1dbfe90b3e00bbd15b0c01fa383e62ab692c8a">
diff --git a/.ddx/executions/20260501T075114-00d7aee2/manifest.json b/.ddx/executions/20260501T075114-00d7aee2/manifest.json
new file mode 100644
index 00000000..d7059950
--- /dev/null
+++ b/.ddx/executions/20260501T075114-00d7aee2/manifest.json
@@ -0,0 +1,35 @@
+{
+  "attempt_id": "20260501T075114-00d7aee2",
+  "bead_id": "ddx-91a4e480",
+  "base_rev": "64dd11e0213076000ec3901b1119f8f4405a05e7",
+  "created_at": "2026-05-01T07:51:15.275719487Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-91a4e480",
+    "title": "docs: extend product-vision.md Physics #4 with context rot and the ralph loop",
+    "acceptance": "Principle 4 in product-vision.md includes the sentence: 'Quality degrades as the context window fills.' Nothing else added.",
+    "parent": "ddx-dcee9b0c",
+    "labels": [
+      "area:docs",
+      "bounded-context"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T07:51:14Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3396374",
+      "execute-loop-heartbeat-at": "2026-05-01T07:51:14.121668619Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T075114-00d7aee2",
+    "prompt": ".ddx/executions/20260501T075114-00d7aee2/prompt.md",
+    "manifest": ".ddx/executions/20260501T075114-00d7aee2/manifest.json",
+    "result": ".ddx/executions/20260501T075114-00d7aee2/result.json",
+    "checks": ".ddx/executions/20260501T075114-00d7aee2/checks.json",
+    "usage": ".ddx/executions/20260501T075114-00d7aee2/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-91a4e480-20260501T075114-00d7aee2"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T075114-00d7aee2/result.json b/.ddx/executions/20260501T075114-00d7aee2/result.json
new file mode 100644
index 00000000..dab38018
--- /dev/null
+++ b/.ddx/executions/20260501T075114-00d7aee2/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-91a4e480",
+  "attempt_id": "20260501T075114-00d7aee2",
+  "base_rev": "64dd11e0213076000ec3901b1119f8f4405a05e7",
+  "result_rev": "7c4e4a928bffe29a527d2bff71d9366e466d21dd",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c4b83c1b",
+  "duration_ms": 42479,
+  "tokens": 2157,
+  "cost_usd": 0.318757,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T075114-00d7aee2",
+  "prompt_file": ".ddx/executions/20260501T075114-00d7aee2/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T075114-00d7aee2/manifest.json",
+  "result_file": ".ddx/executions/20260501T075114-00d7aee2/result.json",
+  "usage_file": ".ddx/executions/20260501T075114-00d7aee2/usage.json",
+  "started_at": "2026-05-01T07:51:15.276045404Z",
+  "finished_at": "2026-05-01T07:51:57.755517471Z"
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
