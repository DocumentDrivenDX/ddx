<bead-review>
  <bead id="ddx-c2860348" iter=1>
    <title>Migrate navigation and layout e2e tests to fixture routes</title>
    <description>
## Context

The frontend route tests need to run against the same fixture-backed Go DDx server as the rest of the e2e suite. The current node/project router uses concrete paths such as `/nodes/&lt;fixture-node-id&gt;/projects/&lt;fixture-project-id&gt;/...`, and tests that still rely on static preview or implicit local data drift fail under the real app. `e2e/navigation.spec.ts` already contains the target route style around line 177, but the navigation/layout coverage must consistently use fixture IDs and seeded backend data.

Update the navigation and layout e2e tests so they pass against the new Playwright harness and its seeded fixture while preserving their existing route/shell coverage.

## In-scope files

- `cli/internal/server/frontend/e2e/navigation.spec.ts` — TC-006, TC-007, and any route setup in this file needed to use fixture node/project IDs.
- `cli/internal/server/frontend/src/routes/layout.e2e.ts` — layout route e2e expectations that depend on project/node data.

## Out-of-scope

- `cli/internal/server/frontend/e2e/app.spec.ts`.
- `cli/internal/server/frontend/e2e/beads-smoke.spec.ts`, `e2e/executions.spec.ts`, `e2e/workers.spec.ts`, and `e2e/screenshots.spec.ts`.
- `cli/internal/server/frontend/playwright.config.ts` and fixture creation except reading the fixture IDs/data created by the prerequisite bead.
- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.
- Frontend route/component changes and Go server changes.
- Skipping tests or deleting assertions to get a green run.

## Rollback / cleanup

Keep route expectations tied to explicit fixture node/project IDs or shared test helpers. Do not introduce dependencies on `$HOME`, `~/.config/ddx`, or the developer's live repository state.
    </description>
    <acceptance>
cd cli/internal/server/frontend &amp;&amp; bun run test:e2e e2e/navigation.spec.ts src/routes/layout.e2e.ts
cd cli/internal/server/frontend &amp;&amp; git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts
    </acceptance>
    <labels>area:test, area:frontend, kind:test-infra, phase:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T050823-5979c8d4/manifest.json</file>
    <file>.ddx/executions/20260429T050823-5979c8d4/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8374615727d5940f8c78b11de260678572cad19b">
diff --git a/.ddx/executions/20260429T050823-5979c8d4/manifest.json b/.ddx/executions/20260429T050823-5979c8d4/manifest.json
new file mode 100644
index 00000000..0b0a4acf
--- /dev/null
+++ b/.ddx/executions/20260429T050823-5979c8d4/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T050823-5979c8d4",
+  "bead_id": "ddx-c2860348",
+  "base_rev": "4bc97242933958dcb3b0017d5d5503a47e2bc76e",
+  "created_at": "2026-04-29T05:08:24.446685294Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c2860348",
+    "title": "Migrate navigation and layout e2e tests to fixture routes",
+    "description": "## Context\n\nThe frontend route tests need to run against the same fixture-backed Go DDx server as the rest of the e2e suite. The current node/project router uses concrete paths such as `/nodes/\u003cfixture-node-id\u003e/projects/\u003cfixture-project-id\u003e/...`, and tests that still rely on static preview or implicit local data drift fail under the real app. `e2e/navigation.spec.ts` already contains the target route style around line 177, but the navigation/layout coverage must consistently use fixture IDs and seeded backend data.\n\nUpdate the navigation and layout e2e tests so they pass against the new Playwright harness and its seeded fixture while preserving their existing route/shell coverage.\n\n## In-scope files\n\n- `cli/internal/server/frontend/e2e/navigation.spec.ts` — TC-006, TC-007, and any route setup in this file needed to use fixture node/project IDs.\n- `cli/internal/server/frontend/src/routes/layout.e2e.ts` — layout route e2e expectations that depend on project/node data.\n\n## Out-of-scope\n\n- `cli/internal/server/frontend/e2e/app.spec.ts`.\n- `cli/internal/server/frontend/e2e/beads-smoke.spec.ts`, `e2e/executions.spec.ts`, `e2e/workers.spec.ts`, and `e2e/screenshots.spec.ts`.\n- `cli/internal/server/frontend/playwright.config.ts` and fixture creation except reading the fixture IDs/data created by the prerequisite bead.\n- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.\n- Frontend route/component changes and Go server changes.\n- Skipping tests or deleting assertions to get a green run.\n\n## Rollback / cleanup\n\nKeep route expectations tied to explicit fixture node/project IDs or shared test helpers. Do not introduce dependencies on `$HOME`, `~/.config/ddx`, or the developer's live repository state.",
+    "acceptance": "cd cli/internal/server/frontend \u0026\u0026 bun run test:e2e e2e/navigation.spec.ts src/routes/layout.e2e.ts\ncd cli/internal/server/frontend \u0026\u0026 git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts",
+    "parent": "ddx-ccdf9cf9",
+    "labels": [
+      "area:test",
+      "area:frontend",
+      "kind:test-infra",
+      "phase:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T05:08:23Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T05:08:23.808437664Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T050823-5979c8d4",
+    "prompt": ".ddx/executions/20260429T050823-5979c8d4/prompt.md",
+    "manifest": ".ddx/executions/20260429T050823-5979c8d4/manifest.json",
+    "result": ".ddx/executions/20260429T050823-5979c8d4/result.json",
+    "checks": ".ddx/executions/20260429T050823-5979c8d4/checks.json",
+    "usage": ".ddx/executions/20260429T050823-5979c8d4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c2860348-20260429T050823-5979c8d4"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T050823-5979c8d4/result.json b/.ddx/executions/20260429T050823-5979c8d4/result.json
new file mode 100644
index 00000000..6de713b0
--- /dev/null
+++ b/.ddx/executions/20260429T050823-5979c8d4/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c2860348",
+  "attempt_id": "20260429T050823-5979c8d4",
+  "base_rev": "4bc97242933958dcb3b0017d5d5503a47e2bc76e",
+  "result_rev": "ecab1e5cf54d36a90cbbf2aa705ecbb18fa26d99",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-e0fad89a",
+  "duration_ms": 544272,
+  "tokens": 23038,
+  "cost_usd": 2.4435937500000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T050823-5979c8d4",
+  "prompt_file": ".ddx/executions/20260429T050823-5979c8d4/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T050823-5979c8d4/manifest.json",
+  "result_file": ".ddx/executions/20260429T050823-5979c8d4/result.json",
+  "usage_file": ".ddx/executions/20260429T050823-5979c8d4/usage.json",
+  "started_at": "2026-04-29T05:08:24.446948168Z",
+  "finished_at": "2026-04-29T05:17:28.719753166Z"
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
