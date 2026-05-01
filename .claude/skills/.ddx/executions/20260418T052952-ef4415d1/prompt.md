<bead-review>
  <bead id=".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b203c889" iter=1>
    <title>execute-bead: add wall-clock deadline alongside idle timeout for agent harness</title>
    <description>
## Root cause RC2 from ddx-0a651925

The `DefaultTimeoutMS = 7200000` (2h) in `cli/internal/agent/types.go:363` is advertised as a hard cap but implemented as an *idle timeout*:

- `cli/internal/agent/executor.go:154-178` resets `time.NewTimer(idleTimeout)` on every stdout/stderr byte.
- `cli/internal/agent/agent_runner.go:264-285` resets on every non-compaction agentlib event.

A provider that emits any heartbeat, retry line, or periodic event defeats the timer indefinitely — enabling the 33–142h hangs observed on agent projects.

Only the claude streaming path (`cli/internal/agent/claude_stream.go:373-384`) uses a true wall-clock deadline via `time.After(timeout)`. That path is why bundle `20260415T015231-c2ffa9ba` fired at exactly 2h 0m 0.098s — and nothing else does.

## Scope

1. Add an absolute wall-clock deadline in addition to (not replacing) the idle timeout.
2. Default the wall-clock bound to something operators can trust — e.g. 3x `DefaultTimeoutMS` or a new `DefaultWallClockMS`.
3. Apply in: `OSExecutor.ExecuteInDir` (subprocess path) and `Runner.RunAgent` (embedded agent path).
4. Reflect the wall-clock error in result.json as a distinct `error: "wall-clock deadline exceeded after Xh"` so it is not confused with idle timeout.

## Non-goals

- Lowering or removing the existing idle timeout.
- Per-provider tuning (can be follow-up).
    </description>
    <acceptance>
- [ ] New default wall-clock deadline (e.g. 3h) applied to both `OSExecutor.ExecuteInDir` and `Runner.RunAgent`
- [ ] Wall-clock timeout fires even when stdout/stderr/agent events are flowing (unit test: fake provider emits 1 event/sec for &gt; deadline, expect cancel at deadline ± 1s)
- [ ] Idle timeout behaviour preserved (unit test: no activity for idle timeout → fire)
- [ ] Result.json distinguishes `timeout: wall-clock` from `timeout: idle`
- [ ] Documented in types.go (DefaultWallClockMS constant with comment)
    </acceptance>
    <labels>ddx, phase:build, kind:bug, area:agent, area:workers, root-cause:rc2</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fff2c86240b98cd687a9dc9879926b17411db3f5">
commit fff2c86240b98cd687a9dc9879926b17411db3f5
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 01:29:51 2026 -0400

    chore: add execution evidence [20260418T051525-]

diff --git a/.ddx/executions/20260418T051525-40ea409f/manifest.json b/.ddx/executions/20260418T051525-40ea409f/manifest.json
new file mode 100644
index 00000000..a50fea6d
--- /dev/null
+++ b/.ddx/executions/20260418T051525-40ea409f/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260418T051525-40ea409f",
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b203c889",
+  "base_rev": "d9c82300aaa4f0c4271f70e9cdc6af30e06ea91c",
+  "created_at": "2026-04-18T05:15:25.889531848Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b203c889",
+    "title": "execute-bead: add wall-clock deadline alongside idle timeout for agent harness",
+    "description": "## Root cause RC2 from ddx-0a651925\n\nThe `DefaultTimeoutMS = 7200000` (2h) in `cli/internal/agent/types.go:363` is advertised as a hard cap but implemented as an *idle timeout*:\n\n- `cli/internal/agent/executor.go:154-178` resets `time.NewTimer(idleTimeout)` on every stdout/stderr byte.\n- `cli/internal/agent/agent_runner.go:264-285` resets on every non-compaction agentlib event.\n\nA provider that emits any heartbeat, retry line, or periodic event defeats the timer indefinitely — enabling the 33–142h hangs observed on agent projects.\n\nOnly the claude streaming path (`cli/internal/agent/claude_stream.go:373-384`) uses a true wall-clock deadline via `time.After(timeout)`. That path is why bundle `20260415T015231-c2ffa9ba` fired at exactly 2h 0m 0.098s — and nothing else does.\n\n## Scope\n\n1. Add an absolute wall-clock deadline in addition to (not replacing) the idle timeout.\n2. Default the wall-clock bound to something operators can trust — e.g. 3x `DefaultTimeoutMS` or a new `DefaultWallClockMS`.\n3. Apply in: `OSExecutor.ExecuteInDir` (subprocess path) and `Runner.RunAgent` (embedded agent path).\n4. Reflect the wall-clock error in result.json as a distinct `error: \"wall-clock deadline exceeded after Xh\"` so it is not confused with idle timeout.\n\n## Non-goals\n\n- Lowering or removing the existing idle timeout.\n- Per-provider tuning (can be follow-up).",
+    "acceptance": "- [ ] New default wall-clock deadline (e.g. 3h) applied to both `OSExecutor.ExecuteInDir` and `Runner.RunAgent`\n- [ ] Wall-clock timeout fires even when stdout/stderr/agent events are flowing (unit test: fake provider emits 1 event/sec for \u003e deadline, expect cancel at deadline ± 1s)\n- [ ] Idle timeout behaviour preserved (unit test: no activity for idle timeout → fire)\n- [ ] Result.json distinguishes `timeout: wall-clock` from `timeout: idle`\n- [ ] Documented in types.go (DefaultWallClockMS constant with comment)",
+    "parent": "ddx-0a651925",
+    "labels": [
+      "ddx",
+      "phase:build",
+      "kind:bug",
+      "area:agent",
+      "area:workers",
+      "root-cause:rc2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T05:15:25Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "execute-loop-heartbeat-at": "2026-04-18T05:15:25.558882127Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T051525-40ea409f",
+    "prompt": ".ddx/executions/20260418T051525-40ea409f/prompt.md",
+    "manifest": ".ddx/executions/20260418T051525-40ea409f/manifest.json",
+    "result": ".ddx/executions/20260418T051525-40ea409f/result.json",
+    "checks": ".ddx/executions/20260418T051525-40ea409f/checks.json",
+    "usage": ".ddx/executions/20260418T051525-40ea409f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b203c889-20260418T051525-40ea409f"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T051525-40ea409f/result.json b/.ddx/executions/20260418T051525-40ea409f/result.json
new file mode 100644
index 00000000..01ea6c1f
--- /dev/null
+++ b/.ddx/executions/20260418T051525-40ea409f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b203c889",
+  "attempt_id": "20260418T051525-40ea409f",
+  "base_rev": "d9c82300aaa4f0c4271f70e9cdc6af30e06ea91c",
+  "result_rev": "cac5768a3e34abac1f61641cb678750a131be1da",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0acc00fd",
+  "duration_ms": 864478,
+  "tokens": 38090,
+  "cost_usd": 5.05552225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T051525-40ea409f",
+  "prompt_file": ".ddx/executions/20260418T051525-40ea409f/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T051525-40ea409f/manifest.json",
+  "result_file": ".ddx/executions/20260418T051525-40ea409f/result.json",
+  "usage_file": ".ddx/executions/20260418T051525-40ea409f/usage.json",
+  "started_at": "2026-04-18T05:15:25.889773015Z",
+  "finished_at": "2026-04-18T05:29:50.368643285Z"
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
## Review: .execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b203c889 iter 1

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
