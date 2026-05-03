<bead-review>
  <bead id="ddx-1c8719eb" iter=1>
    <title>availability: Playwright e2e for the 5-provider scenario (claude/codex/openrouter/lmstudio-vidar/lmstudio-bragi)</title>
    <description>
End-to-end test mocking the 5 providers from the user's spec: claude, codex, openrouter, lmstudio @vidar, lmstudio @bragi. Each visible with its model list.
    </description>
    <acceptance>
1. Fixture mocks 5 providers + per-provider models. 2. Test asserts all 5 visible with model lists. 3. Refresh button drives a re-query.
    </acceptance>
    <labels>phase:2, story:9, area:tests, kind:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T130113-ba4363ca/manifest.json</file>
    <file>.ddx/executions/20260503T130113-ba4363ca/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="6ec15f1d868504b14032453f97ce5e8dfabd1010">
diff --git a/.ddx/executions/20260503T130113-ba4363ca/manifest.json b/.ddx/executions/20260503T130113-ba4363ca/manifest.json
new file mode 100644
index 00000000..ff0245ad
--- /dev/null
+++ b/.ddx/executions/20260503T130113-ba4363ca/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260503T130113-ba4363ca",
+  "bead_id": "ddx-1c8719eb",
+  "base_rev": "892cdb1739525b4b4872c8e8967b4b5bfa6923d0",
+  "created_at": "2026-05-03T13:01:15.689186749Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-1c8719eb",
+    "title": "availability: Playwright e2e for the 5-provider scenario (claude/codex/openrouter/lmstudio-vidar/lmstudio-bragi)",
+    "description": "End-to-end test mocking the 5 providers from the user's spec: claude, codex, openrouter, lmstudio @vidar, lmstudio @bragi. Each visible with its model list.",
+    "acceptance": "1. Fixture mocks 5 providers + per-provider models. 2. Test asserts all 5 visible with model lists. 3. Refresh button drives a re-query.",
+    "parent": "ddx-12e97cb9",
+    "labels": [
+      "phase:2",
+      "story:9",
+      "area:tests",
+      "kind:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T13:01:13Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T13:01:13.82485991Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T130113-ba4363ca",
+    "prompt": ".ddx/executions/20260503T130113-ba4363ca/prompt.md",
+    "manifest": ".ddx/executions/20260503T130113-ba4363ca/manifest.json",
+    "result": ".ddx/executions/20260503T130113-ba4363ca/result.json",
+    "checks": ".ddx/executions/20260503T130113-ba4363ca/checks.json",
+    "usage": ".ddx/executions/20260503T130113-ba4363ca/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-1c8719eb-20260503T130113-ba4363ca"
+  },
+  "prompt_sha": "07a65921c132b9b2e797217eb50071618b4c52ad192b0b0535dc8ed106f7af51"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T130113-ba4363ca/result.json b/.ddx/executions/20260503T130113-ba4363ca/result.json
new file mode 100644
index 00000000..38f11497
--- /dev/null
+++ b/.ddx/executions/20260503T130113-ba4363ca/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-1c8719eb",
+  "attempt_id": "20260503T130113-ba4363ca",
+  "base_rev": "892cdb1739525b4b4872c8e8967b4b5bfa6923d0",
+  "result_rev": "75d6aa7ad0b6520ad770a50ff383f03a47fce4a3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d0656f38",
+  "duration_ms": 308802,
+  "tokens": 11376,
+  "cost_usd": 1.4710357500000004,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T130113-ba4363ca",
+  "prompt_file": ".ddx/executions/20260503T130113-ba4363ca/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T130113-ba4363ca/manifest.json",
+  "result_file": ".ddx/executions/20260503T130113-ba4363ca/result.json",
+  "usage_file": ".ddx/executions/20260503T130113-ba4363ca/usage.json",
+  "started_at": "2026-05-03T13:01:15.690050998Z",
+  "finished_at": "2026-05-03T13:06:24.492958414Z"
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
