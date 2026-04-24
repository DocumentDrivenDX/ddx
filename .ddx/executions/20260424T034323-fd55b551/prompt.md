<bead-review>
  <bead id="ddx-3984a48e" iter=1>
    <title>migrate remaining cli/cmd/* to git.Command (Phase 2b)</title>
    <description>
Migrate remaining cli/cmd/ git callsites to git.Command. Excludes install.go which is in B5.

Files: agent_cmd.go, agent_workers.go, checkpoint.go, status.go, bead.go, doc.go, bead_review.go, log.go.

Each: replace exec.Command("git", args...) with git.Command(ctx, dir, args...). Pass appropriate dir (cmd.Dir value, or '-C &lt;path&gt;' arg target).
    </description>
    <acceptance>
No bare exec.Command("git"...) in cli/cmd/ except in test helpers. All existing cli/cmd/ tests pass.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c9bdf7823a76efb8d97a660bf83ceca4d756dfa4">
commit c9bdf7823a76efb8d97a660bf83ceca4d756dfa4
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 23:43:21 2026 -0400

    chore: add execution evidence [20260424T033612-]

diff --git a/.ddx/executions/20260424T033612-690dec51/manifest.json b/.ddx/executions/20260424T033612-690dec51/manifest.json
new file mode 100644
index 00000000..7c6a80cd
--- /dev/null
+++ b/.ddx/executions/20260424T033612-690dec51/manifest.json
@@ -0,0 +1,33 @@
+{
+  "attempt_id": "20260424T033612-690dec51",
+  "bead_id": "ddx-3984a48e",
+  "base_rev": "73d93746bb31d18103b0905c99ae6f7ffc274b8e",
+  "created_at": "2026-04-24T03:36:13.175011967Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3984a48e",
+    "title": "migrate remaining cli/cmd/* to git.Command (Phase 2b)",
+    "description": "Migrate remaining cli/cmd/ git callsites to git.Command. Excludes install.go which is in B5.\n\nFiles: agent_cmd.go, agent_workers.go, checkpoint.go, status.go, bead.go, doc.go, bead_review.go, log.go.\n\nEach: replace exec.Command(\"git\", args...) with git.Command(ctx, dir, args...). Pass appropriate dir (cmd.Dir value, or '-C \u003cpath\u003e' arg target).",
+    "acceptance": "No bare exec.Command(\"git\"...) in cli/cmd/ except in test helpers. All existing cli/cmd/ tests pass.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "blocked-by": "ddx-aa8a5fb3",
+      "claimed-at": "2026-04-24T03:36:12Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-24T03:36:12.663424968Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T033612-690dec51",
+    "prompt": ".ddx/executions/20260424T033612-690dec51/prompt.md",
+    "manifest": ".ddx/executions/20260424T033612-690dec51/manifest.json",
+    "result": ".ddx/executions/20260424T033612-690dec51/result.json",
+    "checks": ".ddx/executions/20260424T033612-690dec51/checks.json",
+    "usage": ".ddx/executions/20260424T033612-690dec51/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3984a48e-20260424T033612-690dec51"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T033612-690dec51/result.json b/.ddx/executions/20260424T033612-690dec51/result.json
new file mode 100644
index 00000000..ee5edf5b
--- /dev/null
+++ b/.ddx/executions/20260424T033612-690dec51/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-3984a48e",
+  "attempt_id": "20260424T033612-690dec51",
+  "base_rev": "73d93746bb31d18103b0905c99ae6f7ffc274b8e",
+  "result_rev": "8481ade68f980fb9e49498ffb432f3cbfdb78aa6",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a7ed9b52",
+  "duration_ms": 427720,
+  "tokens": 22959,
+  "cost_usd": 3.933810749999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T033612-690dec51",
+  "prompt_file": ".ddx/executions/20260424T033612-690dec51/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T033612-690dec51/manifest.json",
+  "result_file": ".ddx/executions/20260424T033612-690dec51/result.json",
+  "usage_file": ".ddx/executions/20260424T033612-690dec51/usage.json",
+  "started_at": "2026-04-24T03:36:13.175269175Z",
+  "finished_at": "2026-04-24T03:43:20.896187982Z"
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
## Review: ddx-3984a48e iter 1

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
