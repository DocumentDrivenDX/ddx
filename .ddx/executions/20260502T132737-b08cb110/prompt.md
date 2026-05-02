<bead-review>
  <bead id=".execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-0a700bd2" iter=1>
    <title>Triage policy framework: config schema + decision engine</title>
    <description>
Foundation for the auto-triage feature (parent: ddx-3c154349). Build the standalone triage decision module that the drain loop will call after each terminal event, plus its config schema. No loop integration in this bead — that lands in a follow-up child once this module is in place.

Scope:
- New package cli/internal/triage/ with TriagePolicy struct, FailureMode enum (review_block, lock_contention, execution_failed, no_changes), Action enum (re_attempt_with_context, escalate_tier, retry_with_backoff, file_followup, needs_human), and Decide(beadID, mode, history) Action method.
- Default policy mirrors AC#3 of parent: BLOCK→re_attempt_with_context (1st), escalate_tier (2nd), needs_human (3rd). Other modes get sensible defaults.
- Config block 'triage:' added to .ddx/config.yaml schema and ResolvedConfig (cli/internal/config/). Optional; defaults applied when absent.
- Unit tests for the policy engine: ladder progression for BLOCK, default for other modes, config override path.

Out of scope:
- Wiring this into execute_bead_loop.go (next child).
- Lock contention retry mechanics (handled by sibling lock bead + next child).
- Reviewer-pairing-degraded surfacing (next child).
    </description>
    <acceptance>
1. cli/internal/triage package exists with TriagePolicy, FailureMode, Action, and Decide(). 2. Default policy implements BLOCK ladder: re_attempt → escalate_tier → needs_human. 3. .ddx/config.yaml triage: block accepted by schema and unmarshals into ResolvedConfig. 4. Unit tests cover ladder progression, default for non-BLOCK modes, config override of default policy.
    </acceptance>
    <labels>phase:2,  story:10,  area:agent,  kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T131904-3c1cc2d3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2ccb4be45b4446b8d93952ef4719025b05deec14">
diff --git a/.ddx/executions/20260502T131904-3c1cc2d3/result.json b/.ddx/executions/20260502T131904-3c1cc2d3/result.json
new file mode 100644
index 00000000..fa3ef9b4
--- /dev/null
+++ b/.ddx/executions/20260502T131904-3c1cc2d3/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-3c154349-20260502T131050-bf2d17f5-0a700bd2",
+  "attempt_id": "20260502T131904-3c1cc2d3",
+  "base_rev": "f541689dbd781dd1fd979bd2f83d4bc95bbb83e1",
+  "result_rev": "315c555a8cd68e72921afff9bec22807ffcd5655",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-39179aea",
+  "duration_ms": 507668,
+  "tokens": 17363,
+  "cost_usd": 2.4751239999999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T131904-3c1cc2d3",
+  "prompt_file": ".ddx/executions/20260502T131904-3c1cc2d3/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T131904-3c1cc2d3/manifest.json",
+  "result_file": ".ddx/executions/20260502T131904-3c1cc2d3/result.json",
+  "usage_file": ".ddx/executions/20260502T131904-3c1cc2d3/usage.json",
+  "started_at": "2026-05-02T13:19:05.476011024Z",
+  "finished_at": "2026-05-02T13:27:33.144455884Z"
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
