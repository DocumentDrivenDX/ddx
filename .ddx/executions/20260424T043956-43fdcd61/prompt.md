<bead-review>
  <bead id="ddx-02e5e3ba" iter=1>
    <title>test: migrate TestExecuteBeadOrphanRecovery to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadOrphanRecovery() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadOrphanRecovery$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadOrphanRecovery no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadOrphanRecovery$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="6effebaea570aae432cb01e7d4456d56132e045c">
commit 6effebaea570aae432cb01e7d4456d56132e045c
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 00:39:53 2026 -0400

    chore: add execution evidence [20260424T043617-]

diff --git a/.ddx/executions/20260424T043617-7e6a33ba/manifest.json b/.ddx/executions/20260424T043617-7e6a33ba/manifest.json
new file mode 100644
index 00000000..70b7b17e
--- /dev/null
+++ b/.ddx/executions/20260424T043617-7e6a33ba/manifest.json
@@ -0,0 +1,77 @@
+{
+  "attempt_id": "20260424T043617-7e6a33ba",
+  "bead_id": "ddx-02e5e3ba",
+  "base_rev": "bec8fe5f90226c856ef765b86f156a402b111671",
+  "created_at": "2026-04-24T04:36:17.975865898Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-02e5e3ba",
+    "title": "test: migrate TestExecuteBeadOrphanRecovery to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadOrphanRecovery() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadOrphanRecovery$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadOrphanRecovery no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadOrphanRecovery$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T04:36:17Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"vidar-omlx\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:31:34.105386122Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=vidar-omlx model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen3.5-27b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-21T17:31:34.305706667Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":5}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:31:34.36493551Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: agent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen3.5-27b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}\ntier=cheap\nprobe_result=ok\nresult_rev=6f65d52d9f2b042c0027528428ef07e1937a0c20\nbase_rev=6f65d52d9f2b042c0027528428ef07e1937a0c20\nretry_after=2026-04-21T23:31:34Z",
+          "created_at": "2026-04-21T17:31:34.552366904Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T04:36:17.460711834Z",
+      "execute-loop-last-detail": "escalation exhausted: agent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen3.5-27b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-21T23:31:34Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T043617-7e6a33ba",
+    "prompt": ".ddx/executions/20260424T043617-7e6a33ba/prompt.md",
+    "manifest": ".ddx/executions/20260424T043617-7e6a33ba/manifest.json",
+    "result": ".ddx/executions/20260424T043617-7e6a33ba/result.json",
+    "checks": ".ddx/executions/20260424T043617-7e6a33ba/checks.json",
+    "usage": ".ddx/executions/20260424T043617-7e6a33ba/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-02e5e3ba-20260424T043617-7e6a33ba"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T043617-7e6a33ba/result.json b/.ddx/executions/20260424T043617-7e6a33ba/result.json
new file mode 100644
index 00000000..24f0894a
--- /dev/null
+++ b/.ddx/executions/20260424T043617-7e6a33ba/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-02e5e3ba",
+  "attempt_id": "20260424T043617-7e6a33ba",
+  "base_rev": "bec8fe5f90226c856ef765b86f156a402b111671",
+  "result_rev": "c39501aa05b966d22fabed5d876af37a7989d95b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a4836c5b",
+  "duration_ms": 215005,
+  "tokens": 8710,
+  "cost_usd": 1.13829375,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T043617-7e6a33ba",
+  "prompt_file": ".ddx/executions/20260424T043617-7e6a33ba/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T043617-7e6a33ba/manifest.json",
+  "result_file": ".ddx/executions/20260424T043617-7e6a33ba/result.json",
+  "usage_file": ".ddx/executions/20260424T043617-7e6a33ba/usage.json",
+  "started_at": "2026-04-24T04:36:17.976192648Z",
+  "finished_at": "2026-04-24T04:39:52.981492518Z"
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
## Review: ddx-02e5e3ba iter 1

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
