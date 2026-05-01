<bead-review>
  <bead id="ddx-23978824" iter=1>
    <title>Providers pane: unify with harnesses (include claude-code and codex), show quota utilization + trend</title>
    <description>
## Observed

Three related issues on `/nodes/.../providers`:

1. **Loads slowly.** The page fires two GraphQL queries on mount — `providerStatuses` (which probes every configured provider for live connectivity) and `defaultRouteStatus` (which resolves the default route). Both are synchronous and serialized only by `Promise.all`. Each provider probe is a real network call.
2. **Missing harnesses.** The pane shows API-endpoint providers (qwen, minimax, etc. — anything wired through the service with an endpoint). It does not show subprocess-invoked harnesses like `claude` (Claude Code) or `codex`. That is accurate to the current `providerStatuses` resolver (`cli/internal/server/graphql/resolver_providers.go:15-50`, which calls `svc.ListProviders`) but not what an operator wants: the operator thinks of "where their agent work can run," which includes both endpoint providers AND subprocess harnesses.
3. **No quota visibility.** Operator need: see current token utilization rate + quota utilization per harness/provider, trend it, and use the trend to decide whether to throttle before hitting a cap.

## Root cause by area

- **Slow load:** `svc.ListProviders` probes every configured provider for liveness. Probes are serial within the service call, and the whole query blocks page render.
- **Missing harnesses:** Subprocess harnesses (claude, codex, gemini) are first-class in the CLI (`ddx agent list`, `ddx agent doctor`, `ddx agent check`) but are not surfaced via `providerStatuses`. They have their own status concepts — binary installed, credentials configured, rate-limit headroom — that don't map cleanly onto the endpoint-provider model.
- **No quota:** No existing data source captures rolling token usage vs quota. Quota information varies wildly by harness: Claude Code has message / session headroom concepts from response headers; codex has rate-limit headers; endpoint providers have API quota per key. There is no unified representation today.

## Proposed direction

Three deliverables, sequenced from easy to hard.

### Deliverable 1 — Unified "Agent endpoints" view (renames/rescopes the page)

- Rename the surface from "Providers" to **"Agent endpoints"** (or similar — confirm copy in the design note). The domain object is "a place where agent work can run," not "an API provider."
- Merge the data: `providerStatuses` stays as the source for endpoint providers. A new `harnessStatuses` resolver (read from `svc.ListHarnesses` / existing `ddx agent list` + `ddx agent doctor` machinery) surfaces subprocess harnesses. Page renders both in one table with a `kind` column (`endpoint` | `harness`) — or two tables if design prefers.
- Status fields common to both: name, kind, reachable (bool), detail (text), last-checked-at, default-for-profile (set of profile names).
- Asynchronous liveness. The page renders an instant first paint from cached status; liveness probes fire in background and patch rows as they return. No synchronous probe-wall on load.

### Deliverable 2 — Token &amp; quota fields

- Extend the status payload (per row) with: `usage { tokensUsedLastHour, tokensUsedLast24h, requestsLastHour, requestsLast24h }` and `quota { ceilingTokens?, ceilingWindowSeconds?, remaining?, resetAt? }`. Any field may be null if the provider/harness doesn't expose it.
- Data sources:
  - **Endpoint providers:** aggregate from the sessions index (`ddx-2ceb02fa`) filtered to the last 1h / 24h, grouped by provider. Quota ceilings come from any headers we capture during calls, plus optional static config.
  - **Subprocess harnesses:** same sessions-index aggregation for usage. Quota — parse rate-limit headers from the harness output where available (Claude Code, codex both emit them). For harnesses that don't, leave quota null and render "not reported."
- Row gains two small inline displays: utilization bar (used / ceiling for the quota window) and a 7-day sparkline of tokens/hour.

### Deliverable 3 — Trend view

- Click into a row → detail page showing: 7-day and 30-day time-series of tokens/hour and requests/hour, with the quota ceiling overlaid. Aggregations come from the sessions index, bucketed server-side to avoid shipping raw rows.
- Simple "projected to hit quota in ~X hours/days" callout computed from the slope of the last 24h — enough to answer "are we on a trend to run out?" without building a forecasting system.

## Cross-bead dependencies

- Depends on `ddx-2ceb02fa` (sessions index) for any usage-rate data. If that bead isn't landed when this work starts, the usage/quota features stub to "not available" and ship purely with the unified-view rename + async liveness.
- Fixture / perf guidance: per the standing directive recorded in `ddx-9ce6842a`, any aggregation here is tested against ≥1k session records. If in-place aggregation can't hit perf targets, bring the DB-substrate question to the user before picking one.
    </description>
    <acceptance>
**User story:** As an operator, I want a single view that shows every place my agents can run — endpoint providers and subprocess harnesses alike — with status, recent token usage, and quota utilization. When I click into one, I see a 7-day trend so I can tell if I'm about to run out.

**Acceptance criteria:**

