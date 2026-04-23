<bead-review>
  <bead id="ddx-726bc5f2" iter=1>
    <title>Add 'Executions' view: browse execute-bead bundles and drill into manifest, prompt, result, session, and tool calls</title>
    <description>
## Observed / Motivation

`.ddx/executions/&lt;ts&gt;-&lt;hash&gt;/{manifest.json, prompt.md, result.json}` bundles are already being produced per `ddx agent execute-bead` attempt — 469 of them on the current repo. They contain the canonical record of what an agent was asked to do and what verdict the attempt produced. **Nothing in the web UI surfaces them today.** A user who wants to ask "why did this bead end up in this state?" has to drop to the filesystem.

Sessions (`sessions.jsonl` / sharded index per `ddx-2ceb02fa`) carry the harness/cost/token telemetry but not the verdict and not the bead linkage. Agent log streams (`agent-*.jsonl`) carry the tool-call trace but are scattered across hundreds of files. **Executions are the natural join** — they connect a bead, a session, a prompt, a verdict, and a tool-call stream.

## Ground truth

Per-execution on disk:

```
.ddx/executions/&lt;ts&gt;-&lt;hash&gt;/
  manifest.json      { harness, model, base_rev, result_rev, verdict, bead_id, execution_dir }
  prompt.md          the prompt that was sent
  result.json        { verdict, rationale, ... }
```

Relations:
- `manifest.bead_id` → bead
- a session id is recorded in bead events (`kind:cost`, `kind:routing`) and in the agent-log stream; both can map back to an execution via correlation metadata
- tool calls live in `.ddx/agent-logs/agent-*.jsonl` (and `agent-s-*.jsonl`), keyed by session id

## Scope

### Part 1 — GraphQL surface

- `executions(projectId, first, after, beadId?, since?, until?, verdict?, harness?)` — paginated list of execution bundles.
- `execution(id)` — one bundle, fully materialized: manifest fields, prompt markdown, result JSON, linked bead (id + title), linked session (id + harness + model + cost + token totals), tool-call stream pointer.
- `executionToolCalls(id, first, after)` — paginated tool-call stream for the execution, read lazily from the agent-log file. Matches the event shape used by the workers detail page.

Executions source of truth: directory scan of `.ddx/executions/` filtered/paginated at the resolver. Shard discipline: follow the sessions-index monthly shard pattern (`ddx-2ceb02fa`) if listing all executions grows beyond tens of thousands — per standing directive `ddx-9ce6842a`, raise the storage decision with the user before switching substrate.

### Part 2 — Sidebar + routes

- Add "Executions" to the project sidebar (`NavShell.svelte`).
- Routes:
  - `/nodes/&lt;n&gt;/projects/&lt;p&gt;/executions` — list view with filters (verdict, harness, bead, date range, free-text over prompt). Table columns: timestamp, bead, harness/model, verdict (pass/block/error), duration, cost.
  - `/nodes/&lt;n&gt;/projects/&lt;p&gt;/executions/&lt;id&gt;` — detail view with five tabs: **Manifest**, **Prompt** (rendered markdown), **Result** (rationale + raw JSON toggle), **Session** (harness, model, cost, token totals, link to `/sessions/&lt;id&gt;`), **Tool calls** (virtualized lazy-scroll list of tool_use + tool_result pairs with expandable payloads).

### Part 3 — Cross-links (make the graph real)

- **Bead detail:** new "Executions" section listing all executions for this bead with verdict + timestamp + link. Already have data: manifest.bead_id.
- **Session detail:** new "Execution" link pointing to the execution this session ran for, when the session has a linked execution.
- **Commits page:** already shows commits; if a commit was produced by an execution (result_rev == commit sha), surface the execution link.
- **Graph/Docs pages:** out of scope for this bead.

### Part 4 — Perf

- Fixture: ≥1,000 executions, per standing directive `ddx-9ce6842a`. Tool-call streams seeded for 10% of them.
- Targets: list view p95 ≤ 200ms HTTP, detail view without tool calls p95 ≤ 150ms HTTP, tool-call page-1 (first 50) p95 ≤ 300ms HTTP.
- If these can't be met with filesystem scan + in-memory filter, raise DB-substrate decision with user per standing directive.

## Out of scope

