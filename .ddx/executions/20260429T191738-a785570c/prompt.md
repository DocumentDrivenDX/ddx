<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce" iter=1>
    <title>[read-coverage] add ddx_bead_blocked + ddx_bead_dep_tree MCP tools</title>
    <description>
FEAT-004/002 gap. HTTP has GET /api/beads/blocked and GET /api/beads/dep/tree/{id}. MCP has ddx_list_beads, ddx_show_bead, ddx_bead_ready, ddx_bead_status but no blocked or dep-tree tools. Agents managing the bead queue via MCP cannot check blocked state or inspect dependency trees. Add both tools.
    </description>
    <acceptance/>
    <labels>read-coverage, server, mcp, feat-004, feat-002</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T191154-99abef11/manifest.json</file>
    <file>.ddx/executions/20260429T191154-99abef11/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d9e2fa6209cc05224c7f0294f531d6a6b84930f1">
diff --git a/.ddx/executions/20260429T191154-99abef11/manifest.json b/.ddx/executions/20260429T191154-99abef11/manifest.json
new file mode 100644
index 00000000..8af2d23c
--- /dev/null
+++ b/.ddx/executions/20260429T191154-99abef11/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T191154-99abef11",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce",
+  "base_rev": "c8f5c4f575069a93b97d783c73918c05c3b26cc4",
+  "created_at": "2026-04-29T19:11:55.009433608Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce",
+    "title": "[read-coverage] add ddx_bead_blocked + ddx_bead_dep_tree MCP tools",
+    "description": "FEAT-004/002 gap. HTTP has GET /api/beads/blocked and GET /api/beads/dep/tree/{id}. MCP has ddx_list_beads, ddx_show_bead, ddx_bead_ready, ddx_bead_status but no blocked or dep-tree tools. Agents managing the bead queue via MCP cannot check blocked state or inspect dependency trees. Add both tools.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "mcp",
+      "feat-004",
+      "feat-002"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T19:11:54Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T19:11:54.0945458Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T191154-99abef11",
+    "prompt": ".ddx/executions/20260429T191154-99abef11/prompt.md",
+    "manifest": ".ddx/executions/20260429T191154-99abef11/manifest.json",
+    "result": ".ddx/executions/20260429T191154-99abef11/result.json",
+    "checks": ".ddx/executions/20260429T191154-99abef11/checks.json",
+    "usage": ".ddx/executions/20260429T191154-99abef11/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce-20260429T191154-99abef11"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T191154-99abef11/result.json b/.ddx/executions/20260429T191154-99abef11/result.json
new file mode 100644
index 00000000..0294e780
--- /dev/null
+++ b/.ddx/executions/20260429T191154-99abef11/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce",
+  "attempt_id": "20260429T191154-99abef11",
+  "base_rev": "c8f5c4f575069a93b97d783c73918c05c3b26cc4",
+  "result_rev": "2290ee669f03ee1bb1ee05d3d06304d8c35b7620",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-c1315d45",
+  "duration_ms": 339518,
+  "tokens": 6453,
+  "cost_usd": 0.606214,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T191154-99abef11",
+  "prompt_file": ".ddx/executions/20260429T191154-99abef11/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T191154-99abef11/manifest.json",
+  "result_file": ".ddx/executions/20260429T191154-99abef11/result.json",
+  "usage_file": ".ddx/executions/20260429T191154-99abef11/usage.json",
+  "started_at": "2026-04-29T19:11:55.009689525Z",
+  "finished_at": "2026-04-29T19:17:34.528596521Z"
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
