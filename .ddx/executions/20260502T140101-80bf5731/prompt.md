<bead-review>
  <bead id="ddx-41481396" iter=1>
    <title>metric: FEAT-005/TD-005 amendments + library/artifacts/met/ template bundle</title>
    <description>
Amend FEAT-005 (artifact convention) to register the metric: top-level frontmatter block (schema_version: 1, unit, direction, goal, budget, source: exec|external, scope) — lives outside ddx: so unknown fields stay inert. Amend TD-005 (metric runtime history) to confirm exec-runs collection ownership. Author library/artifacts/met/ template bundle for plugin authors.
    </description>
    <acceptance>
1. FEAT-005 has metric: block schema. 2. TD-005 references exec-runs collection (no rename to metric-runs). 3. library/artifacts/met/{template,prompt,examples} bundle exists. 4. ddx doc audit clean.
    </acceptance>
    <labels>phase:2, story:13, area:specs, kind:doc</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T135717-c4e9c6f6/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="82ed16f48b09b09d58a81e7f37d1d0a8c33d6de0">
diff --git a/.ddx/executions/20260502T135717-c4e9c6f6/result.json b/.ddx/executions/20260502T135717-c4e9c6f6/result.json
new file mode 100644
index 00000000..436cb5da
--- /dev/null
+++ b/.ddx/executions/20260502T135717-c4e9c6f6/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-41481396",
+  "attempt_id": "20260502T135717-c4e9c6f6",
+  "base_rev": "cba67be88574a6e596d913b026de13b1d5b69c83",
+  "result_rev": "dc565a651094c776a05994503214b6b1c5dc9c26",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-301e6cef",
+  "duration_ms": 216065,
+  "tokens": 10583,
+  "cost_usd": 1.2243229999999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T135717-c4e9c6f6",
+  "prompt_file": ".ddx/executions/20260502T135717-c4e9c6f6/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T135717-c4e9c6f6/manifest.json",
+  "result_file": ".ddx/executions/20260502T135717-c4e9c6f6/result.json",
+  "usage_file": ".ddx/executions/20260502T135717-c4e9c6f6/usage.json",
+  "started_at": "2026-05-02T13:57:18.862143189Z",
+  "finished_at": "2026-05-02T14:00:54.927601388Z"
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
