<bead-review>
  <bead id="ddx-9f7a04f4" iter=1>
    <title>bd/br external-backend support for non-default collections (ADR-004 step 6)</title>
    <description>
Extend cli/internal/bead/backend_external.go so bd and br backends can serve non-'beads' collections (notably 'beads-archive') without breaking the existing 'beads' interchange contract. Strategy: route per-collection list/import operations through bd/br when the backend supports it; otherwise fall back to JSONL-backed file under .ddx/ for the non-default collection. Keep schema_compat_test.go locking the shared envelope.

Depends on ddx-2f453147.
    </description>
    <acceptance>
1. External backend opens 'beads-archive' collection without panicking and produces a working read/write path (real or fallback). 2. bd/br interchange for default 'beads' collection unchanged: schema_compat_test.go and existing import/export tests still pass. 3. Behavior documented in code comments where the routing decision is made. 4. New test covers opening 'beads-archive' through the external backend.
    </acceptance>
    <labels>area:beads, area:storage, kind:feature, adr:004</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T050626-d4361471/manifest.json</file>
    <file>.ddx/executions/20260502T050626-d4361471/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9ede5414e01b7f7528971fa38ddfa4c37ddd8555">
diff --git a/.ddx/executions/20260502T050626-d4361471/manifest.json b/.ddx/executions/20260502T050626-d4361471/manifest.json
new file mode 100644
index 00000000..287aaefa
--- /dev/null
+++ b/.ddx/executions/20260502T050626-d4361471/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T050626-d4361471",
+  "bead_id": "ddx-9f7a04f4",
+  "base_rev": "f4164cc77fbbff07e0c9142b08f4c6baf5f471a4",
+  "created_at": "2026-05-02T05:06:28.034748401Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9f7a04f4",
+    "title": "bd/br external-backend support for non-default collections (ADR-004 step 6)",
+    "description": "Extend cli/internal/bead/backend_external.go so bd and br backends can serve non-'beads' collections (notably 'beads-archive') without breaking the existing 'beads' interchange contract. Strategy: route per-collection list/import operations through bd/br when the backend supports it; otherwise fall back to JSONL-backed file under .ddx/ for the non-default collection. Keep schema_compat_test.go locking the shared envelope.\n\nDepends on ddx-2f453147.",
+    "acceptance": "1. External backend opens 'beads-archive' collection without panicking and produces a working read/write path (real or fallback). 2. bd/br interchange for default 'beads' collection unchanged: schema_compat_test.go and existing import/export tests still pass. 3. Behavior documented in code comments where the routing decision is made. 4. New test covers opening 'beads-archive' through the external backend.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "adr:004"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T05:06:26Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T05:06:26.695648043Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T050626-d4361471",
+    "prompt": ".ddx/executions/20260502T050626-d4361471/prompt.md",
+    "manifest": ".ddx/executions/20260502T050626-d4361471/manifest.json",
+    "result": ".ddx/executions/20260502T050626-d4361471/result.json",
+    "checks": ".ddx/executions/20260502T050626-d4361471/checks.json",
+    "usage": ".ddx/executions/20260502T050626-d4361471/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9f7a04f4-20260502T050626-d4361471"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T050626-d4361471/result.json b/.ddx/executions/20260502T050626-d4361471/result.json
new file mode 100644
index 00000000..8cb5e462
--- /dev/null
+++ b/.ddx/executions/20260502T050626-d4361471/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9f7a04f4",
+  "attempt_id": "20260502T050626-d4361471",
+  "base_rev": "f4164cc77fbbff07e0c9142b08f4c6baf5f471a4",
+  "result_rev": "078352b4066822234650fca6c50db726cf0d3001",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-cc9bb34f",
+  "duration_ms": 412195,
+  "tokens": 21832,
+  "cost_usd": 2.6642709999999994,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T050626-d4361471",
+  "prompt_file": ".ddx/executions/20260502T050626-d4361471/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T050626-d4361471/manifest.json",
+  "result_file": ".ddx/executions/20260502T050626-d4361471/result.json",
+  "usage_file": ".ddx/executions/20260502T050626-d4361471/usage.json",
+  "started_at": "2026-05-02T05:06:28.035075734Z",
+  "finished_at": "2026-05-02T05:13:20.230905724Z"
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
