<bead-review>
  <bead id="ddx-6ff597ca" iter=1>
    <title>Document graph needs an integrity audit/repair surface for duplicate IDs, missing deps, and orphaned entries</title>
    <description>
## Observed

The graph view at `/nodes/.../projects/.../graph` renders a long flat list of amber-styled warnings, currently dominated by `duplicate document id "AC-AGENT-001" in "/Users/erik/Projects/ddx/.claude/worktrees/agent-a2818c5c/docs/resources/agent-harness-ac.md"` and similar entries. Two distinct concerns:

1. **Input noise** — most duplicates today come from `.claude/worktrees/&lt;agent&gt;/...` copies of the repo being indexed (tracked in `ddx-12cae4dd`). That fix will eliminate a large class of false positives but will not make the warning surface usable.
2. **Brittle degradation** — when documents have real problems (duplicate IDs in the canonical tree, broken frontmatter, missing `depends_on` targets, `id_to_path` mismatches), the system doesn't just warn — downstream consumers (documents list, staleness, UI routing) misbehave. And there is no workflow for a user to triage, diff, or fix the offending files.

## Current state of warnings

- `cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/graph/+page.svelte:22-30` dumps `graph.warnings: string[]` into a flat amber block. No grouping, no categorization, no links to the offending file, no way to dismiss or act.
- Warning strings are generated in `cli/internal/docgraph/docgraph.go` (duplicate id, parse failure, required-root-missing, id_to_path mismatch/missing, cascade unknowns) as unstructured `fmt.Sprintf` output.
- No stale-document-pruning: if a `.md` is deleted on disk, any id_to_path entry pointing at it yields a warning instead of being auto-cleaned.

## Scope of this bead (deliberately not overlapping ddx-12cae4dd)

Assume `.claude/worktrees` noise is gone. This bead delivers the **integrity surface** that remains valuable: a way to see, categorize, and act on real document problems.

### Deliverables

1. **Structured warnings** — extend `docgraph.Graph.Warnings` from `[]string` to `[]GraphIssue{kind, path, id, message, relatedPath?}` and plumb through GraphQL. Keep a `messageLines(): []string` shim for any caller still using the old shape during the grace period.
2. **Integrity panel on /graph** — replace the flat amber block with a collapsible panel grouped by issue kind (Duplicate ID, Missing dep target, Broken id_to_path, Parse error, Required root missing). Each group shows count + expandable list; each row shows file path (clickable → document viewer), the offending ID, and a copyable message.
3. **Summary badge** — small badge next to the page title showing total issue count, so a clean graph reads clean.
4. **Audit GraphQL query** — `query { docGraphIssues { kind, path, id, message, relatedPath } }` usable independently of the full graph for future dashboard use. Reuses the same structured slice.
5. **Repair affordances (minimum viable)** — for two common cases:
   - Duplicate ID: show both file paths side-by-side and a "copy suggested unique ID" button (deterministic suggestion, e.g., append a short hash of the path). No auto-write.
   - Missing dep target: show the document declaring the missing dep, and a copy-to-clipboard snippet of the frontmatter line to remove. No auto-write.
   Auto-repair is explicitly out of scope until we trust the detection.
6. **CLI parity** — `ddx doctor` (or `ddx docs audit`) prints the same structured issues in a terminal-friendly form, so headless users and CI can surface problems.

### Out of scope

