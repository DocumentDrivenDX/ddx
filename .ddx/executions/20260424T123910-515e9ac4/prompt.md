<bead-review>
  <bead id="ddx-217666e0" iter=1>
    <title>migrate execute_bead.go to git.Command</title>
    <description>
Split from timed-out ddx-65b88ae1. Migrate the 21 bare git exec callsites in cli/internal/agent/execute_bead.go to cli/internal/git.Command. Replace 'exec.CommandContext(ctx, "git", args...)' (and any 'exec.Command("git", args...)') with 'git.Command(ctx, dir, args...)' where dir is the original cmd.Dir (or -C target). Import "github.com/DocumentDrivenDX/ddx/internal/git".
    </description>
    <acceptance>
grep on execute_bead.go returns no bare git exec.Command matches. All existing *_test.go files in cli/internal/agent/ pass. Build green.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="052899271636d80767bea22810921ac514a3b727">
commit 052899271636d80767bea22810921ac514a3b727
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 08:39:08 2026 -0400

    chore: add execution evidence [20260424T123118-]

diff --git a/.ddx/executions/20260424T123118-4cf0c28e/manifest.json b/.ddx/executions/20260424T123118-4cf0c28e/manifest.json
new file mode 100644
index 00000000..6b746f9d
--- /dev/null
+++ b/.ddx/executions/20260424T123118-4cf0c28e/manifest.json
@@ -0,0 +1,32 @@
+{
+  "attempt_id": "20260424T123118-4cf0c28e",
+  "bead_id": "ddx-217666e0",
+  "base_rev": "8f2c8deb31196543ab9ffa9f7fb970667a5244db",
+  "created_at": "2026-04-24T12:31:19.049335572Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-217666e0",
+    "title": "migrate execute_bead.go to git.Command",
+    "description": "Split from timed-out ddx-65b88ae1. Migrate the 21 bare git exec callsites in cli/internal/agent/execute_bead.go to cli/internal/git.Command. Replace 'exec.CommandContext(ctx, \"git\", args...)' (and any 'exec.Command(\"git\", args...)') with 'git.Command(ctx, dir, args...)' where dir is the original cmd.Dir (or -C target). Import \"github.com/DocumentDrivenDX/ddx/internal/git\".",
+    "acceptance": "grep on execute_bead.go returns no bare git exec.Command matches. All existing *_test.go files in cli/internal/agent/ pass. Build green.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "claimed-at": "2026-04-24T12:31:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-24T12:31:18.484665532Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T123118-4cf0c28e",
+    "prompt": ".ddx/executions/20260424T123118-4cf0c28e/prompt.md",
+    "manifest": ".ddx/executions/20260424T123118-4cf0c28e/manifest.json",
+    "result": ".ddx/executions/20260424T123118-4cf0c28e/result.json",
+    "checks": ".ddx/executions/20260424T123118-4cf0c28e/checks.json",
+    "usage": ".ddx/executions/20260424T123118-4cf0c28e/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-217666e0-20260424T123118-4cf0c28e"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T123118-4cf0c28e/result.json b/.ddx/executions/20260424T123118-4cf0c28e/result.json
new file mode 100644
index 00000000..c6302691
--- /dev/null
+++ b/.ddx/executions/20260424T123118-4cf0c28e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-217666e0",
+  "attempt_id": "20260424T123118-4cf0c28e",
+  "base_rev": "8f2c8deb31196543ab9ffa9f7fb970667a5244db",
+  "result_rev": "1ee82603f525848cc7a748bcd4833cad5a0cc74c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2af6eaca",
+  "duration_ms": 468282,
+  "tokens": 11649,
+  "cost_usd": 1.9185605,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T123118-4cf0c28e",
+  "prompt_file": ".ddx/executions/20260424T123118-4cf0c28e/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T123118-4cf0c28e/manifest.json",
+  "result_file": ".ddx/executions/20260424T123118-4cf0c28e/result.json",
+  "usage_file": ".ddx/executions/20260424T123118-4cf0c28e/usage.json",
+  "started_at": "2026-04-24T12:31:19.04964278Z",
+  "finished_at": "2026-04-24T12:39:07.332295892Z"
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
## Review: ddx-217666e0 iter 1

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
