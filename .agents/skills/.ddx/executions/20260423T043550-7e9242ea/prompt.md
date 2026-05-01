<bead-review>
  <bead id="ddx-0a33bc5f" iter=1>
    <title>Efficacy page is slow and sources from the wrong place: rework as session aggregation, seed ≥1k fixture, raise DB decision if in-place insufficient</title>
    <description>
## Observed

The Efficacy page at `/nodes/.../projects/.../efficacy` loads very slowly. User feedback: efficacy should really be "an aggregation of sessions grouped by provider / harness / model" — which is not what the current implementation does.

## Ground truth (current implementation)

`cli/internal/server/graphql/resolver_feat008.go:676-833` —

- `EfficacyRows` builds its rollup from **bead evidence events** (`kind:routing` and `kind:cost` on every closed bead), not from sessions.
- Computation path: `store.ReadAll()` → for every closed bead, read its event file → filter for routing/cost → per-(harness, provider, model) aggregate.
- There is a cache (`efficacyMemo`), keyed by a fingerprint that itself walks every event on every closed bead to compute event count + max timestamp. So cache validation costs almost as much as cache rebuild.
- Cost shape: `O(closed_beads × events_per_bead)` on every request (cache miss) and even on cache hit the fingerprint walk happens.

## Why this is wrong (per user feedback)

- Efficacy is a sessions-view concept: "how is harness/provider/model X doing?" Sessions are the natural row. Bead events are a lossy, indirect proxy — they only exist for *closed* beads, they duplicate session state, and they force coupling between the efficacy feature and bead lifecycle.
- The bead-event path misses: open beads with in-flight sessions, sessions that didn't close a bead, benchmark/quorum sessions, non-execute-bead invocations (e.g., `ddx agent run`).
- The sessions feed is already being reworked into a sharded pointer index (`ddx-2ceb02fa`). Efficacy should read from that index — single source of truth.

## Proposed direction

### Part 1 — Reshape the data source

- Rebuild `EfficacyRows` as an aggregation over `sessions.index` (the sharded index from `ddx-2ceb02fa`), not over bead events.
- Grouping key: `(harness, provider, model)` — optionally filterable by project, bead, label, time range.
- Metrics per row: attempts (count), successes (count + rate), median input/output tokens, median duration, median cost. Identical shape to today; only the source changes.
- Shard awareness: time-range queries map to a shard set (same pattern as `ddx-2ceb02fa`); the default view reads current + previous shard.

### Part 2 — Fixture + baseline (honor user's 1,000+ record floor)

- Perf harness target: aggregate ≥**1,000 session rows** (≥ 10 distinct `(harness, provider, model)` groups) within the budget below.
- Stretch fixture: 50,000 session rows across 24 monthly shards. Measures steady-state behavior as the index grows.
- Baseline captured in the bead notes before any optimization.

### Part 3 — Perf targets (first pass, in-place)

- Default Efficacy view (all-time) on a 10k-session, 2-shard fixture: p95 ≤ 150ms in-process / ≤ 400ms HTTP.
- Date-filtered view (last 30 days, 1 shard): p95 ≤ 60ms in-process / ≤ 200ms HTTP.
- If in-place optimization (scope, pushdown, memoize-per-shard, streaming aggregation) **cannot hit these numbers** on the 50k stretch fixture, **do not silently change substrate** — raise the decision with the user. Candidate backends named by the user: axon, sqlite, postgres. Bring a recommendation + rationale, not a fait accompli.

### Part 4 — Drop the bead-event read path

