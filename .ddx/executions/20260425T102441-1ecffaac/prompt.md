<bead-review>
  <bead id="ddx-da995601" iter=1>
    <title>clean up leftovers B7 missed: cmd/init.go + server/state.go + bead/ + delete gitEnvForDir</title>
    <description>
B7 (ddx-d3119355) closed but left work undone. Finish it.

Remaining bare git exec callsites per audit:
- cli/cmd/init.go (4 callsites) — migrate, then delete gitEnvForDir() helper and update callers to use git.CleanEnv if any remain
- cli/internal/server/ (3 callsites)
- cli/internal/bead/ (2 callsites)
- cli/internal/git/ (16 callsites — most are inside the wrapper's own implementation; review and migrate only the ones that are NOT the wrapper calling git internally)

For init.go: the legacy gitEnvForDir() from the pre-B4 partial fix should be removed. Any callers switch to passing through git.Command (which scrubs automatically) or git.CleanEnv if spawning non-git processes.
    </description>
    <acceptance>
grep shows no bare git exec.Command in cli/cmd/init.go, cli/internal/server/, cli/internal/bead/. gitEnvForDir function removed. cli/internal/git/ callsites reviewed and any outside the wrapper's self-use migrated. Build + tests green.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="cb950966f1526e1d963cb2262a706fc73e9fc9e0">
commit cb950966f1526e1d963cb2262a706fc73e9fc9e0
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 06:24:39 2026 -0400

    chore: add execution evidence [20260425T101225-]

diff --git a/.ddx/executions/20260425T101225-a1869f2d/manifest.json b/.ddx/executions/20260425T101225-a1869f2d/manifest.json
new file mode 100644
index 00000000..72d782d5
--- /dev/null
+++ b/.ddx/executions/20260425T101225-a1869f2d/manifest.json
@@ -0,0 +1,66 @@
+{
+  "attempt_id": "20260425T101225-a1869f2d",
+  "bead_id": "ddx-da995601",
+  "base_rev": "6ab50d99e67d2d59aaa50a4436f3fa0fc2e11e66",
+  "created_at": "2026-04-25T10:12:26.412340284Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-da995601",
+    "title": "clean up leftovers B7 missed: cmd/init.go + server/state.go + bead/ + delete gitEnvForDir",
+    "description": "B7 (ddx-d3119355) closed but left work undone. Finish it.\n\nRemaining bare git exec callsites per audit:\n- cli/cmd/init.go (4 callsites) — migrate, then delete gitEnvForDir() helper and update callers to use git.CleanEnv if any remain\n- cli/internal/server/ (3 callsites)\n- cli/internal/bead/ (2 callsites)\n- cli/internal/git/ (16 callsites — most are inside the wrapper's own implementation; review and migrate only the ones that are NOT the wrapper calling git internally)\n\nFor init.go: the legacy gitEnvForDir() from the pre-B4 partial fix should be removed. Any callers switch to passing through git.Command (which scrubs automatically) or git.CleanEnv if spawning non-git processes.",
+    "acceptance": "grep shows no bare git exec.Command in cli/cmd/init.go, cli/internal/server/, cli/internal/bead/. gitEnvForDir function removed. cli/internal/git/ callsites reviewed and any outside the wrapper's self-use migrated. Build + tests green.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "claimed-at": "2026-04-25T10:12:25Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:35.973550648Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:36.077156978Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-25T02:23:36.13877431Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-25T02:23:36.264812557Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-25T10:12:25.440495533Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260425T101225-a1869f2d",
+    "prompt": ".ddx/executions/20260425T101225-a1869f2d/prompt.md",
+    "manifest": ".ddx/executions/20260425T101225-a1869f2d/manifest.json",
+    "result": ".ddx/executions/20260425T101225-a1869f2d/result.json",
+    "checks": ".ddx/executions/20260425T101225-a1869f2d/checks.json",
+    "usage": ".ddx/executions/20260425T101225-a1869f2d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-da995601-20260425T101225-a1869f2d"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T101225-a1869f2d/result.json b/.ddx/executions/20260425T101225-a1869f2d/result.json
new file mode 100644
index 00000000..c6273326
--- /dev/null
+++ b/.ddx/executions/20260425T101225-a1869f2d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-da995601",
+  "attempt_id": "20260425T101225-a1869f2d",
+  "base_rev": "6ab50d99e67d2d59aaa50a4436f3fa0fc2e11e66",
+  "result_rev": "297ce446dc6a314b97d6b00848cbcf44ec0b5871",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a388ac9d",
+  "duration_ms": 731708,
+  "tokens": 27811,
+  "cost_usd": 5.2540615000000015,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T101225-a1869f2d",
+  "prompt_file": ".ddx/executions/20260425T101225-a1869f2d/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T101225-a1869f2d/manifest.json",
+  "result_file": ".ddx/executions/20260425T101225-a1869f2d/result.json",
+  "usage_file": ".ddx/executions/20260425T101225-a1869f2d/usage.json",
+  "started_at": "2026-04-25T10:12:26.412629618Z",
+  "finished_at": "2026-04-25T10:24:38.121362568Z"
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
## Review: ddx-da995601 iter 1

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
