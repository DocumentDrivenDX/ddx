<bead-review>
  <bead id="ddx-d7740325" iter=1>
    <title>B4b: Author TD for prompt versioning, DDX_PROMPT_VARIANT selector, offline comparison</title>
    <description>
Author a new technical-design (TD) document capturing the measurement roadmap per /tmp/story-12-final.md 'New TD' section. Covers: prompt_version field on manifest (after prompt_sha lands in B3), DDX_PROMPT_VARIANT=&lt;name&gt; environment selector for offline A/B, offline-comparison roadmap leveraging existing telemetry (usage.json, session_index.go session tokens, resolver_feat008.go efficacy view). NOT implementing the selector or version field in this story — TD captures the design only.
    </description>
    <acceptance>
AC8 (TD portion): New TD document authored under docs/helix/ (or appropriate TD location) describing prompt_version, DDX_PROMPT_VARIANT selector, and the offline-comparison roadmap. TD is explicit that implementation is deferred.
    </acceptance>
    <labels>phase:2, story:12, tier:cheap</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T122910-3d1551a9/manifest.json</file>
    <file>.ddx/executions/20260502T122910-3d1551a9/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ec3494a2c8942b522a7c86a55ca4817ecfee6e43">
diff --git a/.ddx/executions/20260502T122910-3d1551a9/manifest.json b/.ddx/executions/20260502T122910-3d1551a9/manifest.json
new file mode 100644
index 00000000..6299bb7a
--- /dev/null
+++ b/.ddx/executions/20260502T122910-3d1551a9/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T122910-3d1551a9",
+  "bead_id": "ddx-d7740325",
+  "base_rev": "1569316b633938539491f1e4fa6e8ec0874a8f55",
+  "created_at": "2026-05-02T12:29:11.503222084Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d7740325",
+    "title": "B4b: Author TD for prompt versioning, DDX_PROMPT_VARIANT selector, offline comparison",
+    "description": "Author a new technical-design (TD) document capturing the measurement roadmap per /tmp/story-12-final.md 'New TD' section. Covers: prompt_version field on manifest (after prompt_sha lands in B3), DDX_PROMPT_VARIANT=\u003cname\u003e environment selector for offline A/B, offline-comparison roadmap leveraging existing telemetry (usage.json, session_index.go session tokens, resolver_feat008.go efficacy view). NOT implementing the selector or version field in this story — TD captures the design only.",
+    "acceptance": "AC8 (TD portion): New TD document authored under docs/helix/ (or appropriate TD location) describing prompt_version, DDX_PROMPT_VARIANT selector, and the offline-comparison roadmap. TD is explicit that implementation is deferred.",
+    "parent": "ddx-a61bf8ee",
+    "labels": [
+      "phase:2",
+      "story:12",
+      "tier:cheap"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T12:29:10Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1724970",
+      "execute-loop-heartbeat-at": "2026-05-02T12:29:10.321580032Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T122910-3d1551a9",
+    "prompt": ".ddx/executions/20260502T122910-3d1551a9/prompt.md",
+    "manifest": ".ddx/executions/20260502T122910-3d1551a9/manifest.json",
+    "result": ".ddx/executions/20260502T122910-3d1551a9/result.json",
+    "checks": ".ddx/executions/20260502T122910-3d1551a9/checks.json",
+    "usage": ".ddx/executions/20260502T122910-3d1551a9/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d7740325-20260502T122910-3d1551a9"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T122910-3d1551a9/result.json b/.ddx/executions/20260502T122910-3d1551a9/result.json
new file mode 100644
index 00000000..c3833a0a
--- /dev/null
+++ b/.ddx/executions/20260502T122910-3d1551a9/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d7740325",
+  "attempt_id": "20260502T122910-3d1551a9",
+  "base_rev": "1569316b633938539491f1e4fa6e8ec0874a8f55",
+  "result_rev": "70a573aa8579626295618a8e1baa4b5b38458f04",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-4735b3cb",
+  "duration_ms": 138834,
+  "tokens": 7047,
+  "cost_usd": 0.6817175000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T122910-3d1551a9",
+  "prompt_file": ".ddx/executions/20260502T122910-3d1551a9/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T122910-3d1551a9/manifest.json",
+  "result_file": ".ddx/executions/20260502T122910-3d1551a9/result.json",
+  "usage_file": ".ddx/executions/20260502T122910-3d1551a9/usage.json",
+  "started_at": "2026-05-02T12:29:11.503459458Z",
+  "finished_at": "2026-05-02T12:31:30.337989045Z"
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
