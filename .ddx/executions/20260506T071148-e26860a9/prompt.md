<bead-review>
  <bead id="ddx-fb790086" iter=1>
    <title>review: enforce adversarial pre-close approval before CloseWithEvidence</title>
    <description>
PROBLEM
After review-group dispatch exists, execute-loop must use it as a pre-close gate. Today review REQUEST_CHANGES/BLOCK semantics are still expressed as reopen behavior rather than preventing close.

ROOT CAUSE WITH FILE:LINE
- cli/internal/agent/execute_bead_loop.go:294 exposes CloseWithEvidence without a required pre-close review decision boundary.
- cli/internal/agent/execute_bead_loop.go:300 documents Reopen for post-close REQUEST_CHANGES/BLOCK.
- cli/internal/agent/execute_bead_review_test.go:242 and cli/internal/agent/execute_bead_review_test.go:279 assert reopen behavior for REQUEST_CHANGES/BLOCK.

PROPOSED FIX
- Invoke the review-group coordinator before CloseWithEvidence for every close-eligible implementation result.
- Close only when both reviewers APPROVE and each approval includes per-AC evidence.
- Treat APPROVE without evidence as review_error: unparseable.
- REQUEST_CHANGES/BLOCK prevent close and record the appropriate review event without closing/reopening the bead.

NON-SCOPE
- Repair-cycle escalation and review-error retry budget.
    </description>
    <acceptance>
1. Execute-loop calls the review-group pre-close gate before CloseWithEvidence for close-eligible implementation results.
2. TestPreCloseReview_UnanimousApproveCloses verifies both evidence-backed APPROVE results are required to close.
3. TestPreCloseReview_ApproveWithoutEvidenceIsReviewError verifies evidence-free APPROVE does not close and records review_error: unparseable.
4. TestPreCloseReview_RequestChangesPreventsClose verifies REQUEST_CHANGES/BLOCK leave the bead unclosed and do not invoke Reopen.
5. Stale reopen assertions in execute_bead_review_test.go are updated to pre-close blocking semantics.
6. cd cli &amp;&amp; go test ./internal/agent/... -run "TestPreCloseReview" -count=1 passes.
7. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:18, area:agent, kind:feature, reliability, adr:024, from:ddx-c851c3dd</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T065732-b288b5e3/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T065732-b288b5e3/manifest.json</file>
    <file>.ddx/executions/20260506T065732-b288b5e3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="85f5f5285ca2e743517445c3309b4ae786b983ad">
<untrusted-data>
diff --git a/.ddx/executions/20260506T065732-b288b5e3/checks/production-reachability.json b/.ddx/executions/20260506T065732-b288b5e3/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T065732-b288b5e3/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T065732-b288b5e3/manifest.json b/.ddx/executions/20260506T065732-b288b5e3/manifest.json
new file mode 100644
index 00000000..aad293d7
--- /dev/null
+++ b/.ddx/executions/20260506T065732-b288b5e3/manifest.json
@@ -0,0 +1,42 @@
+{
+  "attempt_id": "20260506T065732-b288b5e3",
+  "bead_id": "ddx-fb790086",
+  "base_rev": "abfe0af5ac1a5c96f4531e37a16179d838f0da95",
+  "created_at": "2026-05-06T06:57:34.85159935Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-fb790086",
+    "title": "review: enforce adversarial pre-close approval before CloseWithEvidence",
+    "description": "PROBLEM\nAfter review-group dispatch exists, execute-loop must use it as a pre-close gate. Today review REQUEST_CHANGES/BLOCK semantics are still expressed as reopen behavior rather than preventing close.\n\nROOT CAUSE WITH FILE:LINE\n- cli/internal/agent/execute_bead_loop.go:294 exposes CloseWithEvidence without a required pre-close review decision boundary.\n- cli/internal/agent/execute_bead_loop.go:300 documents Reopen for post-close REQUEST_CHANGES/BLOCK.\n- cli/internal/agent/execute_bead_review_test.go:242 and cli/internal/agent/execute_bead_review_test.go:279 assert reopen behavior for REQUEST_CHANGES/BLOCK.\n\nPROPOSED FIX\n- Invoke the review-group coordinator before CloseWithEvidence for every close-eligible implementation result.\n- Close only when both reviewers APPROVE and each approval includes per-AC evidence.\n- Treat APPROVE without evidence as review_error: unparseable.\n- REQUEST_CHANGES/BLOCK prevent close and record the appropriate review event without closing/reopening the bead.\n\nNON-SCOPE\n- Repair-cycle escalation and review-error retry budget.",
+    "acceptance": "1. Execute-loop calls the review-group pre-close gate before CloseWithEvidence for close-eligible implementation results.\n2. TestPreCloseReview_UnanimousApproveCloses verifies both evidence-backed APPROVE results are required to close.\n3. TestPreCloseReview_ApproveWithoutEvidenceIsReviewError verifies evidence-free APPROVE does not close and records review_error: unparseable.\n4. TestPreCloseReview_RequestChangesPreventsClose verifies REQUEST_CHANGES/BLOCK leave the bead unclosed and do not invoke Reopen.\n5. Stale reopen assertions in execute_bead_review_test.go are updated to pre-close blocking semantics.\n6. cd cli \u0026\u0026 go test ./internal/agent/... -run \"TestPreCloseReview\" -count=1 passes.\n7. lefthook run pre-commit passes.",
+    "parent": "ddx-42b917fe",
+    "labels": [
+      "phase:2",
+      "story:18",
+      "area:agent",
+      "kind:feature",
+      "reliability",
+      "adr:024",
+      "from:ddx-c851c3dd"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T06:57:32Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T06:57:32.24490931Z",
+      "spec_id": "ADR-024"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T065732-b288b5e3",
+    "prompt": ".ddx/executions/20260506T065732-b288b5e3/prompt.md",
+    "manifest": ".ddx/executions/20260506T065732-b288b5e3/manifest.json",
+    "result": ".ddx/executions/20260506T065732-b288b5e3/result.json",
+    "checks": ".ddx/executions/20260506T065732-b288b5e3/checks.json",
+    "usage": ".ddx/executions/20260506T065732-b288b5e3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-fb790086-20260506T065732-b288b5e3"
+  },
+  "prompt_sha": "890d8075a44089870a8a4b06f93af39d8a6ee7ba630984dba314e891d553ca6b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T065732-b288b5e3/result.json b/.ddx/executions/20260506T065732-b288b5e3/result.json
new file mode 100644
index 00000000..e3df83ab
--- /dev/null
+++ b/.ddx/executions/20260506T065732-b288b5e3/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-fb790086",
+  "attempt_id": "20260506T065732-b288b5e3",
+  "base_rev": "abfe0af5ac1a5c96f4531e37a16179d838f0da95",
+  "result_rev": "e9b57cf519d538d1770c67b74154a4e6ec0c3964",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-ab929341",
+  "duration_ms": 846653,
+  "tokens": 16215071,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T065732-b288b5e3",
+  "prompt_file": ".ddx/executions/20260506T065732-b288b5e3/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T065732-b288b5e3/manifest.json",
+  "result_file": ".ddx/executions/20260506T065732-b288b5e3/result.json",
+  "usage_file": ".ddx/executions/20260506T065732-b288b5e3/usage.json",
+  "started_at": "2026-05-06T06:57:34.852012474Z",
+  "finished_at": "2026-05-06T07:11:41.505088103Z"
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
