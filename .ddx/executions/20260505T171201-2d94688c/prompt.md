<bead-review>
  <bead id="ddx-fe1bed9d" iter=1>
    <title>try.Attempt: relocate decomposition and push outcome routing</title>
    <description>
PROBLEM\nThe execute loop still owns the terminal routing for declined_needs_decomposition, push_failed, and push_conflict. Those outcomes are not just presentation details: they decide whether a bead is parked, which event is emitted, and whether the claim is released, so keeping them in the loop means try.Attempt does not own the full outcome classification contract.\n\nROOT CAUSE\n- cli/internal/agent/execute_bead_loop.go:1063-1160 branches inline on ExecuteBeadStatusDeclinedNeedsDecomposition, ExecuteBeadStatusPushFailed, and ExecuteBeadStatusPushConflict, emits the corresponding events, and applies the cooldowns.\n- cli/internal/agent/execute_bead_status.go:320-333 classifies push_failed / push_conflict from a merged outcome, but the loop still performs the actual parking and event emission.\n- cli/internal/agent/try/attempt.go:99-137 only returns the legacy conflict-recovery outcome and does not yet absorb these routes.\n\nPROPOSED FIX\n- Extend cli/internal/agent/try/attempt.go so decomposition and push-failure / push-conflict policies return structured disposition + cooldown + event instructions.\n- Preserve the current event payloads and labels (decomposition-recommendation, push-failed, push-conflict) and keep the cooldown caps unchanged.\n- Remove the corresponding branches from execute_bead_loop.go so the loop only consumes the structured attempt result.\n- Keep push_failed claim refusal and cooldown persistence semantics intact.\n\nNON-SCOPE\n- No_changes adjudication.\n- Rate-limit retry policy.\n- Any change to the push-failure / push-conflict status taxonomy.\n\nINTERSECTIONS\n- Preserves ddx-fba752b9 / ddx-af54ebf3 / push-conflict behavior while moving ownership into try.Attempt.\n
    </description>
    <acceptance>
1. TestAttempt_DeclinedNeedsDecomposition_ParksWithStructuredEvent verifies try.Attempt returns the decomposition recommendation and park instructions rather than leaving the loop to classify it.\n2. TestAttempt_PushFailed_ParksWithCooldown and TestAttempt_PushConflict_ParksWithCooldown verify the attempt layer owns the push-failed and push-conflict parking behavior.\n3. TestExecuteBeadWorkerDeclinedNeedsDecompositionParksBead, TestExecuteBeadWorkerPushFailedStaysOpenAndParks, TestExecuteBeadWorkerPushFailedCooldownCappedAt24h, and TestExecuteBeadWorkerPushConflictParksAndEmitsEvent remain green after the move.\n4. TestClaimRefusesPushFailedBead still fails loudly until the operator clears execute-loop-last-status.\n5. cd cli &amp;&amp; go test ./internal/agent/... green.\n6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, kind:refactor, story:10, observed-failure</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T170409-802a7da5/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T170409-802a7da5/manifest.json</file>
    <file>.ddx/executions/20260505T170409-802a7da5/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="062295347e7d1a26086ad6b1bca5bc46d6783a11">
