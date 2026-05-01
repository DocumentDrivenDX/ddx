<bead-review>
  <bead id="ddx-4b202bbb" iter=1>
    <title>website-reorg epic: narrative restructure, features section, design tokens, Playwright coverage</title>
    <description/>
    <acceptance>
All 7 homepage sections live; /features/ section with maturity badges; DESIGN.md tokens wired to Hugo; Playwright e2e gaps closed; UI screenshots committed to website/static/ui/
    </acceptance>
    <labels>area:website, epic</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T033606-75bc07b7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="031d8ae50c2e284f809f65519d22527c0838ba64">
diff --git a/.ddx/executions/20260501T033606-75bc07b7/result.json b/.ddx/executions/20260501T033606-75bc07b7/result.json
new file mode 100644
index 00000000..6ebd169a
--- /dev/null
+++ b/.ddx/executions/20260501T033606-75bc07b7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4b202bbb",
+  "attempt_id": "20260501T033606-75bc07b7",
+  "base_rev": "564d9414ef19621443fdc2ed85e89d2f96165d32",
+  "result_rev": "cb50ecb3216bd292baa001d0d2e8ca155d4ecdc8",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-ab6a4386",
+  "duration_ms": 936990,
+  "tokens": 31575,
+  "cost_usd": 1.8021916999999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T033606-75bc07b7",
+  "prompt_file": ".ddx/executions/20260501T033606-75bc07b7/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T033606-75bc07b7/manifest.json",
+  "result_file": ".ddx/executions/20260501T033606-75bc07b7/result.json",
+  "usage_file": ".ddx/executions/20260501T033606-75bc07b7/usage.json",
+  "started_at": "2026-05-01T03:36:07.515646133Z",
+  "finished_at": "2026-05-01T03:51:44.506071693Z"
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
