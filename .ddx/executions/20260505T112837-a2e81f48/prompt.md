<bead-review>
  <bead id="ddx-b69f04f8" iter=1>
    <title>federation: spoke lifecycle — idempotent register, jittered heartbeat, URL-change handling</title>
    <description>
PROBLEM
When a spoke node starts with --hub-address, it must register with the hub and maintain a heartbeat so the hub's FederationRegistry knows the spoke is alive. Today no --hub-address flag exists and no spoke lifecycle (register / heartbeat / stale detection / URL-change re-register) is implemented.

ROOT CAUSE
- cli/internal/server/server.go: no --hub-address flag wired.
- cli/internal/federation/: no spoke-side registration client or heartbeat loop exists.
- The hub's FederationRegistry (from ddx-90f48012, a dep) provides the register endpoint and stale detection, but the spoke side has no code to call it.
- Per locked decisions from the federation planning: register is idempotent on stable node_id; duplicate node_id rejected (not silently merged); heartbeat jittered at 30s; stale threshold at 2 minutes.

PROPOSED FIX
- Add --hub-address string flag to the server command (cli/cmd/server.go or equivalent).
- Add cli/internal/federation/spoke_lifecycle.go:
  - RegisterWithHub(ctx, hubAddress, nodeID, localURL): idempotent POST to hub register endpoint.
  - StartHeartbeat(ctx, hubAddress, nodeID): jittered 30s heartbeat loop; on failure → stale detection.
  - HandleURLChange(ctx, hubAddress, nodeID, newURL): re-register when local URL flips.
- Stale threshold: hub marks spoke offline after 2 minutes without heartbeat.

NON-SCOPE
- Hub-side registry implementation (ddx-90f48012, a dep).
- Federation query fan-out (existing fanout.go).
- Mutation routing (S15-7b / ddx-eb75a32d).
    </description>
    <acceptance>
1. --hub-address flag wired to server command and passed to spoke lifecycle on startup.
2. RegisterWithHub: idempotent (re-registration with same node_id does not duplicate); duplicate node_id (different node) hard-rejected with error.
3. StartHeartbeat: heartbeat interval jittered around 30s.
4. Hub marks spoke as offline after 2 minutes without heartbeat.
5. HandleURLChange: re-registers when local URL differs from registered URL.
6. TestSpokeLifecycle_Register_Idempotent verifies re-registration is a no-op.
7. TestSpokeLifecycle_DuplicateNodeID_Rejected verifies distinct node with same ID is rejected.
8. TestSpokeLifecycle_Heartbeat_30sJitter verifies heartbeat fires within [25s, 35s] window.
9. TestSpokeLifecycle_URLChange_Triggers_Reregister verifies URL flip causes re-register.
10. cd cli &amp;&amp; go test ./internal/server/... ./internal/federation/... green.
11. lefthook run pre-commit passes.
    </acceptance>
    <notes>
REVIEW:BLOCK

Diff contains only execution evidence (manifest.json, result.json) — no source code changes are visible. Cannot evaluate any of the 6 acceptance criteria from this diff alone.
    </notes>
    <labels>phase:2, story:14, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T112641-d3692c0c/manifest.json</file>
    <file>.ddx/executions/20260505T112641-d3692c0c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="eaaf639699d8a1b61eabc58bbc04e9791baa5104">
