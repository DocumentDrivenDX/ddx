<bead-review>
  <bead id="ddx-68c372a6" iter=1>
    <title>test: migrate TestExecuteBeadMerge to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadMerge() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadMerge$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadMerge no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadMerge$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="30b1f1283b7c4246b74aee85001203e60055d762">
commit 30b1f1283b7c4246b74aee85001203e60055d762
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 23:49:32 2026 -0400

    chore: add execution evidence [20260424T034347-]

diff --git a/.ddx/executions/20260424T034347-9277349d/manifest.json b/.ddx/executions/20260424T034347-9277349d/manifest.json
new file mode 100644
index 00000000..2f3e36d7
--- /dev/null
+++ b/.ddx/executions/20260424T034347-9277349d/manifest.json
@@ -0,0 +1,125 @@
+{
+  "attempt_id": "20260424T034347-9277349d",
+  "bead_id": "ddx-68c372a6",
+  "base_rev": "ef362b38080778289988f02f07c5f86745bf85df",
+  "created_at": "2026-04-24T03:43:47.599060771Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-68c372a6",
+    "title": "test: migrate TestExecuteBeadMerge to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadMerge() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadMerge$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadMerge no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadMerge$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T03:43:47Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"vidar-omlx\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:27:36.013515468Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=vidar-omlx model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness=agent model=qwen3.5-27b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen3.5-27b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-21T17:27:36.196029674Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "worker=worker-20260421T172735-5040 runtime=26s pid=0 reason=stop",
+          "created_at": "2026-04-21T17:28:01.344325765Z",
+          "kind": "bead.stopped",
+          "source": "server-workers",
+          "summary": "stop"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"gemini\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:28:01.603781334Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=gemini"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness=gemini model= probe=ok\ncancelled",
+          "created_at": "2026-04-21T17:28:01.781635293Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":16},{\"tier\":\"smart\",\"harness\":\"gemini\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":24876}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:28:01.83850117Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: cancelled\ntier=smart\nprobe_result=ok\nresult_rev=72f9c8c3d34f218f83e785eeeb09ac57a802e2cb\nbase_rev=72f9c8c3d34f218f83e785eeeb09ac57a802e2cb\nretry_after=2026-04-21T23:28:01Z",
+          "created_at": "2026-04-21T17:28:01.999254558Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"resolved_model\":\"qwen/qwen3.6\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T23:47:13.021453745Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235 model=qwen/qwen3.6"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-23T00:20:51.437771346Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260422T234723-8b0a7b43\",\"harness\":\"agent\",\"provider\":\"omlx-vidar-1235\",\"input_tokens\":2093464,\"output_tokens\":12804,\"total_tokens\":2106268,\"cost_usd\":0,\"duration_ms\":1991524,\"exit_code\":1}",
+          "created_at": "2026-04-23T00:20:51.580689242Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2106268"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T03:43:47.078057148Z",
+      "execute-loop-last-detail": "escalation exhausted: cancelled",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-21T23:28:01Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T034347-9277349d",
+    "prompt": ".ddx/executions/20260424T034347-9277349d/prompt.md",
+    "manifest": ".ddx/executions/20260424T034347-9277349d/manifest.json",
+    "result": ".ddx/executions/20260424T034347-9277349d/result.json",
+    "checks": ".ddx/executions/20260424T034347-9277349d/checks.json",
+    "usage": ".ddx/executions/20260424T034347-9277349d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-68c372a6-20260424T034347-9277349d"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T034347-9277349d/result.json b/.ddx/executions/20260424T034347-9277349d/result.json
new file mode 100644
index 00000000..cbd386c4
--- /dev/null
+++ b/.ddx/executions/20260424T034347-9277349d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-68c372a6",
+  "attempt_id": "20260424T034347-9277349d",
+  "base_rev": "ef362b38080778289988f02f07c5f86745bf85df",
+  "result_rev": "68e0bb275670ec81faabe918368605662b574c76",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-03864ca2",
+  "duration_ms": 343714,
+  "tokens": 16559,
+  "cost_usd": 2.5556304999999995,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T034347-9277349d",
+  "prompt_file": ".ddx/executions/20260424T034347-9277349d/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T034347-9277349d/manifest.json",
+  "result_file": ".ddx/executions/20260424T034347-9277349d/result.json",
+  "usage_file": ".ddx/executions/20260424T034347-9277349d/usage.json",
+  "started_at": "2026-04-24T03:43:47.599389398Z",
+  "finished_at": "2026-04-24T03:49:31.313909802Z"
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
## Review: ddx-68c372a6 iter 1

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
