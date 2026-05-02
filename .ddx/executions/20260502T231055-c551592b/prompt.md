<bead-review>
  <bead id="ddx-f3c7149d" iter=1>
    <title>S15-4: operatorPromptApprove/cancel + per-project allowlist + auto-approve</title>
    <description>
Add operatorPromptApprove and operatorPromptCancel mutations transitioning proposed→ready or proposed→cancelled. Add config web.operator_prompt.auto_approve (default ON for localhost, OFF for ts-net). Add web.operator_prompt.allow_identities per-project allowlist keyed on ts-net identity. Default empty allowlist → ts-net peers see read-only UI. See /tmp/story-15-final.md §Implementation #5 and §Additional security controls bullet 3.
    </description>
    <acceptance>
Approve mutation transitions proposed→ready (trusted only); Cancel transitions to cancelled; auto-approve config skips proposed for configured localhost identity only and never for ts-net by default; allowlist enforced per-project — non-listed ts-net peers get 403 on submit; tests cover approve/cancel/denied paths and config matrix.
    </acceptance>
    <labels>phase:2, story:15, kind:graphql</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T230542-d74b3541/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="34330480d01a2795f390d58fc8e4d1cc4500f2ab">
diff --git a/.ddx/executions/20260502T230542-d74b3541/result.json b/.ddx/executions/20260502T230542-d74b3541/result.json
new file mode 100644
index 00000000..457ba705
--- /dev/null
+++ b/.ddx/executions/20260502T230542-d74b3541/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-f3c7149d",
+  "attempt_id": "20260502T230542-d74b3541",
+  "base_rev": "b037dd053fb2097a6231576d2e176ea0f23e4b8b",
+  "result_rev": "7c1ff6a5fed5f09dd1bf2f8fdf06cbb83137ec57",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-00407c36",
+  "duration_ms": 305600,
+  "tokens": 12903,
+  "cost_usd": 1.9377262500000008,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T230542-d74b3541",
+  "prompt_file": ".ddx/executions/20260502T230542-d74b3541/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T230542-d74b3541/manifest.json",
+  "result_file": ".ddx/executions/20260502T230542-d74b3541/result.json",
+  "usage_file": ".ddx/executions/20260502T230542-d74b3541/usage.json",
+  "started_at": "2026-05-02T23:05:43.512910766Z",
+  "finished_at": "2026-05-02T23:10:49.113574387Z"
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
