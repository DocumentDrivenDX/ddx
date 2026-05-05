<bead-review>
  <bead id=".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7" iter=1>
    <title>bead(axon): add changeEvents websocket subscription transport</title>
    <description>
PROBLEM
TD-030 requires a GraphQL subscription client for Axon changeEvents, but cli/internal/bead/axon_backend.go:20-26 still only documents a local JSONL emulation. There is no transport helper in cli/internal/bead/axon/ to open a live subscription stream against Axons WebSocket endpoint.

ROOT CAUSE
cli/internal/bead/axon_backend.go:20-26 keeps the backend in-process, so the package currently has no websocket transport boundary where GraphQL subscription setup, event decoding, and stream lifecycle management can live.

PROPOSED FIX
- Add a small subscription transport in cli/internal/bead/axon/ that opens a GraphQL WebSocket connection and subscribes to changeEvents.
- Define a narrow stream helper and event decoding path in the same package so the transport stays isolated from backend_axon.go.
- Add a focused subscription test that uses the helper to receive ordered changeEvents from a test endpoint.

NON-SCOPE
- Schema snapshot and genqlient binding generation.
- Replacing backend_axon.go with a production wire-path implementation.
- Axon server-side subscription schema work.
    </description>
    <acceptance>
1. cli/internal/bead/axon/ contains a websocket subscription helper for changeEvents.
2. TestAxonSubscription_ChangeEventsStream is added in cli/internal/bead/axon/ and receives ordered events from the helper.
3. The transport is wired through the package helper used by the test, not left as dead code.
4. cd cli &amp;&amp; go test ./internal/bead/axon/... passes.
5. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2,  area:beads,  area:storage,  kind:feature,  blocked-on-upstream:axon-05c1019d,  blocked-on-upstream:axon-82b6f7b2,  spec:TD-030</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T103824-7d0186fe/manifest.json</file>
    <file>.ddx/executions/20260505T103824-7d0186fe/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1a5e45f4aecdbf13d2f1e7afff61a837f303e98a">
