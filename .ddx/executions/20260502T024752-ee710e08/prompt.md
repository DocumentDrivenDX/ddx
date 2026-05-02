<bead-review>
  <bead id="ddx-8a530449" iter=1>
    <title>Curated CLI noun-verb reference; preserve auto-generated full ref</title>
    <description>
Create website/content/docs/cli/_index.md (or equivalent) as a hand-curated noun-verb command reference matching CLAUDE.md's CLI Command Overview section. Keep auto-generated cli/commands/* full reference accessible via a 'Complete reference' link. Document in front matter or comment that this curated index is hand-maintained — regen process should not overwrite it.
    </description>
    <acceptance>
1. Curated CLI reference page exists with sections for Core, Bead Tracker, Queue Work, Agent Service, Resource Commands, Embedded Utilities (matching CLAUDE.md). 2. Page links to 'Complete command reference' that points at the cli/commands/ auto-generated tree. 3. Comment or front matter explicitly notes 'hand-curated; do not auto-regenerate'. 4. Run 'grep -c "^## " website/content/docs/cli/_index.md' returns &gt;= 6 (one per section).
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c0ab686975a6d95da8a628c0d440429a32099fa8">
commit c0ab686975a6d95da8a628c0d440429a32099fa8
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:47:49 2026 -0400

    chore: add execution evidence [20260502T024645-]

diff --git a/.ddx/executions/20260502T024645-49aba732/manifest.json b/.ddx/executions/20260502T024645-49aba732/manifest.json
new file mode 100644
index 00000000..dd780795
--- /dev/null
+++ b/.ddx/executions/20260502T024645-49aba732/manifest.json
@@ -0,0 +1,71 @@
+{
+  "attempt_id": "20260502T024645-49aba732",
+  "bead_id": "ddx-8a530449",
+  "base_rev": "fca68a6cfbd29acbc7082768d5ca5fa6d0b6856a",
+  "created_at": "2026-05-02T02:46:46.95364936Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-8a530449",
+    "title": "Curated CLI noun-verb reference; preserve auto-generated full ref",
+    "description": "Create website/content/docs/cli/_index.md (or equivalent) as a hand-curated noun-verb command reference matching CLAUDE.md's CLI Command Overview section. Keep auto-generated cli/commands/* full reference accessible via a 'Complete reference' link. Document in front matter or comment that this curated index is hand-maintained — regen process should not overwrite it.",
+    "acceptance": "1. Curated CLI reference page exists with sections for Core, Bead Tracker, Queue Work, Agent Service, Resource Commands, Embedded Utilities (matching CLAUDE.md). 2. Page links to 'Complete command reference' that points at the cli/commands/ auto-generated tree. 3. Comment or front matter explicitly notes 'hand-curated; do not auto-regenerate'. 4. Run 'grep -c \"^## \" website/content/docs/cli/_index.md' returns \u003e= 6 (one per section).",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:46:45Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:53.711883097Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:53.843830236Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-05-02T01:36:53.95776177Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-05-02T01:36:54.265853163Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T02:46:45.405496119Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T024645-49aba732",
+    "prompt": ".ddx/executions/20260502T024645-49aba732/prompt.md",
+    "manifest": ".ddx/executions/20260502T024645-49aba732/manifest.json",
+    "result": ".ddx/executions/20260502T024645-49aba732/result.json",
+    "checks": ".ddx/executions/20260502T024645-49aba732/checks.json",
+    "usage": ".ddx/executions/20260502T024645-49aba732/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-8a530449-20260502T024645-49aba732"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T024645-49aba732/result.json b/.ddx/executions/20260502T024645-49aba732/result.json
new file mode 100644
index 00000000..a568cf33
--- /dev/null
+++ b/.ddx/executions/20260502T024645-49aba732/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-8a530449",
+  "attempt_id": "20260502T024645-49aba732",
+  "base_rev": "fca68a6cfbd29acbc7082768d5ca5fa6d0b6856a",
+  "result_rev": "286585443c1ae8c6513e621051b3b702300ff2b1",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c9478a9c",
+  "duration_ms": 59830,
+  "tokens": 3828,
+  "cost_usd": 0.365548,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T024645-49aba732",
+  "prompt_file": ".ddx/executions/20260502T024645-49aba732/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T024645-49aba732/manifest.json",
+  "result_file": ".ddx/executions/20260502T024645-49aba732/result.json",
+  "usage_file": ".ddx/executions/20260502T024645-49aba732/usage.json",
+  "started_at": "2026-05-02T02:46:46.953971693Z",
+  "finished_at": "2026-05-02T02:47:46.784271346Z"
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
## Review: ddx-8a530449 iter 1

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
