<bead-review>
  <bead id="ddx-bd37afc7" iter=1>
    <title>Remove live harness execution tests from DDx</title>
    <description>
DDx should not execute real third-party harness binaries now that harness ownership lives in Fizeau. Remove or quarantine live harness execution tests such as opencode/claude/codex echo tests from DDx packages, and replace any needed DDx coverage with Fizeau service stubs, fake executors, or contract-level assertions. The normal DDx suite should not start opencode, claude, codex, gemini, pi, or other real harness processes.
    </description>
    <acceptance>
1. DDx go test ./... does not invoke real harness binaries (opencode, claude, codex, gemini, pi) as part of the default or short test suites.\n2. Existing DDx behavior coverage is preserved with stubs/fakes at the DDx-Fizeau boundary where needed.\n3. Any remaining live harness smoke tests are moved to Fizeau or gated behind an explicit opt-in env var/build tag with a clear comment.\n4. go test ./internal/agent ./cmd passes without requiring installed harness CLIs.\n5. A grep/static check documents the remaining allowed harness binary invocations in tests.
    </acceptance>
    <labels>area:agent, area:test, kind:cleanup</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T040146-d9dcddba/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="dfbaf1f3c0ff5aac557b014e7b6c65f4331944d2">
diff --git a/.ddx/executions/20260501T040146-d9dcddba/result.json b/.ddx/executions/20260501T040146-d9dcddba/result.json
new file mode 100644
index 00000000..07a6e85a
--- /dev/null
+++ b/.ddx/executions/20260501T040146-d9dcddba/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-bd37afc7",
+  "attempt_id": "20260501T040146-d9dcddba",
+  "base_rev": "c015d68433d939b3e4379a464c91cc2ea014e648",
+  "result_rev": "6d2c3112328adc5ea3dc629a1c67c8cbadf3f5b0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-11d8cc95",
+  "duration_ms": 443676,
+  "tokens": 11004,
+  "cost_usd": 1.3745802999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T040146-d9dcddba",
+  "prompt_file": ".ddx/executions/20260501T040146-d9dcddba/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T040146-d9dcddba/manifest.json",
+  "result_file": ".ddx/executions/20260501T040146-d9dcddba/result.json",
+  "usage_file": ".ddx/executions/20260501T040146-d9dcddba/usage.json",
+  "started_at": "2026-05-01T04:01:47.146713458Z",
+  "finished_at": "2026-05-01T04:09:10.8227738Z"
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
