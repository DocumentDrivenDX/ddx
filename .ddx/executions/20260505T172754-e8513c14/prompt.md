<bead-review>
  <bead id="ddx-ad41ccb1" iter=1>
    <title>agent-output: print selected route economics in default ddx work output</title>
    <description>
PROBLEM
  The default human output for ddx work should tell the operator which harness/provider/model were selected and the predicted speed, cost, and power for that route. Today the summary is mostly bead-level status, so operators cannot quickly see whether fizeau routed to the expected class of model or why a run is slow/expensive.

ROOT CAUSE
  fizeau already emits routing_decision with selected harness/provider/model and candidate components: service_execute.go:431 emits ServiceRoutingDecisionData, and service_events.go:142-183 exposes candidates[].cost_usd_per_1k_tokens plus components.power and components.speed_tps. DDx drains only harness/provider/model and selected candidate power in cli/internal/agent/agent_runner_service.go:234-318; Result in cli/internal/agent/types.go:96-122 has ActualPower but no predicted_speed_tps or predicted_cost_usd_per_1k_tokens. cli/internal/agent/execute_bead.go:831-850 copies only harness/provider/model/ActualPower from Result, and cli/cmd/agent_cmd.go:1955-1961 prints failed attempts without route economics.

PROPOSED FIX
  - Extend cli/internal/agent.Result and ExecuteBeadReport with route prediction fields: PredictedSpeedTPS float64, PredictedCostUSDPer1kTokens float64, PredictedPower int, and CostSource string if available. Keep ActualPower for final/routing_actual behavior.
  - Update drainServiceEvents to select the eligible candidate matching routing_decision.model and copy components.speed_tps, cost_usd_per_1k_tokens, cost_source, and components.power.
  - Propagate the fields through executeOnService -&gt; ExecuteBeadWithConfig -&gt; ExecuteBeadReport -&gt; result.json and execute-bead event bodies.
  - In default non-JSON ddx work / execute-loop output, print a concise route line near bead claim or first attempt completion: harness=&lt;h&gt; provider=&lt;p&gt; model=&lt;m&gt; power=&lt;n&gt; speed=&lt;tps&gt; tok/s cost=$&lt;x&gt;/1k tok. Keep --json output machine-readable and add fields there rather than human prose.

NON-SCOPE
  - Changing fizeau routing scoring.
  - New upstream fizeau fields; current routing_decision already has the needed candidate data.
  - Progress-event rendering; tracked in ddx-cad808ea.

PARENT
  No parent.

DEPS
  No deps. fizeau already emits routing_decision candidate cost/speed/power fields.
    </description>
    <acceptance>
1. cli/internal/agent/types.go Result includes predicted route fields for speed TPS, cost USD per 1k tokens, cost source, and predicted power without removing ActualPower.
2. TestDrainServiceEvents_CapturesRouteEconomics verifies routing_decision.candidates for the selected eligible model populate speed_tps, cost_usd_per_1k_tokens, cost_source, and power.
3. TestExecuteBead_ReportIncludesRouteEconomics verifies ExecuteBeadReport/result.json carries harness/provider/model plus predicted speed/cost/power fields.
4. TestWorkDefaultOutput_PrintsSelectedRouteEconomics verifies non-JSON ddx work output includes harness, provider, model, predicted speed, cost, and power for an attempted bead.
5. TestWorkJSONOutput_IncludesRouteEconomicsWithoutHumanLines verifies --json includes the fields structurally and does not include the human route line.
6. cd cli &amp;&amp; go test ./internal/agent/... ./cmd/... green.
7. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, area:cli, kind:feature, operator-ux, routing, triage:no-changes-unjustified</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T171827-5ea92a57/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T171827-5ea92a57/manifest.json</file>
    <file>.ddx/executions/20260505T171827-5ea92a57/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9695c2faaffd7e0ca962e977a4fb2b37faf961f8">
