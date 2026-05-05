<bead-review>
  <bead id="ddx-cad808ea" iter=1>
    <title>agent-output: render fizeau progress events during ddx work</title>
    <description>
PROBLEM
  ddx work currently prints the claimed bead line (for example, "fizeau-38095979: catalog: bound remote refresh and plugin sync work") and then can sit silent for long periods while fizeau/LLM execution is active. Operators need occasional progress updates for each LLM turn, tool call, final response, context summary, and compaction event.

ROOT CAUSE
  cli/cmd/agent_cmd.go:1680-1682 starts TailSessionLogs while ExecuteBead runs, and cli/internal/agent/session_log_tailer.go:18-41 tails agent-*.jsonl files. cli/internal/agent/session_log_format.go:80-250 formats existing session log event types (llm.request, llm.response, tool.call, compaction.end), but it has no parser/rendering path for the upstream fizeau progress event proposed in fizeau-acd91e9c and fizeau-0e13d680. service_run.go:188-194 drains fizeau service events into Result, but progress events are not retained or surfaced to stdout.

PROPOSED FIX
  - After fizeau-acd91e9c and fizeau-0e13d680 land, update DDx's fizeau dependency and add progress-event decoding in cli/internal/agent/session_log_format.go.
  - Render progress events as concise stdout lines: thinking start/update/complete with token and duration fields; tool start/complete with bounded command/name and duration; response start/complete with output tokens; context/compaction updates with bounded session summary.
  - Keep output bounded and default-on for human output, but do not affect --json output.
  - Ensure TailSessionLogs prints each progress line once and does not replay old session logs.

NON-SCOPE
  - Defining the upstream fizeau progress event schema; tracked in fizeau-acd91e9c.
  - Subprocess harness progress synthesis in fizeau; tracked in fizeau-0e13d680.
  - Changing execute-bead review or landing behavior.

PARENT
  No parent.

DEPS
  - fizeau-acd91e9c: upstream progress event schema/native service mapping.
  - fizeau-0e13d680: upstream subprocess-backed harness progress parity.
    </description>
    <acceptance>
1. cli/internal/agent/session_log_format.go recognizes fizeau progress event JSON and renders bounded human lines for thinking start/update/complete, tool start/complete, response completion, context summary, and compaction progress.
2. TestFormatSessionLogLines_ProgressThinking verifies examples equivalent to "thinking ..." and "thinking complete 8 tok in 5s" render from progress events.
3. TestFormatSessionLogLines_ProgressTool verifies examples equivalent to "running tool call `ls -al` ..." and "completed in 3s, 100 tok" render with bounded command text.
4. TestFormatSessionLogLines_ProgressResponseAndContext verifies response token and session-context summary progress events render and remain bounded.
5. TestTailSessionLogs_ProgressEventsNotReplayed verifies the tailer prints new progress lines once and does not replay pre-existing log lines.
6. --json output from ddx work / ddx agent execute-loop remains structured JSON without human progress lines mixed in.
7. cd cli &amp;&amp; go test ./internal/agent/... ./cmd/... green.
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, area:cli, kind:feature, operator-ux, upstream:fizeau</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T110812-aa87f66b/manifest.json</file>
    <file>.ddx/executions/20260505T110812-aa87f66b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a77656bd6774793feba2951f0517b0c1307e4e42">
