<bead-review>
  <bead id="ddx-653f6ac9" iter=1>
    <title>routing-cleanup: dead-code detector + remove compensating DDx-side routing logic</title>
    <description>
Cover AC #7 from ddx-fdd3ea36. Search DDx for code that re-implements upstream routing concerns: allow-list checks, exact-pin filtering, profile-to-preference mapping, provider cost scoring. Must be absent on the default path. ResolveProfileLadder, ResolveTierModelRef, workersByHarness escalation helpers remain reachable via --escalate only. Add a CI check that catches orphaned compensating helpers in future PRs.
    </description>
    <acceptance>
1. Audit: zero matches in DDx for DDx-local allow-list checks, exact-pin filtering, profile-to-preference mapping, provider cost scoring on the default path. 2. ResolveProfileLadder / ResolveTierModelRef / workersByHarness helpers: still present, only reachable via --escalate. 3. Dead-code detector (e.g. a small tools/lint or unused-symbol scan in CI) runs in CI and fails when any of the listed helpers becomes unreachable from any code path including --escalate. 4. CI step is documented in CONTRIBUTING or docs/dev/.
    </acceptance>
    <labels>feat-006, routing, cleanup</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e6be306b126b02186b00d36e2ac109b0321cb726">
commit e6be306b126b02186b00d36e2ac109b0321cb726
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 21:10:39 2026 -0400

    chore: add execution evidence [20260430T010123-]

diff --git a/.ddx/executions/20260430T010123-325e083a/manifest.json b/.ddx/executions/20260430T010123-325e083a/manifest.json
new file mode 100644
index 00000000..a4568ddd
--- /dev/null
+++ b/.ddx/executions/20260430T010123-325e083a/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260430T010123-325e083a",
+  "bead_id": "ddx-653f6ac9",
+  "base_rev": "3806d295d6ca5f2e1982d01ef479b8e35f04dd2f",
+  "created_at": "2026-04-30T01:01:24.615362553Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-653f6ac9",
+    "title": "routing-cleanup: dead-code detector + remove compensating DDx-side routing logic",
+    "description": "Cover AC #7 from ddx-fdd3ea36. Search DDx for code that re-implements upstream routing concerns: allow-list checks, exact-pin filtering, profile-to-preference mapping, provider cost scoring. Must be absent on the default path. ResolveProfileLadder, ResolveTierModelRef, workersByHarness escalation helpers remain reachable via --escalate only. Add a CI check that catches orphaned compensating helpers in future PRs.",
+    "acceptance": "1. Audit: zero matches in DDx for DDx-local allow-list checks, exact-pin filtering, profile-to-preference mapping, provider cost scoring on the default path. 2. ResolveProfileLadder / ResolveTierModelRef / workersByHarness helpers: still present, only reachable via --escalate. 3. Dead-code detector (e.g. a small tools/lint or unused-symbol scan in CI) runs in CI and fails when any of the listed helpers becomes unreachable from any code path including --escalate. 4. CI step is documented in CONTRIBUTING or docs/dev/.",
+    "parent": "ddx-fdd3ea36",
+    "labels": [
+      "feat-006",
+      "routing",
+      "cleanup"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-30T01:01:23Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2278885",
+      "execute-loop-heartbeat-at": "2026-04-30T01:01:23.807206466Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260430T010123-325e083a",
+    "prompt": ".ddx/executions/20260430T010123-325e083a/prompt.md",
+    "manifest": ".ddx/executions/20260430T010123-325e083a/manifest.json",
+    "result": ".ddx/executions/20260430T010123-325e083a/result.json",
+    "checks": ".ddx/executions/20260430T010123-325e083a/checks.json",
+    "usage": ".ddx/executions/20260430T010123-325e083a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-653f6ac9-20260430T010123-325e083a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260430T010123-325e083a/result.json b/.ddx/executions/20260430T010123-325e083a/result.json
new file mode 100644
index 00000000..728bb818
--- /dev/null
+++ b/.ddx/executions/20260430T010123-325e083a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-653f6ac9",
+  "attempt_id": "20260430T010123-325e083a",
+  "base_rev": "3806d295d6ca5f2e1982d01ef479b8e35f04dd2f",
+  "result_rev": "f345369c3de6145e107d40008e10efa4d037ed25",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-89942bac",
+  "duration_ms": 552707,
+  "tokens": 32973,
+  "cost_usd": 4.8062792499999984,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260430T010123-325e083a",
+  "prompt_file": ".ddx/executions/20260430T010123-325e083a/prompt.md",
+  "manifest_file": ".ddx/executions/20260430T010123-325e083a/manifest.json",
+  "result_file": ".ddx/executions/20260430T010123-325e083a/result.json",
+  "usage_file": ".ddx/executions/20260430T010123-325e083a/usage.json",
+  "started_at": "2026-04-30T01:01:24.615616014Z",
+  "finished_at": "2026-04-30T01:10:37.322846059Z"
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
## Review: ddx-653f6ac9 iter 1

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
