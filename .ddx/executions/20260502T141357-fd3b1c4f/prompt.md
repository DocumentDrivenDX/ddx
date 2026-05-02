<bead-review>
  <bead id="ddx-6d132f62" iter=1>
    <title>Story 16.0: Backend run-detail resolver + tool-call persistence + bundle-file confinement</title>
    <description>
Persist normalized ToolCallEntry at drain time onto AgentSession/Run detail backing (extend existing drain in service_run.go / agent_runner_service.go). Extend Run GraphQL resolver with: prompt, response, stderr, toolCalls(first, after), bundleFiles[] (path, size, mimeType), bundleFile(path) -&gt; {content, truncated, sizeBytes}. Bundle-root confinement: canonical-path check rejecting any path outside &lt;projectRoot&gt;/.ddx/executions/&lt;runId&gt;/. Whitelist enforcement for inline content (allowed extensions: *.txt, *.md, manifest.json, prompt.md, result.json; &lt;64KB). Resolver unit tests cover: path traversal (.., symlink, absolute), size cap (&gt;64KB returns truncated=true with sizeBytes), whitelist enforcement, tool-call persistence-at-drain (not log parsing). See /tmp/story-16-final.md.
    </description>
    <acceptance>
Run resolver exposes prompt/response/stderr/toolCalls(first,after)/bundleFiles[]/bundleFile(path). ToolCallEntry persisted at drain time (not parsed from logs at resolver time). bundleFile rejects path traversal, symlink escape, absolute paths outside bundle root with 404. Files &gt;64KB or non-whitelisted extensions return truncated=true,sizeBytes=N (no content). Unit tests cover all of the above. Existing drain code in service_run.go / agent_runner_service.go updated to write normalized ToolCallEntry to detail backing.
    </acceptance>
    <labels>phase:2, story:16</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T135904-ecd434db/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2f9f23d20596ad09cb60de0466b4699a033ab3b1">
diff --git a/.ddx/executions/20260502T135904-ecd434db/result.json b/.ddx/executions/20260502T135904-ecd434db/result.json
new file mode 100644
index 00000000..06d6ef70
--- /dev/null
+++ b/.ddx/executions/20260502T135904-ecd434db/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-6d132f62",
+  "attempt_id": "20260502T135904-ecd434db",
+  "base_rev": "8d9483cf5d6253469022eb4ccad7e1af45f59ed2",
+  "result_rev": "0b1b396470224e5f872052aba04e8d6f6f924e9a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-b8d55f46",
+  "duration_ms": 886434,
+  "tokens": 39188,
+  "cost_usd": 7.882867249999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T135904-ecd434db",
+  "prompt_file": ".ddx/executions/20260502T135904-ecd434db/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T135904-ecd434db/manifest.json",
+  "result_file": ".ddx/executions/20260502T135904-ecd434db/result.json",
+  "usage_file": ".ddx/executions/20260502T135904-ecd434db/usage.json",
+  "started_at": "2026-05-02T13:59:05.945271889Z",
+  "finished_at": "2026-05-02T14:13:52.379974846Z"
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
