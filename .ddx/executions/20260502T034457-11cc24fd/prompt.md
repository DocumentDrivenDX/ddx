<bead-review>
  <bead id="ddx-3d2ed549" iter=1>
    <title>Verify execute-bead /tmp evidence pattern fix end-to-end</title>
    <description>
Commit 6473f3fa updated cli/internal/agent/execute_bead.go to instruct agents to write investigation reports under .ddx/executions/&lt;run-id&gt;/ instead of /tmp. The fix landed too late to help B15a/B21 in the redesign drain. Now that v0.6.2-alpha4 is installed with the fix, verify the system prompt actually steers agents correctly. Strategy: file a tiny test bead with an 'output a report' AC, observe whether the agent writes to .ddx/executions/ or /tmp.
    </description>
    <acceptance>
1. A test bead is filed with AC 'write a one-line report named hello.md describing the current Go version'. 2. ddx work --once executes the bead with claude harness. 3. The resulting commit includes a hello.md file under .ddx/executions/&lt;run-id&gt;/, NOT in /tmp. 4. If verification fails, the fix is reopened as a follow-up bead with a stricter prompt.
    </acceptance>
    <labels>area:agent, kind:test, validation</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T034139-ff5dccc5/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="178449742852a45b04e4e5c5979319d44ec7e173">
diff --git a/.ddx/executions/20260502T034139-ff5dccc5/result.json b/.ddx/executions/20260502T034139-ff5dccc5/result.json
new file mode 100644
index 00000000..0852c204
--- /dev/null
+++ b/.ddx/executions/20260502T034139-ff5dccc5/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-3d2ed549",
+  "attempt_id": "20260502T034139-ff5dccc5",
+  "base_rev": "d83526a6c2eba6e120863b6e131a6fa34f4ae82e",
+  "result_rev": "35a22c2e3df8f10fc70bd907537ac5dd87ba64a4",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3264b7e8",
+  "duration_ms": 191029,
+  "tokens": 10848,
+  "cost_usd": 1.10019225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T034139-ff5dccc5",
+  "prompt_file": ".ddx/executions/20260502T034139-ff5dccc5/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T034139-ff5dccc5/manifest.json",
+  "result_file": ".ddx/executions/20260502T034139-ff5dccc5/result.json",
+  "usage_file": ".ddx/executions/20260502T034139-ff5dccc5/usage.json",
+  "started_at": "2026-05-02T03:41:40.607563396Z",
+  "finished_at": "2026-05-02T03:44:51.636888807Z"
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
