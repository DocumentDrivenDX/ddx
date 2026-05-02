<bead-review>
  <bead id="ddx-51ac997b" iter=1>
    <title>escalation: ladder.Next (MinPower-only output) + tests</title>
    <description>
Implement the escalation ladder as MinPower-only floors. NO vendor names. ladder.Next picks next floor on substantive failure. Skip-tier (no viable provider at floor) returns typed NoViableProviderError so the loop bumps further. Uses MinPower/MaxPower fields on ExecuteRequest that ALREADY exist in fizeau v0.9.28 (agent-79e194aa closed) — no upstream work needed for this bead. Reads previous attempt's RoutingActual.Power (lands via agent-3bb96bf5 / S10_1) to compute next floor.
    </description>
    <acceptance>
1. Ladder type + Next() method in cli/internal/agent/escalation/. 2. Unit tests cover all transitions. 3. Skip-tier returns typed NoViableProviderError. 4. Tests assert ladder uses RoutingActual.Power as the input for next-floor computation.
    </acceptance>
    <labels>phase:2, story:10, area:routing, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T134946-d770eb2c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="87890fc42a643cb2bfbb5d3b7e5b631c9ac95b0f">
diff --git a/.ddx/executions/20260502T134946-d770eb2c/result.json b/.ddx/executions/20260502T134946-d770eb2c/result.json
new file mode 100644
index 00000000..00cb8162
--- /dev/null
+++ b/.ddx/executions/20260502T134946-d770eb2c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-51ac997b",
+  "attempt_id": "20260502T134946-d770eb2c",
+  "base_rev": "7dcf538522fd2677c60539677d6afa8a0eeaa112",
+  "result_rev": "c71355805f69274d9d77bb18aae34d2cefec140f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6e0d9d40",
+  "duration_ms": 349350,
+  "tokens": 20,
+  "cost_usd": 1.5333110000000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T134946-d770eb2c",
+  "prompt_file": ".ddx/executions/20260502T134946-d770eb2c/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T134946-d770eb2c/manifest.json",
+  "result_file": ".ddx/executions/20260502T134946-d770eb2c/result.json",
+  "usage_file": ".ddx/executions/20260502T134946-d770eb2c/usage.json",
+  "started_at": "2026-05-02T13:49:48.146239176Z",
+  "finished_at": "2026-05-02T13:55:37.497219911Z"
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
