<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8" iter=1>
    <title>[read-coverage] add bead evidence + cooldown + routing HTTP + MCP</title>
    <description>
FEAT-004 gap. CLI has: ddx bead evidence list &lt;id&gt;, ddx bead cooldown show &lt;id&gt;, ddx bead routing. None of these are exposed over HTTP or MCP. Add GET /api/beads/{id}/evidence, GET /api/beads/{id}/cooldown, GET /api/beads/{id}/routing HTTP routes and corresponding MCP tools (ddx_bead_evidence, ddx_bead_cooldown). Low priority — these are operational details primarily consumed by the execute-loop itself.
    </description>
    <acceptance/>
    <labels>read-coverage, server, http, mcp, feat-004</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T202423-4fc8564e/manifest.json</file>
    <file>.ddx/executions/20260429T202423-4fc8564e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5c5bf0201c3c8760c6c503a2369171b96d98e957">
diff --git a/.ddx/executions/20260429T202423-4fc8564e/manifest.json b/.ddx/executions/20260429T202423-4fc8564e/manifest.json
new file mode 100644
index 00000000..be3a5c11
--- /dev/null
+++ b/.ddx/executions/20260429T202423-4fc8564e/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T202423-4fc8564e",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8",
+  "base_rev": "d969131fb344bcd33940ac4c920dbd8d59940c13",
+  "created_at": "2026-04-29T20:24:24.346836535Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8",
+    "title": "[read-coverage] add bead evidence + cooldown + routing HTTP + MCP",
+    "description": "FEAT-004 gap. CLI has: ddx bead evidence list \u003cid\u003e, ddx bead cooldown show \u003cid\u003e, ddx bead routing. None of these are exposed over HTTP or MCP. Add GET /api/beads/{id}/evidence, GET /api/beads/{id}/cooldown, GET /api/beads/{id}/routing HTTP routes and corresponding MCP tools (ddx_bead_evidence, ddx_bead_cooldown). Low priority — these are operational details primarily consumed by the execute-loop itself.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "http",
+      "mcp",
+      "feat-004"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T20:24:21Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T20:24:21.452582796Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T202423-4fc8564e",
+    "prompt": ".ddx/executions/20260429T202423-4fc8564e/prompt.md",
+    "manifest": ".ddx/executions/20260429T202423-4fc8564e/manifest.json",
+    "result": ".ddx/executions/20260429T202423-4fc8564e/result.json",
+    "checks": ".ddx/executions/20260429T202423-4fc8564e/checks.json",
+    "usage": ".ddx/executions/20260429T202423-4fc8564e/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8-20260429T202423-4fc8564e"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T202423-4fc8564e/result.json b/.ddx/executions/20260429T202423-4fc8564e/result.json
new file mode 100644
index 00000000..74bc1e53
--- /dev/null
+++ b/.ddx/executions/20260429T202423-4fc8564e/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8",
+  "attempt_id": "20260429T202423-4fc8564e",
+  "base_rev": "d969131fb344bcd33940ac4c920dbd8d59940c13",
+  "result_rev": "9a994dd5f7f168fc7ceaef2ad1bef7db489ce7a7",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-eedb9de3",
+  "duration_ms": 871076,
+  "tokens": 19253,
+  "cost_usd": 1.6286221,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T202423-4fc8564e",
+  "prompt_file": ".ddx/executions/20260429T202423-4fc8564e/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T202423-4fc8564e/manifest.json",
+  "result_file": ".ddx/executions/20260429T202423-4fc8564e/result.json",
+  "usage_file": ".ddx/executions/20260429T202423-4fc8564e/usage.json",
+  "started_at": "2026-04-29T20:24:24.347163535Z",
+  "finished_at": "2026-04-29T20:38:55.423968264Z"
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
