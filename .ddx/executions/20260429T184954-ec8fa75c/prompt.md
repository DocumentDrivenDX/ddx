<bead-review>
  <bead id="ddx-24a9286f" iter=1>
    <title>[artifact-run-arch] sidecar .ddx.yaml schema design</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. Sidecar pairing rule (foo.png &lt;-&gt; foo.png.ddx.yaml? or foo.ddx.yaml?); conflict rule when both frontmatter and sidecar exist; how sidecars participate in ddx doc validate orphan checks.
    </description>
    <acceptance/>
    <labels>design, plan-2026-04-29, architecture</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T184732-548d1720/manifest.json</file>
    <file>.ddx/executions/20260429T184732-548d1720/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4f19fc6189f91633a458a27e889280a0b46554fd">
diff --git a/.ddx/executions/20260429T184732-548d1720/manifest.json b/.ddx/executions/20260429T184732-548d1720/manifest.json
new file mode 100644
index 00000000..778fc404
--- /dev/null
+++ b/.ddx/executions/20260429T184732-548d1720/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T184732-548d1720",
+  "bead_id": "ddx-24a9286f",
+  "base_rev": "83ca0b3fa97bea3f5d384a4915d8eb9454dc288b",
+  "created_at": "2026-04-29T18:47:33.018062298Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-24a9286f",
+    "title": "[artifact-run-arch] sidecar .ddx.yaml schema design",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. Sidecar pairing rule (foo.png \u003c-\u003e foo.png.ddx.yaml? or foo.ddx.yaml?); conflict rule when both frontmatter and sidecar exist; how sidecars participate in ddx doc validate orphan checks.",
+    "labels": [
+      "design",
+      "plan-2026-04-29",
+      "architecture"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T18:47:30Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T18:47:30.19948814Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T184732-548d1720",
+    "prompt": ".ddx/executions/20260429T184732-548d1720/prompt.md",
+    "manifest": ".ddx/executions/20260429T184732-548d1720/manifest.json",
+    "result": ".ddx/executions/20260429T184732-548d1720/result.json",
+    "checks": ".ddx/executions/20260429T184732-548d1720/checks.json",
+    "usage": ".ddx/executions/20260429T184732-548d1720/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-24a9286f-20260429T184732-548d1720"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T184732-548d1720/result.json b/.ddx/executions/20260429T184732-548d1720/result.json
new file mode 100644
index 00000000..6324bde1
--- /dev/null
+++ b/.ddx/executions/20260429T184732-548d1720/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-24a9286f",
+  "attempt_id": "20260429T184732-548d1720",
+  "base_rev": "83ca0b3fa97bea3f5d384a4915d8eb9454dc288b",
+  "result_rev": "cda6618972e27ae60010b11fed2693046fdac9c3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-dd48aeb4",
+  "duration_ms": 137681,
+  "tokens": 6536,
+  "cost_usd": 0.34150545,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T184732-548d1720",
+  "prompt_file": ".ddx/executions/20260429T184732-548d1720/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T184732-548d1720/manifest.json",
+  "result_file": ".ddx/executions/20260429T184732-548d1720/result.json",
+  "usage_file": ".ddx/executions/20260429T184732-548d1720/usage.json",
+  "started_at": "2026-04-29T18:47:33.018345673Z",
+  "finished_at": "2026-04-29T18:49:50.699597148Z"
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
