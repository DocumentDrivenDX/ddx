<bead-review>
  <bead id="ddx-4d81bb26" iter=1>
    <title>Remove remaining DDx-side usage/quota cache consumers</title>
    <description>
PROBLEM
DDx still has local usage/routing cache consumers after the provider-native parser removal bead closed. These paths can contradict Fizeau route/status semantics and preserve stale source names like stats-cache, quota-snapshot, ddx-metrics, routing-outcomes, and burn-summaries.

EVIDENCE
- cli/internal/agent/routing_metrics.go still defines RoutingMetricsStore and reads/writes routing-outcomes.jsonl and burn-summaries.jsonl.
- cli/cmd/agent_usage.go reads RoutingMetricsStore for agent usage enrichment.
- cli/cmd/agent_route_status.go merges RoutingMetricsStore data into route status.
- cli/internal/server/providers.go reads RoutingMetricsStore and maps stats-cache/quota-snapshot/http-balance/http-models/recent-session-log into provider signal sources.
- cli/cmd/agent_cmd.go still emits stats-cache as a signal kind.
- Existing removal bead ddx-3610f1fc is closed and only covered the old parser/health-shadow layer.

GOAL
Make DDx a thin consumer of Fizeau service routing/status data. DDx may keep execution logs for audit, but provider availability/quota/current usage must not be derived from stale DDx-side cache files or provider-native cache labels.
    </description>
    <acceptance>
1. Remove or demote RoutingMetricsStore consumers from provider availability, quota, route-status, and agent usage decisions; DDx must use Fizeau service ListHarnesses/ListProviders/RouteStatus/RecordRouteAttempt for those semantics.
2. cli/internal/server/providers.go no longer reports stats-cache, quota-snapshot, http-balance, http-models, recent-session-log, or ddx-metrics as current provider signal sources unless they are direct Fizeau-provided source names with matching semantics.
3. cli/cmd/agent_route_status.go no longer merges routing-outcomes.jsonl into current routing availability/status.
4. cli/cmd/agent_usage.go does not infer current quota/headroom from DDx routing-outcomes.jsonl or burn-summaries.jsonl. Historical audit display remains allowed if clearly labeled audit-only.
5. Tests cover stale local cache data not affecting provider quota/availability/status.
6. cd cli &amp;&amp; go test ./cmd ./internal/agent ./internal/server/... passes.
    </acceptance>
    <labels>area:agent, area:routing, kind:cleanup, thin-consumer, triage:no-changes-unjustified</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T155333-1e854f74/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T155333-1e854f74/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="da3278a82f68607de77bd6407d222942b7a2b25c">
diff --git a/.ddx/executions/20260505T155333-1e854f74/checks/production-reachability.json b/.ddx/executions/20260505T155333-1e854f74/checks/production-reachability.json
new file mode 100644
index 00000000..89e73251
--- /dev/null
+++ b/.ddx/executions/20260505T155333-1e854f74/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no non-test Go files changed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T155333-1e854f74/result.json b/.ddx/executions/20260505T155333-1e854f74/result.json
new file mode 100644
index 00000000..b2aed7f2
--- /dev/null
+++ b/.ddx/executions/20260505T155333-1e854f74/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-4d81bb26",
+  "attempt_id": "20260505T155333-1e854f74",
+  "base_rev": "8de9ff7b49a2799f2911d5ac21d8ce5ba6312541",
+  "result_rev": "3289493074cf3a23e9b3995c3ca7e8b2ebddd4e7",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-1659b1f3",
+  "duration_ms": 524109,
+  "tokens": 10333242,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T155333-1e854f74",
+  "prompt_file": ".ddx/executions/20260505T155333-1e854f74/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T155333-1e854f74/manifest.json",
+  "result_file": ".ddx/executions/20260505T155333-1e854f74/result.json",
+  "usage_file": ".ddx/executions/20260505T155333-1e854f74/usage.json",
+  "started_at": "2026-05-05T15:53:35.788266568Z",
+  "finished_at": "2026-05-05T16:02:19.897889734Z"
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
