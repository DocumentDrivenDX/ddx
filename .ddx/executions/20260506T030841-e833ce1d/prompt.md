<bead-review>
  <bead id="ddx-06eb05d8" iter=1>
    <title>C7: uniform Guard contract + delete attempted/hookFailed maps + pick() derives from drain.result</title>
    <description>
PROBLEM
  The drain loop tracks attempted and hookFailed beads via ad-hoc in-memory maps rather than a structured Guard contract, making the loop body untestable in isolation and the skip policy implicit.

ROOT CAUSE
  cli/internal/agent/execute_bead_loop.go:403-413 declares attempted := make(map[string]struct{}) and hookFailed := make(map[string]struct{}). Lines 534-544 manually clear both maps between sleep cycles. Lines 591-601 implement the two-strikes pre-claim hook policy by mutating these maps inline. nextCandidate (line 1216) accepts attempted as a parameter and checks membership at line 1223. No Guard interface exists; the pre-claim logic and the complexity gate each embed their skip logic directly in the loop body.

PROPOSED FIX
  - Define Guard interface in cli/internal/agent/work/guard.go: Allow(ctx context.Context, beadID string) (bool, reason string).
  - Implement two concrete Guards: PreClaimGuard (wraps runtime.PreClaimHook two-strikes logic) and ComplexityGuard (wraps w.ComplexityGate nil-check).
  - Delete attempted and hookFailed maps from execute_bead_loop.go:403-413. Replace nextCandidate's attempted parameter with drain.result.Results slice (already populated per-iteration): filter beads whose ID appears in Results.
  - Pre-claim hook denial: call w.Store.SetExecutionCooldown(candidate.ID, 30s, ...) instead of recording in hookFailed.
  - Add TestGuard_PreClaim_TwoStrikesSkips and TestGuard_Complexity_NilGateWarnsOnce to a new cli/internal/agent/work/guard_test.go.

NON-SCOPE
  - Heartbeat extraction (C11, ddx-0b8780db).
  - File renames (C13, ddx-387a0178).
  - Any change to SetExecutionCooldown duration policy (controlled by caller constants).

INTERSECTIONS
- P1: guard denials should skip the candidate rather than wedge the queue.
- P2: each guard owns only its own skip policy.
- P4: a denied bead stays isolated from other beads.
    </description>
    <acceptance>
1. Guard interface in cli/internal/agent/work/guard.go with Allow(ctx context.Context, beadID string) (bool, string) signature.
2. PreClaimGuard and ComplexityGuard implement Guard; loop body passes them to nextCandidate.
3. attempted and hookFailed maps deleted from cli/internal/agent/execute_bead_loop.go:403-413; no references remain.
4. nextCandidate signature no longer accepts attempted map; filters via drain.result.Results instead.
5. Pre-claim hook denial calls w.Store.SetExecutionCooldown with 30s cooldown; hookFailed logic removed.
6. TestGuard_PreClaim_TwoStrikesSkips in cli/internal/agent/work/guard_test.go: two consecutive Allow calls with hook error → second returns false.
7. TestGuard_Complexity_NilGateWarnsOnce: nil ComplexityGate → one warning, allow proceeds.
8. Existing TestPreClaimHookDivergedSkipsBead in cli/internal/agent/execute_bead_preclaim_test.go still passes.
9. cd cli &amp;&amp; go test ./internal/agent/... ./internal/agent/work/... green.
10. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, refactor, kind:refactor</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T030402-e17ce643/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T030402-e17ce643/manifest.json</file>
    <file>.ddx/executions/20260506T030402-e17ce643/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b1dc0adf04a393ecde63a3e95d389e66f1166266">
