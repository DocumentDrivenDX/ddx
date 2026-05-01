<bead-review>
  <bead id="ddx-2940b60d" iter=1>
    <title>beads layout: h1, filter chips, table body use raw text-xs/text-sm/text-xl instead of semantic tokens</title>
    <description>
src/routes/nodes/[nodeId]/projects/[projectId]/beads/+layout.svelte uses raw Tailwind size classes instead of semantic tokens:

Line 307: h1 uses text-xl instead of text-headline-lg (20px/800)
Lines 309, 477: text-sm instead of text-body-sm (13px)
Lines 327: text-sm for search input instead of text-body-sm
Lines 333, 357, 382: text-xs for filter labels instead of text-body-sm or text-label-caps
Lines 292-293: chip classes use text-xs instead of text-label-caps (chips are label/badge contexts)
Line 438: font-mono text-xs for ID column — should be font-mono-code text-mono-code (13px)
Line 471: font-mono text-xs for priority column — should be font-mono-code text-mono-code
Line 540: create panel h2 uses text-base — should be text-headline-md

The beads layout also has inconsistent chip sizing: chipClass() uses text-xs (raw) for both active and inactive states; the runs page uses the same pattern. These chips are filter toggles, not label-caps badges, but they should use a consistent token regardless.
    </description>
    <acceptance>
All raw text-xs, text-sm, text-xl, text-base in beads/+layout.svelte replaced with appropriate semantic tokens. font-mono replaced with font-mono-code. Chip classes use text-label-caps (11px/700) since chips in this context function as label/filter badges.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T142443-9f2f695f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5aaa30d94a4634298d101c7aa71763384356fe29">
diff --git a/.ddx/executions/20260501T142443-9f2f695f/result.json b/.ddx/executions/20260501T142443-9f2f695f/result.json
new file mode 100644
index 00000000..5f1afc3e
--- /dev/null
+++ b/.ddx/executions/20260501T142443-9f2f695f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2940b60d",
+  "attempt_id": "20260501T142443-9f2f695f",
+  "base_rev": "7d0b0d8e0342c05810f50cd19a8a5137f8431d72",
+  "result_rev": "9901732ad842166ecab8ec4de9a18e4f9e3ddc67",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-eba3f78e",
+  "duration_ms": 158100,
+  "tokens": 8437,
+  "cost_usd": 1.05934275,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T142443-9f2f695f",
+  "prompt_file": ".ddx/executions/20260501T142443-9f2f695f/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T142443-9f2f695f/manifest.json",
+  "result_file": ".ddx/executions/20260501T142443-9f2f695f/result.json",
+  "usage_file": ".ddx/executions/20260501T142443-9f2f695f/usage.json",
+  "started_at": "2026-05-01T14:24:44.773940011Z",
+  "finished_at": "2026-05-01T14:27:22.874084955Z"
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
