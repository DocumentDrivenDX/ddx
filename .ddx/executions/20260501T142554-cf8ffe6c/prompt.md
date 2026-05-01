<bead-review>
  <bead id="ddx-d0e8888b" iter=1>
    <title>runs pages: replace hardcoded purple-/blue-/teal- layer badge colors with semantic tokens</title>
    <description>
Two files use raw Tailwind palette colors for layer badges (work/try/run) that are not semantic tokens:

1. src/routes/nodes/[nodeId]/projects/[projectId]/runs/+page.svelte layerBadgeClass() function (lines 97-108):
   - 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300' (work)
   - 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300' (try)
   - 'bg-teal-100 text-teal-800 dark:bg-teal-900/30 dark:text-teal-300' (run)

2. src/routes/nodes/[nodeId]/projects/[projectId]/runs/[runId]/+page.svelte layerBadgeClass() function (lines 43-54): same three color sets.

These layer values (work/try/run) are semantic concepts in the DDx run hierarchy and should have named tokens or at minimum use existing semantic accent/status tokens consistently. The layer badge also uses rounded-full pill shape while status badges are sharp-cornered, which is inconsistent.

Additionally runs/+page.svelte lines 134-135 use raw text-xl and text-sm instead of text-headline-lg and text-body-sm.
    </description>
    <acceptance>
layerBadgeClass() in both runs pages no longer references purple-*, blue-*, or teal-* Tailwind palette classes. Layer colors are expressed via semantic tokens or a defined CSS utility class (e.g. badge-layer-work/try/run) added to app.css using existing CSS variable patterns. Raw text-xl/text-sm replaced with text-headline-lg/text-body-sm.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T142332-57f145f7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="85aa04fcb2be1c80202e19f8ebace1b142c3215d">
diff --git a/.ddx/executions/20260501T142332-57f145f7/result.json b/.ddx/executions/20260501T142332-57f145f7/result.json
new file mode 100644
index 00000000..d8a2cfb8
--- /dev/null
+++ b/.ddx/executions/20260501T142332-57f145f7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d0e8888b",
+  "attempt_id": "20260501T142332-57f145f7",
+  "base_rev": "c9de4479dde79d216e7d582c853a9e006fc45fc3",
+  "result_rev": "12ad0bddba3a0695dc29351e1474d2764656088d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-dab52333",
+  "duration_ms": 135556,
+  "tokens": 8123,
+  "cost_usd": 0.9791107499999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T142332-57f145f7",
+  "prompt_file": ".ddx/executions/20260501T142332-57f145f7/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T142332-57f145f7/manifest.json",
+  "result_file": ".ddx/executions/20260501T142332-57f145f7/result.json",
+  "usage_file": ".ddx/executions/20260501T142332-57f145f7/usage.json",
+  "started_at": "2026-05-01T14:23:33.779184773Z",
+  "finished_at": "2026-05-01T14:25:49.335484241Z"
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
