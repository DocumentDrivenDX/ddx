<bead-review>
  <bead id="ddx-36d175ae" iter=1>
    <title>runs: 302 redirects + internal link sweep + nav drop</title>
    <description>
Sessions and Executions routes return 302 (with sunset header per codex) → /runs?layer=run / ?layer=try. Internal link sweep updates anchors. Nav drops the retired tabs.
    </description>
    <acceptance>
1. Sessions/Executions routes return 302. 2. Sunset header set with deprecation date. 3. Internal links updated. 4. Nav drops Sessions and Executions tabs.
    </acceptance>
    <labels>phase:2, story:8, area:web, kind:cleanup</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T071943-c26e8953/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="db432b0d84b2556f6c1dd52d6c077270334a6bc9">
diff --git a/.ddx/executions/20260503T071943-c26e8953/result.json b/.ddx/executions/20260503T071943-c26e8953/result.json
new file mode 100644
index 00000000..2d02d541
--- /dev/null
+++ b/.ddx/executions/20260503T071943-c26e8953/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-36d175ae",
+  "attempt_id": "20260503T071943-c26e8953",
+  "base_rev": "2261f21930a6643c407a457db7ff997e86e3cc0f",
+  "result_rev": "2aa89947393d9c5e32345f7f8b071c26b4a826e3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-35c52b8f",
+  "duration_ms": 837371,
+  "tokens": 23970,
+  "cost_usd": 4.5874545,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T071943-c26e8953",
+  "prompt_file": ".ddx/executions/20260503T071943-c26e8953/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T071943-c26e8953/manifest.json",
+  "result_file": ".ddx/executions/20260503T071943-c26e8953/result.json",
+  "usage_file": ".ddx/executions/20260503T071943-c26e8953/usage.json",
+  "started_at": "2026-05-03T07:19:44.467294572Z",
+  "finished_at": "2026-05-03T07:33:41.838948053Z"
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
