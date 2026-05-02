<bead-review>
  <bead id=".execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-5f67936e" iter=1>
    <title>Triage integration: wire decision engine into review BLOCK / REQUEST_CHANGES paths</title>
    <description>
Second of three children of ddx-3c154349. Depends on the triage policy framework child landing first.

Scope:
- In cli/internal/agent/execute_bead_loop.go, after the VerdictBlock and VerdictRequestChanges branches that already record events and reopen, call into triage.Decide(beadID, mode=review_block, history) to choose the next action.
- Track the per-bead BLOCK count by reading prior 'review' events with Summary='BLOCK' from the bead store. No new persistent state needed.
- Implement the three actions for review_block:
  - re_attempt_with_context: leave bead reopened (current behavior) so next drain pass picks it up; ensure the prompt assembler picks up the BLOCK rationale (already in bead notes via Reopen()).
  - escalate_tier: set a metadata hint or tier-pin so the next attempt runs at a higher tier (use existing escalation.ModelTier mechanism).
  - needs_human: add 'needs_human' label to the bead and emit a triage-decision event.
- Surface review-pairing-degraded events in the triage decision (AC#6 of parent): when the latest BLOCK was paired with a degraded reviewer, prefer re_attempt_with_context with a fresh reviewer pair rather than escalating.

Out of scope:
- Lock-contention retry path (next child).
- no_changes / execution_failed triage paths (this child is review-only).
    </description>
    <acceptance>
1. Loop calls triage.Decide() after VerdictBlock and VerdictRequestChanges branches in execute_bead_loop.go. 2. BLOCK ladder progression works: 1st BLOCK → re-attempt; 2nd → tier escalation hint applied; 3rd → needs_human label + triage-decision event. 3. review-pairing-degraded events found in bead history are passed into the decision context. 4. Tests for BLOCK→re-attempt, BLOCK→escalate at 2nd attempt, BLOCK→needs_human at 3rd, and review-pairing-degraded biases toward re-attempt.
    </acceptance>
    <labels>phase:2,  story:10,  area:agent,  kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T132821-5c8c3373/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e6a354d12f417ada9fa6b248355083457cdf3d7b">
diff --git a/.ddx/executions/20260502T132821-5c8c3373/result.json b/.ddx/executions/20260502T132821-5c8c3373/result.json
new file mode 100644
index 00000000..ef0c8248
--- /dev/null
+++ b/.ddx/executions/20260502T132821-5c8c3373/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-5f67936e",
+  "attempt_id": "20260502T132821-5c8c3373",
+  "base_rev": "444be02cc06a4c34e634a53ba7adb32aee06391f",
+  "result_rev": "9c948479a84dc5b4eb7902eb1f19818697258aba",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2e08cc8c",
+  "duration_ms": 464382,
+  "tokens": 23585,
+  "cost_usd": 2.8269717500000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T132821-5c8c3373",
+  "prompt_file": ".ddx/executions/20260502T132821-5c8c3373/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T132821-5c8c3373/manifest.json",
+  "result_file": ".ddx/executions/20260502T132821-5c8c3373/result.json",
+  "usage_file": ".ddx/executions/20260502T132821-5c8c3373/usage.json",
+  "started_at": "2026-05-02T13:28:23.221447848Z",
+  "finished_at": "2026-05-02T13:36:07.604335923Z"
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
