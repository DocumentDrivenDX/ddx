<bead-review>
  <bead id="ddx-2e54b560" iter=1>
    <title>ADR-022 step 3: operator-cancel via bead-store marker + mid-attempt poll</title>
    <description>
Server endpoint /api/beads/&lt;id&gt;/cancel writes extra.cancel-requested:true. Worker mid-attempt poll (10s default) re-reads bead extra; on cancel-requested:true, abort at next safe point (between LLM turns / git ops) with preserved_for_review reason 'operator_cancel'. Idempotent: second cancel writes cancel-honored:true silently. ~150 LOC.
    </description>
    <acceptance>
1. POST /api/beads/&lt;id&gt;/cancel writes extra.cancel-requested:true.
2. Worker mid-attempt loop polls bead extra every 10s.
3. On cancel-requested, worker emits 'preserved_for_review' with reason 'operator_cancel'; cancel-honored:true is set.
4. Idempotent.
5. Tests: TestOperatorCancel_SetsBeadExtra, TestWorker_HonorsCancelMidAttempt, TestCancelLatency_UnderSLA.
6. cd cli &amp;&amp; go test green; lefthook pre-commit passes.
7. WIRED-IN: integration test (TestOperatorCancel_DuringRealClaudeAttempt) issues cancel against a worker mid-flight on a real (or scripted-claude-stub-with-realistic-timing) attempt; asserts the actual subprocess receives the cancel signal and aborts at next safe point. Not just a stub-attempt cancel.
    </acceptance>
    <labels>phase:2, area:server, area:agent, kind:feature, adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T062235-a3b53a6c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4be73a4444a48b06831dc81daf7826496261b7db">
diff --git a/.ddx/executions/20260503T062235-a3b53a6c/result.json b/.ddx/executions/20260503T062235-a3b53a6c/result.json
new file mode 100644
index 00000000..a4b84d52
--- /dev/null
+++ b/.ddx/executions/20260503T062235-a3b53a6c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2e54b560",
+  "attempt_id": "20260503T062235-a3b53a6c",
+  "base_rev": "5b8a4dc453c42c698c7c299d9793e267910318a4",
+  "result_rev": "996e01f758141a7930faa338eb4b1387d59fe5ab",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a66f5b61",
+  "duration_ms": 1398044,
+  "tokens": 45362,
+  "cost_usd": 8.376722499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T062235-a3b53a6c",
+  "prompt_file": ".ddx/executions/20260503T062235-a3b53a6c/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T062235-a3b53a6c/manifest.json",
+  "result_file": ".ddx/executions/20260503T062235-a3b53a6c/result.json",
+  "usage_file": ".ddx/executions/20260503T062235-a3b53a6c/usage.json",
+  "started_at": "2026-05-03T06:22:37.175345986Z",
+  "finished_at": "2026-05-03T06:45:55.219689789Z"
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
