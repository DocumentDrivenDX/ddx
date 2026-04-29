<bead-review>
  <bead id="ddx-b06a2f9f" iter=1>
    <title>Document the fixture-backed frontend e2e harness</title>
    <description>
## Context

Once the default Playwright e2e harness boots the Go DDx server against a self-contained fixture, the frontend runbook needs to explain the new local workflow. Developers should know whether to build the `ddx` binary first or rely on `go run`, which command starts the suite, where the fixture lives, and how to extend the fixture without depending on `$HOME`, `~/.config/ddx`, or the repository's live `.ddx/` data.

Update the canonical frontend README/runbook so future test authors understand the fixture-backed harness and the separate demo recording configuration.

## In-scope files

- `cli/internal/server/frontend/README.md`.

## Out-of-scope

- `cli/internal/server/frontend/playwright.config.ts` and fixture implementation.
- Any e2e spec migration.
- `cli/internal/server/frontend/playwright.demo.config.ts` and `e2e/demo-recording.spec.ts`.
- Go server source changes.
- Broad documentation rewrites outside the frontend e2e runbook.

## Rollback / cleanup

Keep the runbook practical and specific: include the run command, the binary or `go run` prerequisite, fixture location, and guidance for adding fixture records. Do not document developer-local config as part of the supported e2e path.
    </description>
    <acceptance>
grep -E "ddx|fixture" cli/internal/server/frontend/README.md
grep -E "bun run test:e2e|go run|binary|fixture" cli/internal/server/frontend/README.md
    </acceptance>
    <labels>area:test, area:frontend, kind:test-infra, phase:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T055553-0664c7fd/manifest.json</file>
    <file>.ddx/executions/20260429T055553-0664c7fd/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8b6760aed3721f0e2136256a78ab36aa46f5dca0">
diff --git a/.ddx/executions/20260429T055553-0664c7fd/manifest.json b/.ddx/executions/20260429T055553-0664c7fd/manifest.json
new file mode 100644
index 00000000..4c3b6c0b
--- /dev/null
+++ b/.ddx/executions/20260429T055553-0664c7fd/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T055553-0664c7fd",
+  "bead_id": "ddx-b06a2f9f",
+  "base_rev": "c9579673bee4ce81dcc62689b59df7e9753e78c8",
+  "created_at": "2026-04-29T05:55:54.243762629Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b06a2f9f",
+    "title": "Document the fixture-backed frontend e2e harness",
+    "description": "## Context\n\nOnce the default Playwright e2e harness boots the Go DDx server against a self-contained fixture, the frontend runbook needs to explain the new local workflow. Developers should know whether to build the `ddx` binary first or rely on `go run`, which command starts the suite, where the fixture lives, and how to extend the fixture without depending on `$HOME`, `~/.config/ddx`, or the repository's live `.ddx/` data.\n\nUpdate the canonical frontend README/runbook so future test authors understand the fixture-backed harness and the separate demo recording configuration.\n\n## In-scope files\n\n- `cli/internal/server/frontend/README.md`.\n\n## Out-of-scope\n\n- `cli/internal/server/frontend/playwright.config.ts` and fixture implementation.\n- Any e2e spec migration.\n- `cli/internal/server/frontend/playwright.demo.config.ts` and `e2e/demo-recording.spec.ts`.\n- Go server source changes.\n- Broad documentation rewrites outside the frontend e2e runbook.\n\n## Rollback / cleanup\n\nKeep the runbook practical and specific: include the run command, the binary or `go run` prerequisite, fixture location, and guidance for adding fixture records. Do not document developer-local config as part of the supported e2e path.",
+    "acceptance": "grep -E \"ddx|fixture\" cli/internal/server/frontend/README.md\ngrep -E \"bun run test:e2e|go run|binary|fixture\" cli/internal/server/frontend/README.md",
+    "parent": "ddx-ccdf9cf9",
+    "labels": [
+      "area:test",
+      "area:frontend",
+      "kind:test-infra",
+      "phase:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T05:55:53Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T05:55:53.56586506Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T055553-0664c7fd",
+    "prompt": ".ddx/executions/20260429T055553-0664c7fd/prompt.md",
+    "manifest": ".ddx/executions/20260429T055553-0664c7fd/manifest.json",
+    "result": ".ddx/executions/20260429T055553-0664c7fd/result.json",
+    "checks": ".ddx/executions/20260429T055553-0664c7fd/checks.json",
+    "usage": ".ddx/executions/20260429T055553-0664c7fd/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b06a2f9f-20260429T055553-0664c7fd"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T055553-0664c7fd/result.json b/.ddx/executions/20260429T055553-0664c7fd/result.json
new file mode 100644
index 00000000..bce2b7d0
--- /dev/null
+++ b/.ddx/executions/20260429T055553-0664c7fd/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b06a2f9f",
+  "attempt_id": "20260429T055553-0664c7fd",
+  "base_rev": "c9579673bee4ce81dcc62689b59df7e9753e78c8",
+  "result_rev": "06adc533a3d97a77881b5570cf8dcc766d60b0ca",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-5a824edd",
+  "duration_ms": 41627,
+  "tokens": 2487,
+  "cost_usd": 0.321295,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T055553-0664c7fd",
+  "prompt_file": ".ddx/executions/20260429T055553-0664c7fd/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T055553-0664c7fd/manifest.json",
+  "result_file": ".ddx/executions/20260429T055553-0664c7fd/result.json",
+  "usage_file": ".ddx/executions/20260429T055553-0664c7fd/usage.json",
+  "started_at": "2026-04-29T05:55:54.244031963Z",
+  "finished_at": "2026-04-29T05:56:35.871947839Z"
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
