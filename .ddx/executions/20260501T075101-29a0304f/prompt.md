<bead-review>
  <bead id="ddx-dcee9b0c" iter=1>
    <title>docs: add bounded context execution concept — context rot, ralph loop, product-vision + concept doc</title>
    <description/>
    <acceptance>
context rot and bounded context execution are named and explained across: product-vision.md physics section, new concept doc, /why/ root cause section, execute-loop feature page
    </acceptance>
    <labels>area:docs, bounded-context, concepts</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T074835-f2ce1ec5/manifest.json</file>
    <file>.ddx/executions/20260501T074835-f2ce1ec5/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e2ac20acd1ce1dbdc8c4a86a8d87c463775b8a80">
diff --git a/.ddx/executions/20260501T074835-f2ce1ec5/manifest.json b/.ddx/executions/20260501T074835-f2ce1ec5/manifest.json
new file mode 100644
index 00000000..490b8d33
--- /dev/null
+++ b/.ddx/executions/20260501T074835-f2ce1ec5/manifest.json
@@ -0,0 +1,35 @@
+{
+  "attempt_id": "20260501T074835-f2ce1ec5",
+  "bead_id": "ddx-dcee9b0c",
+  "base_rev": "97ad8cce851d70da9ccffeab946642b635d8fa9a",
+  "created_at": "2026-05-01T07:48:36.725645055Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-dcee9b0c",
+    "title": "docs: add bounded context execution concept — context rot, ralph loop, product-vision + concept doc",
+    "acceptance": "context rot and bounded context execution are named and explained across: product-vision.md physics section, new concept doc, /why/ root cause section, execute-loop feature page",
+    "labels": [
+      "area:docs",
+      "bounded-context",
+      "concepts"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T07:48:35Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3396374",
+      "execute-loop-heartbeat-at": "2026-05-01T07:48:35.749426835Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T074835-f2ce1ec5",
+    "prompt": ".ddx/executions/20260501T074835-f2ce1ec5/prompt.md",
+    "manifest": ".ddx/executions/20260501T074835-f2ce1ec5/manifest.json",
+    "result": ".ddx/executions/20260501T074835-f2ce1ec5/result.json",
+    "checks": ".ddx/executions/20260501T074835-f2ce1ec5/checks.json",
+    "usage": ".ddx/executions/20260501T074835-f2ce1ec5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-dcee9b0c-20260501T074835-f2ce1ec5"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T074835-f2ce1ec5/result.json b/.ddx/executions/20260501T074835-f2ce1ec5/result.json
new file mode 100644
index 00000000..0789ba3f
--- /dev/null
+++ b/.ddx/executions/20260501T074835-f2ce1ec5/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-dcee9b0c",
+  "attempt_id": "20260501T074835-f2ce1ec5",
+  "base_rev": "97ad8cce851d70da9ccffeab946642b635d8fa9a",
+  "result_rev": "504f582b5595da3d66d3771a03adb272da0cd77f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-9421a335",
+  "duration_ms": 140873,
+  "tokens": 7768,
+  "cost_usd": 0.875531,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T074835-f2ce1ec5",
+  "prompt_file": ".ddx/executions/20260501T074835-f2ce1ec5/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T074835-f2ce1ec5/manifest.json",
+  "result_file": ".ddx/executions/20260501T074835-f2ce1ec5/result.json",
+  "usage_file": ".ddx/executions/20260501T074835-f2ce1ec5/usage.json",
+  "started_at": "2026-05-01T07:48:36.725958888Z",
+  "finished_at": "2026-05-01T07:50:57.598986805Z"
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
