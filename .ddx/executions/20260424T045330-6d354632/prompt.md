<bead-review>
  <bead id="ddx-ca1f533d" iter=1>
    <title>test: migrate TestExecuteBeadAgentErrorNoCommits to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadAgentErrorNoCommits() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadAgentErrorNoCommits$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadAgentErrorNoCommits no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadAgentErrorNoCommits$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5fd583139687d48234a708e4961c262e554128a4">
commit 5fd583139687d48234a708e4961c262e554128a4
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 00:53:28 2026 -0400

    chore: add execution evidence [20260424T044932-]

diff --git a/.ddx/executions/20260424T044932-91e76e8d/manifest.json b/.ddx/executions/20260424T044932-91e76e8d/manifest.json
new file mode 100644
index 00000000..facf58fe
--- /dev/null
+++ b/.ddx/executions/20260424T044932-91e76e8d/manifest.json
@@ -0,0 +1,77 @@
+{
+  "attempt_id": "20260424T044932-91e76e8d",
+  "bead_id": "ddx-ca1f533d",
+  "base_rev": "e39e89e5ef6adde01bdd5b2adda3316a1cea5293",
+  "created_at": "2026-04-24T04:49:33.242183511Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ca1f533d",
+    "title": "test: migrate TestExecuteBeadAgentErrorNoCommits to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadAgentErrorNoCommits() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadAgentErrorNoCommits$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadAgentErrorNoCommits no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadAgentErrorNoCommits$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T04:49:32Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"vidar-coder\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:32:06.825958241Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=vidar-coder model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nagent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway ",
+          "created_at": "2026-04-21T17:32:07.041585225Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":15023}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:32:07.10115361Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "infrastructure failure (deferred): agent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway \ntier=cheap\nprobe_result=ok\nresult_rev=a474ac05ad0859ef0878a4917e76aede9171eb60\nbase_rev=a474ac05ad0859ef0878a4917e76aede9171eb60\nretry_after=2026-04-21T23:32:07Z",
+          "created_at": "2026-04-21T17:32:07.26689053Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T04:49:32.714715114Z",
+      "execute-loop-last-detail": "infrastructure failure (deferred): agent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway ",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-21T23:32:07Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T044932-91e76e8d",
+    "prompt": ".ddx/executions/20260424T044932-91e76e8d/prompt.md",
+    "manifest": ".ddx/executions/20260424T044932-91e76e8d/manifest.json",
+    "result": ".ddx/executions/20260424T044932-91e76e8d/result.json",
+    "checks": ".ddx/executions/20260424T044932-91e76e8d/checks.json",
+    "usage": ".ddx/executions/20260424T044932-91e76e8d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ca1f533d-20260424T044932-91e76e8d"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T044932-91e76e8d/result.json b/.ddx/executions/20260424T044932-91e76e8d/result.json
new file mode 100644
index 00000000..a810baf2
--- /dev/null
+++ b/.ddx/executions/20260424T044932-91e76e8d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ca1f533d",
+  "attempt_id": "20260424T044932-91e76e8d",
+  "base_rev": "e39e89e5ef6adde01bdd5b2adda3316a1cea5293",
+  "result_rev": "759796bd2cadcc04ae50ca1f52e1bc9456a0cec9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d4372a49",
+  "duration_ms": 234215,
+  "tokens": 10945,
+  "cost_usd": 1.5424725,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T044932-91e76e8d",
+  "prompt_file": ".ddx/executions/20260424T044932-91e76e8d/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T044932-91e76e8d/manifest.json",
+  "result_file": ".ddx/executions/20260424T044932-91e76e8d/result.json",
+  "usage_file": ".ddx/executions/20260424T044932-91e76e8d/usage.json",
+  "started_at": "2026-04-24T04:49:33.242505886Z",
+  "finished_at": "2026-04-24T04:53:27.457618398Z"
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
## Review: ddx-ca1f533d iter 1

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
