<bead-review>
  <bead id="ddx-62985b35" iter=1>
    <title>test: migrate TestExecuteBeadAgentErrorMessageInOutput to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadAgentErrorMessageInOutput() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadAgentErrorMessageInOutput$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadAgentErrorMessageInOutput no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadAgentErrorMessageInOutput$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7cbd41237e3b80eae4dfc642275adcab48d61c6c">
commit 7cbd41237e3b80eae4dfc642275adcab48d61c6c
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 01:10:44 2026 -0400

    chore: add execution evidence [20260424T050745-]

diff --git a/.ddx/executions/20260424T050745-eb96719b/manifest.json b/.ddx/executions/20260424T050745-eb96719b/manifest.json
new file mode 100644
index 00000000..dcde9bbe
--- /dev/null
+++ b/.ddx/executions/20260424T050745-eb96719b/manifest.json
@@ -0,0 +1,86 @@
+{
+  "attempt_id": "20260424T050745-eb96719b",
+  "bead_id": "ddx-62985b35",
+  "base_rev": "1064993148d5bfa255501d43274bcf8e8cbd8502",
+  "created_at": "2026-04-24T05:07:45.608634875Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-62985b35",
+    "title": "test: migrate TestExecuteBeadAgentErrorMessageInOutput to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadAgentErrorMessageInOutput() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadAgentErrorMessageInOutput$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadAgentErrorMessageInOutput no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadAgentErrorMessageInOutput$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T05:07:45Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"openrouter\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:34:19.711997718Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=openrouter model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260421T173405-3436212e\",\"harness\":\"agent\",\"provider\":\"openrouter\",\"model\":\"qwen3.5-27b\",\"input_tokens\":1837,\"output_tokens\":266,\"total_tokens\":2103,\"cost_usd\":0.0011895,\"duration_ms\":13750,\"exit_code\":0}",
+          "created_at": "2026-04-21T17:34:19.769546368Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2103 cost_usd=0.0012 model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nno_changes",
+          "created_at": "2026-04-21T17:34:19.971620582Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"no_changes\",\"cost_usd\":0.0011895,\"duration_ms\":13750}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0.0011895,\"wasted_cost_usd\":0.0011895}",
+          "created_at": "2026-04-21T17:34:20.03865864Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0012 wasted_cost_usd=0.0012"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: no_changes\ntier=cheap\nprobe_result=ok\nresult_rev=77904a814695fe39b316b71875878e121ff56179\nbase_rev=77904a814695fe39b316b71875878e121ff56179\nretry_after=2026-04-21T23:34:20Z",
+          "created_at": "2026-04-21T17:34:20.259784168Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T05:07:45.076709661Z",
+      "execute-loop-last-detail": "escalation exhausted: no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-04-21T23:34:20Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T050745-eb96719b",
+    "prompt": ".ddx/executions/20260424T050745-eb96719b/prompt.md",
+    "manifest": ".ddx/executions/20260424T050745-eb96719b/manifest.json",
+    "result": ".ddx/executions/20260424T050745-eb96719b/result.json",
+    "checks": ".ddx/executions/20260424T050745-eb96719b/checks.json",
+    "usage": ".ddx/executions/20260424T050745-eb96719b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-62985b35-20260424T050745-eb96719b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T050745-eb96719b/result.json b/.ddx/executions/20260424T050745-eb96719b/result.json
new file mode 100644
index 00000000..0de26893
--- /dev/null
+++ b/.ddx/executions/20260424T050745-eb96719b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-62985b35",
+  "attempt_id": "20260424T050745-eb96719b",
+  "base_rev": "1064993148d5bfa255501d43274bcf8e8cbd8502",
+  "result_rev": "7a459736cc6697d45d2fde796f1a6608704444b2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-53202e78",
+  "duration_ms": 177479,
+  "tokens": 8007,
+  "cost_usd": 1.0065089999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T050745-eb96719b",
+  "prompt_file": ".ddx/executions/20260424T050745-eb96719b/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T050745-eb96719b/manifest.json",
+  "result_file": ".ddx/executions/20260424T050745-eb96719b/result.json",
+  "usage_file": ".ddx/executions/20260424T050745-eb96719b/usage.json",
+  "started_at": "2026-04-24T05:07:45.608940208Z",
+  "finished_at": "2026-04-24T05:10:43.088360679Z"
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
## Review: ddx-62985b35 iter 1

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
