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
    <file>.ddx/executions/20260506T005752-1f271178/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T005752-1f271178/manifest.json</file>
    <file>.ddx/executions/20260506T005752-1f271178/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7d7bd1fabbb0b2bc4668f3f950245ca9d283a6f3">
<untrusted-data>
diff --git a/.ddx/executions/20260506T005752-1f271178/checks/production-reachability.json b/.ddx/executions/20260506T005752-1f271178/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T005752-1f271178/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T005752-1f271178/manifest.json b/.ddx/executions/20260506T005752-1f271178/manifest.json
new file mode 100644
index 00000000..4f717479
--- /dev/null
+++ b/.ddx/executions/20260506T005752-1f271178/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260506T005752-1f271178",
+  "bead_id": "ddx-06eb05d8",
+  "base_rev": "57be05187e5532cb4bafd4b99b8c6d454f45e53c",
+  "created_at": "2026-05-06T00:57:54.480085097Z",
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
+      "claimed-at": "2026-05-06T00:57:52Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T00:57:52.066863527Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T005752-1f271178",
+    "prompt": ".ddx/executions/20260506T005752-1f271178/prompt.md",
+    "manifest": ".ddx/executions/20260506T005752-1f271178/manifest.json",
+    "result": ".ddx/executions/20260506T005752-1f271178/result.json",
+    "checks": ".ddx/executions/20260506T005752-1f271178/checks.json",
+    "usage": ".ddx/executions/20260506T005752-1f271178/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-06eb05d8-20260506T005752-1f271178"
+  },
+  "prompt_sha": "cecfcf707398a4c91ebf70057b821c673627d4b3bb4fc83980284e26c23811d8"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T005752-1f271178/result.json b/.ddx/executions/20260506T005752-1f271178/result.json
new file mode 100644
index 00000000..f7bdf4c8
--- /dev/null
+++ b/.ddx/executions/20260506T005752-1f271178/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-06eb05d8",
+  "attempt_id": "20260506T005752-1f271178",
+  "base_rev": "57be05187e5532cb4bafd4b99b8c6d454f45e53c",
+  "result_rev": "5335d0625d10c2dfbcd964f475c32ea04b6d0e3c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-2daa4e72",
+  "duration_ms": 1049699,
+  "tokens": 13332011,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T005752-1f271178",
+  "prompt_file": ".ddx/executions/20260506T005752-1f271178/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T005752-1f271178/manifest.json",
+  "result_file": ".ddx/executions/20260506T005752-1f271178/result.json",
+  "usage_file": ".ddx/executions/20260506T005752-1f271178/usage.json",
+  "started_at": "2026-05-06T00:57:54.480688767Z",
+  "finished_at": "2026-05-06T01:15:24.180292549Z"
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
