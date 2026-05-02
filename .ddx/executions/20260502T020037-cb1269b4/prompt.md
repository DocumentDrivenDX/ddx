<bead-review>
  <bead id="ddx-c1b1754f" iter=1>
    <title>Author docs/dev/engineering-principles.md with 6 engineering principles</title>
    <description>
Create docs/dev/engineering-principles.md with these 6 internal engineering principles, each with: rule, decision generated, alternative rejected, tradeoff, real DDx feature it shapes. Locked list: 1) Platform Not Methodology, 2) Project-Local by Default, 3) Bounded Context per Attempt, 4) Evidence on Disk, 5) Cheap-Default Escalate on Failure, 6) Reversible Over Ergonomic.
    </description>
    <acceptance>
1. docs/dev/engineering-principles.md exists with all 6 principles. 2. Each has: rule, decision, alternative, tradeoff, DDx feature. 3. File begins with cross-link: 'For user-facing domain principles, see docs/helix/01-frame/principles.md.' 4. Run 'grep -c "^### " docs/dev/engineering-principles.md' returns 6.
    </acceptance>
    <labels>site-redesign, area:docs, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f80a38c2a5d120f78642fcb1fa9aeeaa916b5061">
commit f80a38c2a5d120f78642fcb1fa9aeeaa916b5061
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:00:35 2026 -0400

    chore: add execution evidence [20260502T015931-]

diff --git a/.ddx/executions/20260502T015931-6f3aa3c3/manifest.json b/.ddx/executions/20260502T015931-6f3aa3c3/manifest.json
new file mode 100644
index 00000000..de31c588
--- /dev/null
+++ b/.ddx/executions/20260502T015931-6f3aa3c3/manifest.json
@@ -0,0 +1,71 @@
+{
+  "attempt_id": "20260502T015931-6f3aa3c3",
+  "bead_id": "ddx-c1b1754f",
+  "base_rev": "164c5fdd83616039a8ff170047c97b94321fcea0",
+  "created_at": "2026-05-02T01:59:33.15847653Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c1b1754f",
+    "title": "Author docs/dev/engineering-principles.md with 6 engineering principles",
+    "description": "Create docs/dev/engineering-principles.md with these 6 internal engineering principles, each with: rule, decision generated, alternative rejected, tradeoff, real DDx feature it shapes. Locked list: 1) Platform Not Methodology, 2) Project-Local by Default, 3) Bounded Context per Attempt, 4) Evidence on Disk, 5) Cheap-Default Escalate on Failure, 6) Reversible Over Ergonomic.",
+    "acceptance": "1. docs/dev/engineering-principles.md exists with all 6 principles. 2. Each has: rule, decision, alternative, tradeoff, DDx feature. 3. File begins with cross-link: 'For user-facing domain principles, see docs/helix/01-frame/principles.md.' 4. Run 'grep -c \"^### \" docs/dev/engineering-principles.md' returns 6.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:docs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T01:59:31Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:42.19753207Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:42.326992462Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-05-02T01:36:42.438273624Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-05-02T01:36:42.658228203Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T01:59:31.862001905Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T015931-6f3aa3c3",
+    "prompt": ".ddx/executions/20260502T015931-6f3aa3c3/prompt.md",
+    "manifest": ".ddx/executions/20260502T015931-6f3aa3c3/manifest.json",
+    "result": ".ddx/executions/20260502T015931-6f3aa3c3/result.json",
+    "checks": ".ddx/executions/20260502T015931-6f3aa3c3/checks.json",
+    "usage": ".ddx/executions/20260502T015931-6f3aa3c3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c1b1754f-20260502T015931-6f3aa3c3"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T015931-6f3aa3c3/result.json b/.ddx/executions/20260502T015931-6f3aa3c3/result.json
new file mode 100644
index 00000000..28c1109b
--- /dev/null
+++ b/.ddx/executions/20260502T015931-6f3aa3c3/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c1b1754f",
+  "attempt_id": "20260502T015931-6f3aa3c3",
+  "base_rev": "164c5fdd83616039a8ff170047c97b94321fcea0",
+  "result_rev": "a0314d65f4701c67f4d2533e3caf9023da44045c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-707730e0",
+  "duration_ms": 59071,
+  "tokens": 3191,
+  "cost_usd": 0.2833235,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T015931-6f3aa3c3",
+  "prompt_file": ".ddx/executions/20260502T015931-6f3aa3c3/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T015931-6f3aa3c3/manifest.json",
+  "result_file": ".ddx/executions/20260502T015931-6f3aa3c3/result.json",
+  "usage_file": ".ddx/executions/20260502T015931-6f3aa3c3/usage.json",
+  "started_at": "2026-05-02T01:59:33.159286159Z",
+  "finished_at": "2026-05-02T02:00:32.231076522Z"
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
## Review: ddx-c1b1754f iter 1

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
