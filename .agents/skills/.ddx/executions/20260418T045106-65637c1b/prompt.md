<bead-review>
  <bead id="ddx-0a651925" iter=1>
    <title>investigate: root cause of 33-142h hung workers (referenced by ddx-b808df39)</title>
    <description>
## Context

ddx-b808df39 (P0: ddx agent stop/kill command for hung workers) treats hung-worker remediation as an operational need, but does NOT investigate the UNDERLYING cause of the hangs. The feature is a useful escape hatch, but shipping it without understanding WHY workers hang means we'll keep needing the escape hatch.

Opus fresh-eyes review (2026-04-18) flagged: 'Hung workers for 33h–142h are an operational defect independent of root cause' — the feature bead is justified, but a SIBLING root-cause investigation is missing from the portfolio.

## Observed

The referenced 33h and 142h hang durations in ddx-b808df39 imply multiple incidents where an execute-bead worker process remained in-progress past any reasonable upper bound for a single bead. Needs to investigate:

1. Is the worker process alive but stuck in a tool call (bash subprocess that never returns)?
2. Is it alive but mid-LLM-call with no timeout (provider hang + no client-side deadline)?
3. Is it dead but the bead's claimed_at never cleared (tracker-side leak)?
4. Is the worktree locked or fsync'd in a way that prevents cleanup?

## Fix direction

This bead is an INVESTIGATION, not a fix. Deliverable is a short report that:

