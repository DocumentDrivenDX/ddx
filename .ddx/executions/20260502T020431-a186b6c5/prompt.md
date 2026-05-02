<bead-review>
  <bead id="ddx-31158251" iter=1>
    <title>Create concepts/run-architecture page; retire execute-loop/execute-bead from primary copy</title>
    <description>
Create website/content/docs/concepts/run-architecture.md describing 'ddx run' (single agent invocation), 'ddx try' (bead attempt in worktree), 'ddx work' (queue drain) as one unified architecture per FEAT-010. Update website/content/docs/concepts/architecture.md and homepage hero copy to use this naming. Retain execute-loop / execute-bead in CLI reference only.
    </description>
    <acceptance>
1. website/content/docs/concepts/run-architecture.md exists, explains the three layers, references FEAT-010. 2. concepts/architecture.md uses ddx run/try/work primary names; execute-loop/execute-bead removed from primary text (still allowed in CLI ref pages). 3. Homepage references the three-layer model. 4. 'rg -n "execute-loop|execute-bead" website/content/docs/concepts/ website/content/_index.md website/layouts/index.html' returns no matches in primary copy (excludes cli/commands/).
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5a6ec79e74b9a9120f107343d86f3fc307528401">
commit 5a6ec79e74b9a9120f107343d86f3fc307528401
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:04:29 2026 -0400

    chore: add execution evidence [20260502T020059-]

diff --git a/.ddx/executions/20260502T020059-dfe37a97/manifest.json b/.ddx/executions/20260502T020059-dfe37a97/manifest.json
new file mode 100644
index 00000000..b0c94c6f
--- /dev/null
+++ b/.ddx/executions/20260502T020059-dfe37a97/manifest.json
@@ -0,0 +1,71 @@
+{
+  "attempt_id": "20260502T020059-dfe37a97",
+  "bead_id": "ddx-31158251",
+  "base_rev": "cfaaa6d3289b160ff79645a6e6d009483b9cde18",
+  "created_at": "2026-05-02T02:01:01.423074028Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-31158251",
+    "title": "Create concepts/run-architecture page; retire execute-loop/execute-bead from primary copy",
+    "description": "Create website/content/docs/concepts/run-architecture.md describing 'ddx run' (single agent invocation), 'ddx try' (bead attempt in worktree), 'ddx work' (queue drain) as one unified architecture per FEAT-010. Update website/content/docs/concepts/architecture.md and homepage hero copy to use this naming. Retain execute-loop / execute-bead in CLI reference only.",
+    "acceptance": "1. website/content/docs/concepts/run-architecture.md exists, explains the three layers, references FEAT-010. 2. concepts/architecture.md uses ddx run/try/work primary names; execute-loop/execute-bead removed from primary text (still allowed in CLI ref pages). 3. Homepage references the three-layer model. 4. 'rg -n \"execute-loop|execute-bead\" website/content/docs/concepts/ website/content/_index.md website/layouts/index.html' returns no matches in primary copy (excludes cli/commands/).",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:00:59Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:45.081312394Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:45.215191613Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-05-02T01:36:45.323749529Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-05-02T01:36:45.541347027Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T02:00:59.883557319Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T020059-dfe37a97",
+    "prompt": ".ddx/executions/20260502T020059-dfe37a97/prompt.md",
+    "manifest": ".ddx/executions/20260502T020059-dfe37a97/manifest.json",
+    "result": ".ddx/executions/20260502T020059-dfe37a97/result.json",
+    "checks": ".ddx/executions/20260502T020059-dfe37a97/checks.json",
+    "usage": ".ddx/executions/20260502T020059-dfe37a97/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-31158251-20260502T020059-dfe37a97"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T020059-dfe37a97/result.json b/.ddx/executions/20260502T020059-dfe37a97/result.json
new file mode 100644
index 00000000..9bf313c6
--- /dev/null
+++ b/.ddx/executions/20260502T020059-dfe37a97/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-31158251",
+  "attempt_id": "20260502T020059-dfe37a97",
+  "base_rev": "cfaaa6d3289b160ff79645a6e6d009483b9cde18",
+  "result_rev": "e1a8aa2e0ad681599cc21e7084cefde0b091dc30",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0131a44f",
+  "duration_ms": 204958,
+  "tokens": 13098,
+  "cost_usd": 1.6924672500000004,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T020059-dfe37a97",
+  "prompt_file": ".ddx/executions/20260502T020059-dfe37a97/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T020059-dfe37a97/manifest.json",
+  "result_file": ".ddx/executions/20260502T020059-dfe37a97/result.json",
+  "usage_file": ".ddx/executions/20260502T020059-dfe37a97/usage.json",
+  "started_at": "2026-05-02T02:01:01.423893113Z",
+  "finished_at": "2026-05-02T02:04:26.382065924Z"
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
## Review: ddx-31158251 iter 1

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
