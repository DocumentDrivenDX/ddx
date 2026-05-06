<bead-review>
  <bead id="ddx-3c6f5bf0" iter=1>
    <title>Exclude hidden root segments from docgraph indexing</title>
    <description>
PROBLEM
  `ddx doc audit` can still index duplicate/stale Markdown documents from hidden tool-managed trees when graph roots are configured inside those trees. The observed dogfood failure is `.agents/skills/docs/...` being indexed alongside canonical `docs/...`, producing duplicate_id noise even though root-level hidden directories should never be part of the document graph.

ROOT CAUSE
  cli/internal/docgraph/docgraph.go:540-548 accepts configured roots verbatim and starts `filepath.Walk` at each configured root. cli/internal/docgraph/docgraph.go:569-578 skips hidden/tool-managed directories only when the walker encounters the directory entry. If the configured root is already inside a hidden segment such as `.agents/skills/docs`, that basename skip never fires. cli/internal/docgraph/docgraph.go:556-568 adds a one-off path-based defense for `.ddx/plugins`, and cli/internal/docgraph/docgraph_test.go:630-677 only tests that narrow `.ddx/plugins` case.

PROPOSED FIX
  - Replace or generalize the `.ddx/plugins`-specific path defense with a repository-relative hidden-segment exclusion used before accepting either directories or files.
  - Any path under a root-relative segment whose basename begins with `.` must be excluded from docgraph indexing, including configured roots that start inside `.agents/`, `.claude/`, `.ddx/`, `.skills/`, or future hidden tool directories.
  - Keep the existing `worktrees` directory skip for non-hidden scratch directories.
  - Add regression coverage for a configured root that points directly at `.agents/skills/docs` and asserts no files/documents from that root are indexed and no duplicate_id issue is produced.
  - Update FEAT-005/FEAT-007 doc discovery wording only if it still describes hidden/tool-managed trees ambiguously.

NON-SCOPE
  - Do not hand-edit `.ddx/beads.jsonl`.
  - Do not delete `.agents/skills/docs` or other installed skill/plugin content as part of this fix.
  - Do not change document frontmatter IDs merely to silence duplicates from hidden trees.

PARENT
  ddx-781af038 (Story 4 / docgraph integrity lineage; follows the incomplete narrower fix in ddx-2502ad71).

DEPS
  No deps.
    </description>
    <acceptance>
1. cli/internal/docgraph/docgraph.go excludes any document path under a root-relative hidden path segment (`.^`/basename starts with `.`), even when a graph config root points directly inside that hidden subtree.
2. Existing canonical docs under `docs/` are still indexed by BuildGraph and BuildGraphWithConfig.
3. TestBuildGraph_ExcludesConfiguredHiddenRoots (or equivalent) in cli/internal/docgraph/docgraph_test.go creates canonical `docs/feat.md` plus duplicate `.agents/skills/docs/feat.md`, configures a root inside `.agents/skills/docs`, and verifies only the canonical doc is indexed or the hidden root yields zero files with no duplicate_id.
4. Existing TestBuildGraph_ExcludesDDxPluginsTree remains green or is replaced by a broader hidden-root regression test that covers `.ddx/plugins` as one case.
5. `ddx doc audit` from the repository root no longer reports duplicate_id issues caused by `.agents/skills/docs`, `.claude`, `.ddx`, `.skills`, or other hidden root segments.
6. `cd cli &amp;&amp; go test ./internal/docgraph/... -run "TestBuildGraph_ExcludesConfiguredHiddenRoots|TestBuildGraph_ExcludesDDxPluginsTree|TestBuildGraph_ExcludesClaudeWorktreesAndStoresRelativePaths" -count=1` passes.
7. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:docgraph, area:docs, kind:fix, regression</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T012517-fc9b8334/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T012517-fc9b8334/manifest.json</file>
    <file>.ddx/executions/20260506T012517-fc9b8334/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9ae64685fb3d244f7e73ff7af30fe0bb6051c275">
