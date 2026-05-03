<bead-review>
  <bead id=".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-86d32a20" iter=1>
    <title>ADR-022 step 5b: workers panel UI migration to derived-view (frontend)</title>
    <description>
Migrate the existing Svelte workers panel at cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/ to consume the new derived-view GraphQL query (added in step 5a). Surface freshness indicator (connected/stale/disconnected), duplicate-worker visibility (same project, multiple worker entries), and 'trusted-peer reported, not authoritative' labeling on reported data. Includes Houdini graphql query update + .gql file changes + +layout.svelte / +page.svelte changes. Plus a Playwright e2e test that loads the panel against fixture state. ~400 LOC frontend.
    </description>
    <acceptance>
1. Workers panel renders new fields: lastEventAt, mirrorFailuresCount, hadDroppedBackfill, freshness state.
2. Freshness indicator visible (e.g., colored badge connected/stale/disconnected).
3. Duplicate-worker display: when two workers exist under same projectRoot both appear distinctly.
4. UI labels reported data appropriately, e.g., 'reported by worker (not authoritative)' near server-derived fields per ADR-022 §auth-context UI labeling.
5. Houdini codegen runs clean (bun run houdini:generate).
6. Playwright e2e test (workers-panel-derived-view.spec.ts or extension to existing) renders fixture state and asserts the new fields/badges are visible.
7. bun test + bun test:e2e green; lefthook pre-commit passes.
Depends on step 5a backend.
    </acceptance>
    <labels>phase:2,  area:web,  kind:feature,  adr:022</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T073734-a85b291e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e73261e8ca057ea38592b8bd511cf83dd22bf746">
diff --git a/.ddx/executions/20260503T073734-a85b291e/result.json b/.ddx/executions/20260503T073734-a85b291e/result.json
new file mode 100644
index 00000000..38cc6a21
--- /dev/null
+++ b/.ddx/executions/20260503T073734-a85b291e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-7b753655-20260503T064620-a387a207-86d32a20",
+  "attempt_id": "20260503T073734-a85b291e",
+  "base_rev": "6f518876d9608366c0287577117256b249bea80e",
+  "result_rev": "2c24ef82f1dfe62a2902b1c4b5b8d8e1f7765081",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-137fe449",
+  "duration_ms": 400082,
+  "tokens": 21368,
+  "cost_usd": 2.12514625,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T073734-a85b291e",
+  "prompt_file": ".ddx/executions/20260503T073734-a85b291e/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T073734-a85b291e/manifest.json",
+  "result_file": ".ddx/executions/20260503T073734-a85b291e/result.json",
+  "usage_file": ".ddx/executions/20260503T073734-a85b291e/usage.json",
+  "started_at": "2026-05-03T07:37:36.000203409Z",
+  "finished_at": "2026-05-03T07:44:16.082285235Z"
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
