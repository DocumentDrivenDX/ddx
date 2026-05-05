<bead-review>
  <bead id=".execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398" iter=1>
    <title>bead: scaffold cli/internal/bead/axon genqlient client and subscriptions</title>
    <description>
PROBLEM
The Axon backend contract requires a generated GraphQL client plus a WebSocket subscription client, but the current implementation is an in-process JSONL emulation instead of a typed Axon package. The gap is visible in cli/internal/bead/axon_backend.go:15-26, which says the backend is a local emulation and not the wire-path implementation. There is no cli/internal/bead/axon/ package yet.

ROOT CAUSE
cli/internal/bead/axon_backend.go:20-26 explicitly keeps the backend as a JSONL-shaped emulation, so there is nowhere for genqlient-generated bindings, schema snapshots, query documents, or a subscription transport to live. The repo has no Axon client package to satisfy TD-030's GraphQL-only requirement.

PROPOSED FIX
- Create cli/internal/bead/axon/schema.graphql with the pinned introspection snapshot.
- Add query/mutation/subscription documents under cli/internal/bead/axon/queries/.
- Add genqlient config and generated Go bindings under cli/internal/bead/axon/.
- Add a small subscription transport in the same package for GraphQL WS changeEvents.

NON-SCOPE
- Replacing backend_axon.go itself; that is the follow-up implementation bead.
- Config routing and package-level test matrix changes.
- Axon server-side schema work.
    </description>
    <acceptance>
1. cli/internal/bead/axon/ exists with schema.graphql, query documents, genqlient config, and generated Go bindings.
2. TestAxonClient_SchemaBindingsCompile and TestAxonSubscription_ChangeEventsStream are added in cli/internal/bead/axon/.
3. cd cli &amp;&amp; go test ./internal/bead/axon/... passes.
4. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2,  area:beads,  area:storage,  kind:feature,  blocked-on-upstream:axon-05c1019d,  blocked-on-upstream:axon-82b6f7b2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T034111-51b1eba2/manifest.json</file>
    <file>.ddx/executions/20260505T034111-51b1eba2/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="87b30b7c2418c8bde95806ca643ea0b71eae46d7">
