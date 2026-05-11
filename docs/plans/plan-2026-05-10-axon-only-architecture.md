---
ddx:
  id: plan-2026-05-10-axon-only-architecture
---
# Axon as Sole Backend / GraphQL Collapse — Exploration

Date: 2026-05-10
Status: Multi-model reviewed (opus); verdict reached — **rejected**. Captured for the record.

## Question being answered

> Do we even need ddx-server's GraphQL? Maybe Axon should have a blob-storage interface and then it's one and done.

I.e., could the architecture collapse to:
- Everything (entities + blobs) lives in Axon.
- Clients (CLI, frontend, notebooks) talk to Axon directly via Axon's existing GraphQL.
- ddx-server's GraphQL surface goes away or becomes a thin auth/projection layer.
- BlobStore as a separate abstraction goes away.

## Verdict (opus review): **Rejected. Keep ddx-server GraphQL + BlobStore as separate abstractions.**

Recommended architecture: **(a) Axon + BlobStore separate, ddx-server GraphQL as the single client-facing API.** Load-bearing reason in the next section.

## Evidence

### ddx-server's GraphQL is doing real, distinct work

`cli/internal/server/graphql/schema.graphql` is **3,584 lines, ~166 type definitions, 60+ root Query fields, 25+ Mutations, 4 Subscriptions** versus Axon's 88-line, 2-query/2-mutation/1-subscription schema. Categorized:

- **~10% Axon-shaped**: bead CRUD that could in principle proxy Axon directly.
- **~90% server-unique**:
  - **Computed bead views** — `beadsReady`, `beadsBlocked`, `beadsDependencyWaiting`, `beadDepTree` resolved via in-process DAG analysis (`resolver_beads.go:229-293`). Axon has no `dependencies-resolved` query.
  - **Live in-process subscriptions** — `workerProgress`, `executionEvidence`, `coordinatorMetrics` are pure server-process state (`resolver_sub_bead.go:13-17`); not in any external store.
  - **Filesystem/git-backed reads** — doc graph, commits, search, artifacts, palette read git + filesystem on the server's host. Not Axon-shaped at all.
  - **Federation hub** — fan-out across spokes (`schema.graphql:2997-3021`, FEAT-026). Axon-per-spoke makes the hub MORE necessary, not less.
  - **Command-shaped mutations** — `workerDispatch`, `runRequeue`, `operatorPromptSubmit` trigger server-side work (spawn goroutines via `WorkerManager`). Axon mutations are CRUD on rows; they don't dispatch jobs.
  - **Auth + CSRF** — tsnet identity, allowlist gating, idempotency dedup live at ddx-server boundary (`server.go:121-233,1162`). Axon has no equivalent identity model.

The frontend coupling reflects this: of the SvelteKit query files (`cli/internal/server/frontend/src/lib/gql/*.ts`), only one (`beads.gql`) maps directly to bead state Axon could serve; the rest (`ProjectQueueSummary`, `EfficacyRows`, `Comparisons`, `Personas`, `PluginsList`, `PaletteSearch`, etc.) are server-computed shapes.

### Axon-as-blob-storage is a wrong fit for the Databricks deployment

Mechanically possible — Postgres `bytea` works for small blobs. But the scoping is wrong:

- Execution evidence is multi-MB per attempt; mirror logs are append-only streams.
- The Databricks-App target is **UC Volumes specifically** for blobs.
- Routing bytes through GraphQL adds a base64 encoding tax, a Postgres row-size headache, and loses the streaming property that StreamStore deferral preserves.

Cost of keeping `BlobStore` separate: two backends, two auth integrations, two consistency stories, no transactional grouping (FEAT-028 calls out the manifest-last + foreign-key discipline as the workaround). Concrete cost is small because `BlobStore` is write-once-per-key with caller-supplied keys.

### What would be lost if ddx-server-graphql died

Walking the server-unique items:
- Computed bead views → would need Axon resolvers OR client-side DAG walk on every load. Painful for a federation hub.
- In-process subscriptions → can't move to Axon; they describe *this server process*. Either Axon becomes a job runner (huge scope expansion) or live worker UI dies.
- Doc graph, commits, search → not Axon-shaped.
- Command-shaped mutations → Axon doesn't spawn goroutines.
- Federation hub → fan-out across spokes is a server concern.
- Auth → would have to be rebuilt inside Axon to match.

## Independent finding: Axon backend itself is not production-ready

A separate opus audit (`cli/internal/bead/axon*` deep review) found that the existing `BackendAxon` is far from production:

- Setting `bead.backend: axon` in `.ddx/config.yaml` today produces **zero wire calls** — `NewStore` calls `NewAxonBackend` with no transport opts (`cli/internal/bead/store.go:127-128`); `WithAxonGraphQLTransport`/`WithAxonGraphQLClient` are only called from tests. The "GraphQL" path is dead code in production.
- Schema mismatch: the GraphQL ops actually invoked (`AxonReadCorpus`, `AxonWriteCorpus`, `saveCorpus`, `AxonBeadRowInput`) are **not defined in `schema.graphql`**. The schema declares per-entity `createEntity`/`updateEntity` only. Three incompatible GraphQL surfaces coexist: the schema, the corpus-blob ops invoked, and the per-entity client never wired up.
- Storage shape is JSONL-shaped (whole-corpus rewrites), not Postgres-shaped (per-row UPSERT). No per-row primary key write op, no version column, no index hints.
- `axonSchemaVersion = 1` is write-only; no v0→v1 migration ladder.
- `AxonBackend` only implements the 4 `RawBackend` methods; all 22 high-level `Backend` methods (Create/Get/Update/Close/Claim/etc.) live on `*Store` and execute full-corpus read → mutate → full-corpus write per call.
- Zero tests run `AxonBackend` against any real GraphQL server (real or simulated). Zero tests run against Postgres.

To call Axon-on-Lakebase "production-default," at minimum these must land:
1. Reconcile `schema.graphql` with the actual ops invoked (or rewrite the backend to use the per-entity ops the generated client already exposes).
2. Real HTTP query transport for `AxonGraphQLTransport` (only websocket subscription transport exists today).
3. Production wiring in `NewStore` that constructs the transport from config.
4. `schema_version` reader with v0→v1 migration ladder.
5. Per-entity read/write ops so a 10k-bead repo doesn't ship the whole corpus per mutation.
6. Postgres DDL + JSONL→Postgres importer (today's `MigrateToAxon` writes JSONL).
7. Integration tests against a real Axon/Postgres instance.

## Implications for the storage abstractions plan

- Keep BlobStore as a separate abstraction (FEAT-028 v1 design unchanged).
- Keep ddx-server's GraphQL as the single client-facing API. Don't try to collapse.
- Axon stays a backend reached through ddx-server, not a peer client surface.
- The "Axon as production-default" claim in FEAT-028 needs a substantial prerequisite list (now captured as a risk row in FEAT-028, expanded above).

## Open questions

- Of the 7 Axon prerequisites above, which are project-blocking for any near-term work, vs. acceptable as "Axon stays JSONL until needed"? If we never deploy as Databricks-App, we don't need them. If we do, all 7 must land.
- Should we file the prerequisites as a tracked epic now, or wait until Databricks deployment is committed?
