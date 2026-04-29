<bead-review>
  <bead id="ddx-969c5500" iter=1>
    <title>FEAT: agent-logs jsonl schema is metadata-only; add verbose mode capturing tool I/O and content</title>
    <description>
## Problem

`.ddx/agent-logs/agent-claude-*.jsonl` (lean schema) records only:
  - `session.start` (model, harness, bead_id)
  - `llm.response` (turn, input_tokens, output_tokens, elapsed_ms)
  - `tool.call`     (turn, tool name)

Notably absent: model output content, tool call arguments, tool call results, finish_reason, full usage breakdown (cache reads, cache writes), error/stop reasons.

Meanwhile, `agent/demos/sessions/*.jsonl` from the sibling `Projects/agent` repo demonstrates a richer schema for the same event types:

```jsonl
{"type":"llm.response","data":{"content":"...","finish_reason":"tool_calls","usage":{"input":899,"output":21,"total":920},"latency_ms":3040,"tool_calls":1}}
{"type":"tool.call","data":{"tool":"edit","input":{"path":"...","old_string":"...","new_string":"..."},"output":"Replaced 1 occurrence","duration_ms":0,"error":""}}
```

## Why it matters

- Cannot replay a real session to reproduce a bug — the inputs are gone.
- Cannot root-cause provider failures (e.g. RotatingKVCache Quantization NYI seen in sessions.jsonl) because the prompt that triggered it is not in the per-event log; only the whole-session metadata in sessions.jsonl has it.
- The lucebox-hub/dflash `bench_server.py` replay-mode benchmark needs full prompt text from real traces, not just metadata, to be representative.
- Inability to tell what tool args caused a slow call.

## Proposed fix

Add a verbose mode to the agent-logs writer (env var `DDX_AGENT_LOG_VERBOSE=1` or config flag) that adopts the agent/demos schema:
  - llm.response: include `content`, `finish_reason`, full `usage` object
  - tool.call: include `input` (tool args), `output` (truncated to N KB), `duration_ms`, `error`
  - new event: `llm.request` with messages_count and possibly truncated message preview

Verbose mode off by default to avoid bloating logs. On for any session with `--debug` or for failed sessions (auto-bump-on-error).

## Side note

`sessions.jsonl` (the 37 MB whole-session log) does have full `prompt` text but no per-turn breakdown. So we have two log formats and neither captures the full picture. This bead is about the per-event log only; whole-session log is out of scope.
    </description>
    <acceptance>
1. `DDX_AGENT_LOG_VERBOSE=1 ddx agent ...` produces a trace where llm.response events include `content`, `finish_reason`, `usage`, and tool.call events include `input`, `output`, `duration_ms`, `error`.
2. Schema documented in cli/internal/agent/session_log_format.go (Go doc comment listing all fields per event type for both lean and verbose modes).
3. Default behavior unchanged (lean mode by default; existing tooling that consumes the lean schema continues to work).
4. Test fixture added for verbose mode that asserts every required field is populated.
    </acceptance>
    <labels>kind:implementation, area:agent, area:measurement</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T031704-db3a0920/manifest.json</file>
    <file>.ddx/executions/20260429T031704-db3a0920/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8d01746cfd3a2df8e7c175a9b700d570c6d8ecd4">
