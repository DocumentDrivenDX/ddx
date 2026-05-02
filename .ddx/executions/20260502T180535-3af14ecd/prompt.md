<bead-review>
  <bead id="ddx-89bc2db4" iter=1>
    <title>B14.6a: Fan-out client/merger - timeout, max-concurrency, partial-result, per-node errors</title>
    <description>
Implement hub-side fan-out client in cli/internal/federation/. Given a federated query, fan out to all registered active spokes' /graphql endpoints in parallel. Bound by configurable max-concurrency and per-node timeout. Return partial results: collect successful spoke responses + per-node error map; never fail the whole query if one spoke is slow or down. Apply version/capability checks before issuing query (skip incompatible spokes with logged reason). Distinguish stale (badge) vs offline (immediate fan-out failure) status updates. Includes mocked HTTP client for unit tests + chaos tests (slow spoke, dead spoke, mid-response disconnect).
    </description>
    <acceptance>
Fan-out is parallel. Max-concurrency enforced. Per-node timeout enforced; slow spoke does not block others. Partial result returned with per-node error map. Version-incompatible spokes skipped with logged reason. Offline spokes (immediate fan-out failure) marked offline; stale spokes (missed heartbeat) marked stale — distinct paths. Unit + chaos tests cover all above.
    </acceptance>
    <labels>phase:2, story:14</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0007ffe022c83fb6ff4118462471807b48bdac00">
commit 0007ffe022c83fb6ff4118462471807b48bdac00
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 14:05:33 2026 -0400

    chore: add execution evidence [20260502T180110-]

diff --git a/.ddx/executions/20260502T180110-a217e7e4/manifest.json b/.ddx/executions/20260502T180110-a217e7e4/manifest.json
new file mode 100644
index 00000000..3e56c96e
--- /dev/null
+++ b/.ddx/executions/20260502T180110-a217e7e4/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260502T180110-a217e7e4",
+  "bead_id": "ddx-89bc2db4",
+  "base_rev": "2bcfd1c6f812de5e7b13df4bf49defd8584aebab",
+  "created_at": "2026-05-02T18:01:11.839469827Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-89bc2db4",
+    "title": "B14.6a: Fan-out client/merger - timeout, max-concurrency, partial-result, per-node errors",
+    "description": "Implement hub-side fan-out client in cli/internal/federation/. Given a federated query, fan out to all registered active spokes' /graphql endpoints in parallel. Bound by configurable max-concurrency and per-node timeout. Return partial results: collect successful spoke responses + per-node error map; never fail the whole query if one spoke is slow or down. Apply version/capability checks before issuing query (skip incompatible spokes with logged reason). Distinguish stale (badge) vs offline (immediate fan-out failure) status updates. Includes mocked HTTP client for unit tests + chaos tests (slow spoke, dead spoke, mid-response disconnect).",
+    "acceptance": "Fan-out is parallel. Max-concurrency enforced. Per-node timeout enforced; slow spoke does not block others. Partial result returned with per-node error map. Version-incompatible spokes skipped with logged reason. Offline spokes (immediate fan-out failure) marked offline; stale spokes (missed heartbeat) marked stale — distinct paths. Unit + chaos tests cover all above.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T18:01:10Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3037698",
+      "execute-loop-heartbeat-at": "2026-05-02T18:01:10.465953311Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T180110-a217e7e4",
+    "prompt": ".ddx/executions/20260502T180110-a217e7e4/prompt.md",
+    "manifest": ".ddx/executions/20260502T180110-a217e7e4/manifest.json",
+    "result": ".ddx/executions/20260502T180110-a217e7e4/result.json",
+    "checks": ".ddx/executions/20260502T180110-a217e7e4/checks.json",
+    "usage": ".ddx/executions/20260502T180110-a217e7e4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-89bc2db4-20260502T180110-a217e7e4"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T180110-a217e7e4/result.json b/.ddx/executions/20260502T180110-a217e7e4/result.json
new file mode 100644
index 00000000..9ea3c9f6
--- /dev/null
+++ b/.ddx/executions/20260502T180110-a217e7e4/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-89bc2db4",
+  "attempt_id": "20260502T180110-a217e7e4",
+  "base_rev": "2bcfd1c6f812de5e7b13df4bf49defd8584aebab",
+  "result_rev": "ef7efd469cbecf8d02b9d5643516cb394962b7a3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-50cfee63",
+  "duration_ms": 259780,
+  "tokens": 19720,
+  "cost_usd": 1.6259852500000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T180110-a217e7e4",
+  "prompt_file": ".ddx/executions/20260502T180110-a217e7e4/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T180110-a217e7e4/manifest.json",
+  "result_file": ".ddx/executions/20260502T180110-a217e7e4/result.json",
+  "usage_file": ".ddx/executions/20260502T180110-a217e7e4/usage.json",
+  "started_at": "2026-05-02T18:01:11.839720244Z",
+  "finished_at": "2026-05-02T18:05:31.620064288Z"
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
## Review: ddx-89bc2db4 iter 1

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
