<bead-review>
  <bead id="ddx-d01e5017" iter=1>
    <title>runs: docs (FEAT-008/010/019/021) + telemetry audit events</title>
    <description>
Update FEAT-008 (web UI), FEAT-010 (run architecture), FEAT-019 (agent eval), FEAT-021 (multi-node dashboard) to reflect new tab structure. Document audit-event schema for re-queue.
    </description>
    <acceptance>
1. All four FEATs updated. 2. Audit-event schema documented. 3. ddx doc audit clean.
    </acceptance>
    <labels>phase:2, story:8, area:specs, kind:doc</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T073208-32e1f775/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="057feb6ae0d242ce65a96d6010191702140ea56d">
diff --git a/.ddx/executions/20260503T073208-32e1f775/result.json b/.ddx/executions/20260503T073208-32e1f775/result.json
new file mode 100644
index 00000000..698a5a7a
--- /dev/null
+++ b/.ddx/executions/20260503T073208-32e1f775/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d01e5017",
+  "attempt_id": "20260503T073208-32e1f775",
+  "base_rev": "f635af84e90aee4df586413216c9d362461ad75c",
+  "result_rev": "be24c550c533ce5f0c27b218c9c7504e0d200c2d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c4269e0a",
+  "duration_ms": 378729,
+  "tokens": 18280,
+  "cost_usd": 3.17373125,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T073208-32e1f775",
+  "prompt_file": ".ddx/executions/20260503T073208-32e1f775/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T073208-32e1f775/manifest.json",
+  "result_file": ".ddx/executions/20260503T073208-32e1f775/result.json",
+  "usage_file": ".ddx/executions/20260503T073208-32e1f775/usage.json",
+  "started_at": "2026-05-03T07:32:10.561826784Z",
+  "finished_at": "2026-05-03T07:38:29.291162307Z"
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