- Fixing `.claude/worktrees` indexing (owned by `ddx-12cae4dd`).
- Auto-rewrite of source files.
- Cross-project graph integrity (today's graph is per-project).
    </description>
    <acceptance>
**User story:** As a developer opening the document graph, I want a structured integrity panel that groups real document problems by kind and points me at the files to fix, so I can triage a noisy graph quickly and confirm when it is healthy.

**Acceptance criteria:**

1. `docgraph.Graph.Warnings` (or a sibling `Issues` field) is a typed slice with `Kind`, `Path`, `ID`, `Message`, and optional `RelatedPath`. Kinds include at minimum: `duplicate_id`, `parse_error`, `missing_dep`, `id_path_missing`, `id_path_mismatch`, `required_root_missing`, `cascade_unknown`.
2. GraphQL schema exposes `DocGraph.issues: [GraphIssue!]!` with the same fields. The existing string `warnings` either stays (rendered from issues) or is deprecated with a schema comment.
3. Unit tests in `cli/internal/docgraph/docgraph_test.go` cover each issue kind: fixtures that produce exactly one instance of that kind and assert the typed output.
4. `/graph` page renders an **Integrity** panel when `issues.length &gt; 0`:
   - grouped by `kind` with a count per group;
   - each row shows path, id, and message;
   - path links to the document viewer (reuses existing documents route);
   - total-issues badge appears next to the page title.
5. With zero issues, neither the panel nor the badge renders, and the graph view is visually identical to a healthy graph today.
6. Duplicate-ID rows show both paths and a "copy suggested unique ID" button; the suggestion is deterministic (same input → same output) and a unit test asserts the suggestion function's stability.
7. `ddx docs audit` (or `ddx doctor docs`) CLI command prints the same issue list grouped by kind, exits 0 if empty and 1 if any issues present. Integration test asserts exit codes and grouped output.
8. Playwright e2e:
   - navigates to `/graph` on a fixture repo with one seeded duplicate-id pair and one missing-dep target;
   - asserts the Integrity panel shows groups "Duplicate ID (1)" and "Missing dep target (1)";
   - expands the Duplicate ID group and asserts both paths are visible;
   - clicks the first path and asserts navigation to the documents route for that file;
   - on a clean fixture repo, asserts neither the panel nor the badge is present.

**Dependency:** best sequenced after `ddx-12cae4dd` lands so the fixture repos are not polluted by `.claude/worktrees` noise, but implementation does not block on it — the two can proceed in parallel and this bead's tests use synthetic fixtures.
    </acceptance>
    <labels>feat-007, feat-008, ui, docgraph</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="eea1d78797372c47d605938d69f3bf81ea227d63">
commit eea1d78797372c47d605938d69f3bf81ea227d63
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 23:00:21 2026 -0400

    chore: add execution evidence [20260423T025034-]

diff --git a/.ddx/executions/20260423T025034-5febdfb4/manifest.json b/.ddx/executions/20260423T025034-5febdfb4/manifest.json
new file mode 100644
index 00000000..766fff11
--- /dev/null
+++ b/.ddx/executions/20260423T025034-5febdfb4/manifest.json
@@ -0,0 +1,154 @@
+{
+  "attempt_id": "20260423T025034-5febdfb4",
+  "bead_id": "ddx-6ff597ca",
+  "base_rev": "dba129f088089b5a4e5de453501f507a66e11825",
+  "created_at": "2026-04-23T02:50:34.892529213Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-6ff597ca",
+    "title": "Document graph needs an integrity audit/repair surface for duplicate IDs, missing deps, and orphaned entries",
+    "description": "## Observed\n\nThe graph view at `/nodes/.../projects/.../graph` renders a long flat list of amber-styled warnings, currently dominated by `duplicate document id \"AC-AGENT-001\" in \"/Users/erik/Projects/ddx/.claude/worktrees/agent-a2818c5c/docs/resources/agent-harness-ac.md\"` and similar entries. Two distinct concerns:\n\n1. **Input noise** — most duplicates today come from `.claude/worktrees/\u003cagent\u003e/...` copies of the repo being indexed (tracked in `ddx-12cae4dd`). That fix will eliminate a large class of false positives but will not make the warning surface usable.\n2. **Brittle degradation** — when documents have real problems (duplicate IDs in the canonical tree, broken frontmatter, missing `depends_on` targets, `id_to_path` mismatches), the system doesn't just warn — downstream consumers (documents list, staleness, UI routing) misbehave. And there is no workflow for a user to triage, diff, or fix the offending files.\n\n## Current state of warnings\n\n- `cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/graph/+page.svelte:22-30` dumps `graph.warnings: string[]` into a flat amber block. No grouping, no categorization, no links to the offending file, no way to dismiss or act.\n- Warning strings are generated in `cli/internal/docgraph/docgraph.go` (duplicate id, parse failure, required-root-missing, id_to_path mismatch/missing, cascade unknowns) as unstructured `fmt.Sprintf` output.\n- No stale-document-pruning: if a `.md` is deleted on disk, any id_to_path entry pointing at it yields a warning instead of being auto-cleaned.\n\n## Scope of this bead (deliberately not overlapping ddx-12cae4dd)\n\nAssume `.claude/worktrees` noise is gone. This bead delivers the **integrity surface** that remains valuable: a way to see, categorize, and act on real document problems.\n\n### Deliverables\n\n1. **Structured warnings** — extend `docgraph.Graph.Warnings` from `[]string` to `[]GraphIssue{kind, path, id, message, relatedPath?}` and plumb through GraphQL. Keep a `messageLines(): []string` shim for any caller still using the old shape during the grace period.\n2. **Integrity panel on /graph** — replace the flat amber block with a collapsible panel grouped by issue kind (Duplicate ID, Missing dep target, Broken id_to_path, Parse error, Required root missing). Each group shows count + expandable list; each row shows file path (clickable → document viewer), the offending ID, and a copyable message.\n3. **Summary badge** — small badge next to the page title showing total issue count, so a clean graph reads clean.\n4. **Audit GraphQL query** — `query { docGraphIssues { kind, path, id, message, relatedPath } }` usable independently of the full graph for future dashboard use. Reuses the same structured slice.\n5. **Repair affordances (minimum viable)** — for two common cases:\n   - Duplicate ID: show both file paths side-by-side and a \"copy suggested unique ID\" button (deterministic suggestion, e.g., append a short hash of the path). No auto-write.\n   - Missing dep target: show the document declaring the missing dep, and a copy-to-clipboard snippet of the frontmatter line to remove. No auto-write.\n   Auto-repair is explicitly out of scope until we trust the detection.\n6. **CLI parity** — `ddx doctor` (or `ddx docs audit`) prints the same structured issues in a terminal-friendly form, so headless users and CI can surface problems.\n\n### Out of scope\n\n- Fixing `.claude/worktrees` indexing (owned by `ddx-12cae4dd`).\n- Auto-rewrite of source files.\n- Cross-project graph integrity (today's graph is per-project).",
+    "acceptance": "**User story:** As a developer opening the document graph, I want a structured integrity panel that groups real document problems by kind and points me at the files to fix, so I can triage a noisy graph quickly and confirm when it is healthy.\n\n**Acceptance criteria:**\n\n1. `docgraph.Graph.Warnings` (or a sibling `Issues` field) is a typed slice with `Kind`, `Path`, `ID`, `Message`, and optional `RelatedPath`. Kinds include at minimum: `duplicate_id`, `parse_error`, `missing_dep`, `id_path_missing`, `id_path_mismatch`, `required_root_missing`, `cascade_unknown`.\n2. GraphQL schema exposes `DocGraph.issues: [GraphIssue!]!` with the same fields. The existing string `warnings` either stays (rendered from issues) or is deprecated with a schema comment.\n3. Unit tests in `cli/internal/docgraph/docgraph_test.go` cover each issue kind: fixtures that produce exactly one instance of that kind and assert the typed output.\n4. `/graph` page renders an **Integrity** panel when `issues.length \u003e 0`:\n   - grouped by `kind` with a count per group;\n   - each row shows path, id, and message;\n   - path links to the document viewer (reuses existing documents route);\n   - total-issues badge appears next to the page title.\n5. With zero issues, neither the panel nor the badge renders, and the graph view is visually identical to a healthy graph today.\n6. Duplicate-ID rows show both paths and a \"copy suggested unique ID\" button; the suggestion is deterministic (same input → same output) and a unit test asserts the suggestion function's stability.\n7. `ddx docs audit` (or `ddx doctor docs`) CLI command prints the same issue list grouped by kind, exits 0 if empty and 1 if any issues present. Integration test asserts exit codes and grouped output.\n8. Playwright e2e:\n   - navigates to `/graph` on a fixture repo with one seeded duplicate-id pair and one missing-dep target;\n   - asserts the Integrity panel shows groups \"Duplicate ID (1)\" and \"Missing dep target (1)\";\n   - expands the Duplicate ID group and asserts both paths are visible;\n   - clicks the first path and asserts navigation to the documents route for that file;\n   - on a clean fixture repo, asserts neither the panel nor the badge is present.\n\n**Dependency:** best sequenced after `ddx-12cae4dd` lands so the fixture repos are not polluted by `.claude/worktrees` noise, but implementation does not block on it — the two can proceed in parallel and this bead's tests use synthetic fixtures.",
+    "labels": [
+      "feat-007",
+      "feat-008",
+      "ui",
+      "docgraph"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-23T02:50:34Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2988370",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"resolved_model\":\"qwen/qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:00:13.622458386Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen/qwen3.6-35b-a3b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen/qwen3.6-35b-a3b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-22T21:00:13.818699505Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"codex/gpt-5.4\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:00:16.490910037Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=codex/gpt-5.4"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness=claude model=codex/gpt-5.4 probe=ok\nunsupported model \"codex/gpt-5.4\" for harness \"claude\"; supported models: sonnet, opus, claude-sonnet-4-6",
+          "created_at": "2026-04-22T21:00:16.687719156Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"gemini\",\"resolved_model\":\"minimax/minimax-m2.7\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:00:19.224377826Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=gemini model=minimax/minimax-m2.7"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness=gemini model=minimax/minimax-m2.7 probe=ok\nunsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+          "created_at": "2026-04-22T21:00:19.420047697Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2282},{\"tier\":\"standard\",\"harness\":\"claude\",\"model\":\"codex/gpt-5.4\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2009},{\"tier\":\"smart\",\"harness\":\"gemini\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2009}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-22T21:00:19.481391495Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=3 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite\ntier=smart\nprobe_result=ok\nresult_rev=3dd6e52825ffc24901d649952b5d35692a9d7cf9\nbase_rev=3dd6e52825ffc24901d649952b5d35692a9d7cf9\nretry_after=2026-04-23T03:00:19Z",
+          "created_at": "2026-04-22T21:00:19.657303154Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-23T02:37:29.897481923Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260423T020941-3eb7703d\",\"harness\":\"claude\",\"input_tokens\":224,\"output_tokens\":77828,\"total_tokens\":78052,\"cost_usd\":19.065060249999995,\"duration_ms\":1667642,\"exit_code\":0}",
+          "created_at": "2026-04-23T02:37:29.970719143Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=78052 cost_usd=19.0651"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-23T02:37:33.069481037Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "REQUEST_CHANGES\n**AC 7 — command path**: The command is registered as `ddx doc audit` (`cli/cmd/doc.go:59`), but the acceptance criteria specifies `ddx docs audit` (or `ddx doctor docs`). If `doc` is the existing noun, this is arguably fine, but the AC should be reconciled. The `Aliases: []string{\"integrity\"}` alias doesn't cover the spec'd names.\nartifact: .ddx/executions/20260423T020941-3eb7703d/reviewer-stream.log",
+          "created_at": "2026-04-23T02:38:21.902871406Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "REQUEST_CHANGES"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-23T02:38:21.976077459Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: REQUEST_CHANGES"
+        },
+        {
+          "actor": "ddx",
+          "body": "post-merge review: REQUEST_CHANGES\n**AC 7 — command path**: The command is registered as `ddx doc audit` (`cli/cmd/doc.go:59`), but the acceptance criteria specifies `ddx docs audit` (or `ddx doctor docs`). If `doc` is the existing noun, this is arguably fine, but the AC should be reconciled. The `Aliases: []string{\"integrity\"}` alias doesn't cover the spec'd names.\n**AC 7 — `--json` exit code**: `cli/cmd/doc.go:79-83` — when `--json` is set, the command encodes issues and returns `nil` unconditionally. The AC requires exit 1 when issues are present regardless of output format, so CI pipelines using `ddx doc audit --json` would incorrectly report success on a broken graph. The test `TestDocAuditCommand_JSONOutput` even documents this as intentional (\"--json always succeeds (exit 0) so piping into jq works\"), which contradicts the AC. Either the AC should be amended or the `--json` path should also return `ExitError` when issues exist (with a `--exit-zero` escape hatch for pipe-friendly use).\nresult_rev=61a8029103abff2e64442ff7b81ab92d04d84f69\nbase_rev=48f9750c2e18e96af665670c653a8f4f72e27ebf",
+          "created_at": "2026-04-23T02:38:22.039899928Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_request_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-23T02:50:34.350905963Z",
+      "execute-loop-last-detail": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+      "execute-loop-last-status": "execution_failed",
+      "feature": "FEAT-007"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260423T025034-5febdfb4",
+    "prompt": ".ddx/executions/20260423T025034-5febdfb4/prompt.md",
+    "manifest": ".ddx/executions/20260423T025034-5febdfb4/manifest.json",
+    "result": ".ddx/executions/20260423T025034-5febdfb4/result.json",
+    "checks": ".ddx/executions/20260423T025034-5febdfb4/checks.json",
+    "usage": ".ddx/executions/20260423T025034-5febdfb4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-6ff597ca-20260423T025034-5febdfb4"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260423T025034-5febdfb4/result.json b/.ddx/executions/20260423T025034-5febdfb4/result.json
new file mode 100644
index 00000000..f1128dfa
--- /dev/null
+++ b/.ddx/executions/20260423T025034-5febdfb4/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-6ff597ca",
+  "attempt_id": "20260423T025034-5febdfb4",
+  "base_rev": "dba129f088089b5a4e5de453501f507a66e11825",
+  "result_rev": "3b7bc8860c7153d181e1c2e4f23a81ffdc32135b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-5928ff21",
+  "duration_ms": 585487,
+  "tokens": 6463225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T025034-5febdfb4",
+  "prompt_file": ".ddx/executions/20260423T025034-5febdfb4/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T025034-5febdfb4/manifest.json",
+  "result_file": ".ddx/executions/20260423T025034-5febdfb4/result.json",
+  "usage_file": ".ddx/executions/20260423T025034-5febdfb4/usage.json",
+  "started_at": "2026-04-23T02:50:34.89349038Z",
+  "finished_at": "2026-04-23T03:00:20.380505465Z"
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
## Review: ddx-6ff597ca iter 1

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
