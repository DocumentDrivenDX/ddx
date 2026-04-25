<bead-review>
  <bead id="ddx-1c46a4f8" iter=1>
    <title>migrate compare_adapter.go, remaining agent files, and cmd/install.go to git.Command; delete cleanGitEnv helper</title>
    <description>
Split from timed-out ddx-65b88ae1. Migrate the remaining 13 git callsites (in compare_adapter.go=6, execute_bead_review.go=2, execute_bead_orchestrator.go=1, script.go=1, cmd/install.go:commitPluginChanges=3) to cli/internal/git.Command.

Additionally: delete the now-redundant cleanGitEnv() helper in cli/internal/agent/compare_adapter.go and its callers. Replace 'cmd.Env = cleanGitEnv()' with 'cmd.Env = git.CleanEnv()' then remove the function definition and its strings/os imports if they become unused.
    </description>
    <acceptance>
grep returns no bare git exec.Command matches in compare_adapter.go, execute_bead_review.go, execute_bead_orchestrator.go, script.go, or cmd/install.go. cleanGitEnv function removed from compare_adapter.go. Build + tests green.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5f7c29eb64faaea2a047c12b8aae5f6a7a4e49ab">
commit 5f7c29eb64faaea2a047c12b8aae5f6a7a4e49ab
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 03:46:17 2026 -0400

    chore: add execution evidence [20260425T073655-]

diff --git a/.ddx/executions/20260425T073655-6b55991a/manifest.json b/.ddx/executions/20260425T073655-6b55991a/manifest.json
new file mode 100644
index 00000000..51a7d0ac
--- /dev/null
+++ b/.ddx/executions/20260425T073655-6b55991a/manifest.json
@@ -0,0 +1,98 @@
+{
+  "attempt_id": "20260425T073655-6b55991a",
+  "bead_id": "ddx-1c46a4f8",
+  "base_rev": "92f17b95dc00901944689b8420bc9c8d29124637",
+  "created_at": "2026-04-25T07:36:55.824698428Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-1c46a4f8",
+    "title": "migrate compare_adapter.go, remaining agent files, and cmd/install.go to git.Command; delete cleanGitEnv helper",
+    "description": "Split from timed-out ddx-65b88ae1. Migrate the remaining 13 git callsites (in compare_adapter.go=6, execute_bead_review.go=2, execute_bead_orchestrator.go=1, script.go=1, cmd/install.go:commitPluginChanges=3) to cli/internal/git.Command.\n\nAdditionally: delete the now-redundant cleanGitEnv() helper in cli/internal/agent/compare_adapter.go and its callers. Replace 'cmd.Env = cleanGitEnv()' with 'cmd.Env = git.CleanEnv()' then remove the function definition and its strings/os imports if they become unused.",
+    "acceptance": "grep returns no bare git exec.Command matches in compare_adapter.go, execute_bead_review.go, execute_bead_orchestrator.go, script.go, or cmd/install.go. cleanGitEnv function removed from compare_adapter.go. Build + tests green.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "claimed-at": "2026-04-25T07:36:55Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx-watchdog",
+          "body": "worker=worker-20260423T234349-9c40 runtime=13h57m15s stalled=1h0m49s pid=0 reason=watchdog",
+          "created_at": "2026-04-24T13:41:04.125216703Z",
+          "kind": "bead.reaped",
+          "source": "server-workers",
+          "summary": "watchdog"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-24T13:41:04.318479758Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260424T124014-1bbc9f57\",\"harness\":\"claude\",\"input_tokens\":63,\"output_tokens\":16024,\"total_tokens\":16087,\"cost_usd\":3.500647,\"duration_ms\":3649067,\"exit_code\":1}",
+          "created_at": "2026-04-24T13:41:04.388205816Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=16087 cost_usd=3.5006"
+        },
+        {
+          "actor": "ddx",
+          "body": "cancelled\nresult_rev=49ebc653014bbe3ebff4cff7d0e293d467dac58d\nbase_rev=96acf2de3775b9ff9df8322e9a7b600f91afb6ba",
+          "created_at": "2026-04-24T13:41:04.852450023Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:17.931767032Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:18.008146905Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-25T02:23:18.077936028Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-25T02:23:18.221128233Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-25T07:36:55.339718686Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260425T073655-6b55991a",
+    "prompt": ".ddx/executions/20260425T073655-6b55991a/prompt.md",
+    "manifest": ".ddx/executions/20260425T073655-6b55991a/manifest.json",
+    "result": ".ddx/executions/20260425T073655-6b55991a/result.json",
+    "checks": ".ddx/executions/20260425T073655-6b55991a/checks.json",
+    "usage": ".ddx/executions/20260425T073655-6b55991a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-1c46a4f8-20260425T073655-6b55991a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T073655-6b55991a/result.json b/.ddx/executions/20260425T073655-6b55991a/result.json
new file mode 100644
index 00000000..c37b2a8e
--- /dev/null
+++ b/.ddx/executions/20260425T073655-6b55991a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-1c46a4f8",
+  "attempt_id": "20260425T073655-6b55991a",
+  "base_rev": "92f17b95dc00901944689b8420bc9c8d29124637",
+  "result_rev": "405af8b9875e40c7c519a73f259c6d169aaeb3e1",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-e916e5b5",
+  "duration_ms": 560225,
+  "tokens": 15176,
+  "cost_usd": 2.86026975,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T073655-6b55991a",
+  "prompt_file": ".ddx/executions/20260425T073655-6b55991a/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T073655-6b55991a/manifest.json",
+  "result_file": ".ddx/executions/20260425T073655-6b55991a/result.json",
+  "usage_file": ".ddx/executions/20260425T073655-6b55991a/usage.json",
+  "started_at": "2026-04-25T07:36:55.825011969Z",
+  "finished_at": "2026-04-25T07:46:16.0510088Z"
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
## Review: ddx-1c46a4f8 iter 1

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
