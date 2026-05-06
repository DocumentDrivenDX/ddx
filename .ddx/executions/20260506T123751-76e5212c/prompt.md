<bead-review>
  <bead id="ddx-958b8fc3" iter=1>
    <title>bead/axon: parameterize conformance suites across JSONL and Axon</title>
    <description>
PROBLEM
The package-level conformance coverage still needs a single matrix that proves both backends satisfy the same behavioral contract. The current chaos harness already names jsonl and axon in cli/internal/bead/chaos_test.go:23-36, but the rest of the package still relies on ad hoc store construction and has no package-wide backend matrix to prove the new Axon path behaves identically.

ROOT CAUSE
cli/internal/bead/chaos_test.go:23-36 only defines the backend table; there is no end-to-end package test matrix yet that runs the conformance suite through both backends and keeps the existing package tests honest. The rest of the test package still defaults to newTestStore(t) in many places, so the Axon path is not yet a first-class test target.

PROPOSED FIX
- Extend the backend test helpers so the package conformance suite can run under both jsonl and axon.
- Keep TestChaos_ConcurrentAppendSafety, TestChaos_AtomicStatusTransitions, TestChaos_ConcurrentCloseAndAppend, and TestChaos_ConcurrentCloseNotLost as explicit backend matrix coverage.
- Make the package test harness clearly fail if either backend diverges.

NON-SCOPE
- GraphQL client generation.
- Backend selection/config plumbing.
- Subscription integration smoke tests.
    </description>
    <acceptance>
1. TestChaos_ConcurrentAppendSafety, TestChaos_AtomicStatusTransitions, TestChaos_ConcurrentCloseAndAppend, and TestChaos_ConcurrentCloseNotLost run under both jsonl and axon subtests.
2. The package conformance helpers make the backend selector explicit instead of silently defaulting in only one path.
3. cd cli &amp;&amp; go test ./internal/bead/... passes.
4. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:feature, blocked-on-upstream:axon-05c1019d, blocked-on-upstream:axon-82b6f7b2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T123426-3fce3c4b/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T123426-3fce3c4b/manifest.json</file>
    <file>.ddx/executions/20260506T123426-3fce3c4b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="749578c98897dfcccb99edc00990afa9e68ada21">
<untrusted-data>
diff --git a/.ddx/executions/20260506T123426-3fce3c4b/checks/production-reachability.json b/.ddx/executions/20260506T123426-3fce3c4b/checks/production-reachability.json
new file mode 100644
index 000000000..89e732516
--- /dev/null
+++ b/.ddx/executions/20260506T123426-3fce3c4b/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no non-test Go files changed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T123426-3fce3c4b/manifest.json b/.ddx/executions/20260506T123426-3fce3c4b/manifest.json
new file mode 100644
index 000000000..6c614a955
--- /dev/null
+++ b/.ddx/executions/20260506T123426-3fce3c4b/manifest.json
@@ -0,0 +1,82 @@
+{
+  "attempt_id": "20260506T123426-3fce3c4b",
+  "bead_id": "ddx-958b8fc3",
+  "base_rev": "e1d4ef06df81c60792bed1f5b1d95b85d7453605",
+  "created_at": "2026-05-06T12:34:29.112600364Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-958b8fc3",
+    "title": "bead/axon: parameterize conformance suites across JSONL and Axon",
+    "description": "PROBLEM\nThe package-level conformance coverage still needs a single matrix that proves both backends satisfy the same behavioral contract. The current chaos harness already names jsonl and axon in cli/internal/bead/chaos_test.go:23-36, but the rest of the package still relies on ad hoc store construction and has no package-wide backend matrix to prove the new Axon path behaves identically.\n\nROOT CAUSE\ncli/internal/bead/chaos_test.go:23-36 only defines the backend table; there is no end-to-end package test matrix yet that runs the conformance suite through both backends and keeps the existing package tests honest. The rest of the test package still defaults to newTestStore(t) in many places, so the Axon path is not yet a first-class test target.\n\nPROPOSED FIX\n- Extend the backend test helpers so the package conformance suite can run under both jsonl and axon.\n- Keep TestChaos_ConcurrentAppendSafety, TestChaos_AtomicStatusTransitions, TestChaos_ConcurrentCloseAndAppend, and TestChaos_ConcurrentCloseNotLost as explicit backend matrix coverage.\n- Make the package test harness clearly fail if either backend diverges.\n\nNON-SCOPE\n- GraphQL client generation.\n- Backend selection/config plumbing.\n- Subscription integration smoke tests.",
+    "acceptance": "1. TestChaos_ConcurrentAppendSafety, TestChaos_AtomicStatusTransitions, TestChaos_ConcurrentCloseAndAppend, and TestChaos_ConcurrentCloseNotLost run under both jsonl and axon subtests.\n2. The package conformance helpers make the backend selector explicit instead of silently defaulting in only one path.\n3. cd cli \u0026\u0026 go test ./internal/bead/... passes.\n4. lefthook run pre-commit passes.",
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
+      "claimed-at": "2026-05-06T12:34:26Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T12:11:35.853913418Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T120844-d2a3f066\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":1670655,\"output_tokens\":10807,\"total_tokens\":1681462,\"cost_usd\":0,\"duration_ms\":168678,\"exit_code\":0}",
+          "created_at": "2026-05-06T12:11:36.081467204Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=1681462 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T12:11:43.216669978Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=8c30b521f1e6ed7e1ecae16d7cb86a9f05b6170c\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T08:16:47-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=9031\noutput_bytes=0\nelapsed_ms=4163",
+          "created_at": "2026-05-06T12:11:47.913977262Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=8c30b521f1e6ed7e1ecae16d7cb86a9f05b6170c\nbase_rev=be7e546b1fe2902a8a429bca7786b9fc072f6df9",
+          "created_at": "2026-05-06T12:11:48.123763561Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T12:34:26.233800599Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T123426-3fce3c4b",
+    "prompt": ".ddx/executions/20260506T123426-3fce3c4b/prompt.md",
+    "manifest": ".ddx/executions/20260506T123426-3fce3c4b/manifest.json",
+    "result": ".ddx/executions/20260506T123426-3fce3c4b/result.json",
+    "checks": ".ddx/executions/20260506T123426-3fce3c4b/checks.json",
+    "usage": ".ddx/executions/20260506T123426-3fce3c4b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-958b8fc3-20260506T123426-3fce3c4b"
+  },
+  "prompt_sha": "75684a2142d0f117f2117376b3f2b24e103e521a292818ffce480693dd6e1089"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T123426-3fce3c4b/result.json b/.ddx/executions/20260506T123426-3fce3c4b/result.json
new file mode 100644
index 000000000..bb42ca7bd
--- /dev/null
+++ b/.ddx/executions/20260506T123426-3fce3c4b/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-958b8fc3",
+  "attempt_id": "20260506T123426-3fce3c4b",
+  "base_rev": "e1d4ef06df81c60792bed1f5b1d95b85d7453605",
+  "result_rev": "81f2afcb5f4ef0870b8e6a2a5e7a20aade28295b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-478fcfa5",
+  "duration_ms": 195185,
+  "tokens": 1890497,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T123426-3fce3c4b",
+  "prompt_file": ".ddx/executions/20260506T123426-3fce3c4b/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T123426-3fce3c4b/manifest.json",
+  "result_file": ".ddx/executions/20260506T123426-3fce3c4b/result.json",
+  "usage_file": ".ddx/executions/20260506T123426-3fce3c4b/usage.json",
+  "started_at": "2026-05-06T12:34:29.11296753Z",
+  "finished_at": "2026-05-06T12:37:44.298337899Z"
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
