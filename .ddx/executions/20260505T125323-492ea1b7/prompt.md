<bead-review>
  <bead id=".execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e" iter=1>
    <title>try.Attempt: relocate no_changes adjudication and labels</title>
    <description>
PROBLEM\nThe no_changes contract is still adjudicated in execute_bead_loop.go after Attempt returns, so the loop owns triage labels, no_changes_count bookkeeping, and already_satisfied closure policy instead of try.Attempt. That makes the attempt contract leaky and keeps no_changes policy split across packages.\n\nROOT CAUSE\n- cli/internal/agent/execute_bead_loop.go:975-1059 calls w.adjudicateNoChangesContract, increments the no_changes counter, applies triage labels, emits no_changes_* events, and closes the bead when the contract is satisfied.\n- cli/internal/agent/try/attempt.go:99-137 only returns OutcomeReported / OutcomeSuccess / OutcomePark for conflict recovery; it does not own the no_changes policy.\n\nPROPOSED FIX\n- Move ParseNoChangesRationale, verification_command execution, and triage label/event selection into cli/internal/agent/try/attempt.go near Attempt().\n- Extend the try outcome contract so the loop only consumes structured disposition data plus cooldown/park reason/event instructions.\n- Keep the persisted status canonical: needs_investigation is represented with triage labels/events, not a new status value.\n- Reduce execute_bead_loop.go to generic structured-outcome handling for the no_changes path.\n\nNON-SCOPE\n- Rate-limit retry handling.\n- Decomposition, push_failed, or push_conflict routing.\n- Any change to the canonical status vocabulary beyond the existing triage-label contract.\n\nINTERSECTIONS\n- Absorbs ddx-b24e9630 behavior.\n
    </description>
    <acceptance>
1. TestAttempt_NoChanges_StaysCanonical_NotNeedsInvestigation verifies the attempt layer does not persist needs_investigation as a status and still emits triage:no-changes-* labels.\n2. TestAttempt_NoChanges_WiresAdjudicateNoChangesContract proves try.Attempt calls adjudicateNoChangesContract rather than leaving it only in execute_bead_loop.go.\n3. TestExecuteBeadWorkerNoChangesVerifiedClosesImmediately, TestExecuteBeadWorkerNoChangesVerificationFailsKeepsBeadOpen, TestExecuteBeadWorkerNoChangesNeedsInvestigationKeepsBeadOpen, and TestExecuteBeadWorkerNoChangesUnjustifiedStaysOpenWithLabel remain green after the move.\n4. cd cli &amp;&amp; go test ./internal/agent/... green.\n5. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, kind:refactor, story:10, observed-failure</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T125043-f4226f94/manifest.json</file>
    <file>.ddx/executions/20260505T125043-f4226f94/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="92713776639a2f72038ea300dcf073719ad91d30">
