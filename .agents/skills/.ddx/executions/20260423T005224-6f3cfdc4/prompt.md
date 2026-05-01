<bead-review>
  <bead id="ddx-9ce6842a" iter=1>
    <title>Bead list and cross-project all-beads are too slow: establish a GraphQL perf harness and fix N·M scan pattern</title>
    <description>
## Observed

Per-project `/beads` page is noticeably slow; the cross-project "all beads" view is substantially slower. Related: `ddx-ad0db8fd` already identified the single-bead-detail slowness caused by the same storage pattern.

## Root cause (same pattern, different call site)

`cli/internal/server/graphql/resolver_beads.go:22,40` —

- `Beads` (cross-project) and `BeadsByProject` (single-project) both call `r.State.GetBeadSnapshots(status, label, projectID, search)`.
- `GetBeadSnapshots` (`cli/internal/server/state_graphql.go:49-105`) iterates every registered project, constructs a `bead.Store` per project, calls `ReadAll()` (scan + JSONL parse of every bead file), filters in Go. Cost is `O(projects × beads_per_project)` on every call, independent of the filter/page size.
- All filtering (status, label, project, search) is Go-side, post-read. The page-size window is applied in `beadConnectionFromSnapshots` after the full corpus is already materialized.
- Cross-project "all beads" = same loop, just without the projectID filter. It is the worst case by design.

This is the same shape that's breaking the Sessions feed (`ddx-2ceb02fa`) and single-bead detail (`ddx-ad0db8fd`). A point fix here is insufficient — we need a measurement surface first so we stop relying on anecdote.

## Proposed direction

### Part 1 — Perf harness (foundational; enables everything else)

Introduce a reusable GraphQL perf harness under `cli/internal/server/perf/` or `cli/internal/server/graphql/bench_test.go`:

- Seeded fixture builder: generates N projects × M beads with configurable distribution of status/labels/dependencies. Reuses existing store primitives (no fake data layer).
- Go benchmark + scriptable HTTP runner (wrapper around an already-running server) reporting p50/p95/p99 per query shape.
- Targets covered on day one: `bead(id:)`, `beadsByProject(projectID, first, after, status, label, search)`, `beads(projectID=nil, first, after, status, label)`, `docGraph`, `sessions` (so the sessions shard work `ddx-2ceb02fa` inherits this harness). New query targets are added by writing a single table row.
- Thresholds are **recorded, not asserted**, in the first pass. CI surfaces a regression if a threshold is breached beyond a configured delta (e.g., +25% p95). Initial pass is "land the harness and record baseline"; a follow-up wires it into CI.

### Part 2 — Fix the scan pattern

Minimum viable, in priority order:

1. **Scope by project when the query is scoped.** `BeadsByProject` should call a new `GetBeadSnapshotsForProject(projectID, filters…)` that only opens that one project's store. Trivial change; eliminates the cross-project factor for the common case.
2. **Early filter pushdown.** `GetBeadSnapshots` should apply status/label/projectID filters at the file-parse boundary where cheap, not after materializing every bead. (Still reads every file — deeper fix below.)
3. **Lightweight in-memory index with invalidation.** After (1) and (2), if "all beads" or search is still slow, introduce an index keyed `{projectID, beadID}` with basic metadata (status, labels, title, updatedAt). Refresh on bead write + debounced filesystem watch. Full-body reads stay lazy — the index never holds bodies. This is the same storage discipline we adopted for sessions in `ddx-2ceb02fa`.

The harness from Part 1 is the gate for choosing between (2) and (3); don't commit to the index without numbers.

## Out of scope

- Full-text search infrastructure (tracked separately if needed).
- Changing the on-disk bead format.
- Cross-node federation performance. Current concern is a single server reading a single workspace.
    </description>
    <acceptance>
**User story:** As a developer using the web UI on a real-world-sized workspace (tens of projects × hundreds of beads each), the beads list opens in under a second and the cross-project all-beads view stays usable. Before any perf fix lands, we can reproduce the problem and measure improvement — not guess.

**Acceptance criteria:**

