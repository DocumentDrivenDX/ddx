<bead-review>
  <bead id="ddx-f6002d1a" iter=1>
    <title>agent workers: stale 'running' entries can't be cleaned without server restart</title>
    <description>
When a local execute-bead/execute-loop process exits abnormally (killed, OOM, parent shell terminated, container crash) the worker entry can remain in the in-memory registry forever. `ddx agent workers` lists it as state=running with elapsed_ms continuously advancing, even though no PID exists.

Reproducing today (axon repo): worker local-20260429T030828-075a28c5 (bead hx-5f4d4ea1, harness=agent, model=qwen3.6) started at 03:08 UTC, has logged elapsed=10+ hours, and `ps -ef | grep execute-bead` shows nothing. The git worktree at /tmp/ddx-exec-wt/.execute-bead-wt-hx-5f4d4ea1-* has only the checkpoint commit (no actual work commits), confirming the worker died early.

Available paths to clean up are all unsatisfying:
1. `ddx agent workers stop --bead hx-5f4d4ea1` returns 'no matching workers' (because the matcher only finds live workers)
2. `ddx agent workers stop &lt;worker-id&gt;` returns server error 400 'worker not running' (same contradiction — the registry says running, the stopper says not running)
3. `systemctl --user restart ddx-server` works but kills every other active worker on the box, including ones for other repos that are doing real work
4. Manual edit of server-state.json risks corruption and isn't documented

The bead itself wasn't blocked (status=open, owner=null) so the registry noise is mostly cosmetic — it pollutes `ddx agent workers` output and confuses operators trying to triage the queue. But over weeks of use these accumulate (saw 14+ stale entries from prior sessions on this same box, with ages 134h–268h).
    </description>
    <acceptance>
AC1. `ddx agent workers prune` (or equivalent) exists and reaps registry entries whose recorded PID is no longer alive (or whose started_at is older than a configurable threshold). Output lists each reaped worker with id, bead_id, harness, age.

AC2. `ddx agent workers stop --bead &lt;id&gt;` succeeds against a stale registry entry too — it should release the registry slot even when the underlying process has already exited. Current 'no matching workers' / 'worker not running' contradiction must be resolved.

AC3. The local-worker registry persists or is reconstructed in a way that survives ddx-server restarts cleanly: either persisted with PID liveness check on restart, or rebuilt from execute-bead/execute-loop on-disk evidence (.ddx/executions/&lt;id&gt;/result.json).

AC4. `ddx agent workers --json` includes a 'liveness' or 'pid_alive' field per entry so operators can see at a glance which entries are live vs stale, even before pruning.
    </acceptance>
    <labels>area:agent, area:workers, kind:operability</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T175957-601d3ad4/manifest.json</file>
    <file>.ddx/executions/20260429T175957-601d3ad4/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="07ae8022f9f55ba3fecc2378ae28e2e0eabf0cad">
