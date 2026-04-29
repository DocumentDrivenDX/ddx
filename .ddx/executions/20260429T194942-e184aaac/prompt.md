<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-88c136e6" iter=1>
    <title>[read-coverage] add MCP process metrics tools</title>
    <description>
FEAT-016 gap. HTTP has /api/metrics/{summary,cost,cycle-time,rework}. MCP has no metrics tools at all. Cost-aware agents (cost-tiered-work standing goal) cannot query process metrics via MCP. Add ddx_metrics_summary, ddx_metrics_cost, ddx_metrics_cycle_time, ddx_metrics_rework MCP tools mirroring the HTTP endpoints.
    </description>
    <acceptance/>
    <labels>read-coverage, server, mcp, feat-016</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T194323-17780ddb/manifest.json</file>
    <file>.ddx/executions/20260429T194323-17780ddb/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="25bf2dcf05a917a75c01a55502de15b4a3d2a1ee">
diff --git a/.ddx/executions/20260429T194323-17780ddb/manifest.json b/.ddx/executions/20260429T194323-17780ddb/manifest.json
new file mode 100644
index 00000000..3e320d48
--- /dev/null
+++ b/.ddx/executions/20260429T194323-17780ddb/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260429T194323-17780ddb",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-88c136e6",
+  "base_rev": "c3a75fb4328c59a7e2c91d0a74f6632eb95eda03",
+  "created_at": "2026-04-29T19:43:23.856475156Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-88c136e6",
+    "title": "[read-coverage] add MCP process metrics tools",
+    "description": "FEAT-016 gap. HTTP has /api/metrics/{summary,cost,cycle-time,rework}. MCP has no metrics tools at all. Cost-aware agents (cost-tiered-work standing goal) cannot query process metrics via MCP. Add ddx_metrics_summary, ddx_metrics_cost, ddx_metrics_cycle_time, ddx_metrics_rework MCP tools mirroring the HTTP endpoints.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "mcp",
+      "feat-016"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T19:43:21Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T19:43:21.000193615Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T194323-17780ddb",
+    "prompt": ".ddx/executions/20260429T194323-17780ddb/prompt.md",
+    "manifest": ".ddx/executions/20260429T194323-17780ddb/manifest.json",
+    "result": ".ddx/executions/20260429T194323-17780ddb/result.json",
+    "checks": ".ddx/executions/20260429T194323-17780ddb/checks.json",
+    "usage": ".ddx/executions/20260429T194323-17780ddb/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-88c136e6-20260429T194323-17780ddb"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T194323-17780ddb/result.json b/.ddx/executions/20260429T194323-17780ddb/result.json
new file mode 100644
index 00000000..61443fc4
--- /dev/null
+++ b/.ddx/executions/20260429T194323-17780ddb/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-88c136e6",
+  "attempt_id": "20260429T194323-17780ddb",
+  "base_rev": "c3a75fb4328c59a7e2c91d0a74f6632eb95eda03",
+  "result_rev": "bfca57d3e3dec55e0ba0997fcfdb34f6650832ab",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-6006fcef",
+  "duration_ms": 374567,
+  "tokens": 9970,
+  "cost_usd": 0.6977794500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T194323-17780ddb",
+  "prompt_file": ".ddx/executions/20260429T194323-17780ddb/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T194323-17780ddb/manifest.json",
+  "result_file": ".ddx/executions/20260429T194323-17780ddb/result.json",
+  "usage_file": ".ddx/executions/20260429T194323-17780ddb/usage.json",
+  "started_at": "2026-04-29T19:43:23.856827781Z",
+  "finished_at": "2026-04-29T19:49:38.424376483Z"
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
