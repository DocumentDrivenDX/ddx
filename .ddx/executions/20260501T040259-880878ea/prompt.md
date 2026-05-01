<bead-review>
  <bead id="ddx-917b4556" iter=1>
    <title>chore: remove root-level DESIGN.md duplicate — canonical source is .stitch/DESIGN.md</title>
    <description/>
    <acceptance>
Root-level DESIGN.md deleted or confirmed non-existent; .stitch/DESIGN.md is the only design language file; all references to DESIGN.md in other files point to .stitch/DESIGN.md
    </acceptance>
    <labels>area:docs, cleanup</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T040106-8afb1cd2/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="4eed144c05e4669b5a4a5cedf63b961a345f490a">
diff --git a/.ddx/executions/20260501T040106-8afb1cd2/result.json b/.ddx/executions/20260501T040106-8afb1cd2/result.json
new file mode 100644
index 00000000..824e26fb
--- /dev/null
+++ b/.ddx/executions/20260501T040106-8afb1cd2/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-917b4556",
+  "attempt_id": "20260501T040106-8afb1cd2",
+  "base_rev": "5792370dc4aeb157abf2daeac745ce16b729dd8a",
+  "result_rev": "5053780f381cc191cee9576af4eb7dc2f3f2af5d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-606219c9",
+  "duration_ms": 107194,
+  "tokens": 8277,
+  "cost_usd": 0.8095882500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T040106-8afb1cd2",
+  "prompt_file": ".ddx/executions/20260501T040106-8afb1cd2/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T040106-8afb1cd2/manifest.json",
+  "result_file": ".ddx/executions/20260501T040106-8afb1cd2/result.json",
+  "usage_file": ".ddx/executions/20260501T040106-8afb1cd2/usage.json",
+  "started_at": "2026-05-01T04:01:07.622618294Z",
+  "finished_at": "2026-05-01T04:02:54.816744813Z"
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
