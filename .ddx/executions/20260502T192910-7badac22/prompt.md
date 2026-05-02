<bead-review>
  <bead id="ddx-8f1ac866" iter=1>
    <title>fizeau: import agent-3bb96bf5 release once tagged (Role + CorrelationID + RoutingActual.Power)</title>
    <description>
Cross-repo coordination bead. Upstream Fizeau work is filed at agent-3bb96bf5 (epic) in /home/erik/Projects/agent. That epic adds Role + CorrelationID to ExecuteRequest + RouteRequest and Power int to ServiceRoutingActual.

This bead's scope: (1) wait for agent-3bb96bf5 to ship a tagged release; (2) bump cli/go.mod to import that tagged version (NEVER go.mod replace per project memory); (3) verify Role / CorrelationID / RoutingActual.Power are accessible from DDx Go code.
    </description>
    <acceptance>
1. agent-3bb96bf5 tagged release identified (e.g., fizeau v0.10.0). 2. cli/go.mod updated to that tagged version. 3. cd cli &amp;&amp; go build ./... succeeds. 4. cd cli &amp;&amp; go test ./... passes (no test consumes the new fields yet — that's S10_2/3/5). 5. A trivial Go file references ExecuteRequest.Role and ServiceRoutingActual.Power to confirm symbols resolve.
    </acceptance>
    <notes>
Upstream Fizeau v0.10.0 tagged and released 2026-05-02 by agent-744cd55e. Release: https://github.com/DocumentDrivenDX/fizeau/releases/tag/v0.10.0. Bump cli/go.mod from v0.9.29 to v0.10.0; new fields ExecuteRequest.Role/CorrelationID, RouteRequest.Role/CorrelationID, ServiceRoutingActual.Power, RouteDecision.Power; reserved metadata keys role/correlation_id; metadata_key_collision warning on final events. Strictly additive — no caller change required to compile.
    </notes>
    <labels>phase:2, story:10, area:upstream, kind:coordination, blocked-on-upstream</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T192418-46bfb2b7/manifest.json</file>
    <file>.ddx/executions/20260502T192418-46bfb2b7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3d0d6617ac049f6772c334f5ac82c2cd223229bf">
diff --git a/.ddx/executions/20260502T192418-46bfb2b7/manifest.json b/.ddx/executions/20260502T192418-46bfb2b7/manifest.json
new file mode 100644
index 00000000..d918b110
--- /dev/null
+++ b/.ddx/executions/20260502T192418-46bfb2b7/manifest.json
@@ -0,0 +1,44 @@
+{
+  "attempt_id": "20260502T192418-46bfb2b7",
+  "bead_id": "ddx-8f1ac866",
+  "base_rev": "4a8ed0ac58aca2fe8a8fcd6b36bfc35e05d6b938",
+  "created_at": "2026-05-02T19:24:19.911920025Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-8f1ac866",
+    "title": "fizeau: import agent-3bb96bf5 release once tagged (Role + CorrelationID + RoutingActual.Power)",
+    "description": "Cross-repo coordination bead. Upstream Fizeau work is filed at agent-3bb96bf5 (epic) in /home/erik/Projects/agent. That epic adds Role + CorrelationID to ExecuteRequest + RouteRequest and Power int to ServiceRoutingActual.\n\nThis bead's scope: (1) wait for agent-3bb96bf5 to ship a tagged release; (2) bump cli/go.mod to import that tagged version (NEVER go.mod replace per project memory); (3) verify Role / CorrelationID / RoutingActual.Power are accessible from DDx Go code.",
+    "acceptance": "1. agent-3bb96bf5 tagged release identified (e.g., fizeau v0.10.0). 2. cli/go.mod updated to that tagged version. 3. cd cli \u0026\u0026 go build ./... succeeds. 4. cd cli \u0026\u0026 go test ./... passes (no test consumes the new fields yet — that's S10_2/3/5). 5. A trivial Go file references ExecuteRequest.Role and ServiceRoutingActual.Power to confirm symbols resolve.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:upstream",
+      "kind:coordination",
+      "blocked-on-upstream"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T19:24:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3446284",
+      "closing_commit_sha": "b9225a48c1b97fdcf8c6f36fbf5eb7c9c465f9d5",
+      "events_attachment": "ddx-8f1ac866/events.jsonl",
+      "execute-loop-heartbeat-at": "2026-05-02T19:24:18.638181347Z",
+      "execute-loop-no-changes-count": 2,
+      "session_id": "eb-fa4c2e81"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T192418-46bfb2b7",
+    "prompt": ".ddx/executions/20260502T192418-46bfb2b7/prompt.md",
+    "manifest": ".ddx/executions/20260502T192418-46bfb2b7/manifest.json",
+    "result": ".ddx/executions/20260502T192418-46bfb2b7/result.json",
+    "checks": ".ddx/executions/20260502T192418-46bfb2b7/checks.json",
+    "usage": ".ddx/executions/20260502T192418-46bfb2b7/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8f1ac866-20260502T192418-46bfb2b7"
+  },
+  "prompt_sha": "88e1969addc373c0247fd774eefa8c003a9721683509523658f01ac8d43b3679"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T192418-46bfb2b7/result.json b/.ddx/executions/20260502T192418-46bfb2b7/result.json
new file mode 100644
index 00000000..449b951b
--- /dev/null
+++ b/.ddx/executions/20260502T192418-46bfb2b7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-8f1ac866",
+  "attempt_id": "20260502T192418-46bfb2b7",
+  "base_rev": "4a8ed0ac58aca2fe8a8fcd6b36bfc35e05d6b938",
+  "result_rev": "66e12170165253145fc5895b5baec14da1ef7bc0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-536fe3b0",
+  "duration_ms": 286967,
+  "tokens": 7550,
+  "cost_usd": 1.02404425,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T192418-46bfb2b7",
+  "prompt_file": ".ddx/executions/20260502T192418-46bfb2b7/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T192418-46bfb2b7/manifest.json",
+  "result_file": ".ddx/executions/20260502T192418-46bfb2b7/result.json",
+  "usage_file": ".ddx/executions/20260502T192418-46bfb2b7/usage.json",
+  "started_at": "2026-05-02T19:24:19.912232108Z",
+  "finished_at": "2026-05-02T19:29:06.879969085Z"
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
