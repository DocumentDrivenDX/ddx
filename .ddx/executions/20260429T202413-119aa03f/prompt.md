<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-ecea5b2b" iter=1>
    <title>[read-coverage] add per-metric-id history/trend HTTP + MCP</title>
    <description>
FEAT-016 gap. CLI has ddx metric history &lt;id&gt; and ddx metric trend &lt;id&gt; for time-series drill-down per metric ID. HTTP has only aggregate endpoints (/api/metrics/summary etc.); no per-metric-id endpoint. MCP has no metrics tools. Add GET /api/metrics/{id}/history and GET /api/metrics/{id}/trend HTTP routes, and ddx_metric_history + ddx_metric_trend MCP tools.
    </description>
    <acceptance/>
    <labels>read-coverage, server, http, mcp, feat-016</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T201527-73d64610/manifest.json</file>
    <file>.ddx/executions/20260429T201527-73d64610/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e2c22538730c79f53fc79d3d89660db1ce587b58">
diff --git a/.ddx/executions/20260429T201527-73d64610/manifest.json b/.ddx/executions/20260429T201527-73d64610/manifest.json
new file mode 100644
index 00000000..f9ed32bd
--- /dev/null
+++ b/.ddx/executions/20260429T201527-73d64610/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T201527-73d64610",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-ecea5b2b",
+  "base_rev": "de3bf172e21fbf2974567836c6180b107b8f7395",
+  "created_at": "2026-04-29T20:15:28.504861292Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-ecea5b2b",
+    "title": "[read-coverage] add per-metric-id history/trend HTTP + MCP",
+    "description": "FEAT-016 gap. CLI has ddx metric history \u003cid\u003e and ddx metric trend \u003cid\u003e for time-series drill-down per metric ID. HTTP has only aggregate endpoints (/api/metrics/summary etc.); no per-metric-id endpoint. MCP has no metrics tools. Add GET /api/metrics/{id}/history and GET /api/metrics/{id}/trend HTTP routes, and ddx_metric_history + ddx_metric_trend MCP tools.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "http",
+      "mcp",
+      "feat-016"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T20:15:25Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T20:15:25.560902708Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T201527-73d64610",
+    "prompt": ".ddx/executions/20260429T201527-73d64610/prompt.md",
+    "manifest": ".ddx/executions/20260429T201527-73d64610/manifest.json",
+    "result": ".ddx/executions/20260429T201527-73d64610/result.json",
+    "checks": ".ddx/executions/20260429T201527-73d64610/checks.json",
+    "usage": ".ddx/executions/20260429T201527-73d64610/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-ecea5b2b-20260429T201527-73d64610"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T201527-73d64610/result.json b/.ddx/executions/20260429T201527-73d64610/result.json
new file mode 100644
index 00000000..2570cb67
--- /dev/null
+++ b/.ddx/executions/20260429T201527-73d64610/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-ecea5b2b",
+  "attempt_id": "20260429T201527-73d64610",
+  "base_rev": "de3bf172e21fbf2974567836c6180b107b8f7395",
+  "result_rev": "21c5c4f474780918e59013aa75e7461a28d2a0f0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-dd8304af",
+  "duration_ms": 520954,
+  "tokens": 34,
+  "cost_usd": 0.8261541499999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T201527-73d64610",
+  "prompt_file": ".ddx/executions/20260429T201527-73d64610/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T201527-73d64610/manifest.json",
+  "result_file": ".ddx/executions/20260429T201527-73d64610/result.json",
+  "usage_file": ".ddx/executions/20260429T201527-73d64610/usage.json",
+  "started_at": "2026-04-29T20:15:28.505259959Z",
+  "finished_at": "2026-04-29T20:24:09.460234653Z"
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
