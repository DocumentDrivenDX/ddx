# DDx GraphQL + SvelteKit Migration Plan — v3 (locked)

**Status**: committed. 4 stages. No more plan iteration.
**Author**: claude-sonnet-4-6 (under erik's direction)
**Date**: 2026-04-14

## Locked constraints (from user)

- **Frontend**: SvelteKit, Svelte 5 runes, `adapter-static`, **Bun** (not npm/pnpm) for package management and scripts.
- **Backend**: Go + `gqlgen`. New `/graphql` endpoint. Existing REST endpoints **stay frozen**. CLI/MCP continue to call REST unchanged. Post-epic: port CLI to GraphQL, then deprecate REST.
- **URL scheme**: `/nodes/:nodeId/projects/:projectId/*` from day one. DDx already has multiple nodes; the node-ID segment is not cargo-cult.
- **Auth**: existing `isTrusted()` localhost-check model stays as-is. No new middleware. No cookies. No bearer tokens. Not in this epic.
- **Delete React entirely**, rebuild from specs. Pre-release software; no 1:1 port required. Specs are the source of truth.
- **4 stages**. Each stage lands as a checkpoint before the next is filed.
- **No more compromises on scope.** The plan addresses the user's stated goals and does not trade them away for velocity.

## Why this plan exists

Two problems with the current React+REST architecture:

1. **No end-to-end type safety.** `src/types.ts` (182 LOC, 12 interfaces) is hand-mirrored from Go structs. Drift is silent and surfaces as runtime errors.
2. **No story for scale.** REST pagination is ad-hoc or absent; the Beads page fetches ALL beads and filters client-side with minisearch. Real-time is polling except for one SSE endpoint. At 10K+ beads this model breaks.

User's non-negotiable rule: *"I'm going to be nagging you about missed edge cases forever. I don't want that."* The architecture must make edge cases compile errors, not runtime surprises.

## Target state

### Backend

- **`POST /graphql`** powered by `gqlgen` — schema-first, codegen produces typed Go resolvers.
- **`schema.graphql`** is the single source of truth. TypeScript client generated from it via Houdini.
- **Relay cursor connections** for every list type. Every page that displays a list uses cursors.
- **Subscriptions** over `graphql-ws`: `workerProgress`, `beadLifecycle`, `executionEvidence`, `coordinatorMetrics`.
- **REST + MCP untouched.** CLI keeps working. MCP tools keep their shapes. Existing `isTrusted()` enforcement stays.
- **Dataloaders**: **deferred** — current `bead.Store` is in-memory so N+1 isn't the bottleneck. Add when a persistent store lands.

### Frontend

- **SvelteKit** (Svelte 5 runes) + `adapter-static`. Builds to static files. Go serves via `//go:embed`.
- **Bun** for package management and scripts. `bun install`, `bun run dev`, `bun run build`, `bun run test`.
- **Houdini** as the GraphQL client — SvelteKit-sanctioned, codegen from `schema.graphql`, typed `load()` functions and typed subscriptions.
- **`bits-ui`** headless primitives + Tailwind styling. `lucide-svelte` icons. `mode-watcher` for dark mode.
- **Testing**: Playwright for e2e, Vitest for unit, svelte-check for typecheck.
- **URL scheme**: `/nodes/:nodeId/projects/:projectId/*` — implemented fresh, not ported.

### Deployment

Unchanged. Go binary embeds SvelteKit build via `//go:embed`. Single binary. No Node runtime in production. Systemd service unchanged.

## Bead authoring rules (enforced per-bead)

Every bead must have:

1. **A user-visible acceptance step**. Not "tests pass." A human runs the server, navigates/clicks/observes a specific outcome.
2. **Explicit file list**. Every file the bead modifies.
3. **Spec-in-sync**. If the bead's acceptance contradicts any spec, the bead edits the spec in the same commit.
4. **Tests**: Go integration test (real store, real git, zero mocks) for backend beads; Vitest + Playwright e2e for frontend beads.
5. **Small enough for sonnet to finish confidently in ≤30 minutes wall-clock.** Split aggressively.

## Stage 1 — Schema + spec lockdown (13 beads)

**Stage gate**: user reviews schema.graphql and the rewritten ADR-002 before Stage 2 is filed.

| # | Title | Size | Files | User-visible acceptance |
|---|---|---|---|---|
| 1 | Draft `schema.graphql` types + queries | **L** | new `cli/internal/server/graphql/schema.graphql` | Schema covers: `Node`, `Project`, `Bead`/`BeadConnection`/`BeadEdge`, `Document`, `DocGraph`, `Commit`/`CommitConnection`, `Worker`, `AgentSession`, `Persona`, `ExecutionRun`, `CoordinatorMetrics`, `Node.root → Query` entry points for every existing REST list operation (beads, projects, documents, docGraph, commits, workers, sessions, personas, executions). Every field doc-commented. **Accept when**: `go run github.com/99designs/gqlgen generate` (dry-run or scratch invocation) parses the schema without error **AND** a reviewer can point at each `GET /api/*` in `server.go:334-417` and name the GraphQL query that replaces it. |
| 2 | Draft `schema.graphql` mutations + subscriptions | **M** | `cli/internal/server/graphql/schema.graphql` (extend) | Mutations: `beadCreate`, `beadUpdate`, `beadClaim`, `beadUnclaim`, `beadReopen`, `documentWrite`. Subscriptions: `workerProgress(workerID)`, `beadLifecycle(projectID)`, `executionEvidence(runID)`, `coordinatorMetrics(projectRoot)`. **Accept when**: each mutation maps to an existing `POST /api/*` handler by name and each subscription maps to a real event source (worker progress SSE, bead store event bus, execution run log tail, coordinator event hook). |
| 3 | `SD-022-gql-svelte-migration.md` — migration design doc | M | new `docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md` | Doc names the full stack (gqlgen, graphql-ws, SvelteKit, Svelte 5, adapter-static, Bun, Houdini, bits-ui, lucide-svelte, Tailwind, Playwright, Vitest, svelte-check). Lists all four stages with their gate criteria. Cross-references ADR-002, FEAT-008, FEAT-021. **Accept when**: reviewer reads the doc end-to-end and can explain the stage sequence without asking questions. |
| 4 | Rewrite `ADR-002-web-stack.md` | **L** | `docs/helix/02-design/adr/ADR-002-web-stack.md` | Full rewrite. Zero references to React, Vite, Bun-as-React-runtime, TanStack Router, TanStack Query, MSW. Names: gqlgen, SvelteKit, Svelte 5, Bun (as SvelteKit runtime), Houdini, bits-ui, Tailwind, Playwright, Vitest. Status: "Accepted 2026-04-14, supersedes 2025-XX-XX React decision." **Accept when**: `grep -iE "react\|vite\|tanstack\|msw" ADR-002-web-stack.md` returns zero matches. |
| 5 | Supersede `ADR-005-local-first-beads-ui.md` | S | `docs/helix/02-design/adr/ADR-005-local-first-beads-ui.md` | Status → "Superseded by ADR-002 v2 (2026-04-14)." Body replaced with a single paragraph explaining that client-side minisearch was a workaround for missing pagination; Relay cursor connections solve this at the schema level. File stays (historical record). |
| 6 | Rewrite `FEAT-008-web-ui.md` | M | `docs/helix/01-frame/features/FEAT-008-web-ui.md` | Full rewrite against GraphQL operation names and Svelte page contracts. Every page lists its GraphQL query/subscription by name. Test plan references Playwright against SvelteKit build. **Accept when**: `grep -iE "react\|vite\|tanstack\|minisearch" FEAT-008-web-ui.md` returns zero matches. |
| 7 | Rewrite `FEAT-021-dashboard-ui.md` | M | `docs/helix/01-frame/features/FEAT-021-dashboard-ui.md` | Full rewrite. URL scheme `/nodes/:nodeId/projects/:projectId/*` preserved (confirmed correct by user). Every TC-NNN identifier maps to a new Playwright spec filename. Uses Houdini query patterns in examples. Zero React references. |
| 8 | Update `FEAT-002-server.md` + `FEAT-020-server-node-state.md` + `SD-019-multi-project-server-topology.md` | M | three files under `docs/helix/01-frame/features/` and `docs/helix/02-design/solution-designs/` | Swap "React frontend fetches..." to "SvelteKit frontend queries via GraphQL..." Add `/graphql` endpoint alongside REST in interface sections. |
| 9 | Update `concerns.md` | M | `docs/helix/01-frame/concerns.md` | Swap any React/Vite/Bun-as-React/TanStack/MiniSearch references in the Web UI subsection (line 46 area) to SvelteKit/Bun/GraphQL/Houdini. **Auth subsection unchanged** — existing localhost+tsnet model stays. **Accept when**: `grep -iE "react\|tanstack\|minisearch" concerns.md` returns zero matches. |
| 10 | Update developer docs | M | `/Users/erik/Projects/ddx/CLAUDE.md`, `/Users/erik/Projects/ddx/cli/cmd/CLAUDE.md`, `/Users/erik/Projects/ddx/README.md` | Dev loop now describes: `cd cli/internal/server/frontend && bun install && bun run dev`, `bun run houdini:generate`, `bun run test`, `bun run test:e2e`. Zero React/Vite references. `cli/CLAUDE.md` and `cli/README.md` **do not exist** — do not create them. |
| 11 | Update external docs | S | `website/content/docs/server/_index.md`, `library/personas/architect-systems.md`, any `docs/resources/*.md` that reference the stack | Same swap. Persona's "architectural context" section names SvelteKit+GraphQL stack. |
| 12 | Mark historical docs superseded (don't rewrite) | S | `docs/helix/02-design/plan-2026-04-04.md`, `docs/helix/06-iterate/alignment-reviews/AR-*.md`, `TP-002` related docs | Add a front-matter or top-of-file note: "Historical — describes the pre-2026-04-14 React stack. Current stack: see ADR-002 v2." Do NOT rewrite bodies; these are historical records. |
| 13 | **Stage 1 spec sweep** (verification bead, critical) | **M** | none (verification only) | Run: `grep -riE "(react\|vite\|tanstack\|react-router\|minisearch\|MSW)" docs/ library/ website/ CLAUDE.md README.md cli/cmd/CLAUDE.md` and paste the complete output into the bead's completion note. Every match MUST be classified as: (a) historical/explicitly-superseded — OK; (b) intentional (doc still references the old stack for historical context) — OK with justification; (c) stale — must be fixed in this same bead. **Accept when**: completion note shows zero category-(c) matches. |

**Stage 1 gate**: user reviews the schema (beads 1-2) and ADR-002 (bead 4) and approves. Stage 2 beads are filed only after gate passes.

## Stage 2 — GraphQL backend (13 beads)

**Stage gate**: every GraphQL operation in the schema runs successfully against real data via GraphiQL, and every integration test passes.

| # | Title | Size | Files | User-visible acceptance |
|---|---|---|---|---|
| 14 | Add gqlgen + scaffold + `/graphql` endpoint + GraphiQL | M | new `cli/internal/server/graphql/gqlgen.yml`, `resolver.go`, `generated/*`, `go.mod`, `server.go` (register routes) | `go run github.com/99designs/gqlgen generate` runs clean. `go build ./...` passes. **Manual**: start `ddx server`, open `http://127.0.0.1:7743/graphiql`, run `{ __typename }`, see `{"data":{"__typename":"Query"}}`. |
| 15 | Query resolver: `node` + `projects` | S | new `cli/internal/server/graphql/resolver_node.go` | **Manual**: in GraphiQL, run `{ node { id name } projects { id path name } }`, see real values matching `ddx status` and `ddx bead list`'s project. |
| 16 | Query resolver: `beads` + `beadsByProject` with Relay cursor connections | M | new `cli/internal/server/graphql/resolver_beads.go` | **Manual**: `{ beads(first: 10) { edges { node { id title status } cursor } pageInfo { hasNextPage endCursor } } }` returns 10 beads with cursors. Then query with `after: "<endCursor>"` and see the next page. Then query filtered by `projectID:`. |
| 17 | Query resolver: `documents` + `docGraph` | S | new `cli/internal/server/graphql/resolver_documents.go` | **Manual**: query returns real documents from the project's library path; `docGraph` returns nodes+edges matching `GET /api/docs/graph`. |
| 18 | Query resolver: `commits` with Relay cursor connection | S | new `cli/internal/server/graphql/resolver_commits.go` | **Manual**: query commits for the current project, paginate via cursor, see real git log output. |
| 19 | Query resolver: `workers` + `agentSessions` + `executions` | M | new `cli/internal/server/graphql/resolver_agent.go` | **Manual**: queries return real worker/session/execution data. Verify one of each by cross-referencing `ddx server workers list` output. |
| 20 | Query resolver: `personas` + `coordinatorMetrics` | S | new `cli/internal/server/graphql/resolver_meta.go` | **Manual**: `personas` returns the list from `library/personas/`; `coordinatorMetrics` returns the in-memory metrics for the current project. |
| 21 | Mutation resolvers: bead lifecycle (5 mutations) | M | new `cli/internal/server/graphql/resolver_mutation_beads.go` | **Manual**: in GraphiQL, `beadCreate` a bead, verify with `ddx bead show <id>`. Then `beadClaim`, `beadUnclaim`, `beadUpdate`, `beadReopen` — each returns the updated bead and `ddx bead show` confirms. |
| 22 | Mutation resolver: `documentWrite` | S | new `cli/internal/server/graphql/resolver_mutation_docs.go` | **Manual**: in GraphiQL, `documentWrite(path: "...", content: "...")`, then verify the write landed via `ddx doc show <path>` OR by checking the resolved library path (not `.ddx/docs/` — the actual target is `s.libraryPath()` which resolves to the configured library). |
| 23 | Subscription resolver: `workerProgress` | M | new `cli/internal/server/graphql/resolver_sub_worker.go`, `server.go` (graphql-ws transport wiring) | Wraps existing `WorkerManager.SubscribeProgress`. **Manual**: open GraphiQL, start `subscription { workerProgress(workerID: "worker-xxx") { phase eventID logLine } }`, then in another terminal start a worker via `ddx agent execute-loop`, see events stream in GraphiQL. |
| 24 | Subscription resolver: `beadLifecycle` + bead store event bus | M | new `cli/internal/server/graphql/resolver_sub_bead.go`, `cli/internal/bead/events.go` (new event bus if missing) | **Manual**: subscribe in GraphiQL, run `ddx bead update <id> --status ready` in a terminal, see the event arrive in GraphiQL. |
| 25 | Subscription resolvers: `executionEvidence` + `coordinatorMetrics` | M | new `cli/internal/server/graphql/resolver_sub_exec.go`, land-coordinator event hook | **Manual**: subscribe to each; execute a bead and watch the evidence events; run an execute-loop and watch the coordinator metrics events. |
| 26 | GraphQL integration tests: queries + mutations + subscriptions | **L** | new `cli/internal/server/graphql/integration_test.go` | Test file uses `t.TempDir()` + real bead store + real git. Tests cover: every Query resolver returns expected shape, every Mutation mutates real state (verified by calling the same store afterwards), at least one Subscription test opens a real WebSocket client and receives a real event. **Zero mocks** per DDx testing doctrine. **Accept when**: `go test ./cli/internal/server/graphql/... -count=1 -v` passes with all three test categories present. |

**Stage 2 gate**: every operation demonstrably works in GraphiQL against real data. User spot-checks a few operations and approves.

## Stage 3 — SvelteKit scaffold (8 beads)

**Stage gate**: `http://127.0.0.1:7743/` loads a SvelteKit-rendered page; navigating to `/nodes/:nodeId` shows the node ID from a real GraphQL query; Houdini codegen produces typed responses.

| # | Title | Size | Files | User-visible acceptance |
|---|---|---|---|---|
| 27 | **Delete React frontend entirely** | S | `git rm -r cli/internal/server/frontend/`, `cli/internal/server/embed.go` (empty or redirect), `cli/internal/server/server.go` (remove or stub `spaHandler`) | `find cli/internal/server/frontend -type f` returns nothing. `go build ./...` still compiles. `ddx server` starts (serves 404 or empty for `/`). **Accept when**: `git log -p cli/internal/server/frontend/` shows the delete commit and `ls cli/internal/server/frontend` fails. |
| 28 | Scaffold SvelteKit with Bun | **L** | new `cli/internal/server/frontend/package.json`, `svelte.config.js`, `vite.config.ts`, `tsconfig.json`, `bun.lock`, `src/app.html`, `src/routes/+layout.svelte`, `.gitignore` | Run `cd cli/internal/server/frontend && bun create svelte@latest . --template skeleton --types typescript --no-add-ons` (or equivalent Bun-compatible scaffold). Install: adapter-static, @sveltejs/adapter-static, Tailwind, Playwright, Vitest, svelte-check, prettier-plugin-svelte, eslint-plugin-svelte. **Manual**: `bun install` succeeds; `bun run dev` starts dev server on :5173; `bun run build` produces `build/index.html` + static assets; `bun run test` (vitest) runs; `bun run test:e2e` (Playwright) runs. Document in `cli/internal/server/frontend/README.md` (new file) the `bun` commands. |
| 29 | **Verify Svelte 5 compatibility of planned libraries** | S | `cli/internal/server/frontend/package.json`, new `cli/internal/server/frontend/STACK.md` | Check that `bits-ui`, `lucide-svelte`, `mode-watcher`, Houdini all have Svelte 5 releases. If any are stuck on Svelte 4, document the fallback (either downgrade the whole scaffold to Svelte 4 or use a different library for that one primitive). **Accept when**: `STACK.md` lists each library, its Svelte 5 status (verified version), and the fallback decision for any incompatible one. |
| 30 | Install Houdini + codegen from schema | M | `cli/internal/server/frontend/package.json`, new `cli/internal/server/frontend/houdini.config.js`, `src/client.ts` | `bun add houdini` + configure to read schema from `../graphql/schema.graphql`. Run `bun run houdini:generate`. **Manual**: write a dummy `query { __typename }` in a `.gql` file, regenerate, observe typed TypeScript types in `$houdini/`. Open one in the editor and confirm it has the expected shape. |
| 31 | Install UI primitives: bits-ui + lucide-svelte + mode-watcher + Tailwind | M | `cli/internal/server/frontend/package.json`, `tailwind.config.js`, `postcss.config.js`, `src/app.css` | `bun add` each. Tailwind configured. **Manual**: create a tiny test page with a button from bits-ui and a Lucide icon, navigate to it in `bun run dev`, see both render; click the dark-mode toggle, see theme switch. |
| 32 | Base layout + theme provider + nav shell | M | new `src/routes/+layout.svelte`, `src/lib/theme.ts`, `src/lib/components/NavShell.svelte`, `src/lib/components/ProjectPicker.svelte` (stub) | Layout has top nav with Node name placeholder, sidebar with placeholder links (Beads, Documents, Graph, Workers, Sessions, Personas, Commits), dark-mode toggle, stub project picker. **Manual**: `/` loads the shell in dev; dark-mode toggle works; nav links are visible (linking to TBD routes is fine for now). |
| 33 | Update Go embed to serve SvelteKit build output | S | `cli/internal/server/embed.go`, `cli/internal/server/server.go` | `//go:embed all:frontend/build` replaces `//go:embed all:frontend/dist`. Static file handler serves `frontend/build/*`. For SvelteKit `adapter-static` with `fallback: 'index.html'`, deep links resolve to the SPA shell via SvelteKit's own client-side routing. **Manual**: `make build` embeds the SvelteKit `build/` output; `./cli/build/ddx server` running locally serves the layout at `http://127.0.0.1:7743/`. Loading `http://127.0.0.1:7743/nodes/abc/projects/def/beads` directly returns the SvelteKit shell (which renders the Svelte router). |
| 34 | NodeContext + ProjectContext stores + root redirect | M | new `src/lib/stores/node.ts`, `src/lib/stores/project.ts`, `src/routes/+layout.ts`, `src/routes/+page.svelte`, `src/routes/nodes/[nodeId]/+page.svelte` (stub) | `+layout.ts` loads `{ node { id name } }` via Houdini on mount. `src/routes/+page.svelte` checks the store and redirects to `/nodes/:nodeId`. `src/routes/nodes/[nodeId]/+page.svelte` renders "Node: `<name>` (`<id>`)" from the store. **Manual**: load `/` in browser → URL becomes `/nodes/node-xxx`, page shows real node name from GraphQL. Check browser devtools Network panel to confirm the GraphQL POST to `/graphql` returned the expected shape. |

**Stage 3 gate**: browser loads `/`, redirects to `/nodes/:nodeId`, and shows real data pulled via Houdini from the Stage 2 GraphQL endpoint. User runs the binary and confirms.

## Stage 4 — Pages + tests + release (17 beads)

**Stage gate**: every page works end-to-end, full test suite passes, `v0.6.0-alpha20` cut.

| # | Title | Size | Files | User-visible acceptance |
|---|---|---|---|---|
| 35 | Project picker wired to GraphQL `projects` query | S | `src/lib/components/ProjectPicker.svelte` | Picker populates from live projects query. Selecting a project navigates to `/nodes/:nodeId/projects/:projectId`. **Manual**: picker shows all registered projects; selecting one updates the URL and highlights. |
| 36 | Beads list view + cursor pagination (project-scoped) | M | new `src/routes/nodes/[nodeId]/projects/[projectId]/beads/+page.*`, `beads.gql` | Table of beads (id, title, status, priority). "Load more" button appends the next cursor page. **Manual**: create 15 test beads via `ddx bead create`, load page, see 10, click "Load more", see 5 more, no duplicates. |
| 37 | Beads filter chips + URL state | S | `src/routes/nodes/[nodeId]/projects/[projectId]/beads/+page.svelte` | Status/label filter chips update URL `?status=open&label=helix`, reload preserves. **Manual**: click a chip, URL updates, table filters; reload preserves. |
| 38 | Beads search input (server-side via GraphQL) | M | `beads.gql` (add `search:` arg), resolver update in Stage 2 covers it | Debounced 200ms input filters results via GraphQL `search:` argument. **Manual**: type a query, results narrow; URL reflects `?q=foo`. |
| 39 | Beads detail panel | M | new `src/lib/components/BeadDetail.svelte` | Row-click opens side panel with full bead metadata + dependencies + history. URL becomes `.../beads/:beadId`. Closing returns to list. **Manual**: click a row, panel opens, shows real bead data; close, returns to list. |
| 40 | Beads mutations: create + claim/unclaim + update | M | new `src/lib/components/BeadForm.svelte`, mutation .gql files | "New bead" button opens form; submit creates bead and it appears in the list. Claim/unclaim buttons in detail panel update status visibly. **Manual**: create a bead, verify with `ddx bead show`; claim it via UI, see status change; verify in terminal. |
| 41 | Beads lifecycle subscription (live updates) | S | `beadLifecycle.gql`, `+page.svelte` | Subscription active while page open; when another process mutates a bead, the row updates live. **Manual**: open page, run `ddx bead claim <id>` in terminal, see row status flip without refresh. |
| 42 | Beads cross-project combined view at `/nodes/:nodeId/beads` | M | new `src/routes/nodes/[nodeId]/beads/+page.*` | Same list pattern, merges beads across all projects. Project filter chips. **Manual**: load `/nodes/:nodeId/beads`, see beads from every registered project with project-badge column. |
| 43 | Documents list + markdown render | M | new `src/routes/nodes/[nodeId]/projects/[projectId]/documents/+page.*`, `+page/[...path]/+page.svelte` | Table of docs; clicking opens rendered markdown view. Use `@portabletext/svelte` or `svelte-markdown` for GFM. **Manual**: click any doc, see rendered markdown matching the file content. |
| 44 | Documents edit-in-place + `documentWrite` mutation | S | `[path]/+page.svelte`, `documentWrite.gql` | "Edit" button swaps render to textarea; save calls mutation; display updates. **Manual**: edit a doc, save, refresh, see change persisted. |
| 45 | Graph page: D3 Svelte island | **L** | new `src/routes/nodes/[nodeId]/projects/[projectId]/graph/+page.*`, `src/lib/components/D3Graph.svelte` | Reimplement the D3 force-simulation in a Svelte component (not a port of React code — a rewrite). Drag, zoom, hover tooltips all work. **Manual**: load the graph, drag nodes, zoom in/out, hover for tooltips; all match a human's expectation of "a D3 force graph." |
| 46 | Workers page: list + live log via `workerProgress` subscription | **L** | new `src/routes/nodes/[nodeId]/workers/+page.*`, `src/routes/nodes/[nodeId]/workers/[workerId]/+page.*`, `worker.gql`, `workerProgress.gql` | List populates from `workers` query. Clicking a worker opens detail with log tail via subscription. **Manual**: start a worker via `ddx agent execute-loop`, open its detail page, see log lines stream in real time as the agent runs. |
| 47 | Agent sessions + Personas + Commits pages | M | three routes under `src/routes/nodes/[nodeId]/projects/[projectId]/` | Sessions: token summary + collapsible detail. Personas: split panel. Commits: cursor-paginated table with bead cross-links. **Manual**: visit each page, see real data. |
| 48 | Playwright suite: Node/Project navigation spec | M | new `cli/internal/server/frontend/e2e/navigation.spec.ts` | Tests: `/` redirects to `/nodes/:nodeId`, nav chrome renders, project picker navigates. ≥5 test cases with TC-NNN identifiers. **Accept when**: `bun run test:e2e -- navigation` passes locally AND on a fresh `make build` + server start. |
| 49 | Playwright suite: Beads + Documents + Graph + Workers specs | **L** | new `beads.spec.ts`, `documents.spec.ts`, `graph.spec.ts`, `workers.spec.ts` | Each spec covers: page loads, real data displays, primary interactions work (filter, search, detail open, mutation, subscription). ≥5 TC-NNN cases per spec. **Accept when**: `bun run test:e2e` passes all four specs. |
| 50 | Rewrite Go SPA handler tests + CI rewrite + lefthook cleanup | M | `cli/internal/server/server_test.go`, `.github/workflows/ci.yml`, `lefthook.yml` | Replace `TestSPAServesIndexHTML`/`TestSPAFallbackForClientRoute` with tests that verify the new `//go:embed` path serves SvelteKit build output (one test loads `/`, one loads a deep link like `/nodes/x/projects/y/beads`, both return HTML containing the SvelteKit app shell). CI: delete the Bun-for-React job, add `bun install` + `bun run build` + `bun run test` + `bun run test:e2e` against the new frontend. lefthook: delete `debug-js` hook (now orphaned). **Accept when**: `go test ./cli/internal/server/...` passes, `act` or a dry-run of the GH workflow succeeds, `lefthook run pre-commit` is clean. |
| 51 | **Final spec consistency audit** (verification bead, critical) | **M** | none | Run the full sweep: `grep -riE "(react\|vite\|tanstack\|react-router\|minisearch\|MSW\|'frontend/dist'|npm install|pnpm)" docs/ library/ website/ CLAUDE.md README.md cli/ .github/ lefthook.yml 2>/dev/null`. Every match MUST be classified in the completion note as (a) historical/superseded — OK; (b) intentional — OK with justification; (c) stale — must be fixed in this same bead before closing. **Accept when**: zero category-(c) matches. The completion note is the proof. |
| 52 | Release v0.6.0-alpha20 | S | git tag, release notes | Full manual regression: start server, navigate every page, verify each subscription fires, claim a bead, create a bead, edit a document, watch a worker log stream. Log manual steps + outcomes in the bead's completion note. Build binary, install to `/home/erik/bin/ddx`, restart `ddx-server` systemd unit, confirm `ddx status` shows new version, confirm the new UI loads. |

**Stage 4 gate**: user manually walks every page, confirms everything works, green-lights the release.

## Totals

- **Stage 1**: 13 beads (3 L, 6 M, 4 S) → ~4 hours sonnet-time
- **Stage 2**: 13 beads (1 L, 8 M, 4 S) → ~4 hours
- **Stage 3**: 8 beads (1 L, 5 M, 2 S) → ~3 hours
- **Stage 4**: 17 beads (3 L, 8 M, 6 S) → ~6 hours
- **Total**: 51 beads, ~17 hours sonnet-time. Wall-clock at execute-loop pace: 2-3 days.

## Risks

1. **Schema design is critical path.** Bad schema = rework everywhere. Beads 1-2 get a human review gate before Stage 2 files.
2. **Svelte 5 ecosystem churn.** Bead 29 verifies compatibility explicitly. Fallback is documented (downgrade to Svelte 4 for the first pass, upgrade later) — but the plan has NO other fallback for a library-level incompatibility.
3. **Subscription lifecycle bugs.** WebSocket drops, cleanup, reconnect. Bead 26's integration tests must drive a real disconnect.
4. **D3 graph page (bead 45) is the most technically sensitive.** Sized L, may need further split if the force-sim + drag + zoom code is bigger than expected.
5. **Bead 29 (Svelte 5 compat) is a gate for Stage 3.** If it finds an incompat, the whole scaffold strategy may need revisiting. Schedule early.
6. **Pre-existing `installation acceptance tests`** (known flaky) may fail during Stage 4 CI rewrite. Accept as pre-existing; do not attempt to fix inside this epic.
7. **Stage 3 bead 27 (delete React)** is destructive but safe: user confirmed the React app has only been loaded once; no production state lost.

## What this plan does NOT include

- Rewriting the CLI to use GraphQL. REST stays for the CLI.
- Rewriting MCP tools. MCP stays at `POST /mcp` with current shapes.
- Auth middleware. Existing `isTrusted()` localhost model stays as-is.
- Dataloaders. Deferred until persistent store exists.
- Observability / tracing / request IDs. Separate concern.
- Deprecating REST. Post-epic, separate effort.
- Data migration. None needed.

## Success criteria

- Every page from the original React app has an equivalent (or better) Svelte page built from FEAT-008 + FEAT-021 specs.
- Every GraphQL operation has a real-store integration test passing (zero mocks).
- Playwright e2e suite passes against the new binary.
- `cli/internal/server/frontend/` contains only SvelteKit source. No React files anywhere in the repo (outside of historical git blobs).
- `grep -riE "react\|tanstack" docs/ library/ website/ CLAUDE.md README.md cli/cmd/CLAUDE.md` returns only historical/superseded citations.
- `v0.6.0-alpha20` released, server restarted, a new bead lands successfully via the new UI stack.
- User reports: "I don't feel the need to nag about missed edge cases anymore."
