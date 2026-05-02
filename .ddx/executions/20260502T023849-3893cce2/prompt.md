<bead-review>
  <bead id="ddx-ea855f63" iter=1>
    <title>Rewrite website/content/why/_index.md with one section per domain principle</title>
    <description>
Replace existing /why content with: software-factory framing intro (1 paragraph), then 10 sections (one per principle) each 50-80 words summarizing the principle and linking to the deep page at /docs/principles/&lt;slug&gt;/. Order matches the principles list. Cross-link to /docs/concepts/software-factory/ from the intro.
    </description>
    <acceptance>
1. /why/_index.md has 10 principle sections. 2. Each section &lt;100 words and includes a link to the corresponding /docs/principles/&lt;slug&gt;/ page. 3. Software-factory framing in intro paragraph with link. 4. 'grep -c "^## " website/content/why/_index.md' returns 10 (or 11 with intro). 5. cd website &amp;&amp; hugo builds.
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e1a0398b2b90d593a4583e5a9d4024164b117da7">
commit e1a0398b2b90d593a4583e5a9d4024164b117da7
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:38:47 2026 -0400

    chore: add execution evidence [20260502T023715-]

diff --git a/.ddx/executions/20260502T023715-f7f7b625/manifest.json b/.ddx/executions/20260502T023715-f7f7b625/manifest.json
new file mode 100644
index 00000000..6649ebf9
--- /dev/null
+++ b/.ddx/executions/20260502T023715-f7f7b625/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T023715-f7f7b625",
+  "bead_id": "ddx-ea855f63",
+  "base_rev": "27fd8d538a29d2f78288a02c798ccc518f873f51",
+  "created_at": "2026-05-02T02:37:16.958217854Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ea855f63",
+    "title": "Rewrite website/content/why/_index.md with one section per domain principle",
+    "description": "Replace existing /why content with: software-factory framing intro (1 paragraph), then 10 sections (one per principle) each 50-80 words summarizing the principle and linking to the deep page at /docs/principles/\u003cslug\u003e/. Order matches the principles list. Cross-link to /docs/concepts/software-factory/ from the intro.",
+    "acceptance": "1. /why/_index.md has 10 principle sections. 2. Each section \u003c100 words and includes a link to the corresponding /docs/principles/\u003cslug\u003e/ page. 3. Software-factory framing in intro paragraph with link. 4. 'grep -c \"^## \" website/content/why/_index.md' returns 10 (or 11 with intro). 5. cd website \u0026\u0026 hugo builds.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:37:15Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:37:15.424705004Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T023715-f7f7b625",
+    "prompt": ".ddx/executions/20260502T023715-f7f7b625/prompt.md",
+    "manifest": ".ddx/executions/20260502T023715-f7f7b625/manifest.json",
+    "result": ".ddx/executions/20260502T023715-f7f7b625/result.json",
+    "checks": ".ddx/executions/20260502T023715-f7f7b625/checks.json",
+    "usage": ".ddx/executions/20260502T023715-f7f7b625/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ea855f63-20260502T023715-f7f7b625"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T023715-f7f7b625/result.json b/.ddx/executions/20260502T023715-f7f7b625/result.json
new file mode 100644
index 00000000..f4d5dc20
--- /dev/null
+++ b/.ddx/executions/20260502T023715-f7f7b625/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ea855f63",
+  "attempt_id": "20260502T023715-f7f7b625",
+  "base_rev": "27fd8d538a29d2f78288a02c798ccc518f873f51",
+  "result_rev": "abdea152b59c6381eac1a4a1a99083acaa158309",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-8f502fda",
+  "duration_ms": 87543,
+  "tokens": 5082,
+  "cost_usd": 0.5637334999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T023715-f7f7b625",
+  "prompt_file": ".ddx/executions/20260502T023715-f7f7b625/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T023715-f7f7b625/manifest.json",
+  "result_file": ".ddx/executions/20260502T023715-f7f7b625/result.json",
+  "usage_file": ".ddx/executions/20260502T023715-f7f7b625/usage.json",
+  "started_at": "2026-05-02T02:37:16.958548352Z",
+  "finished_at": "2026-05-02T02:38:44.501549686Z"
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
## Review: ddx-ea855f63 iter 1

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