<untrusted-data>
diff --git a/.ddx/executions/20260506T012517-fc9b8334/checks/production-reachability.json b/.ddx/executions/20260506T012517-fc9b8334/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T012517-fc9b8334/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T012517-fc9b8334/manifest.json b/.ddx/executions/20260506T012517-fc9b8334/manifest.json
new file mode 100644
index 00000000..c93b7909
--- /dev/null
+++ b/.ddx/executions/20260506T012517-fc9b8334/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260506T012517-fc9b8334",
+  "bead_id": "ddx-3c6f5bf0",
+  "base_rev": "c3de7f327343069829f09a91d4b22e83f2cfacc9",
+  "created_at": "2026-05-06T01:25:19.678602353Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3c6f5bf0",
+    "title": "Exclude hidden root segments from docgraph indexing",
+    "description": "PROBLEM\n  `ddx doc audit` can still index duplicate/stale Markdown documents from hidden tool-managed trees when graph roots are configured inside those trees. The observed dogfood failure is `.agents/skills/docs/...` being indexed alongside canonical `docs/...`, producing duplicate_id noise even though root-level hidden directories should never be part of the document graph.\n\nROOT CAUSE\n  cli/internal/docgraph/docgraph.go:540-548 accepts configured roots verbatim and starts `filepath.Walk` at each configured root. cli/internal/docgraph/docgraph.go:569-578 skips hidden/tool-managed directories only when the walker encounters the directory entry. If the configured root is already inside a hidden segment such as `.agents/skills/docs`, that basename skip never fires. cli/internal/docgraph/docgraph.go:556-568 adds a one-off path-based defense for `.ddx/plugins`, and cli/internal/docgraph/docgraph_test.go:630-677 only tests that narrow `.ddx/plugins` case.\n\nPROPOSED FIX\n  - Replace or generalize the `.ddx/plugins`-specific path defense with a repository-relative hidden-segment exclusion used before accepting either directories or files.\n  - Any path under a root-relative segment whose basename begins with `.` must be excluded from docgraph indexing, including configured roots that start inside `.agents/`, `.claude/`, `.ddx/`, `.skills/`, or future hidden tool directories.\n  - Keep the existing `worktrees` directory skip for non-hidden scratch directories.\n  - Add regression coverage for a configured root that points directly at `.agents/skills/docs` and asserts no files/documents from that root are indexed and no duplicate_id issue is produced.\n  - Update FEAT-005/FEAT-007 doc discovery wording only if it still describes hidden/tool-managed trees ambiguously.\n\nNON-SCOPE\n  - Do not hand-edit `.ddx/beads.jsonl`.\n  - Do not delete `.agents/skills/docs` or other installed skill/plugin content as part of this fix.\n  - Do not change document frontmatter IDs merely to silence duplicates from hidden trees.\n\nPARENT\n  ddx-781af038 (Story 4 / docgraph integrity lineage; follows the incomplete narrower fix in ddx-2502ad71).\n\nDEPS\n  No deps.",
+    "acceptance": "1. cli/internal/docgraph/docgraph.go excludes any document path under a root-relative hidden path segment (`.^`/basename starts with `.`), even when a graph config root points directly inside that hidden subtree.\n2. Existing canonical docs under `docs/` are still indexed by BuildGraph and BuildGraphWithConfig.\n3. TestBuildGraph_ExcludesConfiguredHiddenRoots (or equivalent) in cli/internal/docgraph/docgraph_test.go creates canonical `docs/feat.md` plus duplicate `.agents/skills/docs/feat.md`, configures a root inside `.agents/skills/docs`, and verifies only the canonical doc is indexed or the hidden root yields zero files with no duplicate_id.\n4. Existing TestBuildGraph_ExcludesDDxPluginsTree remains green or is replaced by a broader hidden-root regression test that covers `.ddx/plugins` as one case.\n5. `ddx doc audit` from the repository root no longer reports duplicate_id issues caused by `.agents/skills/docs`, `.claude`, `.ddx`, `.skills`, or other hidden root segments.\n6. `cd cli \u0026\u0026 go test ./internal/docgraph/... -run \"TestBuildGraph_ExcludesConfiguredHiddenRoots|TestBuildGraph_ExcludesDDxPluginsTree|TestBuildGraph_ExcludesClaudeWorktreesAndStoresRelativePaths\" -count=1` passes.\n7. `lefthook run pre-commit` passes.",
+    "parent": "ddx-781af038",
+    "labels": [
+      "phase:2",
+      "area:docgraph",
+      "area:docs",
+      "kind:fix",
+      "regression"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T01:25:17Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T01:25:17.228654845Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T012517-fc9b8334",
+    "prompt": ".ddx/executions/20260506T012517-fc9b8334/prompt.md",
+    "manifest": ".ddx/executions/20260506T012517-fc9b8334/manifest.json",
+    "result": ".ddx/executions/20260506T012517-fc9b8334/result.json",
+    "checks": ".ddx/executions/20260506T012517-fc9b8334/checks.json",
+    "usage": ".ddx/executions/20260506T012517-fc9b8334/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3c6f5bf0-20260506T012517-fc9b8334"
+  },
+  "prompt_sha": "548359d235c1033a42477ebf5030c8ffb8cb0408dd6dd387c48808090d8459df"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T012517-fc9b8334/result.json b/.ddx/executions/20260506T012517-fc9b8334/result.json
new file mode 100644
index 00000000..46de7207
--- /dev/null
+++ b/.ddx/executions/20260506T012517-fc9b8334/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-3c6f5bf0",
+  "attempt_id": "20260506T012517-fc9b8334",
+  "base_rev": "c3de7f327343069829f09a91d4b22e83f2cfacc9",
+  "result_rev": "ff7c319167af567898aaed97ca18adda5a4af899",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-207bf602",
+  "duration_ms": 181691,
+  "tokens": 1974674,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T012517-fc9b8334",
+  "prompt_file": ".ddx/executions/20260506T012517-fc9b8334/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T012517-fc9b8334/manifest.json",
+  "result_file": ".ddx/executions/20260506T012517-fc9b8334/result.json",
+  "usage_file": ".ddx/executions/20260506T012517-fc9b8334/usage.json",
+  "started_at": "2026-05-06T01:25:19.678976519Z",
+  "finished_at": "2026-05-06T01:28:21.370455525Z"
+}
\ No newline at end of file
</untrusted-data>
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
