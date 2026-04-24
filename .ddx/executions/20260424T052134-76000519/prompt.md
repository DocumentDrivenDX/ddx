<bead-review>
  <bead id="ddx-9e2dad4b" iter=1>
    <title>test: migrate TestExecuteBeadCompoundErrorAgentAndHeadRevFailure to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadCompoundErrorAgentAndHeadRevFailure() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadCompoundErrorAgentAndHeadRevFailure$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadCompoundErrorAgentAndHeadRevFailure no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadCompoundErrorAgentAndHeadRevFailure$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9d836f33e20aa611aa979c0994433ab384327223">
commit 9d836f33e20aa611aa979c0994433ab384327223
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 01:21:31 2026 -0400

    chore: add execution evidence [20260424T051722-]

diff --git a/.ddx/executions/20260424T051722-521be246/manifest.json b/.ddx/executions/20260424T051722-521be246/manifest.json
new file mode 100644
index 00000000..99641286
--- /dev/null
+++ b/.ddx/executions/20260424T051722-521be246/manifest.json
@@ -0,0 +1,86 @@
+{
+  "attempt_id": "20260424T051722-521be246",
+  "bead_id": "ddx-9e2dad4b",
+  "base_rev": "2c5824b31d028f7a74f92286730983d22f281afb",
+  "created_at": "2026-04-24T05:17:23.421973399Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9e2dad4b",
+    "title": "test: migrate TestExecuteBeadCompoundErrorAgentAndHeadRevFailure to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadCompoundErrorAgentAndHeadRevFailure() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadCompoundErrorAgentAndHeadRevFailure$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadCompoundErrorAgentAndHeadRevFailure no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadCompoundErrorAgentAndHeadRevFailure$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T05:17:22Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"bragi\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-21T17:36:00.87570122Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=bragi model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260421T173509-af28c820\",\"harness\":\"agent\",\"provider\":\"bragi\",\"model\":\"qwen3.5-27b\",\"input_tokens\":1844,\"output_tokens\":203,\"total_tokens\":2047,\"cost_usd\":0,\"duration_ms\":50811,\"exit_code\":0}",
+          "created_at": "2026-04-21T17:36:00.938785836Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2047 model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nno_changes",
+          "created_at": "2026-04-21T17:36:01.128764893Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"no_changes\",\"cost_usd\":0,\"duration_ms\":50811}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-21T17:36:01.186700471Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=1 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: no_changes\ntier=cheap\nprobe_result=ok\nresult_rev=de24de7382a46af5f788bf91b7c16f80bdb33291\nbase_rev=de24de7382a46af5f788bf91b7c16f80bdb33291\nretry_after=2026-04-21T23:36:01Z",
+          "created_at": "2026-04-21T17:36:01.409671627Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T05:17:22.968284916Z",
+      "execute-loop-last-detail": "escalation exhausted: no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-04-21T23:36:01Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T051722-521be246",
+    "prompt": ".ddx/executions/20260424T051722-521be246/prompt.md",
+    "manifest": ".ddx/executions/20260424T051722-521be246/manifest.json",
+    "result": ".ddx/executions/20260424T051722-521be246/result.json",
+    "checks": ".ddx/executions/20260424T051722-521be246/checks.json",
+    "usage": ".ddx/executions/20260424T051722-521be246/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9e2dad4b-20260424T051722-521be246"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T051722-521be246/result.json b/.ddx/executions/20260424T051722-521be246/result.json
new file mode 100644
index 00000000..d1d25e09
--- /dev/null
+++ b/.ddx/executions/20260424T051722-521be246/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9e2dad4b",
+  "attempt_id": "20260424T051722-521be246",
+  "base_rev": "2c5824b31d028f7a74f92286730983d22f281afb",
+  "result_rev": "d96854fcaedb3e11d3bc6de26505542ed5614cb2",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-e2f8ae17",
+  "duration_ms": 247262,
+  "tokens": 11148,
+  "cost_usd": 1.378206,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T051722-521be246",
+  "prompt_file": ".ddx/executions/20260424T051722-521be246/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T051722-521be246/manifest.json",
+  "result_file": ".ddx/executions/20260424T051722-521be246/result.json",
+  "usage_file": ".ddx/executions/20260424T051722-521be246/usage.json",
+  "started_at": "2026-04-24T05:17:23.422294107Z",
+  "finished_at": "2026-04-24T05:21:30.685092367Z"
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
## Review: ddx-9e2dad4b iter 1

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
