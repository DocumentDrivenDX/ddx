<bead-review>
  <bead id="ddx-79115cc9" iter=1>
    <title>website: add context rot to /why/ root cause section</title>
    <description/>
    <acceptance>
/why/ root cause section names context rot as an explicit failure mode alongside 'agents execute in isolation with no shared memory'; explains that even within a single run, unbounded execution causes quality decay; bounded context execution named as reason ddx work is designed the way it is. Depends on /why/ page existing (ddx-f53b8c4b).
    </acceptance>
    <labels>area:website, bounded-context</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T075429-5be1bb29/manifest.json</file>
    <file>.ddx/executions/20260501T075429-5be1bb29/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fa71b02c347027977d82080dce64947c71322633">
diff --git a/.ddx/executions/20260501T075429-5be1bb29/manifest.json b/.ddx/executions/20260501T075429-5be1bb29/manifest.json
new file mode 100644
index 00000000..c554fa1d
--- /dev/null
+++ b/.ddx/executions/20260501T075429-5be1bb29/manifest.json
@@ -0,0 +1,35 @@
+{
+  "attempt_id": "20260501T075429-5be1bb29",
+  "bead_id": "ddx-79115cc9",
+  "base_rev": "cc0066719ef9a0ad1616f6e556b804cb9471e011",
+  "created_at": "2026-05-01T07:54:30.076103326Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-79115cc9",
+    "title": "website: add context rot to /why/ root cause section",
+    "acceptance": "/why/ root cause section names context rot as an explicit failure mode alongside 'agents execute in isolation with no shared memory'; explains that even within a single run, unbounded execution causes quality decay; bounded context execution named as reason ddx work is designed the way it is. Depends on /why/ page existing (ddx-f53b8c4b).",
+    "parent": "ddx-dcee9b0c",
+    "labels": [
+      "area:website",
+      "bounded-context"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T07:54:29Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3396374",
+      "execute-loop-heartbeat-at": "2026-05-01T07:54:29.039852797Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T075429-5be1bb29",
+    "prompt": ".ddx/executions/20260501T075429-5be1bb29/prompt.md",
+    "manifest": ".ddx/executions/20260501T075429-5be1bb29/manifest.json",
+    "result": ".ddx/executions/20260501T075429-5be1bb29/result.json",
+    "checks": ".ddx/executions/20260501T075429-5be1bb29/checks.json",
+    "usage": ".ddx/executions/20260501T075429-5be1bb29/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-79115cc9-20260501T075429-5be1bb29"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T075429-5be1bb29/result.json b/.ddx/executions/20260501T075429-5be1bb29/result.json
new file mode 100644
index 00000000..d73243bb
--- /dev/null
+++ b/.ddx/executions/20260501T075429-5be1bb29/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-79115cc9",
+  "attempt_id": "20260501T075429-5be1bb29",
+  "base_rev": "cc0066719ef9a0ad1616f6e556b804cb9471e011",
+  "result_rev": "f26bdfd67bf6de30ed40892e2e4736bb3c5d4002",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-e673aea0",
+  "duration_ms": 47101,
+  "tokens": 2490,
+  "cost_usd": 0.301157,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T075429-5be1bb29",
+  "prompt_file": ".ddx/executions/20260501T075429-5be1bb29/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T075429-5be1bb29/manifest.json",
+  "result_file": ".ddx/executions/20260501T075429-5be1bb29/result.json",
+  "usage_file": ".ddx/executions/20260501T075429-5be1bb29/usage.json",
+  "started_at": "2026-05-01T07:54:30.076424576Z",
+  "finished_at": "2026-05-01T07:55:17.178248849Z"
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
