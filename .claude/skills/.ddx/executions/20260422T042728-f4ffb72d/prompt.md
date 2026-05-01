<bead-review>
  <bead id="ddx-85dc2ab8" iter=1>
    <title>FEAT-008 test fixture fix: FILTER_BEADS needs P0 bead for default-sort test</title>
    <description>
US-082g.a in beads.spec.ts expects rows.first() to have data-priority=0 (or matches /[23]/), but FILTER_BEADS fixture contains priorities 1,2,3. Fix the fixture by adding a P0 bead OR adjusting the test expectation to match the fixture range. I wrote both so this is my bug. Prefer: add a P0 bead to FILTER_BEADS so default priority-asc sort surfaces it first.
    </description>
    <acceptance>
FILTER_BEADS includes at least one priority=0 bead; US-082g.a passes the default-sort assertion.
    </acceptance>
    <labels>ui, feat-008, test-fix</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="cc6136a906082fdb88c86bf8eca6c0693b7768c1">
commit cc6136a906082fdb88c86bf8eca6c0693b7768c1
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 00:27:26 2026 -0400

    chore: add execution evidence [20260422T041930-]

diff --git a/.ddx/executions/20260422T041930-8b64e9a1/manifest.json b/.ddx/executions/20260422T041930-8b64e9a1/manifest.json
new file mode 100644
index 00000000..6252dcc9
--- /dev/null
+++ b/.ddx/executions/20260422T041930-8b64e9a1/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260422T041930-8b64e9a1",
+  "bead_id": "ddx-85dc2ab8",
+  "base_rev": "d736ea1faa9c9f1e4fc4909cc15f1b65fe72fc6d",
+  "created_at": "2026-04-22T04:19:30.583716638Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-85dc2ab8",
+    "title": "FEAT-008 test fixture fix: FILTER_BEADS needs P0 bead for default-sort test",
+    "description": "US-082g.a in beads.spec.ts expects rows.first() to have data-priority=0 (or matches /[23]/), but FILTER_BEADS fixture contains priorities 1,2,3. Fix the fixture by adding a P0 bead OR adjusting the test expectation to match the fixture range. I wrote both so this is my bug. Prefer: add a P0 bead to FILTER_BEADS so default priority-asc sort surfaces it first.",
+    "acceptance": "FILTER_BEADS includes at least one priority=0 bead; US-082g.a passes the default-sort assertion.",
+    "parent": "ddx-4a9d30db",
+    "labels": [
+      "ui",
+      "feat-008",
+      "test-fix"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-22T04:19:30Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1682344",
+      "execute-loop-heartbeat-at": "2026-04-22T04:19:30.058087291Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260422T041930-8b64e9a1",
+    "prompt": ".ddx/executions/20260422T041930-8b64e9a1/prompt.md",
+    "manifest": ".ddx/executions/20260422T041930-8b64e9a1/manifest.json",
+    "result": ".ddx/executions/20260422T041930-8b64e9a1/result.json",
+    "checks": ".ddx/executions/20260422T041930-8b64e9a1/checks.json",
+    "usage": ".ddx/executions/20260422T041930-8b64e9a1/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-85dc2ab8-20260422T041930-8b64e9a1"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260422T041930-8b64e9a1/result.json b/.ddx/executions/20260422T041930-8b64e9a1/result.json
new file mode 100644
index 00000000..e4320d59
--- /dev/null
+++ b/.ddx/executions/20260422T041930-8b64e9a1/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-85dc2ab8",
+  "attempt_id": "20260422T041930-8b64e9a1",
+  "base_rev": "d736ea1faa9c9f1e4fc4909cc15f1b65fe72fc6d",
+  "result_rev": "912c627aed2383b06c47e5d27cf304987e938d4c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-9d39e9fc",
+  "duration_ms": 474834,
+  "tokens": 3548699,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T041930-8b64e9a1",
+  "prompt_file": ".ddx/executions/20260422T041930-8b64e9a1/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T041930-8b64e9a1/manifest.json",
+  "result_file": ".ddx/executions/20260422T041930-8b64e9a1/result.json",
+  "usage_file": ".ddx/executions/20260422T041930-8b64e9a1/usage.json",
+  "started_at": "2026-04-22T04:19:30.583995179Z",
+  "finished_at": "2026-04-22T04:27:25.418084501Z"
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
## Review: ddx-85dc2ab8 iter 1

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