diff --git a/.ddx/executions/20260505T110812-aa87f66b/manifest.json b/.ddx/executions/20260505T110812-aa87f66b/manifest.json
new file mode 100644
index 00000000..310e0c3c
--- /dev/null
+++ b/.ddx/executions/20260505T110812-aa87f66b/manifest.json
@@ -0,0 +1,101 @@
+{
+  "attempt_id": "20260505T110812-aa87f66b",
+  "bead_id": "ddx-cad808ea",
+  "base_rev": "414e15e6738d7d42670918f238ff720eaf5b7aad",
+  "created_at": "2026-05-05T11:08:14.190490192Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cad808ea",
+    "title": "agent-output: render fizeau progress events during ddx work",
+    "description": "PROBLEM\n  ddx work currently prints the claimed bead line (for example, \"fizeau-38095979: catalog: bound remote refresh and plugin sync work\") and then can sit silent for long periods while fizeau/LLM execution is active. Operators need occasional progress updates for each LLM turn, tool call, final response, context summary, and compaction event.\n\nROOT CAUSE\n  cli/cmd/agent_cmd.go:1680-1682 starts TailSessionLogs while ExecuteBead runs, and cli/internal/agent/session_log_tailer.go:18-41 tails agent-*.jsonl files. cli/internal/agent/session_log_format.go:80-250 formats existing session log event types (llm.request, llm.response, tool.call, compaction.end), but it has no parser/rendering path for the upstream fizeau progress event proposed in fizeau-acd91e9c and fizeau-0e13d680. service_run.go:188-194 drains fizeau service events into Result, but progress events are not retained or surfaced to stdout.\n\nPROPOSED FIX\n  - After fizeau-acd91e9c and fizeau-0e13d680 land, update DDx's fizeau dependency and add progress-event decoding in cli/internal/agent/session_log_format.go.\n  - Render progress events as concise stdout lines: thinking start/update/complete with token and duration fields; tool start/complete with bounded command/name and duration; response start/complete with output tokens; context/compaction updates with bounded session summary.\n  - Keep output bounded and default-on for human output, but do not affect --json output.\n  - Ensure TailSessionLogs prints each progress line once and does not replay old session logs.\n\nNON-SCOPE\n  - Defining the upstream fizeau progress event schema; tracked in fizeau-acd91e9c.\n  - Subprocess harness progress synthesis in fizeau; tracked in fizeau-0e13d680.\n  - Changing execute-bead review or landing behavior.\n\nPARENT\n  No parent.\n\nDEPS\n  - fizeau-acd91e9c: upstream progress event schema/native service mapping.\n  - fizeau-0e13d680: upstream subprocess-backed harness progress parity.",
+    "acceptance": "1. cli/internal/agent/session_log_format.go recognizes fizeau progress event JSON and renders bounded human lines for thinking start/update/complete, tool start/complete, response completion, context summary, and compaction progress.\n2. TestFormatSessionLogLines_ProgressThinking verifies examples equivalent to \"thinking ...\" and \"thinking complete 8 tok in 5s\" render from progress events.\n3. TestFormatSessionLogLines_ProgressTool verifies examples equivalent to \"running tool call `ls -al` ...\" and \"completed in 3s, 100 tok\" render with bounded command text.\n4. TestFormatSessionLogLines_ProgressResponseAndContext verifies response token and session-context summary progress events render and remain bounded.\n5. TestTailSessionLogs_ProgressEventsNotReplayed verifies the tailer prints new progress lines once and does not replay pre-existing log lines.\n6. --json output from ddx work / ddx agent execute-loop remains structured JSON without human progress lines mixed in.\n7. cd cli \u0026\u0026 go test ./internal/agent/... ./cmd/... green.\n8. lefthook run pre-commit passes.",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "area:cli",
+      "kind:feature",
+      "operator-ux",
+      "upstream:fizeau"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T11:08:12Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T21:53:17.474354989Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=34d3027e10db5cb970f663470cb6e40afc2aaf6c\nbase_rev=34d3027e10db5cb970f663470cb6e40afc2aaf6c\nretry_after=2026-05-05T03:53:17Z",
+          "created_at": "2026-05-04T21:53:18.207856353Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T04:29:38.762673492Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T041538-044142a9\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":13015542,\"output_tokens\":26291,\"total_tokens\":13041833,\"cost_usd\":0,\"duration_ms\":838125,\"exit_code\":0}",
+          "created_at": "2026-05-05T04:29:39.048225802Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=13041833 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T04:29:46.543530814Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=dd963e4f94c344dfea528a90ab8833388cab6faa\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T00:34:51-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=12693\noutput_bytes=0\nelapsed_ms=4182",
+          "created_at": "2026-05-05T04:29:51.276489841Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=dd963e4f94c344dfea528a90ab8833388cab6faa\nbase_rev=9e259bf650838b776ee9a4a0c6a2642f083f5525",
+          "created_at": "2026-05-05T04:29:51.494182671Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T11:08:12.191445624Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T03:53:17Z",
+      "upstream-fizeau": "fizeau-acd91e9c,fizeau-0e13d680"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T110812-aa87f66b",
+    "prompt": ".ddx/executions/20260505T110812-aa87f66b/prompt.md",
+    "manifest": ".ddx/executions/20260505T110812-aa87f66b/manifest.json",
+    "result": ".ddx/executions/20260505T110812-aa87f66b/result.json",
+    "checks": ".ddx/executions/20260505T110812-aa87f66b/checks.json",
+    "usage": ".ddx/executions/20260505T110812-aa87f66b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cad808ea-20260505T110812-aa87f66b"
+  },
+  "prompt_sha": "a9d8c3120cfc96bb21f22e33f599d418a470b600cd0f32d232cca1c02edb114b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T110812-aa87f66b/result.json b/.ddx/executions/20260505T110812-aa87f66b/result.json
new file mode 100644
index 00000000..c881d17c
--- /dev/null
+++ b/.ddx/executions/20260505T110812-aa87f66b/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-cad808ea",
+  "attempt_id": "20260505T110812-aa87f66b",
+  "base_rev": "414e15e6738d7d42670918f238ff720eaf5b7aad",
+  "result_rev": "5b749cba0d4b55938b2e04273323d382f90b3aa2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-cc970e4a",
+  "duration_ms": 169773,
+  "tokens": 3433924,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T110812-aa87f66b",
+  "prompt_file": ".ddx/executions/20260505T110812-aa87f66b/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T110812-aa87f66b/manifest.json",
+  "result_file": ".ddx/executions/20260505T110812-aa87f66b/result.json",
+  "usage_file": ".ddx/executions/20260505T110812-aa87f66b/usage.json",
+  "started_at": "2026-05-05T11:08:14.190875733Z",
+  "finished_at": "2026-05-05T11:11:03.964077341Z"
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
