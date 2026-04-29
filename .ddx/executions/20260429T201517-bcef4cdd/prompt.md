<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915" iter=1>
    <title>[read-coverage] add GET /api/docs/changed REST route</title>
    <description>
FEAT-007 gap. MCP has ddx_doc_changed (list artifacts changed since a git ref) but no equivalent HTTP REST route. REST clients (non-MCP) cannot query changed artifacts. Add GET /api/docs/changed?since=&lt;ref&gt; route mirroring the MCP tool. Low complexity add alongside other FEAT-007 server work.
    </description>
    <acceptance/>
    <labels>read-coverage, server, http, feat-007</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T200946-5369ed03/manifest.json</file>
    <file>.ddx/executions/20260429T200946-5369ed03/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8330801598f7a46f45abc8b556d33f19f58059cf">
diff --git a/.ddx/executions/20260429T200946-5369ed03/manifest.json b/.ddx/executions/20260429T200946-5369ed03/manifest.json
new file mode 100644
index 00000000..dc44fa4e
--- /dev/null
+++ b/.ddx/executions/20260429T200946-5369ed03/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260429T200946-5369ed03",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915",
+  "base_rev": "407fa6f4cde287bb7e16ba73973f9bc203e46624",
+  "created_at": "2026-04-29T20:09:46.921374605Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915",
+    "title": "[read-coverage] add GET /api/docs/changed REST route",
+    "description": "FEAT-007 gap. MCP has ddx_doc_changed (list artifacts changed since a git ref) but no equivalent HTTP REST route. REST clients (non-MCP) cannot query changed artifacts. Add GET /api/docs/changed?since=\u003cref\u003e route mirroring the MCP tool. Low complexity add alongside other FEAT-007 server work.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "http",
+      "feat-007"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T20:09:44Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T20:09:44.04268713Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T200946-5369ed03",
+    "prompt": ".ddx/executions/20260429T200946-5369ed03/prompt.md",
+    "manifest": ".ddx/executions/20260429T200946-5369ed03/manifest.json",
+    "result": ".ddx/executions/20260429T200946-5369ed03/result.json",
+    "checks": ".ddx/executions/20260429T200946-5369ed03/checks.json",
+    "usage": ".ddx/executions/20260429T200946-5369ed03/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915-20260429T200946-5369ed03"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T200946-5369ed03/result.json b/.ddx/executions/20260429T200946-5369ed03/result.json
new file mode 100644
index 00000000..8ac6e0fa
--- /dev/null
+++ b/.ddx/executions/20260429T200946-5369ed03/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915",
+  "attempt_id": "20260429T200946-5369ed03",
+  "base_rev": "407fa6f4cde287bb7e16ba73973f9bc203e46624",
+  "result_rev": "128136fc4a897ae57dd7b7347c2f06edba9aaa7d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-8416601b",
+  "duration_ms": 326537,
+  "tokens": 7336,
+  "cost_usd": 0.6200976499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T200946-5369ed03",
+  "prompt_file": ".ddx/executions/20260429T200946-5369ed03/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T200946-5369ed03/manifest.json",
+  "result_file": ".ddx/executions/20260429T200946-5369ed03/result.json",
+  "usage_file": ".ddx/executions/20260429T200946-5369ed03/usage.json",
+  "started_at": "2026-04-29T20:09:46.921676771Z",
+  "finished_at": "2026-04-29T20:15:13.458905212Z"
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
