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
    <file>.ddx/executions/20260505T124242-583923ab/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T124242-583923ab/manifest.json</file>
    <file>.ddx/executions/20260505T124242-583923ab/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b7a4a4a4d3d307300e7965a9499c7d27c958ea07">
diff --git a/.ddx/executions/20260505T124242-583923ab/checks/production-reachability.json b/.ddx/executions/20260505T124242-583923ab/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T124242-583923ab/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T124242-583923ab/manifest.json b/.ddx/executions/20260505T124242-583923ab/manifest.json
new file mode 100644
index 00000000..02733571
--- /dev/null
+++ b/.ddx/executions/20260505T124242-583923ab/manifest.json
@@ -0,0 +1,156 @@
+{
+  "attempt_id": "20260505T124242-583923ab",
+  "bead_id": "ddx-4d81bb26",
+  "base_rev": "8a4a4bbd8b13d5fc0282c2e93e06ca6262848387",
+  "created_at": "2026-05-05T12:42:44.381823922Z",
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
+      "claimed-at": "2026-05-05T12:42:42Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
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
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T08:05:57.742554738Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T075319-cd3f2016\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":15797223,\"output_tokens\":51878,\"total_tokens\":15849101,\"cost_usd\":0,\"duration_ms\":756439,\"exit_code\":0}",
+          "created_at": "2026-05-05T08:05:57.952534799Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=15849101 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T08:06:05.087500564Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=a998917abae2ae276dd4cf08a159caef5f215264\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T04:11:09-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=12150\noutput_bytes=0\nelapsed_ms=4204",
+          "created_at": "2026-05-05T08:06:09.831404874Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=a998917abae2ae276dd4cf08a159caef5f215264\nbase_rev=f01c29ea89ea4983e648136c9143f62722b08496",
+          "created_at": "2026-05-05T08:06:10.029344361Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T10:37:31.686548138Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T102429-9aa59978\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":14270434,\"output_tokens\":28364,\"total_tokens\":14298798,\"cost_usd\":0,\"duration_ms\":780518,\"exit_code\":0}",
+          "created_at": "2026-05-05T10:37:31.910323243Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=14298798 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T10:37:37.536042917Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=d58f47a1d017d49554b36325ebaca8a8bc4577e3\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T06:42:42-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=14459\noutput_bytes=0\nelapsed_ms=4214",
+          "created_at": "2026-05-05T10:37:42.295637815Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=d58f47a1d017d49554b36325ebaca8a8bc4577e3\nbase_rev=4f615584a16f733e92a5dfe16404b8c5f54d6133",
+          "created_at": "2026-05-05T10:37:42.489464903Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T12:42:42.047068524Z",
+      "execute-loop-last-detail": "agent exited without a commit or no_changes_rationale.txt; dirty paths: cli/cmd/agent_cmd.go, cli/cmd/agent_route_status.go, cli/cmd/agent_route_status_test.go, cli/cmd/agent_usage.go, cli/internal/server/providers.go, cli/internal/server/providers_test.go",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-05-05T07:52:13Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T124242-583923ab",
+    "prompt": ".ddx/executions/20260505T124242-583923ab/prompt.md",
+    "manifest": ".ddx/executions/20260505T124242-583923ab/manifest.json",
+    "result": ".ddx/executions/20260505T124242-583923ab/result.json",
+    "checks": ".ddx/executions/20260505T124242-583923ab/checks.json",
+    "usage": ".ddx/executions/20260505T124242-583923ab/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-4d81bb26-20260505T124242-583923ab"
+  },
+  "prompt_sha": "3a78ec465b09a3751a1f4e544bf2f465c447f0455268933ac5a1ad2d65cdea69"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T124242-583923ab/result.json b/.ddx/executions/20260505T124242-583923ab/result.json
new file mode 100644
index 00000000..a39e253d
--- /dev/null
+++ b/.ddx/executions/20260505T124242-583923ab/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-4d81bb26",
+  "attempt_id": "20260505T124242-583923ab",
+  "base_rev": "8a4a4bbd8b13d5fc0282c2e93e06ca6262848387",
+  "result_rev": "f387edb89af68f7453cb9548c61aa56bf641e1d3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-16a18bec",
+  "duration_ms": 368375,
+  "tokens": 4926315,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T124242-583923ab",
+  "prompt_file": ".ddx/executions/20260505T124242-583923ab/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T124242-583923ab/manifest.json",
+  "result_file": ".ddx/executions/20260505T124242-583923ab/result.json",
+  "usage_file": ".ddx/executions/20260505T124242-583923ab/usage.json",
+  "started_at": "2026-05-05T12:42:44.382367046Z",
+  "finished_at": "2026-05-05T12:48:52.757548165Z"
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
