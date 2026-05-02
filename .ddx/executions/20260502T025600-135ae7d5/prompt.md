<bead-review>
  <bead id="ddx-067caa3e" iter=1>
    <title>Cross-reference engineering and domain principle docs</title>
    <description>
Add reciprocal cross-references: docs/dev/engineering-principles.md mentions 'For user-facing domain principles see docs/helix/01-frame/principles.md'; docs/helix/01-frame/principles.md preface mentions 'For DDx-internal architecture decisions, see docs/dev/engineering-principles.md.' These should be bidirectional and visible at the top of each file.
    </description>
    <acceptance>
1. Both files have prominent cross-link to the other. 2. 'rg -n "engineering-principles" docs/helix/01-frame/principles.md' returns 1+. 3. 'rg -n "01-frame/principles" docs/dev/engineering-principles.md' returns 1+.
    </acceptance>
    <labels>site-redesign, area:docs, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2376ea5825ae05e7294066f16f56af08582a5d6e">
commit 2376ea5825ae05e7294066f16f56af08582a5d6e
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:55:58 2026 -0400

    chore: add execution evidence [20260502T025518-]

diff --git a/.ddx/executions/20260502T025518-7b07a008/manifest.json b/.ddx/executions/20260502T025518-7b07a008/manifest.json
new file mode 100644
index 00000000..8374336e
--- /dev/null
+++ b/.ddx/executions/20260502T025518-7b07a008/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T025518-7b07a008",
+  "bead_id": "ddx-067caa3e",
+  "base_rev": "c9430d3e85598957b78b3350a9bce69bbfc8413a",
+  "created_at": "2026-05-02T02:55:19.873559805Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-067caa3e",
+    "title": "Cross-reference engineering and domain principle docs",
+    "description": "Add reciprocal cross-references: docs/dev/engineering-principles.md mentions 'For user-facing domain principles see docs/helix/01-frame/principles.md'; docs/helix/01-frame/principles.md preface mentions 'For DDx-internal architecture decisions, see docs/dev/engineering-principles.md.' These should be bidirectional and visible at the top of each file.",
+    "acceptance": "1. Both files have prominent cross-link to the other. 2. 'rg -n \"engineering-principles\" docs/helix/01-frame/principles.md' returns 1+. 3. 'rg -n \"01-frame/principles\" docs/dev/engineering-principles.md' returns 1+.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:docs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:55:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:55:18.171692191Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T025518-7b07a008",
+    "prompt": ".ddx/executions/20260502T025518-7b07a008/prompt.md",
+    "manifest": ".ddx/executions/20260502T025518-7b07a008/manifest.json",
+    "result": ".ddx/executions/20260502T025518-7b07a008/result.json",
+    "checks": ".ddx/executions/20260502T025518-7b07a008/checks.json",
+    "usage": ".ddx/executions/20260502T025518-7b07a008/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-067caa3e-20260502T025518-7b07a008"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T025518-7b07a008/result.json b/.ddx/executions/20260502T025518-7b07a008/result.json
new file mode 100644
index 00000000..38286395
--- /dev/null
+++ b/.ddx/executions/20260502T025518-7b07a008/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-067caa3e",
+  "attempt_id": "20260502T025518-7b07a008",
+  "base_rev": "c9430d3e85598957b78b3350a9bce69bbfc8413a",
+  "result_rev": "6dd6b65152fb0dc8595bc881d6f4548fae42b547",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c1840e4a",
+  "duration_ms": 35570,
+  "tokens": 1919,
+  "cost_usd": 0.41671149999999996,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T025518-7b07a008",
+  "prompt_file": ".ddx/executions/20260502T025518-7b07a008/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T025518-7b07a008/manifest.json",
+  "result_file": ".ddx/executions/20260502T025518-7b07a008/result.json",
+  "usage_file": ".ddx/executions/20260502T025518-7b07a008/usage.json",
+  "started_at": "2026-05-02T02:55:19.873980805Z",
+  "finished_at": "2026-05-02T02:55:55.444018872Z"
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
## Review: ddx-067caa3e iter 1

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
