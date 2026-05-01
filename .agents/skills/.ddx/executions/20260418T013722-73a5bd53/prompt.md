<bead-review>
  <bead id="ddx-b3e942d6" iter=1>
    <title>Apply metric ratchets before DDx lands managed bead attempts</title>
    <description>
CONTRACT-001 claims DDx owns metric ratchet evaluation sufficient to decide merge vs preserve for managed execution, but DDx today stores metric definitions/runs and comparison metadata without feeding ratchet outcomes into execute-bead/execute-loop landing decisions. HELIX should not need its own parallel ratchet gate when DDx owns the managed execution substrate.

Add a DDx-managed ratchet gate that can preserve failing attempts before merge and emit deterministic evidence.
    </description>
    <acceptance>
- [ ] Managed bead landing can resolve declared metric ratchet/threshold requirements from DDx execution definitions or governing graph docs before merge
- [ ] A ratchet miss preserves the attempt and records machine-readable evidence naming the metric, baseline/threshold, observed value, and decision
- [ ] A ratchet pass records machine-readable evidence and allows landing
- [ ] Result/report surfaces expose enough structured data for HELIX to distinguish ratchet-preserved outcomes from generic execution failures
- [ ] Deterministic tests cover pass and fail cases without live external dependencies
    </acceptance>
    <labels>ddx, area:exec, area:metric, area:agent</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ef06678bb119270ff4a19921b21806fee3d4dd0e">
commit ef06678bb119270ff4a19921b21806fee3d4dd0e
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 17 21:37:21 2026 -0400

    chore: add execution evidence [20260418T011848-]

diff --git a/.ddx/executions/20260418T011848-50b8eb2d/manifest.json b/.ddx/executions/20260418T011848-50b8eb2d/manifest.json
new file mode 100644
index 00000000..70ed143b
--- /dev/null
+++ b/.ddx/executions/20260418T011848-50b8eb2d/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260418T011848-50b8eb2d",
+  "bead_id": "ddx-b3e942d6",
+  "base_rev": "582a83f4a71f78406bca140c1225c5fbbd83fe3b",
+  "created_at": "2026-04-18T01:18:48.919076384Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b3e942d6",
+    "title": "Apply metric ratchets before DDx lands managed bead attempts",
+    "description": "CONTRACT-001 claims DDx owns metric ratchet evaluation sufficient to decide merge vs preserve for managed execution, but DDx today stores metric definitions/runs and comparison metadata without feeding ratchet outcomes into execute-bead/execute-loop landing decisions. HELIX should not need its own parallel ratchet gate when DDx owns the managed execution substrate.\n\nAdd a DDx-managed ratchet gate that can preserve failing attempts before merge and emit deterministic evidence.",
+    "acceptance": "- [ ] Managed bead landing can resolve declared metric ratchet/threshold requirements from DDx execution definitions or governing graph docs before merge\n- [ ] A ratchet miss preserves the attempt and records machine-readable evidence naming the metric, baseline/threshold, observed value, and decision\n- [ ] A ratchet pass records machine-readable evidence and allows landing\n- [ ] Result/report surfaces expose enough structured data for HELIX to distinguish ratchet-preserved outcomes from generic execution failures\n- [ ] Deterministic tests cover pass and fail cases without live external dependencies",
+    "labels": [
+      "ddx",
+      "area:exec",
+      "area:metric",
+      "area:agent"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T01:18:48Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1588886",
+      "execute-loop-heartbeat-at": "2026-04-18T01:18:48.626336038Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T011848-50b8eb2d",
+    "prompt": ".ddx/executions/20260418T011848-50b8eb2d/prompt.md",
+    "manifest": ".ddx/executions/20260418T011848-50b8eb2d/manifest.json",
+    "result": ".ddx/executions/20260418T011848-50b8eb2d/result.json",
+    "checks": ".ddx/executions/20260418T011848-50b8eb2d/checks.json",
+    "usage": ".ddx/executions/20260418T011848-50b8eb2d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b3e942d6-20260418T011848-50b8eb2d"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T011848-50b8eb2d/result.json b/.ddx/executions/20260418T011848-50b8eb2d/result.json
new file mode 100644
index 00000000..6780181c
--- /dev/null
+++ b/.ddx/executions/20260418T011848-50b8eb2d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b3e942d6",
+  "attempt_id": "20260418T011848-50b8eb2d",
+  "base_rev": "582a83f4a71f78406bca140c1225c5fbbd83fe3b",
+  "result_rev": "85fa53c0b1b074270eb413757f2c789edda4dbed",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-096a62b7",
+  "duration_ms": 1111483,
+  "tokens": 54799,
+  "cost_usd": 8.832884250000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T011848-50b8eb2d",
+  "prompt_file": ".ddx/executions/20260418T011848-50b8eb2d/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T011848-50b8eb2d/manifest.json",
+  "result_file": ".ddx/executions/20260418T011848-50b8eb2d/result.json",
+  "usage_file": ".ddx/executions/20260418T011848-50b8eb2d/usage.json",
+  "started_at": "2026-04-18T01:18:48.919360676Z",
+  "finished_at": "2026-04-18T01:37:20.402763111Z"
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
## Review: ddx-b3e942d6 iter 1

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
