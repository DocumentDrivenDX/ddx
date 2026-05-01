<bead-review>
  <bead id="ddx-b808df39" iter=1>
    <title>feat: ddx agent stop/kill command for hung workers</title>
    <description>
## Problem

`ddx agent workers` lists running workers but provides no way to stop them.
Workers can hang indefinitely (today observed: 9 workers running 33h–142h on
agent projects). Operators have no clean way to terminate them — they have to
hunt for the process manually (`ps aux | grep claude`) and `kill -9`, which
bypasses any cleanup the worker might perform (temp files, worktree cleanup,
bead state updates).

Workers tracked in JSON today expose only: id, kind, state, bead_id, harness,
model, started_at, attempts, failures. No PID is surfaced, so external tooling
cannot cleanly target them either.

## Proposed shape

```
ddx agent workers stop &lt;worker-id&gt;        # graceful stop (SIGTERM, then SIGKILL after grace)
ddx agent workers stop --all-over 1h      # stop every running worker older than 1h
ddx agent workers stop --state running    # stop all running workers
ddx agent workers stop --bead &lt;bead-id&gt;   # stop the worker assigned to a bead
```

Graceful path:
1. Send SIGTERM to worker process group
2. Wait configurable grace period (default 30s)
3. Send SIGKILL if still alive
4. Mark worker state as 'stopped' (distinct from 'failed' and 'exited')
5. Run worker cleanup hook (worktree teardown, bead state revert if still claimed)

Also surface PID in `ddx agent workers --json` so external tooling can target
processes directly when the CLI path is unavailable.

## Related

Observed on agent project 2026-04-17: nine workers running 33h–142h with no
bead assigned; root cause unclear. Even if the root cause gets fixed, operators
need a supported way to reap them.

## Files likely touched

- cmd/agent/workers.go — add stop subcommand and pid JSON field
- internal/agent/worker/registry.go — surface pid; add Stop() with graceful-then-force semantics
- internal/agent/worker/process.go — process group signaling, grace timer
- worker state machine — add 'stopping' and 'stopped' states
    </description>
    <acceptance>
`ddx agent workers stop &lt;id&gt;` gracefully terminates a running worker and updates its state to 'stopped'. `--all-over &lt;duration&gt;` reaps all workers older than the threshold. PID is exposed in `--json` output. Worker cleanup hook runs on graceful stop. Test coverage for stop/grace/force paths.
    </acceptance>
    <labels>kind:feature, area:cli, area:agent</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="14417881bb7dc38dea64ac916f4295b970f1c375">
commit 14417881bb7dc38dea64ac916f4295b970f1c375
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 03:13:56 2026 -0400

    chore: add execution evidence [20260418T065645-]

