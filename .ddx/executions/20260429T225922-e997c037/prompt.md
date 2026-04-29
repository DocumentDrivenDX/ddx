<bead-review>
  <bead id="ddx-acafd9b6" iter=1>
    <title>[artifact-run-arch] ship adversarial-review skill</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. New skill at skills/ddx/adversarial-review/. Workflow skill for adversarial review composing ddx run primitives.
    </description>
    <acceptance/>
    <labels>skill, plan-2026-04-29, library</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T225552-48a4eed2/manifest.json</file>
    <file>.ddx/executions/20260429T225552-48a4eed2/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3dfe7a313a2a4053fb0b31d26217d35465cd779a">
diff --git a/.ddx/executions/20260429T225552-48a4eed2/manifest.json b/.ddx/executions/20260429T225552-48a4eed2/manifest.json
new file mode 100644
index 00000000..d6259cb0
--- /dev/null
+++ b/.ddx/executions/20260429T225552-48a4eed2/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T225552-48a4eed2",
+  "bead_id": "ddx-acafd9b6",
+  "base_rev": "798a82ce5758928b9c24206baa4e711979ec7348",
+  "created_at": "2026-04-29T22:55:53.02599362Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-acafd9b6",
+    "title": "[artifact-run-arch] ship adversarial-review skill",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. New skill at skills/ddx/adversarial-review/. Workflow skill for adversarial review composing ddx run primitives.",
+    "labels": [
+      "skill",
+      "plan-2026-04-29",
+      "library"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T22:55:50Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T22:55:50.250220164Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T225552-48a4eed2",
+    "prompt": ".ddx/executions/20260429T225552-48a4eed2/prompt.md",
+    "manifest": ".ddx/executions/20260429T225552-48a4eed2/manifest.json",
+    "result": ".ddx/executions/20260429T225552-48a4eed2/result.json",
+    "checks": ".ddx/executions/20260429T225552-48a4eed2/checks.json",
+    "usage": ".ddx/executions/20260429T225552-48a4eed2/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-acafd9b6-20260429T225552-48a4eed2"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T225552-48a4eed2/result.json b/.ddx/executions/20260429T225552-48a4eed2/result.json
new file mode 100644
index 00000000..4b5321cf
--- /dev/null
+++ b/.ddx/executions/20260429T225552-48a4eed2/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-acafd9b6",
+  "attempt_id": "20260429T225552-48a4eed2",
+  "base_rev": "798a82ce5758928b9c24206baa4e711979ec7348",
+  "result_rev": "9b6ede22162e5d747079f5b4572e8eccdee1724f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-e66622d5",
+  "duration_ms": 206090,
+  "tokens": 8023,
+  "cost_usd": 0.52279705,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T225552-48a4eed2",
+  "prompt_file": ".ddx/executions/20260429T225552-48a4eed2/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T225552-48a4eed2/manifest.json",
+  "result_file": ".ddx/executions/20260429T225552-48a4eed2/result.json",
+  "usage_file": ".ddx/executions/20260429T225552-48a4eed2/usage.json",
+  "started_at": "2026-04-29T22:55:53.02624662Z",
+  "finished_at": "2026-04-29T22:59:19.116895644Z"
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