diff --git a/.ddx/executions/20260429T031704-db3a0920/manifest.json b/.ddx/executions/20260429T031704-db3a0920/manifest.json
new file mode 100644
index 00000000..454e527e
--- /dev/null
+++ b/.ddx/executions/20260429T031704-db3a0920/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T031704-db3a0920",
+  "bead_id": "ddx-969c5500",
+  "base_rev": "4fa2686a26f7425d9a7d484a69ce694f95ca1320",
+  "created_at": "2026-04-29T03:17:04.933050788Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-969c5500",
+    "title": "FEAT: agent-logs jsonl schema is metadata-only; add verbose mode capturing tool I/O and content",
+    "description": "## Problem\n\n`.ddx/agent-logs/agent-claude-*.jsonl` (lean schema) records only:\n  - `session.start` (model, harness, bead_id)\n  - `llm.response` (turn, input_tokens, output_tokens, elapsed_ms)\n  - `tool.call`     (turn, tool name)\n\nNotably absent: model output content, tool call arguments, tool call results, finish_reason, full usage breakdown (cache reads, cache writes), error/stop reasons.\n\nMeanwhile, `agent/demos/sessions/*.jsonl` from the sibling `Projects/agent` repo demonstrates a richer schema for the same event types:\n\n```jsonl\n{\"type\":\"llm.response\",\"data\":{\"content\":\"...\",\"finish_reason\":\"tool_calls\",\"usage\":{\"input\":899,\"output\":21,\"total\":920},\"latency_ms\":3040,\"tool_calls\":1}}\n{\"type\":\"tool.call\",\"data\":{\"tool\":\"edit\",\"input\":{\"path\":\"...\",\"old_string\":\"...\",\"new_string\":\"...\"},\"output\":\"Replaced 1 occurrence\",\"duration_ms\":0,\"error\":\"\"}}\n```\n\n## Why it matters\n\n- Cannot replay a real session to reproduce a bug — the inputs are gone.\n- Cannot root-cause provider failures (e.g. RotatingKVCache Quantization NYI seen in sessions.jsonl) because the prompt that triggered it is not in the per-event log; only the whole-session metadata in sessions.jsonl has it.\n- The lucebox-hub/dflash `bench_server.py` replay-mode benchmark needs full prompt text from real traces, not just metadata, to be representative.\n- Inability to tell what tool args caused a slow call.\n\n## Proposed fix\n\nAdd a verbose mode to the agent-logs writer (env var `DDX_AGENT_LOG_VERBOSE=1` or config flag) that adopts the agent/demos schema:\n  - llm.response: include `content`, `finish_reason`, full `usage` object\n  - tool.call: include `input` (tool args), `output` (truncated to N KB), `duration_ms`, `error`\n  - new event: `llm.request` with messages_count and possibly truncated message preview\n\nVerbose mode off by default to avoid bloating logs. On for any session with `--debug` or for failed sessions (auto-bump-on-error).\n\n## Side note\n\n`sessions.jsonl` (the 37 MB whole-session log) does have full `prompt` text but no per-turn breakdown. So we have two log formats and neither captures the full picture. This bead is about the per-event log only; whole-session log is out of scope.",
+    "acceptance": "1. `DDX_AGENT_LOG_VERBOSE=1 ddx agent ...` produces a trace where llm.response events include `content`, `finish_reason`, `usage`, and tool.call events include `input`, `output`, `duration_ms`, `error`.\n2. Schema documented in cli/internal/agent/session_log_format.go (Go doc comment listing all fields per event type for both lean and verbose modes).\n3. Default behavior unchanged (lean mode by default; existing tooling that consumes the lean schema continues to work).\n4. Test fixture added for verbose mode that asserts every required field is populated.",
+    "labels": [
+      "kind:implementation",
+      "area:agent",
+      "area:measurement"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T03:17:04Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T03:17:04.266105769Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T031704-db3a0920",
+    "prompt": ".ddx/executions/20260429T031704-db3a0920/prompt.md",
+    "manifest": ".ddx/executions/20260429T031704-db3a0920/manifest.json",
+    "result": ".ddx/executions/20260429T031704-db3a0920/result.json",
+    "checks": ".ddx/executions/20260429T031704-db3a0920/checks.json",
+    "usage": ".ddx/executions/20260429T031704-db3a0920/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-969c5500-20260429T031704-db3a0920"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T031704-db3a0920/result.json b/.ddx/executions/20260429T031704-db3a0920/result.json
new file mode 100644
index 00000000..c8b3b4a4
--- /dev/null
+++ b/.ddx/executions/20260429T031704-db3a0920/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-969c5500",
+  "attempt_id": "20260429T031704-db3a0920",
+  "base_rev": "4fa2686a26f7425d9a7d484a69ce694f95ca1320",
+  "result_rev": "093d2ffc61f10d10f1996a6f60d919b44b448cdb",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-723cab28",
+  "duration_ms": 543145,
+  "tokens": 15673,
+  "cost_usd": 1.71533925,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T031704-db3a0920",
+  "prompt_file": ".ddx/executions/20260429T031704-db3a0920/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T031704-db3a0920/manifest.json",
+  "result_file": ".ddx/executions/20260429T031704-db3a0920/result.json",
+  "usage_file": ".ddx/executions/20260429T031704-db3a0920/usage.json",
+  "started_at": "2026-04-29T03:17:04.933332913Z",
+  "finished_at": "2026-04-29T03:26:08.078784258Z"
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
