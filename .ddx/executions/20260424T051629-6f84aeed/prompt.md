<bead-review>
  <bead id="ddx-7f9a7009" iter=1>
    <title>test: migrate TestExecuteBeadHeadRevFailure to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadHeadRevFailure() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadHeadRevFailure$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadHeadRevFailure no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadHeadRevFailure$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7df005969d2fcd756d25b7d809caa792d0eccb1d">
commit 7df005969d2fcd756d25b7d809caa792d0eccb1d
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 01:16:27 2026 -0400

    chore: add execution evidence [20260424T051127-]

diff --git a/.ddx/executions/20260424T051127-b2b83494/manifest.json b/.ddx/executions/20260424T051127-b2b83494/manifest.json
new file mode 100644
index 00000000..3f0c8216
--- /dev/null
+++ b/.ddx/executions/20260424T051127-b2b83494/manifest.json
@@ -0,0 +1,86 @@
+{
+  "attempt_id": "20260424T051127-b2b83494",
+  "bead_id": "ddx-7f9a7009",
+  "base_rev": "3bd613ccc342790f49689157782aa121a2c044e5",
+  "created_at": "2026-04-24T05:11:27.514355324Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-7f9a7009",
+    "title": "test: migrate TestExecuteBeadHeadRevFailure to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadHeadRevFailure() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadHeadRevFailure$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadHeadRevFailure no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadHeadRevFailure$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T05:11:26Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"bragi\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:35:08.617271893Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=bragi model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260421T173420-65f71d0c\",\"harness\":\"agent\",\"provider\":\"bragi\",\"model\":\"qwen3.5-27b\",\"input_tokens\":1830,\"output_tokens\":188,\"total_tokens\":2018,\"cost_usd\":0,\"duration_ms\":47443,\"exit_code\":0}",
+          "created_at": "2026-04-21T17:35:08.680064371Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2018 model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nno_changes",
+          "created_at": "2026-04-21T17:35:08.864454103Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"no_changes\",\"cost_usd\":0,\"duration_ms\":47443}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:35:08.92503425Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: no_changes\ntier=cheap\nprobe_result=ok\nresult_rev=13e04767683747bcafe74bcaa1a68900049f546f\nbase_rev=13e04767683747bcafe74bcaa1a68900049f546f\nretry_after=2026-04-21T23:35:09Z",
+          "created_at": "2026-04-21T17:35:09.144093738Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T05:11:26.993026775Z",
+      "execute-loop-last-detail": "escalation exhausted: no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-04-21T23:35:09Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T051127-b2b83494",
+    "prompt": ".ddx/executions/20260424T051127-b2b83494/prompt.md",
+    "manifest": ".ddx/executions/20260424T051127-b2b83494/manifest.json",
+    "result": ".ddx/executions/20260424T051127-b2b83494/result.json",
+    "checks": ".ddx/executions/20260424T051127-b2b83494/checks.json",
+    "usage": ".ddx/executions/20260424T051127-b2b83494/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-7f9a7009-20260424T051127-b2b83494"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T051127-b2b83494/result.json b/.ddx/executions/20260424T051127-b2b83494/result.json
new file mode 100644
index 00000000..ff285b20
--- /dev/null
+++ b/.ddx/executions/20260424T051127-b2b83494/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-7f9a7009",
+  "attempt_id": "20260424T051127-b2b83494",
+  "base_rev": "3bd613ccc342790f49689157782aa121a2c044e5",
+  "result_rev": "2140ed45babfc83a05ec1065755c2947e55ff9b2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-f51324f2",
+  "duration_ms": 298481,
+  "tokens": 12457,
+  "cost_usd": 1.33286425,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T051127-b2b83494",
+  "prompt_file": ".ddx/executions/20260424T051127-b2b83494/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T051127-b2b83494/manifest.json",
+  "result_file": ".ddx/executions/20260424T051127-b2b83494/result.json",
+  "usage_file": ".ddx/executions/20260424T051127-b2b83494/usage.json",
+  "started_at": "2026-04-24T05:11:27.514660198Z",
+  "finished_at": "2026-04-24T05:16:25.995746927Z"
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
## Review: ddx-7f9a7009 iter 1

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
