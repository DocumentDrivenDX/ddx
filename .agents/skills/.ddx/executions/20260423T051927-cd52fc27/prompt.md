<bead-review>
  <bead id="ddx-6904a90b" iter=1>
    <title>Sessions 'Total cost' is ambiguous: split into cash paid, subscription-equivalent, and local compute</title>
    <description>
## Observed

The Sessions page ($ sums `session.cost` into one "Total Cost" card, `cli/internal/server/frontend/src/routes/.../sessions/+page.svelte:21,28,91`). The underlying `cost_usd` field is fed from three very different billing realities and the UI treats them as one:

1. **Cash paid (pay-per-token).** API providers (OpenRouter, direct OpenAI, direct Anthropic API). `cost_usd` = actual billed amount.
2. **Subscription-equivalent.** Claude Code, Codex — flat-rate subscriptions. The CLI/harness emits `total_cost_usd` as the **dollar-equivalent** for tokens consumed under the subscription (what you would have paid pay-per-token). You paid a flat fee up front; this is a value-extracted number, not a cash-out number.
3. **Local compute.** Embedded `agent` harness, ollama/vllm endpoints. Today emits cost 0 because no one is billing; real cost is electricity + hardware amortization, not currently modeled.

A user reading "Total Cost: $X" does not know which of those X is.

## What's needed

Three independent rollups, each explicitly labeled, each computable without ambiguity:

1. **Cash** — sum of `cost_usd` across sessions whose billing mode is `paid`.
2. **Subscription-equivalent** — sum across sessions whose billing mode is `subscription`. Copy: "Dollar-equivalent for tokens consumed via Claude Code / Codex subscription. You did not pay this in cash; it's the pay-per-token rate you would have been charged."
3. **Local** — either a count of sessions (if no rate is configured) or a computed dollar estimate if `local_cost_per_1k_tokens` is set in config. Default: count-only, to avoid inventing numbers.

## Backing data model

Session records (the sharded index from `ddx-2ceb02fa`) need a new field `billingMode: "paid" | "subscription" | "local"`. Derive at write time:

- `paid`: harness is an API-endpoint provider (openrouter, openai-compat with an API key, anthropic API, …). Probably any harness with `Surface == "openai-compat"` or a known public-api provider prefix.
- `subscription`: harness is `claude` (Claude Code) or `codex` or `gemini-cli` when using a Google account. Identify by harness name + surface.
- `local`: harness is `agent` (embedded) OR endpoint is localhost / private IP / configured-as-local in provider config.

Classification lives in one place — a small `billingModeFor(harness, surface, baseURL)` function — so the logic isn't duplicated between writer and reader.

## Scope

### Part 1 — Data model

- Extend `SessionEntry` / the session index row with `billingMode string` (enum validated on write).
- Add `billingModeFor(...)` helper + unit tests for each harness/surface combination.
- Back-fill: `ddx agent log reindex` (from `ddx-2ceb02fa`) computes `billingMode` when re-shard-writing legacy sessions.

### Part 2 — Backend rollup

- Modify the sessions GraphQL query (or add a sibling `sessionsCostSummary(projectId, since?)`) to return `{ cashUsd, subscriptionEquivUsd, localSessionCount, localEstimatedUsd? }`.
- Reuse the perf-safe path from `ddx-0a33bc5f` (sessions-index aggregation, shard-aware date filter).

### Part 3 — UI

- Replace the single "Total Cost" card on the Sessions page with three cards:
  - **Cash paid** ($X.XX) — tooltip "Billed by pay-per-token APIs (OpenRouter, direct API keys)"
  - **Subscription value** ($X.XX) — tooltip "Dollar-equivalent for tokens consumed under Claude Code / Codex subscriptions. Not cash out of pocket."
  - **Local sessions** (N) — tooltip "Sessions served locally. Compute cost not modeled." If `local_cost_per_1k_tokens` is set, show "$X.XX est." instead of a count.
- Per-row: small badge `cash` / `sub` / `local` in the cost column, color-coded (green/blue/gray). Tooltip explains the row's bucket.

### Part 4 — Optional config

- `.ddx/config.yaml` gains `cost.local_per_1k_tokens: &lt;float&gt;`. When set, local-session total gets rendered as dollars. When unset, count-only. No auto-estimation.

## Out of scope

