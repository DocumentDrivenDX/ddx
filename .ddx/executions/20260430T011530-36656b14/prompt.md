<bead-review>
  <bead id="ddx-950321ef" iter=1>
    <title>routing-frontend-goldens: Playwright drain-queue fixtures matching real config</title>
    <description>
Cover AC #6 from ddx-fdd3ea36. Playwright e2e fixture matches user's real config (endpoints: lmstudio + omlx + lmstudio + lmstudio, with and without gemini in allow-list). Click Drain on project home → within 5s a worker row appears with harness backed by a live endpoint. Never gemini + non-gemini model. Never 19 failures. After drain completes: worker status success (or typed error exit with reason recorded on the bead). Attempts count matches actual work done. Lives under cli/internal/server/frontend/src/routes/**/*.e2e.ts.
    </description>
    <acceptance>
1. New Playwright e2e file under cli/internal/server/frontend/src/routes/.../*.e2e.ts. 2. Fixture replicates real-config endpoint shape (lmstudio + omlx + lmstudio + lmstudio). 3. Test scenario: click Drain → worker row appears within 5s with a live-endpoint-backed harness. 4. Assertion: never gemini + non-gemini model; never 19-failure pattern. 5. Assertion: drain completion status is success OR typed error with reason on the bead; attempts count matches work done. 6. Test runs under bun run test:e2e in CI.
    </acceptance>
    <labels>feat-006, routing, frontend, playwright</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9d36e8caf3d41ac56c5a118185ad012889cc65b2">
commit 9d36e8caf3d41ac56c5a118185ad012889cc65b2
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 21:15:27 2026 -0400

    chore: add execution evidence [20260430T011125-]

diff --git a/.ddx/executions/20260430T011125-970a9b6f/manifest.json b/.ddx/executions/20260430T011125-970a9b6f/manifest.json
new file mode 100644
index 00000000..806aaf74
--- /dev/null
+++ b/.ddx/executions/20260430T011125-970a9b6f/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260430T011125-970a9b6f",
+  "bead_id": "ddx-950321ef",
+  "base_rev": "f7c624e2aef51c2c5badf228ba7cf344d558f0f2",
+  "created_at": "2026-04-30T01:11:26.747940914Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-950321ef",
+    "title": "routing-frontend-goldens: Playwright drain-queue fixtures matching real config",
+    "description": "Cover AC #6 from ddx-fdd3ea36. Playwright e2e fixture matches user's real config (endpoints: lmstudio + omlx + lmstudio + lmstudio, with and without gemini in allow-list). Click Drain on project home → within 5s a worker row appears with harness backed by a live endpoint. Never gemini + non-gemini model. Never 19 failures. After drain completes: worker status success (or typed error exit with reason recorded on the bead). Attempts count matches actual work done. Lives under cli/internal/server/frontend/src/routes/**/*.e2e.ts.",
+    "acceptance": "1. New Playwright e2e file under cli/internal/server/frontend/src/routes/.../*.e2e.ts. 2. Fixture replicates real-config endpoint shape (lmstudio + omlx + lmstudio + lmstudio). 3. Test scenario: click Drain → worker row appears within 5s with a live-endpoint-backed harness. 4. Assertion: never gemini + non-gemini model; never 19-failure pattern. 5. Assertion: drain completion status is success OR typed error with reason on the bead; attempts count matches work done. 6. Test runs under bun run test:e2e in CI.",
+    "parent": "ddx-fdd3ea36",
+    "labels": [
+      "feat-006",
+      "routing",
+      "frontend",
+      "playwright"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-30T01:11:25Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "execute-loop-heartbeat-at": "2026-04-30T01:11:25.939043283Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260430T011125-970a9b6f",
+    "prompt": ".ddx/executions/20260430T011125-970a9b6f/prompt.md",
+    "manifest": ".ddx/executions/20260430T011125-970a9b6f/manifest.json",
+    "result": ".ddx/executions/20260430T011125-970a9b6f/result.json",
+    "checks": ".ddx/executions/20260430T011125-970a9b6f/checks.json",
+    "usage": ".ddx/executions/20260430T011125-970a9b6f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-950321ef-20260430T011125-970a9b6f"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260430T011125-970a9b6f/result.json b/.ddx/executions/20260430T011125-970a9b6f/result.json
new file mode 100644
index 00000000..b735d332
--- /dev/null
+++ b/.ddx/executions/20260430T011125-970a9b6f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-950321ef",
+  "attempt_id": "20260430T011125-970a9b6f",
+  "base_rev": "f7c624e2aef51c2c5badf228ba7cf344d558f0f2",
+  "result_rev": "110966e811e3bd0f00addc89b9093f93712c1e8f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-86ac526b",
+  "duration_ms": 239465,
+  "tokens": 14307,
+  "cost_usd": 1.6312054999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260430T011125-970a9b6f",
+  "prompt_file": ".ddx/executions/20260430T011125-970a9b6f/prompt.md",
+  "manifest_file": ".ddx/executions/20260430T011125-970a9b6f/manifest.json",
+  "result_file": ".ddx/executions/20260430T011125-970a9b6f/result.json",
+  "usage_file": ".ddx/executions/20260430T011125-970a9b6f/usage.json",
+  "started_at": "2026-04-30T01:11:26.748199997Z",
+  "finished_at": "2026-04-30T01:15:26.213879443Z"
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
## Review: ddx-950321ef iter 1

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
