<bead-review>
  <bead id="ddx-ad77c46d" iter=1>
    <title>operator-prompts: schema/template/status-model — operator-prompt type, proposed status, no-self-mutation rule</title>
    <description>
New bead.issueType: operator-prompt with new status: proposed. Operator-prompt cannot mutate other operator-prompts (hard ban). Library template for operator-prompt artifacts.
    </description>
    <acceptance>
1. Bead schema accepts operator-prompt. 2. proposed status implemented. 3. Self-mutation rejected at bead-create time. 4. Tests cover all three.
    </acceptance>
    <labels>phase:2, story:15, area:beads, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T140540-155225b7/manifest.json</file>
    <file>.ddx/executions/20260502T140540-155225b7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="90ee4794ab527b86714760c51677d5070e75e6fb">
diff --git a/.ddx/executions/20260502T140540-155225b7/manifest.json b/.ddx/executions/20260502T140540-155225b7/manifest.json
new file mode 100644
index 00000000..0b29301c
--- /dev/null
+++ b/.ddx/executions/20260502T140540-155225b7/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T140540-155225b7",
+  "bead_id": "ddx-ad77c46d",
+  "base_rev": "7b1ec8eda06cd65ed0e5e0264b9e1e7c81c7611a",
+  "created_at": "2026-05-02T14:05:42.215610501Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ad77c46d",
+    "title": "operator-prompts: schema/template/status-model — operator-prompt type, proposed status, no-self-mutation rule",
+    "description": "New bead.issueType: operator-prompt with new status: proposed. Operator-prompt cannot mutate other operator-prompts (hard ban). Library template for operator-prompt artifacts.",
+    "acceptance": "1. Bead schema accepts operator-prompt. 2. proposed status implemented. 3. Self-mutation rejected at bead-create time. 4. Tests cover all three.",
+    "parent": "ddx-1d85c927",
+    "labels": [
+      "phase:2",
+      "story:15",
+      "area:beads",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T14:05:40Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1727028",
+      "execute-loop-heartbeat-at": "2026-05-02T14:05:40.559738678Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T140540-155225b7",
+    "prompt": ".ddx/executions/20260502T140540-155225b7/prompt.md",
+    "manifest": ".ddx/executions/20260502T140540-155225b7/manifest.json",
+    "result": ".ddx/executions/20260502T140540-155225b7/result.json",
+    "checks": ".ddx/executions/20260502T140540-155225b7/checks.json",
+    "usage": ".ddx/executions/20260502T140540-155225b7/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ad77c46d-20260502T140540-155225b7"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T140540-155225b7/result.json b/.ddx/executions/20260502T140540-155225b7/result.json
new file mode 100644
index 00000000..a713b659
--- /dev/null
+++ b/.ddx/executions/20260502T140540-155225b7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ad77c46d",
+  "attempt_id": "20260502T140540-155225b7",
+  "base_rev": "7b1ec8eda06cd65ed0e5e0264b9e1e7c81c7611a",
+  "result_rev": "a8c438885dbf94462a01c3f65aae8253f928612d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-459ac8bf",
+  "duration_ms": 228368,
+  "tokens": 11725,
+  "cost_usd": 1.7738884999999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T140540-155225b7",
+  "prompt_file": ".ddx/executions/20260502T140540-155225b7/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T140540-155225b7/manifest.json",
+  "result_file": ".ddx/executions/20260502T140540-155225b7/result.json",
+  "usage_file": ".ddx/executions/20260502T140540-155225b7/usage.json",
+  "started_at": "2026-05-02T14:05:42.215900834Z",
+  "finished_at": "2026-05-02T14:09:30.58485844Z"
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
