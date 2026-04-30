<bead-review>
  <bead id="ddx-b711930f" iter=1>
    <title>[artifact-run-arch] ship replay-bead skill</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. New skill at skills/ddx/replay-bead/. Re-run a bead with altered conditions, baseline diff. Replaces agent replay CLI.
    </description>
    <acceptance/>
    <labels>skill, plan-2026-04-29, library</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3f378dcd72a9b7d4b3d84ec036e1f086a4384cfd">
commit 3f378dcd72a9b7d4b3d84ec036e1f086a4384cfd
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 21:26:28 2026 -0400

    chore: add execution evidence [20260430T012354-]

diff --git a/.ddx/executions/20260430T012354-a08c45bc/manifest.json b/.ddx/executions/20260430T012354-a08c45bc/manifest.json
new file mode 100644
index 00000000..0d127d77
--- /dev/null
+++ b/.ddx/executions/20260430T012354-a08c45bc/manifest.json
@@ -0,0 +1,35 @@
+{
+  "attempt_id": "20260430T012354-a08c45bc",
+  "bead_id": "ddx-b711930f",
+  "base_rev": "640625caf0b68ee2d8d60b5737896c77c1445b60",
+  "created_at": "2026-04-30T01:23:55.763746964Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b711930f",
+    "title": "[artifact-run-arch] ship replay-bead skill",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md. New skill at skills/ddx/replay-bead/. Re-run a bead with altered conditions, baseline diff. Replaces agent replay CLI.",
+    "labels": [
+      "skill",
+      "plan-2026-04-29",
+      "library"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-30T01:23:54Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "execute-loop-heartbeat-at": "2026-04-30T01:23:54.95865204Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260430T012354-a08c45bc",
+    "prompt": ".ddx/executions/20260430T012354-a08c45bc/prompt.md",
+    "manifest": ".ddx/executions/20260430T012354-a08c45bc/manifest.json",
+    "result": ".ddx/executions/20260430T012354-a08c45bc/result.json",
+    "checks": ".ddx/executions/20260430T012354-a08c45bc/checks.json",
+    "usage": ".ddx/executions/20260430T012354-a08c45bc/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b711930f-20260430T012354-a08c45bc"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260430T012354-a08c45bc/result.json b/.ddx/executions/20260430T012354-a08c45bc/result.json
new file mode 100644
index 00000000..397491f1
--- /dev/null
+++ b/.ddx/executions/20260430T012354-a08c45bc/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b711930f",
+  "attempt_id": "20260430T012354-a08c45bc",
+  "base_rev": "640625caf0b68ee2d8d60b5737896c77c1445b60",
+  "result_rev": "a8e23991650a93b6ff6d71f2993f71a1fc204e9b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-667185c5",
+  "duration_ms": 151256,
+  "tokens": 8552,
+  "cost_usd": 0.92314625,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260430T012354-a08c45bc",
+  "prompt_file": ".ddx/executions/20260430T012354-a08c45bc/prompt.md",
+  "manifest_file": ".ddx/executions/20260430T012354-a08c45bc/manifest.json",
+  "result_file": ".ddx/executions/20260430T012354-a08c45bc/result.json",
+  "usage_file": ".ddx/executions/20260430T012354-a08c45bc/usage.json",
+  "started_at": "2026-04-30T01:23:55.764007589Z",
+  "finished_at": "2026-04-30T01:26:27.020106612Z"
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
## Review: ddx-b711930f iter 1

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
