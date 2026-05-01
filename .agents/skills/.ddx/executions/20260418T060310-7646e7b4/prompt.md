<bead-review>
  <bead id=".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-34d039b7" iter=1>
    <title>execute-bead: autonomous worker watchdog with process-level reaper</title>
    <description>
## Residual risk after RC1–RC4 from ddx-0a651925

Even with context propagation (RC1), wall-clock bounds (RC2), tightened breakers (RC3), and provider deadlines (RC4), a goroutine-level deadlock or panic-swallowed handler could still leave a worker alive past any reasonable bound. Today, `server.WorkerManager.Stop` (workers.go:648) only calls `handle.cancel()` — it has no escalation to OS-level signal and no autonomous trigger. ddx-b808df39 adds the *operator* tool; this bead adds the *autonomous* watchdog.

## Scope

1. WorkerManager runs a supervisor goroutine that, every minute, inspects each `workerHandle`.
2. If `time.Since(record.StartedAt) &gt; WatchdogDeadline` (default e.g. 6h) AND `record.CurrentAttempt` has not transitioned in `StallDeadline` (default e.g. 1h), escalate:
   - Send SIGTERM to the worker's process group (requires surfacing PID, aligned with ddx-b808df39).
   - After 30s grace, send SIGKILL.
   - Mark record.State = "reaped"; update claimed bead state so the claim is not leaked.
3. Emit a `bead.reaped` event on the tracker with the full timeline so post-mortems are easy.

## Non-goals

- Per-bead runtime tuning; defaults are enough.
- Replacing ddx-b808df39 — this is defense-in-depth, not duplication. ddx-b808df39 is operator-initiated; this bead is system-initiated.

## Dependencies

- ddx-b808df39 surfaces PID in `ddx agent workers --json` output; this bead reuses that plumbing.
    </description>
    <acceptance>
- [ ] WorkerManager starts a supervisor goroutine on first worker launch
- [ ] Watchdog reaps workers past `WatchdogDeadline` with no phase transition in `StallDeadline`
- [ ] SIGTERM→SIGKILL escalation works on an intentionally-wedged test worker (unit test with a goroutine that ignores ctx)
- [ ] Reaped worker's bead claim is released (tracker status returns to open or claim-expired)
- [ ] `bead.reaped` event recorded on tracker with reason="watchdog" and duration
- [ ] Watchdog deadlines are configurable via ddx server config
    </acceptance>
    <labels>ddx, phase:build, kind:enhancement, area:agent, area:workers, root-cause:defense-in-depth</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="13cef95535aea82c1f00dd596704f141ed131af7">
commit 13cef95535aea82c1f00dd596704f141ed131af7
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 02:03:09 2026 -0400

    chore: add execution evidence [20260418T054825-]

diff --git a/.ddx/executions/20260418T054825-291242ac/manifest.json b/.ddx/executions/20260418T054825-291242ac/manifest.json
new file mode 100644
index 00000000..e8d88c43
--- /dev/null
+++ b/.ddx/executions/20260418T054825-291242ac/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260418T054825-291242ac",
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-34d039b7",
+  "base_rev": "a8eb23221eb853c703e290be6ffa8e4e78e6630a",
+  "created_at": "2026-04-18T05:48:25.617642127Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-34d039b7",
+    "title": "execute-bead: autonomous worker watchdog with process-level reaper",
+    "description": "## Residual risk after RC1–RC4 from ddx-0a651925\n\nEven with context propagation (RC1), wall-clock bounds (RC2), tightened breakers (RC3), and provider deadlines (RC4), a goroutine-level deadlock or panic-swallowed handler could still leave a worker alive past any reasonable bound. Today, `server.WorkerManager.Stop` (workers.go:648) only calls `handle.cancel()` — it has no escalation to OS-level signal and no autonomous trigger. ddx-b808df39 adds the *operator* tool; this bead adds the *autonomous* watchdog.\n\n## Scope\n\n1. WorkerManager runs a supervisor goroutine that, every minute, inspects each `workerHandle`.\n2. If `time.Since(record.StartedAt) \u003e WatchdogDeadline` (default e.g. 6h) AND `record.CurrentAttempt` has not transitioned in `StallDeadline` (default e.g. 1h), escalate:\n   - Send SIGTERM to the worker's process group (requires surfacing PID, aligned with ddx-b808df39).\n   - After 30s grace, send SIGKILL.\n   - Mark record.State = \"reaped\"; update claimed bead state so the claim is not leaked.\n3. Emit a `bead.reaped` event on the tracker with the full timeline so post-mortems are easy.\n\n## Non-goals\n\n- Per-bead runtime tuning; defaults are enough.\n- Replacing ddx-b808df39 — this is defense-in-depth, not duplication. ddx-b808df39 is operator-initiated; this bead is system-initiated.\n\n## Dependencies\n\n- ddx-b808df39 surfaces PID in `ddx agent workers --json` output; this bead reuses that plumbing.",
+    "acceptance": "- [ ] WorkerManager starts a supervisor goroutine on first worker launch\n- [ ] Watchdog reaps workers past `WatchdogDeadline` with no phase transition in `StallDeadline`\n- [ ] SIGTERM→SIGKILL escalation works on an intentionally-wedged test worker (unit test with a goroutine that ignores ctx)\n- [ ] Reaped worker's bead claim is released (tracker status returns to open or claim-expired)\n- [ ] `bead.reaped` event recorded on tracker with reason=\"watchdog\" and duration\n- [ ] Watchdog deadlines are configurable via ddx server config",
+    "parent": "ddx-0a651925",
+    "labels": [
+      "ddx",
+      "phase:build",
+      "kind:enhancement",
+      "area:agent",
+      "area:workers",
+      "root-cause:defense-in-depth"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T05:48:25Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "execute-loop-heartbeat-at": "2026-04-18T05:48:25.239579219Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T054825-291242ac",
+    "prompt": ".ddx/executions/20260418T054825-291242ac/prompt.md",
+    "manifest": ".ddx/executions/20260418T054825-291242ac/manifest.json",
+    "result": ".ddx/executions/20260418T054825-291242ac/result.json",
+    "checks": ".ddx/executions/20260418T054825-291242ac/checks.json",
+    "usage": ".ddx/executions/20260418T054825-291242ac/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-34d039b7-20260418T054825-291242ac"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T054825-291242ac/result.json b/.ddx/executions/20260418T054825-291242ac/result.json
new file mode 100644
index 00000000..a0246a26
--- /dev/null
+++ b/.ddx/executions/20260418T054825-291242ac/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-34d039b7",
+  "attempt_id": "20260418T054825-291242ac",
+  "base_rev": "a8eb23221eb853c703e290be6ffa8e4e78e6630a",
+  "result_rev": "e3f9bb541339cf9c54d1747d7dfe4a86bf3eaeeb",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6ca39274",
+  "duration_ms": 883205,
+  "tokens": 42199,
+  "cost_usd": 6.290450999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T054825-291242ac",
+  "prompt_file": ".ddx/executions/20260418T054825-291242ac/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T054825-291242ac/manifest.json",
+  "result_file": ".ddx/executions/20260418T054825-291242ac/result.json",
+  "usage_file": ".ddx/executions/20260418T054825-291242ac/usage.json",
+  "started_at": "2026-04-18T05:48:25.619168584Z",
+  "finished_at": "2026-04-18T06:03:08.824349849Z"
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
## Review: .execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-34d039b7 iter 1

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