diff --git a/.ddx/executions/20260505T103824-7d0186fe/manifest.json b/.ddx/executions/20260505T103824-7d0186fe/manifest.json
new file mode 100644
index 00000000..ef30a803
--- /dev/null
+++ b/.ddx/executions/20260505T103824-7d0186fe/manifest.json
@@ -0,0 +1,123 @@
+{
+  "attempt_id": "20260505T103824-7d0186fe",
+  "bead_id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7",
+  "base_rev": "39e3dacaccc4dff18b327447c81489ec10d62b81",
+  "created_at": "2026-05-05T10:38:26.353194087Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7",
+    "title": "bead(axon): add changeEvents websocket subscription transport",
+    "description": "PROBLEM\nTD-030 requires a GraphQL subscription client for Axon changeEvents, but cli/internal/bead/axon_backend.go:20-26 still only documents a local JSONL emulation. There is no transport helper in cli/internal/bead/axon/ to open a live subscription stream against Axons WebSocket endpoint.\n\nROOT CAUSE\ncli/internal/bead/axon_backend.go:20-26 keeps the backend in-process, so the package currently has no websocket transport boundary where GraphQL subscription setup, event decoding, and stream lifecycle management can live.\n\nPROPOSED FIX\n- Add a small subscription transport in cli/internal/bead/axon/ that opens a GraphQL WebSocket connection and subscribes to changeEvents.\n- Define a narrow stream helper and event decoding path in the same package so the transport stays isolated from backend_axon.go.\n- Add a focused subscription test that uses the helper to receive ordered changeEvents from a test endpoint.\n\nNON-SCOPE\n- Schema snapshot and genqlient binding generation.\n- Replacing backend_axon.go with a production wire-path implementation.\n- Axon server-side subscription schema work.\n",
+    "acceptance": "1. cli/internal/bead/axon/ contains a websocket subscription helper for changeEvents.\n2. TestAxonSubscription_ChangeEventsStream is added in cli/internal/bead/axon/ and receives ordered events from the helper.\n3. The transport is wired through the package helper used by the test, not left as dead code.\n4. cd cli \u0026\u0026 go test ./internal/bead/axon/... passes.\n5. lefthook run pre-commit passes.",
+    "parent": "ddx-8d747049",
+    "labels": [
+      "phase:2",
+      " area:beads",
+      " area:storage",
+      " kind:feature",
+      " blocked-on-upstream:axon-05c1019d",
+      " blocked-on-upstream:axon-82b6f7b2",
+      " spec:TD-030"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T10:38:24Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T03:17:35.924409424Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T031641-18b3d274\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":408898,\"output_tokens\":5411,\"total_tokens\":414309,\"cost_usd\":0,\"duration_ms\":52561,\"exit_code\":0}",
+          "created_at": "2026-05-05T03:17:36.162456078Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=414309 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T03:17:42.9372406Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=e82b2d653ed7d3b3f02413f9709ed886ef600d91\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-04T23:22:47-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=8731\noutput_bytes=0\nelapsed_ms=4116",
+          "created_at": "2026-05-05T03:17:47.581198184Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=e82b2d653ed7d3b3f02413f9709ed886ef600d91\nbase_rev=fca6bc829d0a8ec4aea96ae3d6b4cc96b6d89442",
+          "created_at": "2026-05-05T03:17:47.807837679Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T03:44:33.331444375Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T034233-3242e317\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":1298141,\"output_tokens\":10411,\"total_tokens\":1308552,\"cost_usd\":0,\"duration_ms\":117297,\"exit_code\":0}",
+          "created_at": "2026-05-05T03:44:33.562547662Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=1308552 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T03:44:39.107190678Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=1a25a6a09dd04f71c63dafe02f2cfe8c229eb5b7\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-04T23:49:43-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=11057\noutput_bytes=0\nelapsed_ms=4156",
+          "created_at": "2026-05-05T03:44:43.829494582Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=1a25a6a09dd04f71c63dafe02f2cfe8c229eb5b7\nbase_rev=ce7ca21674432f4bb97fd1a9d7da1ee9eec31893",
+          "created_at": "2026-05-05T03:44:44.057444206Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T10:38:24.377973154Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T103824-7d0186fe",
+    "prompt": ".ddx/executions/20260505T103824-7d0186fe/prompt.md",
+    "manifest": ".ddx/executions/20260505T103824-7d0186fe/manifest.json",
+    "result": ".ddx/executions/20260505T103824-7d0186fe/result.json",
+    "checks": ".ddx/executions/20260505T103824-7d0186fe/checks.json",
+    "usage": ".ddx/executions/20260505T103824-7d0186fe/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7-20260505T103824-7d0186fe"
+  },
+  "prompt_sha": "0b42f2db86e4681071a6bbbba1ba30b8fca818ede0cbd7a701ee6c57d2678824"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T103824-7d0186fe/result.json b/.ddx/executions/20260505T103824-7d0186fe/result.json
new file mode 100644
index 00000000..144e5d76
--- /dev/null
+++ b/.ddx/executions/20260505T103824-7d0186fe/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": ".execute-bead-wt-.execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398-20260505T024017-db84a9cd-f2dae5c7",
+  "attempt_id": "20260505T103824-7d0186fe",
+  "base_rev": "39e3dacaccc4dff18b327447c81489ec10d62b81",
+  "result_rev": "35faaa8845554a40b76196c3d6533b8cd3e7a51b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-e7eecc3c",
+  "duration_ms": 64152,
+  "tokens": 504464,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T103824-7d0186fe",
+  "prompt_file": ".ddx/executions/20260505T103824-7d0186fe/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T103824-7d0186fe/manifest.json",
+  "result_file": ".ddx/executions/20260505T103824-7d0186fe/result.json",
+  "usage_file": ".ddx/executions/20260505T103824-7d0186fe/usage.json",
+  "started_at": "2026-05-05T10:38:26.35357192Z",
+  "finished_at": "2026-05-05T10:39:30.506470401Z"
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
