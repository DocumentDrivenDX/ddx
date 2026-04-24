<bead-review>
  <bead id="ddx-a730058a" iter=1>
    <title>execute-bead: write live run-state file during execution for operator observability</title>
    <description>
During ddx agent execute-bead and ddx agent execute-loop, write a .ddx/run-state.json file at execution start and remove it on completion. This file records the currently-executing bead ID, attempt ID, harness, start time, and execution worktree path so operators and HELIX can observe what is currently running without polling the bead tracker.

This capability is implied by CONTRACT-001 Section 5 (Always-on runtime metrics and provenance) but is currently absent from the DDx source. The post-execution evidence bundle (.ddx/executions/) captures historical facts, but there is no live-state signal during execution.

## Scope

- On execute-bead start: write .ddx/run-state.json with fields: bead_id, attempt_id, harness, model, started_at (RFC3339), worktree_path
- On execute-bead end (success or failure): remove .ddx/run-state.json (or overwrite with status=idle)
- execute-loop may write N run-state entries or a single active one depending on parallelism model
- File must be written atomically (write tmp + rename) to avoid partial-read races

## Contract reference

CONTRACT-001 Section 5: Always-on runtime metrics and provenance — per-attempt capture of harness, model, session ID, elapsed, tokens, cost, base revision, result revision.
    </description>
    <acceptance>
1. .ddx/run-state.json is written at execute-bead start with required fields (bead_id, attempt_id, harness, model, started_at, worktree_path)
2. .ddx/run-state.json is removed (or set to status=idle) when execute-bead completes normally
3. .ddx/run-state.json is removed on execute-bead error or crash recovery path
4. execute-loop writes/clears run-state correctly for sequential bead execution
5. Unit test: write/read/cleanup cycle passes
6. Integration test: crashed worker leaves stale run-state that orphan-recovery cleans up
    </acceptance>
    <labels>area:cli, area:agent, kind:feat, phase:build, contract:CONTRACT-001</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fca9ba16c07df6b6676d33db1ea5cceaf63be717">
commit fca9ba16c07df6b6676d33db1ea5cceaf63be717
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 23:35:55 2026 -0400

    chore: add execution evidence [20260424T032813-]

diff --git a/.ddx/executions/20260424T032813-9a7b8808/manifest.json b/.ddx/executions/20260424T032813-9a7b8808/manifest.json
new file mode 100644
index 00000000..d5c849ba
--- /dev/null
+++ b/.ddx/executions/20260424T032813-9a7b8808/manifest.json
@@ -0,0 +1,64 @@
+{
+  "attempt_id": "20260424T032813-9a7b8808",
+  "bead_id": "ddx-a730058a",
+  "base_rev": "a7427caea9d89204db602b1697603af17fc885e6",
+  "created_at": "2026-04-24T03:28:13.874351382Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a730058a",
+    "title": "execute-bead: write live run-state file during execution for operator observability",
+    "description": "During ddx agent execute-bead and ddx agent execute-loop, write a .ddx/run-state.json file at execution start and remove it on completion. This file records the currently-executing bead ID, attempt ID, harness, start time, and execution worktree path so operators and HELIX can observe what is currently running without polling the bead tracker.\n\nThis capability is implied by CONTRACT-001 Section 5 (Always-on runtime metrics and provenance) but is currently absent from the DDx source. The post-execution evidence bundle (.ddx/executions/) captures historical facts, but there is no live-state signal during execution.\n\n## Scope\n\n- On execute-bead start: write .ddx/run-state.json with fields: bead_id, attempt_id, harness, model, started_at (RFC3339), worktree_path\n- On execute-bead end (success or failure): remove .ddx/run-state.json (or overwrite with status=idle)\n- execute-loop may write N run-state entries or a single active one depending on parallelism model\n- File must be written atomically (write tmp + rename) to avoid partial-read races\n\n## Contract reference\n\nCONTRACT-001 Section 5: Always-on runtime metrics and provenance — per-attempt capture of harness, model, session ID, elapsed, tokens, cost, base revision, result revision.",
+    "acceptance": "1. .ddx/run-state.json is written at execute-bead start with required fields (bead_id, attempt_id, harness, model, started_at, worktree_path)\n2. .ddx/run-state.json is removed (or set to status=idle) when execute-bead completes normally\n3. .ddx/run-state.json is removed on execute-bead error or crash recovery path\n4. execute-loop writes/clears run-state correctly for sequential bead execution\n5. Unit test: write/read/cleanup cycle passes\n6. Integration test: crashed worker leaves stale run-state that orphan-recovery cleans up",
+    "labels": [
+      "area:cli",
+      "area:agent",
+      "kind:feat",
+      "phase:build",
+      "contract:CONTRACT-001"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T03:28:13Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T06:28:52.192494375Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T06:29:56.673180331Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T07:19:46.987407176Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T03:28:13.354703228Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T032813-9a7b8808",
+    "prompt": ".ddx/executions/20260424T032813-9a7b8808/prompt.md",
+    "manifest": ".ddx/executions/20260424T032813-9a7b8808/manifest.json",
+    "result": ".ddx/executions/20260424T032813-9a7b8808/result.json",
+    "checks": ".ddx/executions/20260424T032813-9a7b8808/checks.json",
+    "usage": ".ddx/executions/20260424T032813-9a7b8808/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a730058a-20260424T032813-9a7b8808"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T032813-9a7b8808/result.json b/.ddx/executions/20260424T032813-9a7b8808/result.json
new file mode 100644
index 00000000..48fdbda5
--- /dev/null
+++ b/.ddx/executions/20260424T032813-9a7b8808/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a730058a",
+  "attempt_id": "20260424T032813-9a7b8808",
+  "base_rev": "a7427caea9d89204db602b1697603af17fc885e6",
+  "result_rev": "588f1ae3a105359a9859f91cf2d1030fb56d210c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d1c4b69f",
+  "duration_ms": 460442,
+  "tokens": 16097,
+  "cost_usd": 2.0900617499999994,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T032813-9a7b8808",
+  "prompt_file": ".ddx/executions/20260424T032813-9a7b8808/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T032813-9a7b8808/manifest.json",
+  "result_file": ".ddx/executions/20260424T032813-9a7b8808/result.json",
+  "usage_file": ".ddx/executions/20260424T032813-9a7b8808/usage.json",
+  "started_at": "2026-04-24T03:28:13.874600923Z",
+  "finished_at": "2026-04-24T03:35:54.317300521Z"
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
## Review: ddx-a730058a iter 1

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
