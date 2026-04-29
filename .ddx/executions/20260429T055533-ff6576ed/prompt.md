<bead-review>
  <bead id="ddx-ae4e467b" iter=1>
    <title>Migrate screenshot e2e coverage to fixture-backed pages</title>
    <description>
## Context

`cli/internal/server/frontend/e2e/screenshots.spec.ts` is part of the page-data-dependent e2e drift: it should render current UI pages from the fixture-backed Go DDx server rather than static preview or developer-local data. This bead is scoped to making the screenshot spec navigate through fixture node/project routes and assert/render stable seeded pages.

Do not rebaseline visual snapshots as part of this task. If pixel diffs show that baselines are genuinely stale after the fixture-backed harness is working, file a separate follow-up bead for visual baseline review.

## In-scope files

- `cli/internal/server/frontend/e2e/screenshots.spec.ts`.

## Out-of-scope

- Visual regression baseline files and screenshot artifact updates.
- `cli/internal/server/frontend/e2e/app.spec.ts`.
- `cli/internal/server/frontend/e2e/navigation.spec.ts`, `e2e/beads-smoke.spec.ts`, `e2e/executions.spec.ts`, and `e2e/workers.spec.ts`.
- `cli/internal/server/frontend/playwright.config.ts` and fixture creation except reading fixture IDs/data created by the prerequisite bead.
- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.
- Frontend source or Go server changes.
- Skipping screenshot coverage to make the run green.

## Rollback / cleanup

Keep the screenshot spec deterministic by using seeded fixture pages and stable selectors. Do not update baselines or recorded media in this bead.
    </description>
    <acceptance>
cd cli/internal/server/frontend &amp;&amp; bun run test:e2e e2e/screenshots.spec.ts
cd cli/internal/server/frontend &amp;&amp; git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts
    </acceptance>
    <labels>area:test, area:frontend, kind:test-infra, phase:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T053212-f81451ea/manifest.json</file>
    <file>.ddx/executions/20260429T053212-f81451ea/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="467a2ff3145edc97d6771ce58deaf9a9b1e8c5bd">
diff --git a/.ddx/executions/20260429T053212-f81451ea/manifest.json b/.ddx/executions/20260429T053212-f81451ea/manifest.json
new file mode 100644
index 00000000..3f79370d
--- /dev/null
+++ b/.ddx/executions/20260429T053212-f81451ea/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260429T053212-f81451ea",
+  "bead_id": "ddx-ae4e467b",
+  "base_rev": "c21699ba66625d0ef0a46ad4a6ecc421dd363feb",
+  "created_at": "2026-04-29T05:32:13.15202224Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ae4e467b",
+    "title": "Migrate screenshot e2e coverage to fixture-backed pages",
+    "description": "## Context\n\n`cli/internal/server/frontend/e2e/screenshots.spec.ts` is part of the page-data-dependent e2e drift: it should render current UI pages from the fixture-backed Go DDx server rather than static preview or developer-local data. This bead is scoped to making the screenshot spec navigate through fixture node/project routes and assert/render stable seeded pages.\n\nDo not rebaseline visual snapshots as part of this task. If pixel diffs show that baselines are genuinely stale after the fixture-backed harness is working, file a separate follow-up bead for visual baseline review.\n\n## In-scope files\n\n- `cli/internal/server/frontend/e2e/screenshots.spec.ts`.\n\n## Out-of-scope\n\n- Visual regression baseline files and screenshot artifact updates.\n- `cli/internal/server/frontend/e2e/app.spec.ts`.\n- `cli/internal/server/frontend/e2e/navigation.spec.ts`, `e2e/beads-smoke.spec.ts`, `e2e/executions.spec.ts`, and `e2e/workers.spec.ts`.\n- `cli/internal/server/frontend/playwright.config.ts` and fixture creation except reading fixture IDs/data created by the prerequisite bead.\n- `cli/internal/server/frontend/e2e/demo-recording.spec.ts` and `playwright.demo.config.ts`.\n- Frontend source or Go server changes.\n- Skipping screenshot coverage to make the run green.\n\n## Rollback / cleanup\n\nKeep the screenshot spec deterministic by using seeded fixture pages and stable selectors. Do not update baselines or recorded media in this bead.",
+    "acceptance": "cd cli/internal/server/frontend \u0026\u0026 bun run test:e2e e2e/screenshots.spec.ts\ncd cli/internal/server/frontend \u0026\u0026 git diff --exit-code -- playwright.demo.config.ts e2e/demo-recording.spec.ts",
+    "parent": "ddx-ccdf9cf9",
+    "labels": [
+      "area:test",
+      "area:frontend",
+      "kind:test-infra",
+      "phase:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T05:32:12Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T05:32:12.441270913Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T053212-f81451ea",
+    "prompt": ".ddx/executions/20260429T053212-f81451ea/prompt.md",
+    "manifest": ".ddx/executions/20260429T053212-f81451ea/manifest.json",
+    "result": ".ddx/executions/20260429T053212-f81451ea/result.json",
+    "checks": ".ddx/executions/20260429T053212-f81451ea/checks.json",
+    "usage": ".ddx/executions/20260429T053212-f81451ea/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ae4e467b-20260429T053212-f81451ea"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T053212-f81451ea/result.json b/.ddx/executions/20260429T053212-f81451ea/result.json
new file mode 100644
index 00000000..d5b67115
--- /dev/null
+++ b/.ddx/executions/20260429T053212-f81451ea/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ae4e467b",
+  "attempt_id": "20260429T053212-f81451ea",
+  "base_rev": "c21699ba66625d0ef0a46ad4a6ecc421dd363feb",
+  "result_rev": "69ddc31887ba951bdafa286d74963582d3a21f74",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2931309a",
+  "duration_ms": 1397224,
+  "tokens": 42,
+  "cost_usd": 2.5108509999999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T053212-f81451ea",
+  "prompt_file": ".ddx/executions/20260429T053212-f81451ea/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T053212-f81451ea/manifest.json",
+  "result_file": ".ddx/executions/20260429T053212-f81451ea/result.json",
+  "usage_file": ".ddx/executions/20260429T053212-f81451ea/usage.json",
+  "started_at": "2026-04-29T05:32:13.15230399Z",
+  "finished_at": "2026-04-29T05:55:30.376508385Z"
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
