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

  <diff rev="61a8029103abff2e64442ff7b81ab92d04d84f69">
commit 61a8029103abff2e64442ff7b81ab92d04d84f69
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 22:37:31 2026 -0400

    chore: add execution evidence [20260423T020941-]

diff --git a/.ddx/executions/20260423T020941-3eb7703d/result.json b/.ddx/executions/20260423T020941-3eb7703d/result.json
new file mode 100644
index 00000000..6d7c2b0f
--- /dev/null
+++ b/.ddx/executions/20260423T020941-3eb7703d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-6ff597ca",
+  "attempt_id": "20260423T020941-3eb7703d",
+  "base_rev": "48f9750c2e18e96af665670c653a8f4f72e27ebf",
+  "result_rev": "3876fd78713713a47d2317b1dfd081ad7067d73e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-0b94e0e6",
+  "duration_ms": 1667642,
+  "tokens": 78052,
+  "cost_usd": 19.065060249999995,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T020941-3eb7703d",
+  "prompt_file": ".ddx/executions/20260423T020941-3eb7703d/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T020941-3eb7703d/manifest.json",
+  "result_file": ".ddx/executions/20260423T020941-3eb7703d/result.json",
+  "usage_file": ".ddx/executions/20260423T020941-3eb7703d/usage.json",
+  "started_at": "2026-04-23T02:09:42.251652103Z",
+  "finished_at": "2026-04-23T02:37:29.893878381Z"
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
