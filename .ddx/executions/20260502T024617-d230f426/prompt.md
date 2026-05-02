<bead-review>
  <bead id="ddx-c14000c5" iter=1>
    <title>Update homepage with software-factory sub-claim and concept-page link</title>
    <description>
Update website/content/_index.md (and/or website/layouts/index.html as applicable) to add the sub-claim 'A document-driven software factory.' under the existing hero. Link to /docs/concepts/software-factory/.
    </description>
    <acceptance>
1. Homepage hero gains 'A document-driven software factory.' sub-claim. 2. Sub-claim is a link to /docs/concepts/software-factory/. 3. cd website &amp;&amp; hugo builds without errors. 4. 'rg -n "document-driven software factory" website/content/_index.md website/layouts/index.html' returns &gt;= 1 match.
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="abc76d673284ed9d1e8cb631bc45ea1763f79a55">
commit abc76d673284ed9d1e8cb631bc45ea1763f79a55
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:46:14 2026 -0400

    chore: add execution evidence [20260502T024539-]

diff --git a/.ddx/executions/20260502T024539-a2b91ff8/manifest.json b/.ddx/executions/20260502T024539-a2b91ff8/manifest.json
new file mode 100644
index 00000000..325247e6
--- /dev/null
+++ b/.ddx/executions/20260502T024539-a2b91ff8/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T024539-a2b91ff8",
+  "bead_id": "ddx-c14000c5",
+  "base_rev": "14d0575ab942bfc83a2b2fde639999830b8589b9",
+  "created_at": "2026-05-02T02:45:40.772252638Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c14000c5",
+    "title": "Update homepage with software-factory sub-claim and concept-page link",
+    "description": "Update website/content/_index.md (and/or website/layouts/index.html as applicable) to add the sub-claim 'A document-driven software factory.' under the existing hero. Link to /docs/concepts/software-factory/.",
+    "acceptance": "1. Homepage hero gains 'A document-driven software factory.' sub-claim. 2. Sub-claim is a link to /docs/concepts/software-factory/. 3. cd website \u0026\u0026 hugo builds without errors. 4. 'rg -n \"document-driven software factory\" website/content/_index.md website/layouts/index.html' returns \u003e= 1 match.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:45:39Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:45:39.240644214Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T024539-a2b91ff8",
+    "prompt": ".ddx/executions/20260502T024539-a2b91ff8/prompt.md",
+    "manifest": ".ddx/executions/20260502T024539-a2b91ff8/manifest.json",
+    "result": ".ddx/executions/20260502T024539-a2b91ff8/result.json",
+    "checks": ".ddx/executions/20260502T024539-a2b91ff8/checks.json",
+    "usage": ".ddx/executions/20260502T024539-a2b91ff8/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c14000c5-20260502T024539-a2b91ff8"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T024539-a2b91ff8/result.json b/.ddx/executions/20260502T024539-a2b91ff8/result.json
new file mode 100644
index 00000000..846b5a5d
--- /dev/null
+++ b/.ddx/executions/20260502T024539-a2b91ff8/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c14000c5",
+  "attempt_id": "20260502T024539-a2b91ff8",
+  "base_rev": "14d0575ab942bfc83a2b2fde639999830b8589b9",
+  "result_rev": "c88cdbccdee9faa22aa5b1027c2da834a40f9e24",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-cf9626ff",
+  "duration_ms": 30980,
+  "tokens": 1512,
+  "cost_usd": 0.2805895,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T024539-a2b91ff8",
+  "prompt_file": ".ddx/executions/20260502T024539-a2b91ff8/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T024539-a2b91ff8/manifest.json",
+  "result_file": ".ddx/executions/20260502T024539-a2b91ff8/result.json",
+  "usage_file": ".ddx/executions/20260502T024539-a2b91ff8/usage.json",
+  "started_at": "2026-05-02T02:45:40.772552429Z",
+  "finished_at": "2026-05-02T02:46:11.752889528Z"
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
## Review: ddx-c14000c5 iter 1

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
