<bead-review>
  <bead id="ddx-f171a715" iter=1>
    <title>operator-prompts: operatorPromptSubmit mutation + CSRF + idempotency + identity-bound audit</title>
    <description>
GraphQL mutation operatorPromptSubmit. CSRF token required. Idempotency key required. Audit captures originating identity (localhost user OR ts-net WhoIs). Default status = proposed (not ready).
    </description>
    <acceptance>
1. operatorPromptSubmit mutation in schema. 2. CSRF required (returns 403 without). 3. Idempotency key dedupes within 24h window. 4. Audit event includes identity. 5. Default = proposed. 6. Tests cover all five.
    </acceptance>
    <labels>phase:2, story:15, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T141636-acf8793c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c599d2c5856f93e82626682e915dd4234bd28f24">
diff --git a/.ddx/executions/20260502T141636-acf8793c/result.json b/.ddx/executions/20260502T141636-acf8793c/result.json
new file mode 100644
index 00000000..ff757f12
--- /dev/null
+++ b/.ddx/executions/20260502T141636-acf8793c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-f171a715",
+  "attempt_id": "20260502T141636-acf8793c",
+  "base_rev": "4364a49615b808c773a23154ffbb0161897c3d7e",
+  "result_rev": "07b6c67e40d7935ee57533142a688d1598f99ef8",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-52635ab1",
+  "duration_ms": 605136,
+  "tokens": 30519,
+  "cost_usd": 4.0480735,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T141636-acf8793c",
+  "prompt_file": ".ddx/executions/20260502T141636-acf8793c/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T141636-acf8793c/manifest.json",
+  "result_file": ".ddx/executions/20260502T141636-acf8793c/result.json",
+  "usage_file": ".ddx/executions/20260502T141636-acf8793c/usage.json",
+  "started_at": "2026-05-02T14:16:38.033442276Z",
+  "finished_at": "2026-05-02T14:26:43.169996679Z"
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
