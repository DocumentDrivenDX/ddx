<bead-review>
  <bead id="ddx-b09a606b" iter=1>
    <title>FEAT-008 P11: real backend resolvers (swap Phase 2 stubs)</title>
    <description>
Replace stubbed resolvers from Phase 2 with real implementations. queueSummary: scan bead tracker. pluginsList/pluginDetail: shell ddx install list --json + registry scan. personaBind: write .ddx/config.yaml persona_bindings. paletteSearch: path+title prefix + FTS, 50-result cap, no body indexing in first pass. efficacyRows/efficacyAttempts: aggregate kind:cost + kind:routing evidence events from closed beads; memoize per (project, filters, max_event_sequence); invalidate on new evidence. workerDispatch(kind: realign-specs|run-checks) stub return queued-worker is acceptable here; real execution is a follow-up bead.
    </description>
    <acceptance>
No mock fallbacks remain; manual smoke test of each action works against a real DDx instance.
    </acceptance>
    <labels>ui, feat-008, phase-11, backend</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9a7585ad707ffe1e1c54030a33823a7d85726bc0">
commit 9a7585ad707ffe1e1c54030a33823a7d85726bc0
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 05:48:08 2026 -0400

    chore: add execution evidence [20260422T094035-]

diff --git a/.ddx/executions/20260422T094035-6369cd19/manifest.json b/.ddx/executions/20260422T094035-6369cd19/manifest.json
new file mode 100644
index 00000000..a1271a17
--- /dev/null
+++ b/.ddx/executions/20260422T094035-6369cd19/manifest.json
@@ -0,0 +1,88 @@
+{
+  "attempt_id": "20260422T094035-6369cd19",
+  "bead_id": "ddx-b09a606b",
+  "base_rev": "822c75dc093f52fccc5dca057b1f23bff892463b",
+  "created_at": "2026-04-22T09:40:35.514213382Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b09a606b",
+    "title": "FEAT-008 P11: real backend resolvers (swap Phase 2 stubs)",
+    "description": "Replace stubbed resolvers from Phase 2 with real implementations. queueSummary: scan bead tracker. pluginsList/pluginDetail: shell ddx install list --json + registry scan. personaBind: write .ddx/config.yaml persona_bindings. paletteSearch: path+title prefix + FTS, 50-result cap, no body indexing in first pass. efficacyRows/efficacyAttempts: aggregate kind:cost + kind:routing evidence events from closed beads; memoize per (project, filters, max_event_sequence); invalidate on new evidence. workerDispatch(kind: realign-specs|run-checks) stub return queued-worker is acceptable here; real execution is a follow-up bead.",
+    "acceptance": "No mock fallbacks remain; manual smoke test of each action works against a real DDx instance.",
+    "parent": "ddx-4a9d30db",
+    "labels": [
+      "ui",
+      "feat-008",
+      "phase-11",
+      "backend"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-22T09:40:35Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1682344",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T09:36:37.485329618Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260422T091838-771b3ab6\",\"harness\":\"codex\",\"input_tokens\":15876340,\"output_tokens\":31980,\"total_tokens\":15908320,\"cost_usd\":0,\"duration_ms\":1078879,\"exit_code\":0}",
+          "created_at": "2026-04-22T09:36:37.559283427Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=15908320"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"fallback_chain\":[],\"resolved_model\":\"\",\"resolved_provider\":\"codex\"}",
+          "created_at": "2026-04-22T09:36:40.13534365Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex"
+        },
+        {
+          "actor": "ddx",
+          "body": "BLOCK: `cli/internal/server/graphql/resolver_feat008.go`, `PluginDispatch` returns `queued-plugin-...` without invoking any real plugin install/dispatch path or validating the requested action/scope.\nBLOCK: `cli/internal/server/graphql/resolver_feat008.go`, `ComparisonDispatch` returns `queued-comparison-...` from `time.Now()` and `len(arms)` only; no real comparison job/record is created.\nREQUEST_CHANGES: `cli/internal/server/graphql/feat008_test.go`, `TestIntegration_FEAT008BackendOperations` smoke-calls `pluginDispatch` and `comparisonDispatch` but does not assert their returned IDs/states or verify any real DDx side effect.",
+          "created_at": "2026-04-22T09:38:55.157786127Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-22T09:38:55.21669684Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "ddx",
+          "body": "post-merge review: BLOCK (flagged for human)\nBLOCK: `cli/internal/server/graphql/resolver_feat008.go`, `PluginDispatch` returns `queued-plugin-...` without invoking any real plugin install/dispatch path or validating the requested action/scope.\nBLOCK: `cli/internal/server/graphql/resolver_feat008.go`, `ComparisonDispatch` returns `queued-comparison-...` from `time.Now()` and `len(arms)` only; no real comparison job/record is created.\nREQUEST_CHANGES: `cli/internal/server/graphql/feat008_test.go`, `TestIntegration_FEAT008BackendOperations` smoke-calls `pluginDispatch` and `comparisonDispatch` but does not assert their returned IDs/states or verify any real DDx side effect.\nresult_rev=486316e3e5acb669b3f52607a336fbd53f4ed633\nbase_rev=40919c6b5937abac116d30144aecec1f2d239d03",
+          "created_at": "2026-04-22T09:38:55.27343722Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-22T09:40:35.044614538Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260422T094035-6369cd19",
+    "prompt": ".ddx/executions/20260422T094035-6369cd19/prompt.md",
+    "manifest": ".ddx/executions/20260422T094035-6369cd19/manifest.json",
+    "result": ".ddx/executions/20260422T094035-6369cd19/result.json",
+    "checks": ".ddx/executions/20260422T094035-6369cd19/checks.json",
+    "usage": ".ddx/executions/20260422T094035-6369cd19/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b09a606b-20260422T094035-6369cd19"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260422T094035-6369cd19/result.json b/.ddx/executions/20260422T094035-6369cd19/result.json
new file mode 100644
index 00000000..6bd13812
--- /dev/null
+++ b/.ddx/executions/20260422T094035-6369cd19/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-b09a606b",
+  "attempt_id": "20260422T094035-6369cd19",
+  "base_rev": "822c75dc093f52fccc5dca057b1f23bff892463b",
+  "result_rev": "d7f5b4f871fcb550a00acd1f707369aeaa0306df",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-ad1dd18f",
+  "duration_ms": 451698,
+  "tokens": 3271857,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T094035-6369cd19",
+  "prompt_file": ".ddx/executions/20260422T094035-6369cd19/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T094035-6369cd19/manifest.json",
+  "result_file": ".ddx/executions/20260422T094035-6369cd19/result.json",
+  "usage_file": ".ddx/executions/20260422T094035-6369cd19/usage.json",
+  "started_at": "2026-04-22T09:40:35.514513299Z",
+  "finished_at": "2026-04-22T09:48:07.213440992Z"
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
## Review: ddx-b09a606b iter 1

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