1. Reproduces or observes at least one hang incident and captures:
   - ps / pgrep state of the worker process tree
   - stack traces via 'go tool pprof' against the worker (if alive)
   - Timeline from .ddx/executions/&lt;attempt&gt;/embedded/*.jsonl (last event type + timestamp)
   - tracker claimed_at and retry_after state

2. Classifies the hang into one of the four categories above (or a new category).

3. Files concrete follow-up beads for each identified root cause:
   - Tool subprocess timeout if bash hangs
   - LLM client deadline if provider hangs
   - Claim expiry / heartbeat if tracker-side leak
   - File-lock / worktree cleanup if fs-side

4. Recommends whether ddx-b808df39's stop/kill mechanism is sufficient as mitigation or needs additional primitives.

## Relation to ddx-b808df39

ddx-b808df39 ships the OPERATOR TOOL to stop a hung worker. This bead finds the BUG that causes hangs. Both are needed; ship ddx-b808df39 first if the operator-tool is more urgent, then this investigation.

## Non-goals

- Fixing the root cause in this bead. Scope is investigation + follow-up bead filing.
- Preventing all possible hangs (impossible). Scope is the actually-observed hang modes.
    </description>
    <acceptance>
1. Written incident report (.md or bead comment body) captures at least one observed hang with: process state, timeline, tracker state, and root-cause classification.

2. Follow-up bead(s) filed for each identified root cause. Each with deterministic AC (e.g. 'add 10m wall-clock timeout on bash tool subprocess execution').

3. This bead closes as 'investigation complete; see child beads for fixes'. ddx-b808df39 references this bead's findings in its implementation notes.
    </acceptance>
    <labels>ddx, phase:build, kind:investigation, area:agent, area:workers</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1d0b3e373a618623062909694332536c74f2630a">
commit 1d0b3e373a618623062909694332536c74f2630a
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 00:51:05 2026 -0400

    chore: add execution evidence [20260418T043148-]

diff --git a/.ddx/executions/20260418T043148-1346e8a3/manifest.json b/.ddx/executions/20260418T043148-1346e8a3/manifest.json
new file mode 100644
index 00000000..3aee6b4f
--- /dev/null
+++ b/.ddx/executions/20260418T043148-1346e8a3/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260418T043148-1346e8a3",
+  "bead_id": "ddx-0a651925",
+  "base_rev": "91e89eaa6fe391ae746d5443567917f54b0e8f20",
+  "created_at": "2026-04-18T04:31:48.392660469Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-0a651925",
+    "title": "investigate: root cause of 33-142h hung workers (referenced by ddx-b808df39)",
+    "description": "## Context\n\nddx-b808df39 (P0: ddx agent stop/kill command for hung workers) treats hung-worker remediation as an operational need, but does NOT investigate the UNDERLYING cause of the hangs. The feature is a useful escape hatch, but shipping it without understanding WHY workers hang means we'll keep needing the escape hatch.\n\nOpus fresh-eyes review (2026-04-18) flagged: 'Hung workers for 33h–142h are an operational defect independent of root cause' — the feature bead is justified, but a SIBLING root-cause investigation is missing from the portfolio.\n\n## Observed\n\nThe referenced 33h and 142h hang durations in ddx-b808df39 imply multiple incidents where an execute-bead worker process remained in-progress past any reasonable upper bound for a single bead. Needs to investigate:\n\n1. Is the worker process alive but stuck in a tool call (bash subprocess that never returns)?\n2. Is it alive but mid-LLM-call with no timeout (provider hang + no client-side deadline)?\n3. Is it dead but the bead's claimed_at never cleared (tracker-side leak)?\n4. Is the worktree locked or fsync'd in a way that prevents cleanup?\n\n## Fix direction\n\nThis bead is an INVESTIGATION, not a fix. Deliverable is a short report that:\n\n1. Reproduces or observes at least one hang incident and captures:\n   - ps / pgrep state of the worker process tree\n   - stack traces via 'go tool pprof' against the worker (if alive)\n   - Timeline from .ddx/executions/\u003cattempt\u003e/embedded/*.jsonl (last event type + timestamp)\n   - tracker claimed_at and retry_after state\n\n2. Classifies the hang into one of the four categories above (or a new category).\n\n3. Files concrete follow-up beads for each identified root cause:\n   - Tool subprocess timeout if bash hangs\n   - LLM client deadline if provider hangs\n   - Claim expiry / heartbeat if tracker-side leak\n   - File-lock / worktree cleanup if fs-side\n\n4. Recommends whether ddx-b808df39's stop/kill mechanism is sufficient as mitigation or needs additional primitives.\n\n## Relation to ddx-b808df39\n\nddx-b808df39 ships the OPERATOR TOOL to stop a hung worker. This bead finds the BUG that causes hangs. Both are needed; ship ddx-b808df39 first if the operator-tool is more urgent, then this investigation.\n\n## Non-goals\n\n- Fixing the root cause in this bead. Scope is investigation + follow-up bead filing.\n- Preventing all possible hangs (impossible). Scope is the actually-observed hang modes.",
+    "acceptance": "1. Written incident report (.md or bead comment body) captures at least one observed hang with: process state, timeline, tracker state, and root-cause classification.\n\n2. Follow-up bead(s) filed for each identified root cause. Each with deterministic AC (e.g. 'add 10m wall-clock timeout on bash tool subprocess execution').\n\n3. This bead closes as 'investigation complete; see child beads for fixes'. ddx-b808df39 references this bead's findings in its implementation notes.",
+    "labels": [
+      "ddx",
+      "phase:build",
+      "kind:investigation",
+      "area:agent",
+      "area:workers"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T04:31:48Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "execute-loop-heartbeat-at": "2026-04-18T04:31:48.036761913Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T043148-1346e8a3",
+    "prompt": ".ddx/executions/20260418T043148-1346e8a3/prompt.md",
+    "manifest": ".ddx/executions/20260418T043148-1346e8a3/manifest.json",
+    "result": ".ddx/executions/20260418T043148-1346e8a3/result.json",
+    "checks": ".ddx/executions/20260418T043148-1346e8a3/checks.json",
+    "usage": ".ddx/executions/20260418T043148-1346e8a3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T043148-1346e8a3/result.json b/.ddx/executions/20260418T043148-1346e8a3/result.json
new file mode 100644
index 00000000..126c5581
--- /dev/null
+++ b/.ddx/executions/20260418T043148-1346e8a3/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-0a651925",
+  "attempt_id": "20260418T043148-1346e8a3",
+  "base_rev": "91e89eaa6fe391ae746d5443567917f54b0e8f20",
+  "result_rev": "13cd213e157cd211c48f51796bd62a2fb8b1fcae",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-15287082",
+  "duration_ms": 1155843,
+  "tokens": 99,
+  "cost_usd": 7.888996,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T043148-1346e8a3",
+  "prompt_file": ".ddx/executions/20260418T043148-1346e8a3/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T043148-1346e8a3/manifest.json",
+  "result_file": ".ddx/executions/20260418T043148-1346e8a3/result.json",
+  "usage_file": ".ddx/executions/20260418T043148-1346e8a3/usage.json",
+  "started_at": "2026-04-18T04:31:48.392948386Z",
+  "finished_at": "2026-04-18T04:51:04.236079367Z"
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
## Review: ddx-0a651925 iter 1

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
