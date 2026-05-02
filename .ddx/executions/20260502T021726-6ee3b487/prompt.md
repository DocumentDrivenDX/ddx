<bead-review>
  <bead id="ddx-a812e176" iter=1>
    <title>Migrate docs/resources/ to docs/helix/00-discover/references/ with REF-NNN IDs and update tests</title>
    <description>
Move docs/resources/* into docs/helix/00-discover/references/ (or research/ for synthesis-style content). ID assignment: spec-driven.md→REF-001, ai-agent-frameworks-2025.md→REF-002, microsoft-ai-agents-2025.md→REF-003, model-catalog.md→REF-004. Files agent-harness-ac.md and ac-work.md already have ddx: IDs (AC-AGENT-001/002) — preserve those IDs but move under 00-discover/research/ as kind=research-synthesis. Normalize frontmatter (id, title, kind, source_url, source_author, accessed, summary, tags). Update cross-references: cli/internal/server/graphql_documents_test.go (lines ~132, ~188), cli/internal/server/frontend/e2e/documents.spec.ts (~217), docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md. Stub docs/resources/README.md pointing forward.
    </description>
    <acceptance>
1. docs/resources/ contains only README.md stub. 2. docs/helix/00-discover/references/ contains REF-001 through REF-004 with normalized frontmatter. 3. AC-AGENT-001 and AC-AGENT-002 preserved at docs/helix/00-discover/research/ (or wherever appropriate). 4. Tests pass: 'cd cli &amp;&amp; go test ./internal/server/...' and 'cd cli/internal/server/frontend &amp;&amp; bun run test:e2e --grep documents'. 5. Run 'rg -n "docs/resources/[a-z]" docs cli website 2&gt;/dev/null | grep -v "docs/resources/README.md"' returns 0 matches.
    </acceptance>
    <labels>site-redesign, area:specs, area:tests, kind:migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="05020a057a0ce66d597cb74bb245dda1d0d7dfbe">
commit 05020a057a0ce66d597cb74bb245dda1d0d7dfbe
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:17:23 2026 -0400

    chore: add execution evidence [20260502T020932-]

diff --git a/.ddx/executions/20260502T020932-f66fa756/manifest.json b/.ddx/executions/20260502T020932-f66fa756/manifest.json
new file mode 100644
index 00000000..76903183
--- /dev/null
+++ b/.ddx/executions/20260502T020932-f66fa756/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T020932-f66fa756",
+  "bead_id": "ddx-a812e176",
+  "base_rev": "5c75fa8547bd3d39dbd85575a679e097acedea17",
+  "created_at": "2026-05-02T02:09:33.585564007Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a812e176",
+    "title": "Migrate docs/resources/ to docs/helix/00-discover/references/ with REF-NNN IDs and update tests",
+    "description": "Move docs/resources/* into docs/helix/00-discover/references/ (or research/ for synthesis-style content). ID assignment: spec-driven.md→REF-001, ai-agent-frameworks-2025.md→REF-002, microsoft-ai-agents-2025.md→REF-003, model-catalog.md→REF-004. Files agent-harness-ac.md and ac-work.md already have ddx: IDs (AC-AGENT-001/002) — preserve those IDs but move under 00-discover/research/ as kind=research-synthesis. Normalize frontmatter (id, title, kind, source_url, source_author, accessed, summary, tags). Update cross-references: cli/internal/server/graphql_documents_test.go (lines ~132, ~188), cli/internal/server/frontend/e2e/documents.spec.ts (~217), docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md. Stub docs/resources/README.md pointing forward.",
+    "acceptance": "1. docs/resources/ contains only README.md stub. 2. docs/helix/00-discover/references/ contains REF-001 through REF-004 with normalized frontmatter. 3. AC-AGENT-001 and AC-AGENT-002 preserved at docs/helix/00-discover/research/ (or wherever appropriate). 4. Tests pass: 'cd cli \u0026\u0026 go test ./internal/server/...' and 'cd cli/internal/server/frontend \u0026\u0026 bun run test:e2e --grep documents'. 5. Run 'rg -n \"docs/resources/[a-z]\" docs cli website 2\u003e/dev/null | grep -v \"docs/resources/README.md\"' returns 0 matches.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:specs",
+      "area:tests",
+      "kind:migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:09:32Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:09:32.071701563Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T020932-f66fa756",
+    "prompt": ".ddx/executions/20260502T020932-f66fa756/prompt.md",
+    "manifest": ".ddx/executions/20260502T020932-f66fa756/manifest.json",
+    "result": ".ddx/executions/20260502T020932-f66fa756/result.json",
+    "checks": ".ddx/executions/20260502T020932-f66fa756/checks.json",
+    "usage": ".ddx/executions/20260502T020932-f66fa756/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a812e176-20260502T020932-f66fa756"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T020932-f66fa756/result.json b/.ddx/executions/20260502T020932-f66fa756/result.json
new file mode 100644
index 00000000..845d6dcf
--- /dev/null
+++ b/.ddx/executions/20260502T020932-f66fa756/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a812e176",
+  "attempt_id": "20260502T020932-f66fa756",
+  "base_rev": "5c75fa8547bd3d39dbd85575a679e097acedea17",
+  "result_rev": "04f48c1d910b57cc4e8c08da87d618e183a422d0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-bd1fa4b5",
+  "duration_ms": 468255,
+  "tokens": 19614,
+  "cost_usd": 3.0277947500000004,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T020932-f66fa756",
+  "prompt_file": ".ddx/executions/20260502T020932-f66fa756/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T020932-f66fa756/manifest.json",
+  "result_file": ".ddx/executions/20260502T020932-f66fa756/result.json",
+  "usage_file": ".ddx/executions/20260502T020932-f66fa756/usage.json",
+  "started_at": "2026-05-02T02:09:33.585899424Z",
+  "finished_at": "2026-05-02T02:17:21.841160908Z"
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
## Review: ddx-a812e176 iter 1

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
