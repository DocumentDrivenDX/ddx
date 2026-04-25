<bead-review>
  <bead id="ddx-46370e6b" iter=1>
    <title>regression test: execute-bead against GIT_DIR-contaminated env leaves outer bare repo untouched</title>
    <description>
Split from timed-out ddx-65b88ae1. Write the regression test that proves the B4+B5[a-c] migrations actually stop Bug A.

Test sketch (Go test in cli/internal/agent/):
1. Create a fake outer bare repo at /tmp/&lt;testname&gt;/bare with 'git init --bare' and capture a copy of its config file bytes.
2. Set GIT_DIR=&lt;bare&gt;/.git, GIT_WORK_TREE=&lt;fake-work&gt;, GIT_INDEX_FILE=&lt;fake-index&gt; in the test env via t.Setenv.
3. Create a separate working repo in a tempdir.
4. Construct a minimal Runner + agent.Script-based no-op harness.
5. Drive an in-process execute-bead end-to-end hitting WorktreeAdd, SynthesizeCommit, CommitTracker, and the landing/preserve-refs path (use --no-merge preserve to exercise update-ref in the base repo without requiring a remote).
6. Assert the fake bare repo's config bytes are byte-identical after the run. Assert no stray files appeared under &lt;bare&gt;/.git.

This is the provable gate: if Bug A is truly fixed, polluted env does not mutate the outer repo.
    </description>
    <acceptance>
Go test exists, passes, and would fail if any git call-site in the agent path reverted to bare exec.Command("git", ...). Test runs in under 60s.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="931f931c6a6196af45b25f56273f30afa7945200">
commit 931f931c6a6196af45b25f56273f30afa7945200
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 03:59:32 2026 -0400

    chore: add execution evidence [20260425T074650-]

diff --git a/.ddx/executions/20260425T074650-016a9bfb/manifest.json b/.ddx/executions/20260425T074650-016a9bfb/manifest.json
new file mode 100644
index 00000000..ee06c13c
--- /dev/null
+++ b/.ddx/executions/20260425T074650-016a9bfb/manifest.json
@@ -0,0 +1,66 @@
+{
+  "attempt_id": "20260425T074650-016a9bfb",
+  "bead_id": "ddx-46370e6b",
+  "base_rev": "05d78016d67b8f2e47b7b20f88a83b7cf312dc98",
+  "created_at": "2026-04-25T07:46:50.676721188Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-46370e6b",
+    "title": "regression test: execute-bead against GIT_DIR-contaminated env leaves outer bare repo untouched",
+    "description": "Split from timed-out ddx-65b88ae1. Write the regression test that proves the B4+B5[a-c] migrations actually stop Bug A.\n\nTest sketch (Go test in cli/internal/agent/):\n1. Create a fake outer bare repo at /tmp/\u003ctestname\u003e/bare with 'git init --bare' and capture a copy of its config file bytes.\n2. Set GIT_DIR=\u003cbare\u003e/.git, GIT_WORK_TREE=\u003cfake-work\u003e, GIT_INDEX_FILE=\u003cfake-index\u003e in the test env via t.Setenv.\n3. Create a separate working repo in a tempdir.\n4. Construct a minimal Runner + agent.Script-based no-op harness.\n5. Drive an in-process execute-bead end-to-end hitting WorktreeAdd, SynthesizeCommit, CommitTracker, and the landing/preserve-refs path (use --no-merge preserve to exercise update-ref in the base repo without requiring a remote).\n6. Assert the fake bare repo's config bytes are byte-identical after the run. Assert no stray files appeared under \u003cbare\u003e/.git.\n\nThis is the provable gate: if Bug A is truly fixed, polluted env does not mutate the outer repo.",
+    "acceptance": "Go test exists, passes, and would fail if any git call-site in the agent path reverted to bare exec.Command(\"git\", ...). Test runs in under 60s.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "claimed-at": "2026-04-25T07:46:50Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:20.542244839Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:20.614584754Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-25T02:23:20.682769211Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-25T02:23:20.815904207Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-25T07:46:50.127639194Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260425T074650-016a9bfb",
+    "prompt": ".ddx/executions/20260425T074650-016a9bfb/prompt.md",
+    "manifest": ".ddx/executions/20260425T074650-016a9bfb/manifest.json",
+    "result": ".ddx/executions/20260425T074650-016a9bfb/result.json",
+    "checks": ".ddx/executions/20260425T074650-016a9bfb/checks.json",
+    "usage": ".ddx/executions/20260425T074650-016a9bfb/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-46370e6b-20260425T074650-016a9bfb"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T074650-016a9bfb/result.json b/.ddx/executions/20260425T074650-016a9bfb/result.json
new file mode 100644
index 00000000..ccbaad3d
--- /dev/null
+++ b/.ddx/executions/20260425T074650-016a9bfb/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-46370e6b",
+  "attempt_id": "20260425T074650-016a9bfb",
+  "base_rev": "05d78016d67b8f2e47b7b20f88a83b7cf312dc98",
+  "result_rev": "25b7c244ebf3e714ead8b577bfda5126416b680f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-f99106c8",
+  "duration_ms": 760295,
+  "tokens": 19820,
+  "cost_usd": 3.6300465000000006,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T074650-016a9bfb",
+  "prompt_file": ".ddx/executions/20260425T074650-016a9bfb/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T074650-016a9bfb/manifest.json",
+  "result_file": ".ddx/executions/20260425T074650-016a9bfb/result.json",
+  "usage_file": ".ddx/executions/20260425T074650-016a9bfb/usage.json",
+  "started_at": "2026-04-25T07:46:50.676991854Z",
+  "finished_at": "2026-04-25T07:59:30.972336106Z"
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
## Review: ddx-46370e6b iter 1

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