diff --git a/.ddx/executions/20260505T171827-5ea92a57/result.json b/.ddx/executions/20260505T171827-5ea92a57/result.json
new file mode 100644
index 00000000..b62400f7
--- /dev/null
+++ b/.ddx/executions/20260505T171827-5ea92a57/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-ad41ccb1",
+  "attempt_id": "20260505T171827-5ea92a57",
+  "base_rev": "dbc8f9ca00ebf0890e55487129034dcc731306ad",
+  "result_rev": "ca3a99d6da0cbff774886cf3643a71bdb2b98ce2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-a2806c62",
+  "duration_ms": 555433,
+  "tokens": 10259960,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T171827-5ea92a57",
+  "prompt_file": ".ddx/executions/20260505T171827-5ea92a57/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T171827-5ea92a57/manifest.json",
+  "result_file": ".ddx/executions/20260505T171827-5ea92a57/result.json",
+  "usage_file": ".ddx/executions/20260505T171827-5ea92a57/usage.json",
+  "started_at": "2026-05-05T17:18:29.612503443Z",
+  "finished_at": "2026-05-05T17:27:45.0462277Z"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T171827-5ea92a57/checks/production-reachability.json b/.ddx/executions/20260505T171827-5ea92a57/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T171827-5ea92a57/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T171827-5ea92a57/manifest.json b/.ddx/executions/20260505T171827-5ea92a57/manifest.json
new file mode 100644
index 00000000..d253bbbb
--- /dev/null
+++ b/.ddx/executions/20260505T171827-5ea92a57/manifest.json
@@ -0,0 +1,134 @@
+{
+  "attempt_id": "20260505T171827-5ea92a57",
+  "bead_id": "ddx-ad41ccb1",
+  "base_rev": "dbc8f9ca00ebf0890e55487129034dcc731306ad",
+  "created_at": "2026-05-05T17:18:29.611944777Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ad41ccb1",
+    "title": "agent-output: print selected route economics in default ddx work output",
+    "description": "PROBLEM\n  The default human output for ddx work should tell the operator which harness/provider/model were selected and the predicted speed, cost, and power for that route. Today the summary is mostly bead-level status, so operators cannot quickly see whether fizeau routed to the expected class of model or why a run is slow/expensive.\n\nROOT CAUSE\n  fizeau already emits routing_decision with selected harness/provider/model and candidate components: service_execute.go:431 emits ServiceRoutingDecisionData, and service_events.go:142-183 exposes candidates[].cost_usd_per_1k_tokens plus components.power and components.speed_tps. DDx drains only harness/provider/model and selected candidate power in cli/internal/agent/agent_runner_service.go:234-318; Result in cli/internal/agent/types.go:96-122 has ActualPower but no predicted_speed_tps or predicted_cost_usd_per_1k_tokens. cli/internal/agent/execute_bead.go:831-850 copies only harness/provider/model/ActualPower from Result, and cli/cmd/agent_cmd.go:1955-1961 prints failed attempts without route economics.\n\nPROPOSED FIX\n  - Extend cli/internal/agent.Result and ExecuteBeadReport with route prediction fields: PredictedSpeedTPS float64, PredictedCostUSDPer1kTokens float64, PredictedPower int, and CostSource string if available. Keep ActualPower for final/routing_actual behavior.\n  - Update drainServiceEvents to select the eligible candidate matching routing_decision.model and copy components.speed_tps, cost_usd_per_1k_tokens, cost_source, and components.power.\n  - Propagate the fields through executeOnService -\u003e ExecuteBeadWithConfig -\u003e ExecuteBeadReport -\u003e result.json and execute-bead event bodies.\n  - In default non-JSON ddx work / execute-loop output, print a concise route line near bead claim or first attempt completion: harness=\u003ch\u003e provider=\u003cp\u003e model=\u003cm\u003e power=\u003cn\u003e speed=\u003ctps\u003e tok/s cost=$\u003cx\u003e/1k tok. Keep --json output machine-readable and add fields there rather than human prose.\n\nNON-SCOPE\n  - Changing fizeau routing scoring.\n  - New upstream fizeau fields; current routing_decision already has the needed candidate data.\n  - Progress-event rendering; tracked in ddx-cad808ea.\n\nPARENT\n  No parent.\n\nDEPS\n  No deps. fizeau already emits routing_decision candidate cost/speed/power fields.",
+    "acceptance": "1. cli/internal/agent/types.go Result includes predicted route fields for speed TPS, cost USD per 1k tokens, cost source, and predicted power without removing ActualPower.\n2. TestDrainServiceEvents_CapturesRouteEconomics verifies routing_decision.candidates for the selected eligible model populate speed_tps, cost_usd_per_1k_tokens, cost_source, and power.\n3. TestExecuteBead_ReportIncludesRouteEconomics verifies ExecuteBeadReport/result.json carries harness/provider/model plus predicted speed/cost/power fields.\n4. TestWorkDefaultOutput_PrintsSelectedRouteEconomics verifies non-JSON ddx work output includes harness, provider, model, predicted speed, cost, and power for an attempted bead.\n5. TestWorkJSONOutput_IncludesRouteEconomicsWithoutHumanLines verifies --json includes the fields structurally and does not include the human route line.\n6. cd cli \u0026\u0026 go test ./internal/agent/... ./cmd/... green.\n7. lefthook run pre-commit passes.",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "area:cli",
+      "kind:feature",
+      "operator-ux",
+      "routing",
+      "triage:no-changes-unjustified"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T17:18:27Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T21:54:09.668245877Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=8d6b5b3aa9eb6a08043f8b350227726205cc9413\nbase_rev=8d6b5b3aa9eb6a08043f8b350227726205cc9413\nretry_after=2026-05-05T03:54:10Z",
+          "created_at": "2026-05-04T21:54:10.422362057Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T04:48:34.378052887Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T042952-02c7f80d\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":22278104,\"output_tokens\":48088,\"total_tokens\":22326192,\"cost_usd\":0,\"duration_ms\":1119665,\"exit_code\":0}",
+          "created_at": "2026-05-05T04:48:34.608081467Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=22326192 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T04:48:41.797397711Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=a5597520a9c491f6914305eb2fd86363cd4e1e35\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T00:53:46-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=13116\noutput_bytes=0\nelapsed_ms=4219",
+          "created_at": "2026-05-05T04:48:46.556559997Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=a5597520a9c491f6914305eb2fd86363cd4e1e35\nbase_rev=73ef66afe45a262eb43c2a0845ed3738f83a8d48",
+          "created_at": "2026-05-05T04:48:46.795455115Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T11:14:24.513751594Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T111116-324b6466\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":1938401,\"output_tokens\":7399,\"total_tokens\":1945800,\"cost_usd\":0,\"duration_ms\":186385,\"exit_code\":0}",
+          "created_at": "2026-05-05T11:14:24.725284308Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=1945800 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "(rationale absent)",
+          "created_at": "2026-05-05T11:14:25.604507917Z",
+          "kind": "no_changes_unjustified",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes_unjustified"
+        },
+        {
+          "actor": "erik",
+          "body": "agent exited without a commit or no_changes_rationale.txt\nresult_rev=fc35aefd4f591a36f91608b4f01586ad6e271c9c\nbase_rev=fc35aefd4f591a36f91608b4f01586ad6e271c9c\nretry_after=2026-05-05T17:14:25Z",
+          "created_at": "2026-05-05T11:14:26.025721389Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T17:18:27.078151488Z",
+      "execute-loop-last-detail": "agent exited without a commit or no_changes_rationale.txt",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-05-05T17:14:25Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T171827-5ea92a57",
+    "prompt": ".ddx/executions/20260505T171827-5ea92a57/prompt.md",
+    "manifest": ".ddx/executions/20260505T171827-5ea92a57/manifest.json",
+    "result": ".ddx/executions/20260505T171827-5ea92a57/result.json",
+    "checks": ".ddx/executions/20260505T171827-5ea92a57/checks.json",
+    "usage": ".ddx/executions/20260505T171827-5ea92a57/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ad41ccb1-20260505T171827-5ea92a57"
+  },
+  "prompt_sha": "937ba9af7470ffdd8d74ab3e8861b68404459bb9f8d417d2d83831f878c5981a"
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
