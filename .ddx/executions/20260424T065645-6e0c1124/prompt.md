<bead-review>
  <bead id="ddx-d3119355" iter=1>
    <title>migrate read-only callsites + delete duplicate helpers (Phase 2c)</title>
    <description>
Migrate the read-only/server git callsites to git.Command, and remove the now-redundant duplicate scrub helpers.

Read-only callsites:
- cli/internal/processmetrics/processmetrics.go
- cli/internal/server/server.go (3647, 3701, 3750, 3762)
- cli/internal/server/graphql/resolver_commits.go
- cli/internal/server/state.go

Delete duplicate helpers (now redundant with git.CleanEnv):
- gitEnvForDir() in cli/cmd/init.go (move callers to git.CleanEnv if any)
- scrubbedGitEnv() in cli/internal/server/server.go (replace callers with git.CleanEnv)
    </description>
    <acceptance>
No bare exec.Command("git"...) anywhere in non-test code. gitEnvForDir and scrubbedGitEnv functions removed. All tests pass.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="758068eaecc6354352197a93f26fa0297de4f530">
commit 758068eaecc6354352197a93f26fa0297de4f530
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 02:56:43 2026 -0400

    chore: add execution evidence [20260424T064858-]

diff --git a/.ddx/executions/20260424T064858-c781c184/manifest.json b/.ddx/executions/20260424T064858-c781c184/manifest.json
new file mode 100644
index 00000000..8d156228
--- /dev/null
+++ b/.ddx/executions/20260424T064858-c781c184/manifest.json
@@ -0,0 +1,33 @@
+{
+  "attempt_id": "20260424T064858-c781c184",
+  "bead_id": "ddx-d3119355",
+  "base_rev": "a196b40f161ebeb579e050f196d60d38ede90747",
+  "created_at": "2026-04-24T06:48:59.26334337Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d3119355",
+    "title": "migrate read-only callsites + delete duplicate helpers (Phase 2c)",
+    "description": "Migrate the read-only/server git callsites to git.Command, and remove the now-redundant duplicate scrub helpers.\n\nRead-only callsites:\n- cli/internal/processmetrics/processmetrics.go\n- cli/internal/server/server.go (3647, 3701, 3750, 3762)\n- cli/internal/server/graphql/resolver_commits.go\n- cli/internal/server/state.go\n\nDelete duplicate helpers (now redundant with git.CleanEnv):\n- gitEnvForDir() in cli/cmd/init.go (move callers to git.CleanEnv if any)\n- scrubbedGitEnv() in cli/internal/server/server.go (replace callers with git.CleanEnv)",
+    "acceptance": "No bare exec.Command(\"git\"...) anywhere in non-test code. gitEnvForDir and scrubbedGitEnv functions removed. All tests pass.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "blocked-by": "ddx-aa8a5fb3",
+      "claimed-at": "2026-04-24T06:48:58Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-24T06:48:58.7211581Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T064858-c781c184",
+    "prompt": ".ddx/executions/20260424T064858-c781c184/prompt.md",
+    "manifest": ".ddx/executions/20260424T064858-c781c184/manifest.json",
+    "result": ".ddx/executions/20260424T064858-c781c184/result.json",
+    "checks": ".ddx/executions/20260424T064858-c781c184/checks.json",
+    "usage": ".ddx/executions/20260424T064858-c781c184/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d3119355-20260424T064858-c781c184"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T064858-c781c184/result.json b/.ddx/executions/20260424T064858-c781c184/result.json
new file mode 100644
index 00000000..0b3a4679
--- /dev/null
+++ b/.ddx/executions/20260424T064858-c781c184/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d3119355",
+  "attempt_id": "20260424T064858-c781c184",
+  "base_rev": "a196b40f161ebeb579e050f196d60d38ede90747",
+  "result_rev": "30025549ddb81108050e24cf4a0c136ce575d241",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-be77920c",
+  "duration_ms": 463689,
+  "tokens": 20255,
+  "cost_usd": 4.18126775,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T064858-c781c184",
+  "prompt_file": ".ddx/executions/20260424T064858-c781c184/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T064858-c781c184/manifest.json",
+  "result_file": ".ddx/executions/20260424T064858-c781c184/result.json",
+  "usage_file": ".ddx/executions/20260424T064858-c781c184/usage.json",
+  "started_at": "2026-04-24T06:48:59.263629662Z",
+  "finished_at": "2026-04-24T06:56:42.953275986Z"
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
## Review: ddx-d3119355 iter 1

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