diff --git a/.ddx/executions/20260505T034111-51b1eba2/manifest.json b/.ddx/executions/20260505T034111-51b1eba2/manifest.json
new file mode 100644
index 00000000..ebdeae43
--- /dev/null
+++ b/.ddx/executions/20260505T034111-51b1eba2/manifest.json
@@ -0,0 +1,122 @@
+{
+  "attempt_id": "20260505T034111-51b1eba2",
+  "bead_id": ".execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398",
+  "base_rev": "0171421b090c9d41dd6ccb3abc707e2e93112e71",
+  "created_at": "2026-05-05T03:41:13.878527954Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398",
+    "title": "bead: scaffold cli/internal/bead/axon genqlient client and subscriptions",
+    "description": "PROBLEM\nThe Axon backend contract requires a generated GraphQL client plus a WebSocket subscription client, but the current implementation is an in-process JSONL emulation instead of a typed Axon package. The gap is visible in cli/internal/bead/axon_backend.go:15-26, which says the backend is a local emulation and not the wire-path implementation. There is no cli/internal/bead/axon/ package yet.\n\nROOT CAUSE\ncli/internal/bead/axon_backend.go:20-26 explicitly keeps the backend as a JSONL-shaped emulation, so there is nowhere for genqlient-generated bindings, schema snapshots, query documents, or a subscription transport to live. The repo has no Axon client package to satisfy TD-030's GraphQL-only requirement.\n\nPROPOSED FIX\n- Create cli/internal/bead/axon/schema.graphql with the pinned introspection snapshot.\n- Add query/mutation/subscription documents under cli/internal/bead/axon/queries/.\n- Add genqlient config and generated Go bindings under cli/internal/bead/axon/.\n- Add a small subscription transport in the same package for GraphQL WS changeEvents.\n\nNON-SCOPE\n- Replacing backend_axon.go itself; that is the follow-up implementation bead.\n- Config routing and package-level test matrix changes.\n- Axon server-side schema work.",
+    "acceptance": "1. cli/internal/bead/axon/ exists with schema.graphql, query documents, genqlient config, and generated Go bindings.\n2. TestAxonClient_SchemaBindingsCompile and TestAxonSubscription_ChangeEventsStream are added in cli/internal/bead/axon/.\n3. cd cli \u0026\u0026 go test ./internal/bead/axon/... passes.\n4. lefthook run pre-commit passes.",
+    "parent": "ddx-8d747049",
+    "labels": [
+      "phase:2",
+      " area:beads",
+      " area:storage",
+      " kind:feature",
+      " blocked-on-upstream:axon-05c1019d",
+      " blocked-on-upstream:axon-82b6f7b2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T03:41:11Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T02:41:53.956840754Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T024017-db84a9cd\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":873315,\"output_tokens\":10167,\"total_tokens\":883482,\"cost_usd\":0,\"duration_ms\":94607,\"exit_code\":0}",
+          "created_at": "2026-05-05T02:41:54.190399846Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=883482 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T02:41:59.376323452Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=d4c1a120fa4a5ea168bd7a1cb8cd8711856e0539\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-04T22:47:03-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=8623\noutput_bytes=0\nelapsed_ms=4178",
+          "created_at": "2026-05-05T02:42:04.072830698Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=d4c1a120fa4a5ea168bd7a1cb8cd8711856e0539\nbase_rev=feff7fa618c3bc501dae3f388b00f2b60f435dcb",
+          "created_at": "2026-05-05T02:42:04.303253252Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T03:15:22.727282483Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T031001-d0bb96a4\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":3626722,\"output_tokens\":31874,\"total_tokens\":3658596,\"cost_usd\":0,\"duration_ms\":319481,\"exit_code\":0}",
+          "created_at": "2026-05-05T03:15:22.97107976Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=3658596 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T03:15:30.121850281Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=4f6711e4f53859720ce51245ef164db1bbfb073d\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-04T23:20:34-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=11489\noutput_bytes=0\nelapsed_ms=4192",
+          "created_at": "2026-05-05T03:15:34.84691607Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=4f6711e4f53859720ce51245ef164db1bbfb073d\nbase_rev=aa27dad20b55805507ebe5da20ba917fe1d4bc7a",
+          "created_at": "2026-05-05T03:15:35.067149777Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T03:41:11.63168604Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T034111-51b1eba2",
+    "prompt": ".ddx/executions/20260505T034111-51b1eba2/prompt.md",
+    "manifest": ".ddx/executions/20260505T034111-51b1eba2/manifest.json",
+    "result": ".ddx/executions/20260505T034111-51b1eba2/result.json",
+    "checks": ".ddx/executions/20260505T034111-51b1eba2/checks.json",
+    "usage": ".ddx/executions/20260505T034111-51b1eba2/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T034111-51b1eba2"
+  },
+  "prompt_sha": "0743a730335018c17e00bb4d9c6f1eb7e2b5ca3dda81b81e0a30b46a3a46fffa"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T034111-51b1eba2/result.json b/.ddx/executions/20260505T034111-51b1eba2/result.json
new file mode 100644
index 00000000..eaa9526e
--- /dev/null
+++ b/.ddx/executions/20260505T034111-51b1eba2/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398",
+  "attempt_id": "20260505T034111-51b1eba2",
+  "base_rev": "0171421b090c9d41dd6ccb3abc707e2e93112e71",
+  "result_rev": "a5643e4f84e7235ce68294b88e9a3ec36f265468",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-90c0a027",
+  "duration_ms": 68516,
+  "tokens": 572745,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T034111-51b1eba2",
+  "prompt_file": ".ddx/executions/20260505T034111-51b1eba2/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T034111-51b1eba2/manifest.json",
+  "result_file": ".ddx/executions/20260505T034111-51b1eba2/result.json",
+  "usage_file": ".ddx/executions/20260505T034111-51b1eba2/usage.json",
+  "started_at": "2026-05-05T03:41:13.878928328Z",
+  "finished_at": "2026-05-05T03:42:22.39573027Z"
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
