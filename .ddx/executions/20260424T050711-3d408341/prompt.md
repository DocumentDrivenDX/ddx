<bead-review>
  <bead id="ddx-0629ed12" iter=1>
    <title>test: migrate TestExecuteBeadAgentErrorWithCommitsPreserves to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadAgentErrorWithCommitsPreserves() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadAgentErrorWithCommitsPreserves$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadAgentErrorWithCommitsPreserves no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadAgentErrorWithCommitsPreserves$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="98a6cd10ca81c2db13e46aad73e3ed6a02a72de4">
commit 98a6cd10ca81c2db13e46aad73e3ed6a02a72de4
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 01:07:09 2026 -0400

    chore: add execution evidence [20260424T050415-]

diff --git a/.ddx/executions/20260424T050415-f124cdc5/manifest.json b/.ddx/executions/20260424T050415-f124cdc5/manifest.json
new file mode 100644
index 00000000..0d795303
--- /dev/null
+++ b/.ddx/executions/20260424T050415-f124cdc5/manifest.json
@@ -0,0 +1,86 @@
+{
+  "attempt_id": "20260424T050415-f124cdc5",
+  "bead_id": "ddx-0629ed12",
+  "base_rev": "ca16f39edb6997abc7b1607ba0517b57b1634e0f",
+  "created_at": "2026-04-24T05:04:15.692595135Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-0629ed12",
+    "title": "test: migrate TestExecuteBeadAgentErrorWithCommitsPreserves to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadAgentErrorWithCommitsPreserves() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadAgentErrorWithCommitsPreserves$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadAgentErrorWithCommitsPreserves no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadAgentErrorWithCommitsPreserves$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T05:04:15Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"bragi\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:34:04.464134948Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=bragi model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260421T173313-5c1ad1f2\",\"harness\":\"agent\",\"provider\":\"bragi\",\"model\":\"qwen3.5-27b\",\"input_tokens\":1845,\"output_tokens\":200,\"total_tokens\":2045,\"cost_usd\":0,\"duration_ms\":50668,\"exit_code\":0}",
+          "created_at": "2026-04-21T17:34:04.534746461Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2045 model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nno_changes",
+          "created_at": "2026-04-21T17:34:04.759796818Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"no_changes\",\"cost_usd\":0,\"duration_ms\":50668}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:34:04.816793719Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: no_changes\ntier=cheap\nprobe_result=ok\nresult_rev=b1e5af26559537bcfcb406a6a98afe67d6de32b4\nbase_rev=b1e5af26559537bcfcb406a6a98afe67d6de32b4\nretry_after=2026-04-21T23:34:04Z",
+          "created_at": "2026-04-21T17:34:05.034444501Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T05:04:15.174963065Z",
+      "execute-loop-last-detail": "escalation exhausted: no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-04-21T23:34:04Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T050415-f124cdc5",
+    "prompt": ".ddx/executions/20260424T050415-f124cdc5/prompt.md",
+    "manifest": ".ddx/executions/20260424T050415-f124cdc5/manifest.json",
+    "result": ".ddx/executions/20260424T050415-f124cdc5/result.json",
+    "checks": ".ddx/executions/20260424T050415-f124cdc5/checks.json",
+    "usage": ".ddx/executions/20260424T050415-f124cdc5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-0629ed12-20260424T050415-f124cdc5"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T050415-f124cdc5/result.json b/.ddx/executions/20260424T050415-f124cdc5/result.json
new file mode 100644
index 00000000..cc97c56f
--- /dev/null
+++ b/.ddx/executions/20260424T050415-f124cdc5/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-0629ed12",
+  "attempt_id": "20260424T050415-f124cdc5",
+  "base_rev": "ca16f39edb6997abc7b1607ba0517b57b1634e0f",
+  "result_rev": "3a4248c8ebc612d5c2a7f022b4d716bddbcc50c0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-aeb4c910",
+  "duration_ms": 172657,
+  "tokens": 8080,
+  "cost_usd": 0.93148025,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T050415-f124cdc5",
+  "prompt_file": ".ddx/executions/20260424T050415-f124cdc5/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T050415-f124cdc5/manifest.json",
+  "result_file": ".ddx/executions/20260424T050415-f124cdc5/result.json",
+  "usage_file": ".ddx/executions/20260424T050415-f124cdc5/usage.json",
+  "started_at": "2026-04-24T05:04:15.692939676Z",
+  "finished_at": "2026-04-24T05:07:08.350740667Z"
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
## Review: ddx-0629ed12 iter 1

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
