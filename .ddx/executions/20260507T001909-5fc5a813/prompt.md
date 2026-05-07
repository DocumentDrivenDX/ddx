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
    <file>.ddx/executions/20260507T001809-db976d1f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3b03e5a33ac2ed5b2a5a401598a5a8721200acce">
<untrusted-data>
diff --git a/.ddx/executions/20260507T001809-db976d1f/result.json b/.ddx/executions/20260507T001809-db976d1f/result.json
new file mode 100644
index 000000000..92424a7b8
--- /dev/null
+++ b/.ddx/executions/20260507T001809-db976d1f/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-fb790086",
+  "attempt_id": "20260507T001809-db976d1f",
+  "base_rev": "a7b280e3d2ede6a9e1726c0fd508bfa7c2f4f4a8",
+  "result_rev": "7bb73055500125e63db5f87af2053164647fa288",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-2808ca26",
+  "duration_ms": 47396,
+  "tokens": 534518,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260507T001809-db976d1f",
+  "prompt_file": ".ddx/executions/20260507T001809-db976d1f/prompt.md",
+  "manifest_file": ".ddx/executions/20260507T001809-db976d1f/manifest.json",
+  "result_file": ".ddx/executions/20260507T001809-db976d1f/result.json",
+  "usage_file": ".ddx/executions/20260507T001809-db976d1f/usage.json",
+  "started_at": "2026-05-07T00:18:12.015625387Z",
+  "finished_at": "2026-05-07T00:18:59.412402785Z"
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