<untrusted-data>
diff --git a/.ddx/executions/20260506T030402-e17ce643/checks/production-reachability.json b/.ddx/executions/20260506T030402-e17ce643/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T030402-e17ce643/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T030402-e17ce643/manifest.json b/.ddx/executions/20260506T030402-e17ce643/manifest.json
new file mode 100644
index 00000000..38519e3a
--- /dev/null
+++ b/.ddx/executions/20260506T030402-e17ce643/manifest.json
@@ -0,0 +1,79 @@
+{
+  "attempt_id": "20260506T030402-e17ce643",
+  "bead_id": "ddx-06eb05d8",
+  "base_rev": "5392aab48303aacae89580ede6b6a8afd209bb7d",
+  "created_at": "2026-05-06T03:04:05.574342631Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-06eb05d8",
+    "title": "C7: uniform Guard contract + delete attempted/hookFailed maps + pick() derives from drain.result",
+    "description": "PROBLEM\n  The drain loop tracks attempted and hookFailed beads via ad-hoc in-memory maps rather than a structured Guard contract, making the loop body untestable in isolation and the skip policy implicit.\n\nROOT CAUSE\n  cli/internal/agent/execute_bead_loop.go:403-413 declares attempted := make(map[string]struct{}) and hookFailed := make(map[string]struct{}). Lines 534-544 manually clear both maps between sleep cycles. Lines 591-601 implement the two-strikes pre-claim hook policy by mutating these maps inline. nextCandidate (line 1216) accepts attempted as a parameter and checks membership at line 1223. No Guard interface exists; the pre-claim logic and the complexity gate each embed their skip logic directly in the loop body.\n\nPROPOSED FIX\n  - Define Guard interface in cli/internal/agent/work/guard.go: Allow(ctx context.Context, beadID string) (bool, reason string).\n  - Implement two concrete Guards: PreClaimGuard (wraps runtime.PreClaimHook two-strikes logic) and ComplexityGuard (wraps w.ComplexityGate nil-check).\n  - Delete attempted and hookFailed maps from execute_bead_loop.go:403-413. Replace nextCandidate's attempted parameter with drain.result.Results slice (already populated per-iteration): filter beads whose ID appears in Results.\n  - Pre-claim hook denial: call w.Store.SetExecutionCooldown(candidate.ID, 30s, ...) instead of recording in hookFailed.\n  - Add TestGuard_PreClaim_TwoStrikesSkips and TestGuard_Complexity_NilGateWarnsOnce to a new cli/internal/agent/work/guard_test.go.\n\nNON-SCOPE\n  - Heartbeat extraction (C11, ddx-0b8780db).\n  - File renames (C13, ddx-387a0178).\n  - Any change to SetExecutionCooldown duration policy (controlled by caller constants).\n\nINTERSECTIONS\n- P1: guard denials should skip the candidate rather than wedge the queue.\n- P2: each guard owns only its own skip policy.\n- P4: a denied bead stays isolated from other beads.",
+    "acceptance": "1. Guard interface in cli/internal/agent/work/guard.go with Allow(ctx context.Context, beadID string) (bool, string) signature.\n2. PreClaimGuard and ComplexityGuard implement Guard; loop body passes them to nextCandidate.\n3. attempted and hookFailed maps deleted from cli/internal/agent/execute_bead_loop.go:403-413; no references remain.\n4. nextCandidate signature no longer accepts attempted map; filters via drain.result.Results instead.\n5. Pre-claim hook denial calls w.Store.SetExecutionCooldown with 30s cooldown; hookFailed logic removed.\n6. TestGuard_PreClaim_TwoStrikesSkips in cli/internal/agent/work/guard_test.go: two consecutive Allow calls with hook error → second returns false.\n7. TestGuard_Complexity_NilGateWarnsOnce: nil ComplexityGate → one warning, allow proceeds.\n8. Existing TestPreClaimHookDivergedSkipsBead in cli/internal/agent/execute_bead_preclaim_test.go still passes.\n9. cd cli \u0026\u0026 go test ./internal/agent/... ./internal/agent/work/... green.\n10. lefthook run pre-commit passes.",
+    "parent": "ddx-5cb6e6cd",
+    "labels": [
+      "phase:2",
+      "refactor",
+      "kind:refactor"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T03:04:02Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T01:15:24.182722714Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T005752-1f271178\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":13289821,\"output_tokens\":42190,\"total_tokens\":13332011,\"cost_usd\":0,\"duration_ms\":1049699,\"exit_code\":0}",
+          "created_at": "2026-05-06T01:15:24.410294523Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=13332011 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T01:15:30.858136676Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: unparseable\nattempt_count=1\nresult_rev=7d7bd1fabbb0b2bc4668f3f950245ca9d283a6f3\n\nreviewer: review-error: unparseable: reviewer output: unparseable JSON verdict: no JSON object found (raw output 66 bytes; see .ddx/executions/20260506T011531-465df8d9)\nharness=claude\nmodel=opus\ninput_bytes=11454\noutput_bytes=66\nelapsed_ms=48608",
+          "created_at": "2026-05-06T01:16:21.986232989Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: unparseable"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=7d7bd1fabbb0b2bc4668f3f950245ca9d283a6f3\nbase_rev=57be05187e5532cb4bafd4b99b8c6d454f45e53c",
+          "created_at": "2026-05-06T01:16:22.177974982Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T03:04:02.760525235Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T030402-e17ce643",
+    "prompt": ".ddx/executions/20260506T030402-e17ce643/prompt.md",
+    "manifest": ".ddx/executions/20260506T030402-e17ce643/manifest.json",
+    "result": ".ddx/executions/20260506T030402-e17ce643/result.json",
+    "checks": ".ddx/executions/20260506T030402-e17ce643/checks.json",
+    "usage": ".ddx/executions/20260506T030402-e17ce643/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-06eb05d8-20260506T030402-e17ce643"
+  },
+  "prompt_sha": "c7a3672212f12fbd485d2731d5781f5c1adbbbd16c11439c99a22c53f1c31ace"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T030402-e17ce643/result.json b/.ddx/executions/20260506T030402-e17ce643/result.json
new file mode 100644
index 00000000..03c40c7b
--- /dev/null
+++ b/.ddx/executions/20260506T030402-e17ce643/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-06eb05d8",
+  "attempt_id": "20260506T030402-e17ce643",
+  "base_rev": "5392aab48303aacae89580ede6b6a8afd209bb7d",
+  "result_rev": "404628dfc45b28cb0269935caf65e029b6b3f77a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-5d729a14",
+  "duration_ms": 268146,
+  "tokens": 2892158,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T030402-e17ce643",
+  "prompt_file": ".ddx/executions/20260506T030402-e17ce643/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T030402-e17ce643/manifest.json",
+  "result_file": ".ddx/executions/20260506T030402-e17ce643/result.json",
+  "usage_file": ".ddx/executions/20260506T030402-e17ce643/usage.json",
+  "started_at": "2026-05-06T03:04:05.574688964Z",
+  "finished_at": "2026-05-06T03:08:33.721215239Z"
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
