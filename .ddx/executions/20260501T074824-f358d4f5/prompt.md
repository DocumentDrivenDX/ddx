<bead-review>
  <bead id="ddx-38d773c1" iter=1>
    <title>website: record bootstrap→beads→work demo for website/static/demos/</title>
    <description/>
    <acceptance>
60-90s terminal screencast committed to website/static/demos/ covering: ddx init, ddx install helix, create beads, ddx work draining queue with agent dispatch visible; homepage demo player references this file; old 07-quickstart.cast replaced or archived
    </acceptance>
    <labels>area:website, demo</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T074403-a6c3369f/manifest.json</file>
    <file>.ddx/executions/20260501T074403-a6c3369f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3c00e49a83e60b4997461a5602feb6dda006fbf3">
diff --git a/.ddx/executions/20260501T074403-a6c3369f/manifest.json b/.ddx/executions/20260501T074403-a6c3369f/manifest.json
new file mode 100644
index 00000000..3bdb2553
--- /dev/null
+++ b/.ddx/executions/20260501T074403-a6c3369f/manifest.json
@@ -0,0 +1,35 @@
+{
+  "attempt_id": "20260501T074403-a6c3369f",
+  "bead_id": "ddx-38d773c1",
+  "base_rev": "43c7146ebe06f4295fa60697a9317e33e81ff349",
+  "created_at": "2026-05-01T07:44:04.825582832Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-38d773c1",
+    "title": "website: record bootstrap→beads→work demo for website/static/demos/",
+    "acceptance": "60-90s terminal screencast committed to website/static/demos/ covering: ddx init, ddx install helix, create beads, ddx work draining queue with agent dispatch visible; homepage demo player references this file; old 07-quickstart.cast replaced or archived",
+    "parent": "ddx-4b202bbb",
+    "labels": [
+      "area:website",
+      "demo"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T07:44:03Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3396374",
+      "execute-loop-heartbeat-at": "2026-05-01T07:44:03.93520672Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T074403-a6c3369f",
+    "prompt": ".ddx/executions/20260501T074403-a6c3369f/prompt.md",
+    "manifest": ".ddx/executions/20260501T074403-a6c3369f/manifest.json",
+    "result": ".ddx/executions/20260501T074403-a6c3369f/result.json",
+    "checks": ".ddx/executions/20260501T074403-a6c3369f/checks.json",
+    "usage": ".ddx/executions/20260501T074403-a6c3369f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-38d773c1-20260501T074403-a6c3369f"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T074403-a6c3369f/result.json b/.ddx/executions/20260501T074403-a6c3369f/result.json
new file mode 100644
index 00000000..23bd7d44
--- /dev/null
+++ b/.ddx/executions/20260501T074403-a6c3369f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-38d773c1",
+  "attempt_id": "20260501T074403-a6c3369f",
+  "base_rev": "43c7146ebe06f4295fa60697a9317e33e81ff349",
+  "result_rev": "4daad57009568813f428cb19cb7e84596996d29e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-00afe57d",
+  "duration_ms": 255772,
+  "tokens": 15038,
+  "cost_usd": 1.65685275,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T074403-a6c3369f",
+  "prompt_file": ".ddx/executions/20260501T074403-a6c3369f/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T074403-a6c3369f/manifest.json",
+  "result_file": ".ddx/executions/20260501T074403-a6c3369f/result.json",
+  "usage_file": ".ddx/executions/20260501T074403-a6c3369f/usage.json",
+  "started_at": "2026-05-01T07:44:04.82588129Z",
+  "finished_at": "2026-05-01T07:48:20.597927563Z"
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
