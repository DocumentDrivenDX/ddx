---
ddx:
  id: FEAT-008
  depends_on:
    - helix.prd
    - FEAT-002
    - FEAT-014
---
# Feature: DDx Server Web UI

**ID:** FEAT-008
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

The DDx server (`ddx-server`) serves a web UI for browsing artifacts, beads,
the artifact dependency graph, and DDx run activity. Artifacts include markdown
documents plus sidecar-identified media such as diagrams, wireframes, images,
and PDFs. The UI is a SvelteKit application built with Svelte 5, embedded into
the Go binary via `embed.FS`, and served alongside the MCP and HTTP API
endpoints from a single process.

`ddx-server` runs as a per-user host daemon (one instance per machine, see
FEAT-002 and FEAT-020), so the same UI enumerates every project that the
host+user registry knows about and binds its navigation to one selected
project context at a time. The node-aware URL structure and combined
cross-project dashboards are specified in FEAT-021; FEAT-008 owns the
underlying views that those routes render.

The UI consumes GraphQL operations defined in `cli/internal/server/graphql/schema.graphql`.
Every page lists its required query or subscription by name for traceability.

## Problem Statement

**Current situation:** DDx artifacts, beads, and the artifact graph are only accessible via CLI commands or MCP tool calls. There's no visual way to browse the artifact library, see the dependency graph, check bead status, regenerate generated artifacts, or review layer-aware run evidence.

**Pain points:**
- CLI output for `ddx doc graph` is text-only — dependency graphs need visual rendering
- Bead status and dependency trees are hard to grok in terminal output
- DDx run activity has no browsable interface that preserves the `work` -> `try` -> `run` layers
- Non-markdown artifacts have no type-aware browser preview, even when the graph knows their identity
- Generated artifacts have no UI affordance for checking provenance or requesting regeneration
- New team members can't quickly understand what artifacts exist or how they relate

**Desired outcome:** A web dashboard served from the same `ddx-server` binary that already handles MCP and HTTP API. Developers open a browser, see their artifact library, dependency graph, bead board, generated-artifact provenance, and DDx run activity — all from one URL.

## Architecture

### Single Binary, Multiple Surfaces

```
ddx-server binary
├── /            → Web UI (embedded SvelteKit)
├── /api/        → HTTP REST API (JSON) — frozen
└── /graphql     → GraphQL endpoint (gqlgen)
```

The SvelteKit frontend queries the `/graphql` endpoint for all data. REST endpoints
remain unchanged and are consumed by the CLI and MCP tools only.

### Project Context

The UI is rendered under the node-scoped URL structure defined in FEAT-021:
`/nodes/:nodeId/projects/:projectId/...` for project-scoped views and
`/nodes/:nodeId/...` for combined node-wide dashboards. The root `/`
redirects to `/nodes/:nodeId` using the ID returned by `GET /api/node`
(FEAT-020). When more than one project is registered the navigation bar
exposes a project picker populated from `GET /api/projects`; selecting a
project swaps the `:projectId` segment in place. All GraphQL queries issued by the
UI are bound to the selected project context.

### Project Navigation Tabs

Each `/nodes/:nodeId/projects/:projectId/...` page exposes the following
project-scoped tabs in the global navigation, in this order:

| Tab | Route segment | Purpose |
|---|---|---|
| Overview | (none) | Project home, recent activity, operator-prompt input (ADR-021) |
| Beads | `beads` | Bead list / kanban / ready / detail (US-082*) |
| Artifacts | `artifacts` | Artifact browser (US-080, US-081b, US-083) |
| Graph | `graph` | Document dependency graph (US-081, US-081c) |
| Runs | `runs` | Layer-aware run history with `work` / `try` / `run` chips, row expansion, and re-queue (US-086b, US-086c) |
| Workers | `workers` | Live + recent worker processes for this project (US-085b, US-086, US-086a) |
| Personas | `personas` | Persona browser + role bindings (US-097) |
| Plugins | `plugins` | Plugin registry + install/uninstall (US-098) |
| Commits | `commits` | Project git commit log (US-093, FEAT-021) |
| Efficacy | `efficacy` | Model efficacy + comparisons (US-096) |

**Sessions and Executions are retired as top-level tabs** (Story 8). The
unified Runs tab carries layer chips and inline row expansion that
surface what those tabs used to show:

- `layer=run` rows expand to AgentSession transcript / billing / cached
  tokens / stderr (the old Sessions detail).
- `layer=try` rows expand to the `.ddx/executions/<attempt-id>/` bundle
  metadata, check results, and verdict (the old Executions detail).
- `layer=work` rows expand to queue inputs and child run links.

**URL deprecation policy.** During the deprecation window, the legacy
URLs respond with **302 redirects (NOT 301)** to the matching filtered
Runs URL with all query params preserved, plus a `Sunset` response
header that names the cutoff date for the redirects:

| Legacy URL | Redirect target |
|---|---|
| `/sessions` | `/runs?layer=run` |
| `/sessions#<id>` | `/runs?layer=run&session=<id>` |
| `/executions[?bead=&verdict=&harness=&q=&after=]` | `/runs?layer=try` with the original query string preserved verbatim |
| `/executions/[id]` | `/runs/[runId]?bundle=true` |

302 (not 301) is intentional: bookmarks and external link aggregators
should not cache the redirect permanently while we still own the legacy
routes. The redirect response carries `Sunset: <RFC 9745 date>` and the
project nav drops the Sessions/Executions entries once the redirects and
the internal-link sweep land (`shellRoutes.ts`, `BeadDetail.svelte`,
commits page, agent page, Workers detail).

### Build Pipeline

```
cli/internal/server/frontend/   → SvelteKit + TypeScript
  bun run build                 → cli/internal/server/frontend/build/
cli/internal/server/            → Go with embed.FS
  go build                      → ddx-server (single binary with embedded UI)
```

During development, SvelteKit's dev server proxies `/graphql` to the running Go server.

### Technology Choices

| Layer | Choice | Why |
|-------|--------|-----|
| Frontend framework | **Svelte 5** | Compile-time optimizations, no Virtual DOM |
| Build tool | **SvelteKit** | Router, layout, API routes, hooks, adapter-static for embedding |
| Styling | **Tailwind CSS** | Consistent with website (Hextra uses Tailwind) |
| Data fetching | **graphql-request + graphql-ws** | Lightweight GraphQL client with typed queries and WebSocket subscriptions |
| Graph visualization | **D3.js** or **Cytoscape.js** | For document dependency graph rendering |
| Embedding | **Go embed.FS** | Compile frontend into the binary — no external files needed |
| Go HTTP | **Chi** or **net/http** | Standard library compatible routing |
| Go GraphQL | **gqlgen** | Schema-first, code-generated resolvers |

### Alternatives Considered

**Templ + htmx** — simpler (no JS toolchain), but the dependency graph visualization and interactive bead board need more interactivity than htmx provides well. Templ + htmx is great for CRUD; we need a richer graph/dashboard experience.

**Local-first with CRDT sync** — interesting for offline use but overkill for v1. The UI is primarily read-heavy (browsing docs, viewing graphs). Can be added later with PowerSync or ElectricSQL if needed.

## Requirements

### Functional

1. **Artifact browser, viewer, editor, and regeneration**
   - List artifacts by category (prompts, personas, patterns, templates, ADRs,
     SDs, diagrams, wireframes, generated images, PDFs)
   - Full-text search across textual artifact content, sidecar metadata, and
     markdown frontmatter
   - View markdown artifacts rendered as markdown with syntax highlighting
   - Render artifacts by `media_type`:
     - `text/markdown`: markdown renderer with syntax-highlighted code blocks
     - `image/svg+xml` with Mermaid provenance: sanitized inline SVG preview
     - `application/vnd.excalidraw+json`: Excalidraw read-only embed
     - `image/*`: inline image preview with metadata fallback
     - `application/pdf`: embedded PDF preview with open-file fallback
     - unknown or binary media: metadata panel plus open/download affordance
   - Show `ddx:` frontmatter or sidecar `.ddx.yaml` metadata (id,
     `media_type`, dependencies, `generated_by`, and staleness status)
   - Clickable dependency links — navigate upstream/downstream docs inline
   - **Editor**: in-browser markdown editing with save-to-disk (calls API to write file)
   - Sidebar showing document dependency tree for current doc
   - Staleness indicator per document (fresh/stale/missing-deps)
   - Generated-artifact provenance indicator: show producing run id, prompt or
     generator summary, and source-hash status when `generated_by` is present
   - Generated-artifact staleness is artifact-local: a `generated_by.source_hash`
     mismatch marks only the generated artifact stale and does not cascade like
     `depends_on`
   - Regenerate action for generated artifacts calls the `artifactRegenerate`
     mutation, returns the new run id, and links directly to the produced run
     detail and updated artifact

