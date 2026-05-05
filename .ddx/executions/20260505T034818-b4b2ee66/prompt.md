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
decomposed into .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-26f338ad (real Axon backend operations on generated GraphQL client), .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-3fa0f047 (backend selection and .ddx config wiring), .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-1a576b9d (parameterized conformance suites across jsonl and axon), and .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-4fa37db2 (WebSocket subscription integration smoke test)
    </notes>
    <labels>phase:2, area:beads, area:storage, kind:feature, blocked-on-upstream:axon-05c1019d, blocked-on-upstream:axon-82b6f7b2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T034647-b37449e4/manifest.json</file>
    <file>.ddx/executions/20260505T034647-b37449e4/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="159f7acd491e2a9a63ddbf4ac03d7dccb0da626a">
diff --git a/.ddx/executions/20260505T034647-b37449e4/manifest.json b/.ddx/executions/20260505T034647-b37449e4/manifest.json
new file mode 100644
index 00000000..8f8d0e0f
--- /dev/null
+++ b/.ddx/executions/20260505T034647-b37449e4/manifest.json
@@ -0,0 +1,141 @@
+{
+  "attempt_id": "20260505T034647-b37449e4",
+  "bead_id": "ddx-8d747049",
+  "base_rev": "19642bd7a723993150c8dd996007827ca863c21e",
+  "created_at": "2026-05-05T03:46:49.796724266Z",
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
+      "claimed-at": "2026-05-05T03:46:47Z",
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
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T02:40:05.752542898Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T023636-7669921a\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":2634513,\"output_tokens\":25315,\"total_tokens\":2659828,\"cost_usd\":0,\"duration_ms\":207490,\"exit_code\":0}",
+          "created_at": "2026-05-05T02:40:05.966027935Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2659828 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T02:40:11.418473539Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=3fc3269708d8c322e66a56475fade55cfb086cbf\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-04T22:45:16-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=8617\noutput_bytes=0\nelapsed_ms=4188",
+          "created_at": "2026-05-05T02:40:16.298946544Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=3fc3269708d8c322e66a56475fade55cfb086cbf\nbase_rev=d2151beaae1c1b742df1110a4c3876ecbb1b0089",
+          "created_at": "2026-05-05T02:40:16.511528539Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T03:31:08.943317645Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T032827-6f86ee82\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":2219560,\"output_tokens\":10805,\"total_tokens\":2230365,\"cost_usd\":0,\"duration_ms\":159683,\"exit_code\":0}",
+          "created_at": "2026-05-05T03:31:09.190336263Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2230365 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T03:31:15.780342855Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=0d97a23e7c21674fc7b3eb9a7c422d1a2ded6221\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-04T23:36:20-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=11456\noutput_bytes=0\nelapsed_ms=4148",
+          "created_at": "2026-05-05T03:31:20.439028375Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=0d97a23e7c21674fc7b3eb9a7c422d1a2ded6221\nbase_rev=998230d93bbcb8bf4c992a7ca8f6ae76862cf2bd",
+          "created_at": "2026-05-05T03:31:20.661854455Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T03:46:47.809373979Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-02T20:57:28Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T034647-b37449e4",
+    "prompt": ".ddx/executions/20260505T034647-b37449e4/prompt.md",
+    "manifest": ".ddx/executions/20260505T034647-b37449e4/manifest.json",
+    "result": ".ddx/executions/20260505T034647-b37449e4/result.json",
+    "checks": ".ddx/executions/20260505T034647-b37449e4/checks.json",
+    "usage": ".ddx/executions/20260505T034647-b37449e4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8d747049-20260505T034647-b37449e4"
+  },
+  "prompt_sha": "3387f0b621b41447f0f0c993d00fbd7a1e0370007ee6782e0d97a817a0472465"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T034647-b37449e4/result.json b/.ddx/executions/20260505T034647-b37449e4/result.json
new file mode 100644
index 00000000..8d81373b
--- /dev/null
+++ b/.ddx/executions/20260505T034647-b37449e4/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-8d747049",
+  "attempt_id": "20260505T034647-b37449e4",
+  "base_rev": "19642bd7a723993150c8dd996007827ca863c21e",
+  "result_rev": "cfe5b635b14f3d031738c1a65fe823c94724210c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-cdf677de",
+  "duration_ms": 81897,
+  "tokens": 824122,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T034647-b37449e4",
+  "prompt_file": ".ddx/executions/20260505T034647-b37449e4/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T034647-b37449e4/manifest.json",
+  "result_file": ".ddx/executions/20260505T034647-b37449e4/result.json",
+  "usage_file": ".ddx/executions/20260505T034647-b37449e4/usage.json",
+  "started_at": "2026-05-05T03:46:49.797093682Z",
+  "finished_at": "2026-05-05T03:48:11.694705021Z"
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
