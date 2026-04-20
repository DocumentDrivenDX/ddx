<bead-review>
  <bead id=".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1" iter=1>
    <title>execute-bead: propagate caller context through Runner.Run and RunAgent</title>
    <description>
## Root cause RC1 from ddx-0a651925

`cli/internal/agent/runner.go:239` (Runner.Run) and `cli/internal/agent/agent_runner.go:163` (Runner.RunAgent) both call `context.WithCancel(context.Background())`, discarding any context from the caller. `RunOptions` exposes no ctx field. `agent.ExecuteBead` likewise takes no ctx, so `server.WorkerManager.runWorker`'s ctx never reaches the agent call.

As a result, `WorkerManager.Stop(id)` (server/workers.go:648) cancels only the worker goroutine; the embedded `agentlib.Run` call (and any subprocess it launched) keeps running until SIGKILL. This is the structural defect behind 33–142h hung workers.

## Scope

1. Add `Context context.Context` to `RunOptions` (or change signatures to `Run(ctx, opts)`). Default to `context.Background()` when nil.
2. Derive the internal ctx from the caller's rather than `context.Background()`.
3. Thread ctx through `agent.ExecuteBead` → `runner.Run` → `agentlib.Run` → provider HTTP.
4. Update `server.WorkerManager.runWorker`'s `singleTierAttempt` closure to pass `ctx` into `agent.ExecuteBead`.

## Non-goals

- New wall-clock timeout logic (covered by sibling RC2 bead)
- Operator-facing stop command (covered by ddx-b808df39)
    </description>
    <acceptance>
- [ ] `RunOptions` accepts a caller ctx, or `Runner.Run`/`Runner.RunAgent` accept ctx explicitly
- [ ] The internal cancel is derived from the caller ctx (no `context.Background()` seeds inside Run/RunAgent)
- [ ] `agent.ExecuteBead` takes ctx and passes it down
- [ ] `server.WorkerManager.Stop(id)` cancels the running agent provider call within 2s in a unit test (fake provider blocks on ctx)
- [ ] Regression test: runner with a canceled ctx returns promptly (&lt;500ms) regardless of mocked provider latency
    </acceptance>
    <labels>ddx, phase:build, kind:bug, area:agent, area:workers, root-cause:rc1</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9b044bcebc981533d2d48d4ffd79a91743dee884">
commit 9b044bcebc981533d2d48d4ffd79a91743dee884
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 01:14:55 2026 -0400

    chore: add execution evidence [20260418T045141-]

diff --git a/.ddx/executions/20260418T045141-e59897a9/manifest.json b/.ddx/executions/20260418T045141-e59897a9/manifest.json
new file mode 100644
index 00000000..22a3668b
--- /dev/null
+++ b/.ddx/executions/20260418T045141-e59897a9/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260418T045141-e59897a9",
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1",
+  "base_rev": "cea3a244ffbe2e921a1338365f9c262f8ba6d10d",
+  "created_at": "2026-04-18T04:51:41.94301397Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1",
+    "title": "execute-bead: propagate caller context through Runner.Run and RunAgent",
+    "description": "## Root cause RC1 from ddx-0a651925\n\n`cli/internal/agent/runner.go:239` (Runner.Run) and `cli/internal/agent/agent_runner.go:163` (Runner.RunAgent) both call `context.WithCancel(context.Background())`, discarding any context from the caller. `RunOptions` exposes no ctx field. `agent.ExecuteBead` likewise takes no ctx, so `server.WorkerManager.runWorker`'s ctx never reaches the agent call.\n\nAs a result, `WorkerManager.Stop(id)` (server/workers.go:648) cancels only the worker goroutine; the embedded `agentlib.Run` call (and any subprocess it launched) keeps running until SIGKILL. This is the structural defect behind 33–142h hung workers.\n\n## Scope\n\n1. Add `Context context.Context` to `RunOptions` (or change signatures to `Run(ctx, opts)`). Default to `context.Background()` when nil.\n2. Derive the internal ctx from the caller's rather than `context.Background()`.\n3. Thread ctx through `agent.ExecuteBead` → `runner.Run` → `agentlib.Run` → provider HTTP.\n4. Update `server.WorkerManager.runWorker`'s `singleTierAttempt` closure to pass `ctx` into `agent.ExecuteBead`.\n\n## Non-goals\n\n- New wall-clock timeout logic (covered by sibling RC2 bead)\n- Operator-facing stop command (covered by ddx-b808df39)\n",
+    "acceptance": "- [ ] `RunOptions` accepts a caller ctx, or `Runner.Run`/`Runner.RunAgent` accept ctx explicitly\n- [ ] The internal cancel is derived from the caller ctx (no `context.Background()` seeds inside Run/RunAgent)\n- [ ] `agent.ExecuteBead` takes ctx and passes it down\n- [ ] `server.WorkerManager.Stop(id)` cancels the running agent provider call within 2s in a unit test (fake provider blocks on ctx)\n- [ ] Regression test: runner with a canceled ctx returns promptly (\u003c500ms) regardless of mocked provider latency",
+    "parent": "ddx-0a651925",
+    "labels": [
+      "ddx",
+      "phase:build",
+      "kind:bug",
+      "area:agent",
+      "area:workers",
+      "root-cause:rc1"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T04:51:41Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "execute-loop-heartbeat-at": "2026-04-18T04:51:41.61349631Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T045141-e59897a9",
+    "prompt": ".ddx/executions/20260418T045141-e59897a9/prompt.md",
+    "manifest": ".ddx/executions/20260418T045141-e59897a9/manifest.json",
+    "result": ".ddx/executions/20260418T045141-e59897a9/result.json",
+    "checks": ".ddx/executions/20260418T045141-e59897a9/checks.json",
+    "usage": ".ddx/executions/20260418T045141-e59897a9/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1-20260418T045141-e59897a9"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T045141-e59897a9/result.json b/.ddx/executions/20260418T045141-e59897a9/result.json
new file mode 100644
index 00000000..8fdc4e33
--- /dev/null
+++ b/.ddx/executions/20260418T045141-e59897a9/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1",
+  "attempt_id": "20260418T045141-e59897a9",
+  "base_rev": "cea3a244ffbe2e921a1338365f9c262f8ba6d10d",
+  "result_rev": "582f00f3d95da5de66a8edd87ab14b08d0e896c6",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-b9f9e0d0",
+  "duration_ms": 1392855,
+  "tokens": 113,
+  "cost_usd": 10.395166249999997,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T045141-e59897a9",
+  "prompt_file": ".ddx/executions/20260418T045141-e59897a9/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T045141-e59897a9/manifest.json",
+  "result_file": ".ddx/executions/20260418T045141-e59897a9/result.json",
+  "usage_file": ".ddx/executions/20260418T045141-e59897a9/usage.json",
+  "started_at": "2026-04-18T04:51:41.943318178Z",
+  "finished_at": "2026-04-18T05:14:54.798742199Z"
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
## Review: .execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1 iter 1

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