1. **Deliverable 1 — Unified view.**
   - New `harnessStatuses` GraphQL resolver returns subprocess harnesses (claude, codex, gemini, agent) with: name, kind=`harness`, reachable, detail, lastCheckedAt, defaultForProfile.
   - Existing `providerStatuses` shape gains `kind=\"endpoint\"` and `lastCheckedAt`.
   - UI page loads within 500ms on a warm cache — it renders from last-known status **first**, then patches rows live as async probes return. Playwright asserts the table is interactive within 500ms of navigation and that row status patches within 5s of the probe completing.
   - Subprocess harnesses claude and codex **appear in the table** on a machine where their binaries are installed, with correct reachable status. Integration test.

2. **Deliverable 2 — Usage + quota fields (stubbable).**
   - Per-row fields `usage` and `quota` as described. All fields nullable.
   - For endpoint providers, `usage.tokensUsedLast24h` and `usage.tokensUsedLastHour` are computed from the sessions index (fixture: ≥1k session rows covering the last 24h). Unit test asserts correctness on a seeded window.
   - For subprocess harnesses that expose rate-limit headers (claude, codex), parsed headers populate `quota.ceilingTokens`, `quota.ceilingWindowSeconds`, `quota.remaining`, `quota.resetAt`. Parser tested with captured header fixtures from each harness.
   - For harnesses/providers that expose nothing, fields remain null and the UI renders `—` / "not reported"; no fabricated data.
   - Utilization bar renders when both `usage` and `quota.ceilingTokens` are present. Sparkline renders when ≥6 hourly buckets of usage are available.

3. **Deliverable 3 — Trend detail.**
   - Detail route `/nodes/.../providers/[name]` shows 7-day and 30-day tokens/hour + requests/hour time series, with quota ceiling overlay when known.
   - Buckets computed server-side from the sessions index; the client receives ≤ 200 data points per series.
   - "Projected to hit quota in ~X" callout uses last-24h slope only. Renders only when `quota.ceilingTokens` is known and the last-24h slope is non-zero positive; otherwise absent.
   - Playwright asserts the detail route renders with seeded usage and the projection text appears for a fixture that has a ceiling + ascending usage.

4. **Perf posture.**
   - Fixture for perf test: ≥1,000 session index rows covering the last 30 days with usage distributed across ≥5 endpoints/harnesses. Per standing directive in `ddx-9ce6842a`.
   - Targets: unified list p95 ≤ 200ms HTTP (excluding async probes); trend detail p95 ≤ 400ms HTTP on the fixture.
   - If in-place aggregation cannot hit the detail-view p95 on a 10,000-row stretch fixture, pause and raise the DB-substrate decision with the user per the standing directive — do not silently switch.

5. **Design note.**
   - One-pager in `docs/helix/02-design/` before implementation recording the page-rename decision, the row model (common fields, kind-specific fields), async-probe strategy, and the quota source per harness/provider. PR links to it.

6. **No regressions.**
   - `ddx agent providers`, `ddx agent doctor`, `ddx agent list`, `ddx agent check` CLI commands continue to work unchanged.
   - `defaultRouteStatus` query remains.

7. **Cross-references.**
   - Depends on `ddx-2ceb02fa` (sessions index) for usage aggregation. If that bead is not yet landed when this work starts, ship Deliverable 1 standalone with usage/quota stubbed as "not available," and re-open the remaining deliverables when the index is ready.
    </acceptance>
    <labels>feat-008, feat-006, feat-014, providers, observability</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="727c5fdb24e9f74ca14e0ba8075156cf62a85ead">
commit 727c5fdb24e9f74ca14e0ba8075156cf62a85ead
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 01:46:15 2026 -0400

    chore: add execution evidence [20260423T052255-]

diff --git a/.ddx/executions/20260423T052255-c8baf04c/result.json b/.ddx/executions/20260423T052255-c8baf04c/result.json
new file mode 100644
index 00000000..e563ef51
--- /dev/null
+++ b/.ddx/executions/20260423T052255-c8baf04c/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-23978824",
+  "attempt_id": "20260423T052255-c8baf04c",
+  "base_rev": "63e1501be452f7bc80a5d5151331292af43fe241",
+  "result_rev": "2173f92b6322462f5c0f76681fddcb5631622372",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-d7f164d2",
+  "duration_ms": 1397986,
+  "tokens": 15935069,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T052255-c8baf04c",
+  "prompt_file": ".ddx/executions/20260423T052255-c8baf04c/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T052255-c8baf04c/manifest.json",
+  "result_file": ".ddx/executions/20260423T052255-c8baf04c/result.json",
+  "usage_file": ".ddx/executions/20260423T052255-c8baf04c/usage.json",
+  "started_at": "2026-04-23T05:22:56.369874723Z",
+  "finished_at": "2026-04-23T05:46:14.356094576Z"
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
## Review: ddx-23978824 iter 1

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
