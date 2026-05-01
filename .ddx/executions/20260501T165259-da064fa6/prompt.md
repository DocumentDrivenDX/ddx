<bead-review>
  <bead id="ddx-76175851" iter=1>
    <title>[visual-suite] V10 author + generate user-workflow capstone</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-visual-suite.md. The coup de grace. Iterative diagram showing how a user works with DDx over time: author/refine artifact -&gt; graph synthesizes context -&gt; bead created -&gt; agent runs (ddx try / ddx work) -&gt; evidence captured -&gt; human re-aligns -&gt; loop closes. Explanatory register; load-bearing artifact. Lives at docs/helix/01-frame/visuals/user-workflow.{prompt.md,png,png.ddx.yaml}. HARD BLOCKED on artifact infrastructure (same as V6/V9). Depends on V6 and V9 outcomes (visual style established).
    </description>
    <acceptance/>
    <labels>generate, plan-2026-04-29-vis, workflow, capstone</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T165030-979aff9f/manifest.json</file>
    <file>.ddx/executions/20260501T165030-979aff9f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="31b8a603f79f1992a147760158b93a3c9d290599">
diff --git a/.ddx/executions/20260501T165030-979aff9f/manifest.json b/.ddx/executions/20260501T165030-979aff9f/manifest.json
new file mode 100644
index 00000000..486e11c7
--- /dev/null
+++ b/.ddx/executions/20260501T165030-979aff9f/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260501T165030-979aff9f",
+  "bead_id": "ddx-76175851",
+  "base_rev": "13bda64d1113e7d322388533a62df08899587405",
+  "created_at": "2026-05-01T16:50:30.850358568Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-76175851",
+    "title": "[visual-suite] V10 author + generate user-workflow capstone",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-visual-suite.md. The coup de grace. Iterative diagram showing how a user works with DDx over time: author/refine artifact -\u003e graph synthesizes context -\u003e bead created -\u003e agent runs (ddx try / ddx work) -\u003e evidence captured -\u003e human re-aligns -\u003e loop closes. Explanatory register; load-bearing artifact. Lives at docs/helix/01-frame/visuals/user-workflow.{prompt.md,png,png.ddx.yaml}. HARD BLOCKED on artifact infrastructure (same as V6/V9). Depends on V6 and V9 outcomes (visual style established).",
+    "labels": [
+      "generate",
+      "plan-2026-04-29-vis",
+      "workflow",
+      "capstone"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T16:50:29Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "566701",
+      "execute-loop-heartbeat-at": "2026-05-01T16:50:29.990199227Z",
+      "execution-eligible": true
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T165030-979aff9f",
+    "prompt": ".ddx/executions/20260501T165030-979aff9f/prompt.md",
+    "manifest": ".ddx/executions/20260501T165030-979aff9f/manifest.json",
+    "result": ".ddx/executions/20260501T165030-979aff9f/result.json",
+    "checks": ".ddx/executions/20260501T165030-979aff9f/checks.json",
+    "usage": ".ddx/executions/20260501T165030-979aff9f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-76175851-20260501T165030-979aff9f"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T165030-979aff9f/result.json b/.ddx/executions/20260501T165030-979aff9f/result.json
new file mode 100644
index 00000000..4e3a23e9
--- /dev/null
+++ b/.ddx/executions/20260501T165030-979aff9f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-76175851",
+  "attempt_id": "20260501T165030-979aff9f",
+  "base_rev": "13bda64d1113e7d322388533a62df08899587405",
+  "result_rev": "cbb9d946318bf0e2622783569b117ea424e7f9fe",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-66501da1",
+  "duration_ms": 143278,
+  "tokens": 8071,
+  "cost_usd": 0.7998595000000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T165030-979aff9f",
+  "prompt_file": ".ddx/executions/20260501T165030-979aff9f/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T165030-979aff9f/manifest.json",
+  "result_file": ".ddx/executions/20260501T165030-979aff9f/result.json",
+  "usage_file": ".ddx/executions/20260501T165030-979aff9f/usage.json",
+  "started_at": "2026-05-01T16:50:30.850714732Z",
+  "finished_at": "2026-05-01T16:52:54.128831506Z"
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