- Writing new execution bundles (that's already happening).
- Executions from other projects/nodes (keep per-project for v1).
- Tool-call payload redaction / privacy. File a follow-up if relevant.
- `.claude/worktrees/` worktree lifecycle tracking — covered by companion bead per user's split request.
    </description>
    <acceptance>
**User story:** As a developer reviewing what my agents have done, I open the Executions view, filter to a specific bead or verdict or harness, click into one, and see the prompt it received, the verdict it produced, the session it corresponds to (with cost + tokens), and every tool call it made — all without leaving the browser.

**Acceptance criteria:**

1. **Sidebar entry.** "Executions" added to the project sidebar between "Sessions" and "Personas" (or wherever fits the existing visual order — design call; captured in the PR description). Active-highlight works.

2. **List route.** `/executions` renders a paginated, filterable table with columns: timestamp, bead (id + title as link), harness/model, verdict (color-coded pass/block/error/other), duration, cost. Default sort: newest first. Filters: verdict, harness, bead, since/until date, free-text search over prompt first ~200 chars.

3. **Detail route.** `/executions/&lt;id&gt;` shows five tabs as described (Manifest, Prompt, Result, Session, Tool calls). Tab switching doesn't re-fetch already-loaded data.

4. **Tool calls pane.** Virtualized list. First 50 calls load with the page; subsequent pages load on scroll. Each entry shows tool name + collapsed input/output, expandable on click. Links to the raw agent-log file for operators who want to diff.

5. **GraphQL completeness.** `executions`, `execution`, `executionToolCalls` resolvers exist and pass feat-008-style integration tests. Schema documented in `schema.graphql`.

6. **Cross-links present.**
   - Bead detail renders an "Executions" section listing linked executions with verdict + timestamp, each clicking into the detail route.
   - Session detail shows "Execution" link when the session has one.
   - Commits page shows execution link when commit sha == execution.result_rev.

7. **Fixture + perf.**
   - Seeded fixture: ≥1,000 executions, ≥100 with seeded tool-call streams (≥50 calls each).
   - List view on this fixture: p95 ≤ 200ms HTTP.
   - Detail view (no tool calls): p95 ≤ 150ms HTTP.
   - Tool-call page 1 (first 50 entries): p95 ≤ 300ms HTTP.
   - If any target cannot be met in-place, raise DB-substrate decision with user before switching.

8. **Playwright e2e.**
   - Seeds a project with 3 executions across 2 beads with mixed verdicts.
   - Navigates to `/executions`, asserts the table renders 3 rows and filter-by-verdict narrows correctly.
   - Clicks into one execution, asserts all 5 tabs exist and the Prompt tab shows the seeded prompt.
   - Opens the Tool calls tab, expands an entry, asserts input/output rendering.
   - Clicks the linked bead from the detail, asserts navigation to bead detail and that the Executions section on that bead lists this execution.

9. **Depends on.**
   - Soft dependency on `ddx-2ceb02fa` for the Session tab's cost/token numbers. If not yet landed, render "not available" for those fields and wire them in after.
   - Adjacent: `ddx-6904a90b` (billing mode) — Session tab's cost shows the correct bucket once that lands.
    </acceptance>
    <labels>feat-008, feat-010, feat-006, ui, executions</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="6eee3548dbeed8d5b27776ccf8b37ff923878cd1">
commit 6eee3548dbeed8d5b27776ccf8b37ff923878cd1
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 02:07:14 2026 -0400

    chore: add execution evidence [20260423T053812-]

diff --git a/.ddx/executions/20260423T053812-719b9c5f/result.json b/.ddx/executions/20260423T053812-719b9c5f/result.json
new file mode 100644
index 00000000..95f98577
--- /dev/null
+++ b/.ddx/executions/20260423T053812-719b9c5f/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-726bc5f2",
+  "attempt_id": "20260423T053812-719b9c5f",
+  "base_rev": "acee1b1720b38ae13859f82fdeb9347678b645d7",
+  "result_rev": "fcb8a93c400421416fb88c3d774068a10501e968",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6c03ea9b",
+  "duration_ms": 1739848,
+  "tokens": 87481,
+  "cost_usd": 19.462767600000003,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T053812-719b9c5f",
+  "prompt_file": ".ddx/executions/20260423T053812-719b9c5f/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T053812-719b9c5f/manifest.json",
+  "result_file": ".ddx/executions/20260423T053812-719b9c5f/result.json",
+  "usage_file": ".ddx/executions/20260423T053812-719b9c5f/usage.json",
+  "started_at": "2026-04-23T05:38:13.364392355Z",
+  "finished_at": "2026-04-23T06:07:13.213086232Z"
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
## Review: ddx-726bc5f2 iter 1

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
