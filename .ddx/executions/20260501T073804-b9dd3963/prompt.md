<bead-review>
  <bead id="ddx-dbeb3ddc" iter=1>
    <title>website: update /ecosystem/ for DDx platform + HELIX workflow two-layer framing</title>
    <description/>
    <acceptance>
/ecosystem/ page reflects DDx=platform primitives / HELIX=workflow methodology split; plugin registry section; agent landscape section; no Dun references; no persona system references
    </acceptance>
    <labels>area:website, content, ecosystem</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T073638-f5135838/manifest.json</file>
    <file>.ddx/executions/20260501T073638-f5135838/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c3f2d37a0c1a5858ba0c1085200f60ed72d6b3f3">
diff --git a/.ddx/executions/20260501T073638-f5135838/manifest.json b/.ddx/executions/20260501T073638-f5135838/manifest.json
new file mode 100644
index 00000000..07778fec
--- /dev/null
+++ b/.ddx/executions/20260501T073638-f5135838/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260501T073638-f5135838",
+  "bead_id": "ddx-dbeb3ddc",
+  "base_rev": "ecf48f3b39bb02e7f3c75c0d00591881a3f5fdce",
+  "created_at": "2026-05-01T07:36:39.338977995Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-dbeb3ddc",
+    "title": "website: update /ecosystem/ for DDx platform + HELIX workflow two-layer framing",
+    "acceptance": "/ecosystem/ page reflects DDx=platform primitives / HELIX=workflow methodology split; plugin registry section; agent landscape section; no Dun references; no persona system references",
+    "parent": "ddx-4b202bbb",
+    "labels": [
+      "area:website",
+      "content",
+      "ecosystem"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T07:36:38Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3396374",
+      "execute-loop-heartbeat-at": "2026-05-01T07:36:38.2878779Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T073638-f5135838",
+    "prompt": ".ddx/executions/20260501T073638-f5135838/prompt.md",
+    "manifest": ".ddx/executions/20260501T073638-f5135838/manifest.json",
+    "result": ".ddx/executions/20260501T073638-f5135838/result.json",
+    "checks": ".ddx/executions/20260501T073638-f5135838/checks.json",
+    "usage": ".ddx/executions/20260501T073638-f5135838/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-dbeb3ddc-20260501T073638-f5135838"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T073638-f5135838/result.json b/.ddx/executions/20260501T073638-f5135838/result.json
new file mode 100644
index 00000000..b7e90566
--- /dev/null
+++ b/.ddx/executions/20260501T073638-f5135838/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-dbeb3ddc",
+  "attempt_id": "20260501T073638-f5135838",
+  "base_rev": "ecf48f3b39bb02e7f3c75c0d00591881a3f5fdce",
+  "result_rev": "4bf99bacf5a99b881dcc54b6697f1c1ca2cd3ad9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3c328551",
+  "duration_ms": 81759,
+  "tokens": 4922,
+  "cost_usd": 0.4661102499999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T073638-f5135838",
+  "prompt_file": ".ddx/executions/20260501T073638-f5135838/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T073638-f5135838/manifest.json",
+  "result_file": ".ddx/executions/20260501T073638-f5135838/result.json",
+  "usage_file": ".ddx/executions/20260501T073638-f5135838/usage.json",
+  "started_at": "2026-05-01T07:36:39.339306119Z",
+  "finished_at": "2026-05-01T07:38:01.099049763Z"
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
