<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-9d21efb4" iter=1>
    <title>[read-coverage] add ddx_doc_dependents MCP tool</title>
    <description>
FEAT-007 gap. HTTP has GET /api/docs/{id}/dependents but MCP has ddx_doc_deps (upstream direction only) with no reverse. Add ddx_doc_dependents MCP tool mirroring the HTTP endpoint. Agents doing cascade staleness analysis or impact-of-change reasoning rely on the reverse direction.
    </description>
    <acceptance/>
    <labels>read-coverage, server, mcp, feat-007</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T185803-f3a78895/manifest.json</file>
    <file>.ddx/executions/20260429T185803-f3a78895/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="09bceaddfa303d45f1ae7149adb7a092ce2ee8f2">
diff --git a/.ddx/executions/20260429T185803-f3a78895/manifest.json b/.ddx/executions/20260429T185803-f3a78895/manifest.json
new file mode 100644
index 00000000..364289ec
--- /dev/null
+++ b/.ddx/executions/20260429T185803-f3a78895/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260429T185803-f3a78895",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-9d21efb4",
+  "base_rev": "7fb66249997dc8e80aadcd24e3a4a23a6ae40fc2",
+  "created_at": "2026-04-29T18:58:04.479160554Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-9d21efb4",
+    "title": "[read-coverage] add ddx_doc_dependents MCP tool",
+    "description": "FEAT-007 gap. HTTP has GET /api/docs/{id}/dependents but MCP has ddx_doc_deps (upstream direction only) with no reverse. Add ddx_doc_dependents MCP tool mirroring the HTTP endpoint. Agents doing cascade staleness analysis or impact-of-change reasoning rely on the reverse direction.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "mcp",
+      "feat-007"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T18:58:01Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T18:58:01.610952339Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T185803-f3a78895",
+    "prompt": ".ddx/executions/20260429T185803-f3a78895/prompt.md",
+    "manifest": ".ddx/executions/20260429T185803-f3a78895/manifest.json",
+    "result": ".ddx/executions/20260429T185803-f3a78895/result.json",
+    "checks": ".ddx/executions/20260429T185803-f3a78895/checks.json",
+    "usage": ".ddx/executions/20260429T185803-f3a78895/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-9d21efb4-20260429T185803-f3a78895"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T185803-f3a78895/result.json b/.ddx/executions/20260429T185803-f3a78895/result.json
new file mode 100644
index 00000000..2c7eab85
--- /dev/null
+++ b/.ddx/executions/20260429T185803-f3a78895/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-9d21efb4",
+  "attempt_id": "20260429T185803-f3a78895",
+  "base_rev": "7fb66249997dc8e80aadcd24e3a4a23a6ae40fc2",
+  "result_rev": "d4b2a0b8cd2b38bd195762290d0675e9eb0e35fb",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-8b594272",
+  "duration_ms": 344860,
+  "tokens": 5076,
+  "cost_usd": 0.4071469,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T185803-f3a78895",
+  "prompt_file": ".ddx/executions/20260429T185803-f3a78895/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T185803-f3a78895/manifest.json",
+  "result_file": ".ddx/executions/20260429T185803-f3a78895/result.json",
+  "usage_file": ".ddx/executions/20260429T185803-f3a78895/usage.json",
+  "started_at": "2026-04-29T18:58:04.47946197Z",
+  "finished_at": "2026-04-29T19:03:49.340457429Z"
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
