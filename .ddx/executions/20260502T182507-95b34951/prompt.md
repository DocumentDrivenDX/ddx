<bead-review>
  <bead id="ddx-e869a89c" iter=1>
    <title>B14.7: Frontend - /federation route + node-picker + scope toggle + status badges</title>
    <description>
SvelteKit frontend additions: /federation overview route showing registered nodes with status badges (active, stale, offline, degraded). Extend node-picker to show all spokes when in federation mode. Combined views accept ?scope=federation to toggle local vs federated data via the federated* GraphQL queries from B14.6b. Per-row node badges. Version-skew, stale, and offline visual states. Direct spoke UI fallback link reachable from each node row in /federation. Houdini codegen run.
    </description>
    <acceptance>
Route /federation renders list of registered nodes with status badges. Node-picker shows spokes. ?scope=federation toggles combined views to use federated* queries. Per-row node badges visible. Stale/offline/degraded each have distinct visual treatment. Direct spoke URL link present and clickable on each /federation row. Houdini types regenerated and committed.
    </acceptance>
    <labels>phase:2, story:14</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="dc7f1be7558e7c6f2d138fa380131854e8133268">
commit dc7f1be7558e7c6f2d138fa380131854e8133268
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 14:25:05 2026 -0400

    chore: add execution evidence [20260502T181621-]

diff --git a/.ddx/executions/20260502T181621-7a485593/manifest.json b/.ddx/executions/20260502T181621-7a485593/manifest.json
new file mode 100644
index 00000000..44130161
--- /dev/null
+++ b/.ddx/executions/20260502T181621-7a485593/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260502T181621-7a485593",
+  "bead_id": "ddx-e869a89c",
+  "base_rev": "b694f13e844598a05bbb66a032fb1c86435289f4",
+  "created_at": "2026-05-02T18:16:23.212546748Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-e869a89c",
+    "title": "B14.7: Frontend - /federation route + node-picker + scope toggle + status badges",
+    "description": "SvelteKit frontend additions: /federation overview route showing registered nodes with status badges (active, stale, offline, degraded). Extend node-picker to show all spokes when in federation mode. Combined views accept ?scope=federation to toggle local vs federated data via the federated* GraphQL queries from B14.6b. Per-row node badges. Version-skew, stale, and offline visual states. Direct spoke UI fallback link reachable from each node row in /federation. Houdini codegen run.",
+    "acceptance": "Route /federation renders list of registered nodes with status badges. Node-picker shows spokes. ?scope=federation toggles combined views to use federated* queries. Per-row node badges visible. Stale/offline/degraded each have distinct visual treatment. Direct spoke URL link present and clickable on each /federation row. Houdini types regenerated and committed.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T18:16:21Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3037698",
+      "execute-loop-heartbeat-at": "2026-05-02T18:16:21.936068543Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T181621-7a485593",
+    "prompt": ".ddx/executions/20260502T181621-7a485593/prompt.md",
+    "manifest": ".ddx/executions/20260502T181621-7a485593/manifest.json",
+    "result": ".ddx/executions/20260502T181621-7a485593/result.json",
+    "checks": ".ddx/executions/20260502T181621-7a485593/checks.json",
+    "usage": ".ddx/executions/20260502T181621-7a485593/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-e869a89c-20260502T181621-7a485593"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T181621-7a485593/result.json b/.ddx/executions/20260502T181621-7a485593/result.json
new file mode 100644
index 00000000..2eb4a3a2
--- /dev/null
+++ b/.ddx/executions/20260502T181621-7a485593/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-e869a89c",
+  "attempt_id": "20260502T181621-7a485593",
+  "base_rev": "b694f13e844598a05bbb66a032fb1c86435289f4",
+  "result_rev": "1f13a93fd3d39c232269ff7323fc73580de35991",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-5a916e47",
+  "duration_ms": 520527,
+  "tokens": 32078,
+  "cost_usd": 4.509465700000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T181621-7a485593",
+  "prompt_file": ".ddx/executions/20260502T181621-7a485593/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T181621-7a485593/manifest.json",
+  "result_file": ".ddx/executions/20260502T181621-7a485593/result.json",
+  "usage_file": ".ddx/executions/20260502T181621-7a485593/usage.json",
+  "started_at": "2026-05-02T18:16:23.212841456Z",
+  "finished_at": "2026-05-02T18:25:03.740332094Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-e869a89c iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>
