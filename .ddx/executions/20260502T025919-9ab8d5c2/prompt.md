<bead-review>
  <bead id="ddx-768cf4ab" iter=1>
    <title>Update FEAT-003 (website) feature spec to reflect new IA</title>
    <description>
Update docs/helix/01-frame/features/FEAT-003-website.md to describe: new /why structure (10 principle sections), /docs/principles/&lt;slug&gt;/ deep page tree, /docs/concepts/software-factory.md, /docs/concepts/run-architecture.md, lineage section in vision, software-factory homepage sub-claim. Note that quorum is de-emphasized and Dun is removed from copy.
    </description>
    <acceptance>
1. FEAT-003 updated with new IA structure. 2. Lists all new top-level concept pages. 3. Notes the principle taxonomy (10 domain pages). 4. ddx doc audit passes on FEAT-003.
    </acceptance>
    <labels>site-redesign, area:specs, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f53202ded08fcbb20e46277ac80eb6ea2f888d9a">
commit f53202ded08fcbb20e46277ac80eb6ea2f888d9a
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:59:17 2026 -0400

    chore: add execution evidence [20260502T025743-]

diff --git a/.ddx/executions/20260502T025743-53077e7b/manifest.json b/.ddx/executions/20260502T025743-53077e7b/manifest.json
new file mode 100644
index 00000000..e12c1150
--- /dev/null
+++ b/.ddx/executions/20260502T025743-53077e7b/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T025743-53077e7b",
+  "bead_id": "ddx-768cf4ab",
+  "base_rev": "a5f9fa55b68e9063c8a9747f3d283654e0b34264",
+  "created_at": "2026-05-02T02:57:44.660051619Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-768cf4ab",
+    "title": "Update FEAT-003 (website) feature spec to reflect new IA",
+    "description": "Update docs/helix/01-frame/features/FEAT-003-website.md to describe: new /why structure (10 principle sections), /docs/principles/\u003cslug\u003e/ deep page tree, /docs/concepts/software-factory.md, /docs/concepts/run-architecture.md, lineage section in vision, software-factory homepage sub-claim. Note that quorum is de-emphasized and Dun is removed from copy.",
+    "acceptance": "1. FEAT-003 updated with new IA structure. 2. Lists all new top-level concept pages. 3. Notes the principle taxonomy (10 domain pages). 4. ddx doc audit passes on FEAT-003.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:57:43Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:57:43.104844238Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T025743-53077e7b",
+    "prompt": ".ddx/executions/20260502T025743-53077e7b/prompt.md",
+    "manifest": ".ddx/executions/20260502T025743-53077e7b/manifest.json",
+    "result": ".ddx/executions/20260502T025743-53077e7b/result.json",
+    "checks": ".ddx/executions/20260502T025743-53077e7b/checks.json",
+    "usage": ".ddx/executions/20260502T025743-53077e7b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-768cf4ab-20260502T025743-53077e7b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T025743-53077e7b/result.json b/.ddx/executions/20260502T025743-53077e7b/result.json
new file mode 100644
index 00000000..eaaec633
--- /dev/null
+++ b/.ddx/executions/20260502T025743-53077e7b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-768cf4ab",
+  "attempt_id": "20260502T025743-53077e7b",
+  "base_rev": "a5f9fa55b68e9063c8a9747f3d283654e0b34264",
+  "result_rev": "bc9687f6142447e6f2e1ae8b255dfd46a23e4438",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-bc04c5a8",
+  "duration_ms": 90597,
+  "tokens": 5541,
+  "cost_usd": 0.6390277499999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T025743-53077e7b",
+  "prompt_file": ".ddx/executions/20260502T025743-53077e7b/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T025743-53077e7b/manifest.json",
+  "result_file": ".ddx/executions/20260502T025743-53077e7b/result.json",
+  "usage_file": ".ddx/executions/20260502T025743-53077e7b/usage.json",
+  "started_at": "2026-05-02T02:57:44.660428368Z",
+  "finished_at": "2026-05-02T02:59:15.258066326Z"
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
## Review: ddx-768cf4ab iter 1

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
