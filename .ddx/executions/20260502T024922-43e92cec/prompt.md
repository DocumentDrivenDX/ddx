<bead-review>
  <bead id="ddx-1a5e8786" iter=1>
    <title>Surface ddx sync (FEAT-023) and node/dashboard (FEAT-020/021) on site</title>
    <description>
Add 'ddx sync' coverage to relevant concept page (probably concepts/architecture.md or new section). Add multi-node dashboard / node identity to server section.
    </description>
    <acceptance>
1. Site mentions ddx sync at least once in primary docs (not just CLI ref) with brief description. 2. Server section references FEAT-020 (node identity) and FEAT-021 (dashboard). 3. Run 'rg -n "ddx sync" website/content/docs/' returns &gt;= 2 matches.
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e019310ede52c1d3e7603a09dff75785789b7d9a">
commit e019310ede52c1d3e7603a09dff75785789b7d9a
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:49:20 2026 -0400

    chore: add execution evidence [20260502T024811-]

diff --git a/.ddx/executions/20260502T024811-31e300ab/manifest.json b/.ddx/executions/20260502T024811-31e300ab/manifest.json
new file mode 100644
index 00000000..f22a3ee6
--- /dev/null
+++ b/.ddx/executions/20260502T024811-31e300ab/manifest.json
@@ -0,0 +1,71 @@
+{
+  "attempt_id": "20260502T024811-31e300ab",
+  "bead_id": "ddx-1a5e8786",
+  "base_rev": "ac1383980b19d6d6c739db91b9458122ba399d75",
+  "created_at": "2026-05-02T02:48:12.932866676Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-1a5e8786",
+    "title": "Surface ddx sync (FEAT-023) and node/dashboard (FEAT-020/021) on site",
+    "description": "Add 'ddx sync' coverage to relevant concept page (probably concepts/architecture.md or new section). Add multi-node dashboard / node identity to server section.",
+    "acceptance": "1. Site mentions ddx sync at least once in primary docs (not just CLI ref) with brief description. 2. Server section references FEAT-020 (node identity) and FEAT-021 (dashboard). 3. Run 'rg -n \"ddx sync\" website/content/docs/' returns \u003e= 2 matches.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:48:11Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:56.67239654Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:56.811332087Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-05-02T01:36:56.920025253Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-05-02T01:36:57.139340748Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T02:48:11.348164498Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T024811-31e300ab",
+    "prompt": ".ddx/executions/20260502T024811-31e300ab/prompt.md",
+    "manifest": ".ddx/executions/20260502T024811-31e300ab/manifest.json",
+    "result": ".ddx/executions/20260502T024811-31e300ab/result.json",
+    "checks": ".ddx/executions/20260502T024811-31e300ab/checks.json",
+    "usage": ".ddx/executions/20260502T024811-31e300ab/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-1a5e8786-20260502T024811-31e300ab"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T024811-31e300ab/result.json b/.ddx/executions/20260502T024811-31e300ab/result.json
new file mode 100644
index 00000000..04ba2110
--- /dev/null
+++ b/.ddx/executions/20260502T024811-31e300ab/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-1a5e8786",
+  "attempt_id": "20260502T024811-31e300ab",
+  "base_rev": "ac1383980b19d6d6c739db91b9458122ba399d75",
+  "result_rev": "d13b2433af22aa70cc4f2e1b59ec42a1df078a4f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-850e8aee",
+  "duration_ms": 64684,
+  "tokens": 4091,
+  "cost_usd": 0.559462,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T024811-31e300ab",
+  "prompt_file": ".ddx/executions/20260502T024811-31e300ab/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T024811-31e300ab/manifest.json",
+  "result_file": ".ddx/executions/20260502T024811-31e300ab/result.json",
+  "usage_file": ".ddx/executions/20260502T024811-31e300ab/usage.json",
+  "started_at": "2026-05-02T02:48:12.933244134Z",
+  "finished_at": "2026-05-02T02:49:17.618055655Z"
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
## Review: ddx-1a5e8786 iter 1

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
