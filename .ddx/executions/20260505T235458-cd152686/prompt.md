<bead-review>
  <bead id="ddx-8dd19492" iter=1>
    <title>bead/axon: add WebSocket subscription integration smoke test</title>
    <description>
PROBLEM
TD-030 requires a live GraphQL subscription path so DDx can subscribe to Axon changeEvents for bead and bead-event updates. The current test suite in cli/internal/bead/axon_backend_test.go covers CRUD, deps, events, and import/export, but there is no subscription smoke test yet.

ROOT CAUSE
cli/internal/bead/axon_backend_test.go:131-179 verifies the split event collection, but nothing in the package opens a WebSocket subscription and asserts that changeEvents are delivered in order. That leaves the live-update path unexercised.

PROPOSED FIX
- Add an integration test in cli/internal/bead/axon/ that opens the subscription endpoint, performs a small bead mutation sequence, and asserts the expected changeEvents arrive.
- Reuse the generated Axon client and subscription transport from the client-scaffolding bead.
- Keep the test isolated and deterministic so it can run in the normal package suite.

NON-SCOPE
- Core backend CRUD logic.
- Config selection.
- Package-wide backend matrix work.
    </description>
    <acceptance>
1. TestAxonSubscription_ChangeEventsStream or TestAxonSubscription_ReceivesBeadMutations is added and passes.
2. The subscription smoke test uses the generated Axon client and a WebSocket transport.
3. cd cli &amp;&amp; go test ./internal/bead/axon/... passes.
4. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:feature, blocked-on-upstream:axon-05c1019d, blocked-on-upstream:axon-82b6f7b2, triage:no-changes-unjustified</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T235342-f9f337d1/manifest.json</file>
    <file>.ddx/executions/20260505T235342-f9f337d1/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="608509b8b2cfe6161172e26d83f0d21401ab5317">
