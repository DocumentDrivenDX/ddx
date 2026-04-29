<bead-review>
  <bead id="ddx-0724aaba" iter=1>
    <title>Migrate app.spec page-data tests to the seeded fixture</title>
    <description>
## Context

Several page-data tests in `cli/internal/server/frontend/e2e/app.spec.ts` fail because the default Playwright harness previously served static preview output with no backend data. Once the Go-server fixture harness is in place, TC-002, TC-003, TC-005, and TC-007 should navigate through `/nodes/&lt;fixture-node-id&gt;/projects/&lt;fixture-project-id&gt;/...` and assert against the seeded fixture documents, beads, personas, and graph data instead of assuming developer-local state.

Rewrite only those TC blocks to use the fixture-backed app and current node/project router. Preserve the intended coverage of the existing TC IDs; update selectors and data expectations only where the current UI and fixture data require it.

## In-scope files

- `cli/internal/server/frontend/e2e/app.spec.ts` — TC-002, TC-003, TC-005, and TC-007 only.

## Out-of-scope

- TC-001 and TC-008 in `e2e/app.spec.ts`.
- Other e2e spec files.
- `cli/internal/server/frontend/playwright.config.ts` and fixture seeding except for reading fixture IDs/data created by the prerequisite bead.
- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.
- Frontend source changes under `src/` or Go server changes under `cli/internal/server/`.
- Deleting, skipping, or weakening TC coverage to get a green run.

## Rollback / cleanup

If the fixture lacks data needed for these tests, stop and file a fixture-followup or coordinate with the harness bead rather than silently depending on the developer's home repository or config.
    </description>
    <acceptance>
cd cli/internal/server/frontend &amp;&amp; bun run test:e2e e2e/app.spec.ts --grep "TC-00(2|3|5|7)"
cd cli/internal/server/frontend &amp;&amp; git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts
    </acceptance>
    <labels>area:test, area:frontend, kind:test-infra, phase:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T044933-d9edf2a0/manifest.json</file>
    <file>.ddx/executions/20260429T044933-d9edf2a0/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="92e6e261cf359a0ae4c9e0ee0b154541ce112a9d">
diff --git a/.ddx/executions/20260429T044933-d9edf2a0/manifest.json b/.ddx/executions/20260429T044933-d9edf2a0/manifest.json
new file mode 100644
index 00000000..df03f382
--- /dev/null
+++ b/.ddx/executions/20260429T044933-d9edf2a0/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T044933-d9edf2a0",
+  "bead_id": "ddx-0724aaba",
+  "base_rev": "774707c2317843fe533bd0a312a31b9177ffab36",
+  "created_at": "2026-04-29T04:49:34.338557526Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-0724aaba",
+    "title": "Migrate app.spec page-data tests to the seeded fixture",
+    "description": "## Context\n\nSeveral page-data tests in `cli/internal/server/frontend/e2e/app.spec.ts` fail because the default Playwright harness previously served static preview output with no backend data. Once the Go-server fixture harness is in place, TC-002, TC-003, TC-005, and TC-007 should navigate through `/nodes/\u003cfixture-node-id\u003e/projects/\u003cfixture-project-id\u003e/...` and assert against the seeded fixture documents, beads, personas, and graph data instead of assuming developer-local state.\n\nRewrite only those TC blocks to use the fixture-backed app and current node/project router. Preserve the intended coverage of the existing TC IDs; update selectors and data expectations only where the current UI and fixture data require it.\n\n## In-scope files\n\n- `cli/internal/server/frontend/e2e/app.spec.ts` — TC-002, TC-003, TC-005, and TC-007 only.\n\n## Out-of-scope\n\n- TC-001 and TC-008 in `e2e/app.spec.ts`.\n- Other e2e spec files.\n- `cli/internal/server/frontend/playwright.config.ts` and fixture seeding except for reading fixture IDs/data created by the prerequisite bead.\n- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.\n- Frontend source changes under `src/` or Go server changes under `cli/internal/server/`.\n- Deleting, skipping, or weakening TC coverage to get a green run.\n\n## Rollback / cleanup\n\nIf the fixture lacks data needed for these tests, stop and file a fixture-followup or coordinate with the harness bead rather than silently depending on the developer's home repository or config.",
+    "acceptance": "cd cli/internal/server/frontend \u0026\u0026 bun run test:e2e e2e/app.spec.ts --grep \"TC-00(2|3|5|7)\"\ncd cli/internal/server/frontend \u0026\u0026 git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts",
+    "parent": "ddx-ccdf9cf9",
+    "labels": [
+      "area:test",
+      "area:frontend",
+      "kind:test-infra",
+      "phase:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T04:49:33Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T04:49:33.689842408Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T044933-d9edf2a0",
+    "prompt": ".ddx/executions/20260429T044933-d9edf2a0/prompt.md",
+    "manifest": ".ddx/executions/20260429T044933-d9edf2a0/manifest.json",
+    "result": ".ddx/executions/20260429T044933-d9edf2a0/result.json",
+    "checks": ".ddx/executions/20260429T044933-d9edf2a0/checks.json",
+    "usage": ".ddx/executions/20260429T044933-d9edf2a0/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-0724aaba-20260429T044933-d9edf2a0"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T044933-d9edf2a0/result.json b/.ddx/executions/20260429T044933-d9edf2a0/result.json
new file mode 100644
index 00000000..09f8de82
--- /dev/null
+++ b/.ddx/executions/20260429T044933-d9edf2a0/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-0724aaba",
+  "attempt_id": "20260429T044933-d9edf2a0",
+  "base_rev": "774707c2317843fe533bd0a312a31b9177ffab36",
+  "result_rev": "1d7861082cb60af93749844d8bf0bad9d190ab4e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-426a6d99",
+  "duration_ms": 1097678,
+  "tokens": 40010,
+  "cost_usd": 6.9099034999999995,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T044933-d9edf2a0",
+  "prompt_file": ".ddx/executions/20260429T044933-d9edf2a0/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T044933-d9edf2a0/manifest.json",
+  "result_file": ".ddx/executions/20260429T044933-d9edf2a0/result.json",
+  "usage_file": ".ddx/executions/20260429T044933-d9edf2a0/usage.json",
+  "started_at": "2026-04-29T04:49:34.338813026Z",
+  "finished_at": "2026-04-29T05:07:52.016950205Z"
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
