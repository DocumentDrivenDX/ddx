<bead-review>
  <bead id="ddx-7b4a39f5" iter=1>
    <title>IntegrityPanel: badge-status-blocked used for non-status UI (IDs, apply buttons); missing dark mode on status-merged/status-blocked prose colors</title>
    <description>
src/lib/components/IntegrityPanel.svelte has several token misuse issues:

1. Lines 274, 295, 315: badge-status-blocked is applied to UI elements that are NOT status badges — an issue ID chip (line 274), a 'Copy suggested unique ID' button (line 295), and an 'Apply fix' button (line 315). badge-status-blocked is a semantic badge class for the 'blocked' bead status; repurposing it for arbitrary UI elements breaks the semantic contract.

2. Lines 332-333: The diff preview uses text-status-blocked (before) and text-status-merged (after). text-status-merged may not be a defined token — check app.css. If it is a CSS utility, it lacks a dark: variant pairing that matches the dark-mode pattern used everywhere else.

3. Lines 338-339: error text uses text-status-blocked which is semantically wrong — a repair error should use text-error / dark:text-dark-error.

4. The panel header (line 215) uses font-semibold directly without a semantic typography class.
    </description>
    <acceptance>
badge-status-blocked is removed from non-status-badge elements. Issue ID chips get a neutral style (bg-bg-canvas border-border-line text-fg-muted). Action buttons use accent/semantic button patterns. Diff text uses appropriate semantic colors with dark variants. Error text uses text-error / dark:text-dark-error. Panel header uses text-headline-md font-headline-md.
    </acceptance>
    <labels>area:ui, kind:design, design-tokens</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T143140-719e465c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5722538e2acef809431ee7c1e44f6fabaa40a8f1">
diff --git a/.ddx/executions/20260501T143140-719e465c/result.json b/.ddx/executions/20260501T143140-719e465c/result.json
new file mode 100644
index 00000000..43d26002
--- /dev/null
+++ b/.ddx/executions/20260501T143140-719e465c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-7b4a39f5",
+  "attempt_id": "20260501T143140-719e465c",
+  "base_rev": "ea0ee1e4d5ca550971c89bdf50f7a4e3e59e7f73",
+  "result_rev": "4f347a98ddcb7d50937975c8cbbf3b5e588d6650",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-82560829",
+  "duration_ms": 167275,
+  "tokens": 8917,
+  "cost_usd": 1.2375699999999996,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T143140-719e465c",
+  "prompt_file": ".ddx/executions/20260501T143140-719e465c/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T143140-719e465c/manifest.json",
+  "result_file": ".ddx/executions/20260501T143140-719e465c/result.json",
+  "usage_file": ".ddx/executions/20260501T143140-719e465c/usage.json",
+  "started_at": "2026-05-01T14:31:41.777193039Z",
+  "finished_at": "2026-05-01T14:34:29.052435664Z"
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
