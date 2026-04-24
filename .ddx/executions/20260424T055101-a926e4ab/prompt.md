<bead-review>
  <bead id="ddx-de9d564f" iter=1>
    <title>test: migrate TestExecuteBeadGatePass to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadGatePass() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadGatePass$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadGatePass no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadGatePass$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="94eb91ce013aafe2d62e5f6691ba6e1acb202ff7">
commit 94eb91ce013aafe2d62e5f6691ba6e1acb202ff7
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 01:50:59 2026 -0400

    chore: add execution evidence [20260424T054715-]

diff --git a/.ddx/executions/20260424T054715-d8d97a0e/manifest.json b/.ddx/executions/20260424T054715-d8d97a0e/manifest.json
new file mode 100644
index 00000000..7588623f
--- /dev/null
+++ b/.ddx/executions/20260424T054715-d8d97a0e/manifest.json
@@ -0,0 +1,77 @@
+{
+  "attempt_id": "20260424T054715-d8d97a0e",
+  "bead_id": "ddx-de9d564f",
+  "base_rev": "b90c14dd574853cf3cbed92fe5d4d2dd0ba1e08a",
+  "created_at": "2026-04-24T05:47:16.572102646Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-de9d564f",
+    "title": "test: migrate TestExecuteBeadGatePass to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadGatePass() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadGatePass$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadGatePass no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadGatePass$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T05:47:15Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"vidar-coder\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:38:51.37376384Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=vidar-coder model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nagent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway ",
+          "created_at": "2026-04-21T17:38:51.583213727Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":15022}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:38:51.647298764Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "infrastructure failure (deferred): agent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway \ntier=cheap\nprobe_result=ok\nresult_rev=1d7593fa35c58260bf08f26ec4b017a4c160cf6b\nbase_rev=1d7593fa35c58260bf08f26ec4b017a4c160cf6b\nretry_after=2026-04-21T23:38:51Z",
+          "created_at": "2026-04-21T17:38:51.835143376Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T05:47:15.724366875Z",
+      "execute-loop-last-detail": "infrastructure failure (deferred): agent: provider error: openai: POST \"http://vidar:1234/v1/chat/completions\": 502 Bad Gateway ",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-21T23:38:51Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T054715-d8d97a0e",
+    "prompt": ".ddx/executions/20260424T054715-d8d97a0e/prompt.md",
+    "manifest": ".ddx/executions/20260424T054715-d8d97a0e/manifest.json",
+    "result": ".ddx/executions/20260424T054715-d8d97a0e/result.json",
+    "checks": ".ddx/executions/20260424T054715-d8d97a0e/checks.json",
+    "usage": ".ddx/executions/20260424T054715-d8d97a0e/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-de9d564f-20260424T054715-d8d97a0e"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T054715-d8d97a0e/result.json b/.ddx/executions/20260424T054715-d8d97a0e/result.json
new file mode 100644
index 00000000..1e55f659
--- /dev/null
+++ b/.ddx/executions/20260424T054715-d8d97a0e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-de9d564f",
+  "attempt_id": "20260424T054715-d8d97a0e",
+  "base_rev": "b90c14dd574853cf3cbed92fe5d4d2dd0ba1e08a",
+  "result_rev": "401c7b088ebc927ecd7206753f1d4c6c44871e41",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d95070c9",
+  "duration_ms": 221816,
+  "tokens": 8546,
+  "cost_usd": 1.26432375,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T054715-d8d97a0e",
+  "prompt_file": ".ddx/executions/20260424T054715-d8d97a0e/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T054715-d8d97a0e/manifest.json",
+  "result_file": ".ddx/executions/20260424T054715-d8d97a0e/result.json",
+  "usage_file": ".ddx/executions/20260424T054715-d8d97a0e/usage.json",
+  "started_at": "2026-04-24T05:47:16.572584729Z",
+  "finished_at": "2026-04-24T05:50:58.388851604Z"
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
## Review: ddx-de9d564f iter 1

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
