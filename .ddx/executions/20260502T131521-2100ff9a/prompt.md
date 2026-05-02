<bead-review>
  <bead id="ddx-3c154349" iter=1>
    <title>ddx work: auto-triage on review BLOCK and other recoverable failure modes</title>
    <description>
Currently when a bead hits a post-merge review BLOCK, the drain loop emits the BLOCK event, leaves the bead open, and moves to the next ready bead. The BLOCK then sits in the queue waiting for a human. Observed in this drain: ddx-a9edd569 (S3_B label-aware collision) hit BLOCK and was abandoned with no triage.

The loop should take an automated triage action when recoverable failure modes are observed, rather than dropping. Failure modes worth triaging:
- post-merge review BLOCK → either (a) re-attempt with the BLOCK reasons embedded in the next prompt's context, or (b) escalate to next tier in the S10_2 ladder, or (c) file a follow-up bead capturing the BLOCK reasons + mark the original needs_human
- 'staging tracker: Unable to create .git/index.lock' (git contention — see sibling bead) → retry with backoff after the lock clears
- 'execution_failed' from substantive (non-infrastructure) error → escalate per S10 ladder
- 'no_changes' (agent decided nothing to do) → re-evaluate against the bead AC; if still seems valid, file a clarification follow-up rather than mark already_satisfied

Triage policy is a config-driven, per-failure-mode mapping. Default policy: escalate-then-needs-human after N attempts at the next tier.
    </description>
    <acceptance>
1. Loop has a triage step after each attempt's terminal event. 2. Per-failure-mode triage policy configurable via .ddx/config.yaml or flag. 3. Default policy: BLOCK→re-attempt-with-context, then escalate-tier on second BLOCK, then needs_human on third. 4. Lock contention errors retry with backoff (see sibling bead for the lock fix itself). 5. Tests cover at least: BLOCK→re-attempt, BLOCK→escalate, lock-error→retry. 6. Existing review-pairing-degraded events surface in the triage decision (so re-attempt gets the right reviewer pairing context).
    </acceptance>
    <notes>
decomposed into 3 children: framework (.execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-0a700bd2), review-block integration (.execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-5f67936e), lock/execution_failed/no_changes integration (.execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-2b64cb0a). Children 2 and 3 depend on child 1.
    </notes>
    <labels>phase:2, story:10, area:agent, kind:fix, observed-failure</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T131050-bf2d17f5/manifest.json</file>
    <file>.ddx/executions/20260502T131050-bf2d17f5/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="af2a6a856c4ba1173303616a241909a26eb2c08b">
diff --git a/.ddx/executions/20260502T131050-bf2d17f5/manifest.json b/.ddx/executions/20260502T131050-bf2d17f5/manifest.json
new file mode 100644
index 00000000..61c35b45
--- /dev/null
+++ b/.ddx/executions/20260502T131050-bf2d17f5/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260502T131050-bf2d17f5",
+  "bead_id": "ddx-3c154349",
+  "base_rev": "b80710e8b8ac3b34531b18984ae32ef568b99532",
+  "created_at": "2026-05-02T13:10:52.116877865Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3c154349",
+    "title": "ddx work: auto-triage on review BLOCK and other recoverable failure modes",
+    "description": "Currently when a bead hits a post-merge review BLOCK, the drain loop emits the BLOCK event, leaves the bead open, and moves to the next ready bead. The BLOCK then sits in the queue waiting for a human. Observed in this drain: ddx-a9edd569 (S3_B label-aware collision) hit BLOCK and was abandoned with no triage.\n\nThe loop should take an automated triage action when recoverable failure modes are observed, rather than dropping. Failure modes worth triaging:\n- post-merge review BLOCK → either (a) re-attempt with the BLOCK reasons embedded in the next prompt's context, or (b) escalate to next tier in the S10_2 ladder, or (c) file a follow-up bead capturing the BLOCK reasons + mark the original needs_human\n- 'staging tracker: Unable to create .git/index.lock' (git contention — see sibling bead) → retry with backoff after the lock clears\n- 'execution_failed' from substantive (non-infrastructure) error → escalate per S10 ladder\n- 'no_changes' (agent decided nothing to do) → re-evaluate against the bead AC; if still seems valid, file a clarification follow-up rather than mark already_satisfied\n\nTriage policy is a config-driven, per-failure-mode mapping. Default policy: escalate-then-needs-human after N attempts at the next tier.",
+    "acceptance": "1. Loop has a triage step after each attempt's terminal event. 2. Per-failure-mode triage policy configurable via .ddx/config.yaml or flag. 3. Default policy: BLOCK→re-attempt-with-context, then escalate-tier on second BLOCK, then needs_human on third. 4. Lock contention errors retry with backoff (see sibling bead for the lock fix itself). 5. Tests cover at least: BLOCK→re-attempt, BLOCK→escalate, lock-error→retry. 6. Existing review-pairing-degraded events surface in the triage decision (so re-attempt gets the right reviewer pairing context).",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:agent",
+      "kind:fix",
+      "observed-failure"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T13:10:50Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1727028",
+      "execute-loop-heartbeat-at": "2026-05-02T13:10:50.682277663Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T131050-bf2d17f5",
+    "prompt": ".ddx/executions/20260502T131050-bf2d17f5/prompt.md",
+    "manifest": ".ddx/executions/20260502T131050-bf2d17f5/manifest.json",
+    "result": ".ddx/executions/20260502T131050-bf2d17f5/result.json",
+    "checks": ".ddx/executions/20260502T131050-bf2d17f5/checks.json",
+    "usage": ".ddx/executions/20260502T131050-bf2d17f5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T131050-bf2d17f5/result.json b/.ddx/executions/20260502T131050-bf2d17f5/result.json
new file mode 100644
index 00000000..cd0dca00
--- /dev/null
+++ b/.ddx/executions/20260502T131050-bf2d17f5/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-3c154349",
+  "attempt_id": "20260502T131050-bf2d17f5",
+  "base_rev": "b80710e8b8ac3b34531b18984ae32ef568b99532",
+  "result_rev": "88d765f40010c53be098d8436d125b880692c8c6",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6282b625",
+  "duration_ms": 265363,
+  "tokens": 15827,
+  "cost_usd": 2.0814275,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T131050-bf2d17f5",
+  "prompt_file": ".ddx/executions/20260502T131050-bf2d17f5/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T131050-bf2d17f5/manifest.json",
+  "result_file": ".ddx/executions/20260502T131050-bf2d17f5/result.json",
+  "usage_file": ".ddx/executions/20260502T131050-bf2d17f5/usage.json",
+  "started_at": "2026-05-02T13:10:52.119085864Z",
+  "finished_at": "2026-05-02T13:15:17.482135281Z"
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
