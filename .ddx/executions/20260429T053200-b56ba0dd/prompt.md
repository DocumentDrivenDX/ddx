<bead-review>
  <bead id="ddx-d15b6aa1" iter=1>
    <title>Migrate bead execution and worker smoke specs to fixture data</title>
    <description>
## Context

The frontend e2e specs for beads, executions, and workers require backend data and currently fail when Playwright serves static preview output without the Go DDx server. After the fixture-backed harness exists, these specs should use the seeded `.ddx/beads.jsonl`, docs library, and any execution/worker fixture state available from the harness instead of relying on developer-local DDx state.

Update the backend-data smoke specs so they pass against the self-contained fixture while preserving coverage of bead lists/status, execution views, and worker-related UI/API behavior.

## In-scope files

- `cli/internal/server/frontend/e2e/beads-smoke.spec.ts`.
- `cli/internal/server/frontend/e2e/executions.spec.ts`.
- `cli/internal/server/frontend/e2e/workers.spec.ts`.

## Out-of-scope

- `cli/internal/server/frontend/e2e/app.spec.ts`.
- `cli/internal/server/frontend/e2e/navigation.spec.ts` and `src/routes/layout.e2e.ts`.
- `cli/internal/server/frontend/e2e/screenshots.spec.ts` and any visual baseline rework.
- `cli/internal/server/frontend/playwright.config.ts` and fixture creation except reading the fixture IDs/data created by the prerequisite bead.
- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.
- Go server source changes under `cli/internal/server/`.
- Test deletion, `.skip`, or broad assertion weakening.

## Rollback / cleanup

If a spec needs additional fixture records, keep the fixture deterministic and self-contained. Do not point tests at the repository's live `.ddx/` tracker or developer-specific worker state.
    </description>
    <acceptance>
cd cli/internal/server/frontend &amp;&amp; bun run test:e2e e2e/beads-smoke.spec.ts e2e/executions.spec.ts e2e/workers.spec.ts
cd cli/internal/server/frontend &amp;&amp; git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts
    </acceptance>
    <labels>area:test, area:frontend, kind:test-infra, phase:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T051758-9c5b320a/manifest.json</file>
    <file>.ddx/executions/20260429T051758-9c5b320a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a1251902ceaa478d5c8e0381cbfe2d3a95786b04">
diff --git a/.ddx/executions/20260429T051758-9c5b320a/manifest.json b/.ddx/executions/20260429T051758-9c5b320a/manifest.json
new file mode 100644
index 00000000..df9440d7
--- /dev/null
+++ b/.ddx/executions/20260429T051758-9c5b320a/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T051758-9c5b320a",
+  "bead_id": "ddx-d15b6aa1",
+  "base_rev": "1354a5397fa732ca6a1042f3e51ec466705ba9a4",
+  "created_at": "2026-04-29T05:17:59.086046022Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d15b6aa1",
+    "title": "Migrate bead execution and worker smoke specs to fixture data",
+    "description": "## Context\n\nThe frontend e2e specs for beads, executions, and workers require backend data and currently fail when Playwright serves static preview output without the Go DDx server. After the fixture-backed harness exists, these specs should use the seeded `.ddx/beads.jsonl`, docs library, and any execution/worker fixture state available from the harness instead of relying on developer-local DDx state.\n\nUpdate the backend-data smoke specs so they pass against the self-contained fixture while preserving coverage of bead lists/status, execution views, and worker-related UI/API behavior.\n\n## In-scope files\n\n- `cli/internal/server/frontend/e2e/beads-smoke.spec.ts`.\n- `cli/internal/server/frontend/e2e/executions.spec.ts`.\n- `cli/internal/server/frontend/e2e/workers.spec.ts`.\n\n## Out-of-scope\n\n- `cli/internal/server/frontend/e2e/app.spec.ts`.\n- `cli/internal/server/frontend/e2e/navigation.spec.ts` and `src/routes/layout.e2e.ts`.\n- `cli/internal/server/frontend/e2e/screenshots.spec.ts` and any visual baseline rework.\n- `cli/internal/server/frontend/playwright.config.ts` and fixture creation except reading the fixture IDs/data created by the prerequisite bead.\n- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.\n- Go server source changes under `cli/internal/server/`.\n- Test deletion, `.skip`, or broad assertion weakening.\n\n## Rollback / cleanup\n\nIf a spec needs additional fixture records, keep the fixture deterministic and self-contained. Do not point tests at the repository's live `.ddx/` tracker or developer-specific worker state.",
+    "acceptance": "cd cli/internal/server/frontend \u0026\u0026 bun run test:e2e e2e/beads-smoke.spec.ts e2e/executions.spec.ts e2e/workers.spec.ts\ncd cli/internal/server/frontend \u0026\u0026 git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts",
+    "parent": "ddx-ccdf9cf9",
+    "labels": [
+      "area:test",
+      "area:frontend",
+      "kind:test-infra",
+      "phase:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T05:17:58Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T05:17:58.394522532Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T051758-9c5b320a",
+    "prompt": ".ddx/executions/20260429T051758-9c5b320a/prompt.md",
+    "manifest": ".ddx/executions/20260429T051758-9c5b320a/manifest.json",
+    "result": ".ddx/executions/20260429T051758-9c5b320a/result.json",
+    "checks": ".ddx/executions/20260429T051758-9c5b320a/checks.json",
+    "usage": ".ddx/executions/20260429T051758-9c5b320a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d15b6aa1-20260429T051758-9c5b320a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T051758-9c5b320a/result.json b/.ddx/executions/20260429T051758-9c5b320a/result.json
new file mode 100644
index 00000000..d0e14646
--- /dev/null
+++ b/.ddx/executions/20260429T051758-9c5b320a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d15b6aa1",
+  "attempt_id": "20260429T051758-9c5b320a",
+  "base_rev": "1354a5397fa732ca6a1042f3e51ec466705ba9a4",
+  "result_rev": "d604d6867fa61f5f7ba024099ce01713335172cf",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c37364c8",
+  "duration_ms": 838855,
+  "tokens": 39142,
+  "cost_usd": 6.05149675,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T051758-9c5b320a",
+  "prompt_file": ".ddx/executions/20260429T051758-9c5b320a/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T051758-9c5b320a/manifest.json",
+  "result_file": ".ddx/executions/20260429T051758-9c5b320a/result.json",
+  "usage_file": ".ddx/executions/20260429T051758-9c5b320a/usage.json",
+  "started_at": "2026-04-29T05:17:59.086331564Z",
+  "finished_at": "2026-04-29T05:31:57.941461485Z"
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
