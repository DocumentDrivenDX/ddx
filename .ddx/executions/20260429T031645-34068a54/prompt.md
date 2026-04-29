<bead-review>
  <bead id="ddx-b3b9501a" iter=1>
    <title>BUG: latency_ms field in agent-claude-*.jsonl traces equals cumulative elapsed_ms</title>
    <description>
## Symptom

Every `llm.response` event in `.ddx/agent-logs/agent-claude-*.jsonl` has `latency_ms == elapsed_ms`. Examples from agent-claude-1776370404950042448.jsonl:

```jsonl
{"data":{"elapsed_ms":7373, "latency_ms":7373, "turn":1, ...}}
{"data":{"elapsed_ms":7995, "latency_ms":7995, "turn":2, ...}}
{"data":{"elapsed_ms":12054,"latency_ms":12054,"turn":3, ...}}
```

`elapsed_ms` is monotonic across the session (cumulative wall time), so per-call LLM latency is unrecoverable from the log.

## Impact

- Cannot answer "what was the LLM-side latency for turn N?" — required for any performance investigation.
- Cannot diff the agent's local-LLM benchmark numbers against captured traces.
- The lucebox-hub/dflash bench_server.py harness needs real per-call latency to validate against reality.

## Likely root cause

Logging code in `cli/internal/agent/session_log_format.go` (or equivalent emit site) is writing the wrong value into `latency_ms`. Probably copy-paste from the elapsed-time field. Per-call latency should be `t_response_received - t_request_sent` for that single call.

## Fix sketch

Capture the per-call latency inside the harness adapter (point where the LLM HTTP call is made) and pass it through to the event emitter. Add a unit test asserting `latency_ms &lt; elapsed_ms` for any non-first event.
    </description>
    <acceptance>
1. New traces from a `ddx agent` invocation show `latency_ms != elapsed_ms` for events past turn 1.
2. Sum of `latency_ms` across an entire session is &lt;= `elapsed_ms` of the last event.
3. Existing trace files: documented as legacy in CHANGELOG; analysis tools may need to ignore `latency_ms` for runs older than the fix commit.
4. Unit test added that simulates two consecutive llm.response events and asserts latency_ms reflects per-call duration, not cumulative.
    </acceptance>
    <labels>kind:bug, area:agent, area:measurement</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T031155-01d2fc99/manifest.json</file>
    <file>.ddx/executions/20260429T031155-01d2fc99/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2dcecec07fca649ffbcaa189350e0530ad00f540">
diff --git a/.ddx/executions/20260429T031155-01d2fc99/manifest.json b/.ddx/executions/20260429T031155-01d2fc99/manifest.json
new file mode 100644
index 00000000..44deb713
--- /dev/null
+++ b/.ddx/executions/20260429T031155-01d2fc99/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T031155-01d2fc99",
+  "bead_id": "ddx-b3b9501a",
+  "base_rev": "eeb62dc4abf960ec5d78e5ef4175ce5af58a004d",
+  "created_at": "2026-04-29T03:11:55.878593731Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b3b9501a",
+    "title": "BUG: latency_ms field in agent-claude-*.jsonl traces equals cumulative elapsed_ms",
+    "description": "## Symptom\n\nEvery `llm.response` event in `.ddx/agent-logs/agent-claude-*.jsonl` has `latency_ms == elapsed_ms`. Examples from agent-claude-1776370404950042448.jsonl:\n\n```jsonl\n{\"data\":{\"elapsed_ms\":7373, \"latency_ms\":7373, \"turn\":1, ...}}\n{\"data\":{\"elapsed_ms\":7995, \"latency_ms\":7995, \"turn\":2, ...}}\n{\"data\":{\"elapsed_ms\":12054,\"latency_ms\":12054,\"turn\":3, ...}}\n```\n\n`elapsed_ms` is monotonic across the session (cumulative wall time), so per-call LLM latency is unrecoverable from the log.\n\n## Impact\n\n- Cannot answer \"what was the LLM-side latency for turn N?\" — required for any performance investigation.\n- Cannot diff the agent's local-LLM benchmark numbers against captured traces.\n- The lucebox-hub/dflash bench_server.py harness needs real per-call latency to validate against reality.\n\n## Likely root cause\n\nLogging code in `cli/internal/agent/session_log_format.go` (or equivalent emit site) is writing the wrong value into `latency_ms`. Probably copy-paste from the elapsed-time field. Per-call latency should be `t_response_received - t_request_sent` for that single call.\n\n## Fix sketch\n\nCapture the per-call latency inside the harness adapter (point where the LLM HTTP call is made) and pass it through to the event emitter. Add a unit test asserting `latency_ms \u003c elapsed_ms` for any non-first event.",
+    "acceptance": "1. New traces from a `ddx agent` invocation show `latency_ms != elapsed_ms` for events past turn 1.\n2. Sum of `latency_ms` across an entire session is \u003c= `elapsed_ms` of the last event.\n3. Existing trace files: documented as legacy in CHANGELOG; analysis tools may need to ignore `latency_ms` for runs older than the fix commit.\n4. Unit test added that simulates two consecutive llm.response events and asserts latency_ms reflects per-call duration, not cumulative.",
+    "labels": [
+      "kind:bug",
+      "area:agent",
+      "area:measurement"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T03:11:55Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T03:11:55.224688122Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T031155-01d2fc99",
+    "prompt": ".ddx/executions/20260429T031155-01d2fc99/prompt.md",
+    "manifest": ".ddx/executions/20260429T031155-01d2fc99/manifest.json",
+    "result": ".ddx/executions/20260429T031155-01d2fc99/result.json",
+    "checks": ".ddx/executions/20260429T031155-01d2fc99/checks.json",
+    "usage": ".ddx/executions/20260429T031155-01d2fc99/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b3b9501a-20260429T031155-01d2fc99"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T031155-01d2fc99/result.json b/.ddx/executions/20260429T031155-01d2fc99/result.json
new file mode 100644
index 00000000..d1542277
--- /dev/null
+++ b/.ddx/executions/20260429T031155-01d2fc99/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b3b9501a",
+  "attempt_id": "20260429T031155-01d2fc99",
+  "base_rev": "eeb62dc4abf960ec5d78e5ef4175ce5af58a004d",
+  "result_rev": "7ad7e2ea337ad7ae033ff7f450512fe213de6c52",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2ff8d561",
+  "duration_ms": 286577,
+  "tokens": 9983,
+  "cost_usd": 1.12842475,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T031155-01d2fc99",
+  "prompt_file": ".ddx/executions/20260429T031155-01d2fc99/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T031155-01d2fc99/manifest.json",
+  "result_file": ".ddx/executions/20260429T031155-01d2fc99/result.json",
+  "usage_file": ".ddx/executions/20260429T031155-01d2fc99/usage.json",
+  "started_at": "2026-04-29T03:11:55.878834356Z",
+  "finished_at": "2026-04-29T03:16:42.456547394Z"
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
