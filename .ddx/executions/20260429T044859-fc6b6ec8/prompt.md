<bead-review>
  <bead id="ddx-9f8092f1" iter=1>
    <title>Migrate TC-001 dashboard tests to fixture node/project routes</title>
    <description>
## Context

TC-001 in `cli/internal/server/frontend/e2e/app.spec.ts` still assumes that visiting `/` renders a Dashboard `h1`. That is stale. Since Stage 3.8 commit `3bf655cf`, `src/routes/+page.svelte` redirects to `/nodes/&lt;node-id&gt;` when a node is selected, and the app's concrete project views live under `/nodes/&lt;fixture-node-id&gt;/projects/&lt;fixture-project-id&gt;/...`. `e2e/navigation.spec.ts` already contains the route shape to mirror around line 177.

After the fixture-backed Playwright harness exists, rewrite only the TC-001 Dashboard tests so they navigate through the fixture node/project route and assert the current UI behavior. Keep the test intent: dashboard-level smoke coverage should prove the shell and expected project entry points render, but it must not depend on `/` being a Dashboard page.

## In-scope files

- `cli/internal/server/frontend/e2e/app.spec.ts` — TC-001 block only, currently around lines 9-57.

## Out-of-scope

- `cli/internal/server/frontend/playwright.config.ts` and `cli/internal/server/frontend/e2e/fixtures/`; those belong to the harness prerequisite.
- TC-002, TC-003, TC-005, TC-007, and TC-008 in `e2e/app.spec.ts`.
- `cli/internal/server/frontend/e2e/navigation.spec.ts` except as a read-only reference for route patterns.
- Frontend route/component changes under `src/routes/`.
- Test deletion, `.skip`, or comment-out changes to make the suite pass.

## Rollback / cleanup

Keep TC-001 present and meaningful. If fixture IDs change during the harness bead, align TC-001 to the fixture constants or helpers rather than hard-coding unrelated local state.
    </description>
    <acceptance>
cd cli/internal/server/frontend &amp;&amp; bun run test:e2e --grep "TC-001"
cd cli/internal/server/frontend &amp;&amp; git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts
    </acceptance>
    <labels>area:test, area:frontend, kind:test-infra, phase:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T044111-cc95ed95/manifest.json</file>
    <file>.ddx/executions/20260429T044111-cc95ed95/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="04c13121efe2427fe4aa590eebb1a8c2e7bc603e">
diff --git a/.ddx/executions/20260429T044111-cc95ed95/manifest.json b/.ddx/executions/20260429T044111-cc95ed95/manifest.json
new file mode 100644
index 00000000..4d5e2150
--- /dev/null
+++ b/.ddx/executions/20260429T044111-cc95ed95/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T044111-cc95ed95",
+  "bead_id": "ddx-9f8092f1",
+  "base_rev": "20bea39a2765035ff85861fd51ea22e5abb9ef27",
+  "created_at": "2026-04-29T04:41:12.495849863Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9f8092f1",
+    "title": "Migrate TC-001 dashboard tests to fixture node/project routes",
+    "description": "## Context\n\nTC-001 in `cli/internal/server/frontend/e2e/app.spec.ts` still assumes that visiting `/` renders a Dashboard `h1`. That is stale. Since Stage 3.8 commit `3bf655cf`, `src/routes/+page.svelte` redirects to `/nodes/\u003cnode-id\u003e` when a node is selected, and the app's concrete project views live under `/nodes/\u003cfixture-node-id\u003e/projects/\u003cfixture-project-id\u003e/...`. `e2e/navigation.spec.ts` already contains the route shape to mirror around line 177.\n\nAfter the fixture-backed Playwright harness exists, rewrite only the TC-001 Dashboard tests so they navigate through the fixture node/project route and assert the current UI behavior. Keep the test intent: dashboard-level smoke coverage should prove the shell and expected project entry points render, but it must not depend on `/` being a Dashboard page.\n\n## In-scope files\n\n- `cli/internal/server/frontend/e2e/app.spec.ts` — TC-001 block only, currently around lines 9-57.\n\n## Out-of-scope\n\n- `cli/internal/server/frontend/playwright.config.ts` and `cli/internal/server/frontend/e2e/fixtures/`; those belong to the harness prerequisite.\n- TC-002, TC-003, TC-005, TC-007, and TC-008 in `e2e/app.spec.ts`.\n- `cli/internal/server/frontend/e2e/navigation.spec.ts` except as a read-only reference for route patterns.\n- Frontend route/component changes under `src/routes/`.\n- Test deletion, `.skip`, or comment-out changes to make the suite pass.\n\n## Rollback / cleanup\n\nKeep TC-001 present and meaningful. If fixture IDs change during the harness bead, align TC-001 to the fixture constants or helpers rather than hard-coding unrelated local state.",
+    "acceptance": "cd cli/internal/server/frontend \u0026\u0026 bun run test:e2e --grep \"TC-001\"\ncd cli/internal/server/frontend \u0026\u0026 git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts",
+    "parent": "ddx-ccdf9cf9",
+    "labels": [
+      "area:test",
+      "area:frontend",
+      "kind:test-infra",
+      "phase:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T04:41:11Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T04:41:11.896915895Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T044111-cc95ed95",
+    "prompt": ".ddx/executions/20260429T044111-cc95ed95/prompt.md",
+    "manifest": ".ddx/executions/20260429T044111-cc95ed95/manifest.json",
+    "result": ".ddx/executions/20260429T044111-cc95ed95/result.json",
+    "checks": ".ddx/executions/20260429T044111-cc95ed95/checks.json",
+    "usage": ".ddx/executions/20260429T044111-cc95ed95/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9f8092f1-20260429T044111-cc95ed95"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T044111-cc95ed95/result.json b/.ddx/executions/20260429T044111-cc95ed95/result.json
new file mode 100644
index 00000000..306fed12
--- /dev/null
+++ b/.ddx/executions/20260429T044111-cc95ed95/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9f8092f1",
+  "attempt_id": "20260429T044111-cc95ed95",
+  "base_rev": "20bea39a2765035ff85861fd51ea22e5abb9ef27",
+  "result_rev": "06ec403ef55f1ca95e24d29a53232e6d6f0635eb",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-36cdde89",
+  "duration_ms": 464184,
+  "tokens": 12215,
+  "cost_usd": 1.90987725,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T044111-cc95ed95",
+  "prompt_file": ".ddx/executions/20260429T044111-cc95ed95/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T044111-cc95ed95/manifest.json",
+  "result_file": ".ddx/executions/20260429T044111-cc95ed95/result.json",
+  "usage_file": ".ddx/executions/20260429T044111-cc95ed95/usage.json",
+  "started_at": "2026-04-29T04:41:12.496101113Z",
+  "finished_at": "2026-04-29T04:48:56.680671213Z"
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
