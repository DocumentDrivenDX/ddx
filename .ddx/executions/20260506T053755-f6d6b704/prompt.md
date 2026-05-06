<bead-review>
  <bead id="ddx-cdec4cc8" iter=1>
    <title>review: add adversarial review-group dispatch primitives</title>
    <description>
PROBLEM
The default adversarial pre-close review gate needs review-group primitives before execute-loop can make close decisions. The parent bead ddx-c851c3dd was too broad and bounced with no_changes_needs_investigation.

ROOT CAUSE WITH FILE:LINE
- cli/internal/agent/execute_bead_review.go:474 builds a single reviewer request without review_group_id or reviewer_index correlation fields.
- cli/internal/agent/execute_bead_review.go:528 runs one DefaultBeadReviewer.ReviewBead call and returns one ReviewResult.
- cli/internal/agent/execute_bead_loop.go:226 still models ReviewVerdict as a single review verdict field.

PROPOSED FIX
- Introduce review group data structures for two reviewer slots over one result_rev.
- Add correlation metadata role=reviewer, review_group_id, reviewer_index, bead_id, attempt_id, result_rev, and implementer route facts to reviewer dispatch.
- Add a coordinator/helper that dispatches both reviewer slots against the same evidence bundle but does not yet change close behavior.

NON-SCOPE
- Close/block policy integration in execute-loop.
- Repair-cycle retry policy.
    </description>
    <acceptance>
1. A review-group coordinator/helper dispatches two reviewer slots for one bead/result_rev and returns both structured reviewer results.
2. TestReviewGroup_DispatchesTwoSlotsSameEvidence verifies both reviewers receive the same evidence bundle identity/path.
3. TestReviewGroup_CorrelationFields verifies role=reviewer, review_group_id, reviewer_index=0/1, bead_id, attempt_id, result_rev, and implementer route facts are present.
4. Existing single-review tests remain green or are updated to use the review-group helper without changing loop close behavior.
5. cd cli &amp;&amp; go test ./internal/agent/... -run "TestReviewGroup|TestBuildReviewExecuteRequest" -count=1 passes.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:18, area:agent, kind:feature, reliability, adr:024, from:ddx-c851c3dd</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T053015-dc163332/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T053015-dc163332/manifest.json</file>
    <file>.ddx/executions/20260506T053015-dc163332/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8d869aba28c4a0c15220de340ca8bd63917305db">
<untrusted-data>
diff --git a/.ddx/executions/20260506T053015-dc163332/checks/production-reachability.json b/.ddx/executions/20260506T053015-dc163332/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T053015-dc163332/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T053015-dc163332/manifest.json b/.ddx/executions/20260506T053015-dc163332/manifest.json
new file mode 100644
index 00000000..5c97e29e
--- /dev/null
+++ b/.ddx/executions/20260506T053015-dc163332/manifest.json
@@ -0,0 +1,52 @@
+{
+  "attempt_id": "20260506T053015-dc163332",
+  "bead_id": "ddx-cdec4cc8",
+  "base_rev": "35efdfe9290802ffbe2b860991574da2aa921f0b",
+  "created_at": "2026-05-06T05:30:17.523477583Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cdec4cc8",
+    "title": "review: add adversarial review-group dispatch primitives",
+    "description": "PROBLEM\nThe default adversarial pre-close review gate needs review-group primitives before execute-loop can make close decisions. The parent bead ddx-c851c3dd was too broad and bounced with no_changes_needs_investigation.\n\nROOT CAUSE WITH FILE:LINE\n- cli/internal/agent/execute_bead_review.go:474 builds a single reviewer request without review_group_id or reviewer_index correlation fields.\n- cli/internal/agent/execute_bead_review.go:528 runs one DefaultBeadReviewer.ReviewBead call and returns one ReviewResult.\n- cli/internal/agent/execute_bead_loop.go:226 still models ReviewVerdict as a single review verdict field.\n\nPROPOSED FIX\n- Introduce review group data structures for two reviewer slots over one result_rev.\n- Add correlation metadata role=reviewer, review_group_id, reviewer_index, bead_id, attempt_id, result_rev, and implementer route facts to reviewer dispatch.\n- Add a coordinator/helper that dispatches both reviewer slots against the same evidence bundle but does not yet change close behavior.\n\nNON-SCOPE\n- Close/block policy integration in execute-loop.\n- Repair-cycle retry policy.",
+    "acceptance": "1. A review-group coordinator/helper dispatches two reviewer slots for one bead/result_rev and returns both structured reviewer results.\n2. TestReviewGroup_DispatchesTwoSlotsSameEvidence verifies both reviewers receive the same evidence bundle identity/path.\n3. TestReviewGroup_CorrelationFields verifies role=reviewer, review_group_id, reviewer_index=0/1, bead_id, attempt_id, result_rev, and implementer route facts are present.\n4. Existing single-review tests remain green or are updated to use the review-group helper without changing loop close behavior.\n5. cd cli \u0026\u0026 go test ./internal/agent/... -run \"TestReviewGroup|TestBuildReviewExecuteRequest\" -count=1 passes.\n6. lefthook run pre-commit passes.",
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
+      "claimed-at": "2026-05-06T05:30:15Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "staging tracker: fatal: Unable to create '/home/erik/Projects/ddx/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository, or the lock file may be stale: exit status 128",
+          "created_at": "2026-05-06T03:29:55.755742612Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T05:30:15.033094242Z",
+      "spec_id": "ADR-024"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T053015-dc163332",
+    "prompt": ".ddx/executions/20260506T053015-dc163332/prompt.md",
+    "manifest": ".ddx/executions/20260506T053015-dc163332/manifest.json",
+    "result": ".ddx/executions/20260506T053015-dc163332/result.json",
+    "checks": ".ddx/executions/20260506T053015-dc163332/checks.json",
+    "usage": ".ddx/executions/20260506T053015-dc163332/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cdec4cc8-20260506T053015-dc163332"
+  },
+  "prompt_sha": "6f84180ed146708ea06dc4603723ddaa526a49b8eefcf4c684138f4c2fd90c75"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T053015-dc163332/result.json b/.ddx/executions/20260506T053015-dc163332/result.json
new file mode 100644
index 00000000..98ba2056
--- /dev/null
+++ b/.ddx/executions/20260506T053015-dc163332/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-cdec4cc8",
+  "attempt_id": "20260506T053015-dc163332",
+  "base_rev": "35efdfe9290802ffbe2b860991574da2aa921f0b",
+  "result_rev": "f9e3691f4157a5364fbc278247ac7b63aace9a69",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-7afcb18b",
+  "duration_ms": 451336,
+  "tokens": 6831381,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T053015-dc163332",
+  "prompt_file": ".ddx/executions/20260506T053015-dc163332/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T053015-dc163332/manifest.json",
+  "result_file": ".ddx/executions/20260506T053015-dc163332/result.json",
+  "usage_file": ".ddx/executions/20260506T053015-dc163332/usage.json",
+  "started_at": "2026-05-06T05:30:17.523777291Z",
+  "finished_at": "2026-05-06T05:37:48.860324361Z"
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
