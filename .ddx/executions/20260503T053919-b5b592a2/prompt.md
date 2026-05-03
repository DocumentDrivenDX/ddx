<bead-review>
  <bead id="ddx-6aedf547" iter=1>
    <title>runs: runRequeue mutation with mandatory idempotencyKey + atomic claims</title>
    <description>
GraphQL mutation runRequeue (input: runId, idempotencyKey, optional layer override). Atomic: detects duplicate keys; refuses to enqueue duplicate. Audit-logged with operator identity.
    </description>
    <acceptance>
1. runRequeue mutation in schema. 2. idempotencyKey REQUIRED. 3. Duplicate-key returns existing requeued bead, not error. 4. Audit event on the originating bead. 5. Tests cover concurrent re-queue with same key.
    </acceptance>
    <labels>phase:2, story:8, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T052724-9252fd14/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="773dd516e8d17db2475ee09de15b64d5f27ebd64">
diff --git a/.ddx/executions/20260503T052724-9252fd14/result.json b/.ddx/executions/20260503T052724-9252fd14/result.json
new file mode 100644
index 00000000..765d953f
--- /dev/null
+++ b/.ddx/executions/20260503T052724-9252fd14/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-6aedf547",
+  "attempt_id": "20260503T052724-9252fd14",
+  "base_rev": "55972567fe3489d95eca6eb5600b0e539c5b6624",
+  "result_rev": "d9491735be26de5bddb13e50dd5b78a5d814b8a2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a5a5f9e5",
+  "duration_ms": 707909,
+  "tokens": 27098,
+  "cost_usd": 5.26351325,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T052724-9252fd14",
+  "prompt_file": ".ddx/executions/20260503T052724-9252fd14/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T052724-9252fd14/manifest.json",
+  "result_file": ".ddx/executions/20260503T052724-9252fd14/result.json",
+  "usage_file": ".ddx/executions/20260503T052724-9252fd14/usage.json",
+  "started_at": "2026-05-03T05:27:25.976459116Z",
+  "finished_at": "2026-05-03T05:39:13.886241579Z"
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
