<bead-review>
  <bead id="ddx-ae1d617e" iter=1>
    <title>migrate execute_bead_land.go to git.Command</title>
    <description>
Split from timed-out ddx-65b88ae1. Migrate the 27 bare exec.Command("git"...) and exec.CommandContext(..."git"...) callsites in cli/internal/agent/execute_bead_land.go to use cli/internal/git.Command (the wrapper from ddx-aa8a5fb3). Each: replace 'exec.CommandContext(ctx, "git", args...)' with 'git.Command(ctx, dir, args...)' where dir is the cmd.Dir the original code set (or the -C target). Import path: "github.com/DocumentDrivenDX/ddx/internal/git".
    </description>
    <acceptance>
grep -n 'exec\.Command(\s*"git"\|exec\.CommandContext(.*"git"' cli/internal/agent/execute_bead_land.go returns no matches. All existing execute_bead_land_test.go tests still pass. Build green.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2db94b5a22745ddffccc974e1a4ba93768593a89">
commit 2db94b5a22745ddffccc974e1a4ba93768593a89
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 08:30:35 2026 -0400

    chore: add execution evidence [20260424T122420-]

diff --git a/.ddx/executions/20260424T122420-5485f224/manifest.json b/.ddx/executions/20260424T122420-5485f224/manifest.json
new file mode 100644
index 00000000..43b30447
--- /dev/null
+++ b/.ddx/executions/20260424T122420-5485f224/manifest.json
@@ -0,0 +1,32 @@
+{
+  "attempt_id": "20260424T122420-5485f224",
+  "bead_id": "ddx-ae1d617e",
+  "base_rev": "f863e352fb050d94efe9d42dec5839c866dbbc8e",
+  "created_at": "2026-04-24T12:24:20.748644765Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ae1d617e",
+    "title": "migrate execute_bead_land.go to git.Command",
+    "description": "Split from timed-out ddx-65b88ae1. Migrate the 27 bare exec.Command(\"git\"...) and exec.CommandContext(...\"git\"...) callsites in cli/internal/agent/execute_bead_land.go to use cli/internal/git.Command (the wrapper from ddx-aa8a5fb3). Each: replace 'exec.CommandContext(ctx, \"git\", args...)' with 'git.Command(ctx, dir, args...)' where dir is the cmd.Dir the original code set (or the -C target). Import path: \"github.com/DocumentDrivenDX/ddx/internal/git\".",
+    "acceptance": "grep -n 'exec\\.Command(\\s*\"git\"\\|exec\\.CommandContext(.*\"git\"' cli/internal/agent/execute_bead_land.go returns no matches. All existing execute_bead_land_test.go tests still pass. Build green.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "claimed-at": "2026-04-24T12:24:20Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-24T12:24:20.010661103Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T122420-5485f224",
+    "prompt": ".ddx/executions/20260424T122420-5485f224/prompt.md",
+    "manifest": ".ddx/executions/20260424T122420-5485f224/manifest.json",
+    "result": ".ddx/executions/20260424T122420-5485f224/result.json",
+    "checks": ".ddx/executions/20260424T122420-5485f224/checks.json",
+    "usage": ".ddx/executions/20260424T122420-5485f224/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ae1d617e-20260424T122420-5485f224"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T122420-5485f224/result.json b/.ddx/executions/20260424T122420-5485f224/result.json
new file mode 100644
index 00000000..417b6a41
--- /dev/null
+++ b/.ddx/executions/20260424T122420-5485f224/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ae1d617e",
+  "attempt_id": "20260424T122420-5485f224",
+  "base_rev": "f863e352fb050d94efe9d42dec5839c866dbbc8e",
+  "result_rev": "4a03f990bb6c0cfd1fd18f1fcfe45d66b023ca99",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3872f70d",
+  "duration_ms": 373206,
+  "tokens": 13831,
+  "cost_usd": 1.9082487499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T122420-5485f224",
+  "prompt_file": ".ddx/executions/20260424T122420-5485f224/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T122420-5485f224/manifest.json",
+  "result_file": ".ddx/executions/20260424T122420-5485f224/result.json",
+  "usage_file": ".ddx/executions/20260424T122420-5485f224/usage.json",
+  "started_at": "2026-04-24T12:24:20.748947765Z",
+  "finished_at": "2026-04-24T12:30:33.954976391Z"
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
## Review: ddx-ae1d617e iter 1

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
