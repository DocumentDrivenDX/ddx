<bead-review>
  <bead id=".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-28f710ee" iter=1>
    <title>ADR-022 step 5a: derived-view GraphQL workers query (backend)</title>
    <description>
Add a GraphQL query that surfaces the server's worker_ingest derived view (cli/internal/server/worker_ingest.go: workerIngestRegistry). Resolver reads from server runtime registry (s.workerIngest.snapshot()) and applies freshnessState() to compute connected/stale/disconnected. Fields exposed: id, project (from Identity.ProjectRoot), harness, state (freshness), last_event_at, mirror_failures_count, had_dropped_backfill, current_bead, current_attempt. The existing 'workers' query is for the in-process execute-loop registry; this is a parallel derived-view query (suggested name: reportedWorkers / workerReports). ~150 LOC backend including schema, resolver, model wiring, and gqlgen-regenerated code.
    </description>
    <acceptance>
1. New GraphQL query exposes id, project, harness, state, lastEventAt, mirrorFailuresCount, hadDroppedBackfill, currentBead, currentAttempt.
2. Resolver reads from s.workerIngest registry; freshness state computed via freshnessState().
3. TestGraphQL_Workers_FreshnessFields validates connected/stale/disconnected transitions over time using a fake clock or time-controlled record manipulation.
4. TestGraphQL_Workers_MultipleWorkersPerProject validates that two workers under the same projectRoot both appear (duplicate-worker visibility).
5. cd cli &amp;&amp; go test ./... passes; gqlgen-generated.go regenerated and consistent with schema; lefthook pre-commit passes.
    </acceptance>
    <labels>phase:2,  area:server,  area:graphql,  kind:feature,  adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T065739-f453b7fb/manifest.json</file>
    <file>.ddx/executions/20260503T065739-f453b7fb/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d3e8003844eef96e4eade648aa7a3fb2558cd314">
diff --git a/.ddx/executions/20260503T065739-f453b7fb/manifest.json b/.ddx/executions/20260503T065739-f453b7fb/manifest.json
new file mode 100644
index 00000000..b5945f89
--- /dev/null
+++ b/.ddx/executions/20260503T065739-f453b7fb/manifest.json
@@ -0,0 +1,130 @@
+{
+  "attempt_id": "20260503T065739-f453b7fb",
+  "bead_id": ".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-28f710ee",
+  "base_rev": "059f723d43fa5ac11d1975dc357d4dc6dbb722ce",
+  "created_at": "2026-05-03T06:57:41.016558463Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-28f710ee",
+    "title": "ADR-022 step 5a: derived-view GraphQL workers query (backend)",
+    "description": "Add a GraphQL query that surfaces the server's worker_ingest derived view (cli/internal/server/worker_ingest.go: workerIngestRegistry). Resolver reads from server runtime registry (s.workerIngest.snapshot()) and applies freshnessState() to compute connected/stale/disconnected. Fields exposed: id, project (from Identity.ProjectRoot), harness, state (freshness), last_event_at, mirror_failures_count, had_dropped_backfill, current_bead, current_attempt. The existing 'workers' query is for the in-process execute-loop registry; this is a parallel derived-view query (suggested name: reportedWorkers / workerReports). ~150 LOC backend including schema, resolver, model wiring, and gqlgen-regenerated code.",
+    "acceptance": "1. New GraphQL query exposes id, project, harness, state, lastEventAt, mirrorFailuresCount, hadDroppedBackfill, currentBead, currentAttempt.\n2. Resolver reads from s.workerIngest registry; freshness state computed via freshnessState().\n3. TestGraphQL_Workers_FreshnessFields validates connected/stale/disconnected transitions over time using a fake clock or time-controlled record manipulation.\n4. TestGraphQL_Workers_MultipleWorkersPerProject validates that two workers under the same projectRoot both appear (duplicate-worker visibility).\n5. cd cli \u0026\u0026 go test ./... passes; gqlgen-generated.go regenerated and consistent with schema; lefthook pre-commit passes.",
+    "parent": "ddx-7b753655",
+    "labels": [
+      "phase:2",
+      " area:server",
+      " area:graphql",
+      " kind:feature",
+      " adr:022"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T06:57:39Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:49:45.591819176Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:50:22.383554269Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:51:06.101988144Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:51:49.872743378Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:52:33.583045865Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:53:17.393364486Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:54:01.158928835Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:54:45.027150819Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:55:35.679995689Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:56:19.277635386Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T06:57:00.720747091Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-03T06:57:39.443085854Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T065739-f453b7fb",
+    "prompt": ".ddx/executions/20260503T065739-f453b7fb/prompt.md",
+    "manifest": ".ddx/executions/20260503T065739-f453b7fb/manifest.json",
+    "result": ".ddx/executions/20260503T065739-f453b7fb/result.json",
+    "checks": ".ddx/executions/20260503T065739-f453b7fb/checks.json",
+    "usage": ".ddx/executions/20260503T065739-f453b7fb/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-28f710ee-20260503T065739-f453b7fb"
+  },
+  "prompt_sha": "198145df283df1a356e0500e0470b49ef9a0016d810c08caccc6b40d3d8d091b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T065739-f453b7fb/result.json b/.ddx/executions/20260503T065739-f453b7fb/result.json
new file mode 100644
index 00000000..757e4015
--- /dev/null
+++ b/.ddx/executions/20260503T065739-f453b7fb/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-28f710ee",
+  "attempt_id": "20260503T065739-f453b7fb",
+  "base_rev": "059f723d43fa5ac11d1975dc357d4dc6dbb722ce",
+  "result_rev": "83ec78530887aa81ae6eb5db5bcaa0718aff7c0b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-8d56ce27",
+  "duration_ms": 1271877,
+  "tokens": 40157,
+  "cost_usd": 7.98828225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T065739-f453b7fb",
+  "prompt_file": ".ddx/executions/20260503T065739-f453b7fb/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T065739-f453b7fb/manifest.json",
+  "result_file": ".ddx/executions/20260503T065739-f453b7fb/result.json",
+  "usage_file": ".ddx/executions/20260503T065739-f453b7fb/usage.json",
+  "started_at": "2026-05-03T06:57:41.016933629Z",
+  "finished_at": "2026-05-03T07:18:52.894353853Z"
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