diff --git a/.ddx/executions/20260505T112641-d3692c0c/manifest.json b/.ddx/executions/20260505T112641-d3692c0c/manifest.json
new file mode 100644
index 00000000..b2c380d2
--- /dev/null
+++ b/.ddx/executions/20260505T112641-d3692c0c/manifest.json
@@ -0,0 +1,147 @@
+{
+  "attempt_id": "20260505T112641-d3692c0c",
+  "bead_id": "ddx-b69f04f8",
+  "base_rev": "68b152e5224ea61b9463a9816b2ea4f8ea8a5c16",
+  "created_at": "2026-05-05T11:26:43.894001778Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b69f04f8",
+    "title": "federation: spoke lifecycle — idempotent register, jittered heartbeat, URL-change handling",
+    "description": "PROBLEM\nWhen a spoke node starts with --hub-address, it must register with the hub and maintain a heartbeat so the hub's FederationRegistry knows the spoke is alive. Today no --hub-address flag exists and no spoke lifecycle (register / heartbeat / stale detection / URL-change re-register) is implemented.\n\nROOT CAUSE\n- cli/internal/server/server.go: no --hub-address flag wired.\n- cli/internal/federation/: no spoke-side registration client or heartbeat loop exists.\n- The hub's FederationRegistry (from ddx-90f48012, a dep) provides the register endpoint and stale detection, but the spoke side has no code to call it.\n- Per locked decisions from the federation planning: register is idempotent on stable node_id; duplicate node_id rejected (not silently merged); heartbeat jittered at 30s; stale threshold at 2 minutes.\n\nPROPOSED FIX\n- Add --hub-address string flag to the server command (cli/cmd/server.go or equivalent).\n- Add cli/internal/federation/spoke_lifecycle.go:\n  - RegisterWithHub(ctx, hubAddress, nodeID, localURL): idempotent POST to hub register endpoint.\n  - StartHeartbeat(ctx, hubAddress, nodeID): jittered 30s heartbeat loop; on failure → stale detection.\n  - HandleURLChange(ctx, hubAddress, nodeID, newURL): re-register when local URL flips.\n- Stale threshold: hub marks spoke offline after 2 minutes without heartbeat.\n\nNON-SCOPE\n- Hub-side registry implementation (ddx-90f48012, a dep).\n- Federation query fan-out (existing fanout.go).\n- Mutation routing (S15-7b / ddx-eb75a32d).",
+    "acceptance": "1. --hub-address flag wired to server command and passed to spoke lifecycle on startup.\n2. RegisterWithHub: idempotent (re-registration with same node_id does not duplicate); duplicate node_id (different node) hard-rejected with error.\n3. StartHeartbeat: heartbeat interval jittered around 30s.\n4. Hub marks spoke as offline after 2 minutes without heartbeat.\n5. HandleURLChange: re-registers when local URL differs from registered URL.\n6. TestSpokeLifecycle_Register_Idempotent verifies re-registration is a no-op.\n7. TestSpokeLifecycle_DuplicateNodeID_Rejected verifies distinct node with same ID is rejected.\n8. TestSpokeLifecycle_Heartbeat_30sJitter verifies heartbeat fires within [25s, 35s] window.\n9. TestSpokeLifecycle_URLChange_Triggers_Reregister verifies URL flip causes re-register.\n10. cd cli \u0026\u0026 go test ./internal/server/... ./internal/federation/... green.\n11. lefthook run pre-commit passes.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14",
+      "area:server",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T11:26:41Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-03T19:32:38.558654112Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-03T19:32:43.883907014Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "erik",
+          "body": "Diff contains only execution evidence (manifest.json, result.json) — no source code changes are visible. Cannot evaluate any of the 6 acceptance criteria from this diff alone.\nharness=claude\nmodel=opus\ninput_bytes=5725\noutput_bytes=526\nelapsed_ms=67752",
+          "created_at": "2026-05-03T19:33:53.809419174Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-05-03T19:33:53.910440303Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"action\":\"re_attempt_with_context\",\"mode\":\"review_block\"}",
+          "created_at": "2026-05-03T19:33:54.003834832Z",
+          "kind": "triage-decision",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block: re_attempt_with_context"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff contains only execution evidence (manifest.json, result.json) — no source code changes are visible. Cannot evaluate any of the 6 acceptance criteria from this diff alone.\nresult_rev=2865f51fa2ea2acb2a6bcc5789b84f0e61565ae7\nbase_rev=942f5e4ba24c97ec8d6acb80c7d5f750465b52b2",
+          "created_at": "2026-05-03T19:33:54.139727346Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T21:55:55.8679181Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=2ade1a0fc515a023390bf4f639b4a7d0a17506ef\nbase_rev=2ade1a0fc515a023390bf4f639b4a7d0a17506ef\nretry_after=2026-05-05T03:55:56Z",
+          "created_at": "2026-05-04T21:55:56.613314746Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T05:22:31.959729936Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T051509-8f87ac41\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":8803178,\"output_tokens\":19058,\"total_tokens\":8822236,\"cost_usd\":0,\"duration_ms\":440434,\"exit_code\":0}",
+          "created_at": "2026-05-05T05:22:32.198634154Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=8822236 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T05:22:39.638269968Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=59a526780f6df5887a67d6b1c02599a18b264db1\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T01:27:44-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=14063\noutput_bytes=0\nelapsed_ms=4194",
+          "created_at": "2026-05-05T05:22:44.37796635Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=59a526780f6df5887a67d6b1c02599a18b264db1\nbase_rev=a48f970b3d66e30ce0a3a2eb320d853beb88f289",
+          "created_at": "2026-05-05T05:22:44.600737995Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T11:26:41.894987177Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T03:55:56Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T112641-d3692c0c",
+    "prompt": ".ddx/executions/20260505T112641-d3692c0c/prompt.md",
+    "manifest": ".ddx/executions/20260505T112641-d3692c0c/manifest.json",
+    "result": ".ddx/executions/20260505T112641-d3692c0c/result.json",
+    "checks": ".ddx/executions/20260505T112641-d3692c0c/checks.json",
+    "usage": ".ddx/executions/20260505T112641-d3692c0c/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b69f04f8-20260505T112641-d3692c0c"
+  },
+  "prompt_sha": "2e2be547b86db4342ffd260d3c312f3377024ffd3154fd1fc385ded7b1972caf"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T112641-d3692c0c/result.json b/.ddx/executions/20260505T112641-d3692c0c/result.json
new file mode 100644
index 00000000..f00a6df6
--- /dev/null
+++ b/.ddx/executions/20260505T112641-d3692c0c/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-b69f04f8",
+  "attempt_id": "20260505T112641-d3692c0c",
+  "base_rev": "68b152e5224ea61b9463a9816b2ea4f8ea8a5c16",
+  "result_rev": "f1f9aca974739709744df935a76f16d69f236bd7",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-edce6a3e",
+  "duration_ms": 108157,
+  "tokens": 1152442,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T112641-d3692c0c",
+  "prompt_file": ".ddx/executions/20260505T112641-d3692c0c/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T112641-d3692c0c/manifest.json",
+  "result_file": ".ddx/executions/20260505T112641-d3692c0c/result.json",
+  "usage_file": ".ddx/executions/20260505T112641-d3692c0c/usage.json",
+  "started_at": "2026-05-05T11:26:43.894406611Z",
+  "finished_at": "2026-05-05T11:28:32.051671289Z"
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
