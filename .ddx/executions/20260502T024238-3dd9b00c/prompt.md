<bead-review>
  <bead id="ddx-99a16fbd" iter=1>
    <title>Add Lineage section to product-vision.md citing software-factory + spec-driven REF artifacts</title>
    <description>
Add a 'Lineage' section near the top of docs/helix/00-discover/product-vision.md citing the three traditions DDx draws from: 1) software factories (REF-007 Greenfield/Short Microsoft 2004), 2) spec-driven development (REF-005 spec-kit, REF-006 Kiro), 3) bounded-context agent execution (REF-008 Lost in Middle, REF-009 Chroma context rot). Use ddx.depends_on to register the link in the artifact graph.
    </description>
    <acceptance>
1. product-vision.md has a 'Lineage' section. 2. References REF-005, REF-006, REF-007, REF-008, REF-009 inline as links or citations. 3. ddx.depends_on includes those REF IDs. 4. ddx doc audit shows the new edges.
    </acceptance>
    <labels>site-redesign, area:specs, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="8301655c0a4d6acc3f5d5f04631ffb8166b979ef">
commit 8301655c0a4d6acc3f5d5f04631ffb8166b979ef
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:42:36 2026 -0400

    chore: add execution evidence [20260502T023933-]

diff --git a/.ddx/executions/20260502T023933-3b830549/manifest.json b/.ddx/executions/20260502T023933-3b830549/manifest.json
new file mode 100644
index 00000000..1d0d192e
--- /dev/null
+++ b/.ddx/executions/20260502T023933-3b830549/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T023933-3b830549",
+  "bead_id": "ddx-99a16fbd",
+  "base_rev": "a760cc3bb41d47743d33c7dcb62311d6fd3ce0b7",
+  "created_at": "2026-05-02T02:39:34.691976063Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-99a16fbd",
+    "title": "Add Lineage section to product-vision.md citing software-factory + spec-driven REF artifacts",
+    "description": "Add a 'Lineage' section near the top of docs/helix/00-discover/product-vision.md citing the three traditions DDx draws from: 1) software factories (REF-007 Greenfield/Short Microsoft 2004), 2) spec-driven development (REF-005 spec-kit, REF-006 Kiro), 3) bounded-context agent execution (REF-008 Lost in Middle, REF-009 Chroma context rot). Use ddx.depends_on to register the link in the artifact graph.",
+    "acceptance": "1. product-vision.md has a 'Lineage' section. 2. References REF-005, REF-006, REF-007, REF-008, REF-009 inline as links or citations. 3. ddx.depends_on includes those REF IDs. 4. ddx doc audit shows the new edges.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:39:33Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:39:33.103795365Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T023933-3b830549",
+    "prompt": ".ddx/executions/20260502T023933-3b830549/prompt.md",
+    "manifest": ".ddx/executions/20260502T023933-3b830549/manifest.json",
+    "result": ".ddx/executions/20260502T023933-3b830549/result.json",
+    "checks": ".ddx/executions/20260502T023933-3b830549/checks.json",
+    "usage": ".ddx/executions/20260502T023933-3b830549/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-99a16fbd-20260502T023933-3b830549"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T023933-3b830549/result.json b/.ddx/executions/20260502T023933-3b830549/result.json
new file mode 100644
index 00000000..1fe6d6e7
--- /dev/null
+++ b/.ddx/executions/20260502T023933-3b830549/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-99a16fbd",
+  "attempt_id": "20260502T023933-3b830549",
+  "base_rev": "a760cc3bb41d47743d33c7dcb62311d6fd3ce0b7",
+  "result_rev": "a23975c9f1042f769c91544c12d3df6b7d0feda9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-daf2d33e",
+  "duration_ms": 178508,
+  "tokens": 12132,
+  "cost_usd": 1.5794210000000004,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T023933-3b830549",
+  "prompt_file": ".ddx/executions/20260502T023933-3b830549/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T023933-3b830549/manifest.json",
+  "result_file": ".ddx/executions/20260502T023933-3b830549/result.json",
+  "usage_file": ".ddx/executions/20260502T023933-3b830549/usage.json",
+  "started_at": "2026-05-02T02:39:34.692289687Z",
+  "finished_at": "2026-05-02T02:42:33.200553213Z"
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
## Review: ddx-99a16fbd iter 1

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