1. **Perf harness exists and is reusable.**
   - Fixture builder generates `N × M` beads across projects with configurable shape (status mix, label density, dep count). Reuses the real `bead.Store` write path (no fake writer).
   - Benchmark covers at minimum: `bead(id:)`, `beadsByProject`, `beads` (cross-project), `docGraph`, `sessions`. Adding a new target is a single-table-row change.
   - Each target produces p50/p95/p99 for in-process calls and end-to-end HTTP. Results write to a human-readable report (markdown or JSON) plus a machine-readable artifact suitable for CI diffing later.

2. **Baseline recorded.** First run of the harness on the current `main` binary is stored in the bead notes and in `docs/helix/04-observe/perf/YYYY-MM-DD-baseline.md` (or equivalent existing location). Numbers include hardware + go version footer.

3. **BeadsByProject scope fix.** `BeadsByProject` calls a new `GetBeadSnapshotsForProject(projectID, …)` that opens exactly one project's store. Unit test asserts only that project's store is opened (no iteration over other projects).

4. **Filter pushdown.** Status/label/projectID filters apply at the per-file / per-entry parse boundary, not post-materialization. Benchmark captures before/after for the common filtered case (e.g., `status=open`).

5. **Perf targets on fixture (10 projects × 500 beads = 5000 beads total):**
   - `bead(id:)` p95 ≤ 50ms in-process / ≤ 200ms HTTP (already covered by `ddx-ad0db8fd`; this bead may inherit the work).
   - `beadsByProject(first=50)` p95 ≤ 75ms in-process / ≤ 250ms HTTP.
   - `beads(first=50, no projectID)` p95 ≤ 200ms in-process / ≤ 500ms HTTP.
   These are targets for the **first** pass (scope + pushdown). If they are not met with (1)+(2), escalate to the in-memory index (step 3 of Part 2 in the description) and update these targets with the new baseline.

6. **Stale-read safety.** Regardless of fix chosen, a test writes a bead via `ddx bead update`, then queries it via GraphQL, then asserts the new value is visible on the very next request. Works under concurrency: N concurrent readers + writers interleaved without torn reads or panics.

7. **CI posture (soft).** The harness is runnable via `make bench-graphql` (or the project's equivalent). CI wiring to fail on &gt;25% regression is a follow-up bead — recorded in this bead's notes with the proposed threshold but not implemented here.

8. **Playwright smoke.** Opening `/beads` on the fixture project returns an interactive (clickable) list within 1s. Opening the cross-project `/beads` page returns within 2s. These are ceilings, not p95 targets — guards against outright regression, not fine-grained perf.

9. **Cross-reference.** This bead's notes list `ddx-ad0db8fd` (bead detail) and `ddx-2ceb02fa` (sessions shard/index) as the other two bead/session perf threads that should inherit this harness.
    </acceptance>
    <labels>feat-008, feat-004, perf, graphql</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7e79441bf5f93f93b1dbc066820ea7990533a3e7">
commit 7e79441bf5f93f93b1dbc066820ea7990533a3e7
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 20:52:22 2026 -0400

    chore: add execution evidence [20260423T002632-]

diff --git a/.ddx/executions/20260423T002632-1f60f8fb/result.json b/.ddx/executions/20260423T002632-1f60f8fb/result.json
new file mode 100644
index 00000000..f7dce9f5
--- /dev/null
+++ b/.ddx/executions/20260423T002632-1f60f8fb/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9ce6842a",
+  "attempt_id": "20260423T002632-1f60f8fb",
+  "base_rev": "a8d23c2381ca55bccd721ea1f3d0fb8debd9bf46",
+  "result_rev": "bc9b5dacf4da04cc1dd5ac383ef4ccdf170d09ab",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-7fc5ab6e",
+  "duration_ms": 1548122,
+  "tokens": 79582,
+  "cost_usd": 14.637292249999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T002632-1f60f8fb",
+  "prompt_file": ".ddx/executions/20260423T002632-1f60f8fb/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T002632-1f60f8fb/manifest.json",
+  "result_file": ".ddx/executions/20260423T002632-1f60f8fb/result.json",
+  "usage_file": ".ddx/executions/20260423T002632-1f60f8fb/usage.json",
+  "started_at": "2026-04-23T00:26:32.799287088Z",
+  "finished_at": "2026-04-23T00:52:20.92217389Z"
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
## Review: ddx-9ce6842a iter 1

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