- After the rewrite, remove `efficacyFingerprint`, `buildEfficacySnapshot` event iteration, `attemptsFromEvidence`, and `efficacyMemo`. They become dead weight once the source moves.
- Evidence events on beads remain (they're bead audit trail, not efficacy data); nothing in bead lifecycle changes.

## Out of scope

- The `comparisons` / "compare arms" feature on this page (tracked separately — see companion bead).
- Project-scope filtering UI (the backend support can be added now; the UI widget is a follow-up).
- Cross-machine / federated efficacy rollups.
    </description>
    <acceptance>
**User story:** As a developer reviewing how different harness/provider/model combinations are performing, I expect the Efficacy page to load quickly on a workspace with tens of thousands of sessions, and I expect the rows to reflect every agent invocation — not just those tied to closed beads.

**Acceptance criteria:**

1. **Source change.** `EfficacyRows` resolver reads from the sharded sessions index (`.ddx/agent-logs/sessions/sessions-YYYY-MM.jsonl`, per `ddx-2ceb02fa`). Bead-event read paths (`efficacyFingerprint`, `buildEfficacySnapshot`'s event loop, `attemptsFromEvidence`, `efficacyMemo`) are removed in the same PR — no dead code retained.

2. **Schema parity.** Row shape and field names stay as they are today (rowKey, harness, provider, model, attempts, successes, successRate, medianInputTokens, medianOutputTokens, medianDurationMs, medianCostUsd, warning). UI does not need to change. Optional new filters (`since`, `until`, `projectId`) are added as nullable args with sensible defaults.

3. **Fixture + baseline recorded.** Fixture seeds ≥10,000 session index rows across 2+ monthly shards with ≥10 distinct `(harness, provider, model)` groups. Baseline latencies (before any optimization beyond source change) written to bead notes with hardware + go-version footer.

4. **First-pass perf targets.**
   - All-time rollup on 10k-row, 2-shard fixture: p95 ≤ 150ms in-process / ≤ 400ms HTTP.
   - Last-30-days rollup on same fixture: p95 ≤ 60ms / ≤ 200ms HTTP.
   - Stretch fixture (50k rows × 24 shards): all-time p95 ≤ 400ms in-process / ≤ 1000ms HTTP, or **user is consulted** before choosing a new substrate.

5. **DB-decision protocol (per user directive 2026-04-22).**
   - If targets in (4) cannot be met with in-place optimization (streaming aggregation, per-shard memoization with correct invalidation, selective field read), the PR author posts a short proposal in the bead notes: measured achievable numbers, proposed substrate from {axon, sqlite, postgres}, and migration cost. Implementation of that substrate requires explicit user approval before landing.

6. **Completeness test.** A fixture seeds sessions that would not appear under the old bead-event path (open-bead sessions, `ddx agent run` sessions, benchmark sessions). The new resolver returns rows for each of them. A matching integration test on the old code path (kept as a golden-output comparison for one cycle) asserts the new output is a strict superset in the cases it should be.

7. **Freshness.** A session written via the sessions index writer (from `ddx-2ceb02fa`) is visible in the Efficacy query within 2s. Integration test writes a row and queries the aggregation.

8. **Playwright smoke.** `/efficacy` opens, renders the rollup table, and clicks into `EfficacyAttempts` for one row — all within normal navigation budgets. Uses the 10k-row fixture; asserts the table has ≥5 rows rendered.

9. **Cross-reference.** This bead is a dependent of `ddx-2ceb02fa` (sessions index must exist). If `ddx-2ceb02fa` is not yet landed when this work starts, bead notes record the stub / interim source chosen and the timeline for switching over.
    </acceptance>
    <labels>feat-008, feat-010, feat-019, efficacy, perf</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="822a1da04ffdf94089ae1874fa695fd62555d0db">
commit 822a1da04ffdf94089ae1874fa695fd62555d0db
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 00:35:48 2026 -0400

    chore: add execution evidence [20260423T041824-]

diff --git a/.ddx/executions/20260423T041824-57af88cc/result.json b/.ddx/executions/20260423T041824-57af88cc/result.json
new file mode 100644
index 00000000..03334480
--- /dev/null
+++ b/.ddx/executions/20260423T041824-57af88cc/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-0a33bc5f",
+  "attempt_id": "20260423T041824-57af88cc",
+  "base_rev": "44a7f9dafa47d90dcd92c96ccced9ae4836c9c11",
+  "result_rev": "41fa9657a837f92919aee4a10cc782c139e69001",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-409d4cca",
+  "duration_ms": 1042164,
+  "tokens": 43308,
+  "cost_usd": 8.5341195,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T041824-57af88cc",
+  "prompt_file": ".ddx/executions/20260423T041824-57af88cc/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T041824-57af88cc/manifest.json",
+  "result_file": ".ddx/executions/20260423T041824-57af88cc/result.json",
+  "usage_file": ".ddx/executions/20260423T041824-57af88cc/usage.json",
+  "started_at": "2026-04-23T04:18:25.276785425Z",
+  "finished_at": "2026-04-23T04:35:47.440866556Z"
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
## Review: ddx-0a33bc5f iter 1

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
