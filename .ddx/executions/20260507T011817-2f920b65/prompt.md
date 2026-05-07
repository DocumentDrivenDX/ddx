<bead-review>
  <bead id="ddx-23cbcb4b" iter=1>
    <title>artifacts: TD-NNN artifact search semantics (title→path→description→frontmatter→body)</title>
    <description>
Author TD documenting artifact search semantics: scope progression (title → path → description → frontmatter → body), size caps, binary skip, perf benchmark. Body search gated on this TD's recommendations.
    </description>
    <acceptance>
1. docs/helix/02-design/technical-designs/TD-&lt;NNN&gt;-artifact-search.md exists. 2. Defines scope, size caps, binary handling, perf budget. 3. ddx doc audit clean.
    </acceptance>
    <labels>phase:2, story:6, area:specs, kind:design</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260507T011649-18512b9a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="048c8d60f366692e6c0b158e2f2f27d9757680a9">
<untrusted-data>
diff --git a/.ddx/executions/20260507T011649-18512b9a/result.json b/.ddx/executions/20260507T011649-18512b9a/result.json
new file mode 100644
index 000000000..c6757c7f0
--- /dev/null
+++ b/.ddx/executions/20260507T011649-18512b9a/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-23cbcb4b",
+  "attempt_id": "20260507T011649-18512b9a",
+  "base_rev": "7c1fee78f84cf47f73f564996d9fd7ea799744f3",
+  "result_rev": "4612750cf56cd307983e8728670193560157ff2d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-e93e2415",
+  "duration_ms": 72871,
+  "tokens": 907710,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260507T011649-18512b9a",
+  "prompt_file": ".ddx/executions/20260507T011649-18512b9a/prompt.md",
+  "manifest_file": ".ddx/executions/20260507T011649-18512b9a/manifest.json",
+  "result_file": ".ddx/executions/20260507T011649-18512b9a/result.json",
+  "usage_file": ".ddx/executions/20260507T011649-18512b9a/usage.json",
+  "started_at": "2026-05-07T01:16:52.413811882Z",
+  "finished_at": "2026-05-07T01:18:05.285051469Z"
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
