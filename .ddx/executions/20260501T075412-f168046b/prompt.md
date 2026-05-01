<bead-review>
  <bead id="ddx-9a6a85c1" iter=1>
    <title>docs: create docs/helix/02-design/concepts/bounded-context-execution.md</title>
    <description/>
    <acceptance>
File exists at docs/helix/02-design/concepts/bounded-context-execution.md. Sections: (1) Context rot — definition, what causes it, why it degrades output quality; (2) The ralph loop — the unbounded session failure pattern; (3) DDx's answer — each bead = one bounded execution unit = one fresh agent invocation in an isolated worktree; ddx work is a bounded context execution loop, not a persistent session; (4) External references — links to 'Lost in the Middle' paper and other published research on in-context degradation. Cross-links to FEAT-010 and execute-loop docs.
    </acceptance>
    <labels>area:docs, bounded-context, concepts</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T075216-19982ef8/manifest.json</file>
    <file>.ddx/executions/20260501T075216-19982ef8/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a6cdc691113b52ce488a286cda129c0acb05d055">
diff --git a/.ddx/executions/20260501T075216-19982ef8/manifest.json b/.ddx/executions/20260501T075216-19982ef8/manifest.json
new file mode 100644
index 00000000..f5968fbc
--- /dev/null
+++ b/.ddx/executions/20260501T075216-19982ef8/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260501T075216-19982ef8",
+  "bead_id": "ddx-9a6a85c1",
+  "base_rev": "11ef41ae279f9dba9b1e1d6b3a9594983b4a15b9",
+  "created_at": "2026-05-01T07:52:17.813847138Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9a6a85c1",
+    "title": "docs: create docs/helix/02-design/concepts/bounded-context-execution.md",
+    "acceptance": "File exists at docs/helix/02-design/concepts/bounded-context-execution.md. Sections: (1) Context rot — definition, what causes it, why it degrades output quality; (2) The ralph loop — the unbounded session failure pattern; (3) DDx's answer — each bead = one bounded execution unit = one fresh agent invocation in an isolated worktree; ddx work is a bounded context execution loop, not a persistent session; (4) External references — links to 'Lost in the Middle' paper and other published research on in-context degradation. Cross-links to FEAT-010 and execute-loop docs.",
+    "parent": "ddx-dcee9b0c",
+    "labels": [
+      "area:docs",
+      "bounded-context",
+      "concepts"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T07:52:16Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3396374",
+      "execute-loop-heartbeat-at": "2026-05-01T07:52:16.884940154Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T075216-19982ef8",
+    "prompt": ".ddx/executions/20260501T075216-19982ef8/prompt.md",
+    "manifest": ".ddx/executions/20260501T075216-19982ef8/manifest.json",
+    "result": ".ddx/executions/20260501T075216-19982ef8/result.json",
+    "checks": ".ddx/executions/20260501T075216-19982ef8/checks.json",
+    "usage": ".ddx/executions/20260501T075216-19982ef8/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9a6a85c1-20260501T075216-19982ef8"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T075216-19982ef8/result.json b/.ddx/executions/20260501T075216-19982ef8/result.json
new file mode 100644
index 00000000..817d72bc
--- /dev/null
+++ b/.ddx/executions/20260501T075216-19982ef8/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9a6a85c1",
+  "attempt_id": "20260501T075216-19982ef8",
+  "base_rev": "11ef41ae279f9dba9b1e1d6b3a9594983b4a15b9",
+  "result_rev": "d20c1535c9b79a07b68222a5b648ca3120470422",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a5e11f84",
+  "duration_ms": 111021,
+  "tokens": 5674,
+  "cost_usd": 0.573051,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T075216-19982ef8",
+  "prompt_file": ".ddx/executions/20260501T075216-19982ef8/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T075216-19982ef8/manifest.json",
+  "result_file": ".ddx/executions/20260501T075216-19982ef8/result.json",
+  "usage_file": ".ddx/executions/20260501T075216-19982ef8/usage.json",
+  "started_at": "2026-05-01T07:52:17.814136471Z",
+  "finished_at": "2026-05-01T07:54:08.836024985Z"
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
