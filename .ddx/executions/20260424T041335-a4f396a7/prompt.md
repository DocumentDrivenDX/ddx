<bead-review>
  <bead id="ddx-feb60c1d" iter=1>
    <title>test: migrate TestExecuteBeadDirtyWorktreeWithoutCommits to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadDirtyWorktreeWithoutCommits() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadDirtyWorktreeWithoutCommits$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadDirtyWorktreeWithoutCommits no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadDirtyWorktreeWithoutCommits$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="6e781c71de230eff6f646431e80f8b25bc52995f">
commit 6e781c71de230eff6f646431e80f8b25bc52995f
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 00:13:33 2026 -0400

    chore: add execution evidence [20260424T041035-]

diff --git a/.ddx/executions/20260424T041035-60c9c64a/manifest.json b/.ddx/executions/20260424T041035-60c9c64a/manifest.json
new file mode 100644
index 00000000..5e9bb432
--- /dev/null
+++ b/.ddx/executions/20260424T041035-60c9c64a/manifest.json
@@ -0,0 +1,77 @@
+{
+  "attempt_id": "20260424T041035-60c9c64a",
+  "bead_id": "ddx-feb60c1d",
+  "base_rev": "504ba54ba2dabce3e2e90f1ef441d3a731fc28da",
+  "created_at": "2026-04-24T04:10:36.056550067Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-feb60c1d",
+    "title": "test: migrate TestExecuteBeadDirtyWorktreeWithoutCommits to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadDirtyWorktreeWithoutCommits() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadDirtyWorktreeWithoutCommits$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadDirtyWorktreeWithoutCommits no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadDirtyWorktreeWithoutCommits$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T04:10:35Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"vidar-coder\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:29:27.437507471Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=vidar-coder model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nagent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway ",
+          "created_at": "2026-04-21T17:29:27.631267728Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":15019}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:29:27.689063519Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "infrastructure failure (deferred): agent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway \ntier=cheap\nprobe_result=ok\nresult_rev=9872f832ad1308d30f06f0b77d77f4e9940afa32\nbase_rev=9872f832ad1308d30f06f0b77d77f4e9940afa32\nretry_after=2026-04-21T23:29:27Z",
+          "created_at": "2026-04-21T17:29:27.846614496Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T04:10:35.414571856Z",
+      "execute-loop-last-detail": "infrastructure failure (deferred): agent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway ",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-21T23:29:27Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T041035-60c9c64a",
+    "prompt": ".ddx/executions/20260424T041035-60c9c64a/prompt.md",
+    "manifest": ".ddx/executions/20260424T041035-60c9c64a/manifest.json",
+    "result": ".ddx/executions/20260424T041035-60c9c64a/result.json",
+    "checks": ".ddx/executions/20260424T041035-60c9c64a/checks.json",
+    "usage": ".ddx/executions/20260424T041035-60c9c64a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-feb60c1d-20260424T041035-60c9c64a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T041035-60c9c64a/result.json b/.ddx/executions/20260424T041035-60c9c64a/result.json
new file mode 100644
index 00000000..1d9c2d73
--- /dev/null
+++ b/.ddx/executions/20260424T041035-60c9c64a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-feb60c1d",
+  "attempt_id": "20260424T041035-60c9c64a",
+  "base_rev": "504ba54ba2dabce3e2e90f1ef441d3a731fc28da",
+  "result_rev": "b4937fbc120087e05d4d05246db3701d53f7bd75",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3a9c09b2",
+  "duration_ms": 176037,
+  "tokens": 6630,
+  "cost_usd": 0.8061395,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T041035-60c9c64a",
+  "prompt_file": ".ddx/executions/20260424T041035-60c9c64a/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T041035-60c9c64a/manifest.json",
+  "result_file": ".ddx/executions/20260424T041035-60c9c64a/result.json",
+  "usage_file": ".ddx/executions/20260424T041035-60c9c64a/usage.json",
+  "started_at": "2026-04-24T04:10:36.057068441Z",
+  "finished_at": "2026-04-24T04:13:32.094992597Z"
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
## Review: ddx-feb60c1d iter 1

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
