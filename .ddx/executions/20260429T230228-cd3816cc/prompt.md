<bead-review>
  <bead id="ddx-3e21a299" iter=1>
    <title>[artifact-run-arch] ship bead-breakdown skill</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. New skill at skills/ddx/bead-breakdown/. Workflow skill for breaking down work into beads composing ddx run primitives.
    </description>
    <acceptance/>
    <labels>skill, plan-2026-04-29, library</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T230057-01686eb4/manifest.json</file>
    <file>.ddx/executions/20260429T230057-01686eb4/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f810bcd9d507875592781e000e31499161091f5d">
diff --git a/.ddx/executions/20260429T230057-01686eb4/manifest.json b/.ddx/executions/20260429T230057-01686eb4/manifest.json
new file mode 100644
index 00000000..27215296
--- /dev/null
+++ b/.ddx/executions/20260429T230057-01686eb4/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T230057-01686eb4",
+  "bead_id": "ddx-3e21a299",
+  "base_rev": "9ff6420a009d5bf59befa95bb8c64561a70a1b5e",
+  "created_at": "2026-04-29T23:00:58.403294624Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3e21a299",
+    "title": "[artifact-run-arch] ship bead-breakdown skill",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. New skill at skills/ddx/bead-breakdown/. Workflow skill for breaking down work into beads composing ddx run primitives.",
+    "labels": [
+      "skill",
+      "plan-2026-04-29",
+      "library"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T23:00:55Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T23:00:55.606513099Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T230057-01686eb4",
+    "prompt": ".ddx/executions/20260429T230057-01686eb4/prompt.md",
+    "manifest": ".ddx/executions/20260429T230057-01686eb4/manifest.json",
+    "result": ".ddx/executions/20260429T230057-01686eb4/result.json",
+    "checks": ".ddx/executions/20260429T230057-01686eb4/checks.json",
+    "usage": ".ddx/executions/20260429T230057-01686eb4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3e21a299-20260429T230057-01686eb4"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T230057-01686eb4/result.json b/.ddx/executions/20260429T230057-01686eb4/result.json
new file mode 100644
index 00000000..d11f6161
--- /dev/null
+++ b/.ddx/executions/20260429T230057-01686eb4/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-3e21a299",
+  "attempt_id": "20260429T230057-01686eb4",
+  "base_rev": "9ff6420a009d5bf59befa95bb8c64561a70a1b5e",
+  "result_rev": "948822dfa2fe77765202492a360ab0dafae21038",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-dbf9dd18",
+  "duration_ms": 86796,
+  "tokens": 4070,
+  "cost_usd": 0.23758255,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T230057-01686eb4",
+  "prompt_file": ".ddx/executions/20260429T230057-01686eb4/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T230057-01686eb4/manifest.json",
+  "result_file": ".ddx/executions/20260429T230057-01686eb4/result.json",
+  "usage_file": ".ddx/executions/20260429T230057-01686eb4/usage.json",
+  "started_at": "2026-04-29T23:00:58.403558499Z",
+  "finished_at": "2026-04-29T23:02:25.199969809Z"
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