<untrusted-data>
diff --git a/.ddx/executions/20260505T235342-f9f337d1/manifest.json b/.ddx/executions/20260505T235342-f9f337d1/manifest.json
new file mode 100644
index 00000000..8ae180e5
--- /dev/null
+++ b/.ddx/executions/20260505T235342-f9f337d1/manifest.json
@@ -0,0 +1,118 @@
+{
+  "attempt_id": "20260505T235342-f9f337d1",
+  "bead_id": "ddx-8dd19492",
+  "base_rev": "b474c58c2ecbc0c6ec1afb992cbec813d87d5635",
+  "created_at": "2026-05-05T23:53:44.569506863Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-8dd19492",
+    "title": "bead/axon: add WebSocket subscription integration smoke test",
+    "description": "PROBLEM\nTD-030 requires a live GraphQL subscription path so DDx can subscribe to Axon changeEvents for bead and bead-event updates. The current test suite in cli/internal/bead/axon_backend_test.go covers CRUD, deps, events, and import/export, but there is no subscription smoke test yet.\n\nROOT CAUSE\ncli/internal/bead/axon_backend_test.go:131-179 verifies the split event collection, but nothing in the package opens a WebSocket subscription and asserts that changeEvents are delivered in order. That leaves the live-update path unexercised.\n\nPROPOSED FIX\n- Add an integration test in cli/internal/bead/axon/ that opens the subscription endpoint, performs a small bead mutation sequence, and asserts the expected changeEvents arrive.\n- Reuse the generated Axon client and subscription transport from the client-scaffolding bead.\n- Keep the test isolated and deterministic so it can run in the normal package suite.\n\nNON-SCOPE\n- Core backend CRUD logic.\n- Config selection.\n- Package-wide backend matrix work.",
+    "acceptance": "1. TestAxonSubscription_ChangeEventsStream or TestAxonSubscription_ReceivesBeadMutations is added and passes.\n2. The subscription smoke test uses the generated Axon client and a WebSocket transport.\n3. cd cli \u0026\u0026 go test ./internal/bead/axon/... passes.\n4. lefthook run pre-commit passes.",
+    "parent": "ddx-8d747049",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "blocked-on-upstream:axon-05c1019d",
+      "blocked-on-upstream:axon-82b6f7b2",
+      "triage:no-changes-unjustified"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T23:53:42Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "427120",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T16:52:20.53617391Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T164739-a33aec7f\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":4533604,\"output_tokens\":29071,\"total_tokens\":4562675,\"cost_usd\":0,\"duration_ms\":278545,\"exit_code\":0}",
+          "created_at": "2026-05-05T16:52:20.759334353Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=4562675 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T16:52:28.484579137Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=ea616762add0f929c6e23d62cebcbcace2f1fe8e\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T12:57:33-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=8307\noutput_bytes=0\nelapsed_ms=4200",
+          "created_at": "2026-05-05T16:52:33.208699318Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=ea616762add0f929c6e23d62cebcbcace2f1fe8e\nbase_rev=993a7ad0e6b7386fc6ed62d2213c1ae6aed5d13b",
+          "created_at": "2026-05-05T16:52:33.413563658Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T18:18:37.911515893Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T181743-e9d18262\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":312742,\"output_tokens\":4928,\"total_tokens\":317670,\"cost_usd\":0,\"duration_ms\":51496,\"exit_code\":0}",
+          "created_at": "2026-05-05T18:18:38.164840691Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=317670 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "(rationale absent)",
+          "created_at": "2026-05-05T18:18:38.945415888Z",
+          "kind": "no_changes_unjustified",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes_unjustified"
+        },
+        {
+          "actor": "erik",
+          "body": "agent exited without a commit or no_changes_rationale.txt\nresult_rev=3258419cba4b4f5736fbe3fee49e046e7402b07a\nbase_rev=3258419cba4b4f5736fbe3fee49e046e7402b07a\nretry_after=2026-05-06T00:18:39Z",
+          "created_at": "2026-05-05T18:18:39.624678899Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T23:53:42.009360177Z",
+      "execute-loop-last-detail": "agent exited without a commit or no_changes_rationale.txt",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T235342-f9f337d1",
+    "prompt": ".ddx/executions/20260505T235342-f9f337d1/prompt.md",
+    "manifest": ".ddx/executions/20260505T235342-f9f337d1/manifest.json",
+    "result": ".ddx/executions/20260505T235342-f9f337d1/result.json",
+    "checks": ".ddx/executions/20260505T235342-f9f337d1/checks.json",
+    "usage": ".ddx/executions/20260505T235342-f9f337d1/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8dd19492-20260505T235342-f9f337d1"
+  },
+  "prompt_sha": "93090391654c00023a5830ac3d39dc1f5800629a248cf815575b711520f3b802"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T235342-f9f337d1/result.json b/.ddx/executions/20260505T235342-f9f337d1/result.json
new file mode 100644
index 00000000..2cb5850e
--- /dev/null
+++ b/.ddx/executions/20260505T235342-f9f337d1/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-8dd19492",
+  "attempt_id": "20260505T235342-f9f337d1",
+  "base_rev": "b474c58c2ecbc0c6ec1afb992cbec813d87d5635",
+  "result_rev": "b8deb90557532838ba48c498474cda7c1d2b7e5a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-2217477a",
+  "duration_ms": 67677,
+  "tokens": 638529,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T235342-f9f337d1",
+  "prompt_file": ".ddx/executions/20260505T235342-f9f337d1/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T235342-f9f337d1/manifest.json",
+  "result_file": ".ddx/executions/20260505T235342-f9f337d1/result.json",
+  "usage_file": ".ddx/executions/20260505T235342-f9f337d1/usage.json",
+  "started_at": "2026-05-05T23:53:44.569934029Z",
+  "finished_at": "2026-05-05T23:54:52.247818287Z"
+}
\ No newline at end of file
</untrusted-data>
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
