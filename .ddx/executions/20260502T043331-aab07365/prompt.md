<bead-review>
  <bead id="ddx-2433c5fc" iter=1>
    <title>Verify bd/br interchange tests still pass with new collection + attachment model</title>
    <description>
ADR-004's compatibility guard #3 says schema_compat_test.go must keep locking the field names and round-trip behavior against real bd export examples. After C2/C3/C4 changes, ensure exports from beads-archive collection AND attachment-linked events still produce valid bd-format JSONL when streamed via ddx bead export.
    </description>
    <acceptance>
1. cli/internal/bead/schema_compat_test.go passes. 2. ddx bead export streams events for archived beads inline (interchange format). 3. Round-trip test: ddx bead export | &lt;bd consumer simulation&gt; succeeds. 4. New test added covering archive+attachment export path.
    </acceptance>
    <labels>area:beads, area:tests, kind:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T043019-7c24f8ca/manifest.json</file>
    <file>.ddx/executions/20260502T043019-7c24f8ca/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0476ebf8fbc28ede1d7e9c7b505d1e6515ca0321">
diff --git a/.ddx/executions/20260502T043019-7c24f8ca/manifest.json b/.ddx/executions/20260502T043019-7c24f8ca/manifest.json
new file mode 100644
index 00000000..eb752927
--- /dev/null
+++ b/.ddx/executions/20260502T043019-7c24f8ca/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T043019-7c24f8ca",
+  "bead_id": "ddx-2433c5fc",
+  "base_rev": "2610d6706ed684bd9fddf4536ad81f5753906273",
+  "created_at": "2026-05-02T04:30:21.102163963Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-2433c5fc",
+    "title": "Verify bd/br interchange tests still pass with new collection + attachment model",
+    "description": "ADR-004's compatibility guard #3 says schema_compat_test.go must keep locking the field names and round-trip behavior against real bd export examples. After C2/C3/C4 changes, ensure exports from beads-archive collection AND attachment-linked events still produce valid bd-format JSONL when streamed via ddx bead export.",
+    "acceptance": "1. cli/internal/bead/schema_compat_test.go passes. 2. ddx bead export streams events for archived beads inline (interchange format). 3. Round-trip test: ddx bead export | \u003cbd consumer simulation\u003e succeeds. 4. New test added covering archive+attachment export path.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:tests",
+      "kind:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T04:30:19Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T04:30:19.684011589Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T043019-7c24f8ca",
+    "prompt": ".ddx/executions/20260502T043019-7c24f8ca/prompt.md",
+    "manifest": ".ddx/executions/20260502T043019-7c24f8ca/manifest.json",
+    "result": ".ddx/executions/20260502T043019-7c24f8ca/result.json",
+    "checks": ".ddx/executions/20260502T043019-7c24f8ca/checks.json",
+    "usage": ".ddx/executions/20260502T043019-7c24f8ca/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-2433c5fc-20260502T043019-7c24f8ca"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T043019-7c24f8ca/result.json b/.ddx/executions/20260502T043019-7c24f8ca/result.json
new file mode 100644
index 00000000..4e510e3b
--- /dev/null
+++ b/.ddx/executions/20260502T043019-7c24f8ca/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2433c5fc",
+  "attempt_id": "20260502T043019-7c24f8ca",
+  "base_rev": "2610d6706ed684bd9fddf4536ad81f5753906273",
+  "result_rev": "6cd489b59b960f01ea07a567cbdb41e6d8f49d01",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-22ab68ba",
+  "duration_ms": 185445,
+  "tokens": 9233,
+  "cost_usd": 1.4524702500000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T043019-7c24f8ca",
+  "prompt_file": ".ddx/executions/20260502T043019-7c24f8ca/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T043019-7c24f8ca/manifest.json",
+  "result_file": ".ddx/executions/20260502T043019-7c24f8ca/result.json",
+  "usage_file": ".ddx/executions/20260502T043019-7c24f8ca/usage.json",
+  "started_at": "2026-05-02T04:30:21.102456838Z",
+  "finished_at": "2026-05-02T04:33:26.547920896Z"
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
