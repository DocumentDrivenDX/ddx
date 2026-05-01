<bead-review>
  <bead id="ddx-e658e2ae" iter=1>
    <title>BeadForm: input text uses text-sm; select status uses inline style with CSS vars that don't exist</title>
    <description>
src/lib/components/BeadForm.svelte has two issues:

1. Line 130: inputClass uses text-sm — should be text-body-sm (13px/400). Similarly line 131: labelClass uses text-xs — should be text-label-caps (11px/700) since these are uppercase field labels.

2. Lines 162-163: The status &lt;select&gt; applies an inline style:
   style="background-color: var(--status-{status}-surface); border-color: var(--status-{status}-border);"
   These CSS variables (--status-open-surface, --status-open-border, etc.) may not be defined for all status values consistently, and using raw CSS variable injection via a template string is fragile. This sidesteps the Tailwind token system entirely. The dynamic color should be achieved through Tailwind class composition (as done everywhere else in the codebase) rather than inline styles.

3. Cancel/Submit button text uses text-sm — should be text-body-sm.
    </description>
    <acceptance>
inputClass uses text-body-sm, labelClass uses text-label-caps. Status select color is expressed through Tailwind classes, not inline style CSS variable injection. Button text uses text-body-sm.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T143448-88423c71/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="14b7587479c1b111fa60142f8fc4a517dcd5b27f">
diff --git a/.ddx/executions/20260501T143448-88423c71/result.json b/.ddx/executions/20260501T143448-88423c71/result.json
new file mode 100644
index 00000000..6efb528b
--- /dev/null
+++ b/.ddx/executions/20260501T143448-88423c71/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-e658e2ae",
+  "attempt_id": "20260501T143448-88423c71",
+  "base_rev": "c6b168485ed21082da1594664823ddc6f2550c39",
+  "result_rev": "b9b7932d5e53353cd64ea52337c935da76ae88db",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c70f7d05",
+  "duration_ms": 89645,
+  "tokens": 6989,
+  "cost_usd": 0.7515947500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T143448-88423c71",
+  "prompt_file": ".ddx/executions/20260501T143448-88423c71/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T143448-88423c71/manifest.json",
+  "result_file": ".ddx/executions/20260501T143448-88423c71/result.json",
+  "usage_file": ".ddx/executions/20260501T143448-88423c71/usage.json",
+  "started_at": "2026-05-01T14:34:49.84600653Z",
+  "finished_at": "2026-05-01T14:36:19.491192643Z"
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