diff --git a/.ddx/executions/20260429T175957-601d3ad4/result.json b/.ddx/executions/20260429T175957-601d3ad4/result.json
new file mode 100644
index 00000000..710922d4
--- /dev/null
+++ b/.ddx/executions/20260429T175957-601d3ad4/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-f6002d1a",
+  "attempt_id": "20260429T175957-601d3ad4",
+  "base_rev": "04871bfb9a1be51beb7be9576224fe69fd2e4df4",
+  "result_rev": "e6aff1f674ea4f55944393b5507c8d64d63a1856",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-1050527a",
+  "duration_ms": 979503,
+  "tokens": 44836,
+  "cost_usd": 3.025082300000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T175957-601d3ad4",
+  "prompt_file": ".ddx/executions/20260429T175957-601d3ad4/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T175957-601d3ad4/manifest.json",
+  "result_file": ".ddx/executions/20260429T175957-601d3ad4/result.json",
+  "usage_file": ".ddx/executions/20260429T175957-601d3ad4/usage.json",
+  "started_at": "2026-04-29T17:59:58.482573824Z",
+  "finished_at": "2026-04-29T18:16:17.985585188Z"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T175957-601d3ad4/manifest.json b/.ddx/executions/20260429T175957-601d3ad4/manifest.json
new file mode 100644
index 00000000..39987600
--- /dev/null
+++ b/.ddx/executions/20260429T175957-601d3ad4/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260429T175957-601d3ad4",
+  "bead_id": "ddx-f6002d1a",
+  "base_rev": "04871bfb9a1be51beb7be9576224fe69fd2e4df4",
+  "created_at": "2026-04-29T17:59:58.482116283Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-f6002d1a",
+    "title": "agent workers: stale 'running' entries can't be cleaned without server restart",
+    "description": "When a local execute-bead/execute-loop process exits abnormally (killed, OOM, parent shell terminated, container crash) the worker entry can remain in the in-memory registry forever. `ddx agent workers` lists it as state=running with elapsed_ms continuously advancing, even though no PID exists.\n\nReproducing today (axon repo): worker local-20260429T030828-075a28c5 (bead hx-5f4d4ea1, harness=agent, model=qwen3.6) started at 03:08 UTC, has logged elapsed=10+ hours, and `ps -ef | grep execute-bead` shows nothing. The git worktree at /tmp/ddx-exec-wt/.execute-bead-wt-hx-5f4d4ea1-* has only the checkpoint commit (no actual work commits), confirming the worker died early.\n\nAvailable paths to clean up are all unsatisfying:\n1. `ddx agent workers stop --bead hx-5f4d4ea1` returns 'no matching workers' (because the matcher only finds live workers)\n2. `ddx agent workers stop \u003cworker-id\u003e` returns server error 400 'worker not running' (same contradiction — the registry says running, the stopper says not running)\n3. `systemctl --user restart ddx-server` works but kills every other active worker on the box, including ones for other repos that are doing real work\n4. Manual edit of server-state.json risks corruption and isn't documented\n\nThe bead itself wasn't blocked (status=open, owner=null) so the registry noise is mostly cosmetic — it pollutes `ddx agent workers` output and confuses operators trying to triage the queue. But over weeks of use these accumulate (saw 14+ stale entries from prior sessions on this same box, with ages 134h–268h).",
+    "acceptance": "AC1. `ddx agent workers prune` (or equivalent) exists and reaps registry entries whose recorded PID is no longer alive (or whose started_at is older than a configurable threshold). Output lists each reaped worker with id, bead_id, harness, age.\n\nAC2. `ddx agent workers stop --bead \u003cid\u003e` succeeds against a stale registry entry too — it should release the registry slot even when the underlying process has already exited. Current 'no matching workers' / 'worker not running' contradiction must be resolved.\n\nAC3. The local-worker registry persists or is reconstructed in a way that survives ddx-server restarts cleanly: either persisted with PID liveness check on restart, or rebuilt from execute-bead/execute-loop on-disk evidence (.ddx/executions/\u003cid\u003e/result.json).\n\nAC4. `ddx agent workers --json` includes a 'liveness' or 'pid_alive' field per entry so operators can see at a glance which entries are live vs stale, even before pruning.",
+    "labels": [
+      "area:agent",
+      "area:workers",
+      "kind:operability"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T17:59:55Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T17:59:55.615231999Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T175957-601d3ad4",
+    "prompt": ".ddx/executions/20260429T175957-601d3ad4/prompt.md",
+    "manifest": ".ddx/executions/20260429T175957-601d3ad4/manifest.json",
+    "result": ".ddx/executions/20260429T175957-601d3ad4/result.json",
+    "checks": ".ddx/executions/20260429T175957-601d3ad4/checks.json",
+    "usage": ".ddx/executions/20260429T175957-601d3ad4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-f6002d1a-20260429T175957-601d3ad4"
+  }
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
