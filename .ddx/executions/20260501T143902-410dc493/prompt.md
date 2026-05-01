<bead-review>
  <bead id="ddx-4006df47" iter=1>
    <title>workers layout: text-lever on ID column should be text-accent-lever; raw text-sm/text-xs present</title>
    <description>
src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte has these token issues:

1. Line 382: ID column uses text-lever (a CSS utility defined in app.css pointing to --status-open-text). This is semantically wrong: it should use text-accent-lever / dark:text-dark-accent-lever, which is how bead IDs are styled in other pages (e.g. executions, runs). text-lever exists as a legacy utility; it should not be used in new code.

2. Line 220: drain worker count uses text-3xl — raw size; should be text-headline-lg (20px/800) or an explicit heading scale token. The '3xl' weight is too large for the defined headline scale but is not in the token set at all.

3. Lines 216-219 drain panel uses text-label-caps correctly but also text-3xl without a paired font-headline token.

4. The file also uses text-sm (lines in start-worker form) where text-body-sm should be used.

5. Line 246 Remove worker button uses text-status-failed inline — should use text-error / dark:text-dark-error to match the semantic error pattern used elsewhere.
    </description>
    <acceptance>
ID column uses text-accent-lever dark:text-dark-accent-lever. text-lever removed. Drain count uses an appropriate headline token. text-sm replaced with text-body-sm. Remove button uses text-error / dark:text-dark-error.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T143640-a371c3f3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b2565d55e51b8c77a46c9c36529b460b387b3d06">
diff --git a/.ddx/executions/20260501T143640-a371c3f3/result.json b/.ddx/executions/20260501T143640-a371c3f3/result.json
new file mode 100644
index 00000000..9c658990
--- /dev/null
+++ b/.ddx/executions/20260501T143640-a371c3f3/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4006df47",
+  "attempt_id": "20260501T143640-a371c3f3",
+  "base_rev": "a9d0f126728bdb0956ca1c5dd98b0eeaa5aedf1e",
+  "result_rev": "257058e3724d53a55f4b92a813197858e4b8b8bd",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-070ed810",
+  "duration_ms": 137302,
+  "tokens": 8160,
+  "cost_usd": 1.0885470000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T143640-a371c3f3",
+  "prompt_file": ".ddx/executions/20260501T143640-a371c3f3/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T143640-a371c3f3/manifest.json",
+  "result_file": ".ddx/executions/20260501T143640-a371c3f3/result.json",
+  "usage_file": ".ddx/executions/20260501T143640-a371c3f3/usage.json",
+  "started_at": "2026-05-01T14:36:41.809949985Z",
+  "finished_at": "2026-05-01T14:38:59.112543604Z"
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
