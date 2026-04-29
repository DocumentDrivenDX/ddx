<bead-review>
  <bead id="ddx-fc6e5b14" iter=1>
    <title>Switch the default Playwright e2e harness to a Go-server fixture</title>
    <description>
## Context

The frontend Playwright suite under `cli/internal/server/frontend` currently starts `bun run preview` on port 4173, which serves static SvelteKit output without the Go DDx backend. The parent e2e suite now includes direct API tests and route/page tests that require real DDx server data. TC-008 in `e2e/app.spec.ts` calls `/api/health`, `/api/documents`, `/api/beads`, `/api/beads/status`, `/api/personas`, and `/api/docs/graph`; those endpoints return 404 under static preview but return 200 from the Go server.

Build a fixture-backed default Playwright harness that boots the Go DDx server on a deterministic local port and points Playwright at it. The fixture must be self-contained so e2e runs do not read the developer's home config or the repository's live `.ddx/` state.

## In-scope files

- `cli/internal/server/frontend/playwright.config.ts` — change the default `webServer` to run the Go DDx server (`go run ./cli` from the repo root or a built `ddx` binary) on a deterministic port, set `use.baseURL` to that server, and keep any static-only preview coverage separate from the default backend-backed project.
- `cli/internal/server/frontend/e2e/fixtures/` — add a fixture project directory containing minimal `.ddx/beads.jsonl` data with open, closed, ready, and blocked beads; a tiny `docs/` library; and any config needed by the server.
- `cli/internal/server/frontend/e2e/app.spec.ts` — only TC-008 API-test alignment if the fixture data requires narrow expectation updates.

## Out-of-scope

- `cli/internal/server/frontend/playwright.demo.config.ts`.
- `cli/internal/server/frontend/e2e/demo-recording.spec.ts`.
- Go server source under `cli/internal/server/`; the API is assumed correct.
- Frontend components or routes.
- Migrating TC-001 through TC-007 page tests; those are separate child beads.

## Rollback / cleanup

Do not leave a half-populated fixture directory. Do not make tests pass by skipping or deleting them. The default test harness must use a temp copy or otherwise isolated fixture workspace, not the developer's actual repository checkout or `$HOME`.
    </description>
    <acceptance>
cd cli/internal/server/frontend &amp;&amp; bun run test:e2e --grep TC-008
cd cli/internal/server/frontend &amp;&amp; ! grep -rE '\$HOME|~/' e2e/fixtures/
cd cli/internal/server/frontend &amp;&amp; grep -E 'webServer|baseURL|ddx|go run' playwright.config.ts
cd cli/internal/server/frontend &amp;&amp; git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts
    </acceptance>
    <labels>area:test, area:frontend, kind:test-infra, phase:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T040702-6ce27625/manifest.json</file>
    <file>.ddx/executions/20260429T040702-6ce27625/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="813bc9a6d2a9cb58f7231d9c41e2255d4ee9015d">
