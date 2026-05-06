<bead-review>
  <bead id="ddx-12638823" iter=1>
    <title>intake: add typed pre-claim actionability hook</title>
    <description>
PROBLEM
The intake parent bead ddx-f3bbcfce was too broad and bounced because it combined hook contracts, safe rewrites, decomposition, AC maps, depth overflow, and no-progress policy. The first executable slice is the typed pre-claim intake hook contract and loop insertion point.

ROOT CAUSE WITH FILE:LINE
- cli/internal/agent/execute_bead_loop.go:680 invokes PreDispatchLintHook before claim, but there is no typed PreClaimIntakeHook with the finalized intake outcomes.
- cli/internal/agent/lint_hook.go:86 constructs lint-only behavior, not actionability/decomposition decisions.

PROPOSED FIX
- Add PreClaimIntakeHook (or equivalent guard) to ExecuteBeadLoopRuntime.
- Define typed outcomes actionable_atomic, actionable_but_rewritten, too_large_decomposed, ambiguous_needs_human, and intake_error.
- Invoke the hook after ready selection/routing preflight and before Claim.
- Implement actionable_atomic pass-through and intake_error fail-open for infrastructure failures only.

NON-SCOPE
- Safe rewrite mutation.
- Child bead creation/decomposition.
- No-progress accounting changes beyond not counting intake_error as implementation no-progress.
    </description>
    <acceptance>
1. ExecuteBeadLoopRuntime exposes a pre-claim intake hook/guard with typed outcomes actionable_atomic, actionable_but_rewritten, too_large_decomposed, ambiguous_needs_human, and intake_error.
2. TestIntake_ActionableAtomic_ClaimsNormally verifies actionable_atomic proceeds to Claim and implementation.
3. TestIntake_InfrastructureErrorFailsOpen verifies intake_error from hook infrastructure records a warning event and does not count as implementation no-progress.
4. TestIntake_HookRunsBeforeClaim verifies the hook fires before Store.Claim and after route preflight.
5. cd cli &amp;&amp; go test ./internal/agent/... -run "TestIntake_.*(ActionableAtomic|InfrastructureError|HookRunsBeforeClaim)" -count=1 passes.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:10, area:agent, kind:feature, reliability, adr:023, adr:024, from:ddx-f3bbcfce</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T032956-8a3c2b28/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T032956-8a3c2b28/manifest.json</file>
    <file>.ddx/executions/20260506T032956-8a3c2b28/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="dc228cdf909f3df68325b714672127fdf9a1618a">
<untrusted-data>
diff --git a/.ddx/executions/20260506T032956-8a3c2b28/checks/production-reachability.json b/.ddx/executions/20260506T032956-8a3c2b28/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T032956-8a3c2b28/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T032956-8a3c2b28/manifest.json b/.ddx/executions/20260506T032956-8a3c2b28/manifest.json
new file mode 100644
index 00000000..7367e0b8
--- /dev/null
+++ b/.ddx/executions/20260506T032956-8a3c2b28/manifest.json
@@ -0,0 +1,43 @@
+{
+  "attempt_id": "20260506T032956-8a3c2b28",
+  "bead_id": "ddx-12638823",
+  "base_rev": "98c83d3cb803e655a9c1adcf79ba472a3e062ea8",
+  "created_at": "2026-05-06T03:29:59.044438326Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-12638823",
+    "title": "intake: add typed pre-claim actionability hook",
+    "description": "PROBLEM\nThe intake parent bead ddx-f3bbcfce was too broad and bounced because it combined hook contracts, safe rewrites, decomposition, AC maps, depth overflow, and no-progress policy. The first executable slice is the typed pre-claim intake hook contract and loop insertion point.\n\nROOT CAUSE WITH FILE:LINE\n- cli/internal/agent/execute_bead_loop.go:680 invokes PreDispatchLintHook before claim, but there is no typed PreClaimIntakeHook with the finalized intake outcomes.\n- cli/internal/agent/lint_hook.go:86 constructs lint-only behavior, not actionability/decomposition decisions.\n\nPROPOSED FIX\n- Add PreClaimIntakeHook (or equivalent guard) to ExecuteBeadLoopRuntime.\n- Define typed outcomes actionable_atomic, actionable_but_rewritten, too_large_decomposed, ambiguous_needs_human, and intake_error.\n- Invoke the hook after ready selection/routing preflight and before Claim.\n- Implement actionable_atomic pass-through and intake_error fail-open for infrastructure failures only.\n\nNON-SCOPE\n- Safe rewrite mutation.\n- Child bead creation/decomposition.\n- No-progress accounting changes beyond not counting intake_error as implementation no-progress.",
+    "acceptance": "1. ExecuteBeadLoopRuntime exposes a pre-claim intake hook/guard with typed outcomes actionable_atomic, actionable_but_rewritten, too_large_decomposed, ambiguous_needs_human, and intake_error.\n2. TestIntake_ActionableAtomic_ClaimsNormally verifies actionable_atomic proceeds to Claim and implementation.\n3. TestIntake_InfrastructureErrorFailsOpen verifies intake_error from hook infrastructure records a warning event and does not count as implementation no-progress.\n4. TestIntake_HookRunsBeforeClaim verifies the hook fires before Store.Claim and after route preflight.\n5. cd cli \u0026\u0026 go test ./internal/agent/... -run \"TestIntake_.*(ActionableAtomic|InfrastructureError|HookRunsBeforeClaim)\" -count=1 passes.\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-a9d130d0",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:agent",
+      "kind:feature",
+      "reliability",
+      "adr:023",
+      "adr:024",
+      "from:ddx-f3bbcfce"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T03:29:56Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T03:29:56.518127627Z",
+      "spec_id": "ADR-023"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T032956-8a3c2b28",
+    "prompt": ".ddx/executions/20260506T032956-8a3c2b28/prompt.md",
+    "manifest": ".ddx/executions/20260506T032956-8a3c2b28/manifest.json",
+    "result": ".ddx/executions/20260506T032956-8a3c2b28/result.json",
+    "checks": ".ddx/executions/20260506T032956-8a3c2b28/checks.json",
+    "usage": ".ddx/executions/20260506T032956-8a3c2b28/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-12638823-20260506T032956-8a3c2b28"
+  },
+  "prompt_sha": "ea37631a01532139f38285c73327de55c939243c387be6c57596cc174515d026"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T032956-8a3c2b28/result.json b/.ddx/executions/20260506T032956-8a3c2b28/result.json
new file mode 100644
index 00000000..51f34c5e
--- /dev/null
+++ b/.ddx/executions/20260506T032956-8a3c2b28/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-12638823",
+  "attempt_id": "20260506T032956-8a3c2b28",
+  "base_rev": "98c83d3cb803e655a9c1adcf79ba472a3e062ea8",
+  "result_rev": "920ccda9d5b50863528e99bc072f3cd50ddfe26a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-0b0465aa",
+  "duration_ms": 518100,
+  "tokens": 6832810,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T032956-8a3c2b28",
+  "prompt_file": ".ddx/executions/20260506T032956-8a3c2b28/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T032956-8a3c2b28/manifest.json",
+  "result_file": ".ddx/executions/20260506T032956-8a3c2b28/result.json",
+  "usage_file": ".ddx/executions/20260506T032956-8a3c2b28/usage.json",
+  "started_at": "2026-05-06T03:29:59.044843159Z",
+  "finished_at": "2026-05-06T03:38:37.145729833Z"
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
