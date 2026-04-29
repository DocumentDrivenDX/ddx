<bead-review>
  <bead id=".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915" iter=1>
    <title>[read-coverage] add GET /api/docs/changed REST route</title>
    <description>
FEAT-007 gap. MCP has ddx_doc_changed (list artifacts changed since a git ref) but no equivalent HTTP REST route. REST clients (non-MCP) cannot query changed artifacts. Add GET /api/docs/changed?since=&lt;ref&gt; route mirroring the MCP tool. Low complexity add alongside other FEAT-007 server work.
    </description>
    <acceptance/>
    <labels>read-coverage, server, http, feat-007</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d6040a9e1d266452896e10ab6ce81e800f503841">
commit d6040a9e1d266452896e10ab6ce81e800f503841
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 19:51:54 2026 -0400

    chore: add execution evidence [20260429T234819-]

diff --git a/.ddx/executions/20260429T234819-f22bdc06/manifest.json b/.ddx/executions/20260429T234819-f22bdc06/manifest.json
new file mode 100644
index 00000000..7c4d26df
--- /dev/null
+++ b/.ddx/executions/20260429T234819-f22bdc06/manifest.json
@@ -0,0 +1,118 @@
+{
+  "attempt_id": "20260429T234819-f22bdc06",
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915",
+  "base_rev": "524965756f3a10cd42694d9b94fc876c57630db6",
+  "created_at": "2026-04-29T23:48:20.197529572Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915",
+    "title": "[read-coverage] add GET /api/docs/changed REST route",
+    "description": "FEAT-007 gap. MCP has ddx_doc_changed (list artifacts changed since a git ref) but no equivalent HTTP REST route. REST clients (non-MCP) cannot query changed artifacts. Add GET /api/docs/changed?since=\u003cref\u003e route mirroring the MCP tool. Low complexity add alongside other FEAT-007 server work.",
+    "labels": [
+      "read-coverage",
+      "server",
+      "http",
+      "feat-007"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T23:48:19Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-29T20:15:13.46340575Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260429T200946-5369ed03\",\"harness\":\"claude\",\"model\":\"sonnet\",\"input_tokens\":28,\"output_tokens\":7308,\"total_tokens\":7336,\"cost_usd\":0.6200976499999998,\"duration_ms\":326537,\"exit_code\":0}",
+          "created_at": "2026-04-29T20:15:13.574937593Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=7336 cost_usd=0.6201 model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"sonnet\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-29T20:15:17.388122016Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "Diff contains only execution evidence files (manifest.json, result.json) — no actual implementation of the GET /api/docs/changed route. Cannot evaluate AC against absent code changes.\nharness=claude\nmodel=opus\ninput_bytes=5650\noutput_bytes=553\nelapsed_ms=5304",
+          "created_at": "2026-04-29T20:15:24.893550735Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-29T20:15:25.000790414Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff contains only execution evidence files (manifest.json, result.json) — no actual implementation of the GET /api/docs/changed route. Cannot evaluate AC against absent code changes.\nresult_rev=8330801598f7a46f45abc8b556d33f19f58059cf\nbase_rev=407fa6f4cde287bb7e16ba73973f9bc203e46624",
+          "created_at": "2026-04-29T20:15:25.093468602Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:28:53.463186082Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-29T23:28:53.583874205Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-29T23:28:53.669656697Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-29T23:28:53.842938556Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-29T23:48:19.24572886Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T234819-f22bdc06",
+    "prompt": ".ddx/executions/20260429T234819-f22bdc06/prompt.md",
+    "manifest": ".ddx/executions/20260429T234819-f22bdc06/manifest.json",
+    "result": ".ddx/executions/20260429T234819-f22bdc06/result.json",
+    "checks": ".ddx/executions/20260429T234819-f22bdc06/checks.json",
+    "usage": ".ddx/executions/20260429T234819-f22bdc06/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915-20260429T234819-f22bdc06"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T234819-f22bdc06/result.json b/.ddx/executions/20260429T234819-f22bdc06/result.json
new file mode 100644
index 00000000..2ebd8e82
--- /dev/null
+++ b/.ddx/executions/20260429T234819-f22bdc06/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915",
+  "attempt_id": "20260429T234819-f22bdc06",
+  "base_rev": "524965756f3a10cd42694d9b94fc876c57630db6",
+  "result_rev": "d1b467ff62e4b9915fc2b104e674e5f7dffc3f4f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-22b25a80",
+  "duration_ms": 212346,
+  "tokens": 7028,
+  "cost_usd": 0.8398350000000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T234819-f22bdc06",
+  "prompt_file": ".ddx/executions/20260429T234819-f22bdc06/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T234819-f22bdc06/manifest.json",
+  "result_file": ".ddx/executions/20260429T234819-f22bdc06/result.json",
+  "usage_file": ".ddx/executions/20260429T234819-f22bdc06/usage.json",
+  "started_at": "2026-04-29T23:48:20.197990117Z",
+  "finished_at": "2026-04-29T23:51:52.544762185Z"
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
## Review: .execute-bead-wt-ddx-44236615-20260429T185004-ffa9adf1-d37e1915 iter 1

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
