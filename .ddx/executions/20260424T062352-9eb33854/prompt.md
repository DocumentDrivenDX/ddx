<bead-review>
  <bead id="ddx-9815ef2f" iter=1>
    <title>test: migrate TestExecuteBeadPromptIsXMLTagged to real git + script harness</title>
    <description>
Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadPromptIsXMLTagged() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.

Pattern:
- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.
- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.
- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.
- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.
- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.

Scope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.

Verification: cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadPromptIsXMLTagged$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.

Governing concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.
    </description>
    <acceptance>
1) TestExecuteBeadPromptIsXMLTagged no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli &amp;&amp; go test ./cmd -run '^TestExecuteBeadPromptIsXMLTagged$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.
    </acceptance>
    <labels>phase:test, area:cli, area:agent, kind:refactor, workstream:fake-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="273a8bdfe349b49269670bb491abecd7fdcfe9e8">
commit 273a8bdfe349b49269670bb491abecd7fdcfe9e8
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 02:23:49 2026 -0400

    chore: add execution evidence [20260424T061930-]

diff --git a/.ddx/executions/20260424T061930-57121602/manifest.json b/.ddx/executions/20260424T061930-57121602/manifest.json
new file mode 100644
index 00000000..18f2c395
--- /dev/null
+++ b/.ddx/executions/20260424T061930-57121602/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260424T061930-57121602",
+  "bead_id": "ddx-9815ef2f",
+  "base_rev": "3eab8da30889c4a5bbf2db18bc2b5031f2089ba8",
+  "created_at": "2026-04-24T06:19:31.096711697Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9815ef2f",
+    "title": "test: migrate TestExecuteBeadPromptIsXMLTagged to real git + script harness",
+    "description": "Migrate cli/cmd/agent_execute_bead_test.go TestExecuteBeadPromptIsXMLTagged() from fakeExecuteBeadGit + fakeAgentRunner to real git + script harness.\n\nPattern:\n- Use cli/internal/agent/integration_helper_test.go newScriptHarnessRepo(t, beadCount) to build an isolated repo with real git.\n- Use runGitInteg (from cli/internal/git/git_basic_test.go) for any supplemental git invocations, so GIT_* env is scrubbed.\n- Replace fakeExecuteBeadGit with the RealLandingGitOps flow used in cli/internal/agent/execute_bead_land_test.go.\n- Replace fakeAgentRunner with the script harness (cli/internal/agent/script.go) driven by a per-attempt directive file. When the test needs to simulate 'agent emits no commits' or 'agent emits a dirty worktree', encode that as script directives rather than a canned Result struct.\n- Preserve every assertion — the point is that the test now exercises real git + real merge/preserve semantics, not that the surface shrinks.\n\nScope limit: ONLY this test. Do not migrate any other test in the same file; touching another test expands blast radius outside the bead.\n\nVerification: cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadPromptIsXMLTagged$' -count=1 passes. No struct ending in -GitOps or -Runner that fakes the thing under test is referenced from this test after the migration.\n\nGoverning concern: concerns.md §testing 'no mocks, period' + 'never mock the thing you are testing'. Parent: ddx-d9df348d.",
+    "acceptance": "1) TestExecuteBeadPromptIsXMLTagged no longer references fakeExecuteBeadGit or fakeAgentRunner. 2) Test dispatches through RealLandingGitOps + newScriptHarnessRepo (or equivalent real-git harness). 3) cd cli \u0026\u0026 go test ./cmd -run '^TestExecuteBeadPromptIsXMLTagged$' -count=1 green. 4) No other test in agent_execute_bead_test.go is modified.",
+    "parent": "ddx-d9df348d",
+    "labels": [
+      "phase:test",
+      "area:cli",
+      "area:agent",
+      "kind:refactor",
+      "workstream:fake-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T06:19:30Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-24T06:19:30.507267159Z",
+      "spec-id": "concerns.testing"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T061930-57121602",
+    "prompt": ".ddx/executions/20260424T061930-57121602/prompt.md",
+    "manifest": ".ddx/executions/20260424T061930-57121602/manifest.json",
+    "result": ".ddx/executions/20260424T061930-57121602/result.json",
+    "checks": ".ddx/executions/20260424T061930-57121602/checks.json",
+    "usage": ".ddx/executions/20260424T061930-57121602/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9815ef2f-20260424T061930-57121602"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T061930-57121602/result.json b/.ddx/executions/20260424T061930-57121602/result.json
new file mode 100644
index 00000000..2038ed30
--- /dev/null
+++ b/.ddx/executions/20260424T061930-57121602/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9815ef2f",
+  "attempt_id": "20260424T061930-57121602",
+  "base_rev": "3eab8da30889c4a5bbf2db18bc2b5031f2089ba8",
+  "result_rev": "041fae82af40fdd85118d60a72c8dbfb525c4183",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0c152b85",
+  "duration_ms": 257818,
+  "tokens": 10501,
+  "cost_usd": 1.32247925,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T061930-57121602",
+  "prompt_file": ".ddx/executions/20260424T061930-57121602/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T061930-57121602/manifest.json",
+  "result_file": ".ddx/executions/20260424T061930-57121602/result.json",
+  "usage_file": ".ddx/executions/20260424T061930-57121602/usage.json",
+  "started_at": "2026-04-24T06:19:31.097030822Z",
+  "finished_at": "2026-04-24T06:23:48.915991355Z"
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
## Review: ddx-9815ef2f iter 1

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
