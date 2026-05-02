<bead-review>
  <bead id=".execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-2b64cb0a" iter=1>
    <title>Triage integration: lock contention retry-with-backoff + execution_failed/no_changes paths</title>
    <description>
Third child of ddx-3c154349. Depends on the triage framework child. Covers the remaining failure modes.

Scope:
- Lock contention: detect 'Unable to create .git/index.lock' / staging-tracker lock errors in attempt outcomes. Triage action: retry_with_backoff (e.g. 30s, 90s, 300s) within the same drain pass before unclaiming. Sibling bead handles the underlying lock fix; this bead wires the retry decision.
- execution_failed (substantive, not infrastructure): triage maps to escalate_tier per the existing S10 ladder; on exhaustion → needs_human. Distinguish from already-handled escalatable statuses by treating execution_failed as an explicit triage entry-point.
- no_changes (NoChangesRationale present): triage decides whether the bead AC is genuinely satisfied (close already_satisfied — current behavior) or whether to file a clarification follow-up bead (when rationale doesn't quote AC items).
- Tests for: lock-error → retry succeeds on second attempt; lock-error → escalate after backoff exhaustion; execution_failed → escalate_tier; no_changes with weak rationale → file follow-up.

Out of scope:
- The actual git-lock infrastructure fix (sibling bead).
- Review BLOCK/REQUEST_CHANGES (sibling child).
    </description>
    <acceptance>
1. Lock contention errors classified and routed to retry_with_backoff with at least 3 backoff steps before escalation. 2. execution_failed enters triage and produces escalate_tier or needs_human depending on attempt count. 3. no_changes with a rationale that does not cite AC items results in a clarification follow-up bead being filed (and the original closed). 4. Tests cover lock-error retry success, lock-error backoff exhaustion, execution_failed escalation, no_changes follow-up.
    </acceptance>
    <labels>phase:2,  story:10,  area:agent,  kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T132857-7e5c2228/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9f058d99acb3f7eca64feb39cc8d69d03e607b75">
diff --git a/.ddx/executions/20260502T132857-7e5c2228/result.json b/.ddx/executions/20260502T132857-7e5c2228/result.json
new file mode 100644
index 00000000..5c1f75cf
--- /dev/null
+++ b/.ddx/executions/20260502T132857-7e5c2228/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-2b64cb0a",
+  "attempt_id": "20260502T132857-7e5c2228",
+  "base_rev": "3f01204dcdd4565f229c551bd384a1e67cd2eda3",
+  "result_rev": "e188d9d328252379837926e4c7c50f001e9f805c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-89c7f0da",
+  "duration_ms": 698962,
+  "tokens": 34014,
+  "cost_usd": 4.116576,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T132857-7e5c2228",
+  "prompt_file": ".ddx/executions/20260502T132857-7e5c2228/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T132857-7e5c2228/manifest.json",
+  "result_file": ".ddx/executions/20260502T132857-7e5c2228/result.json",
+  "usage_file": ".ddx/executions/20260502T132857-7e5c2228/usage.json",
+  "started_at": "2026-05-02T13:28:59.640591023Z",
+  "finished_at": "2026-05-02T13:40:38.603229071Z"
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
