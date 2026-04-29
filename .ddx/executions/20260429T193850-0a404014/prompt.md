<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-4c9f51f9" iter=1>
    <title>[read-coverage] add agent models/catalog/capabilities HTTP + MCP</title>
    <description>
FEAT-006 gap. CLI has ddx agent models, ddx agent catalog show, ddx agent capabilities, ddx agent usage. No HTTP routes or MCP tools exist for any of these. There is no server-side way to discover available models, tier assignments, or capability metadata. Required for the endpoint-first routing redesign and automated model selection. Add /api/agent/models, /api/agent/catalog, /api/agent/capabilities HTTP routes and corresponding MCP tools.
    </description>
    <acceptance/>
    <labels>read-coverage, server, http, mcp, feat-006</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T192729-eeef7323/manifest.json</file>
    <file>.ddx/executions/20260429T192729-eeef7323/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c21f3af1e1518ad35c0cb301984aae7037a5e686">
diff --git a/.ddx/executions/20260429T192729-eeef7323/manifest.json b/.ddx/executions/20260429T192729-eeef7323/manifest.json
new file mode 100644
index 00000000..4838c3c2
--- /dev/null
+++ b/.ddx/executions/20260429T192729-eeef7323/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T192729-eeef7323",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-4c9f51f9",
+  "base_rev": "1c921a9189bf2412d0d1edffe43cd8218b30e32f",
+  "created_at": "2026-04-29T19:27:30.331265689Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-4c9f51f9",
+    "title": "[read-coverage] add agent models/catalog/capabilities HTTP + MCP",
+    "description": "FEAT-006 gap. CLI has ddx agent models, ddx agent catalog show, ddx agent capabilities, ddx agent usage. No HTTP routes or MCP tools exist for any of these. There is no server-side way to discover available models, tier assignments, or capability metadata. Required for the endpoint-first routing redesign and automated model selection. Add /api/agent/models, /api/agent/catalog, /api/agent/capabilities HTTP routes and corresponding MCP tools.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "http",
+      "mcp",
+      "feat-006"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T19:27:27Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T19:27:27.495581724Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T192729-eeef7323",
+    "prompt": ".ddx/executions/20260429T192729-eeef7323/prompt.md",
+    "manifest": ".ddx/executions/20260429T192729-eeef7323/manifest.json",
+    "result": ".ddx/executions/20260429T192729-eeef7323/result.json",
+    "checks": ".ddx/executions/20260429T192729-eeef7323/checks.json",
+    "usage": ".ddx/executions/20260429T192729-eeef7323/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-4c9f51f9-20260429T192729-eeef7323"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T192729-eeef7323/result.json b/.ddx/executions/20260429T192729-eeef7323/result.json
new file mode 100644
index 00000000..4e2b0a89
--- /dev/null
+++ b/.ddx/executions/20260429T192729-eeef7323/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-4c9f51f9",
+  "attempt_id": "20260429T192729-eeef7323",
+  "base_rev": "1c921a9189bf2412d0d1edffe43cd8218b30e32f",
+  "result_rev": "80b1fcf0efcbd181162ecff23f99974ec3046ca0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-16a13f69",
+  "duration_ms": 675203,
+  "tokens": 21104,
+  "cost_usd": 2.259370350000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T192729-eeef7323",
+  "prompt_file": ".ddx/executions/20260429T192729-eeef7323/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T192729-eeef7323/manifest.json",
+  "result_file": ".ddx/executions/20260429T192729-eeef7323/result.json",
+  "usage_file": ".ddx/executions/20260429T192729-eeef7323/usage.json",
+  "started_at": "2026-04-29T19:27:30.331560814Z",
+  "finished_at": "2026-04-29T19:38:45.535236658Z"
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
