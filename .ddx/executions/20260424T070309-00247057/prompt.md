<bead-review>
  <bead id="ddx-6f876b36" iter=1>
    <title>feat(doctor): detect+fix core.bare and stray core.worktree (Phase 3)</title>
    <description>
ddx doctor needs to detect existing repo corruption from prior incidents and offer auto-fix.

Add checks to cli/cmd/doctor.go (or wherever doctor lives):
1. core.bare = true on a repo with a working tree (not actually bare) — flag + offer 'git config --unset core.bare'
2. Stray core.worktree entries that don't match the actual worktree path — flag + offer 'git config --unset core.worktree'
3. extensions.worktreeConfig missing → warning (not fail), with note that it prevents per-worktree config corruption
    </description>
    <acceptance>
Test: deliberately corrupt a test repo with 'git config core.bare true', run doctor, assert it's detected. With --fix flag (or interactive), assert the bad config gets removed. Doctor still passes on a clean repo.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="365b26ac6bdb751d8e24e4b31f0ca95c6bd9d0fc">
commit 365b26ac6bdb751d8e24e4b31f0ca95c6bd9d0fc
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 03:03:07 2026 -0400

    chore: add execution evidence [20260424T065743-]

diff --git a/.ddx/executions/20260424T065743-2ad6ca89/manifest.json b/.ddx/executions/20260424T065743-2ad6ca89/manifest.json
new file mode 100644
index 00000000..86139f67
--- /dev/null
+++ b/.ddx/executions/20260424T065743-2ad6ca89/manifest.json
@@ -0,0 +1,32 @@
+{
+  "attempt_id": "20260424T065743-2ad6ca89",
+  "bead_id": "ddx-6f876b36",
+  "base_rev": "b264b257c09baee6775497386b37482d5531eb62",
+  "created_at": "2026-04-24T06:57:44.252066454Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-6f876b36",
+    "title": "feat(doctor): detect+fix core.bare and stray core.worktree (Phase 3)",
+    "description": "ddx doctor needs to detect existing repo corruption from prior incidents and offer auto-fix.\n\nAdd checks to cli/cmd/doctor.go (or wherever doctor lives):\n1. core.bare = true on a repo with a working tree (not actually bare) — flag + offer 'git config --unset core.bare'\n2. Stray core.worktree entries that don't match the actual worktree path — flag + offer 'git config --unset core.worktree'\n3. extensions.worktreeConfig missing → warning (not fail), with note that it prevents per-worktree config corruption",
+    "acceptance": "Test: deliberately corrupt a test repo with 'git config core.bare true', run doctor, assert it's detected. With --fix flag (or interactive), assert the bad config gets removed. Doctor still passes on a clean repo.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "claimed-at": "2026-04-24T06:57:43Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-24T06:57:43.736910693Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T065743-2ad6ca89",
+    "prompt": ".ddx/executions/20260424T065743-2ad6ca89/prompt.md",
+    "manifest": ".ddx/executions/20260424T065743-2ad6ca89/manifest.json",
+    "result": ".ddx/executions/20260424T065743-2ad6ca89/result.json",
+    "checks": ".ddx/executions/20260424T065743-2ad6ca89/checks.json",
+    "usage": ".ddx/executions/20260424T065743-2ad6ca89/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-6f876b36-20260424T065743-2ad6ca89"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T065743-2ad6ca89/result.json b/.ddx/executions/20260424T065743-2ad6ca89/result.json
new file mode 100644
index 00000000..21562308
--- /dev/null
+++ b/.ddx/executions/20260424T065743-2ad6ca89/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-6f876b36",
+  "attempt_id": "20260424T065743-2ad6ca89",
+  "base_rev": "b264b257c09baee6775497386b37482d5531eb62",
+  "result_rev": "fd1fd281c08b92459da1ca8d0a4f3e52e262f0be",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-ed953598",
+  "duration_ms": 321915,
+  "tokens": 16632,
+  "cost_usd": 1.8053800000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T065743-2ad6ca89",
+  "prompt_file": ".ddx/executions/20260424T065743-2ad6ca89/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T065743-2ad6ca89/manifest.json",
+  "result_file": ".ddx/executions/20260424T065743-2ad6ca89/result.json",
+  "usage_file": ".ddx/executions/20260424T065743-2ad6ca89/usage.json",
+  "started_at": "2026-04-24T06:57:44.252358495Z",
+  "finished_at": "2026-04-24T07:03:06.168208204Z"
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
## Review: ddx-6f876b36 iter 1

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
