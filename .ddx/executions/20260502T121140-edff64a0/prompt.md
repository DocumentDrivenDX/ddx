<bead-review>
  <bead id="ddx-4d7e1d93" iter=1>
    <title>B1: Extract shared execute-bead instruction blocks (byte-equivalent refactor)</title>
    <description>
Pull duplicated text from executeBeadInstructionsClaudeText (execute_bead.go:1208) and executeBeadInstructionsAgentText (execute_bead.go:1274) into named constants: instrStep0SizeCheck, instrInvestigationReports, instrReviewGate, instrBeadOverride, instrCoreConstraints. Both variants compose: per-harness preamble + per-harness process body + shared blocks. Rendered prompt MUST be byte-identical (or whitespace-only-equivalent) to current output. Plan: /tmp/story-12-final.md §B1. Hard dep on Story 10 prompt edits if running concurrently.
    </description>
    <acceptance>
AC1: Both variants share &gt;=4 named instruction-block constants; no shared block text duplicated across variants in source. AC6: All existing prompt tests pass unchanged (execute_bead_evidence_path_test.go, review_verdict_test.go, execute_bead_failure_mode_test.go). Rendered prompt is byte-identical to pre-refactor output (or only whitespace differences) — verify with golden fixture or diff.
    </acceptance>
    <labels>phase:2, story:12, tier:cheap</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T115600-64bd7a2c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="516b3e95e8cdd36afca1643cd686c8bccbd3cf9b">
diff --git a/.ddx/executions/20260502T115600-64bd7a2c/result.json b/.ddx/executions/20260502T115600-64bd7a2c/result.json
new file mode 100644
index 00000000..95911db6
--- /dev/null
+++ b/.ddx/executions/20260502T115600-64bd7a2c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4d7e1d93",
+  "attempt_id": "20260502T115600-64bd7a2c",
+  "base_rev": "c376d16b0ee65d8aa6586590666361476f79a66b",
+  "result_rev": "1f4ba5aa0d674098b04855692c3dbd38858fba4f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-4df14844",
+  "duration_ms": 934327,
+  "tokens": 42051,
+  "cost_usd": 3.52522475,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T115600-64bd7a2c",
+  "prompt_file": ".ddx/executions/20260502T115600-64bd7a2c/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T115600-64bd7a2c/manifest.json",
+  "result_file": ".ddx/executions/20260502T115600-64bd7a2c/result.json",
+  "usage_file": ".ddx/executions/20260502T115600-64bd7a2c/usage.json",
+  "started_at": "2026-05-02T11:56:01.829974221Z",
+  "finished_at": "2026-05-02T12:11:36.157704806Z"
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
