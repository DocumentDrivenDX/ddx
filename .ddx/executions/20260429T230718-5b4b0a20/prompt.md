<bead-review>
  <bead id="ddx-f2f2e05a" iter=1>
    <title>[visual-suite] V2 ddx doctor: detect package.json across known locations</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-visual-suite.md. Extend ddx doctor to scan known package.json locations (/, website/, cli/internal/server/frontend/) and report 'bun install' if node_modules missing or stale relative to bun.lock. Optionally extend ddx init summary to mention discovered package.json files. ~100 LoC. No new schema, no DDx-owned lockfile, no ddx tool run wrapper — bun owns all that.
    </description>
    <acceptance/>
    <labels>tooling, plan-2026-04-29-vis, doctor</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T230242-d818077b/manifest.json</file>
    <file>.ddx/executions/20260429T230242-d818077b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4c9b39e11efade0da6840bd9fab7d302a334d13c">
diff --git a/.ddx/executions/20260429T230242-d818077b/manifest.json b/.ddx/executions/20260429T230242-d818077b/manifest.json
new file mode 100644
index 00000000..586ac98e
--- /dev/null
+++ b/.ddx/executions/20260429T230242-d818077b/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T230242-d818077b",
+  "bead_id": "ddx-f2f2e05a",
+  "base_rev": "56b255c612f82794dc78750f3696a905faca417e",
+  "created_at": "2026-04-29T23:02:43.689581507Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-f2f2e05a",
+    "title": "[visual-suite] V2 ddx doctor: detect package.json across known locations",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-visual-suite.md. Extend ddx doctor to scan known package.json locations (/, website/, cli/internal/server/frontend/) and report 'bun install' if node_modules missing or stale relative to bun.lock. Optionally extend ddx init summary to mention discovered package.json files. ~100 LoC. No new schema, no DDx-owned lockfile, no ddx tool run wrapper — bun owns all that.",
+    "labels": [
+      "tooling",
+      "plan-2026-04-29-vis",
+      "doctor"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T23:02:40Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T23:02:40.86475533Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T230242-d818077b",
+    "prompt": ".ddx/executions/20260429T230242-d818077b/prompt.md",
+    "manifest": ".ddx/executions/20260429T230242-d818077b/manifest.json",
+    "result": ".ddx/executions/20260429T230242-d818077b/result.json",
+    "checks": ".ddx/executions/20260429T230242-d818077b/checks.json",
+    "usage": ".ddx/executions/20260429T230242-d818077b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-f2f2e05a-20260429T230242-d818077b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T230242-d818077b/result.json b/.ddx/executions/20260429T230242-d818077b/result.json
new file mode 100644
index 00000000..342fc224
--- /dev/null
+++ b/.ddx/executions/20260429T230242-d818077b/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-f2f2e05a",
+  "attempt_id": "20260429T230242-d818077b",
+  "base_rev": "56b255c612f82794dc78750f3696a905faca417e",
+  "result_rev": "63c76efea54e66b19555a48ee00110e3c40682f9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-be62451b",
+  "duration_ms": 270288,
+  "tokens": 9394,
+  "cost_usd": 0.77380305,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T230242-d818077b",
+  "prompt_file": ".ddx/executions/20260429T230242-d818077b/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T230242-d818077b/manifest.json",
+  "result_file": ".ddx/executions/20260429T230242-d818077b/result.json",
+  "usage_file": ".ddx/executions/20260429T230242-d818077b/usage.json",
+  "started_at": "2026-04-29T23:02:43.689806631Z",
+  "finished_at": "2026-04-29T23:07:13.978629803Z"
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
