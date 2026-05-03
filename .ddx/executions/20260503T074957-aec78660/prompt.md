<bead-review>
  <bead id=".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-7888e046" iter=1>
    <title>ADR-022 step 5c: TestWorkersPanel_LiveTransitions wired-in integration test</title>
    <description>
Wired-in integration test (from step 5 AC #8). Starts a real worker process; through the GraphQL workers query (step 5a) asserts the panel observes 'connected' state within 30s; advances time past 2× probe interval (60s) without events and asserts state transitions to 'stale'; advances past 10× probe interval (300s) and asserts 'disconnected'. Not fixture-based — exercises the live data flow with timing. Likely requires a clock-injection seam in worker_ingest.go's freshnessState (currently uses time.Now via parameter) so the test can advance synthetic time, and a way to spawn a worker that registers against the test's server. ~150 LOC + small clock-seam refactor in worker_ingest.go.
    </description>
    <acceptance>
1. Test TestWorkersPanel_LiveTransitions exists in cli/internal/server/ (or graphql package).
2. Test starts a real worker that registers with the test server via /api/workers/register.
3. Asserts GraphQL query reports state='connected' within 30s of worker start.
4. Without further events, after 2× probe interval (60s synthetic) asserts state='stale'.
5. After 10× probe interval (300s synthetic) asserts state='disconnected'.
6. Uses a clock-injection mechanism (not real wall-clock sleep) to keep test fast.
7. cd cli &amp;&amp; go test passes; lefthook pre-commit passes.
Depends on step 5a backend.
    </acceptance>
    <labels>phase:2,  area:test,  area:server,  kind:test,  adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T073940-14769e52/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8cafb68b0ecf4bc6a45e122a67116f1418e1c7eb">
diff --git a/.ddx/executions/20260503T073940-14769e52/result.json b/.ddx/executions/20260503T073940-14769e52/result.json
new file mode 100644
index 00000000..36014fd0
--- /dev/null
+++ b/.ddx/executions/20260503T073940-14769e52/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-7888e046",
+  "attempt_id": "20260503T073940-14769e52",
+  "base_rev": "c0ecbf06faadac1c9c8c08f83d9db2136bd00f2d",
+  "result_rev": "3a3e2970d9c756b5770d7485c67d0884801a1e5f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-7166da78",
+  "duration_ms": 610269,
+  "tokens": 19518,
+  "cost_usd": 2.3884494999999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T073940-14769e52",
+  "prompt_file": ".ddx/executions/20260503T073940-14769e52/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T073940-14769e52/manifest.json",
+  "result_file": ".ddx/executions/20260503T073940-14769e52/result.json",
+  "usage_file": ".ddx/executions/20260503T073940-14769e52/usage.json",
+  "started_at": "2026-05-03T07:39:41.586905684Z",
+  "finished_at": "2026-05-03T07:49:51.856649357Z"
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
