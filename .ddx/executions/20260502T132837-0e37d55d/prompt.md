<bead-review>
  <bead id="ddx-4caba860" iter=1>
    <title>workers: project-scope audit + e2e</title>
    <description>
Audit Workers tab to confirm it scopes to currently selected repo only. Add e2e covering the scope filter.
    </description>
    <acceptance>
1. Workers list shows only currently-selected-project workers. 2. e2e covers a multi-project setup and asserts scoping.
    </acceptance>
    <labels>phase:2, story:8, area:web, area:tests, kind:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T132613-360df354/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f2b2f88ace2af30ae357f6e2806e57b6f84ed816">
diff --git a/.ddx/executions/20260502T132613-360df354/result.json b/.ddx/executions/20260502T132613-360df354/result.json
new file mode 100644
index 00000000..590345be
--- /dev/null
+++ b/.ddx/executions/20260502T132613-360df354/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4caba860",
+  "attempt_id": "20260502T132613-360df354",
+  "base_rev": "c72dd00d84153a2b5d8a3e1c932832fd7adfc80a",
+  "result_rev": "26a8c8b6e7c3abe9b5473a1bb4f50ba01a22338e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2a10a1b9",
+  "duration_ms": 136228,
+  "tokens": 8458,
+  "cost_usd": 1.3199210000000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T132613-360df354",
+  "prompt_file": ".ddx/executions/20260502T132613-360df354/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T132613-360df354/manifest.json",
+  "result_file": ".ddx/executions/20260502T132613-360df354/result.json",
+  "usage_file": ".ddx/executions/20260502T132613-360df354/usage.json",
+  "started_at": "2026-05-02T13:26:14.835620651Z",
+  "finished_at": "2026-05-02T13:28:31.063641947Z"
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
