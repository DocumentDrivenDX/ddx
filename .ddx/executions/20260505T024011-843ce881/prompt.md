<bead-review>
  <bead id="ddx-8d747049" iter=1>
    <title>axon backend: prototype implementation via genqlient-generated GraphQL Go client</title>
    <description>
Per the TD from B_TD: implement the axon backend in cli/internal/bead/backend_axon.go (or per the existing backend.go shape). Use genqlient to generate a typed Go client from Axon's GraphQL introspection schema — no protoc/protobuf toolchain. Wire as a selectable backend (config flag .ddx/config.yaml backend: axon|jsonl|bd). Default stays jsonl Day 1. Existing tests must pass against both backends.

Subscription support: implement a subscription client (genqlient supports subscriptions via separate library, or use a simple WebSocket GraphQL subscriptions client) so that DDx server can subscribe to Axon ChangeEvents on ddx_beads / ddx_bead_events for live UI updates.
    </description>
    <acceptance>
1. backend_axon.go lands. 2. genqlient-generated client at cli/internal/bead/axon/. 3. Selectable via .ddx/config.yaml. 4. Default unchanged (jsonl). 5. cd cli &amp;&amp; go test ./internal/bead/... passes against both backends (parameterized test). 6. chaos_test.go suite passes against axon backend. 7. WebSocket subscription client present and exercised by an integration test.
    </acceptance>
    <notes>
decomposed into .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398, .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-26f338ad, .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-3fa0f047, .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-1a576b9d, .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-4fa37db2
    </notes>
    <labels>phase:2, area:beads, area:storage, kind:feature, blocked-on-upstream:axon-05c1019d, blocked-on-upstream:axon-82b6f7b2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T023636-7669921a/manifest.json</file>
    <file>.ddx/executions/20260505T023636-7669921a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3fc3269708d8c322e66a56475fade55cfb086cbf">
diff --git a/.ddx/executions/20260505T023636-7669921a/manifest.json b/.ddx/executions/20260505T023636-7669921a/manifest.json
new file mode 100644
index 00000000..b726c2cb
--- /dev/null
+++ b/.ddx/executions/20260505T023636-7669921a/manifest.json
@@ -0,0 +1,61 @@
+{
+  "attempt_id": "20260505T023636-7669921a",
+  "bead_id": "ddx-8d747049",
+  "base_rev": "d2151beaae1c1b742df1110a4c3876ecbb1b0089",
+  "created_at": "2026-05-05T02:36:38.259108931Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-8d747049",
+    "title": "axon backend: prototype implementation via genqlient-generated GraphQL Go client",
+    "description": "Per the TD from B_TD: implement the axon backend in cli/internal/bead/backend_axon.go (or per the existing backend.go shape). Use genqlient to generate a typed Go client from Axon's GraphQL introspection schema — no protoc/protobuf toolchain. Wire as a selectable backend (config flag .ddx/config.yaml backend: axon|jsonl|bd). Default stays jsonl Day 1. Existing tests must pass against both backends.\n\nSubscription support: implement a subscription client (genqlient supports subscriptions via separate library, or use a simple WebSocket GraphQL subscriptions client) so that DDx server can subscribe to Axon ChangeEvents on ddx_beads / ddx_bead_events for live UI updates.",
+    "acceptance": "1. backend_axon.go lands. 2. genqlient-generated client at cli/internal/bead/axon/. 3. Selectable via .ddx/config.yaml. 4. Default unchanged (jsonl). 5. cd cli \u0026\u0026 go test ./internal/bead/... passes against both backends (parameterized test). 6. chaos_test.go suite passes against axon backend. 7. WebSocket subscription client present and exercised by an integration test.",
+    "parent": "ddx-5d49b14e",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "blocked-on-upstream:axon-05c1019d",
+      "blocked-on-upstream:axon-82b6f7b2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T02:36:36Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-02T14:57:28.122200947Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=1e317a67b9fde512d6602b7a0ed521638c650210\nbase_rev=1e317a67b9fde512d6602b7a0ed521638c650210\nretry_after=2026-05-02T20:57:28Z",
+          "created_at": "2026-05-02T14:57:28.223620125Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T02:36:36.221020149Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-02T20:57:28Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T023636-7669921a",
+    "prompt": ".ddx/executions/20260505T023636-7669921a/prompt.md",
+    "manifest": ".ddx/executions/20260505T023636-7669921a/manifest.json",
+    "result": ".ddx/executions/20260505T023636-7669921a/result.json",
+    "checks": ".ddx/executions/20260505T023636-7669921a/checks.json",
+    "usage": ".ddx/executions/20260505T023636-7669921a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a"
+  },
+  "prompt_sha": "5f25bde53401ef7819211a96329f65f6cdffab3fc8398fc70e2c35199c4ee866"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T023636-7669921a/result.json b/.ddx/executions/20260505T023636-7669921a/result.json
new file mode 100644
index 00000000..4051904b
--- /dev/null
+++ b/.ddx/executions/20260505T023636-7669921a/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-8d747049",
+  "attempt_id": "20260505T023636-7669921a",
+  "base_rev": "d2151beaae1c1b742df1110a4c3876ecbb1b0089",
+  "result_rev": "a6f8dc762090b0c90b600807a5b41bb4f41196e0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-24a2e9eb",
+  "duration_ms": 207490,
+  "tokens": 2659828,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T023636-7669921a",
+  "prompt_file": ".ddx/executions/20260505T023636-7669921a/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T023636-7669921a/manifest.json",
+  "result_file": ".ddx/executions/20260505T023636-7669921a/result.json",
+  "usage_file": ".ddx/executions/20260505T023636-7669921a/usage.json",
+  "started_at": "2026-05-05T02:36:38.25942768Z",
+  "finished_at": "2026-05-05T02:40:05.749672318Z"
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
