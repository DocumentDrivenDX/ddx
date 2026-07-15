# Agent inventory unified view

This note defines the Fizeau/DDx boundary for the provider and harness views at
`/nodes/.../providers`.

## Decision

Fizeau owns inventory. Every displayed provider, model, and harness row—and
each row's identity, default marker, availability, and automatic-routing
eligibility—must originate in Fizeau's public `ListProviders`, `ListModels`, or
`ListHarnesses` result for the request's project.

DDx must not:

- translate `.ddx/config.yaml` endpoint blocks into provider rows;
- merge configured snapshots with Fizeau listings;
- promote the first provider or a named-but-absent filter into a synthetic row;
- invent a `default` profile or infer profile membership from `IsDefault`;
- call `ResolveRoute` to predict what a future request might use; or
- substitute a cached DDx registry for the current Fizeau listing.

An explicit provider or harness name that is absent from the applicable
Fizeau listing has empty/not-found semantics. Arbitrary future Fizeau harness
names remain valid identities; DDx may format a display label but may not use a
hard-coded lookup to decide membership or eligibility.

## Request-scoped service

REST, MCP, and GraphQL inventory operations construct a Fizeau service with
the request context and project path. A single REST/MCP inventory request
reuses that service for its listing calls and any factual health enrichment.
The narrow inventory interfaces intentionally omit `ResolveRoute`.

## Factual enrichment

DDx may enrich an existing Fizeau row with evidence that describes completed
or current facts without changing the listing:

- public Fizeau `RouteStatus` health/performance observations;
- completed-session usage, latency, success, and cost evidence;
- public quota, account, and usage-window fields returned on Fizeau DTOs; and
- presentation-only labels such as `harnessDisplayName`.

Enrichment must never add, remove, rename, default, or reclassify a row.
Failure to obtain `RouteStatus` leaves health/performance unknown; it does not
remove the corresponding `ListHarnesses` row. Completed-session identity may
support historical trend views when a current listing no longer contains that
identity, but it is not current inventory.

## GraphQL and frontend contract

`providerStatuses` and `harnessStatuses` expose the two Fizeau listings in the
shared `ProviderStatus` presentation shape. Its `autoRoutingEligible` field is
copied from `ProviderInfo.IncludeByDefault` or
`HarnessInfo.AutoRoutingEligible`; it is never inferred by DDx.
`providerModels` first validates the requested provider or harness against its
current Fizeau listing, then returns `ListModels` facts—including each model's
`AutoRoutable` value—for the canonical listed identity.

The API and page deliberately have no `defaultRouteStatus`,
`defaultForProfile`, legacy `Provider` query, or “Current route for default
profile” widget. Actual provider/model/harness facts returned by completed
executions remain available on execution and session surfaces.

## Non-goals

- Changing Fizeau routing or inventory production.
- Reinterpreting billing, pricing, quotas, or completed-session cost evidence.
- Removing legacy configuration fields before the migration bead owns them.
- Removing provider-process supervision or Docker credential provisioning
  before their Fizeau dependencies land.
