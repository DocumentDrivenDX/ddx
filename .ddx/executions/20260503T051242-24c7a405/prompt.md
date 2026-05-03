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
    <labels>phase:2, area:server, kind:feature, adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T050134-93403624/manifest.json</file>
    <file>.ddx/executions/20260503T050134-93403624/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="812bb7e2dad2b359c746c12fd260bd50bf6db3bf">
diff --git a/.ddx/executions/20260503T050134-93403624/manifest.json b/.ddx/executions/20260503T050134-93403624/manifest.json
new file mode 100644
index 00000000..051f5f34
--- /dev/null
+++ b/.ddx/executions/20260503T050134-93403624/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260503T050134-93403624",
+  "bead_id": "ddx-2892f2cf",
+  "base_rev": "8b1741efbff567a63341cc7d87f0cc76298ea584",
+  "created_at": "2026-05-03T05:01:35.682215372Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-2892f2cf",
+    "title": "ADR-022 step 1: server ingestion endpoints (register + event + backfill)",
+    "description": "Implement POST /api/workers/register, POST /api/workers/\u003cid\u003e/event, POST /api/workers/\u003cid\u003e/backfill per ADR-022 rev 5 §Worker-server interface. In-memory derived view backed by append-only .ddx/server/worker-events.jsonl. Freshness state machine (connected/stale/disconnected). requireTrusted boundary. ~300 LOC.",
+    "acceptance": "1. Three POST endpoints registered + handlers implemented. 2. JSONL append on each event. 3. In-memory registry tracks worker_id, last_event_at, mirror_failures_count, had_dropped_backfill. 4. Tests: TestWorkerRegister_HappyPath, TestWorkerEvent_AppendsToJSONL, TestWorkerBackfill_PostsBufferedEvents, TestWorkerEvent_410_TriggersReregister. 5. cd cli \u0026\u0026 go test ./internal/server/... green; lefthook pre-commit passes.",
+    "parent": "ddx-d7aca866",
+    "labels": [
+      "phase:2",
+      "area:server",
+      "kind:feature",
+      "adr:022"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T05:01:34Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T05:01:34.074959776Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T050134-93403624",
+    "prompt": ".ddx/executions/20260503T050134-93403624/prompt.md",
+    "manifest": ".ddx/executions/20260503T050134-93403624/manifest.json",
+    "result": ".ddx/executions/20260503T050134-93403624/result.json",
+    "checks": ".ddx/executions/20260503T050134-93403624/checks.json",
+    "usage": ".ddx/executions/20260503T050134-93403624/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-2892f2cf-20260503T050134-93403624"
+  },
+  "prompt_sha": "17ba6b6b2a502f032328c355c282b9be0793441ef37d1bb22202947dfaff843b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T050134-93403624/result.json b/.ddx/executions/20260503T050134-93403624/result.json
new file mode 100644
index 00000000..280b537e
--- /dev/null
+++ b/.ddx/executions/20260503T050134-93403624/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2892f2cf",
+  "attempt_id": "20260503T050134-93403624",
+  "base_rev": "8b1741efbff567a63341cc7d87f0cc76298ea584",
+  "result_rev": "476d10fe187980a5b8e62cf7ffdb889a6a1f2f23",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3810f814",
+  "duration_ms": 662596,
+  "tokens": 18406,
+  "cost_usd": 2.6440842499999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T050134-93403624",
+  "prompt_file": ".ddx/executions/20260503T050134-93403624/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T050134-93403624/manifest.json",
+  "result_file": ".ddx/executions/20260503T050134-93403624/result.json",
+  "usage_file": ".ddx/executions/20260503T050134-93403624/usage.json",
+  "started_at": "2026-05-03T05:01:35.682547538Z",
+  "finished_at": "2026-05-03T05:12:38.279325352Z"
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
