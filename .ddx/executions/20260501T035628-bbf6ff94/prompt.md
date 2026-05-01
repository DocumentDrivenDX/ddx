<bead-review>
  <bead id="ddx-c5f9c009" iter=1>
    <title>e2e: beads.spec.ts — add dependency tree/DAG view test</title>
    <description/>
    <acceptance>
beads.spec.ts has test that renders bead dependency DAG view; verifies parent/child bead relationships visible; test passes in CI
    </acceptance>
    <labels>area:e2e, playwright, beads</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T035434-a2102c7b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8fee1989768673c794f73996ff55a48956744222">
diff --git a/.ddx/executions/20260501T035434-a2102c7b/result.json b/.ddx/executions/20260501T035434-a2102c7b/result.json
new file mode 100644
index 00000000..5de94a06
--- /dev/null
+++ b/.ddx/executions/20260501T035434-a2102c7b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c5f9c009",
+  "attempt_id": "20260501T035434-a2102c7b",
+  "base_rev": "27229aef8a367e451e575b037bf7048468918d4e",
+  "result_rev": "f2487372087e5f132e39e788d40948f631e5a802",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0f200f00",
+  "duration_ms": 107993,
+  "tokens": 5771,
+  "cost_usd": 0.8377842499999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T035434-a2102c7b",
+  "prompt_file": ".ddx/executions/20260501T035434-a2102c7b/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T035434-a2102c7b/manifest.json",
+  "result_file": ".ddx/executions/20260501T035434-a2102c7b/result.json",
+  "usage_file": ".ddx/executions/20260501T035434-a2102c7b/usage.json",
+  "started_at": "2026-05-01T03:54:35.329895783Z",
+  "finished_at": "2026-05-01T03:56:23.323310998Z"
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
