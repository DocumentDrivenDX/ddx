<bead-review>
  <bead id="ddx-65543530" iter=1>
    <title>runs: canonicalize Run data projection (fix state_runs.go bundle layering; lossless AgentSession join)</title>
    <description>
state_runs.go:164 currently synthesizes execution bundles as RunLayerRun; per the three-layer model bundles should be RunLayerTry. Fix the layer assignment. Add lossless join with AgentSession (which carries billingMode, cached tokens, prompt/response/stderr, outcome — NOT in Run today). User decision: keep AgentSession as backing store under layer=run.
    </description>
    <acceptance>
1. state_runs.go: bundles → RunLayerTry; agent sessions → RunLayerRun. 2. Run resolver returns lossless join (Run + AgentSession fields when applicable). 3. Existing runs tests pass; add new test for layering correctness. 4. cd cli &amp;&amp; go test ./internal/server/... passes.
    </acceptance>
    <labels>phase:2, story:8, area:server, kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T113725-7f3f409d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e80a1b950e0869e3ada49f86224054f3c5614c26">
diff --git a/.ddx/executions/20260502T113725-7f3f409d/result.json b/.ddx/executions/20260502T113725-7f3f409d/result.json
new file mode 100644
index 00000000..8cf2c012
--- /dev/null
+++ b/.ddx/executions/20260502T113725-7f3f409d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-65543530",
+  "attempt_id": "20260502T113725-7f3f409d",
+  "base_rev": "d9bae52c773dcf9014b11f02dd49f90b0568f205",
+  "result_rev": "d37a25c28ce9294de6acc32e799f4f4991ed2147",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0c4ecf30",
+  "duration_ms": 366924,
+  "tokens": 20283,
+  "cost_usd": 2.7755557499999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T113725-7f3f409d",
+  "prompt_file": ".ddx/executions/20260502T113725-7f3f409d/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T113725-7f3f409d/manifest.json",
+  "result_file": ".ddx/executions/20260502T113725-7f3f409d/result.json",
+  "usage_file": ".ddx/executions/20260502T113725-7f3f409d/usage.json",
+  "started_at": "2026-05-02T11:37:26.905039953Z",
+  "finished_at": "2026-05-02T11:43:33.829105151Z"
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
