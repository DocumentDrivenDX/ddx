<bead-review>
  <bead id=".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b0562380" iter=1>
    <title>execute-bead: bound compaction-stuck breaker by wall-clock time</title>
    <description>
## Root cause RC3 from ddx-0a651925

`cli/internal/agent/agent_runner.go:218-241` trips at `stuckThreshold = maxConsecutiveCompactionFailures` (50 by default). Observed on bundle `20260416T230316-efbf91c5`: the breaker took **2h 8m** to fire (7,720,908 ms). If each iteration takes ~3 min to emit a no-op compaction event, 50 events = ~2.5h. The counter is useful but fires far too late to be a hang-guard.

## Scope

Either:
1. **Time-bounded** — fire after N no-op events **within M minutes** (e.g. 10 within 15m), OR
2. **Threshold lowered** — drop default from 50 to ~10 (only for agent harness where one iteration is already expensive), OR
3. **Wall-clock guard layered on top** — independent breaker fires after e.g. 20 min of consecutive no-op compaction events regardless of count.

Recommend option 1 or 3: time-based thresholds are more robust to provider-speed variance.

## Non-goals

- Changing the compaction algorithm itself.
- Extending breakers outside the compaction path.
    </description>
    <acceptance>
- [ ] Compaction-stuck detection fires within 15 minutes when no-op compaction events are the only activity, regardless of event rate
- [ ] No regression on the existing behaviour for healthy agents (normal runs do not trip)
- [ ] Unit test: fake agentlib emits one no-op compaction event every 3 seconds → breaker fires within 15 min
- [ ] Result.json `detail` clearly reports the time-based breaker when it fires
    </acceptance>
    <labels>ddx, phase:build, kind:enhancement, area:agent, root-cause:rc3</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7c69dc3ec37cdc89ddbaf2ce961001d451a32304">
commit 7c69dc3ec37cdc89ddbaf2ce961001d451a32304
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sun Apr 19 18:56:21 2026 -0400

    chore: add execution evidence [20260419T224036-]

diff --git a/.ddx/executions/20260419T224036-113a5ba3/manifest.json b/.ddx/executions/20260419T224036-113a5ba3/manifest.json
new file mode 100644
index 00000000..28ca9210
--- /dev/null
+++ b/.ddx/executions/20260419T224036-113a5ba3/manifest.json
@@ -0,0 +1,58 @@
+{
+  "attempt_id": "20260419T224036-113a5ba3",
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b0562380",
+  "base_rev": "8ac5ead13ebbbd653f1bc6a4f8056f9c7ff995e6",
+  "created_at": "2026-04-19T22:40:36.366181178Z",
+  "requested": {
+    "harness": "codex",
+    "model": "gpt-5.4",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b0562380",
+    "title": "execute-bead: bound compaction-stuck breaker by wall-clock time",
+    "description": "## Root cause RC3 from ddx-0a651925\n\n`cli/internal/agent/agent_runner.go:218-241` trips at `stuckThreshold = maxConsecutiveCompactionFailures` (50 by default). Observed on bundle `20260416T230316-efbf91c5`: the breaker took **2h 8m** to fire (7,720,908 ms). If each iteration takes ~3 min to emit a no-op compaction event, 50 events = ~2.5h. The counter is useful but fires far too late to be a hang-guard.\n\n## Scope\n\nEither:\n1. **Time-bounded** — fire after N no-op events **within M minutes** (e.g. 10 within 15m), OR\n2. **Threshold lowered** — drop default from 50 to ~10 (only for agent harness where one iteration is already expensive), OR\n3. **Wall-clock guard layered on top** — independent breaker fires after e.g. 20 min of consecutive no-op compaction events regardless of count.\n\nRecommend option 1 or 3: time-based thresholds are more robust to provider-speed variance.\n\n## Non-goals\n\n- Changing the compaction algorithm itself.\n- Extending breakers outside the compaction path.",
+    "acceptance": "- [ ] Compaction-stuck detection fires within 15 minutes when no-op compaction events are the only activity, regardless of event rate\n- [ ] No regression on the existing behaviour for healthy agents (normal runs do not trip)\n- [ ] Unit test: fake agentlib emits one no-op compaction event every 3 seconds → breaker fires within 15 min\n- [ ] Result.json `detail` clearly reports the time-based breaker when it fires",
+    "parent": "ddx-0a651925",
+    "labels": [
+      "ddx",
+      "phase:build",
+      "kind:enhancement",
+      "area:agent",
+      "root-cause:rc3"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-19T22:40:19Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "721226",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"agent\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-19T22:40:28.230027915Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=agent model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nmodel \"qwen3.5-27b\" is not in the catalog and no discovered provider serves it",
+          "created_at": "2026-04-19T22:40:28.393590348Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-19T22:40:19.931211262Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260419T224036-113a5ba3",
+    "prompt": ".ddx/executions/20260419T224036-113a5ba3/prompt.md",
+    "manifest": ".ddx/executions/20260419T224036-113a5ba3/manifest.json",
+    "result": ".ddx/executions/20260419T224036-113a5ba3/result.json",
+    "checks": ".ddx/executions/20260419T224036-113a5ba3/checks.json",
+    "usage": ".ddx/executions/20260419T224036-113a5ba3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b0562380-20260419T224036-113a5ba3"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260419T224036-113a5ba3/result.json b/.ddx/executions/20260419T224036-113a5ba3/result.json
new file mode 100644
index 00000000..4457020a
--- /dev/null
+++ b/.ddx/executions/20260419T224036-113a5ba3/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": ".execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b0562380",
+  "attempt_id": "20260419T224036-113a5ba3",
+  "base_rev": "8ac5ead13ebbbd653f1bc6a4f8056f9c7ff995e6",
+  "result_rev": "da35065a9647bf31d79ec7d4f3b1948399889f96",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4",
+  "session_id": "eb-e18e2206",
+  "duration_ms": 944649,
+  "tokens": 10499431,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260419T224036-113a5ba3",
+  "prompt_file": ".ddx/executions/20260419T224036-113a5ba3/prompt.md",
+  "manifest_file": ".ddx/executions/20260419T224036-113a5ba3/manifest.json",
+  "result_file": ".ddx/executions/20260419T224036-113a5ba3/result.json",
+  "usage_file": ".ddx/executions/20260419T224036-113a5ba3/usage.json",
+  "started_at": "2026-04-19T22:40:36.366426511Z",
+  "finished_at": "2026-04-19T22:56:21.01602486Z"
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
## Review: .execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-b0562380 iter 1

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
