<bead-review>
  <bead id="ddx-2436fa73" iter=1>
    <title>review: BuildReviewExecuteRequest; remove implHarness default; emit review-pairing-degraded</title>
    <description>
Today reviewer defaults to implementer harness (cli/internal/agent/execute_bead_review.go:521) — directly violating R4 (different reviewer). Build the reviewer ExecuteRequest with: Role='reviewer'; SAME CorrelationID as the implementer call (joins them in events / session log / aggregations); MinPower = implementer.RoutingActual.Power + 1.

'Different provider' Day 1 is best-effort emergent — the MinPower bump usually forces Fizeau to pick a different candidate, but not guaranteed. When same-provider results, log a 'review-pairing-degraded' event with implementer + reviewer routing details so operators can see degradation.
    </description>
    <acceptance>
1. execute_bead_review.go no longer defaults to implementer harness. 2. Reviewer call carries Role='reviewer' + same CorrelationID as implementer + MinPower=actualPower+1. 3. review-pairing-degraded event emitted when reviewer's RoutingActual.Provider == implementer's. 4. Tests cover: happy path (different provider), degraded path (same provider), and event payload includes both routing details.
    </acceptance>
    <labels>phase:2, story:10, area:routing, kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T130148-99e92001/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="6dc6d11357bd7916cd466387f3831621d0cd6f76">
diff --git a/.ddx/executions/20260502T130148-99e92001/result.json b/.ddx/executions/20260502T130148-99e92001/result.json
new file mode 100644
index 00000000..fca1c0d5
--- /dev/null
+++ b/.ddx/executions/20260502T130148-99e92001/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2436fa73",
+  "attempt_id": "20260502T130148-99e92001",
+  "base_rev": "1b1c578b27c46ee2a6f3b90bfce799e221b3b69e",
+  "result_rev": "0dd8a81710495742decf1ed52dfd77e522011dac",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-7bd03530",
+  "duration_ms": 981011,
+  "tokens": 47391,
+  "cost_usd": 9.8679225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T130148-99e92001",
+  "prompt_file": ".ddx/executions/20260502T130148-99e92001/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T130148-99e92001/manifest.json",
+  "result_file": ".ddx/executions/20260502T130148-99e92001/result.json",
+  "usage_file": ".ddx/executions/20260502T130148-99e92001/usage.json",
+  "started_at": "2026-05-02T13:01:49.822462437Z",
+  "finished_at": "2026-05-02T13:18:10.834453664Z"
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
