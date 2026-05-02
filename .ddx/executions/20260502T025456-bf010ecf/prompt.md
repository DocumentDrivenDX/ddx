<bead-review>
  <bead id="ddx-b3319694" iter=1>
    <title>Replace website/content/docs/concepts/principles.md with redirect to new principles index</title>
    <description>
The existing concepts/principles page carries old/internal principles. Replace with a thin index that redirects or links to the new /docs/principles/&lt;slug&gt;/ pages. Either delete and add nav redirect, or keep as a brief umbrella page describing the 10 principles with links to deep pages.
    </description>
    <acceptance>
1. website/content/docs/concepts/principles.md no longer carries the 9-principle outdated list. 2. Either redirects to /docs/principles/ (umbrella) or deleted with nav update. 3. cd website &amp;&amp; hugo builds without 404 warnings on principle internal links.
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5304b680376e71a1b4fe8f50d75b73d5d68f5b9e">
commit 5304b680376e71a1b4fe8f50d75b73d5d68f5b9e
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:54:54 2026 -0400

    chore: add execution evidence [20260502T025340-]

diff --git a/.ddx/executions/20260502T025340-195666bf/manifest.json b/.ddx/executions/20260502T025340-195666bf/manifest.json
new file mode 100644
index 00000000..11e29340
--- /dev/null
+++ b/.ddx/executions/20260502T025340-195666bf/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T025340-195666bf",
+  "bead_id": "ddx-b3319694",
+  "base_rev": "1c162d0ad0bc97ffa540d5133be594518bd3eb55",
+  "created_at": "2026-05-02T02:53:41.474376804Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b3319694",
+    "title": "Replace website/content/docs/concepts/principles.md with redirect to new principles index",
+    "description": "The existing concepts/principles page carries old/internal principles. Replace with a thin index that redirects or links to the new /docs/principles/\u003cslug\u003e/ pages. Either delete and add nav redirect, or keep as a brief umbrella page describing the 10 principles with links to deep pages.",
+    "acceptance": "1. website/content/docs/concepts/principles.md no longer carries the 9-principle outdated list. 2. Either redirects to /docs/principles/ (umbrella) or deleted with nav update. 3. cd website \u0026\u0026 hugo builds without 404 warnings on principle internal links.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:53:39Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:53:39.959393622Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T025340-195666bf",
+    "prompt": ".ddx/executions/20260502T025340-195666bf/prompt.md",
+    "manifest": ".ddx/executions/20260502T025340-195666bf/manifest.json",
+    "result": ".ddx/executions/20260502T025340-195666bf/result.json",
+    "checks": ".ddx/executions/20260502T025340-195666bf/checks.json",
+    "usage": ".ddx/executions/20260502T025340-195666bf/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b3319694-20260502T025340-195666bf"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T025340-195666bf/result.json b/.ddx/executions/20260502T025340-195666bf/result.json
new file mode 100644
index 00000000..a06c8c09
--- /dev/null
+++ b/.ddx/executions/20260502T025340-195666bf/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b3319694",
+  "attempt_id": "20260502T025340-195666bf",
+  "base_rev": "1c162d0ad0bc97ffa540d5133be594518bd3eb55",
+  "result_rev": "8448c93cb8781da604faabbfaa2808f38ce626ed",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-5288823a",
+  "duration_ms": 69675,
+  "tokens": 3958,
+  "cost_usd": 0.5498205,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T025340-195666bf",
+  "prompt_file": ".ddx/executions/20260502T025340-195666bf/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T025340-195666bf/manifest.json",
+  "result_file": ".ddx/executions/20260502T025340-195666bf/result.json",
+  "usage_file": ".ddx/executions/20260502T025340-195666bf/usage.json",
+  "started_at": "2026-05-02T02:53:41.474731679Z",
+  "finished_at": "2026-05-02T02:54:51.150137521Z"
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
## Review: ddx-b3319694 iter 1

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