diff --git a/.ddx/executions/20260505T170409-802a7da5/checks/production-reachability.json b/.ddx/executions/20260505T170409-802a7da5/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T170409-802a7da5/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T170409-802a7da5/manifest.json b/.ddx/executions/20260505T170409-802a7da5/manifest.json
new file mode 100644
index 00000000..e5bee47f
--- /dev/null
+++ b/.ddx/executions/20260505T170409-802a7da5/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260505T170409-802a7da5",
+  "bead_id": "ddx-fe1bed9d",
+  "base_rev": "0b83c6b83957d6ae72910c394b7550e7108d4c16",
+  "created_at": "2026-05-05T17:04:12.392106139Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-fe1bed9d",
+    "title": "try.Attempt: relocate decomposition and push outcome routing",
+    "description": "PROBLEM\\nThe execute loop still owns the terminal routing for declined_needs_decomposition, push_failed, and push_conflict. Those outcomes are not just presentation details: they decide whether a bead is parked, which event is emitted, and whether the claim is released, so keeping them in the loop means try.Attempt does not own the full outcome classification contract.\\n\\nROOT CAUSE\\n- cli/internal/agent/execute_bead_loop.go:1063-1160 branches inline on ExecuteBeadStatusDeclinedNeedsDecomposition, ExecuteBeadStatusPushFailed, and ExecuteBeadStatusPushConflict, emits the corresponding events, and applies the cooldowns.\\n- cli/internal/agent/execute_bead_status.go:320-333 classifies push_failed / push_conflict from a merged outcome, but the loop still performs the actual parking and event emission.\\n- cli/internal/agent/try/attempt.go:99-137 only returns the legacy conflict-recovery outcome and does not yet absorb these routes.\\n\\nPROPOSED FIX\\n- Extend cli/internal/agent/try/attempt.go so decomposition and push-failure / push-conflict policies return structured disposition + cooldown + event instructions.\\n- Preserve the current event payloads and labels (decomposition-recommendation, push-failed, push-conflict) and keep the cooldown caps unchanged.\\n- Remove the corresponding branches from execute_bead_loop.go so the loop only consumes the structured attempt result.\\n- Keep push_failed claim refusal and cooldown persistence semantics intact.\\n\\nNON-SCOPE\\n- No_changes adjudication.\\n- Rate-limit retry policy.\\n- Any change to the push-failure / push-conflict status taxonomy.\\n\\nINTERSECTIONS\\n- Preserves ddx-fba752b9 / ddx-af54ebf3 / push-conflict behavior while moving ownership into try.Attempt.\\n",
+    "acceptance": "1. TestAttempt_DeclinedNeedsDecomposition_ParksWithStructuredEvent verifies try.Attempt returns the decomposition recommendation and park instructions rather than leaving the loop to classify it.\\n2. TestAttempt_PushFailed_ParksWithCooldown and TestAttempt_PushConflict_ParksWithCooldown verify the attempt layer owns the push-failed and push-conflict parking behavior.\\n3. TestExecuteBeadWorkerDeclinedNeedsDecompositionParksBead, TestExecuteBeadWorkerPushFailedStaysOpenAndParks, TestExecuteBeadWorkerPushFailedCooldownCappedAt24h, and TestExecuteBeadWorkerPushConflictParksAndEmitsEvent remain green after the move.\\n4. TestClaimRefusesPushFailedBead still fails loudly until the operator clears execute-loop-last-status.\\n5. cd cli \u0026\u0026 go test ./internal/agent/... green.\\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-c8f79963",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "kind:refactor",
+      "story:10",
+      "observed-failure"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T17:04:09Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "execute-loop-heartbeat-at": "2026-05-05T17:04:09.718174867Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T170409-802a7da5",
+    "prompt": ".ddx/executions/20260505T170409-802a7da5/prompt.md",
+    "manifest": ".ddx/executions/20260505T170409-802a7da5/manifest.json",
+    "result": ".ddx/executions/20260505T170409-802a7da5/result.json",
+    "checks": ".ddx/executions/20260505T170409-802a7da5/checks.json",
+    "usage": ".ddx/executions/20260505T170409-802a7da5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-fe1bed9d-20260505T170409-802a7da5"
+  },
+  "prompt_sha": "895a8b5392ab2bc0e0eb9be618d90f051d831c09232536d806e6cfbf4388e162"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T170409-802a7da5/result.json b/.ddx/executions/20260505T170409-802a7da5/result.json
new file mode 100644
index 00000000..8b32037d
--- /dev/null
+++ b/.ddx/executions/20260505T170409-802a7da5/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-fe1bed9d",
+  "attempt_id": "20260505T170409-802a7da5",
+  "base_rev": "0b83c6b83957d6ae72910c394b7550e7108d4c16",
+  "result_rev": "a2c2fd2a6c411f195585c3b4013393d275983401",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-48418111",
+  "duration_ms": 460319,
+  "tokens": 5603174,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T170409-802a7da5",
+  "prompt_file": ".ddx/executions/20260505T170409-802a7da5/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T170409-802a7da5/manifest.json",
+  "result_file": ".ddx/executions/20260505T170409-802a7da5/result.json",
+  "usage_file": ".ddx/executions/20260505T170409-802a7da5/usage.json",
+  "started_at": "2026-05-05T17:04:12.39251918Z",
+  "finished_at": "2026-05-05T17:11:52.712497883Z"
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
