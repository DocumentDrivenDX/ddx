# Agent endpoints unified view

Design note for `ddx-23978824`: unify the providers page to include both endpoint providers and subprocess harnesses, surface token utilization + quota trend.

## Decision summary

| Topic | Decision |
| --- | --- |
| Page name | "Agent endpoints" (replaces "Providers" as page title; URL stays `/nodes/.../providers` for link stability) |
| Row model | One table, `kind` column distinguishes `endpoint` vs `harness` |
| Liveness | Paint cached status first; async probes patch rows as they complete |
| Usage source | `SessionIndexEntry` aggregated server-side into hourly buckets |
| Quota source | Harness rate-limit headers (claude, codex) parsed from captured header blobs; null otherwise |
| Trend view | Dedicated route `/providers/[name]` with 7d/30d series |
| Projection | Linear slope over last-24h window; callout only when ceiling known + positive slope |

## Row model

Common fields — `ProviderStatus` (existing type) gains:

- `kind: ENDPOINT | HARNESS` — discriminant
- `reachable: Boolean!` — last-known ability to accept work; false when the row is only a configured snapshot or explicitly unavailable
- `detail: String!` — human-readable status detail (error text, binary path, or "not checked yet")
- `lastCheckedAt: String` — RFC3339 timestamp of the in-process probe result
- `usage: ProviderUsage` (nullable) — token/request counts over rolling windows
- `quota: ProviderQuota` (nullable) — ceiling/remaining/resetAt
- `defaultForProfile: [String!]!` — profile names where this endpoint is the default candidate

New query `harnessStatuses: [ProviderStatus!]!` returns subprocess harnesses in the same shape so the frontend can concatenate and render one table.

## ProviderUsage

```graphql
type ProviderUsage {
  tokensUsedLastHour: Int
  tokensUsedLast24h: Int
  requestsLastHour: Int
  requestsLast24h: Int
}
```

Computed by aggregating `SessionIndexEntry` rows:
- Filter by `Provider == row.name` (endpoint) or `Harness == row.name` (harness)
- Filter by `StartedAt` within last hour / last 24h
- Sum `Tokens` (fall back to `InputTokens + OutputTokens`), count entries

## ProviderQuota

```graphql
type ProviderQuota {
  ceilingTokens: Int
  ceilingWindowSeconds: Int
  remaining: Int
  resetAt: String
}
```

- **Endpoint providers** — null until the provider service captures headers (out of scope for this bead).
- **Subprocess harnesses (claude, codex)** — populated from captured rate-limit headers stored in `SessionIndexEntry.Detail` metadata or a harness-specific log tail. Parser lives in `cli/internal/agent/ratelimit_headers.go` with fixture-backed tests.

Harness-specific header mapping:

- **Claude Code** — `anthropic-ratelimit-requests-limit`, `-remaining`, `-reset`, and `anthropic-ratelimit-tokens-limit`, `-remaining`, `-reset`. Tokens family maps to `ceilingTokens` / `remaining` / `resetAt`; window is 1 minute (fixed by the Anthropic API contract).
- **Codex** — `x-ratelimit-limit-tokens`, `x-ratelimit-remaining-tokens`, `x-ratelimit-reset-tokens`. Window is 1 minute for the headline quota.
- **Other harnesses (gemini, agent)** — no standard headers; leave null.

## Async liveness

Initial page load strategy:
1. Frontend fires `providerStatuses` + `harnessStatuses` concurrently in one GraphQL request. The endpoint resolver returns the in-process cache when present; otherwise it returns a configured endpoint snapshot without probing `/models`. Harness inventory is read directly from the harness catalog.
2. Returning endpoint rows schedules a background provider probe. Later `providerStatuses` reads patch rows from the cache once the probe finishes.
3. If only legacy/global provider config is present and no DDx endpoint snapshots exist, the synchronous fallback is bounded so the page can still first-paint harness rows instead of waiting on a full probe wall.

The status cache is deliberately process-local. It is a rendering cache, not the source of truth; DDx can replace it with a persisted probe snapshot store later without changing the GraphQL row model.

## Trend view

`providerTrend(name: String!, windowDays: Int! = 7): ProviderTrend` returns:

```graphql
type ProviderTrend {
  name: String!
  kind: ProviderKind!
  windowDays: Int!
  series: [ProviderTrendPoint!]!
  ceilingTokens: Int
  projectedRunOutHours: Float
}

type ProviderTrendPoint {
  bucketStart: String!      # RFC3339 truncated to hour
  tokens: Int!
  requests: Int!
}
```

Server bucketing: `StartedAt` truncated to the hour, summed per bucket. Maximum of `windowDays * 24` points; for a 30-day window that's 720 points — under the 200-point client budget only after downsampling to 1-point-per-4-hours when `windowDays > 8`. The resolver returns pre-downsampled buckets: hourly for ≤7d, 4-hourly beyond.

Projection: last-24h slope in tokens/hour derived by least-squares fit over the 24 hourly buckets. `projectedRunOutHours = remainingTokens / slope` when slope > 0 and ceiling is known. Null otherwise — the UI treats null as "no projection."

## Perf posture

Per `ddx-9ce6842a`: unified list target p95 ≤ 200ms HTTP, trend detail p95 ≤ 400ms HTTP over a ≥1,000-row seeded fixture. If detail aggregation cannot hit p95 on a 10k-row stretch fixture, raise the DB-substrate decision with the user per the standing directive — do not silently swap backends.

## Billing mode

`ddx-6904a90b` splits session costs into paid/subscription/local via `billingMode`. The unified view defers that semantic to a follow-up; this bead's `usage.tokensUsed*` fields are raw totals regardless of billing mode, and the existing `Harness.IsSubscription` / `IsLocal` flags on the REST `ProviderSummary` retain their current meaning. When billing-mode plumbing lands across the sessions index, the trend view can layer a cost axis; the schema shape is extensible.

## Out of scope

- Server-side push of probe completion (delivered via polling / manual refresh).
- Quota ceilings for endpoint providers beyond the ones already in `HarnessInfo.Quota`.
- Per-model quota breakdown in the unified list row (handled by existing REST `/api/providers/{harness}` detail response).
