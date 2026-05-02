<bead-review>
  <bead id="ddx-9c9a2eed" iter=1>
    <title>De-emphasize quorum review: remove from features-list prominence</title>
    <description>
Per locked decision, quorum review is just a skill — don't promote to primary capability. Remove from features-list prominence on website/content/features/_index.md and website/content/_index.md. May be mentioned in skills documentation only.
    </description>
    <acceptance>
1. quorum review removed from primary features list on features/_index.md. 2. Homepage hero/features grid no longer features quorum review. 3. Run 'rg -n -i "quorum" website/content/_index.md website/content/features/_index.md' returns 0 matches (or only in deprecated/skills context).
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="49a98ad359f0ffc1944ed784bbd84615f94743c9">
commit 49a98ad359f0ffc1944ed784bbd84615f94743c9
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:53:20 2026 -0400

    chore: add execution evidence [20260502T025228-]

diff --git a/.ddx/executions/20260502T025228-60a31752/manifest.json b/.ddx/executions/20260502T025228-60a31752/manifest.json
new file mode 100644
index 00000000..ff478bdf
--- /dev/null
+++ b/.ddx/executions/20260502T025228-60a31752/manifest.json
@@ -0,0 +1,71 @@
+{
+  "attempt_id": "20260502T025228-60a31752",
+  "bead_id": "ddx-9c9a2eed",
+  "base_rev": "ddec65272acdd4b3624c8119ecbf2c9921b6599b",
+  "created_at": "2026-05-02T02:52:30.187805364Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9c9a2eed",
+    "title": "De-emphasize quorum review: remove from features-list prominence",
+    "description": "Per locked decision, quorum review is just a skill — don't promote to primary capability. Remove from features-list prominence on website/content/features/_index.md and website/content/_index.md. May be mentioned in skills documentation only.",
+    "acceptance": "1. quorum review removed from primary features list on features/_index.md. 2. Homepage hero/features grid no longer features quorum review. 3. Run 'rg -n -i \"quorum\" website/content/_index.md website/content/features/_index.md' returns 0 matches (or only in deprecated/skills context).",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:52:28Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:37:02.416978839Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:37:02.529355542Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-05-02T01:37:02.642567327Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-05-02T01:37:02.86428282Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T02:52:28.69457603Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T025228-60a31752",
+    "prompt": ".ddx/executions/20260502T025228-60a31752/prompt.md",
+    "manifest": ".ddx/executions/20260502T025228-60a31752/manifest.json",
+    "result": ".ddx/executions/20260502T025228-60a31752/result.json",
+    "checks": ".ddx/executions/20260502T025228-60a31752/checks.json",
+    "usage": ".ddx/executions/20260502T025228-60a31752/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9c9a2eed-20260502T025228-60a31752"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T025228-60a31752/result.json b/.ddx/executions/20260502T025228-60a31752/result.json
new file mode 100644
index 00000000..25ad1f09
--- /dev/null
+++ b/.ddx/executions/20260502T025228-60a31752/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9c9a2eed",
+  "attempt_id": "20260502T025228-60a31752",
+  "base_rev": "ddec65272acdd4b3624c8119ecbf2c9921b6599b",
+  "result_rev": "3d96ee2261533f12a0f1ca27a25c46b7118fbe3c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-1234d2ba",
+  "duration_ms": 47345,
+  "tokens": 1773,
+  "cost_usd": 0.32906425,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T025228-60a31752",
+  "prompt_file": ".ddx/executions/20260502T025228-60a31752/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T025228-60a31752/manifest.json",
+  "result_file": ".ddx/executions/20260502T025228-60a31752/result.json",
+  "usage_file": ".ddx/executions/20260502T025228-60a31752/usage.json",
+  "started_at": "2026-05-02T02:52:30.188168072Z",
+  "finished_at": "2026-05-02T02:53:17.53374352Z"
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
## Review: ddx-9c9a2eed iter 1

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