- Subscription flat-rate cost modeling ("I pay $20/month for Claude Code"). A useful adjacent bead: divide flat rate by consumption to show $/1M-tokens realized value. Not here.
- Quota trend / headroom. Covered by `ddx-23978824`.
- Reclassifying historical sessions beyond what reindex can infer from their stored harness/surface fields.
    </description>
    <acceptance>
**User story:** As a developer looking at the Sessions page, I can tell at a glance how much I actually paid in cash versus what I consumed under subscriptions versus what I ran locally. No number on the page conflates those three categories.

**Acceptance criteria:**

1. **`billingMode` field.** Session records (index rows + live writer) carry `billingMode` ∈ `{paid, subscription, local}`. Enum is validated on write. Unit test covers each value.

2. **Classifier.** `billingModeFor(harness, surface, baseURL)` is the single source of truth. Test table includes at minimum:
   - claude (Claude Code) → subscription
   - codex → subscription
   - openrouter (any model) → paid
   - openai/anthropic with API key → paid
   - agent (embedded) → local
   - openai-compat endpoint pointing at localhost/127.*/192.168.*/10.* → local
   - openai-compat endpoint pointing at a public host with an API key → paid

3. **Reindex backfill.** Running `ddx agent log reindex` (from `ddx-2ceb02fa`) on a fixture with sessions from all three modes produces index rows with correct `billingMode`. No row is written without one.

4. **Rollup resolver.** GraphQL exposes `sessionsCostSummary(projectId, since?, until?) { cashUsd, subscriptionEquivUsd, localSessionCount, localEstimatedUsd }`. Returns zeros/null correctly when no sessions match. Respects shard-scoped reads from `ddx-0a33bc5f`.

5. **UI — three cards.** The Sessions page replaces the "Total Cost" card with three cards as described (Cash paid, Subscription value, Local sessions). Tooltips explain each bucket. Playwright asserts all three render and that hover on each shows the explanatory text.

6. **UI — per-row badge.** Each session row's cost column shows a small `cash`/`sub`/`local` badge with a tooltip explaining the bucket. Playwright asserts badge presence and correct classification on a seeded fixture containing one session of each type.

7. **Zero-state.** When the project has zero sessions of a bucket, that card shows `—` / `0`, not `$0.00`, to avoid implying a paid-zero vs absent-data collision. Playwright asserts.

8. **Optional local cost config.** Setting `cost.local_per_1k_tokens` in `.ddx/config.yaml` causes the Local card to render as dollars using the configured rate × sum(totalTokens) for local sessions. Unset: count-only. Integration test covers both paths.

9. **Accessibility.** The three cards and per-row badges expose their bucket meaning via `aria-label`, not just tooltip, so screen-reader users get the same clarification.

10. **Cross-references.**
    - Depends on `ddx-2ceb02fa` (sessions index) — the `billingMode` field must land in the index schema.
    - Bead notes in `ddx-23978824` should reference this bead since the unified-endpoints view has overlapping concepts (what's a paid endpoint vs a subscription harness vs a local endpoint).
    </acceptance>
    <labels>feat-008, feat-010, cost, billing, clarity</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1e17b5fdc26696ad0c1c4dd3f94470a3c67e864b">
commit 1e17b5fdc26696ad0c1c4dd3f94470a3c67e864b
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 01:19:25 2026 -0400

    chore: add execution evidence [20260423T045613-]

diff --git a/.ddx/executions/20260423T045613-389c0053/result.json b/.ddx/executions/20260423T045613-389c0053/result.json
new file mode 100644
index 00000000..d3aafb4f
--- /dev/null
+++ b/.ddx/executions/20260423T045613-389c0053/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-6904a90b",
+  "attempt_id": "20260423T045613-389c0053",
+  "base_rev": "69dd0f557b3619b5e50c4336b2f81d34119751d8",
+  "result_rev": "5c44d00d9ba32301c82cbffd3c776582f76f592c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-d108bd0a",
+  "duration_ms": 1390246,
+  "tokens": 20541352,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T045613-389c0053",
+  "prompt_file": ".ddx/executions/20260423T045613-389c0053/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T045613-389c0053/manifest.json",
+  "result_file": ".ddx/executions/20260423T045613-389c0053/result.json",
+  "usage_file": ".ddx/executions/20260423T045613-389c0053/usage.json",
+  "started_at": "2026-04-23T04:56:14.251450162Z",
+  "finished_at": "2026-04-23T05:19:24.497559896Z"
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
## Review: ddx-6904a90b iter 1

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
