<bead-review>
  <bead id="ddx-7e7b12f0" iter=1>
    <title>test(web): e2e regression check for doc graph edge color tokens</title>
    <description>
Add Playwright e2e at cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/graph/graph.e2e.ts that loads the doc graph route, asserts at least one &lt;line&gt; edge exists, and verifies its computed stroke resolves to rgb(75, 85, 99) (light fg-muted #4B5563) and the marker path fill matches. Then toggle dark class on &lt;html&gt; and re-assert against #B8AF9C. Sibling pattern: drain.e2e.ts, personas.e2e.ts. Theme-toggle plumbing may need addInitScript to set theme pre-load. Fixture: needs a project with &gt;=1 dependency edge in the GraphQL mock; add fixture if none exists. See /tmp/story-1-final.md (bead-B).
    </description>
    <acceptance>
AC1: graph.e2e.ts file exists at the specified route directory.
AC2: Test asserts at least one &lt;line&gt; SVG edge is rendered.
AC3: Light-theme assertion: edge stroke computed style equals rgb(75, 85, 99); arrow marker path fill matches.
AC4: Dark-theme assertion (after toggling html.dark): edge stroke computed style equals the rgb form of #B8AF9C; marker fill matches.
AC5: Test passes locally via bun run test:e2e.
    </acceptance>
    <labels>phase:2,  story:1,  tier:standard</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T141457-52fe30ca/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="af4bbcdb696bd329fb5c1841aa7f4c38fff78e88">
diff --git a/.ddx/executions/20260502T141457-52fe30ca/result.json b/.ddx/executions/20260502T141457-52fe30ca/result.json
new file mode 100644
index 00000000..07ec3e2a
--- /dev/null
+++ b/.ddx/executions/20260502T141457-52fe30ca/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-7e7b12f0",
+  "attempt_id": "20260502T141457-52fe30ca",
+  "base_rev": "c1741ea9e0b6c76d77c83b58204fdaae294e0b08",
+  "result_rev": "3b0fefc1afbb496f3207f81f14e144d80d68d145",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-97c25179",
+  "duration_ms": 473031,
+  "tokens": 26517,
+  "cost_usd": 4.001205250000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T141457-52fe30ca",
+  "prompt_file": ".ddx/executions/20260502T141457-52fe30ca/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T141457-52fe30ca/manifest.json",
+  "result_file": ".ddx/executions/20260502T141457-52fe30ca/result.json",
+  "usage_file": ".ddx/executions/20260502T141457-52fe30ca/usage.json",
+  "started_at": "2026-05-02T14:14:59.719968483Z",
+  "finished_at": "2026-05-02T14:22:52.751061686Z"
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
