<bead-review>
  <bead id="ddx-345226c9" iter=1>
    <title>artifacts: group-by control + grouping.ts + urlState.ts shared helper</title>
    <description>
Add a group-by selector to the artifacts list page (cli/internal/server/frontend/src/routes/.../artifacts/). Default groupBy=Folder. Implement grouping.ts (group derivations: Folder, Prefix, MediaType) and urlState.ts (shared URL-state helper for q, mediaType, groupBy, sort, filters; preserves unrelated keys + ?back=). Use generic 'Workflow stage' label (not 'HELIX phase'); axis hidden when axisAvailable() is false.
    </description>
    <acceptance>
1. Group-by dropdown visible; default Folder. 2. grouping.ts exports prefixOf, folderOf, workflowStageOf with regex-based derivation. 3. urlState.ts is the single source of truth for URL state. 4. ARIA roles for group headers. 5. sessionStorage persistence with stable keys. 6. cd cli/internal/server/frontend &amp;&amp; bun test passes.
    </acceptance>
    <labels>phase:2, story:5, area:web, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T125032-7a33e609/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="86bbb5bfeb951c8733f81e21e92635584bca0dca">
diff --git a/.ddx/executions/20260502T125032-7a33e609/result.json b/.ddx/executions/20260502T125032-7a33e609/result.json
new file mode 100644
index 00000000..029575d0
--- /dev/null
+++ b/.ddx/executions/20260502T125032-7a33e609/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-345226c9",
+  "attempt_id": "20260502T125032-7a33e609",
+  "base_rev": "c37608177d14941e1e7115d59f4147fd2df69267",
+  "result_rev": "19b3008b1b9bc712bc3834adcac7f79e4006d5f7",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-e2781680",
+  "duration_ms": 333433,
+  "tokens": 19920,
+  "cost_usd": 1.88070925,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T125032-7a33e609",
+  "prompt_file": ".ddx/executions/20260502T125032-7a33e609/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T125032-7a33e609/manifest.json",
+  "result_file": ".ddx/executions/20260502T125032-7a33e609/result.json",
+  "usage_file": ".ddx/executions/20260502T125032-7a33e609/usage.json",
+  "started_at": "2026-05-02T12:50:34.24087308Z",
+  "finished_at": "2026-05-02T12:56:07.67414854Z"
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
