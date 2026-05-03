<bead-review>
  <bead id="ddx-d99a1dd1" iter=1>
    <title>runs: re-queue UI (try, run) + 'Start worker from this drain' (work)</title>
    <description>
Re-queue button on layer=try and layer=run rows. For layer=work, a 'Start worker from this drain' action prefilling the original queue inputs (codex-pushed; user confirmed).
    </description>
    <acceptance>
1. Re-queue button visible on try and run rows. 2. Modal prefilled with original config; idempotency key generated client-side. 3. work-layer 'Start worker' action prefilled with original drain config.
    </acceptance>
    <labels>phase:2, story:8, area:web, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T060104-e7053ec7/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c98613417eeeba8c77df42828c4156d6935c506f">
diff --git a/.ddx/executions/20260503T060104-e7053ec7/result.json b/.ddx/executions/20260503T060104-e7053ec7/result.json
new file mode 100644
index 00000000..fc4d27d1
--- /dev/null
+++ b/.ddx/executions/20260503T060104-e7053ec7/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d99a1dd1",
+  "attempt_id": "20260503T060104-e7053ec7",
+  "base_rev": "f7adfc00fed1d694f9f67441a5c85430efcd1f20",
+  "result_rev": "12d0b6181f2f3a8bc4bfb8fb7a62a214a998977b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a83a41e2",
+  "duration_ms": 485844,
+  "tokens": 16750,
+  "cost_usd": 2.5460863,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T060104-e7053ec7",
+  "prompt_file": ".ddx/executions/20260503T060104-e7053ec7/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T060104-e7053ec7/manifest.json",
+  "result_file": ".ddx/executions/20260503T060104-e7053ec7/result.json",
+  "usage_file": ".ddx/executions/20260503T060104-e7053ec7/usage.json",
+  "started_at": "2026-05-03T06:01:05.509836504Z",
+  "finished_at": "2026-05-03T06:09:11.353992696Z"
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
