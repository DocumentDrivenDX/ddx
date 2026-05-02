<bead-review>
  <bead id="ddx-8fcfe2a7" iter=1>
    <title>ddx bead archive command + size-based trigger (ADR-004 step 4)</title>
    <description>
Add an explicit 'ddx bead archive' command that moves closed beads from the active collection into beads-archive, optionally extracting their events into the attachment sidecar (uses the API from ddx-cd1f0f7e). Archival selection is configurable: default is closed-status AND beads.jsonl size exceeds threshold (default 4MB). Provide CLI flags --max-size, --older-than, --max-count to override. Operation is atomic per-bead: row appears in archive (and sidecar exists) before the active row is removed, using existing lock+rewrite primitives.

Decisions taken from epic ddx-0c0565f3: archival trigger defaults to file-size (&gt;4MB) with closed-only filter; closed-bead events DO move into attachment sidecar. Depends on ddx-f7f09b6e and ddx-cd1f0f7e.
    </description>
    <acceptance>
1. 'ddx bead archive' command exists with help text. 2. Default invocation archives closed beads only, and only when active file size exceeds the configured threshold. 3. --max-size, --older-than, --max-count flags override the trigger. 4. After archive: active file size shrinks, archive grows, attachment sidecars created for moved events. 5. Operation is crash-safe (active row only removed after archive write succeeds). 6. ddx bead show &lt;id&gt; still works for archived beads (via read-through from ddx-f7f09b6e). 7. Test covers a synthetic 5MB beads.jsonl shrinking under threshold post-archive.
    </acceptance>
    <labels>area:beads, area:storage, kind:feature, adr:004</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T045420-3ab0acb7/manifest.json</file>
    <file>.ddx/executions/20260502T045420-3ab0acb7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="cd0b37b5aa12ed73187fa618b42ef6b41dce237f">
diff --git a/.ddx/executions/20260502T045420-3ab0acb7/manifest.json b/.ddx/executions/20260502T045420-3ab0acb7/manifest.json
new file mode 100644
index 00000000..4ffc5d00
--- /dev/null
+++ b/.ddx/executions/20260502T045420-3ab0acb7/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T045420-3ab0acb7",
+  "bead_id": "ddx-8fcfe2a7",
+  "base_rev": "5fc2a5538221497b5b251fa716a99309faaa4d31",
+  "created_at": "2026-05-02T04:54:21.463753299Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-8fcfe2a7",
+    "title": "ddx bead archive command + size-based trigger (ADR-004 step 4)",
+    "description": "Add an explicit 'ddx bead archive' command that moves closed beads from the active collection into beads-archive, optionally extracting their events into the attachment sidecar (uses the API from ddx-cd1f0f7e). Archival selection is configurable: default is closed-status AND beads.jsonl size exceeds threshold (default 4MB). Provide CLI flags --max-size, --older-than, --max-count to override. Operation is atomic per-bead: row appears in archive (and sidecar exists) before the active row is removed, using existing lock+rewrite primitives.\n\nDecisions taken from epic ddx-0c0565f3: archival trigger defaults to file-size (\u003e4MB) with closed-only filter; closed-bead events DO move into attachment sidecar. Depends on ddx-f7f09b6e and ddx-cd1f0f7e.",
+    "acceptance": "1. 'ddx bead archive' command exists with help text. 2. Default invocation archives closed beads only, and only when active file size exceeds the configured threshold. 3. --max-size, --older-than, --max-count flags override the trigger. 4. After archive: active file size shrinks, archive grows, attachment sidecars created for moved events. 5. Operation is crash-safe (active row only removed after archive write succeeds). 6. ddx bead show \u003cid\u003e still works for archived beads (via read-through from ddx-f7f09b6e). 7. Test covers a synthetic 5MB beads.jsonl shrinking under threshold post-archive.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "adr:004"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T04:54:20Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T04:54:20.168029722Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T045420-3ab0acb7",
+    "prompt": ".ddx/executions/20260502T045420-3ab0acb7/prompt.md",
+    "manifest": ".ddx/executions/20260502T045420-3ab0acb7/manifest.json",
+    "result": ".ddx/executions/20260502T045420-3ab0acb7/result.json",
+    "checks": ".ddx/executions/20260502T045420-3ab0acb7/checks.json",
+    "usage": ".ddx/executions/20260502T045420-3ab0acb7/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8fcfe2a7-20260502T045420-3ab0acb7"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T045420-3ab0acb7/result.json b/.ddx/executions/20260502T045420-3ab0acb7/result.json
new file mode 100644
index 00000000..c85161ab
--- /dev/null
+++ b/.ddx/executions/20260502T045420-3ab0acb7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-8fcfe2a7",
+  "attempt_id": "20260502T045420-3ab0acb7",
+  "base_rev": "5fc2a5538221497b5b251fa716a99309faaa4d31",
+  "result_rev": "4875afbdc8531e93eccb0cacc9780aa6f8732e7d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-096e9294",
+  "duration_ms": 385565,
+  "tokens": 22409,
+  "cost_usd": 2.545452,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T045420-3ab0acb7",
+  "prompt_file": ".ddx/executions/20260502T045420-3ab0acb7/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T045420-3ab0acb7/manifest.json",
+  "result_file": ".ddx/executions/20260502T045420-3ab0acb7/result.json",
+  "usage_file": ".ddx/executions/20260502T045420-3ab0acb7/usage.json",
+  "started_at": "2026-05-02T04:54:21.464026424Z",
+  "finished_at": "2026-05-02T05:00:47.029661363Z"
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
