<bead-review>
  <bead id="ddx-2502ad71" iter=1>
    <title>scanner: extend skip-list to .ddx/plugins/* + complete duplicate-ID cleanup</title>
    <description>
Predecessor ddx-58764e1b closed but only partially fixed the issue. Scanner skip-list now excludes .agents/.claude (good), but .ddx/plugins/helix/docs/ still gets scanned and produces duplicates for SD-021..023, TD-004..010, TP-002..020, helix.workflow.principles, helix.prd, product-vision, etc. Current ddx doc audit still reports 86 duplicate_id issues. Fix: extend skip-list to also exclude .ddx/plugins/*/docs/ (or .ddx/plugins/* entirely if no other paths there matter). Also resolve missing-dep helix.workflow.principles → helix.workflow.
    </description>
    <acceptance>
1. cli/internal/docgraph/docgraph.go skip-list excludes .ddx/plugins/* (or narrower if justified). 2. After fix: ddx doc audit reports 0 duplicate_id issues. 3. Missing-dep resolved or explicitly justified. 4. New unit test in docgraph_test.go for the .ddx/plugins/* skip. 5. Audit before/after captured under .ddx/executions/&lt;run-id&gt;/. 6. FEAT-005 prose amendment: replace 'directory-agnostic' wording with 'content-based discovery within configured roots, subject to an exclusion list of tool-managed/storage dirs'.
    </acceptance>
    <labels>phase:2, story:4, area:docgraph, kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T124307-1e7a20e1/manifest.json</file>
    <file>.ddx/executions/20260502T124307-1e7a20e1/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d79fdad80dce64ca0c4995a3816c0b1509c60a76">
diff --git a/.ddx/executions/20260502T124307-1e7a20e1/manifest.json b/.ddx/executions/20260502T124307-1e7a20e1/manifest.json
new file mode 100644
index 00000000..c1958d5c
--- /dev/null
+++ b/.ddx/executions/20260502T124307-1e7a20e1/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T124307-1e7a20e1",
+  "bead_id": "ddx-2502ad71",
+  "base_rev": "2c2b6c60674bede7867f95e933b07008d5610438",
+  "created_at": "2026-05-02T12:43:10.043044552Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-2502ad71",
+    "title": "scanner: extend skip-list to .ddx/plugins/* + complete duplicate-ID cleanup",
+    "description": "Predecessor ddx-58764e1b closed but only partially fixed the issue. Scanner skip-list now excludes .agents/.claude (good), but .ddx/plugins/helix/docs/ still gets scanned and produces duplicates for SD-021..023, TD-004..010, TP-002..020, helix.workflow.principles, helix.prd, product-vision, etc. Current ddx doc audit still reports 86 duplicate_id issues. Fix: extend skip-list to also exclude .ddx/plugins/*/docs/ (or .ddx/plugins/* entirely if no other paths there matter). Also resolve missing-dep helix.workflow.principles → helix.workflow.",
+    "acceptance": "1. cli/internal/docgraph/docgraph.go skip-list excludes .ddx/plugins/* (or narrower if justified). 2. After fix: ddx doc audit reports 0 duplicate_id issues. 3. Missing-dep resolved or explicitly justified. 4. New unit test in docgraph_test.go for the .ddx/plugins/* skip. 5. Audit before/after captured under .ddx/executions/\u003crun-id\u003e/. 6. FEAT-005 prose amendment: replace 'directory-agnostic' wording with 'content-based discovery within configured roots, subject to an exclusion list of tool-managed/storage dirs'.",
+    "parent": "ddx-781af038",
+    "labels": [
+      "phase:2",
+      "story:4",
+      "area:docgraph",
+      "kind:fix"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T12:43:07Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1724970",
+      "execute-loop-heartbeat-at": "2026-05-02T12:43:07.400387442Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T124307-1e7a20e1",
+    "prompt": ".ddx/executions/20260502T124307-1e7a20e1/prompt.md",
+    "manifest": ".ddx/executions/20260502T124307-1e7a20e1/manifest.json",
+    "result": ".ddx/executions/20260502T124307-1e7a20e1/result.json",
+    "checks": ".ddx/executions/20260502T124307-1e7a20e1/checks.json",
+    "usage": ".ddx/executions/20260502T124307-1e7a20e1/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-2502ad71-20260502T124307-1e7a20e1"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T124307-1e7a20e1/result.json b/.ddx/executions/20260502T124307-1e7a20e1/result.json
new file mode 100644
index 00000000..c13353ea
--- /dev/null
+++ b/.ddx/executions/20260502T124307-1e7a20e1/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2502ad71",
+  "attempt_id": "20260502T124307-1e7a20e1",
+  "base_rev": "2c2b6c60674bede7867f95e933b07008d5610438",
+  "result_rev": "6bd04599c0805b68f1152bd8ad4f6a3012a8cee5",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-309244ee",
+  "duration_ms": 418283,
+  "tokens": 20009,
+  "cost_usd": 2.832008250000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T124307-1e7a20e1",
+  "prompt_file": ".ddx/executions/20260502T124307-1e7a20e1/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T124307-1e7a20e1/manifest.json",
+  "result_file": ".ddx/executions/20260502T124307-1e7a20e1/result.json",
+  "usage_file": ".ddx/executions/20260502T124307-1e7a20e1/usage.json",
+  "started_at": "2026-05-02T12:43:10.04328376Z",
+  "finished_at": "2026-05-02T12:50:08.326319053Z"
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
