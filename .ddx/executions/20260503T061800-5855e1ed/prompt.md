<bead-review>
  <bead id="ddx-2bb01b91" iter=1>
    <title>agentmetrics: GraphQL agentMetrics(window, groupBy) resolver + revision-keyed cache</title>
    <description>
GraphQL resolver agentMetrics(window: 24h|7d|30d, groupBy: MODEL|HARNESS|PROVIDER|TIER) returning attempts, success_rate, mean+p50+p95 duration_ms, mean cost, effective cost/success, in/out tokens, last_seen_at. Revision-keyed cache (survives ADR-004 archive). Default window=7d; all-time removed from UI.
    </description>
    <acceptance>
1. agentMetrics query in GraphQL schema. 2. 4 axes supported (MODEL/HARNESS/PROVIDER/TIER). 3. 3 windows (24h/7d/30d). 4. Cache keyed by run-store revision. 5. Per-axis tests.
    </acceptance>
    <labels>phase:2, story:11, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T060706-03afb7f5/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a45969951e08419679c40ea5d1ba3d4315324026">
diff --git a/.ddx/executions/20260503T060706-03afb7f5/result.json b/.ddx/executions/20260503T060706-03afb7f5/result.json
new file mode 100644
index 00000000..9e33a55c
--- /dev/null
+++ b/.ddx/executions/20260503T060706-03afb7f5/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2bb01b91",
+  "attempt_id": "20260503T060706-03afb7f5",
+  "base_rev": "95deccd10e6c7efd56b63fcdb9810ad08230e195",
+  "result_rev": "57e5e00e833ea711625c557b8786af243a3c1076",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-7258f0f4",
+  "duration_ms": 646398,
+  "tokens": 30332,
+  "cost_usd": 4.6473847500000005,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T060706-03afb7f5",
+  "prompt_file": ".ddx/executions/20260503T060706-03afb7f5/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T060706-03afb7f5/manifest.json",
+  "result_file": ".ddx/executions/20260503T060706-03afb7f5/result.json",
+  "usage_file": ".ddx/executions/20260503T060706-03afb7f5/usage.json",
+  "started_at": "2026-05-03T06:07:08.223765978Z",
+  "finished_at": "2026-05-03T06:17:54.622102954Z"
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
