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
    <labels>phase:2, area:beads, area:storage, kind:feature, blocked-on-upstream:axon-05c1019d, blocked-on-upstream:axon-82b6f7b2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T164739-a33aec7f/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T164739-a33aec7f/manifest.json</file>
    <file>.ddx/executions/20260505T164739-a33aec7f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ea616762add0f929c6e23d62cebcbcace2f1fe8e">
diff --git a/.ddx/executions/20260505T164739-a33aec7f/checks/production-reachability.json b/.ddx/executions/20260505T164739-a33aec7f/checks/production-reachability.json
new file mode 100644
index 00000000..89e73251
--- /dev/null
+++ b/.ddx/executions/20260505T164739-a33aec7f/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no non-test Go files changed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T164739-a33aec7f/manifest.json b/.ddx/executions/20260505T164739-a33aec7f/manifest.json
new file mode 100644
index 00000000..671ba812
--- /dev/null
+++ b/.ddx/executions/20260505T164739-a33aec7f/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260505T164739-a33aec7f",
+  "bead_id": "ddx-8dd19492",
+  "base_rev": "993a7ad0e6b7386fc6ed62d2213c1ae6aed5d13b",
+  "created_at": "2026-05-05T16:47:41.987541973Z",
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
+      "blocked-on-upstream:axon-82b6f7b2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T16:47:39Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "execute-loop-heartbeat-at": "2026-05-05T16:47:39.319847681Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T164739-a33aec7f",
+    "prompt": ".ddx/executions/20260505T164739-a33aec7f/prompt.md",
+    "manifest": ".ddx/executions/20260505T164739-a33aec7f/manifest.json",
+    "result": ".ddx/executions/20260505T164739-a33aec7f/result.json",
+    "checks": ".ddx/executions/20260505T164739-a33aec7f/checks.json",
+    "usage": ".ddx/executions/20260505T164739-a33aec7f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8dd19492-20260505T164739-a33aec7f"
+  },
+  "prompt_sha": "89251377544339e2e53f0a38136b684571ffbe263228c7d534c3f8ada601cf41"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T164739-a33aec7f/result.json b/.ddx/executions/20260505T164739-a33aec7f/result.json
new file mode 100644
index 00000000..87573274
--- /dev/null
+++ b/.ddx/executions/20260505T164739-a33aec7f/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-8dd19492",
+  "attempt_id": "20260505T164739-a33aec7f",
+  "base_rev": "993a7ad0e6b7386fc6ed62d2213c1ae6aed5d13b",
+  "result_rev": "aabc0b3152cb60202a9abd01ba819a91a280a1ba",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-2402a3e5",
+  "duration_ms": 278545,
+  "tokens": 4562675,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T164739-a33aec7f",
+  "prompt_file": ".ddx/executions/20260505T164739-a33aec7f/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T164739-a33aec7f/manifest.json",
+  "result_file": ".ddx/executions/20260505T164739-a33aec7f/result.json",
+  "usage_file": ".ddx/executions/20260505T164739-a33aec7f/usage.json",
+  "started_at": "2026-05-05T16:47:41.987981681Z",
+  "finished_at": "2026-05-05T16:52:20.533729788Z"
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
