<bead-review>
  <bead id="ddx-d1c4e27a" iter=1>
    <title>e2e: sessions.spec.ts — add per-run evidence artifact detail test</title>
    <description/>
    <acceptance>
sessions.spec.ts has test that: navigates to a run/session, verifies evidence artifact detail view is accessible; test passes in CI
    </acceptance>
    <labels>area:e2e, playwright, sessions</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T035638-4d58aec7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d1b1144b20b96cadefc4b43673e5b1dca6b8c724">
diff --git a/.ddx/executions/20260501T035638-4d58aec7/result.json b/.ddx/executions/20260501T035638-4d58aec7/result.json
new file mode 100644
index 00000000..853eba8b
--- /dev/null
+++ b/.ddx/executions/20260501T035638-4d58aec7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d1c4e27a",
+  "attempt_id": "20260501T035638-4d58aec7",
+  "base_rev": "3894a324c4243f202f4aeeffd35fca0763dfe3e1",
+  "result_rev": "dcb18d44585141117f1445f0b675c5dd2adb1755",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c6d805c3",
+  "duration_ms": 175020,
+  "tokens": 9275,
+  "cost_usd": 1.72178325,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T035638-4d58aec7",
+  "prompt_file": ".ddx/executions/20260501T035638-4d58aec7/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T035638-4d58aec7/manifest.json",
+  "result_file": ".ddx/executions/20260501T035638-4d58aec7/result.json",
+  "usage_file": ".ddx/executions/20260501T035638-4d58aec7/usage.json",
+  "started_at": "2026-05-01T03:56:39.569876155Z",
+  "finished_at": "2026-05-01T03:59:34.590455182Z"
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
