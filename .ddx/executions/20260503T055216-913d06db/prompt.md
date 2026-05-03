<bead-review>
  <bead id="ddx-98257c6a" iter=1>
    <title>ADR-022 step 2: worker server-probe goroutine + best-effort event mirror</title>
    <description>
Add server-probe goroutine to worker; periodic reachability check on ~/.local/share/ddx/server.addr with jittered 30s interval, immediate first probe, exponential backoff on consecutive failures (5min cap); state transitions emit register/disconnect; in-memory ring buffer (200 events) for backfill on Connected transition; event POSTs are best-effort. ~250 LOC.
    </description>
    <acceptance>
1. Probe goroutine; cadence + jitter + backoff per ADR §Probe + freshness state model.
2. Register on NotConnected→Connected; clear worker_id on Connected→NotConnected.
3. Ring buffer for backfill (200 cap; oldest dropped silently; had_dropped_backfill flag set if overflow).
4. Best-effort event mirror; failures logged + continue.
5. Tests: TestWorker_ProbeImmediateFirst, TestWorker_ProbeJitter, TestWorker_BackfillOnReconnect, TestWorker_BackoffOnFailures, TestWorker_NoServer_StillWorks.
6. cd cli &amp;&amp; go test ./internal/agent/... green; lefthook pre-commit passes.
7. WIRED-IN: integration test (TestWorker_RealAttemptEvents_FlowToServer) starts a server (from step 1), starts a real ddx work worker process against a fixture bead, observes that the worker's bead-attempt events appear in the server's derived view within 5s. Asserts the entire worker-server flow end-to-end, not just the probe goroutine in isolation.
    </acceptance>
    <labels>phase:2, area:agent, kind:feature, adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T053005-4c51820b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0fc6e6e55c7cac6bdc2df4163f03a68dc5f688dc">
diff --git a/.ddx/executions/20260503T053005-4c51820b/result.json b/.ddx/executions/20260503T053005-4c51820b/result.json
new file mode 100644
index 00000000..66f4e04c
--- /dev/null
+++ b/.ddx/executions/20260503T053005-4c51820b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-98257c6a",
+  "attempt_id": "20260503T053005-4c51820b",
+  "base_rev": "ae76629661e736309401ef2439e01309def646a0",
+  "result_rev": "07c295149d0450b2b1ee71dca72436d75a7d5622",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-e9d3be34",
+  "duration_ms": 1324887,
+  "tokens": 41869,
+  "cost_usd": 6.901916000000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T053005-4c51820b",
+  "prompt_file": ".ddx/executions/20260503T053005-4c51820b/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T053005-4c51820b/manifest.json",
+  "result_file": ".ddx/executions/20260503T053005-4c51820b/result.json",
+  "usage_file": ".ddx/executions/20260503T053005-4c51820b/usage.json",
+  "started_at": "2026-05-03T05:30:06.69008088Z",
+  "finished_at": "2026-05-03T05:52:11.57768172Z"
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
