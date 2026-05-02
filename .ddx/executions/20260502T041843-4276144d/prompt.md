<bead-review>
  <bead id="ddx-5d67c121" iter=1>
    <title>Add attachment storage model for bead events arrays</title>
    <description>
Per ADR-004's attachment model and the TD from C1: store the events array of closed/archived beads under .ddx/attachments/&lt;bead-id&gt;/events.jsonl (or whatever layout the TD picked) instead of inline in the bead row. The bead row carries only a reference. Lazy-load when 'ddx bead show &lt;id&gt;' or other event-aware tools query the bead.
    </description>
    <acceptance>
1. New closed beads write events to attachment store, not inline. 2. ddx bead show &lt;id&gt; reads attachments transparently. 3. Bead-row size reduced (verify by stat on a few attachments-using beads). 4. bd/br export still produces a single inline events array (interchange compatibility). 5. Tests: 'cd cli &amp;&amp; go test -run TestAttachment ./internal/bead/'.
    </acceptance>
    <labels>area:beads, area:storage, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T040950-3adbe727/manifest.json</file>
    <file>.ddx/executions/20260502T040950-3adbe727/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="21b35da5894bf2e0197c3a9880f30faf4f7c371e">
diff --git a/.ddx/executions/20260502T040950-3adbe727/manifest.json b/.ddx/executions/20260502T040950-3adbe727/manifest.json
new file mode 100644
index 00000000..4fb4f673
--- /dev/null
+++ b/.ddx/executions/20260502T040950-3adbe727/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T040950-3adbe727",
+  "bead_id": "ddx-5d67c121",
+  "base_rev": "b957c4dd6950172096007a82114882c9f3b61c5d",
+  "created_at": "2026-05-02T04:09:52.293537218Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-5d67c121",
+    "title": "Add attachment storage model for bead events arrays",
+    "description": "Per ADR-004's attachment model and the TD from C1: store the events array of closed/archived beads under .ddx/attachments/\u003cbead-id\u003e/events.jsonl (or whatever layout the TD picked) instead of inline in the bead row. The bead row carries only a reference. Lazy-load when 'ddx bead show \u003cid\u003e' or other event-aware tools query the bead.",
+    "acceptance": "1. New closed beads write events to attachment store, not inline. 2. ddx bead show \u003cid\u003e reads attachments transparently. 3. Bead-row size reduced (verify by stat on a few attachments-using beads). 4. bd/br export still produces a single inline events array (interchange compatibility). 5. Tests: 'cd cli \u0026\u0026 go test -run TestAttachment ./internal/bead/'.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T04:09:50Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T04:09:50.73061168Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T040950-3adbe727",
+    "prompt": ".ddx/executions/20260502T040950-3adbe727/prompt.md",
+    "manifest": ".ddx/executions/20260502T040950-3adbe727/manifest.json",
+    "result": ".ddx/executions/20260502T040950-3adbe727/result.json",
+    "checks": ".ddx/executions/20260502T040950-3adbe727/checks.json",
+    "usage": ".ddx/executions/20260502T040950-3adbe727/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-5d67c121-20260502T040950-3adbe727"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T040950-3adbe727/result.json b/.ddx/executions/20260502T040950-3adbe727/result.json
new file mode 100644
index 00000000..14516903
--- /dev/null
+++ b/.ddx/executions/20260502T040950-3adbe727/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-5d67c121",
+  "attempt_id": "20260502T040950-3adbe727",
+  "base_rev": "b957c4dd6950172096007a82114882c9f3b61c5d",
+  "result_rev": "dcd29dd528e2a81a8192f78e9924ce04cf975b62",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-11779ff2",
+  "duration_ms": 526378,
+  "tokens": 28427,
+  "cost_usd": 4.494042250000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T040950-3adbe727",
+  "prompt_file": ".ddx/executions/20260502T040950-3adbe727/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T040950-3adbe727/manifest.json",
+  "result_file": ".ddx/executions/20260502T040950-3adbe727/result.json",
+  "usage_file": ".ddx/executions/20260502T040950-3adbe727/usage.json",
+  "started_at": "2026-05-02T04:09:52.293907426Z",
+  "finished_at": "2026-05-02T04:18:38.672005566Z"
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
