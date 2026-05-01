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

  <diff rev="dd0c18f7ef6aa908b39792a5881b9f34a87e14ab">
commit dd0c18f7ef6aa908b39792a5881b9f34a87e14ab
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 22:09:21 2026 -0400

    chore: add execution evidence [20260423T014514-]

diff --git a/.ddx/executions/20260423T014514-bc0872e9/result.json b/.ddx/executions/20260423T014514-bc0872e9/result.json
new file mode 100644
index 00000000..61c35b5b
--- /dev/null
+++ b/.ddx/executions/20260423T014514-bc0872e9/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-12cae4dd",
+  "attempt_id": "20260423T014514-bc0872e9",
+  "base_rev": "f3bcb38e913f664ebf1380b13255d8547a15cfe9",
+  "result_rev": "5ae2fb8c774ab27e5a37b2131104d19f18b5c315",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3cf42b9a",
+  "duration_ms": 1445824,
+  "tokens": 62854,
+  "cost_usd": 13.526168750000004,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T014514-bc0872e9",
+  "prompt_file": ".ddx/executions/20260423T014514-bc0872e9/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T014514-bc0872e9/manifest.json",
+  "result_file": ".ddx/executions/20260423T014514-bc0872e9/result.json",
+  "usage_file": ".ddx/executions/20260423T014514-bc0872e9/usage.json",
+  "started_at": "2026-04-23T01:45:14.909335352Z",
+  "finished_at": "2026-04-23T02:09:20.734065638Z"
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