diff --git a/.ddx/executions/20260418T065645-2b07ca41/manifest.json b/.ddx/executions/20260418T065645-2b07ca41/manifest.json
new file mode 100644
index 00000000..0c7805da
--- /dev/null
+++ b/.ddx/executions/20260418T065645-2b07ca41/manifest.json
@@ -0,0 +1,105 @@
+{
+  "attempt_id": "20260418T065645-2b07ca41",
+  "bead_id": "ddx-b808df39",
+  "base_rev": "b11f8eaec45e9f1d48e2cda42d438fd73d2c360b",
+  "created_at": "2026-04-18T06:56:45.466605693Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b808df39",
+    "title": "feat: ddx agent stop/kill command for hung workers",
+    "description": "## Problem\n\n`ddx agent workers` lists running workers but provides no way to stop them.\nWorkers can hang indefinitely (today observed: 9 workers running 33h–142h on\nagent projects). Operators have no clean way to terminate them — they have to\nhunt for the process manually (`ps aux | grep claude`) and `kill -9`, which\nbypasses any cleanup the worker might perform (temp files, worktree cleanup,\nbead state updates).\n\nWorkers tracked in JSON today expose only: id, kind, state, bead_id, harness,\nmodel, started_at, attempts, failures. No PID is surfaced, so external tooling\ncannot cleanly target them either.\n\n## Proposed shape\n\n```\nddx agent workers stop \u003cworker-id\u003e        # graceful stop (SIGTERM, then SIGKILL after grace)\nddx agent workers stop --all-over 1h      # stop every running worker older than 1h\nddx agent workers stop --state running    # stop all running workers\nddx agent workers stop --bead \u003cbead-id\u003e   # stop the worker assigned to a bead\n```\n\nGraceful path:\n1. Send SIGTERM to worker process group\n2. Wait configurable grace period (default 30s)\n3. Send SIGKILL if still alive\n4. Mark worker state as 'stopped' (distinct from 'failed' and 'exited')\n5. Run worker cleanup hook (worktree teardown, bead state revert if still claimed)\n\nAlso surface PID in `ddx agent workers --json` so external tooling can target\nprocesses directly when the CLI path is unavailable.\n\n## Related\n\nObserved on agent project 2026-04-17: nine workers running 33h–142h with no\nbead assigned; root cause unclear. Even if the root cause gets fixed, operators\nneed a supported way to reap them.\n\n## Files likely touched\n\n- cmd/agent/workers.go — add stop subcommand and pid JSON field\n- internal/agent/worker/registry.go — surface pid; add Stop() with graceful-then-force semantics\n- internal/agent/worker/process.go — process group signaling, grace timer\n- worker state machine — add 'stopping' and 'stopped' states",
+    "acceptance": "`ddx agent workers stop \u003cid\u003e` gracefully terminates a running worker and updates its state to 'stopped'. `--all-over \u003cduration\u003e` reaps all workers older than the threshold. PID is exposed in `--json` output. Worker cleanup hook runs on graceful stop. Test coverage for stop/grace/force paths.",
+    "labels": [
+      "kind:feature",
+      "area:cli",
+      "area:agent"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T06:56:45Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"agent\",\"resolved_model\":\"qwen3.5-27b\",\"route_reason\":\"direct-override\",\"fallback_chain\":[],\"base_url\":\"http://vidar:1235/v1\"}",
+          "created_at": "2026-04-18T00:50:01.129009994Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=agent model=qwen3.5-27b reason=direct-override"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=cheap harness=agent model=qwen3.5-27b probe=ok\nagent: provider error: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen3.5-27b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-18T00:50:01.184305406Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"agent\",\"resolved_model\":\"minimax/minimax-m2.7\",\"route_reason\":\"direct-override\",\"fallback_chain\":[],\"base_url\":\"http://vidar:1235/v1\"}",
+          "created_at": "2026-04-18T00:50:09.534003274Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=agent model=minimax/minimax-m2.7 reason=direct-override"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=standard harness=agent model=minimax/minimax-m2.7 probe=ok\nagent: provider error: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'minimax/minimax-m2.7' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-18T00:50:09.588801769Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"agent\",\"resolved_model\":\"minimax/minimax-m2.7\",\"route_reason\":\"direct-override\",\"fallback_chain\":[],\"base_url\":\"http://vidar:1235/v1\"}",
+          "created_at": "2026-04-18T00:50:17.968477264Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=agent model=minimax/minimax-m2.7 reason=direct-override"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness=agent model=minimax/minimax-m2.7 probe=ok\nagent: provider error: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'minimax/minimax-m2.7' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-18T00:50:18.027608848Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen3.5-27b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":20},{\"tier\":\"standard\",\"harness\":\"agent\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":8},{\"tier\":\"smart\",\"harness\":\"agent\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":6}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-18T00:50:18.07249112Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=3 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "escalation exhausted: agent: provider error: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'minimax/minimax-m2.7' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}\ntier=smart\nprobe_result=ok\nresult_rev=01bf5ea5af7dcf4e1d75fd74c6be9f5c7487df04\nbase_rev=01bf5ea5af7dcf4e1d75fd74c6be9f5c7487df04\nretry_after=2026-04-18T06:50:18Z",
+          "created_at": "2026-04-18T00:50:18.194074669Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-18T06:56:45.084449004Z",
+      "execute-loop-last-detail": "escalation exhausted: agent: provider error: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'minimax/minimax-m2.7' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-18T06:50:18Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T065645-2b07ca41",
+    "prompt": ".ddx/executions/20260418T065645-2b07ca41/prompt.md",
+    "manifest": ".ddx/executions/20260418T065645-2b07ca41/manifest.json",
+    "result": ".ddx/executions/20260418T065645-2b07ca41/result.json",
+    "checks": ".ddx/executions/20260418T065645-2b07ca41/checks.json",
+    "usage": ".ddx/executions/20260418T065645-2b07ca41/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b808df39-20260418T065645-2b07ca41"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T065645-2b07ca41/result.json b/.ddx/executions/20260418T065645-2b07ca41/result.json
new file mode 100644
index 00000000..c125e862
--- /dev/null
+++ b/.ddx/executions/20260418T065645-2b07ca41/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b808df39",
+  "attempt_id": "20260418T065645-2b07ca41",
+  "base_rev": "b11f8eaec45e9f1d48e2cda42d438fd73d2c360b",
+  "result_rev": "0add4dcdd399f873783ba581756b7d472ddef2d0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-899ac7be",
+  "duration_ms": 1030540,
+  "tokens": 53705,
+  "cost_usd": 8.455020250000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T065645-2b07ca41",
+  "prompt_file": ".ddx/executions/20260418T065645-2b07ca41/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T065645-2b07ca41/manifest.json",
+  "result_file": ".ddx/executions/20260418T065645-2b07ca41/result.json",
+  "usage_file": ".ddx/executions/20260418T065645-2b07ca41/usage.json",
+  "started_at": "2026-04-18T06:56:45.46698311Z",
+  "finished_at": "2026-04-18T07:13:56.007892947Z"
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
## Review: ddx-b808df39 iter 1

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
