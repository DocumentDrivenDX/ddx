<bead-review>
  <bead id="ddx-2513f08c" iter=1>
    <title>Project home page is orphaned: add it to the sidebar and make the DDx logo a link</title>
    <description>
## Observed

The project home page (`/nodes/.../projects/&lt;id&gt;/`) renders a useful overview — queue summary (ready/blocked/in-progress) plus three action cards (drain queue, re-align specs, run checks). User feedback: "I see it when I navigate to a new project, then it disappears."

## Root cause

`cli/internal/server/frontend/src/lib/components/NavShell.svelte:26-36` — the sidebar's `pages` array contains: beads, documents, graph, workers, sessions, personas, plugins, commits, efficacy. **There is no entry for the project home/overview.** Once a user clicks any sidebar link, there is no nav-level path back to the home page — only clicking the project in the `ProjectPicker` again, or editing the URL.

Adjacent: the `&lt;span&gt;DDx&lt;/span&gt;` brand in the header (`NavShell.svelte:61`) is not a link — common affordance for "go home" is missing.

## Proposed direction

1. Prepend a "Overview" (or "Home") entry to `pages` in `NavShell.svelte`, routing to `/nodes/&lt;nodeId&gt;/projects/&lt;projectId&gt;/` (empty path segment after the project id). Use the `LayoutDashboard` icon that's already imported, or pick a dedicated home icon — design call.
2. Wrap the `DDx` brand in an `&lt;a&gt;` that routes to `/` (node selector) or to the current project's home if one is selected. Pick one and stay consistent.

Trivial change; the biggest decision is whether the brand link goes to node-root or project-home. Recommend project-home when a project is selected (less jarring) and node-root otherwise.
    </description>
    <acceptance>
**User story:** As a developer on any project sub-page, I can always return to the project overview with a single click — either from a sidebar entry or from the DDx brand in the header.

**Acceptance criteria:**

1. Sidebar has an "Overview" (or equivalent) entry at the top of the `pages` list, routing to `/nodes/&lt;nodeId&gt;/projects/&lt;projectId&gt;/`.
2. The entry is the active/highlighted one when the current URL matches the project-home route exactly (no trailing segment after `projectId`).
3. DDx brand in the header is an anchor. When a project is selected, it routes to that project's home; otherwise routes to `/`.
4. Playwright: navigate into `/beads`, click the Overview sidebar entry, assert URL is project home and queue summary renders. Then navigate into `/sessions`, click the DDx brand, assert URL is project home.
5. No regression to any existing sidebar entry.
    </acceptance>
    <labels>feat-008, ui, navigation</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a79c580a2c14a95724733f12ee12c9b2261695ce">
commit a79c580a2c14a95724733f12ee12c9b2261695ce
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 02:44:28 2026 -0400

    chore: add execution evidence [20260424T064038-]

