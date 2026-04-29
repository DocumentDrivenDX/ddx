<bead-review>
  <bead id="ddx-dd4c423d" iter=1>
    <title>routing-drain-regression: 19-burn historical bug regression test</title>
    <description>
Cover AC #10 from ddx-fdd3ea36. Standalone integration test that reproduces the historical drain-queue failure mode (empty spec + user's real config + gemini binary installed + minimax model in overrides → 19 failures). Before parent's work: test fails with 19-burn. After: test succeeds or exits with one clean typed error. Test stays in the suite as a permanent regression guard. Note: handle gemini-binary availability — either skip when absent, OR use a stub binary fixture. Document the choice.
    </description>
    <acceptance>
1. New integration test file (e.g. cli/internal/agent/drain_regression_test.go) exists. 2. Test reproduces the original failure: empty spec, user-config-shaped fixture (lmstudio + omlx + lmstudio + lmstudio endpoints + gemini-in-overrides). 3. Test asserts: at most 1 failed attempt, never 19. 4. gemini-binary presence handled deterministically (either fixture stub or t.Skip with clear message). 5. Test is wired into go test ./... default run, not behind a tag (unless gemini-binary requirement makes that necessary — document the rationale).
    </acceptance>
    <labels>feat-006, routing, regression</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T035839-ec1070fd/manifest.json</file>
    <file>.ddx/executions/20260429T035839-ec1070fd/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9d71a47f4f44a4f32538129780dc8ada70509938">
diff --git a/.ddx/executions/20260429T035839-ec1070fd/manifest.json b/.ddx/executions/20260429T035839-ec1070fd/manifest.json
new file mode 100644
index 00000000..6105266b
--- /dev/null
+++ b/.ddx/executions/20260429T035839-ec1070fd/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260429T035839-ec1070fd",
+  "bead_id": "ddx-dd4c423d",
+  "base_rev": "3b31a2e61544c4a8ae02a8dbee503a122ecc50cb",
+  "created_at": "2026-04-29T03:58:40.336078943Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-dd4c423d",
+    "title": "routing-drain-regression: 19-burn historical bug regression test",
+    "description": "Cover AC #10 from ddx-fdd3ea36. Standalone integration test that reproduces the historical drain-queue failure mode (empty spec + user's real config + gemini binary installed + minimax model in overrides → 19 failures). Before parent's work: test fails with 19-burn. After: test succeeds or exits with one clean typed error. Test stays in the suite as a permanent regression guard. Note: handle gemini-binary availability — either skip when absent, OR use a stub binary fixture. Document the choice.",
+    "acceptance": "1. New integration test file (e.g. cli/internal/agent/drain_regression_test.go) exists. 2. Test reproduces the original failure: empty spec, user-config-shaped fixture (lmstudio + omlx + lmstudio + lmstudio endpoints + gemini-in-overrides). 3. Test asserts: at most 1 failed attempt, never 19. 4. gemini-binary presence handled deterministically (either fixture stub or t.Skip with clear message). 5. Test is wired into go test ./... default run, not behind a tag (unless gemini-binary requirement makes that necessary — document the rationale).",
+    "parent": "ddx-fdd3ea36",
+    "labels": [
+      "feat-006",
+      "routing",
+      "regression"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T03:58:39Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T03:58:39.662184Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T035839-ec1070fd",
+    "prompt": ".ddx/executions/20260429T035839-ec1070fd/prompt.md",
+    "manifest": ".ddx/executions/20260429T035839-ec1070fd/manifest.json",
+    "result": ".ddx/executions/20260429T035839-ec1070fd/result.json",
+    "checks": ".ddx/executions/20260429T035839-ec1070fd/checks.json",
+    "usage": ".ddx/executions/20260429T035839-ec1070fd/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-dd4c423d-20260429T035839-ec1070fd"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T035839-ec1070fd/result.json b/.ddx/executions/20260429T035839-ec1070fd/result.json
new file mode 100644
index 00000000..3a0c27fd
--- /dev/null
+++ b/.ddx/executions/20260429T035839-ec1070fd/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-dd4c423d",
+  "attempt_id": "20260429T035839-ec1070fd",
+  "base_rev": "3b31a2e61544c4a8ae02a8dbee503a122ecc50cb",
+  "result_rev": "090c497eac861201bdd1b3f5c415586e7cf5737b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-77dcf8a9",
+  "duration_ms": 478172,
+  "tokens": 17880,
+  "cost_usd": 2.678425750000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T035839-ec1070fd",
+  "prompt_file": ".ddx/executions/20260429T035839-ec1070fd/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T035839-ec1070fd/manifest.json",
+  "result_file": ".ddx/executions/20260429T035839-ec1070fd/result.json",
+  "usage_file": ".ddx/executions/20260429T035839-ec1070fd/usage.json",
+  "started_at": "2026-04-29T03:58:40.336345151Z",
+  "finished_at": "2026-04-29T04:06:38.50859405Z"
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
