<bead-review>
  <bead id="ddx-755f5881" iter=1>
    <title>routing-default-path: dispatch and ddx-work issue ResolveRoute(Profile: default) with no synthesis</title>
    <description>
Cover D1 + D2 from ddx-fdd3ea36. The default ddx agent run / ddx work / ddx agent execute-bead path must call agentlib.ResolveRoute(Profile: spec.Profile, no Model/Harness/Provider) exactly once and pass the result through. workerDispatchAdapter.DispatchWorker treats empty input as {Profile: "default"} explicitly. No per-tier iteration, no ResolveTierModelRef lookup, no preference-order fallback on the default path. Tier-ladder machinery becomes opt-in via --escalate only (kept reachable, not deleted). Touched files (expected): cli/internal/agent/dispatch.go, cli/internal/agent/execute_bead_loop.go, cli/internal/agent/profile_ladder.go, cli/internal/server/graphql_adapters.go (workerDispatchAdapter), cli/cmd/agent_run.go, cli/cmd/work.go.
    </description>
    <acceptance>
1. Unit test: workerDispatchAdapter.DispatchWorker({}) → spec with Profile: "default", no other fields. 2. Unit test: default ddx agent run path calls ResolveRoute exactly once with (Profile: "default"). 3. Unit test: ResolveTierModelRef and ResolveProfileLadder are NOT in the call graph on the default path (verified via test seam or call-counting fake). 4. Integration test: same config that caused the historical 19-burn drain-queue failure now either succeeds or returns a single typed error — never 19-burn. 5. --escalate flag still routes through the tier-ladder machinery (regression coverage). 6. No changes to config schema in this bead — that lives in routing-config-deprecation.
    </acceptance>
    <labels>feat-006, routing</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T025051-28afce7c/manifest.json</file>
    <file>.ddx/executions/20260429T025051-28afce7c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="354a717ef54f429003b463299f88d20e5134a30b">
diff --git a/.ddx/executions/20260429T025051-28afce7c/manifest.json b/.ddx/executions/20260429T025051-28afce7c/manifest.json
new file mode 100644
index 00000000..c477f211
--- /dev/null
+++ b/.ddx/executions/20260429T025051-28afce7c/manifest.json
@@ -0,0 +1,62 @@
+{
+  "attempt_id": "20260429T025051-28afce7c",
+  "bead_id": "ddx-755f5881",
+  "base_rev": "2b3ecb8fd682681e60a184d29276554dbfc4c9ad",
+  "created_at": "2026-04-29T02:50:51.772920789Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-755f5881",
+    "title": "routing-default-path: dispatch and ddx-work issue ResolveRoute(Profile: default) with no synthesis",
+    "description": "Cover D1 + D2 from ddx-fdd3ea36. The default ddx agent run / ddx work / ddx agent execute-bead path must call agentlib.ResolveRoute(Profile: spec.Profile, no Model/Harness/Provider) exactly once and pass the result through. workerDispatchAdapter.DispatchWorker treats empty input as {Profile: \"default\"} explicitly. No per-tier iteration, no ResolveTierModelRef lookup, no preference-order fallback on the default path. Tier-ladder machinery becomes opt-in via --escalate only (kept reachable, not deleted). Touched files (expected): cli/internal/agent/dispatch.go, cli/internal/agent/execute_bead_loop.go, cli/internal/agent/profile_ladder.go, cli/internal/server/graphql_adapters.go (workerDispatchAdapter), cli/cmd/agent_run.go, cli/cmd/work.go.",
+    "acceptance": "1. Unit test: workerDispatchAdapter.DispatchWorker({}) → spec with Profile: \"default\", no other fields. 2. Unit test: default ddx agent run path calls ResolveRoute exactly once with (Profile: \"default\"). 3. Unit test: ResolveTierModelRef and ResolveProfileLadder are NOT in the call graph on the default path (verified via test seam or call-counting fake). 4. Integration test: same config that caused the historical 19-burn drain-queue failure now either succeeds or returns a single typed error — never 19-burn. 5. --escalate flag still routes through the tier-ladder machinery (regression coverage). 6. No changes to config schema in this bead — that lives in routing-config-deprecation.",
+    "parent": "ddx-fdd3ea36",
+    "labels": [
+      "feat-006",
+      "routing"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T02:50:51Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-29T02:48:59.770853892Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260429T004606-2efb6a16\",\"harness\":\"claude\",\"input_tokens\":110,\"output_tokens\":44806,\"total_tokens\":44916,\"cost_usd\":9.251122249999998,\"duration_ms\":7363383,\"exit_code\":1}",
+          "created_at": "2026-04-29T02:48:59.856141354Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=44916 cost_usd=9.2511"
+        },
+        {
+          "actor": "ddx",
+          "body": "exit status 1\nresult_rev=4c2c822bc6b93c55c3f3b991d136269531574f13\nbase_rev=4c2c822bc6b93c55c3f3b991d136269531574f13\nretry_after=2026-04-29T08:49:04Z",
+          "created_at": "2026-04-29T02:49:04.499893801Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-29T02:50:51.204751582Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T025051-28afce7c",
+    "prompt": ".ddx/executions/20260429T025051-28afce7c/prompt.md",
+    "manifest": ".ddx/executions/20260429T025051-28afce7c/manifest.json",
+    "result": ".ddx/executions/20260429T025051-28afce7c/result.json",
+    "checks": ".ddx/executions/20260429T025051-28afce7c/checks.json",
+    "usage": ".ddx/executions/20260429T025051-28afce7c/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-755f5881-20260429T025051-28afce7c"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T025051-28afce7c/result.json b/.ddx/executions/20260429T025051-28afce7c/result.json
new file mode 100644
index 00000000..5dc2c7a6
--- /dev/null
+++ b/.ddx/executions/20260429T025051-28afce7c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-755f5881",
+  "attempt_id": "20260429T025051-28afce7c",
+  "base_rev": "2b3ecb8fd682681e60a184d29276554dbfc4c9ad",
+  "result_rev": "2d1e590388c4e76cbe70427167ffd5e5ce6938e4",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-182309ee",
+  "duration_ms": 1249942,
+  "tokens": 45958,
+  "cost_usd": 8.675351249999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T025051-28afce7c",
+  "prompt_file": ".ddx/executions/20260429T025051-28afce7c/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T025051-28afce7c/manifest.json",
+  "result_file": ".ddx/executions/20260429T025051-28afce7c/result.json",
+  "usage_file": ".ddx/executions/20260429T025051-28afce7c/usage.json",
+  "started_at": "2026-04-29T02:50:51.773185414Z",
+  "finished_at": "2026-04-29T03:11:41.715442223Z"
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
