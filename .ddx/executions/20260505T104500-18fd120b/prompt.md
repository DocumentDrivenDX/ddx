<bead-review>
  <bead id="ddx-c8f79963" iter=1>
    <title>C5: move no_changes adjudication, decomposition, push-failed, push-conflict into try.Attempt (absorb ddx-b24e9630 + ddx-c6e3db02 behavior)</title>
    <description>
PROBLEM
execute_bead_loop.go:875-1036 handles no_changes adjudication, decomposition, push-failed, and push-conflict inline in the loop body. These are distinct outcome behaviors that belong in try.Attempt so the loop only sees Disposition + Cooldown + ParkReason + RecordEvents.

ROOT CAUSE
- cli/internal/agent/execute_bead_loop.go:875-1036: the block that follows a bead attempt contains outcome-specific branches (no_changes, decomposition, push_failed, push_conflict) as raw loop logic rather than Disposition values from try.Attempt.
- ddx-b24e9630 (no_changes verification): adjudicateNoChangesContract function exists in the codebase — confirm it is called from try.Attempt's no_changes branch (not merely defined). If wired: preserve. If unwired: this child must wire it.
- ddx-c6e3db02 (rate-limit retry): EvaluateRetryPolicy (or similar) must be called from try.Attempt's rate-limit detection path — confirm actual wiring before this refactor moves it.
- try/attempt.go:67 is the candidate location for the relocated no_changes adjudication logic.
- The dead-code audit (ddx-09d2990c) must close before this child to establish which functions are genuinely wired vs. only defined.

PROPOSED FIX
- Remove lines 875-1036 from execute_bead_loop.go; replace with: read Disposition + Cooldown + ParkReason + RecordEvents from try.Attempt result.
- Move no_changes adjudication (verification_command runner + triage label logic) into try.Attempt's no_changes branch near try/attempt.go:67.
- Move rate-limit retry (Retry-After honor + exponential backoff + budget cap) into try.Attempt's rate-limit detection.
- Status NEVER changes to needs_investigation on no_changes; stays in canonical state 6; uses triage:no-changes-unverified label per ADR-004.

NON-SCOPE
- Do not modify the Drain loop contract beyond replacing lines 875-1036 with Disposition/Cooldown reads.
- ddx-09d2990c wire-or-delete must close first (hard dep).

INTERSECTIONS
- P1: no_changes, decomposition, push-failed, and push-conflict must fail open instead of wedging the loop.
- P2: try.Attempt owns the outcome classification; the drain loop should not own per-outcome policy.
- P3: emit structured events so operators can see the exact degradation instead of a silent retry loop.
- P4: the handling must stay per-bead.
- P6: auto-retry should remain limited to transient classes only.
    </description>
    <acceptance>
1. execute_bead_loop.go:875-1036 removed; equivalent logic in try.Attempt with proper Disposition/ParkReason returns.
2. ddx-b24e9630 behavior preserved in try.Attempt: verification_command runner invoked from no_changes branch near try/attempt.go:67; triage:no-changes-* labels applied.
3. ddx-c6e3db02 behavior preserved in try.Attempt: Retry-After honor + exponential backoff + budget cap in rate-limit detection path.
4. Wire-confirmation: adjudicateNoChangesContract and EvaluateRetryPolicy confirmed called from try.Attempt (not just defined) — grep assertions in test file.
5. TestAttempt_NoChanges_StaysCanonical_NotNeedsInvestigation verifies status does not become needs_investigation.
6. TestAttempt_RateLimit_Retry_HonorsRetryAfter verifies Retry-After header honored.
7. C0 fixture diff: no_changes_verified, no_changes_unjustified, decomposition, push_failed, push_conflict lifecycles byte-identical.
8. cd cli &amp;&amp; go test ./internal/agent/... green.
9. lefthook run pre-commit passes.
    </acceptance>
    <notes>
decomposed into .execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e (no_changes adjudication), .execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-16072585 (rate-limit retry), and .execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-e209db79 (decomposition + push outcomes)
    </notes>
    <labels>phase:2, refactor, kind:refactor</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T104148-6e54f8ef/manifest.json</file>
    <file>.ddx/executions/20260505T104148-6e54f8ef/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="6767469c50c412e485e6205cadf223c876d1948e">
