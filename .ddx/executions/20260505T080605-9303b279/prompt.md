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
    <file>.ddx/executions/20260505T075319-cd3f2016/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T075319-cd3f2016/manifest.json</file>
    <file>.ddx/executions/20260505T075319-cd3f2016/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a998917abae2ae276dd4cf08a159caef5f215264">
diff --git a/.ddx/executions/20260505T075319-cd3f2016/checks/production-reachability.json b/.ddx/executions/20260505T075319-cd3f2016/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T075319-cd3f2016/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T075319-cd3f2016/manifest.json b/.ddx/executions/20260505T075319-cd3f2016/manifest.json
new file mode 100644
index 00000000..be05e6e2
--- /dev/null
+++ b/.ddx/executions/20260505T075319-cd3f2016/manifest.json
@@ -0,0 +1,76 @@
+{
+  "attempt_id": "20260505T075319-cd3f2016",
+  "bead_id": "ddx-4d81bb26",
+  "base_rev": "f01c29ea89ea4983e648136c9143f62722b08496",
+  "created_at": "2026-05-05T07:53:21.300811415Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-4d81bb26",
+    "title": "Remove remaining DDx-side usage/quota cache consumers",
+    "description": "PROBLEM\nDDx still has local usage/routing cache consumers after the provider-native parser removal bead closed. These paths can contradict Fizeau route/status semantics and preserve stale source names like stats-cache, quota-snapshot, ddx-metrics, routing-outcomes, and burn-summaries.\n\nEVIDENCE\n- cli/internal/agent/routing_metrics.go still defines RoutingMetricsStore and reads/writes routing-outcomes.jsonl and burn-summaries.jsonl.\n- cli/cmd/agent_usage.go reads RoutingMetricsStore for agent usage enrichment.\n- cli/cmd/agent_route_status.go merges RoutingMetricsStore data into route status.\n- cli/internal/server/providers.go reads RoutingMetricsStore and maps stats-cache/quota-snapshot/http-balance/http-models/recent-session-log into provider signal sources.\n- cli/cmd/agent_cmd.go still emits stats-cache as a signal kind.\n- Existing removal bead ddx-3610f1fc is closed and only covered the old parser/health-shadow layer.\n\nGOAL\nMake DDx a thin consumer of Fizeau service routing/status data. DDx may keep execution logs for audit, but provider availability/quota/current usage must not be derived from stale DDx-side cache files or provider-native cache labels.",
+    "acceptance": "1. Remove or demote RoutingMetricsStore consumers from provider availability, quota, route-status, and agent usage decisions; DDx must use Fizeau service ListHarnesses/ListProviders/RouteStatus/RecordRouteAttempt for those semantics.\n2. cli/internal/server/providers.go no longer reports stats-cache, quota-snapshot, http-balance, http-models, recent-session-log, or ddx-metrics as current provider signal sources unless they are direct Fizeau-provided source names with matching semantics.\n3. cli/cmd/agent_route_status.go no longer merges routing-outcomes.jsonl into current routing availability/status.\n4. cli/cmd/agent_usage.go does not infer current quota/headroom from DDx routing-outcomes.jsonl or burn-summaries.jsonl. Historical audit display remains allowed if clearly labeled audit-only.\n5. Tests cover stale local cache data not affecting provider quota/availability/status.\n6. cd cli \u0026\u0026 go test ./cmd ./internal/agent ./internal/server/... passes.",
+    "labels": [
+      "area:agent",
+      "area:routing",
+      "kind:cleanup",
+      "thin-consumer",
+      "triage:no-changes-unjustified"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T07:53:19Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T01:52:11.203235617Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T014149-221cdee8\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":11914463,\"output_tokens\":45341,\"total_tokens\":11959804,\"cost_usd\":0,\"duration_ms\":603815,\"exit_code\":0}",
+          "created_at": "2026-05-05T01:52:11.610021364Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=11959804 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "(rationale absent)",
+          "created_at": "2026-05-05T01:52:12.838549597Z",
+          "kind": "no_changes_unjustified",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes_unjustified"
+        },
+        {
+          "actor": "erik",
+          "body": "agent exited without a commit or no_changes_rationale.txt; dirty paths: cli/cmd/agent_cmd.go, cli/cmd/agent_route_status.go, cli/cmd/agent_route_status_test.go, cli/cmd/agent_usage.go, cli/internal/server/providers.go, cli/internal/server/providers_test.go\nresult_rev=d037dba3d2263736fc357aa4a3b087eef11603cf\nbase_rev=d037dba3d2263736fc357aa4a3b087eef11603cf\nretry_after=2026-05-05T07:52:13Z",
+          "created_at": "2026-05-05T01:52:13.304494748Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T07:53:19.325910276Z",
+      "execute-loop-last-detail": "agent exited without a commit or no_changes_rationale.txt; dirty paths: cli/cmd/agent_cmd.go, cli/cmd/agent_route_status.go, cli/cmd/agent_route_status_test.go, cli/cmd/agent_usage.go, cli/internal/server/providers.go, cli/internal/server/providers_test.go",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-05-05T07:52:13Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T075319-cd3f2016",
+    "prompt": ".ddx/executions/20260505T075319-cd3f2016/prompt.md",
+    "manifest": ".ddx/executions/20260505T075319-cd3f2016/manifest.json",
+    "result": ".ddx/executions/20260505T075319-cd3f2016/result.json",
+    "checks": ".ddx/executions/20260505T075319-cd3f2016/checks.json",
+    "usage": ".ddx/executions/20260505T075319-cd3f2016/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-4d81bb26-20260505T075319-cd3f2016"
+  },
+  "prompt_sha": "47e065900bc1af405ec179d07f6e1e6e914779bef64d5f0d3a02819c24161367"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T075319-cd3f2016/result.json b/.ddx/executions/20260505T075319-cd3f2016/result.json
new file mode 100644
index 00000000..7e9e456e
--- /dev/null
+++ b/.ddx/executions/20260505T075319-cd3f2016/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-4d81bb26",
+  "attempt_id": "20260505T075319-cd3f2016",
+  "base_rev": "f01c29ea89ea4983e648136c9143f62722b08496",
+  "result_rev": "578714ae226d0ae1f4ef42e9e9b272d44b965a50",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-a2ac1233",
+  "duration_ms": 756439,
+  "tokens": 15849101,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T075319-cd3f2016",
+  "prompt_file": ".ddx/executions/20260505T075319-cd3f2016/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T075319-cd3f2016/manifest.json",
+  "result_file": ".ddx/executions/20260505T075319-cd3f2016/result.json",
+  "usage_file": ".ddx/executions/20260505T075319-cd3f2016/usage.json",
+  "started_at": "2026-05-05T07:53:21.30115129Z",
+  "finished_at": "2026-05-05T08:05:57.740552698Z"
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
