<bead-review>
  <bead id="ddx-12cae4dd" iter=1>
    <title>Web UI documents view includes .claude/worktrees noise and serves absolute paths</title>
    <description>
## Observed

Browsing the web UI to a document URL renders `Document not found` instead of content. The URL contains a worktree path with a leading double slash, e.g.:

  /nodes/&lt;node&gt;/projects/&lt;proj&gt;/documents//Users/erik/Projects/ddx/.claude/worktrees/agent-a0673989/docs/resources/agent-harness-ac.md

Two distinct defects:

1. **`.claude/worktrees/` pollution** — the documents list surfaces markdown files from AI agent worktrees checked out under `.claude/worktrees/`. These are throwaway copies of the repo; they are noise and frequently shadow / duplicate the real `docs/` tree.
2. **Absolute document paths** — document IDs/paths are absolute (`/Users/erik/...`), producing malformed URLs (leading double slash) and failing the reader resolver.

## Root cause

`cli/internal/docgraph/docgraph.go:247` (`findMarkdownFiles`) walks `workingDir` and only skips `.git` and `.ddx`. It traverses `.claude/` (including `.claude/worktrees/&lt;agent&gt;/…`) and records every `.md` it finds.

`cli/internal/docgraph/docgraph.go:128` stores `doc.Path = filepath.Clean(filePath)`. Because `filepath.Walk` hands back paths rooted at the absolute `workingDir`, `doc.Path` ends up absolute. The GraphQL document resolver surfaces this absolute path as the document's id/path, which the frontend then interpolates into a URL.

## Scope of fix

- Exclude `.claude/` (and any `worktrees/` subtree) from doc discovery. `.claude` is a tool-managed directory analogous to `.git`/`.ddx`; it is not a source of canonical project documents.
- Normalize `doc.Path` to be relative to `workingDir` so routes stay clean and portable.
- No change required on the frontend beyond existing URL handling once paths are relative.
    </description>
    <acceptance>
**User story:** As a developer browsing the DDx web UI, I want the documents list to show only canonical project documents under the configured roots, so I can find `docs/resources/agent-harness-ac.md` without seeing duplicates from agent worktrees or broken absolute-path links.

**Acceptance criteria:**

1. `findMarkdownFiles` skips `.claude` (in addition to `.git` and `.ddx`) and does not descend into any `worktrees/` directory regardless of parent.
2. `doc.Path` stored in the graph is relative to `workingDir`; no document path begins with `/`.
3. GraphQL `documents` query returns zero entries whose path contains `.claude/worktrees/` when the repo has active worktrees under `.claude/worktrees/`.
4. GraphQL `document(id:)` resolver handles both the previously-absolute form and the new relative form for a grace period (read-side tolerance) OR all callers are updated atomically.
5. Web UI document route `/documents/:path` renders the correct document content for `docs/resources/agent-harness-ac.md` (no double slash, no 404).
6. Unit test in `cli/internal/docgraph/docgraph_test.go` creates a fixture with `.claude/worktrees/agent-x/docs/foo.md` and `docs/foo.md`, and asserts only the latter appears in the graph with a relative path.
7. Playwright e2e test in `cli/internal/server/frontend/` navigates to the documents list, asserts no listed document has a path containing `.claude/` or starting with `/`, clicks a real `docs/` document, and verifies the content renders.
    </acceptance>
    <labels>feat-008, feat-007, ui, docgraph</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="16abc192b8792fae34a513abf67e5603d26d51c6">
commit 16abc192b8792fae34a513abf67e5603d26d51c6
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 22:17:51 2026 -0400

    chore: add execution evidence [20260423T021107-]