2. **Document dependency graph**
   - Interactive visual graph (D3/Cytoscape) showing all document relationships
   - Color-code nodes by staleness (green=fresh, orange=stale, red=missing-deps)
   - Click node to navigate to document detail
   - Filter by document type, staleness status
   - Zoom/pan for large graphs
   - Layout options (hierarchical top-down, force-directed)

3. **Bead views**

   The beads UI runs entirely in the browser. On load, the client fetches the
   full bead set from the `beads` query (GraphQL). All search, filter, sort, and graph
   traversal happens client-side using an in-browser data layer.

   **Master-detail list view:**
   - Searchable list: full-text across title, description, acceptance, labels
   - Sortable columns: ID, title, status, priority, labels, owner, updated_at
   - Filterable by status, priority, label, owner
   - Click a bead to open detail panel (split pane or slide-over)
   - Detail shows: all fields, dependency tree, execution beads, agent sessions
   - Graph traversal: click a dependency → navigate to that bead's detail
   - Drill-down into run records linked to the bead (shows layer, pass/fail,
     logs, duration)

   **Kanban board view:**
   - Columns represent status: `open`, `in_progress`, `closed`
   - Cards within columns ordered by priority (P0 top, P4 bottom)
   - Dependency grouping: beads that block each other cluster visually
     (similar to Trello, Fuzz by 37signals, GitHub Projects)
   - Drag-and-drop to change status (calls `beadUpdate` mutation)
   - Swimlanes option: group rows by label (e.g., `area:cli`, `area:web`)
   - Color coding: priority → card border, labels → chips, blocked → dimmed
   - Collapsed card shows: title, priority badge, label chips, dep count
   - Expanded card shows: description, acceptance, execution summary

   **Ready queue view:**
   - Filtered view showing only beads with all dependencies satisfied
   - Sorted by priority, then by creation date
   - One-click claim (calls `beadClaim` mutation)
   - Single-ticket ready work is shown ahead of epics; open epics appear in a
     separate epic lane rather than the ordinary execute-loop queue

   **Bead detail actions:**

   The detail panel is not just a viewer — it is the primary interaction
   surface for evaluating and driving work items.

   *Navigation:*
   - `spec-id` links to the governing artifact (rendered inline or navigates
     to the document viewer with that artifact's content)
   - Parent bead link navigates to the parent's detail
   - Dependency links navigate to each dep's detail
   - Run records link to the layer-aware run detail view (logs, results)
   - Agent session links navigate to the agent log viewer for that session

   *Review and re-evaluation:*
   - "Review" action: side-by-side view of the bead's acceptance criteria
     vs the latest execution results and implementation diff. The user or
     agent evaluates whether the work actually meets the spec.
   - "Re-review" action on closed beads: re-evaluates against current
     artifact state — did the governing spec change since the bead was
     closed? If so, flag as potentially stale.
   - Artifact drift indicator: if the `spec-id` artifact's content hash
     changed since the bead was last updated, show a warning badge.

   *Status and lifecycle:*
   - Re-open a closed bead (status → open) with a reason field
   - Re-run bead work: dispatch `ddx try <bead>` through the server worker API,
     then open the resulting `try` record under the layer-aware run history
   - Run linked checks: when the bead references an execution definition,
     dispatch that definition as project checks and show the resulting record
     separately from bead re-run history
   - Claim / unclaim for agent coordination
   - Inline status transitions (open → in_progress → closed)

   *Mutation:*
   - Edit all bead fields inline (title, description, acceptance, labels,
     priority, spec-id)
   - Create new beads with guided form
   - Add/remove dependencies by selecting other beads
   - All mutations call the HTTP API which writes to the JSONL store

4. **Agent log monitor**
   - Live-updating list of agent invocations (newest first)
   - Columns: timestamp, harness, model, tokens, duration, exit code
   - Click to expand: DDx metadata, native session/trace references, and any
     DDx-owned detail available for that invocation
   - Filter by harness, time range, exit code
   - Token usage summary (provider-native or DDx-observed, depending on source)
   - Auto-refresh on configurable interval (or subscription in v2)

5. **Layer-aware run history and detail**
   - List DDx run records from the unified substrate with explicit layer chips:
     `work`, `try`, and `run`
   - Filter by layer, status, harness, provider, model, bead id, artifact id,
     and time range without introducing a run-type catalog beyond the three
     layers
   - Free-text search across bead id, bead title, and output excerpt; multi-
     column sort on Started / Duration / Cost. Search/sort/filter primitives
     are the shared Story 6 chip + sort-header + search-input components
     (PRIMITIVE reuse — Runs has its own page model for live updates,
     parent/child hierarchy, layer expansion, and re-queue affordances).
     Story 7's virtualization wrapper is reused with a Runs-specific live
     adapter when row counts exceed 1k.
   - Inline row expansion is **layer-aware** (replaces the retired Sessions
     and Executions top-level tabs):
     - `layer=work` row expands to queue inputs, selected beads, stop
       condition, retry/defer decisions, and child try-attempt links in
       chronological order.
     - `layer=try` row expands to bead id, base/result revisions, isolated
       worktree path, merge/preserve outcome, checks, verdict, and child
       layer-1 run links — sourced losslessly from the
       `.ddx/executions/<attempt-id>/` bundle attached to the layer-2 record.
     - `layer=run` row expands to prompt/config summary, power bounds,
       selected harness/provider/model, token + cached-token + cost +
       duration signals, output, stderr, and evidence links — sourced
       losslessly from the joined `AgentSession` row (FEAT-010 §"Layer-to-
       substrate mapping for the Runs UI").
   - Drill-down links preserve hierarchy in both directions:
     `work` -> `try` -> `run`, with breadcrumbs back to parent records
   - Artifact-producing runs show `produces_artifact`; artifact pages link back
     to the producing run and expose regeneration history
   - **Re-queue affordances** (Story 8):
     - `layer=try` rows show a `Re-queue` button when the originating bead is
       open AND no try is currently active for that bead. The button opens a
       confirmation dialog prefilled with the original config; the client
       generates the `idempotencyKey` and sends it with the
       `runRequeue` mutation (FEAT-010).
     - `layer=run` rows show `Re-queue` only when prompt / harness / model are
       replayable; re-queueing a successful run requires double-confirmation.
     - `layer=work` rows show **`Start worker from this drain`** (different
       label from "Re-queue"; same dispatch path) prefilled with the
       original queue inputs, stop condition, selected beads, and harness /
       profile from the original work run.
     - Re-queue against a closed bead is disabled and surfaces a
       "Reopen bead first" link.
     - Every successful re-queue emits a `run_requeue` audit event on the
       originating bead (schema: FEAT-010 §"Re-queue audit events").
   - Layer-aware run views consume read-only GraphQL/HTTP state except for the
     narrow `artifactRegenerate` and `runRequeue` mutations.

6. **Status dashboard**
   - Summary cards: document count by type, bead counts by status, stale document count, recent agent activity
   - Quick links to ready beads, stale documents, recent agent activity
   - Project health indicators (library populated, config valid, beads active)
   - Worker list includes single-ticket workers and epic workers separately,
     showing the active bead/epic, worktree, branch, and current phase.
     The worker list is populated from
     `GET /api/projects/:project/workers` (see FEAT-002). Each worker card
     renders the `current_attempt.phase` field as a color-coded phase badge:

     | Phase | Badge color |
     |-------|------------|
     | `queueing` | gray |
     | `launching` | blue |
     | `running` | green |
     | `post_checks` | yellow |
     | `landing` | purple |
     | `done` | teal |
     | `preserved` | orange |
     | `failed` | red |

   - Clicking a worker card expands to show the `recent_phases` timeline from
     `GET /api/projects/:project/workers/:id`. The expanded view also shows
     elapsed time, harness/model identity, cumulative token counts, and a link
     to the full execution evidence bundle for completed attempts.
   - For in-flight workers, the card auto-refreshes using the `workerProgress`
     subscription. The UI updates the phase badge, elapsed time, and token count from
     each progress event without re-fetching the full worker record. When the terminal
     phase event arrives, the stream closes and the card switches to a static completed state.

7. **Epic execution view**
   - Lists open epics separately from single-ticket ready work
   - Shows epic branch name, epic worktree path, active child bead, and merge
     gate status
   - Shows sequential child-bead history on the epic branch
   - Allows the user to inspect the final epic merge candidate and merge-gate
     results before the merge commit is created

8. **Provider / Harness Dashboard**

   A dedicated page — separate from the status dashboard, agent log monitor,
   and worker progress view — that gives operators a clear picture of
   configured providers and the routing signals DDx uses to make harness
   decisions.

   **Provider list panel (left/top):**
   - One row per configured harness (claude, codex, opencode, gemini, …)
   - Columns: harness name, display name, availability badge (`available` |
     `unavailable` | `unknown`), auth state badge, quota/headroom pill
     (`ok` | `blocked` | `unknown`), cost class, freshness timestamp
   - Searchable by harness name, model, status, or cost class
   - Sortable by any column; default sort by availability, then name
   - Filter chips: availability status, auth state, quota state, signal source
   - All data fetched from `GET /api/providers` on load; auto-refreshes on a
     configurable interval (default 60 s)

   **Provider detail panel (right/bottom, shown on row click):**
   - Full routing signal snapshot for the selected harness
   - Per-model quota/headroom with source attribution and `unknown` when no
     stable live source exists
   - Historical usage table: 7-day and 30-day windows for input/output/total
     tokens and cost; `unknown` rendered as `—` not `0`
   - Burn estimate row: daily token rate, subscription burn class, confidence
     label (`high` / `medium` / `low`), freshness timestamp
   - Performance signals: p50/p95 latency, success rate, sample count, window
   - Signal source provenance list: which sources contributed to this snapshot
     (e.g. `stats-cache`, `native-session-jsonl`, `ddx-metrics`, `none`)
   - Data fetched from `GET /api/providers/:harness` on panel open

   **Unknown-state semantics:**
   - `unknown` values are rendered with an explicit `—` or "unknown" label
     and a tooltip explaining why (e.g., "no stable non-PTY quota source
     confirmed for Claude")
   - Fields from provider-native sources are labeled "provider-reported"
   - Fields derived by DDx from observed metrics are labeled "DDx-estimated"
   - Fields that are genuinely absent carry a `?` badge, not a synthesized `0`

   **Relationship to other views:**
   - Provider dashboard is host+user global (not project-scoped); it is
     accessible from the top-level navigation bar alongside the project picker
   - The status dashboard worker cards link to worker detail and the agent log
     monitor — not to the provider dashboard
   - The agent log monitor session detail links to the provider dashboard for
     the harness used in that invocation (via harness name)
   - The provider dashboard does NOT embed worker progress, bead state, or
     execution history; those live in their own dedicated views

   **Reporting and export:**
   - "Copy JSON" action on the detail panel copies the `GET /api/providers/:harness`
     response verbatim — useful for bug reports and operator audits
   - Time-window selector (7d / 30d / custom) scopes the historical usage table

9. **Persona viewer**
   - Browse personas with descriptions and tags
   - See which roles each persona is bound to in project config
   - View full persona content rendered as markdown

### Non-Functional

- **Performance:** Initial page load <2s. API calls <200ms. Graph renders smoothly for 100+ nodes.
- **Deployment:** Single binary. No external runtime dependencies. No separate frontend server in production.
- **Development:** `bun run dev` proxies to Go server. Hot reload for frontend. `go run` for backend.
- **Responsive:** Usable on tablet/laptop screens. Not optimized for mobile.

## User Stories

### US-080: Developer Browses Document Library
**As a** developer onboarding to a project
**I want** to open a web UI and browse all available documents
**So that** I can quickly understand what documents exist and how they're organized

**Acceptance Criteria:**
- Given ddx-server is running, when I open `http://localhost:PORT`, then I am redirected to `/nodes/:nodeId` and see a dashboard with document/artifact counts by category
- Given I click on "Personas", then I see a list of all persona documents with names and descriptions
- Given I click on a persona, then I see its full content rendered as markdown
- Given I press the browser Back button, then I return to the Personas list with my scroll position preserved

**E2E Test:** `artifacts.spec.ts` — full workflow: land on dashboard → navigate to personas → open persona → verify rendered content → navigate back

### US-081: Developer Views and Navigates Dependency Graph
**As a** developer checking document health
**I want** to see a visual graph of document dependencies I can interact with
**So that** I can identify stale documents, understand relationships, and navigate directly to any doc from the graph

**Acceptance Criteria:**
- Given I open the graph view, then I see nodes and edges representing dependency relationships; the graph renders with correct dimensions on first paint (no collapsed/zero-size SVG)
- Given documents are stale, then stale nodes are colored amber; documents with missing dependencies are colored red; fresh documents are green
- Given I click a node, then I navigate to that document's artifact detail view without a full page reload
- Given I press Back after clicking a node, then I return to the graph with the same zoom level and pan position restored
- Given I pan or zoom the graph, then the viewport state persists across the Back/Forward navigation cycle
- Given I apply a staleness filter chip (fresh / stale / missing), then only matching nodes and their connecting edges remain visible
- Given I apply a type filter chip, then only nodes of that media type remain visible
- Given I am on the graph page, then a `View documents` link is visible that navigates to the artifact browser filtered to markdown
- Given I am on an artifact detail page, then a `View in Graph` link navigates to the graph with that node highlighted
- Given the graph is rendered in either light or dark theme, then doc graph edges and arrowheads meet WCAG AA non-text contrast (>=3:1 against the canvas background) in both themes

**E2E Test:** `graph.spec.ts` — full workflow: open graph → verify non-zero render → pan and zoom → click stale node → verify amber color → click node → verify artifact detail → Back → verify same viewport → apply staleness filter → verify node count changes → follow `View documents` link

### US-081b: Developer Views and Regenerates Multi-Media Artifacts
**As a** developer working with generated project artifacts
**I want** the artifact browser to list all artifacts by type and render each
  one by media type, exposing regeneration when provenance exists
**So that** diagrams, images, PDFs, and markdown docs are all inspectable and
  refreshable without leaving the UI

**Acceptance Criteria:**
- Given I navigate to `/nodes/:nodeId/projects/:projectId/artifacts`, then I see
  a list of all project artifacts with category filter chips and a search bar
- Given I filter by category (e.g. "images"), then only artifacts of that
  media type are shown and the URL updates to reflect the filter
- Given an artifact has `media_type: text/markdown`, when I open it, then I see
  rendered markdown with syntax-highlighted code blocks
- Given an artifact has Mermaid-generated SVG metadata, Excalidraw JSON,
  `image/*`, or `application/pdf`, when I open it, then the viewer renders the
  corresponding preview and still exposes the raw sidecar metadata
- Given an artifact's media type is unknown or binary-only, then the viewer
  shows an explicit metadata-only fallback with an open/download affordance
- Given an artifact has `generated_by`, then I see the producing run id,
  generator/prompt summary, source-hash status, and a `Regenerate` action
- Given I click `Regenerate`, then the UI calls `artifactRegenerate`, shows the
  returned run id, and provides a link to both the layer-aware run detail and
  the updated artifact — I can follow either link without losing my place
- Given `generated_by.source_hash` no longer matches, then only the generated
  artifact is marked stale; downstream `depends_on` staleness does not cascade
  from provenance alone
- Given I navigate to `/documents`, then I am redirected to `/artifacts` with a
  `mediaType=text/markdown` filter pre-applied

**E2E Test:** `artifacts.spec.ts` — full workflow: open artifacts list → filter by category → open artifact → verify media renderer → open provenance panel → click Regenerate → verify run link → follow link to run detail → navigate back to artifact → navigate back to list

### US-082: Developer Monitors Bead Status on Kanban Board
**As a** developer tracking work items
**I want** to see beads as cards on a kanban board grouped by status
**So that** I can quickly understand what's ready, blocked, and in progress

**Acceptance Criteria:**
- Given beads exist, when I open the board view, then I see columns for
  open, in_progress, and closed with cards sorted by priority
- Given beads have dependencies, then blocking/blocked relationships are
  visually indicated (dimmed cards, connector lines, or grouping)
- Given I drag a card to a new column, then the bead status updates via API
  and the card visually moves without a page reload
- Given I click a card, then a detail panel opens showing description,
  acceptance criteria, execution runs, and the dependency graph
- Given I close the detail panel, then I return to the board with my scroll
  position and filter state intact

**E2E Test:** `beads.spec.ts` — full workflow: open beads board → verify columns and priority sort → drag card to new column → verify status updated → open detail panel → verify all fields → close panel → verify board state preserved

### US-082b: Developer Searches and Traverses Beads
**As a** developer investigating a work item
**I want** to search beads and navigate their dependency graph
**So that** I can trace blocking relationships and inspect execution evidence

**Acceptance Criteria:**
- Given I type in the search bar, then results filter instantly (client-side,
  no server round-trip) across title, description, acceptance, and labels
- Given I'm viewing a bead's detail, when I click a dependency link, then I
  navigate to that bead's detail without a full page reload
- Given a bead has linked execution runs, then I see their pass/fail status,
  duration, and can expand to see logs
- Given I navigate via a dependency link, then the browser Back button returns
  me to the originating bead's detail

**E2E Test:** `beads.spec.ts` — full workflow: type search term → verify instant filter → open bead detail → click dependency link → verify navigation → expand run log → press Back → verify return to originating bead

### US-082c: Developer Views Bead Execution Evidence
**As a** developer evaluating whether a bead is truly done
**I want** to drill into the execution beads and agent activity associated
  with a work item
**So that** I can verify tests passed and the implementation matches acceptance

**Acceptance Criteria:**
- Given a bead has execution runs, when I open its detail, then I see a list
  of runs with status, harness, duration, and timestamp
- Given I click a run, then I see structured results and raw log output
- Given a bead has linked agent activity, then I see runtime summaries and any
  available native session references
- Given I expand a run and then click back, then the bead detail is still
  open showing the same run list

**E2E Test:** `beads.spec.ts` — full workflow: open bead with runs → verify run list fields → click run → verify structured results and logs → press Back → verify bead detail still showing run list

### US-082d: Supervisor Reviews Bead Against Governing Artifact
**As a** supervisor evaluating completed work
**I want** to review a bead's implementation against its governing spec and
  acceptance criteria side-by-side
**So that** I can confirm the work meets the spec or re-open it

**Acceptance Criteria:**
- Given I click "Review" on a closed bead, then I see the bead's acceptance
  criteria alongside the latest execution results and a link to the governing
  artifact's content
- Given the governing artifact changed since the bead was closed, then a
  drift warning badge is shown
- Given I determine the work is insufficient, when I click "Re-open" and
  provide a reason, then the bead status returns to open with the reason
  recorded and the bead appears in the open column immediately
- Given I re-open a bead, then the reason is visible in the bead's history

**E2E Test:** `beads.spec.ts` — full workflow: open closed bead → click Review → verify AC and results side-by-side → verify drift badge when spec changed → click Re-open with reason → verify bead is open with reason recorded → verify bead appears in open column

### US-082e: Supervisor Re-runs Bead Work from Bead Detail
**As a** supervisor who wants fresh evidence
**I want** to re-run bead work directly from the bead detail
**So that** I can verify the implementation still works after spec changes

**Acceptance Criteria:**
- Given I click "Re-run bead", then the UI dispatches `ddx try <bead>` through
  the server worker API; it does not dispatch `ddx run` directly for bead work
- Given the re-run is dispatched, then a new worker card appears in the Workers
  view and a pending try record appears in the bead's run list within 1s
- Given the try completes, then the result appears in the bead's run list with
  layer `try`, its child layer-1 `run` records, and the UI refreshes without a
  manual reload
- Given a bead has a linked execution definition and I click "Run linked
  checks", then that definition is dispatched as project checks and displayed
  separately from bead re-run history
- Given I want to run an agent review, when I click "Agent review", then
  an agent session is dispatched with the bead's context as the prompt

**E2E Test:** `beads.spec.ts` — full workflow: open bead detail → click Re-run → verify worker appears in Workers view → verify pending try record in run list → wait for completion → verify try + child run records appear → click try record → verify layer-aware detail

### US-082f: Developer Navigates from Bead to Related Artifacts
**As a** developer exploring a work item
**I want** to navigate from a bead to its governing spec, parent bead,
  dependencies, and execution evidence in one click
**So that** I can build a complete picture without switching to the terminal

**Acceptance Criteria:**
- Given a bead has `spec-id: FEAT-001`, when I click the spec link, then I
  navigate to the document viewer showing FEAT-001's content
- Given a bead has a parent, when I click the parent link, then I navigate
  to the parent bead's detail
- Given a bead has execution runs, when I click a run, then I navigate to
  the execution detail view with logs and structured results
- Given I followed any of these links, then the browser Back button returns
  me to the originating bead's detail at the same scroll position

**E2E Test:** `beads.spec.ts` — full workflow: open bead with spec-id and parent → click spec link → verify document viewer → press Back → click parent link → verify parent detail → press Back → click run → verify run detail → press Back → verify return to bead detail

### US-083: Developer Edits Document in Browser
**As a** developer fixing a stale document
**I want** to edit its markdown content in the browser and save
**So that** I don't need to switch to my editor for quick fixes

**Acceptance Criteria:**
- Given I'm viewing a document, when I click "Edit", then I see a markdown editor with the current content
- Given I make changes and click "Save", then the file is written to disk via the API
- Given the save succeeds, then the rendered view updates and staleness is rechecked
- Given I navigate away without saving, then the UI warns me about unsaved changes

**E2E Test:** `artifacts.spec.ts` — full workflow: open markdown artifact → click Edit → modify content → click Save → verify rendered view updated → verify staleness rechecked → open artifact again in editor → navigate away → verify unsaved-changes warning

### US-084: Developer Searches Across All Documents
**As a** developer looking for how something is documented
**I want** to search across all document content
**So that** I can find relevant docs without knowing their location

**Acceptance Criteria:**
- Given I type in the search bar, then results appear from all document types
- Given results are shown, then I see matching snippets with the search term highlighted
- Given I click a result, then I navigate to that document's detail view
- Given I press Back, then I return to the search results with my query intact

**E2E Test:** `artifacts.spec.ts` — full workflow: type search term → verify results with highlighted snippets → click result → verify document detail → press Back → verify search results still showing same query

### US-085: Developer Creates and Manages Beads in the UI
**As a** developer triaging work
**I want** to create beads, set their status, and manage dependencies in the browser
**So that** I can manage the work queue visually

**Acceptance Criteria:**
- Given I click "New Bead", then I see a form with title, type, priority, labels, description, acceptance
- Given I submit the form, then a bead is created via the API and appears in the list immediately without a page reload
- Given I click a bead's status, then I can transition it (open → in_progress → closed)
- Given I transition a bead's status, then the change is reflected in the list and board views without a reload
- Given I'm viewing a bead, then I can add/remove dependencies by selecting other beads

**E2E Test:** `beads.spec.ts` — full workflow: click New Bead → fill form → submit → verify bead appears in list → click status → transition to in_progress → verify list updated → open bead → add dependency → verify dependency link visible

### US-085b: Operator Tracks Live Worker Progress from Status Dashboard
**As an** operator supervising a running execute-loop
**I want** to see each worker's current execution phase in real time
**So that** I can tell whether an agent is making progress, stalled, or done
**without** leaving the browser

**Acceptance Criteria:**
- Given a worker is in the `running` phase, when I open the status dashboard,
  then I see a green "running" phase badge on the worker card with the elapsed
  time and current token count
- Given the worker transitions to `post_checks`, then the badge updates to
  yellow "post_checks" within the subscription delivery latency (no manual refresh required)
- Given I click a worker card, then I see the `recent_phases` timeline showing
  when each phase started (timestamp and elapsed since attempt began)
- Given the worker reaches a terminal phase (`done`, `preserved`, or `failed`),
  then the card updates to the terminal badge color and stops live-updating
- Given I look at a worker card, then I can see harness, model, and bead title
  without expanding the card
- Given I expand a worker card for a completed attempt, then I see a link to
  the execution evidence bundle for that attempt

**E2E Test:** `workers.spec.ts` — full workflow: open workers view with a running worker → verify phase badge and elapsed time → observe badge transition via subscription → expand card → verify phases timeline → wait for terminal phase → verify card stops updating → follow evidence link

### US-087: Operator Inspects Provider Availability and Routing Signals
**As an** operator about to queue a batch of agent work
**I want** to see configured providers with their availability, auth state,
  and quota/headroom at a glance
**So that** I can choose which harnesses are viable before dispatching

**Acceptance Criteria:**
- Given providers are configured, when I open the Provider Dashboard, then I
  see one row per harness with availability, auth state, and quota/headroom badges
- Given a provider's quota/headroom has no stable live source, then the badge
  shows "unknown" with a tooltip — not a fabricated `ok` or `blocked` value
- Given I click a row, then the detail panel opens showing the full routing
  signal snapshot with source attribution and freshness timestamp
- Given I click the harness link in the agent log monitor for an invocation,
  then I navigate to that harness's detail in the Provider Dashboard
- Given I navigate back from the provider detail, then I return to the provider
  list with the same sort and filter state

**E2E Test:** `providers.spec.ts` — full workflow (using fixture scenarios): open provider list → verify availability/auth/quota badges per fixture scenario → click row → verify detail panel → hover unknown badge → verify tooltip text → click harness link from agent log → verify navigation to provider detail → navigate back

### US-088: Operator Reviews Provider Utilization and Burn Rate
**As an** operator tracking subscription usage
**I want** to see historical token consumption, burn rate, and confidence level
  for each configured provider
**So that** I can balance load across providers and anticipate quota pressure

**Acceptance Criteria:**
- Given a provider has DDx-observed or provider-native usage history, when I
  view its detail panel, then I see a 7-day and 30-day usage table with
  input/output/total tokens and cost where known
- Given a token or cost field has no trustworthy source, then it renders as `—`
  not `0` or a fabricated value
- Given a burn estimate exists, then the detail panel shows the daily token rate,
  subscription burn class, confidence label, and the timestamp of the last
  contributing signal
- Given I want to share a provider's signal state with a colleague, then I can
  use "Copy JSON" to get the raw API response for the selected harness

**E2E Test:** `providers.spec.ts` — full workflow (using fixture scenarios): open provider detail → verify 7d/30d usage tables → verify `—` for missing values → verify burn estimate row → click Copy JSON → verify clipboard contents match API response → change time window → verify table updates

### US-088b: Operator Distinguishes Source Types in Provider Detail
**As an** operator debugging a routing decision
**I want** to know whether each signal field came from the provider directly,
  DDx estimation, or is unknown
**So that** I can trust or question the data DDx is routing on

**Acceptance Criteria:**
- Given a field came from a provider-native source (e.g. `stats-cache`), then
  a "provider-reported" label or badge is shown
- Given a field was derived by DDx from observed invocation metrics, then a
  "DDx-estimated" label is shown
- Given no source exists for a field, then a `?` badge appears — no `0` or
  inferred value is displayed
- Given I hover a `?` or "unknown" badge, then a tooltip explains why the
  value is unavailable (e.g., "no stable non-PTY quota source confirmed")

**E2E Test:** `providers.spec.ts` — full workflow (using fixture scenarios 2 and 5): open provider detail → find provider-reported field → verify badge label → find DDx-estimated field → verify badge → hover `?` badge → verify tooltip text matches expected explanation

### US-086: Developer Monitors Agent Activity in Real Time
**As a** developer running agents against my project
**I want** to see agent invocations as they happen with routing metadata and
available session references
**So that** I can diagnose issues and track token usage without DDx duplicating
provider logs

**Acceptance Criteria:**
- Given agents have been invoked, when I open the agent log view, then I see
  recent invocations sorted by time
- Given I click an invocation, then I see DDx runtime metadata plus native
  session or trace references and any available DDx-owned detail
- Given I filter by harness, then only invocations for that harness are shown
- Given I look at the summary, then I see total tokens consumed by harness and
  by day where a signal source exists
- Given I click the harness name in an invocation row, then I navigate to that
  harness's detail in the Provider Dashboard

**E2E Test:** `workers.spec.ts` — full workflow: open agent log view → verify invocations sorted by time → click invocation → verify metadata and session references → apply harness filter → verify filtered list → click harness name → verify navigation to Provider Dashboard

### US-086b: Operator Drills Through Work, Try, and Run Evidence
**As an** operator auditing queue-drain behavior
**I want** run history to preserve the `work` -> `try` -> `run` hierarchy
**So that** I can distinguish queue orchestration, bead attempts, and single
  agent invocations without flattening them into one ambiguous list

**Acceptance Criteria:**
- Given I navigate to `/nodes/:nodeId/projects/:projectId/runs`, then I see
  run records for the project with a layer badge on each (`work`, `try`, or `run`)
- Given I filter by layer `work`, then only work records are shown and the URL
  reflects the filter
- Given I click a `work` record, then I navigate to its detail showing queue
  inputs, selected beads, stop condition, and child try attempts in order
- Given I click a child `try` record, then I navigate to its detail showing
  bead id, base/result revisions, worktree path, merge or preserve outcome,
  checks, and child layer-1 run records
- Given I click a layer-1 `run` record, then I navigate to its detail showing
  prompt/config summary, power bounds, selected harness/provider/model,
  duration, token and cost signals, output, and evidence links
- Given I am on any run detail, then breadcrumb navigation is visible and
  pressing Back takes me to the parent record without a full page reload
- Given a run produced an artifact, then the run detail links to that artifact;
  navigating to the artifact shows a "Produced by run" link back to this run
- Given I filter the run history, then available layer filters are exactly
  `work`, `try`, and `run`; no other run type labels appear

**E2E Test:** `runs.spec.ts` — full workflow: navigate to project runs → filter by layer `work` → click work record → verify fields and child tries → click try → verify fields and child runs → click layer-1 run → verify all fields → follow evidence link → press Back through breadcrumbs to run list → verify filter state restored

### US-086c: Operator Re-queues a Run from the Runs Tab
**As an** operator who wants fresh evidence on an existing bead or to
  restart a queue drain
**I want** to re-queue a `try` or `run` row, and to start a new worker
  from a `work` row, directly from the Runs tab
**So that** I do not have to leave the UI to dispatch follow-up work

**Acceptance Criteria:**
- Given I am on a `layer=try` row whose originating bead is open and has
  no active try, then a `Re-queue` button is visible. Clicking it opens a
  confirmation dialog prefilled with the original `(bead, harness, model)`
  config; the client generates an `idempotencyKey` and submits it with the
  `runRequeue` mutation (FEAT-010); on success the originating bead is
  reopened and a `run_requeue` audit event appears on the bead
- Given I am on a `layer=run` row with replayable `(prompt, harness,
  model)`, then `Re-queue` is visible. If the run terminated with
  success, the dialog requires explicit double-confirmation
- Given I am on a `layer=work` row, then a **`Start worker from this
  drain`** action is visible (instead of `Re-queue`). Confirming it
  dispatches a new worker prefilled with the original queue inputs, stop
  condition, selected beads, and harness/profile
- Given the originating bead is closed, then the `Re-queue` button is
  disabled and a "Reopen bead first" link is shown in its place
- Given a duplicate submission (double-click race or accidental retry)
  arrives with the same `idempotencyKey`, then the second response
  carries `deduplicated=true`, the bead is reopened only once, and only
  one `run_requeue` audit event is appended

**E2E Test:** `runs.spec.ts` — full workflow: open Runs tab → click try-row Re-queue → verify confirmation dialog with prefilled config → confirm → verify originating bead reopens and audit event appears → re-click Re-queue (race) → verify single audit event and `deduplicated=true` response → switch to closed-bead row → verify Re-queue disabled with "Reopen bead first" link → switch to layer=work row → verify `Start worker from this drain` action prefills queue inputs → confirm → verify new worker appears in Workers tab

### US-081c: Developer Repairs Graph Integrity Issues from the UI
**As a** developer maintaining a healthy document graph
**I want** to repair known integrity issues (missing dependencies, duplicate IDs)
  directly from the graph view
**So that** I don't have to hand-edit YAML files to clear issues the UI already
  understands

**Acceptance Criteria:**
- Given IntegrityPanel shows issues, when I expand an issue, then I see the
  proposed change (diff-style: which file, which key, old value → new value)
  alongside the existing "Copy" affordances
- Given an issue is auto-repairable (missing dep removal, duplicate ID rename
  for an unambiguous choice), then a `Apply fix` button is visible for that
  issue; non-repairable issues (parse errors, cycles, ambiguous duplicates,
  missing required roots with no content) show a `Cannot auto-repair` label
  with a tooltip explaining why
- Given I click `Apply fix`, then the server validates: path is within the
  project, the issue still matches the current file hash, and the structured
  YAML/frontmatter edit is safe to apply; on success, the file is written and
  the graph reloads showing the resolved or changed state
- Given validation fails (stale issue, dirty/unwritable file, invalid YAML,
  path traversal, generated-file guard), then an inline error banner names the
  exact failure and the file is not modified
- Given the fix resolves the issue, then the issue disappears from
  IntegrityPanel without a manual reload; if the fix creates a new secondary
  issue, the new issue appears immediately in the panel
- Given I repair duplicate IDs and the chosen file has downstream dependents,
  then the panel warns "N files reference this ID — they will be updated" before
  I confirm (cascade-aware, not just single-file)
- Given concurrent repairs are in-flight, then the second apply request returns
  a 409 Conflict; the UI surfaces "Another repair is in progress — try again"
  without a broken state

**E2E Test:** `graph.spec.ts` — full workflow: open graph → expand auto-repairable issue → verify diff preview → click Apply fix → verify file written + issue removed + graph reloads → expand non-repairable issue → verify Cannot auto-repair label + tooltip → introduce stale-issue scenario → click Apply fix → verify error banner + file unchanged

### US-081a: Developer Follows Intra-Repo Markdown Links
**As a** developer reading one doc and referenced by another
**I want** markdown links inside the rendered view to navigate to the
  linked doc within the UI
**So that** I can traverse the document graph by following the links the
  author actually wrote, not only the dependency edges

**Acceptance Criteria:**
- Given a rendered doc contains a link like
  `[FEAT-006](../features/FEAT-006-agent-service.md)` or
  `[spec](/docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md)`,
  when I click the link, then I navigate to that doc's detail view in the
  SAME tab without a full page reload
- Given I click a link and then hit the browser Back button, then I return
  to the previous doc with its scroll position restored
- Given a link points outside the repo (absolute http[s] URL), when I click
  it, then it opens in a new tab (`target=_blank rel=noopener`) and DOES
  NOT navigate away from the UI
- Given a link points to an anchor on the same doc (`#section-id`), when I
  click it, then the page scrolls to that anchor without changing the route

**E2E Test:** `artifacts.spec.ts` — full workflow: open doc with intra-repo link → click relative link → verify same-tab navigation → press Back → verify scroll position → click external link → verify new tab → click anchor link → verify scroll to anchor

### US-082g: Developer Sorts and Filters the Beads List
**As a** developer triaging the backlog
**I want** to sort and filter the beads list by common criteria
**So that** I can find what's ready, what's blocked, what's mine, and what
  was most recently updated without scrolling through hundreds of rows

**Acceptance Criteria:**
- Given the beads list is open, then sort controls are present for
  `Priority` (P0 first by default), `Updated`, `Created`, `Title`. Clicking
  a header toggles asc/desc; active sort is visible.
- Given I click a status filter chip (Open / Ready / Blocked / In-progress
  / Closed), then only beads matching that status appear AND the URL
  updates to `?status=<name>` so the filter is bookmarkable
- Given I apply multiple filters (e.g. `?status=open&priority=p0`), then
  all filters compose with AND semantics; the URL reflects every active
  chip
- Given I type in the search box with active filters, then the search
  narrows the already-filtered set (filter scope, not search scope)
- Given I click a label chip on any bead row, then the list filters to
  that label; clicking it again removes the filter
- Given the list is filtered to empty, then a zero-state panel explains
  the filter combination and offers a "clear filters" affordance
- Given I reload a URL with filter + sort params, then the same view is
  restored without interaction

**E2E Test:** `beads.spec.ts` — full workflow: open list → click sort header → verify sort → click status filter → verify URL and filtered results → add second filter → verify AND composition → type in search → verify narrowed results → click label chip → verify filter added → remove chip → verify full list → copy URL with filters → reload → verify same view

### US-083a: Developer Toggles WYSIWYG vs Plain-Markdown Editor
**As a** developer editing a doc in the browser
**I want** to choose between a rich preview-while-editing view and a
  raw-markdown textarea
**So that** I can fix a typo in the rendered view or hand-edit frontmatter
  without a round-trip

**Acceptance Criteria:**
- Given I'm editing a doc, then a mode toggle is visible with two options:
  `WYSIWYG` (default) and `Plain`
- Given I'm in `WYSIWYG` mode, when I toggle to `Plain`, then the editor
  switches to a monospace textarea containing the exact raw markdown
  (frontmatter preserved); unsaved edits are retained across the toggle
- Given I'm in `Plain` mode and I toggle back to `WYSIWYG`, then my raw
  edits re-render immediately
- Given the doc has YAML frontmatter, `Plain` mode shows and edits the
  frontmatter; `WYSIWYG` mode shows a collapsible "Frontmatter" panel
  above the content
- Given I save from either mode, then the file is written to disk via
  `documentWrite` with the raw markdown; the render view refreshes; the
  doc's "Updated" timestamp advances

**E2E Test:** `artifacts.spec.ts` — full workflow: open markdown artifact in editor (WYSIWYG) → make edit → toggle to Plain → verify raw markdown with frontmatter and edits preserved → edit frontmatter in Plain → toggle back to WYSIWYG → verify renders correctly → save → verify render updated and timestamp advanced

### US-085c: Developer Deletes (Soft-Closes) a Bead from the UI
**As a** developer cleaning up mis-filed beads
**I want** to delete a bead from the UI
**So that** mistakes don't pollute the queue

**Acceptance Criteria:**
- Given I am on a bead's detail panel, then a `Delete` button is visible
  with destructive styling (icon + danger palette token; not red alone)
- Given I click `Delete`, then a confirmation modal opens showing the
  bead ID, title, and a required text field where I type the bead ID to
  confirm (prevents accidental deletes)
- Given I confirm, then the bead is soft-closed via the standard close
  path (status → `closed`, reason recorded as `deleted via UI`); the
  list refreshes; the detail panel closes; the URL redirects to the
  beads list
- Given I cancel the modal, then the bead is unchanged and the modal
  closes; focus returns to the Delete button
- Given a bead has active child beads (parent of N open beads), then
  the confirmation modal surfaces the child count and requires
  additional `--cascade` checkbox opt-in; canceled by default

**E2E Test:** `beads.spec.ts` — full workflow: open bead detail → click Delete → verify modal shows ID and title → cancel → verify bead unchanged and focus returned → click Delete again → type bead ID → confirm → verify bead status closed → verify redirect to beads list → verify bead not in open list

### US-086a: Developer Sees Streaming Agent Response Text
**As a** developer watching an agent work on a bead
**I want** to see the agent's response content stream in as it's produced
**So that** I can tell what the agent is doing without tailing log files

**Acceptance Criteria:**
- Given a worker is in a running phase for a bead, when I open the worker
  detail, then a "Live response" panel shows accumulated response text,
  updating as the `workerProgress` subscription delivers `text_delta`
  frames
- Given the response contains tool calls, then each tool call is rendered
  as a collapsible card showing the tool name, inputs, and (once returned)
  the output, interleaved with text in delivery order
- Given the worker reaches a terminal phase, then the live-response panel
  freezes at its final content with a "Completed at HH:MM:SS" timestamp
  and a link to the execution evidence bundle
- Given the subscription disconnects mid-stream, then a banner shows
  "Reconnecting…" and the panel auto-resumes on reconnect without losing
  the text received so far

**E2E Test:** `workers.spec.ts` — full workflow: open running worker detail → verify live response panel updates → verify tool call card rendered → wait for terminal phase → verify panel frozen with timestamp → verify evidence bundle link → simulate disconnect → verify reconnecting banner → verify text preserved after reconnect

### US-095: Operator Initiates Work from the UI
**As an** operator managing a project without context-switching to a
  terminal
**I want** to start common work flows from the UI
**So that** "drain the queue", "re-align the specs", and "run the tests"
  are one click away

**Acceptance Criteria:**
- Given I am on a project view, then an `Actions` panel exposes at
  minimum three actions:
  - `Drain queue` — dispatches `ddx work` as a server-side worker
  - `Re-align specs` — dispatches a HELIX `align` action as a worker
  - `Run checks` — dispatches the project's execution definitions
- Given I click an action, then a confirmation dialog shows the expected
  scope (e.g. "N ready beads will be attempted"); on confirm, the worker
  is dispatched and appears in the Workers list within 1s
- Given dispatching fails (e.g. queue already has an active worker of
  the same kind), then the dialog surfaces the error with a clear
  remediation hint; no silent failure
- Given I start an action, then the originating button shows a spinner
  until the worker is `running`; the button becomes an anchor to that
  worker's detail
- Given I lack permission or a prerequisite is missing (no ready beads,
  no spec tree, no check suite), then the action is disabled with a
  tooltip explaining the prerequisite

**E2E Test:** `actions.spec.ts` — full workflow: open project view → verify Actions panel → click Drain queue → verify confirmation dialog with bead count → confirm → verify worker appears in Workers list within 1s → verify button becomes worker link → click disabled action → verify tooltip explains prerequisite

### US-096: Operator Views Model Efficacy and Runs Comparisons
**As an** operator pursuing the cost-tiered throughput-per-dollar goal
**I want** to see per-model completion rates, cost, and latency, and run
  A/B comparisons between prompts or models
**So that** I can steer the system toward the models and prompts that
  actually work

**Acceptance Criteria:**
- Given closed beads carry `kind:cost` + `kind:routing` evidence events,
  when I open the `Efficacy` view, then a table lists every distinct
  `(harness, provider, model)` tuple used in the last N days with:
  success count, attempt count, completion rate, median tokens in/out,
  median duration, median cost (or `—` when no cost signal)
- Given I filter by tier / label / spec-id, then the table refilters
  live; URL encodes the filter for bookmarking
- Given I click `Compare`, then a dialog lets me pick N `(model, prompt)`
  pairs; on submit, DDx dispatches `ddx agent compare` and the resulting
  `ComparisonRecord` appears under `Comparisons` with per-arm outputs,
  diffs, and (if configured) grader scores
- Given a model's completion rate crosses the configured adaptive-min-tier
  floor (trailing-window success rate < threshold), then a warning badge
  appears on that row with a tooltip linking to the routing metrics doc
- Given I click a row, then a detail panel shows the last 10 attempts
  with per-attempt outcome, evidence links, and links to the execution
  bundles; click-through to the bead that originated each attempt

**E2E Test:** `actions.spec.ts` — full workflow: open Efficacy view → verify model table with all columns → apply label filter → verify URL encodes filter → hover warning badge → verify tooltip → click row → verify detail panel with attempts → click bead link → verify navigation to bead detail → press Back → verify detail panel still open

### US-097: Developer Browses and Binds Personas
**As a** developer configuring a project for consistent agent behavior
**I want** to browse available personas in the UI and bind them to roles
**So that** I don't have to hand-edit `.ddx/config.yaml`

**Acceptance Criteria:**
- Given personas are installed (via `ddx install` or the DDx default
  plugin), when I open the `Personas` page, then I see a card per persona
  with name, role tags, and a short description pulled from the
  persona file's frontmatter
- Given a persona's card, when I click it, then I see the full persona
  body rendered as markdown plus the list of projects currently binding
  this persona and the role each binds
- Given I am on a persona detail and I click `Bind to role`, then a form
  lets me pick a role (code-reviewer, test-engineer, implementer, etc.)
  and project; submitting writes the binding to `.ddx/config.yaml`'s
  `persona_bindings` map via a server-side mutation
- Given a role is already bound in the selected project, then the form
  warns "This will replace the existing binding: `X`" and requires
  confirmation before overwriting
- Given I complete a binding, then the persona detail shows the new
  binding immediately without a page reload

**E2E Test:** `personas.spec.ts` — full workflow: open Personas page → verify cards with name/tags/description → click persona → verify markdown render and current bindings → click Bind to role → fill form → submit → verify binding appears on persona detail → bind same role again → verify warning and confirmation required

### US-098: Operator Browses and Installs Plugins
**As an** operator setting up DDx for a new project
**I want** to see available plugins and install one from the UI
**So that** I don't have to know the exact `ddx install <name>` invocation

**Acceptance Criteria:**
- Given the `Plugins` page is open, then I see all plugins from the
  built-in registry plus any configured additional registries, with
  name, version, type (workflow / plugin / persona-pack / template-pack),
  description, keywords, and install status (installed / available /
  update-available)
- Given I click an available plugin, when I click `Install`, then a
  modal shows the install scope (global vs project) and required disk
  space; on confirm, the UI triggers `ddx install <name>` server-side
  and streams install progress via a worker-backed dispatch
- Given installation completes, then the plugin card updates to
  "installed" status without a page reload
- Given a plugin is installed, then I can view its manifest
  (package.yaml), its skills, its prompts, and its templates from the
  same card; an `Uninstall` action with confirmation is present
- Given a plugin has an available update, then the card shows both
  current and target versions and an `Update` action

**E2E Test:** `plugins.spec.ts` — full workflow: open Plugins page → verify registry listing with status badges → click available plugin → click Install → verify modal with scope and disk space → confirm → observe install progress → verify status updated to installed → view manifest/skills/prompts → click Uninstall → confirm → verify removed

### US-099: Developer Uses a Keyboard Command Palette
**As a** developer who lives on the keyboard
**I want** a command palette (Cmd+K / Ctrl+K) for navigation and actions
**So that** I can jump to any bead, document, or action without using the
  mouse

**Acceptance Criteria:**
- Given I press `Cmd+K` (or `Ctrl+K` on non-mac), then a modal search
  palette opens with focus in the input field
- Given I type in the palette, then results include: documents (matching
  path or title), beads (matching id or title), actions from US-095,
  and navigation entries (every top-level page)
- Given I press `Enter` on a result, then I navigate to that destination
  (or trigger that action); the palette closes
- Given I press `Escape`, then the palette closes without changes
- Given I press `Cmd+K` while on a bead detail, then bead-specific actions
  (Claim, Unclaim, Close, Reopen, Re-run, Delete) appear at the top of
  the result list with their keybindings
- Given I'm on a deep URL and open the palette, then navigation results
  preserve the current project/node context (relative paths, not
  absolute)

**E2E Test:** `command-palette.spec.ts` — full workflow: press Cmd+K → verify palette opens with focus → type document title → verify result appears → press Enter → verify navigation and palette closed → reopen → press Escape → verify palette closed → navigate to bead detail → press Cmd+K → verify bead-specific actions at top of list → trigger Claim action → verify bead claimed

## Provider Dashboard: Playwright Fixture Scenarios

The provider dashboard has deterministic fixture data requirements so that
Playwright tests can cover all meaningful display states without live provider
credentials. The fixture layer is a static JSON handler mounted at
`/api/providers` and `/api/providers/:harness` during test runs. Each scenario
below defines the fixture variant and the expected rendered state.

### Fixture Scenarios

**Scenario 1 — all-healthy (green path)**

Provider list returns two harnesses, both `available` + `authenticated` +
`quota_headroom: ok`. Detail panel shows populated 7d/30d usage, burn estimate
with `confidence: high`, and `signal_sources: ["native-session-jsonl", "ddx-metrics"]`.
Expected: all badges green, no `?` badges, no unknown labels.

```json
[
  {
    "harness": "codex", "status": "available", "auth_state": "authenticated",
    "quota_headroom": "ok", "cost_class": "subscription",
    "signal_sources": ["native-session-jsonl", "ddx-metrics"],
    "freshness_ts": "2026-04-14T05:00:00Z",
    "recent_success_rate": 0.99, "recent_latency_p50_ms": 3100
  },
  {
    "harness": "claude", "status": "available", "auth_state": "authenticated",
    "quota_headroom": "ok", "cost_class": "subscription",
    "signal_sources": ["stats-cache", "ddx-metrics"],
    "freshness_ts": "2026-04-14T05:00:00Z",
    "recent_success_rate": 0.97, "recent_latency_p50_ms": 4200
  }
]
```

**Scenario 2 — quota unknown (common real-world state for Claude)**

Claude harness has `quota_headroom: unknown` because no stable non-PTY quota
source exists. Codex is `quota_headroom: ok`. Expected: Claude row shows an
"unknown" pill with tooltip "no stable non-PTY quota source confirmed". All
other state fields are populated.

**Scenario 3 — provider unavailable**

One harness has `status: unavailable` and `auth_state: unauthenticated`.
Expected: row shows red "unavailable" badge and gray "unauthenticated" badge.
Detail panel shows empty usage tables with `—` cells and a signal source list
of `["none"]`.

**Scenario 4 — stale signals / low-confidence burn**

Provider is available and authenticated, but `freshness_ts` is 48 hours ago
and `burn_estimate.confidence` is `low`. Expected: freshness timestamp renders
with a "stale" indicator; burn estimate row shows `confidence: low` label.

**Scenario 5 — all unknown (offline / fresh install)**

Provider list returns one harness with `status: unknown`, `auth_state: unknown`,
`quota_headroom: unknown`, `signal_sources: ["none"]`. Expected: every badge
shows "unknown" or `—`; no zeros appear in usage tables; tooltip text explains
each unknown field.

**Scenario 6 — search and filter**

Provider list returns five harnesses with mixed statuses. Test: entering a
search term filters the list in real time (client-side); selecting a filter
chip for `status: available` hides unavailable rows; clearing chips restores
the full list.

### Fixture File Layout

```
cli/internal/server/frontend/
└── src/
    └── testing/
        └── fixtures/
            └── providers/
                ├── all-healthy.json
                ├── quota-unknown.json
                ├── provider-unavailable.json
                ├── stale-signals.json
                └── all-unknown.json
```

Playwright tests mount the appropriate fixture via request interceptors before
each scenario. The fixture format matches the `/api/providers` and
`/api/providers/:harness` response shapes defined in FEAT-002.

## Implementation Notes

### Embedding Pattern (Pocketbase-style)

```go
//go:embed all:frontend/build
var frontendFiles embed.FS

func main() {
    distFS, _ := fs.Sub(frontendFiles, "frontend/build") // relative to embed.go in cli/internal/server/
    
    mux := http.NewServeMux()
    mux.Handle("/api/", apiRouter)
    mux.Handle("/mcp/", mcpHandler)
    mux.Handle("/", spaHandler(http.FS(distFS))) // SPA fallback
}

// spaHandler serves static files, falling back to index.html for SPA routing
func spaHandler(fs http.FileSystem) http.Handler {
    fileServer := http.FileServer(fs)
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if _, err := fs.Open(r.URL.Path); err != nil {
            r.URL.Path = "/"
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

### Development Workflow

```bash
# Terminal 1: Go server
cd cli/internal/server && go run . --library-path ../../../.ddx/library

# Terminal 2: SvelteKit dev server with proxy
cd cli/internal/server/frontend && bun run dev
# svelte.config.js proxies /graphql and /api to localhost:8080
```

### Project Structure

```
ddx/
└── cli/
    └── internal/
        └── server/
            ├── server.go          # HTTP API and MCP handlers
            ├── embed.go           # embed.FS declaration
            └── frontend/
                ├── package.json
                ├── svelte.config.js
                └── src/
                │   ├── app.html
                │   ├── routes/
                │   │   ├── +layout.svelte
                │   │   ├── nodes/
                │   │   │   └── [nodeId]/
                │   │   │       ├── +page.svelte         # node dashboard
                │   │   │       └── projects/
                │   │   │           └── [projectId]/
                │   │   │               ├── +page.svelte # project dashboard
                │   │   │               ├── beads/
                │   │   │               │   └── +page.svelte      # beads list (query: beads)
                │   │   │               ├── artifacts/
                │   │   │               │   └── +page.svelte      # artifacts list/viewer (query: artifacts)
                │   │   │               ├── documents/
                │   │   │               │   └── +page.svelte      # legacy markdown document route
                │   │   │               ├── runs/
                │   │   │               │   └── +page.svelte      # layer-aware run history (query: runs)
                │   │   │               └── graph/
                │   │   │                   └── +page.svelte      # dependency graph (query: docGraph)
                │   │   └── graphql/
                │   │       ├── beads.gql
                │   │       ├── artifacts.gql
                │   │       ├── runs.gql
                │   │       └── workers.gql
                │   └── components/
                └── build/         # SvelteKit output (embedded into Go)
```

## Dependencies

- FEAT-002 (DDx server HTTP/MCP API) — the web UI consumes GraphQL at `/graphql`;
  REST endpoints stay frozen for CLI/MCP compatibility
- FEAT-004 (Beads) — for bead board; queries `beads` and mutations `beadCreate`, `beadUpdate`, `beadClaim`
- FEAT-005 (Artifacts) — for artifact identity, sidecar metadata,
  `media_type`, and `generated_by`
- FEAT-007 (Doc graph) — for dependency graph visualization; queries
  `docGraph` with artifact nodes and provenance edges
- FEAT-006 (Agent service) — for agent activity and invocation detail
- FEAT-010 (Executions) — for unified `work` / `try` / `run` records and the
  narrow `artifactRegenerate` write surface
- FEAT-014 (Agent Usage Awareness and Routing Signals) — governs the routing
  signal model consumed by the provider dashboard
- FEAT-021 (Dashboard UI) — defines URL scheme and navigation patterns
- ADR-021 (Operator-Prompt Beads as the Web Write Path) — trust contract,
  audit-as-bead, multi-node delegation, prompt-injection threat model, and
  allowed-mutation scope for the Story 15 operator-prompt input on the
  project home page
- SvelteKit, Svelte 5, Bun, graphql-request, graphql-ws, bits-ui, lucide-svelte, Tailwind
- Playwright for testing (no MSW)
- Go embed.FS, Chi or net/http, gqlgen

## Out of Scope

- Real-time sync / collaborative editing
- Mobile-optimized layout
- Authentication (localhost-only for v1; auth deferred to FEAT-002 server security)
- Server-side search or pagination for beads (client-side only — GraphQL cursors in v2)

## Test Plan

### Unit Tests

- Component tests for beads list, filter chips, detail panel
- Page-level tests for artifact viewer, graph page, worker detail, run detail
- Store tests for node context, project selection, dark mode

### E2E Tests (Playwright)

All tests run against the built SvelteKit app served by `ddx-server`. Each
spec file covers the complete end-to-end workflow for its user stories —
every test exercises the full flow from entry point through actions to
outcome, including navigation back.

| E2E Spec | User Stories | Workflow Covered |
|---|---|---|
| `navigation.spec.ts` | US-094 (via FEAT-021) | Root redirect, project picker, node identity in nav bar |
| `artifacts.spec.ts` | US-080, US-081b, US-083, US-083a, US-084, US-081a | Browse library → open artifact → render by media type → edit + save → search → follow intra-repo links |
| `graph.spec.ts` | US-081, US-081c | Open graph → pan/zoom → identify stale nodes → click node → navigate back; graph cross-link from artifact detail; integrity issue expand → apply fix → verify graph reload; non-repairable issue tooltip; stale-issue error |
| `beads.spec.ts` | US-082, US-082b, US-082c, US-082d, US-082e, US-082f, US-082g, US-085, US-085b, US-085c | Board view, search/filter, execution evidence, review vs spec, re-run, navigate to artifacts, sort/filter with URL state, create/manage, worker progress, delete |
| `workers.spec.ts` | US-085b, US-086, US-086a | Worker list, live phase updates via subscription, streaming response text with tool calls |
| `runs.spec.ts` | US-086b, US-086c | Navigate to runs → filter by layer → drill work→try→run → verify fields at each level → breadcrumb back to list; re-queue try/run/work flows incl. idempotency dedupe and closed-bead block |
| `providers.spec.ts` | US-087, US-088, US-088b | Provider list with fixture scenarios → detail panel → unknown semantics → Copy JSON → harness cross-link |
| `actions.spec.ts` | US-095, US-096 | Initiate work from UI → dispatch worker → efficacy table → model comparison |
| `personas.spec.ts` | US-097 | Browse personas → view detail → bind to role → handle conflict |
| `plugins.spec.ts` | US-098 | Browse registry → install → observe progress → view manifest → uninstall |
| `command-palette.spec.ts` | US-099 | Open palette → search → navigate → context-aware bead actions |