diff --git a/.ddx/executions/20260429T040702-6ce27625/manifest.json b/.ddx/executions/20260429T040702-6ce27625/manifest.json
new file mode 100644
index 00000000..15910c9b
--- /dev/null
+++ b/.ddx/executions/20260429T040702-6ce27625/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T040702-6ce27625",
+  "bead_id": "ddx-fc6e5b14",
+  "base_rev": "67e7ba37db4d4926bbb5e896e5b383e816d90d3c",
+  "created_at": "2026-04-29T04:07:02.955919588Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-fc6e5b14",
+    "title": "Switch the default Playwright e2e harness to a Go-server fixture",
+    "description": "## Context\n\nThe frontend Playwright suite under `cli/internal/server/frontend` currently starts `bun run preview` on port 4173, which serves static SvelteKit output without the Go DDx backend. The parent e2e suite now includes direct API tests and route/page tests that require real DDx server data. TC-008 in `e2e/app.spec.ts` calls `/api/health`, `/api/documents`, `/api/beads`, `/api/beads/status`, `/api/personas`, and `/api/docs/graph`; those endpoints return 404 under static preview but return 200 from the Go server.\n\nBuild a fixture-backed default Playwright harness that boots the Go DDx server on a deterministic local port and points Playwright at it. The fixture must be self-contained so e2e runs do not read the developer's home config or the repository's live `.ddx/` state.\n\n## In-scope files\n\n- `cli/internal/server/frontend/playwright.config.ts` — change the default `webServer` to run the Go DDx server (`go run ./cli` from the repo root or a built `ddx` binary) on a deterministic port, set `use.baseURL` to that server, and keep any static-only preview coverage separate from the default backend-backed project.\n- `cli/internal/server/frontend/e2e/fixtures/` — add a fixture project directory containing minimal `.ddx/beads.jsonl` data with open, closed, ready, and blocked beads; a tiny `docs/` library; and any config needed by the server.\n- `cli/internal/server/frontend/e2e/app.spec.ts` — only TC-008 API-test alignment if the fixture data requires narrow expectation updates.\n\n## Out-of-scope\n\n- `cli/internal/server/frontend/playwright.demo.config.ts`.\n- `cli/internal/server/frontend/e2e/demo-recording.spec.ts`.\n- Go server source under `cli/internal/server/`; the API is assumed correct.\n- Frontend components or routes.\n- Migrating TC-001 through TC-007 page tests; those are separate child beads.\n\n## Rollback / cleanup\n\nDo not leave a half-populated fixture directory. Do not make tests pass by skipping or deleting them. The default test harness must use a temp copy or otherwise isolated fixture workspace, not the developer's actual repository checkout or `$HOME`.",
+    "acceptance": "cd cli/internal/server/frontend \u0026\u0026 bun run test:e2e --grep TC-008\ncd cli/internal/server/frontend \u0026\u0026 ! grep -rE '\\$HOME|~/' e2e/fixtures/\ncd cli/internal/server/frontend \u0026\u0026 grep -E 'webServer|baseURL|ddx|go run' playwright.config.ts\ncd cli/internal/server/frontend \u0026\u0026 git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts",
+    "parent": "ddx-ccdf9cf9",
+    "labels": [
+      "area:test",
+      "area:frontend",
+      "kind:test-infra",
+      "phase:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T04:07:02Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T04:07:02.252417746Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T040702-6ce27625",
+    "prompt": ".ddx/executions/20260429T040702-6ce27625/prompt.md",
+    "manifest": ".ddx/executions/20260429T040702-6ce27625/manifest.json",
+    "result": ".ddx/executions/20260429T040702-6ce27625/result.json",
+    "checks": ".ddx/executions/20260429T040702-6ce27625/checks.json",
+    "usage": ".ddx/executions/20260429T040702-6ce27625/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-fc6e5b14-20260429T040702-6ce27625"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T040702-6ce27625/result.json b/.ddx/executions/20260429T040702-6ce27625/result.json
new file mode 100644
index 00000000..8651ac3f
--- /dev/null
+++ b/.ddx/executions/20260429T040702-6ce27625/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-fc6e5b14",
+  "attempt_id": "20260429T040702-6ce27625",
+  "base_rev": "67e7ba37db4d4926bbb5e896e5b383e816d90d3c",
+  "result_rev": "e38318c63f17ddf3cc4b26c8544954c782ef522c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-db452fb4",
+  "duration_ms": 432314,
+  "tokens": 24229,
+  "cost_usd": 3.02389275,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T040702-6ce27625",
+  "prompt_file": ".ddx/executions/20260429T040702-6ce27625/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T040702-6ce27625/manifest.json",
+  "result_file": ".ddx/executions/20260429T040702-6ce27625/result.json",
+  "usage_file": ".ddx/executions/20260429T040702-6ce27625/usage.json",
+  "started_at": "2026-04-29T04:07:02.956186046Z",
+  "finished_at": "2026-04-29T04:14:15.270775016Z"
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