diff --git a/.ddx/executions/20260423T021107-3834d7a9/manifest.json b/.ddx/executions/20260423T021107-3834d7a9/manifest.json
new file mode 100644
index 00000000..565b9879
--- /dev/null
+++ b/.ddx/executions/20260423T021107-3834d7a9/manifest.json
@@ -0,0 +1,154 @@
+{
+  "attempt_id": "20260423T021107-3834d7a9",
+  "bead_id": "ddx-12cae4dd",
+  "base_rev": "c4bf2710d52ebd6d04a64796fa8fdea0e15c12c5",
+  "created_at": "2026-04-23T02:11:07.855075836Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-12cae4dd",
+    "title": "Web UI documents view includes .claude/worktrees noise and serves absolute paths",
+    "description": "## Observed\n\nBrowsing the web UI to a document URL renders `Document not found` instead of content. The URL contains a worktree path with a leading double slash, e.g.:\n\n  /nodes/\u003cnode\u003e/projects/\u003cproj\u003e/documents//Users/erik/Projects/ddx/.claude/worktrees/agent-a0673989/docs/resources/agent-harness-ac.md\n\nTwo distinct defects:\n\n1. **`.claude/worktrees/` pollution** — the documents list surfaces markdown files from AI agent worktrees checked out under `.claude/worktrees/`. These are throwaway copies of the repo; they are noise and frequently shadow / duplicate the real `docs/` tree.\n2. **Absolute document paths** — document IDs/paths are absolute (`/Users/erik/...`), producing malformed URLs (leading double slash) and failing the reader resolver.\n\n## Root cause\n\n`cli/internal/docgraph/docgraph.go:247` (`findMarkdownFiles`) walks `workingDir` and only skips `.git` and `.ddx`. It traverses `.claude/` (including `.claude/worktrees/\u003cagent\u003e/…`) and records every `.md` it finds.\n\n`cli/internal/docgraph/docgraph.go:128` stores `doc.Path = filepath.Clean(filePath)`. Because `filepath.Walk` hands back paths rooted at the absolute `workingDir`, `doc.Path` ends up absolute. The GraphQL document resolver surfaces this absolute path as the document's id/path, which the frontend then interpolates into a URL.\n\n## Scope of fix\n\n- Exclude `.claude/` (and any `worktrees/` subtree) from doc discovery. `.claude` is a tool-managed directory analogous to `.git`/`.ddx`; it is not a source of canonical project documents.\n- Normalize `doc.Path` to be relative to `workingDir` so routes stay clean and portable.\n- No change required on the frontend beyond existing URL handling once paths are relative.",
+    "acceptance": "**User story:** As a developer browsing the DDx web UI, I want the documents list to show only canonical project documents under the configured roots, so I can find `docs/resources/agent-harness-ac.md` without seeing duplicates from agent worktrees or broken absolute-path links.\n\n**Acceptance criteria:**\n\n1. `findMarkdownFiles` skips `.claude` (in addition to `.git` and `.ddx`) and does not descend into any `worktrees/` directory regardless of parent.\n2. `doc.Path` stored in the graph is relative to `workingDir`; no document path begins with `/`.\n3. GraphQL `documents` query returns zero entries whose path contains `.claude/worktrees/` when the repo has active worktrees under `.claude/worktrees/`.\n4. GraphQL `document(id:)` resolver handles both the previously-absolute form and the new relative form for a grace period (read-side tolerance) OR all callers are updated atomically.\n5. Web UI document route `/documents/:path` renders the correct document content for `docs/resources/agent-harness-ac.md` (no double slash, no 404).\n6. Unit test in `cli/internal/docgraph/docgraph_test.go` creates a fixture with `.claude/worktrees/agent-x/docs/foo.md` and `docs/foo.md`, and asserts only the latter appears in the graph with a relative path.\n7. Playwright e2e test in `cli/internal/server/frontend/` navigates to the documents list, asserts no listed document has a path containing `.claude/` or starting with `/`, clicks a real `docs/` document, and verifies the content renders.",
+    "labels": [
+      "feat-008",
+      "feat-007",
+      "ui",
+      "docgraph"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-23T02:11:07Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2988370",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"resolved_model\":\"qwen/qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:00:02.172732523Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen/qwen3.6-35b-a3b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen/qwen3.6-35b-a3b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-22T21:00:02.369478392Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"codex/gpt-5.4\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:00:04.946466858Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=codex/gpt-5.4"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness=claude model=codex/gpt-5.4 probe=ok\nunsupported model \"codex/gpt-5.4\" for harness \"claude\"; supported models: sonnet, opus, claude-sonnet-4-6",
+          "created_at": "2026-04-22T21:00:05.150693171Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"gemini\",\"resolved_model\":\"minimax/minimax-m2.7\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:00:07.731285506Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=gemini model=minimax/minimax-m2.7"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness=gemini model=minimax/minimax-m2.7 probe=ok\nunsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+          "created_at": "2026-04-22T21:00:07.935697276Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2021},{\"tier\":\"standard\",\"harness\":\"claude\",\"model\":\"codex/gpt-5.4\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2010},{\"tier\":\"smart\",\"harness\":\"gemini\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2010}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-22T21:00:07.994408038Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=3 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite\ntier=smart\nprobe_result=ok\nresult_rev=1d72b42360ecda28ed12449c395b8c6486cfeb6d\nbase_rev=1d72b42360ecda28ed12449c395b8c6486cfeb6d\nretry_after=2026-04-23T03:00:08Z",
+          "created_at": "2026-04-22T21:00:08.164021459Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-23T02:09:20.736870511Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260423T014514-bc0872e9\",\"harness\":\"claude\",\"input_tokens\":142,\"output_tokens\":62712,\"total_tokens\":62854,\"cost_usd\":13.526168750000004,\"duration_ms\":1445824,\"exit_code\":0}",
+          "created_at": "2026-04-23T02:09:20.804287008Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=62854 cost_usd=13.5262"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-23T02:09:23.661610903Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "**All AC items**: The merged diff consists solely of `.ddx/executions/20260423T014514-bc0872e9/result.json`, which is execution bookkeeping. Zero functional changes were landed — no modifications to `cli/internal/docgraph/docgraph.go`, no GraphQL resolver updates, no frontend changes, no tests. The execution result claims `\"outcome\": \"task_succeeded\"` but the merge commit contains none of the required implementation. The work branch may have been incorrectly merged or the agent's changes were lost during the merge process. Investigate whether the implementation commits from `5ae2fb8c` were actually included in the merge at `4671d2db`.",
+          "created_at": "2026-04-23T02:09:41.40986412Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-23T02:09:41.480377407Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "ddx",
+          "body": "post-merge review: BLOCK (flagged for human)\n**All AC items**: The merged diff consists solely of `.ddx/executions/20260423T014514-bc0872e9/result.json`, which is execution bookkeeping. Zero functional changes were landed — no modifications to `cli/internal/docgraph/docgraph.go`, no GraphQL resolver updates, no frontend changes, no tests. The execution result claims `\"outcome\": \"task_succeeded\"` but the merge commit contains none of the required implementation. The work branch may have been incorrectly merged or the agent's changes were lost during the merge process. Investigate whether the implementation commits from `5ae2fb8c` were actually included in the merge at `4671d2db`.\nresult_rev=dd0c18f7ef6aa908b39792a5881b9f34a87e14ab\nbase_rev=f3bcb38e913f664ebf1380b13255d8547a15cfe9",
+          "created_at": "2026-04-23T02:09:41.541955407Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-23T02:11:07.374044823Z",
+      "execute-loop-last-detail": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+      "execute-loop-last-status": "execution_failed",
+      "feature": "FEAT-008"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260423T021107-3834d7a9",
+    "prompt": ".ddx/executions/20260423T021107-3834d7a9/prompt.md",
+    "manifest": ".ddx/executions/20260423T021107-3834d7a9/manifest.json",
+    "result": ".ddx/executions/20260423T021107-3834d7a9/result.json",
+    "checks": ".ddx/executions/20260423T021107-3834d7a9/checks.json",
+    "usage": ".ddx/executions/20260423T021107-3834d7a9/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-12cae4dd-20260423T021107-3834d7a9"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260423T021107-3834d7a9/result.json b/.ddx/executions/20260423T021107-3834d7a9/result.json
new file mode 100644
index 00000000..088227a0
--- /dev/null
+++ b/.ddx/executions/20260423T021107-3834d7a9/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-12cae4dd",
+  "attempt_id": "20260423T021107-3834d7a9",
+  "base_rev": "c4bf2710d52ebd6d04a64796fa8fdea0e15c12c5",
+  "result_rev": "4f237bf1d9f0345163b4ed83013a7ddcfca551fa",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-e26e396a",
+  "duration_ms": 403187,
+  "tokens": 2737484,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T021107-3834d7a9",
+  "prompt_file": ".ddx/executions/20260423T021107-3834d7a9/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T021107-3834d7a9/manifest.json",
+  "result_file": ".ddx/executions/20260423T021107-3834d7a9/result.json",
+  "usage_file": ".ddx/executions/20260423T021107-3834d7a9/usage.json",
+  "started_at": "2026-04-23T02:11:07.855520128Z",
+  "finished_at": "2026-04-23T02:17:51.042922784Z"
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
## Review: ddx-12cae4dd iter 1

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
