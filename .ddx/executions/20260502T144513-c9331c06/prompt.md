<bead-review>
  <bead id="ddx-263ad347" iter=1>
    <title>operator-prompts: operatorPromptApprove/Cancel + per-project allowlist + auto-approve flag</title>
    <description>
operatorPromptApprove and operatorPromptCancel mutations. Per-project allowlist (which identities can auto-approve). Auto-approve flag opt-in only for configured-localhost identity (NEVER blanket ts-net per locked decision). Manual approve = proposed → ready.
    </description>
    <acceptance>
1. Approve/cancel mutations exist. 2. Per-project allowlist enforced. 3. Auto-approve restricted to configured-localhost. 4. Tests cover happy + denied paths.
    </acceptance>
    <labels>phase:2, story:15, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T143037-e45d8c1d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ab25674fdeda550c8a7e9302da09429571c45a73">
diff --git a/.ddx/executions/20260502T143037-e45d8c1d/result.json b/.ddx/executions/20260502T143037-e45d8c1d/result.json
new file mode 100644
index 00000000..d1ce08fe
--- /dev/null
+++ b/.ddx/executions/20260502T143037-e45d8c1d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-263ad347",
+  "attempt_id": "20260502T143037-e45d8c1d",
+  "base_rev": "32c79f439b0c7b7cb4b0d3c30f857b989c958c52",
+  "result_rev": "9b771a6c93370248358d629977b65a199870a082",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-b20158cc",
+  "duration_ms": 865008,
+  "tokens": 46508,
+  "cost_usd": 8.618962,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T143037-e45d8c1d",
+  "prompt_file": ".ddx/executions/20260502T143037-e45d8c1d/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T143037-e45d8c1d/manifest.json",
+  "result_file": ".ddx/executions/20260502T143037-e45d8c1d/result.json",
+  "usage_file": ".ddx/executions/20260502T143037-e45d8c1d/usage.json",
+  "started_at": "2026-05-02T14:30:38.599358335Z",
+  "finished_at": "2026-05-02T14:45:03.607401522Z"
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
