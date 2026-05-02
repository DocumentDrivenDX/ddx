<bead-review>
  <bead id="ddx-6f41aa55" iter=1>
    <title>fix(web): doc graph edges and arrowheads use foreground token, not border-line</title>
    <description>
Swap edge stroke and arrow marker fill in cli/internal/server/frontend/src/lib/components/D3Graph.svelte from border-line/dark-border-line tokens to fg-muted/dark-fg-muted, so edges meet WCAG AA 3:1 contrast for non-text graphics. Surface: D3Graph.svelte lines 107 (arrow marker fill) and 165 (edge line stroke). Bump stroke-width 1.5 -&gt; 1.75 and stroke-opacity 0.6 -&gt; 0.9; add stroke-linecap="round". Sole consumer is routes/nodes/[nodeId]/projects/[projectId]/graph/+page.svelte. See /tmp/story-1-final.md (bead-A).
    </description>
    <acceptance>
AC1: D3Graph.svelte line 107 arrow marker uses fill-fg-muted dark:fill-dark-fg-muted (not border-line).
AC2: D3Graph.svelte line 165 edge &lt;line&gt; uses stroke-fg-muted dark:stroke-dark-fg-muted.
AC3: stroke-width is 1.75, stroke-opacity is 0.9, stroke-linecap="round" present on edges.
AC4: bun run build (or equivalent typecheck) succeeds in cli/internal/server/frontend.
AC6: No other files modified outside D3Graph.svelte.
AC7: Light theme edge color resolves to fg-muted (#4B5563) and dark to dark-fg-muted (#B8AF9C) — verified via tailwind.config.js token lookup.
    </acceptance>
    <labels>phase:2,  story:1,  tier:cheap</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T141310-dfab3875/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="17ca8a9c60c167474e3c0f569b4d857d6cfe09f3">
diff --git a/.ddx/executions/20260502T141310-dfab3875/result.json b/.ddx/executions/20260502T141310-dfab3875/result.json
new file mode 100644
index 00000000..babfa9b7
--- /dev/null
+++ b/.ddx/executions/20260502T141310-dfab3875/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-6f41aa55",
+  "attempt_id": "20260502T141310-dfab3875",
+  "base_rev": "cf0213f92e4ac8eb8edcfb13fb1db6bb936a9e3c",
+  "result_rev": "e94020ef11d3f60409c24bcf1fd9f48dd4e48935",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3eba7098",
+  "duration_ms": 85660,
+  "tokens": 4739,
+  "cost_usd": 0.6711107500000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T141310-dfab3875",
+  "prompt_file": ".ddx/executions/20260502T141310-dfab3875/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T141310-dfab3875/manifest.json",
+  "result_file": ".ddx/executions/20260502T141310-dfab3875/result.json",
+  "usage_file": ".ddx/executions/20260502T141310-dfab3875/usage.json",
+  "started_at": "2026-05-02T14:13:11.603293266Z",
+  "finished_at": "2026-05-02T14:14:37.263630558Z"
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
