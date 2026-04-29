<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce" iter=1>
    <title>[read-coverage] add ddx_bead_blocked + ddx_bead_dep_tree MCP tools</title>
    <description>
FEAT-004/002 gap. HTTP has GET /api/beads/blocked and GET /api/beads/dep/tree/{id}. MCP has ddx_list_beads, ddx_show_bead, ddx_bead_ready, ddx_bead_status but no blocked or dep-tree tools. Agents managing the bead queue via MCP cannot check blocked state or inspect dependency trees. Add both tools.
    </description>
    <acceptance/>
    <labels>read-coverage, server, mcp, feat-004, feat-002</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c4b785d2fdb2f6365cf523fba14e50d656e6e5a4">
commit c4b785d2fdb2f6365cf523fba14e50d656e6e5a4
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 19:35:27 2026 -0400

    chore: add execution evidence [20260429T233205-]

diff --git a/.ddx/executions/20260429T233205-bcb836c6/manifest.json b/.ddx/executions/20260429T233205-bcb836c6/manifest.json
new file mode 100644
index 00000000..2eb1cd70
--- /dev/null
+++ b/.ddx/executions/20260429T233205-bcb836c6/manifest.json
@@ -0,0 +1,119 @@
+{
+  "attempt_id": "20260429T233205-bcb836c6",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce",
+  "base_rev": "0eb0715b802eee303d115627eb903d0f6523fca7",
+  "created_at": "2026-04-29T23:32:06.661263003Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce",
+    "title": "[read-coverage] add ddx_bead_blocked + ddx_bead_dep_tree MCP tools",
+    "description": "FEAT-004/002 gap. HTTP has GET /api/beads/blocked and GET /api/beads/dep/tree/{id}. MCP has ddx_list_beads, ddx_show_bead, ddx_bead_ready, ddx_bead_status but no blocked or dep-tree tools. Agents managing the bead queue via MCP cannot check blocked state or inspect dependency trees. Add both tools.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "mcp",
+      "feat-004",
+      "feat-002"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T23:32:05Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-29T19:17:34.531090602Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260429T191154-99abef11\",\"harness\":\"claude\",\"model\":\"sonnet\",\"input_tokens\":25,\"output_tokens\":6428,\"total_tokens\":6453,\"cost_usd\":0.606214,\"duration_ms\":339518,\"exit_code\":0}",
+          "created_at": "2026-04-29T19:17:34.628334445Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=6453 cost_usd=0.6062 model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"sonnet\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-29T19:17:38.542777442Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "Diff under review contains only execution manifest/result JSON artifacts. No MCP tool implementation (ddx_bead_blocked, ddx_bead_dep_tree) or tests are present in the changed files, so acceptance cannot be evaluated.\nharness=claude\nmodel=opus\ninput_bytes=5690\noutput_bytes=553\nelapsed_ms=5406",
+          "created_at": "2026-04-29T19:17:44.149514324Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-29T19:17:44.256350077Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff under review contains only execution manifest/result JSON artifacts. No MCP tool implementation (ddx_bead_blocked, ddx_bead_dep_tree) or tests are present in the changed files, so acceptance cannot be evaluated.\nresult_rev=d9e2fa6209cc05224c7f0294f531d6a6b84930f1\nbase_rev=c8f5c4f575069a93b97d783c73918c05c3b26cc4",
+          "created_at": "2026-04-29T19:17:44.349039048Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:28:36.985724086Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:28:37.108670914Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-29T23:28:37.193182908Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-29T23:28:37.365970559Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-29T23:32:05.793486211Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T233205-bcb836c6",
+    "prompt": ".ddx/executions/20260429T233205-bcb836c6/prompt.md",
+    "manifest": ".ddx/executions/20260429T233205-bcb836c6/manifest.json",
+    "result": ".ddx/executions/20260429T233205-bcb836c6/result.json",
+    "checks": ".ddx/executions/20260429T233205-bcb836c6/checks.json",
+    "usage": ".ddx/executions/20260429T233205-bcb836c6/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce-20260429T233205-bcb836c6"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T233205-bcb836c6/result.json b/.ddx/executions/20260429T233205-bcb836c6/result.json
new file mode 100644
index 00000000..029eb062
--- /dev/null
+++ b/.ddx/executions/20260429T233205-bcb836c6/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce",
+  "attempt_id": "20260429T233205-bcb836c6",
+  "base_rev": "0eb0715b802eee303d115627eb903d0f6523fca7",
+  "result_rev": "05236b2afa0321ad8ff22b29a4241f3a3c650bbc",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c7307619",
+  "duration_ms": 198164,
+  "tokens": 7519,
+  "cost_usd": 1.1036115,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T233205-bcb836c6",
+  "prompt_file": ".ddx/executions/20260429T233205-bcb836c6/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T233205-bcb836c6/manifest.json",
+  "result_file": ".ddx/executions/20260429T233205-bcb836c6/result.json",
+  "usage_file": ".ddx/executions/20260429T233205-bcb836c6/usage.json",
+  "started_at": "2026-04-29T23:32:06.661625711Z",
+  "finished_at": "2026-04-29T23:35:24.825810022Z"
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
## Review: .execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-6d9490ce iter 1

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
