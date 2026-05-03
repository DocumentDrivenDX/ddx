<bead-review>
  <bead id="ddx-2892f2cf" iter=1>
    <title>ADR-022 step 1: server ingestion endpoints (register + event + backfill)</title>
    <description>
Implement POST /api/workers/register, POST /api/workers/&lt;id&gt;/event, POST /api/workers/&lt;id&gt;/backfill per ADR-022 rev 5 §Worker-server interface. In-memory derived view backed by append-only .ddx/server/worker-events.jsonl. Freshness state machine (connected/stale/disconnected). requireTrusted boundary. ~300 LOC.
    </description>
    <acceptance>
1. Three POST endpoints registered + handlers implemented.
2. JSONL append on each event.
3. In-memory registry tracks worker_id, last_event_at, mirror_failures_count, had_dropped_backfill.
4. Tests: TestWorkerRegister_HappyPath, TestWorkerEvent_AppendsToJSONL, TestWorkerBackfill_PostsBufferedEvents, TestWorkerEvent_410_TriggersReregister.
5. cd cli &amp;&amp; go test ./internal/server/... green; lefthook pre-commit passes.
6. WIRED-IN: integration test in this bead's test file (TestServerIngestion_RealWorkerCanRegister) spawns a real subprocess that POSTs to /api/workers/register and asserts the in-memory registry shows the worker. Not a unit test of the handler — actually exercises the production HTTP path with a real client.
    </acceptance>
    <notes>
REVIEW:BLOCK

AC #6 not satisfied: required wired-in integration test TestServerIngestion_RealWorkerCanRegister that spawns a real subprocess POSTing to /api/workers/register is missing. Other ACs (handlers, JSONL append, registry fields, four named unit tests) are implemented.
    </notes>
    <labels>phase:2, area:server, kind:feature, adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T052048-9cafd33a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b73c669ed970c72017f8e461ad9f01f83a890f1d">
diff --git a/.ddx/executions/20260503T052048-9cafd33a/result.json b/.ddx/executions/20260503T052048-9cafd33a/result.json
new file mode 100644
index 00000000..1355cca1
--- /dev/null
+++ b/.ddx/executions/20260503T052048-9cafd33a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2892f2cf",
+  "attempt_id": "20260503T052048-9cafd33a",
+  "base_rev": "14946b1bba139d7236039173e46f90eb1f070d56",
+  "result_rev": "23fdaefb0070585af721a6883992ab0ddb8716e2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-94e9ccae",
+  "duration_ms": 475044,
+  "tokens": 10886,
+  "cost_usd": 1.35938575,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T052048-9cafd33a",
+  "prompt_file": ".ddx/executions/20260503T052048-9cafd33a/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T052048-9cafd33a/manifest.json",
+  "result_file": ".ddx/executions/20260503T052048-9cafd33a/result.json",
+  "usage_file": ".ddx/executions/20260503T052048-9cafd33a/usage.json",
+  "started_at": "2026-05-03T05:20:50.415801161Z",
+  "finished_at": "2026-05-03T05:28:45.460784914Z"
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
