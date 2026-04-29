<bead-review>
  <bead id="ddx-27586b57" iter=1>
    <title>[artifact-run-arch] update prd.md for artifact + 3-layer architecture</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md.

Scope of PRD update:

(1) Restructure Problem section into 6 clusters keyed to thesis principles in product-vision.md:
- Abstraction (no structure / no composition / no integrity guarantees / no transferability)
- Iteration over tracked work (no reusable work-item store / no reusable execution evidence)
- Methodology plurality (no reusable agent dispatch — every workflow tool reinvents)
- LLM physics (NEW cluster — token cost as a permanent constraint; nothing in current PRD)
- Evidence/provenance (no provenance for generated artifacts; no measurement; no feedback capture)
- Human-AI collaboration (no composition for handoffs; no discoverability; no network access)

Existing problem bullets re-grouped under these headers; add the LLM-physics cluster.

(2) Goals (primary): reword #1; add three-layer-architecture goal; add generated-artifact-regeneration goal; add 100% read coverage on HTTP/MCP goal.

(3) Non-goals: sharpen loop wording — DDx owns mechanical queue drain (ddx work); content-aware supervisory decisions remain plugin/HELIX. Add: 'DDx does not catalog run types beyond the three layers.'

(4) Feature blurbs: FEAT-001 (CLI: run/try/work top-level + ddx agent passthrough); FEAT-005 (multi-media identity); FEAT-006 (layer-1 consumer + structural passthrough); FEAT-007 (sidecar + generated_by + separate staleness); FEAT-008/021 (media-type rendering + regenerate + layer-aware run views); FEAT-010 (3-layer architecture + substrate unification); FEAT-019 (child of FEAT-010 — evaluation UX, workflow shapes -&gt; skills).

Summary: artifacts (multi-media); three-layer run architecture; one substrate; invocation upstream.

Do NOT touch docs/helix/01-frame/principles.md.
    </description>
    <acceptance/>
    <labels>frame, plan-2026-04-29</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T175518-a355fb38/manifest.json</file>
    <file>.ddx/executions/20260429T175518-a355fb38/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5beb855d6875b46fa8e535a7c500e1b8aa43e0ea">
diff --git a/.ddx/executions/20260429T175518-a355fb38/manifest.json b/.ddx/executions/20260429T175518-a355fb38/manifest.json
new file mode 100644
index 00000000..8dd15d6d
--- /dev/null
+++ b/.ddx/executions/20260429T175518-a355fb38/manifest.json
@@ -0,0 +1,35 @@
+{
+  "attempt_id": "20260429T175518-a355fb38",
+  "bead_id": "ddx-27586b57",
+  "base_rev": "51ca473ba8f5c3e887646570329b08e1b22c14f8",
+  "created_at": "2026-04-29T17:55:19.505209925Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-27586b57",
+    "title": "[artifact-run-arch] update prd.md for artifact + 3-layer architecture",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md.\n\nScope of PRD update:\n\n(1) Restructure Problem section into 6 clusters keyed to thesis principles in product-vision.md:\n- Abstraction (no structure / no composition / no integrity guarantees / no transferability)\n- Iteration over tracked work (no reusable work-item store / no reusable execution evidence)\n- Methodology plurality (no reusable agent dispatch — every workflow tool reinvents)\n- LLM physics (NEW cluster — token cost as a permanent constraint; nothing in current PRD)\n- Evidence/provenance (no provenance for generated artifacts; no measurement; no feedback capture)\n- Human-AI collaboration (no composition for handoffs; no discoverability; no network access)\n\nExisting problem bullets re-grouped under these headers; add the LLM-physics cluster.\n\n(2) Goals (primary): reword #1; add three-layer-architecture goal; add generated-artifact-regeneration goal; add 100% read coverage on HTTP/MCP goal.\n\n(3) Non-goals: sharpen loop wording — DDx owns mechanical queue drain (ddx work); content-aware supervisory decisions remain plugin/HELIX. Add: 'DDx does not catalog run types beyond the three layers.'\n\n(4) Feature blurbs: FEAT-001 (CLI: run/try/work top-level + ddx agent passthrough); FEAT-005 (multi-media identity); FEAT-006 (layer-1 consumer + structural passthrough); FEAT-007 (sidecar + generated_by + separate staleness); FEAT-008/021 (media-type rendering + regenerate + layer-aware run views); FEAT-010 (3-layer architecture + substrate unification); FEAT-019 (child of FEAT-010 — evaluation UX, workflow shapes -\u003e skills).\n\nSummary: artifacts (multi-media); three-layer run architecture; one substrate; invocation upstream.\n\nDo NOT touch docs/helix/01-frame/principles.md.",
+    "labels": [
+      "frame",
+      "plan-2026-04-29"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T17:55:16Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T17:55:16.585336646Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T175518-a355fb38",
+    "prompt": ".ddx/executions/20260429T175518-a355fb38/prompt.md",
+    "manifest": ".ddx/executions/20260429T175518-a355fb38/manifest.json",
+    "result": ".ddx/executions/20260429T175518-a355fb38/result.json",
+    "checks": ".ddx/executions/20260429T175518-a355fb38/checks.json",
+    "usage": ".ddx/executions/20260429T175518-a355fb38/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-27586b57-20260429T175518-a355fb38"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T175518-a355fb38/result.json b/.ddx/executions/20260429T175518-a355fb38/result.json
new file mode 100644
index 00000000..7d6c1ed2
--- /dev/null
+++ b/.ddx/executions/20260429T175518-a355fb38/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-27586b57",
+  "attempt_id": "20260429T175518-a355fb38",
+  "base_rev": "51ca473ba8f5c3e887646570329b08e1b22c14f8",
+  "result_rev": "bebb231bc28254fc270a78732dcbff4f6c7f1a39",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-bf791d91",
+  "duration_ms": 261595,
+  "tokens": 13421,
+  "cost_usd": 0.6535323000000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T175518-a355fb38",
+  "prompt_file": ".ddx/executions/20260429T175518-a355fb38/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T175518-a355fb38/manifest.json",
+  "result_file": ".ddx/executions/20260429T175518-a355fb38/result.json",
+  "usage_file": ".ddx/executions/20260429T175518-a355fb38/usage.json",
+  "started_at": "2026-04-29T17:55:19.505539592Z",
+  "finished_at": "2026-04-29T17:59:41.100724861Z"
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