diff --git a/.ddx/executions/20260424T064038-cebf2748/manifest.json b/.ddx/executions/20260424T064038-cebf2748/manifest.json
new file mode 100644
index 00000000..bbe0e68f
--- /dev/null
+++ b/.ddx/executions/20260424T064038-cebf2748/manifest.json
@@ -0,0 +1,130 @@
+{
+  "attempt_id": "20260424T064038-cebf2748",
+  "bead_id": "ddx-2513f08c",
+  "base_rev": "9e1e2cb2aefdea8d5b3052b3d24930b78fd65ee0",
+  "created_at": "2026-04-24T06:40:38.806258409Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-2513f08c",
+    "title": "Project home page is orphaned: add it to the sidebar and make the DDx logo a link",
+    "description": "## Observed\n\nThe project home page (`/nodes/.../projects/\u003cid\u003e/`) renders a useful overview — queue summary (ready/blocked/in-progress) plus three action cards (drain queue, re-align specs, run checks). User feedback: \"I see it when I navigate to a new project, then it disappears.\"\n\n## Root cause\n\n`cli/internal/server/frontend/src/lib/components/NavShell.svelte:26-36` — the sidebar's `pages` array contains: beads, documents, graph, workers, sessions, personas, plugins, commits, efficacy. **There is no entry for the project home/overview.** Once a user clicks any sidebar link, there is no nav-level path back to the home page — only clicking the project in the `ProjectPicker` again, or editing the URL.\n\nAdjacent: the `\u003cspan\u003eDDx\u003c/span\u003e` brand in the header (`NavShell.svelte:61`) is not a link — common affordance for \"go home\" is missing.\n\n## Proposed direction\n\n1. Prepend a \"Overview\" (or \"Home\") entry to `pages` in `NavShell.svelte`, routing to `/nodes/\u003cnodeId\u003e/projects/\u003cprojectId\u003e/` (empty path segment after the project id). Use the `LayoutDashboard` icon that's already imported, or pick a dedicated home icon — design call.\n2. Wrap the `DDx` brand in an `\u003ca\u003e` that routes to `/` (node selector) or to the current project's home if one is selected. Pick one and stay consistent.\n\nTrivial change; the biggest decision is whether the brand link goes to node-root or project-home. Recommend project-home when a project is selected (less jarring) and node-root otherwise.",
+    "acceptance": "**User story:** As a developer on any project sub-page, I can always return to the project overview with a single click — either from a sidebar entry or from the DDx brand in the header.\n\n**Acceptance criteria:**\n\n1. Sidebar has an \"Overview\" (or equivalent) entry at the top of the `pages` list, routing to `/nodes/\u003cnodeId\u003e/projects/\u003cprojectId\u003e/`.\n2. The entry is the active/highlighted one when the current URL matches the project-home route exactly (no trailing segment after `projectId`).\n3. DDx brand in the header is an anchor. When a project is selected, it routes to that project's home; otherwise routes to `/`.\n4. Playwright: navigate into `/beads`, click the Overview sidebar entry, assert URL is project home and queue summary renders. Then navigate into `/sessions`, click the DDx brand, assert URL is project home.\n5. No regression to any existing sidebar entry.",
+    "labels": [
+      "feat-008",
+      "ui",
+      "navigation"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T06:40:38Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"resolved_model\":\"qwen/qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:02:09.784028787Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen/qwen3.6-35b-a3b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen/qwen3.6-35b-a3b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-22T21:02:09.982732129Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"codex/gpt-5.4\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:02:12.891641856Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=codex/gpt-5.4"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness=claude model=codex/gpt-5.4 probe=ok\nunsupported model \"codex/gpt-5.4\" for harness \"claude\"; supported models: sonnet, opus, claude-sonnet-4-6",
+          "created_at": "2026-04-22T21:02:13.089139907Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"gemini\",\"resolved_model\":\"minimax/minimax-m2.7\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:02:16.022147433Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=gemini model=minimax/minimax-m2.7"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness=gemini model=minimax/minimax-m2.7 probe=ok\nunsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+          "created_at": "2026-04-22T21:02:16.221191691Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2224},{\"tier\":\"standard\",\"harness\":\"claude\",\"model\":\"codex/gpt-5.4\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2332},{\"tier\":\"smart\",\"harness\":\"gemini\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2338}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-22T21:02:16.281081147Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=3 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite\ntier=smart\nprobe_result=ok\nresult_rev=e77302300ec4b67fa093c4480308f27bf0a1906f\nbase_rev=e77302300ec4b67fa093c4480308f27bf0a1906f\nretry_after=2026-04-23T03:02:16Z",
+          "created_at": "2026-04-22T21:02:16.57498027Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T06:28:52.96231205Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T06:29:57.388537048Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T07:19:47.964392781Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T06:40:38.257220376Z",
+      "execute-loop-last-detail": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-04-23T03:02:16Z",
+      "feature": "FEAT-008"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T064038-cebf2748",
+    "prompt": ".ddx/executions/20260424T064038-cebf2748/prompt.md",
+    "manifest": ".ddx/executions/20260424T064038-cebf2748/manifest.json",
+    "result": ".ddx/executions/20260424T064038-cebf2748/result.json",
+    "checks": ".ddx/executions/20260424T064038-cebf2748/checks.json",
+    "usage": ".ddx/executions/20260424T064038-cebf2748/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-2513f08c-20260424T064038-cebf2748"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T064038-cebf2748/result.json b/.ddx/executions/20260424T064038-cebf2748/result.json
new file mode 100644
index 00000000..541e7335
--- /dev/null
+++ b/.ddx/executions/20260424T064038-cebf2748/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-2513f08c",
+  "attempt_id": "20260424T064038-cebf2748",
+  "base_rev": "9e1e2cb2aefdea8d5b3052b3d24930b78fd65ee0",
+  "result_rev": "d921957a962dbc95488de0c9fdf5a1bdcdcdfff5",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-5603e405",
+  "duration_ms": 228872,
+  "tokens": 13265,
+  "cost_usd": 1.4326612499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T064038-cebf2748",
+  "prompt_file": ".ddx/executions/20260424T064038-cebf2748/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T064038-cebf2748/manifest.json",
+  "result_file": ".ddx/executions/20260424T064038-cebf2748/result.json",
+  "usage_file": ".ddx/executions/20260424T064038-cebf2748/usage.json",
+  "started_at": "2026-04-24T06:40:38.806661909Z",
+  "finished_at": "2026-04-24T06:44:27.679444179Z"
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
## Review: ddx-2513f08c iter 1

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
