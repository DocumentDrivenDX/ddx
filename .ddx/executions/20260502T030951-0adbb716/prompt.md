<bead-review>
  <bead id="ddx-d532992f" iter=1>
    <title>Investigation/report beads must write evidence to .ddx/executions/, not /tmp</title>
    <description>
B15a (ddx-8d51b137) wrote its investigation report to /tmp/feat-011-status.md. /tmp is outside the repo, so the post-merge review couldn't see the evidence and flagged BLOCK. Manual workaround: commit 7f943dc1 copied the report into .ddx/executions/20260502T020500-79017d32/. Pattern fix: any bead whose AC says 'output a report' or 'document findings' should write under .ddx/executions/&lt;run-id&gt;/ (or another in-repo path) so the evidence survives reboots and is reviewable. Either: (a) update the execute-bead system prompt to instruct agents to use .ddx/executions/&lt;run-id&gt;/ for investigation outputs, or (b) extend the AC template guidance in CLAUDE.md / SKILL.md to require in-repo evidence paths, or (c) both.
    </description>
    <acceptance>
1. execute-bead system prompt (cli/internal/agent/execute_bead.go executeBeadInstructionsClaudeText/AgentText) instructs agents to write investigation/report outputs under .ddx/executions/&lt;run-id&gt;/ rather than /tmp. 2. CLAUDE.md or relevant guidance file documents the convention. 3. A test or demonstration bead with 'output a report at &lt;path&gt;' AC writes to .ddx/executions/ when executed.
    </acceptance>
    <labels>process, area:agent, kind:improvement</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7d27ac89aa634c155f0a9906eb6544e762501863">
commit 7d27ac89aa634c155f0a9906eb6544e762501863
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 23:09:49 2026 -0400

    chore: add execution evidence [20260502T030443-]

diff --git a/.ddx/executions/20260502T030443-9e707444/manifest.json b/.ddx/executions/20260502T030443-9e707444/manifest.json
new file mode 100644
index 00000000..55825fb9
--- /dev/null
+++ b/.ddx/executions/20260502T030443-9e707444/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T030443-9e707444",
+  "bead_id": "ddx-d532992f",
+  "base_rev": "9a8b5b8f83dcbbe644daab24f50e2912d873831e",
+  "created_at": "2026-05-02T03:04:44.948043465Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d532992f",
+    "title": "Investigation/report beads must write evidence to .ddx/executions/, not /tmp",
+    "description": "B15a (ddx-8d51b137) wrote its investigation report to /tmp/feat-011-status.md. /tmp is outside the repo, so the post-merge review couldn't see the evidence and flagged BLOCK. Manual workaround: commit 7f943dc1 copied the report into .ddx/executions/20260502T020500-79017d32/. Pattern fix: any bead whose AC says 'output a report' or 'document findings' should write under .ddx/executions/\u003crun-id\u003e/ (or another in-repo path) so the evidence survives reboots and is reviewable. Either: (a) update the execute-bead system prompt to instruct agents to use .ddx/executions/\u003crun-id\u003e/ for investigation outputs, or (b) extend the AC template guidance in CLAUDE.md / SKILL.md to require in-repo evidence paths, or (c) both.",
+    "acceptance": "1. execute-bead system prompt (cli/internal/agent/execute_bead.go executeBeadInstructionsClaudeText/AgentText) instructs agents to write investigation/report outputs under .ddx/executions/\u003crun-id\u003e/ rather than /tmp. 2. CLAUDE.md or relevant guidance file documents the convention. 3. A test or demonstration bead with 'output a report at \u003cpath\u003e' AC writes to .ddx/executions/ when executed.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "process",
+      "area:agent",
+      "kind:improvement"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T03:04:43Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T03:04:43.355677251Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T030443-9e707444",
+    "prompt": ".ddx/executions/20260502T030443-9e707444/prompt.md",
+    "manifest": ".ddx/executions/20260502T030443-9e707444/manifest.json",
+    "result": ".ddx/executions/20260502T030443-9e707444/result.json",
+    "checks": ".ddx/executions/20260502T030443-9e707444/checks.json",
+    "usage": ".ddx/executions/20260502T030443-9e707444/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d532992f-20260502T030443-9e707444"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T030443-9e707444/result.json b/.ddx/executions/20260502T030443-9e707444/result.json
new file mode 100644
index 00000000..8acbd4b9
--- /dev/null
+++ b/.ddx/executions/20260502T030443-9e707444/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d532992f",
+  "attempt_id": "20260502T030443-9e707444",
+  "base_rev": "9a8b5b8f83dcbbe644daab24f50e2912d873831e",
+  "result_rev": "6473f3fa061fd3d369bf3515417e1115892a631b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-4401eb20",
+  "duration_ms": 302266,
+  "tokens": 9759,
+  "cost_usd": 1.1282507499999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T030443-9e707444",
+  "prompt_file": ".ddx/executions/20260502T030443-9e707444/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T030443-9e707444/manifest.json",
+  "result_file": ".ddx/executions/20260502T030443-9e707444/result.json",
+  "usage_file": ".ddx/executions/20260502T030443-9e707444/usage.json",
+  "started_at": "2026-05-02T03:04:44.948507589Z",
+  "finished_at": "2026-05-02T03:09:47.215224138Z"
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
## Review: ddx-d532992f iter 1

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
