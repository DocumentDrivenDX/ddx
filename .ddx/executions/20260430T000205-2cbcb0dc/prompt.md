<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8" iter=1>
    <title>[read-coverage] add bead evidence + cooldown + routing HTTP + MCP</title>
    <description>
FEAT-004 gap. CLI has: ddx bead evidence list &lt;id&gt;, ddx bead cooldown show &lt;id&gt;, ddx bead routing. None of these are exposed over HTTP or MCP. Add GET /api/beads/{id}/evidence, GET /api/beads/{id}/cooldown, GET /api/beads/{id}/routing HTTP routes and corresponding MCP tools (ddx_bead_evidence, ddx_bead_cooldown). Low priority — these are operational details primarily consumed by the execute-loop itself.
    </description>
    <acceptance/>
    <labels>read-coverage, server, http, mcp, feat-004</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b67961b18ac2acb37c9fea55f62b76aaf435f7f7">
commit b67961b18ac2acb37c9fea55f62b76aaf435f7f7
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 20:02:03 2026 -0400

    chore: add execution evidence [20260429T235731-]

diff --git a/.ddx/executions/20260429T235731-b7a9889b/manifest.json b/.ddx/executions/20260429T235731-b7a9889b/manifest.json
new file mode 100644
index 00000000..d76f961f
--- /dev/null
+++ b/.ddx/executions/20260429T235731-b7a9889b/manifest.json
@@ -0,0 +1,119 @@
+{
+  "attempt_id": "20260429T235731-b7a9889b",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8",
+  "base_rev": "c00f544b8c368a93fdb1f8422f4a9b3526df7364",
+  "created_at": "2026-04-29T23:57:32.557385213Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8",
+    "title": "[read-coverage] add bead evidence + cooldown + routing HTTP + MCP",
+    "description": "FEAT-004 gap. CLI has: ddx bead evidence list \u003cid\u003e, ddx bead cooldown show \u003cid\u003e, ddx bead routing. None of these are exposed over HTTP or MCP. Add GET /api/beads/{id}/evidence, GET /api/beads/{id}/cooldown, GET /api/beads/{id}/routing HTTP routes and corresponding MCP tools (ddx_bead_evidence, ddx_bead_cooldown). Low priority — these are operational details primarily consumed by the execute-loop itself.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "http",
+      "mcp",
+      "feat-004"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T23:57:31Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-29T20:38:55.426645969Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260429T202423-4fc8564e\",\"harness\":\"claude\",\"model\":\"sonnet\",\"input_tokens\":49,\"output_tokens\":19204,\"total_tokens\":19253,\"cost_usd\":1.6286221,\"duration_ms\":871076,\"exit_code\":0}",
+          "created_at": "2026-04-29T20:38:55.533018368Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=19253 cost_usd=1.6286 model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"sonnet\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-29T20:38:59.333022114Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "Diff under review only contains execution evidence artifacts (manifest.json, result.json). No HTTP routes or MCP tools are present in the diff to evaluate against the acceptance criteria.\nharness=claude\nmodel=opus\ninput_bytes=5934\noutput_bytes=539\nelapsed_ms=5095",
+          "created_at": "2026-04-29T20:39:06.611858455Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-29T20:39:06.707601072Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff under review only contains execution evidence artifacts (manifest.json, result.json). No HTTP routes or MCP tools are present in the diff to evaluate against the acceptance criteria.\nresult_rev=5c5bf0201c3c8760c6c503a2369171b96d98e957\nbase_rev=d969131fb344bcd33940ac4c920dbd8d59940c13",
+          "created_at": "2026-04-29T20:39:06.802009815Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:28:58.942891581Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:28:59.041145852Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-29T23:28:59.127663261Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-29T23:28:59.304295865Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-29T23:57:31.714442331Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T235731-b7a9889b",
+    "prompt": ".ddx/executions/20260429T235731-b7a9889b/prompt.md",
+    "manifest": ".ddx/executions/20260429T235731-b7a9889b/manifest.json",
+    "result": ".ddx/executions/20260429T235731-b7a9889b/result.json",
+    "checks": ".ddx/executions/20260429T235731-b7a9889b/checks.json",
+    "usage": ".ddx/executions/20260429T235731-b7a9889b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8-20260429T235731-b7a9889b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T235731-b7a9889b/result.json b/.ddx/executions/20260429T235731-b7a9889b/result.json
new file mode 100644
index 00000000..ee24d5a6
--- /dev/null
+++ b/.ddx/executions/20260429T235731-b7a9889b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8",
+  "attempt_id": "20260429T235731-b7a9889b",
+  "base_rev": "c00f544b8c368a93fdb1f8422f4a9b3526df7364",
+  "result_rev": "e6c36568081c0cfdf62ec3305cf7bb8d0c6b1d3d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2d808633",
+  "duration_ms": 269485,
+  "tokens": 13421,
+  "cost_usd": 1.769932,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T235731-b7a9889b",
+  "prompt_file": ".ddx/executions/20260429T235731-b7a9889b/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T235731-b7a9889b/manifest.json",
+  "result_file": ".ddx/executions/20260429T235731-b7a9889b/result.json",
+  "usage_file": ".ddx/executions/20260429T235731-b7a9889b/usage.json",
+  "started_at": "2026-04-29T23:57:32.557717297Z",
+  "finished_at": "2026-04-30T00:02:02.04288113Z"
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
## Review: .execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-55635fd8 iter 1

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
