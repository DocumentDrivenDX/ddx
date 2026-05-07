<bead-review>
  <bead id="ddx-cd42fc05" iter=1>
    <title>metric: FEAT-014/FEAT-016 cross-reference edits</title>
    <description>
Cross-reference FEAT-014 (token awareness) and FEAT-016 (process metrics) with the new MET artifact convention.
    </description>
    <acceptance>
1. Both FEATs have explicit cross-refs to MET artifacts. 2. ddx doc audit clean.
    </acceptance>
    <labels>phase:2, story:13, area:specs, kind:doc</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260507T022437-88a063be/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="13170e595e19a69fe4a49c58f9f93fc2a9e11ebf">
<untrusted-data>
diff --git a/.ddx/executions/20260507T022437-88a063be/result.json b/.ddx/executions/20260507T022437-88a063be/result.json
new file mode 100644
index 000000000..3114df30c
--- /dev/null
+++ b/.ddx/executions/20260507T022437-88a063be/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-cd42fc05",
+  "attempt_id": "20260507T022437-88a063be",
+  "base_rev": "a30d73af9a201dcb2494d3693a4cb0471d7d067e",
+  "result_rev": "a626057a5d2b68bfb36f217a52695cedd9ed2d98",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-27ece786",
+  "duration_ms": 105312,
+  "tokens": 1062256,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260507T022437-88a063be",
+  "prompt_file": ".ddx/executions/20260507T022437-88a063be/prompt.md",
+  "manifest_file": ".ddx/executions/20260507T022437-88a063be/manifest.json",
+  "result_file": ".ddx/executions/20260507T022437-88a063be/result.json",
+  "usage_file": ".ddx/executions/20260507T022437-88a063be/usage.json",
+  "started_at": "2026-05-07T02:24:40.326691548Z",
+  "finished_at": "2026-05-07T02:26:25.638734147Z"
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
