<bead-review>
  <bead id="ddx-296019fe" iter=1>
    <title>metric: integration round-trip test + mixed-unit refusal</title>
    <description>
End-to-end: define MET → exec.run → metric history populated → ddx metric show reads back. Compare/Trend refuse mixed units; History groups by unit.
    </description>
    <acceptance>
1. Integration test covers full round-trip. 2. Compare/Trend refuse mixed units with clear error. 3. History grouping correct.
    </acceptance>
    <labels>phase:2, story:13, area:tests, kind:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T152806-9d9de558/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T152806-9d9de558/manifest.json</file>
    <file>.ddx/executions/20260505T152806-9d9de558/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="20cc5c6b17170528f48bf9ceeaf55b783c77494c">
diff --git a/.ddx/executions/20260505T152806-9d9de558/checks/production-reachability.json b/.ddx/executions/20260505T152806-9d9de558/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T152806-9d9de558/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T152806-9d9de558/manifest.json b/.ddx/executions/20260505T152806-9d9de558/manifest.json
new file mode 100644
index 00000000..52e4b871
--- /dev/null
+++ b/.ddx/executions/20260505T152806-9d9de558/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260505T152806-9d9de558",
+  "bead_id": "ddx-296019fe",
+  "base_rev": "b0ed49825be9b52a5d0c602bd15b43c05eda1b88",
+  "created_at": "2026-05-05T15:28:08.819959165Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-296019fe",
+    "title": "metric: integration round-trip test + mixed-unit refusal",
+    "description": "End-to-end: define MET → exec.run → metric history populated → ddx metric show reads back. Compare/Trend refuse mixed units; History groups by unit.",
+    "acceptance": "1. Integration test covers full round-trip. 2. Compare/Trend refuse mixed units with clear error. 3. History grouping correct.",
+    "parent": "ddx-921616ea",
+    "labels": [
+      "phase:2",
+      "story:13",
+      "area:tests",
+      "kind:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T15:28:05Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "execute-loop-heartbeat-at": "2026-05-05T15:28:05.986569001Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T152806-9d9de558",
+    "prompt": ".ddx/executions/20260505T152806-9d9de558/prompt.md",
+    "manifest": ".ddx/executions/20260505T152806-9d9de558/manifest.json",
+    "result": ".ddx/executions/20260505T152806-9d9de558/result.json",
+    "checks": ".ddx/executions/20260505T152806-9d9de558/checks.json",
+    "usage": ".ddx/executions/20260505T152806-9d9de558/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-296019fe-20260505T152806-9d9de558"
+  },
+  "prompt_sha": "3106020ff44d73318ee00aff8282a18c70b90f1c9d42f420b54b90fe97f49823"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T152806-9d9de558/result.json b/.ddx/executions/20260505T152806-9d9de558/result.json
new file mode 100644
index 00000000..240e1ede
--- /dev/null
+++ b/.ddx/executions/20260505T152806-9d9de558/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-296019fe",
+  "attempt_id": "20260505T152806-9d9de558",
+  "base_rev": "b0ed49825be9b52a5d0c602bd15b43c05eda1b88",
+  "result_rev": "c538a60583f20efe23e108715ac0aeb443a56397",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-c2778a60",
+  "duration_ms": 676487,
+  "tokens": 12752991,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T152806-9d9de558",
+  "prompt_file": ".ddx/executions/20260505T152806-9d9de558/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T152806-9d9de558/manifest.json",
+  "result_file": ".ddx/executions/20260505T152806-9d9de558/result.json",
+  "usage_file": ".ddx/executions/20260505T152806-9d9de558/usage.json",
+  "started_at": "2026-05-05T15:28:08.820341998Z",
+  "finished_at": "2026-05-05T15:39:25.307991894Z"
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
