<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-7a85a707" iter=1>
    <title>[read-coverage] add MCP server registry + plugin manifest HTTP + MCP</title>
    <description>
FEAT-009/015 gap. CLI has ddx mcp list (MCP server registry) and ddx install installed/outdated/search (plugin manifest). No HTTP routes or MCP tools expose this. Agents and UIs cannot discover which MCP servers are configured or which plugins are installed without spawning the CLI. Add /api/mcp-servers, /api/plugins HTTP routes and ddx_list_mcp_servers, ddx_list_plugins MCP tools.
    </description>
    <acceptance/>
    <labels>read-coverage, server, http, mcp, feat-009, feat-015</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T195516-aac5252f/manifest.json</file>
    <file>.ddx/executions/20260429T195516-aac5252f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5bab4108846f931684cf79f33b1dc9eb72fe338f">
diff --git a/.ddx/executions/20260429T195516-aac5252f/manifest.json b/.ddx/executions/20260429T195516-aac5252f/manifest.json
new file mode 100644
index 00000000..6d57d91d
--- /dev/null
+++ b/.ddx/executions/20260429T195516-aac5252f/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260429T195516-aac5252f",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-7a85a707",
+  "base_rev": "c86ec1f12369804d16e38a0a2a371cf5a65156d3",
+  "created_at": "2026-04-29T19:55:17.178431052Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-7a85a707",
+    "title": "[read-coverage] add MCP server registry + plugin manifest HTTP + MCP",
+    "description": "FEAT-009/015 gap. CLI has ddx mcp list (MCP server registry) and ddx install installed/outdated/search (plugin manifest). No HTTP routes or MCP tools expose this. Agents and UIs cannot discover which MCP servers are configured or which plugins are installed without spawning the CLI. Add /api/mcp-servers, /api/plugins HTTP routes and ddx_list_mcp_servers, ddx_list_plugins MCP tools.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "http",
+      "mcp",
+      "feat-009",
+      "feat-015"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T19:55:12Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T19:55:12.679347302Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T195516-aac5252f",
+    "prompt": ".ddx/executions/20260429T195516-aac5252f/prompt.md",
+    "manifest": ".ddx/executions/20260429T195516-aac5252f/manifest.json",
+    "result": ".ddx/executions/20260429T195516-aac5252f/result.json",
+    "checks": ".ddx/executions/20260429T195516-aac5252f/checks.json",
+    "usage": ".ddx/executions/20260429T195516-aac5252f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-7a85a707-20260429T195516-aac5252f"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T195516-aac5252f/result.json b/.ddx/executions/20260429T195516-aac5252f/result.json
new file mode 100644
index 00000000..3ffa5ae8
--- /dev/null
+++ b/.ddx/executions/20260429T195516-aac5252f/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-7a85a707",
+  "attempt_id": "20260429T195516-aac5252f",
+  "base_rev": "c86ec1f12369804d16e38a0a2a371cf5a65156d3",
+  "result_rev": "40ccc39b15e9e189ce1eb7a94ee47a9bcdfecc0e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-8709214f",
+  "duration_ms": 852766,
+  "tokens": 34487,
+  "cost_usd": 2.54401945,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T195516-aac5252f",
+  "prompt_file": ".ddx/executions/20260429T195516-aac5252f/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T195516-aac5252f/manifest.json",
+  "result_file": ".ddx/executions/20260429T195516-aac5252f/result.json",
+  "usage_file": ".ddx/executions/20260429T195516-aac5252f/usage.json",
+  "started_at": "2026-04-29T19:55:17.178727927Z",
+  "finished_at": "2026-04-29T20:09:29.944954883Z"
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