diff --git a/.ddx/executions/20260505T104148-6e54f8ef/manifest.json b/.ddx/executions/20260505T104148-6e54f8ef/manifest.json
new file mode 100644
index 00000000..7b1a7575
--- /dev/null
+++ b/.ddx/executions/20260505T104148-6e54f8ef/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260505T104148-6e54f8ef",
+  "bead_id": "ddx-c8f79963",
+  "base_rev": "cc20ebbed0a7f65613f40cee4668133706ea8d91",
+  "created_at": "2026-05-05T10:41:50.714875275Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c8f79963",
+    "title": "C5: move no_changes adjudication, decomposition, push-failed, push-conflict into try.Attempt (absorb ddx-b24e9630 + ddx-c6e3db02 behavior)",
+    "description": "PROBLEM\nexecute_bead_loop.go:875-1036 handles no_changes adjudication, decomposition, push-failed, and push-conflict inline in the loop body. These are distinct outcome behaviors that belong in try.Attempt so the loop only sees Disposition + Cooldown + ParkReason + RecordEvents.\n\nROOT CAUSE\n- cli/internal/agent/execute_bead_loop.go:875-1036: the block that follows a bead attempt contains outcome-specific branches (no_changes, decomposition, push_failed, push_conflict) as raw loop logic rather than Disposition values from try.Attempt.\n- ddx-b24e9630 (no_changes verification): adjudicateNoChangesContract function exists in the codebase — confirm it is called from try.Attempt's no_changes branch (not merely defined). If wired: preserve. If unwired: this child must wire it.\n- ddx-c6e3db02 (rate-limit retry): EvaluateRetryPolicy (or similar) must be called from try.Attempt's rate-limit detection path — confirm actual wiring before this refactor moves it.\n- try/attempt.go:67 is the candidate location for the relocated no_changes adjudication logic.\n- The dead-code audit (ddx-09d2990c) must close before this child to establish which functions are genuinely wired vs. only defined.\n\nPROPOSED FIX\n- Remove lines 875-1036 from execute_bead_loop.go; replace with: read Disposition + Cooldown + ParkReason + RecordEvents from try.Attempt result.\n- Move no_changes adjudication (verification_command runner + triage label logic) into try.Attempt's no_changes branch near try/attempt.go:67.\n- Move rate-limit retry (Retry-After honor + exponential backoff + budget cap) into try.Attempt's rate-limit detection.\n- Status NEVER changes to needs_investigation on no_changes; stays in canonical state 6; uses triage:no-changes-unverified label per ADR-004.\n\nNON-SCOPE\n- Do not modify the Drain loop contract beyond replacing lines 875-1036 with Disposition/Cooldown reads.\n- ddx-09d2990c wire-or-delete must close first (hard dep).\n\nINTERSECTIONS\n- P1: no_changes, decomposition, push-failed, and push-conflict must fail open instead of wedging the loop.\n- P2: try.Attempt owns the outcome classification; the drain loop should not own per-outcome policy.\n- P3: emit structured events so operators can see the exact degradation instead of a silent retry loop.\n- P4: the handling must stay per-bead.\n- P6: auto-retry should remain limited to transient classes only.",
+    "acceptance": "1. execute_bead_loop.go:875-1036 removed; equivalent logic in try.Attempt with proper Disposition/ParkReason returns.\n2. ddx-b24e9630 behavior preserved in try.Attempt: verification_command runner invoked from no_changes branch near try/attempt.go:67; triage:no-changes-* labels applied.\n3. ddx-c6e3db02 behavior preserved in try.Attempt: Retry-After honor + exponential backoff + budget cap in rate-limit detection path.\n4. Wire-confirmation: adjudicateNoChangesContract and EvaluateRetryPolicy confirmed called from try.Attempt (not just defined) — grep assertions in test file.\n5. TestAttempt_NoChanges_StaysCanonical_NotNeedsInvestigation verifies status does not become needs_investigation.\n6. TestAttempt_RateLimit_Retry_HonorsRetryAfter verifies Retry-After header honored.\n7. C0 fixture diff: no_changes_verified, no_changes_unjustified, decomposition, push_failed, push_conflict lifecycles byte-identical.\n8. cd cli \u0026\u0026 go test ./internal/agent/... green.\n9. lefthook run pre-commit passes.",
+    "parent": "ddx-5cb6e6cd",
+    "labels": [
+      "phase:2",
+      "refactor",
+      "kind:refactor"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T10:41:48Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "execute-loop-heartbeat-at": "2026-05-05T10:41:48.764015526Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T104148-6e54f8ef",
+    "prompt": ".ddx/executions/20260505T104148-6e54f8ef/prompt.md",
+    "manifest": ".ddx/executions/20260505T104148-6e54f8ef/manifest.json",
+    "result": ".ddx/executions/20260505T104148-6e54f8ef/result.json",
+    "checks": ".ddx/executions/20260505T104148-6e54f8ef/checks.json",
+    "usage": ".ddx/executions/20260505T104148-6e54f8ef/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef"
+  },
+  "prompt_sha": "a7641f722274489a90bcbadc6aaa714be392785c2362ed4091fe6be63df89971"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T104148-6e54f8ef/result.json b/.ddx/executions/20260505T104148-6e54f8ef/result.json
new file mode 100644
index 00000000..6f8c5fc0
--- /dev/null
+++ b/.ddx/executions/20260505T104148-6e54f8ef/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-c8f79963",
+  "attempt_id": "20260505T104148-6e54f8ef",
+  "base_rev": "cc20ebbed0a7f65613f40cee4668133706ea8d91",
+  "result_rev": "610413162067327424487afbcdb0d88b01b5bc3f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-088b6fa8",
+  "duration_ms": 184069,
+  "tokens": 2429874,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T104148-6e54f8ef",
+  "prompt_file": ".ddx/executions/20260505T104148-6e54f8ef/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T104148-6e54f8ef/manifest.json",
+  "result_file": ".ddx/executions/20260505T104148-6e54f8ef/result.json",
+  "usage_file": ".ddx/executions/20260505T104148-6e54f8ef/usage.json",
+  "started_at": "2026-05-05T10:41:50.715392108Z",
+  "finished_at": "2026-05-05T10:44:54.785111131Z"
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