diff --git a/.ddx/executions/20260505T125043-f4226f94/manifest.json b/.ddx/executions/20260505T125043-f4226f94/manifest.json
new file mode 100644
index 00000000..3e76a801
--- /dev/null
+++ b/.ddx/executions/20260505T125043-f4226f94/manifest.json
@@ -0,0 +1,81 @@
+{
+  "attempt_id": "20260505T125043-f4226f94",
+  "bead_id": ".execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e",
+  "base_rev": "f9e1d699b9e8d7fa32244c3ffec1330e784d234b",
+  "created_at": "2026-05-05T12:50:46.661841651Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e",
+    "title": "try.Attempt: relocate no_changes adjudication and labels",
+    "description": "PROBLEM\\nThe no_changes contract is still adjudicated in execute_bead_loop.go after Attempt returns, so the loop owns triage labels, no_changes_count bookkeeping, and already_satisfied closure policy instead of try.Attempt. That makes the attempt contract leaky and keeps no_changes policy split across packages.\\n\\nROOT CAUSE\\n- cli/internal/agent/execute_bead_loop.go:975-1059 calls w.adjudicateNoChangesContract, increments the no_changes counter, applies triage labels, emits no_changes_* events, and closes the bead when the contract is satisfied.\\n- cli/internal/agent/try/attempt.go:99-137 only returns OutcomeReported / OutcomeSuccess / OutcomePark for conflict recovery; it does not own the no_changes policy.\\n\\nPROPOSED FIX\\n- Move ParseNoChangesRationale, verification_command execution, and triage label/event selection into cli/internal/agent/try/attempt.go near Attempt().\\n- Extend the try outcome contract so the loop only consumes structured disposition data plus cooldown/park reason/event instructions.\\n- Keep the persisted status canonical: needs_investigation is represented with triage labels/events, not a new status value.\\n- Reduce execute_bead_loop.go to generic structured-outcome handling for the no_changes path.\\n\\nNON-SCOPE\\n- Rate-limit retry handling.\\n- Decomposition, push_failed, or push_conflict routing.\\n- Any change to the canonical status vocabulary beyond the existing triage-label contract.\\n\\nINTERSECTIONS\\n- Absorbs ddx-b24e9630 behavior.\\n",
+    "acceptance": "1. TestAttempt_NoChanges_StaysCanonical_NotNeedsInvestigation verifies the attempt layer does not persist needs_investigation as a status and still emits triage:no-changes-* labels.\\n2. TestAttempt_NoChanges_WiresAdjudicateNoChangesContract proves try.Attempt calls adjudicateNoChangesContract rather than leaving it only in execute_bead_loop.go.\\n3. TestExecuteBeadWorkerNoChangesVerifiedClosesImmediately, TestExecuteBeadWorkerNoChangesVerificationFailsKeepsBeadOpen, TestExecuteBeadWorkerNoChangesNeedsInvestigationKeepsBeadOpen, and TestExecuteBeadWorkerNoChangesUnjustifiedStaysOpenWithLabel remain green after the move.\\n4. cd cli \u0026\u0026 go test ./internal/agent/... green.\\n5. lefthook run pre-commit passes.",
+    "parent": "ddx-c8f79963",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "kind:refactor",
+      "story:10",
+      "observed-failure"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T12:50:43Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T10:58:00.971092722Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T104506-9482a189\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":15777397,\"output_tokens\":62078,\"total_tokens\":15839475,\"cost_usd\":0,\"duration_ms\":772874,\"exit_code\":0}",
+          "created_at": "2026-05-05T10:58:01.209924406Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=15839475 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T10:58:07.32554575Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=7ce24244ed707fa841a2cc4f5974119447a073a2\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T07:03:11-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=10264\noutput_bytes=0\nelapsed_ms=4179",
+          "created_at": "2026-05-05T10:58:12.017288522Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=7ce24244ed707fa841a2cc4f5974119447a073a2\nbase_rev=b31470d404c198d84910359465e0be6dd219d983",
+          "created_at": "2026-05-05T10:58:12.244457011Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T12:50:43.424780398Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T125043-f4226f94",
+    "prompt": ".ddx/executions/20260505T125043-f4226f94/prompt.md",
+    "manifest": ".ddx/executions/20260505T125043-f4226f94/manifest.json",
+    "result": ".ddx/executions/20260505T125043-f4226f94/result.json",
+    "checks": ".ddx/executions/20260505T125043-f4226f94/checks.json",
+    "usage": ".ddx/executions/20260505T125043-f4226f94/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e-20260505T125043-f4226f94"
+  },
+  "prompt_sha": "005b01d1b04abf1966a75798640c85d726010525b455efa38f0b4018955a075b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T125043-f4226f94/result.json b/.ddx/executions/20260505T125043-f4226f94/result.json
new file mode 100644
index 00000000..d2567393
--- /dev/null
+++ b/.ddx/executions/20260505T125043-f4226f94/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e",
+  "attempt_id": "20260505T125043-f4226f94",
+  "base_rev": "f9e1d699b9e8d7fa32244c3ffec1330e784d234b",
+  "result_rev": "0ab3180787f9ca825a5dc27a11fcd7ca93c83cba",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-0e1c21c4",
+  "duration_ms": 149648,
+  "tokens": 1356438,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T125043-f4226f94",
+  "prompt_file": ".ddx/executions/20260505T125043-f4226f94/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T125043-f4226f94/manifest.json",
+  "result_file": ".ddx/executions/20260505T125043-f4226f94/result.json",
+  "usage_file": ".ddx/executions/20260505T125043-f4226f94/usage.json",
+  "started_at": "2026-05-05T12:50:46.663836316Z",
+  "finished_at": "2026-05-05T12:53:16.312007045Z"
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
