<bead-review>
  <bead id="ddx-4b202bbb" iter=1>
    <title>website-reorg epic: narrative restructure, features section, design tokens, Playwright coverage</title>
    <description/>
    <acceptance>
All 7 homepage sections live; /features/ section with maturity badges; DESIGN.md tokens wired to Hugo; Playwright e2e gaps closed; UI screenshots committed to website/static/ui/
    </acceptance>
    <notes>
REVIEW:BLOCK

Diff contains only an execution result.json metadata file. None of the acceptance criteria (homepage sections, /features/ section, DESIGN.md tokens, Playwright coverage, UI screenshots) are evidenced in the changed files.
    </notes>
    <labels>area:website, epic</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T035243-15df167e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="62f779d507648e187d0332133f1838e553fe9239">
diff --git a/.ddx/executions/20260501T035243-15df167e/result.json b/.ddx/executions/20260501T035243-15df167e/result.json
new file mode 100644
index 00000000..8e9776e9
--- /dev/null
+++ b/.ddx/executions/20260501T035243-15df167e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4b202bbb",
+  "attempt_id": "20260501T035243-15df167e",
+  "base_rev": "cdcabbf7e2511eee5cfcff0de2f78032c5b9cb25",
+  "result_rev": "42176ea01a1eec19fbd8a89f75e4108445111c72",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-60d63caf",
+  "duration_ms": 125620,
+  "tokens": 7523,
+  "cost_usd": 0.7832574999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T035243-15df167e",
+  "prompt_file": ".ddx/executions/20260501T035243-15df167e/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T035243-15df167e/manifest.json",
+  "result_file": ".ddx/executions/20260501T035243-15df167e/result.json",
+  "usage_file": ".ddx/executions/20260501T035243-15df167e/usage.json",
+  "started_at": "2026-05-01T03:52:45.782407539Z",
+  "finished_at": "2026-05-01T03:54:51.402832064Z"
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
