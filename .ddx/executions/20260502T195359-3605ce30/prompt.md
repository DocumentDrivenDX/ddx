<bead-review>
  <bead id="ddx-b24e9630" iter=1>
    <title>execute-bead: distinguish 'AC verified, nothing to do' from 'gave up without progress'; do not silently close on no_changes</title>
    <description>
Today execute-bead's no_changes outcome conflates two cases:
1. Legitimate already_satisfied: the AC is currently met by prior commits; agent correctly says 'nothing to do'
2. Silent failure: agent didn't know how to do the work, gave up, wrote no_changes_rationale.txt; loop closes the bead anyway

Both result in the same artifact (no_changes_rationale.txt) and the same close. The second case is a serious failure mode masquerading as success.

**Constraint (per ADR-004)**: bd/br compatibility forbids adding new persisted status values. The bead's status stays in the canonical 6 (open/in_progress/closed/blocked/proposed/cancelled). DDx-specific execution semantics live in LABELS or EVENTS, NOT in new statuses.

Fix has two halves:

A) Execute-bead PROMPT change (cli/internal/agent/execute_bead.go executeBeadInstructionsClaudeText / AgentText):
- When the agent decides 'no changes', it MUST include in the no_changes_rationale.txt one of:
  - verification_command: &lt;shell command that proves the AC is currently met&gt; AND its output showing pass
  - status: needs_investigation + reason: &lt;why I cannot make progress&gt;
- Bare 'no changes' without either is rejected by the loop

B) LOOP-side verification (cli/internal/agent/execute_bead_loop.go):
- Parse no_changes_rationale.txt
- If verification_command present: run it; if exit 0, legitimate already_satisfied → close. If non-zero, fail to verify → bead STAYS OPEN with label 'triage:no-changes-unverified' added + emit no_changes_unverified event
- If status: needs_investigation: bead STAYS OPEN with label 'triage:needs-investigation' added (NOT a status change), event captures the agent's reason
- If neither present: reject — bead stays open with label 'triage:no-changes-unjustified' added + emit no_changes_unjustified event

Status NEVER changes to needs_investigation (that would violate bd/br compatibility). Auto-triage (ddx-3c154349) consumes the labels + events to drive its triage policy.
    </description>
    <acceptance>
1. Execute-bead prompt requires verification_command OR explicit needs-investigation marker in any no_changes_rationale.txt. 2. Loop verifies the verification_command before closing as already_satisfied. 3. Implementation aligns exactly with TD-NNN NoChangesContract (per ddx-673833f4) — uses LABELS not new statuses (e.g., 'triage:needs-investigation', 'triage:no-changes-unverified', 'triage:no-changes-unjustified'). **bd/br round-trip via ddx bead export must produce a bd-importable record after this bead lands.** 4. New events use canonical names from TD: no_changes_unverified, no_changes_unjustified, no_changes_verified. 5. Tests cover all paths: verified→close, unverified→stay-open+label, unjustified→stay-open+label, needs-investigation→stay-open+label. 6. Existing beads currently in 'closed via no_changes' state are not retroactively reopened. 7. Auto-triage (ddx-3c154349) consumes the triage:* labels per TriageContract. 8. **Verify schema_compat_test.go passes.**
    </acceptance>
    <labels>phase:2, story:10, area:agent, kind:fix, observed-failure</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T192946-f3e84c05/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="432b3f22065a783015e249c3008f3012693af9cf">
diff --git a/.ddx/executions/20260502T192946-f3e84c05/result.json b/.ddx/executions/20260502T192946-f3e84c05/result.json
new file mode 100644
index 00000000..4bab1b79
--- /dev/null
+++ b/.ddx/executions/20260502T192946-f3e84c05/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b24e9630",
+  "attempt_id": "20260502T192946-f3e84c05",
+  "base_rev": "a6105492fabd023966ef3cbd66f98687b359204f",
+  "result_rev": "b543f08488d5b62129e694111bc1ca8704131346",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0fafa611",
+  "duration_ms": 1445097,
+  "tokens": 65535,
+  "cost_usd": 10.580965749999995,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T192946-f3e84c05",
+  "prompt_file": ".ddx/executions/20260502T192946-f3e84c05/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T192946-f3e84c05/manifest.json",
+  "result_file": ".ddx/executions/20260502T192946-f3e84c05/result.json",
+  "usage_file": ".ddx/executions/20260502T192946-f3e84c05/usage.json",
+  "started_at": "2026-05-02T19:29:47.543086955Z",
+  "finished_at": "2026-05-02T19:53:52.641054576Z"
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
