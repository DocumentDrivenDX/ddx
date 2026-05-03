---
ddx:
  id: FEAT-021
  depends_on:
    - helix.prd
    - FEAT-008
    - FEAT-020
    - SD-019
---
# Feature: Multi-Node Dashboard UI

**ID:** FEAT-021
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

Extends the FEAT-008 web UI with a node/project-aware dashboard. The UI can
show a combined view of beads and DDx run activity across all registered
projects on a node, or narrow to a specific project for context-dependent
views (artifact browser, dependency graph, run history, commit log). Node,
project, and page are all embedded in the URL so every view is bookmarkable
and shareable.

The dashboard UI is implemented as a SvelteKit application with graphql-request for
GraphQL data fetching. Every page uses `+page.ts` load functions to fetch data
from the `/graphql` endpoint defined in SD-022.

## Problem Statement

**Current situation:** The FEAT-008 UI is implicitly scoped to the single
project the server was started in. There is no node concept, no project picker,
and no combined cross-project view for beads or layer-aware run history.

**Pain points:**
- Operator cannot see all active work across projects in one dashboard
- Navigating to a different project requires restarting the server in that
  project's directory
- Deep links to a specific bead, artifact, or run record are not stable because the URL
  carries no project context
- Queue-drain evidence is flattened as agent sessions instead of preserving
  the `work` -> `try` -> `run` hierarchy from FEAT-010
- Node identity is invisible in the UI — there is no indication of which
  machine or server instance is being viewed

**Desired outcome:** A single `ddx server` UI that surfaces everything
happening on the node. The operator can scan the full bead queue and run
history across all projects, drill into a specific project for artifact-level
work, and bookmark any view with a stable URL.

## URL Structure

All routes embed context so every view is directly addressable:

```
/                                     → redirect to /nodes/:nodeId
/federation                           → federation overview (hub-mode only); see FEAT-026
/nodes/:nodeId                        → node overview (health, project list)
/nodes/:nodeId/runs                   → combined run history (all projects)
/nodes/:nodeId/beads                  → combined bead queue (all projects)
/nodes/:nodeId/projects/:projectId                   → project overview
/nodes/:nodeId/projects/:projectId/beads             → project beads
/nodes/:nodeId/projects/:projectId/artifacts         → artifact browser
/nodes/:nodeId/projects/:projectId/documents         → document browser
/nodes/:nodeId/projects/:projectId/graph             → doc dependency graph
/nodes/:nodeId/projects/:projectId/commits           → commit log
/nodes/:nodeId/projects/:projectId/runs              → project run history
/nodes/:nodeId/projects/:projectId/runs/:runId       → project run detail
```

**Combined views** (`/nodes/:nodeId/runs`, `/nodes/:nodeId/beads`) operate
across all registered projects. They call node-scoped read APIs for beads and
the unified run substrate (since one server = one node). When the node is a
**federation hub** (FEAT-026, ADR-007), these combined views accept
`?scope=federation` to fan out across registered spokes; the default scope
remains node-local. The hub also serves `/nodes/:spokeId/...` for any
registered spoke, resolving the route through the federated read fan-out.

**Project-scoped views** (`/nodes/:nodeId/projects/:projectId/...`) pass the
project ID to the existing `/api/projects/:project/...` API routes defined in
SD-019. The UI must always resolve which project context to bind before
making API calls.

The `:nodeId` segment is the stable node ID from `GET /api/node` (e.g.
`node-7029e8d6`). The `:projectId` segment is the stable project ID from
`GET /api/projects` (e.g. `proj-96d7ea83`).

## Views

### Node Overview (`/nodes/:nodeId`)

- Node name, ID, and uptime
- Health summary cards: library status, bead store, doc graph per project
- Project list with last-seen timestamp and health badge
- Quick links to combined runs and combined beads
- Server version and started_at

**GraphQL Query:** `node` query from FEAT-020, `projects` query for project list.

**File:** `src/routes/nodes/[nodeId]/+page.svelte`

### Combined Bead Queue (`/nodes/:nodeId/beads`)

All beads from all registered projects merged into one view. The project each
bead belongs to is shown as a column/badge. Supports all filter/sort/search
capabilities from FEAT-008 US-082, applied across projects.

- Project filter chip to narrow to one project (updates URL)
- Full-text search across all projects (client-side)
- Status columns: open, in_progress, closed
- Priority sort within each column
- Clicking a bead navigates to its project-scoped detail:
  `/nodes/:nodeId/projects/:projectId/beads#:beadId`

**GraphQL Query:** `beads` with `projectID:` argument per SD-019.

**File:** `src/routes/nodes/[nodeId]/beads/+page.svelte`

### Combined Run History (`/nodes/:nodeId/runs`)

All DDx run records across all registered projects, newest first. The project
each record belongs to is shown inline, and records retain their FEAT-010 layer
label: `work`, `try`, or `run`.

- Project filter to narrow
- Harness filter
- Layer filter (`work`, `try`, `run`)
- Status filter
- Time range filter
- Click to expand: DDx metadata, native session references, token usage,
  parent/child links, and artifact producer links
- Layer-aware row expansion replaces the retired Sessions and Executions
  tabs: `layer=run` rows expose the AgentSession transcript / billing /
  cached-token detail; `layer=try` rows expose the `.ddx/executions/`
  bundle metadata, check results, and verdict; `layer=work` rows expose
  queue inputs and child run links.
- Re-queue affordances per FEAT-008 US-086c (try-layer + run-layer
  re-queue button; work-layer "Start worker from this drain"); every
  successful re-queue emits a `run_requeue` audit event on the
  originating bead per FEAT-010 §"Re-queue audit events".
- Same capabilities as FEAT-008 US-086 and US-086b

**GraphQL Query:** `runs` from FEAT-010 with optional `projectID` omitted.

**File:** `src/routes/nodes/[nodeId]/runs/+page.svelte`

### Project Overview (`/nodes/:nodeId/projects/:projectId`)

- Project name, path, git remote
- Library configured: yes/no
- Bead summary: counts by status
- Recent run activity: last 5 run records
- Quick links to project-scoped views

**GraphQL Query:** `project` by ID, `beads` count aggregation, `runs`
filtered by project.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/+page.svelte`

### Project Beads (`/nodes/:nodeId/projects/:projectId/beads`)

Same as FEAT-008 bead views (list, kanban, ready queue, detail) scoped to one
project. This is the same UI already specified in FEAT-008 US-082 through
US-086, with the URL carrying the project context.

**GraphQL Query:** `beadsByProject` with cursor pagination per SD-019.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/beads/+page.svelte`

### Artifact Browser (`/nodes/:nodeId/projects/:projectId/artifacts`)

FEAT-008 artifact browser scoped to the selected project.

**GraphQL Query:** `artifacts` with project context per SD-019.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/artifacts/+page.svelte`

### Document Browser (`/nodes/:nodeId/projects/:projectId/documents`)

Compatibility route for markdown-focused document navigation. It redirects to
or aliases the project artifact browser with a `media_type=text/markdown`
filter; new navigation should use `/artifacts`.

**GraphQL Query:** `artifacts` with project context per SD-019.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/documents/+page.svelte`

### Document Dependency Graph (`/nodes/:nodeId/projects/:projectId/graph`)

FEAT-008 dependency graph view scoped to the selected project.

**GraphQL Query:** `docGraph` per SD-019.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/graph/+page.svelte`

### Commit Log (`/nodes/:nodeId/projects/:projectId/commits`)

Git commit history for the project, served by a new `GET /api/projects/:project/commits`
endpoint. Displays:
- Commit SHA (short), author, date, subject line
- Expand to show full commit message and changed files
- Filter by author, date range
- Link from a commit to any bead whose `closing_commit_sha` matches

**GraphQL Query:** `commits` with cursor pagination per SD-019.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/commits/+page.svelte`

### Project Run History (`/nodes/:nodeId/projects/:projectId/runs`)

DDx run records filtered to one project. This is the same UI as the combined
run history view with the project context bound in the URL.

**GraphQL Query:** `runs` with project filter per FEAT-010.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/runs/+page.svelte`

### Project Run Detail (`/nodes/:nodeId/projects/:projectId/runs/:runId`)

Layer-aware run detail for a single `work`, `try`, or `run` record. Work
records show child try attempts, try records show child layer-1 runs, and
layer-1 runs show prompt/config summary, selected harness/provider/model,
power bounds, cost/token/duration signals, evidence links, and produced
artifact links.

**GraphQL Query:** `run` by id with project filter per FEAT-010.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/runs/[runId]/+page.svelte`

## Navigation

The global navigation bar shows:
- Node name + ID (links to node overview)
- Project picker dropdown (populated from `/api/projects`)
  - Selecting a project navigates to `/nodes/:nodeId/projects/:projectId`
  - "All projects" option navigates to combined views
- Active page tab: Overview | Beads | Artifacts | Graph | Runs | Workers | Personas | Plugins | Commits | Efficacy

Sessions and Executions are no longer top-level activity tabs. The unified
Runs view (this feature + FEAT-008 §5) carries layer chips (`work`, `try`,
`run`) and an inline row-expansion pane that surfaces what those tabs used
to show — agent-session transcript and cost detail under `layer=run`,
execute-bead bundle detail under `layer=try`, queue-drain detail under
`layer=work`. Legacy URLs (`/sessions`, `/executions`, `/executions/[id]`)
respond with 302 redirects (NOT 301) to the matching filtered Runs URL with
all query params preserved and a `Sunset` header set for the deprecation
window.

The project picker changes the `:projectId` segment in-place while preserving
the current page tab. So switching project while on the Graph tab navigates to
the new project's graph directly. When switching from a run detail to another
project, the UI falls back to that project's run history unless the same run id
exists in the target project.

**GraphQL Query:** `projects` for picker options.

**File:** `src/lib/components/ProjectPicker.svelte`

## Requirements

### Functional

1. Root `/` redirects to `/nodes/:nodeId` using the ID from `GET /api/node`.
2. All URLs listed in the URL Structure section are valid deep links — loading
   them directly in a browser renders the correct view with data.
3. The node overview page is populated from `GET /api/node` and
   `GET /api/projects`.
4. Combined bead view fetches all projects' beads client-side and merges them.
5. Combined run view fetches node-wide run records from the unified run
   substrate and preserves `work`, `try`, and `run` layer labels.
6. Project-scoped views bind all API calls to
   `/api/projects/:project/...` per SD-019.
7. The project picker is present on every project-scoped page and updates the
   URL when changed.
8. A new `GET /api/projects/:project/commits` endpoint serves git commit
   history for the project.
9. Commit log view links commits to matching beads where
   `closing_commit_sha` matches.
10. All filter/sort/search state is stored in URL query parameters so filtered
    views are also bookmarkable.
11. Project run detail routes support parent/child drill-down:
    `work` -> `try` -> `run`, with breadcrumbs back up the hierarchy.
12. Project artifact routes expose FEAT-008 media-type rendering and link to
    producing runs when `generated_by` is present.

### Non-Functional

- Combined bead view with 500 beads across 5 projects renders in <500ms
  (client-side merge).
- Navigation between views within the same node+project uses client-side
  routing (no full page reload).
- The UI degrades gracefully when a project becomes unavailable: its beads and
  run records are shown with a stale badge rather than being silently dropped.

## User Stories

### US-090: Operator Views All Active Work in One Dashboard
**As an** operator managing multiple projects on one machine
**I want** to see all open beads and recent run records from every project
**So that** I can understand the full workload without switching context

**Acceptance Criteria:**
- Given multiple projects are registered, when I open `/nodes/:nodeId/beads`,
  then I see beads from all projects with a project badge on each
- Given I filter by project, then only beads from that project are shown and
  the URL updates to reflect the filter
- Given I open `/nodes/:nodeId/runs`, then I see run records from all projects
  merged and sorted by time with a project badge and a layer badge (`work`,
  `try`, or `run`) on each record
- Given I click a run record, then I navigate to that run's project-scoped
  detail at `/nodes/:nodeId/projects/:projectId/runs/:runId`; the `projectId`
  is carried in each run record so the combined list can form this URL without
  a separate lookup; I see layer-appropriate detail: queue inputs for `work`,
  bead/worktree fields for `try`, execution summary for `run`
- Given I navigate back from a run detail, then the combined run list is
  restored with the same sort and filter state

**E2E Test:** `node-beads.spec.ts` and `node-runs.spec.ts` — full workflows:
- node-beads: open combined beads → verify cross-project beads with project badges → filter by project → verify URL and filtered results → click bead → verify navigation to project-scoped bead detail → press Back
- node-runs: open combined runs → verify cross-project records with layer and project badges → filter by layer → click work record → verify detail → click child try → verify fields → press Back through hierarchy

### US-091: Operator Navigates to a Project-Scoped View
**As an** operator investigating one project
**I want** to select a project and see its artifacts, graph, runs, and commits
**So that** I can do project-specific work without losing the node context

**Acceptance Criteria:**
- Given I am on the node overview, when I click a project, then I navigate to
  that project's overview at `/nodes/:nodeId/projects/:projectId`
- Given I am on the artifact browser for project A, when I switch projects in
  the picker, then I navigate to the artifact browser for project B without
  losing the current page context (e.g. still on `/artifacts`)
- Given I copy the current URL and open it in a new tab, then I see the same
  view with the same project and page selected
- Given I switch projects while on a run detail, then the UI falls back to
  that project's run history

**E2E Test:** `navigation.spec.ts` — full workflow: open node overview → click project → verify project overview URL → open artifact browser → switch project in picker → verify URL and content updated to project B → copy URL → open in new tab → verify same view → navigate to run detail → switch project → verify fallback to run history

### US-092: Operator Bookmarks a Filtered View
**As an** operator who checks the ready queue every morning
**I want** to bookmark the ready queue filtered to my project
**So that** I can open it directly without navigating through the UI

**Acceptance Criteria:**
- Given I filter the bead list to status=open and project=ddx, then the URL
  contains those filter parameters
- Given I reload the page with that URL, then the same filter is applied
  without any interaction
- Given I share the URL with a colleague on the same machine, then they see
  the same view

**E2E Test:** `navigation.spec.ts` — full workflow: apply status + project filters → verify URL params → copy URL → reload page → verify same filter applied without interaction

### US-093: Operator Traces a Bead to Its Closing Commit
**As an** operator reviewing completed work
**I want** to see the git commit that closed a bead
**So that** I can inspect the code change that fulfilled the bead's acceptance

**Acceptance Criteria:**
- Given a bead has `closing_commit_sha` set, when I view the commit log, then
  that commit is highlighted and linked to the bead
- Given I click the commit link from the bead detail, then I navigate to the
  commit log with that commit in view
- Given I click the bead link from the commit row, then I navigate to that
  bead's detail

**E2E Test:** `commits.spec.ts` — full workflow: open bead with closing commit → click commit link → verify commit log opens with commit highlighted → click bead link from commit row → verify bead detail opens → press Back → verify commit log restored at same position

### US-094: Operator Sees Node Identity in the UI
**As an** operator with multiple machines
**I want** to see which node I am connected to
**So that** I don't confuse the eitri server with the bragi server

**Acceptance Criteria:**
- Given I open the web UI, then the node name and ID are visible in the
  navigation bar or node overview
- Given the URL contains `:nodeId`, then it matches the ID returned by
  `GET /api/node`

**E2E Test:** `navigation.spec.ts` — full workflow: open UI → verify node name and ID visible in nav → verify URL nodeId matches GET /api/node response

### US-094b: Operator Opens Layer-Aware Run Routes
**As an** operator reviewing DDx execution evidence
**I want** project-scoped run routes for history and detail
**So that** queue drains, bead attempts, and single agent invocations remain
  distinct in the URL and UI

**Acceptance Criteria:**
- Given I navigate to `/nodes/:nodeId/projects/:projectId/runs`, then I see
  project-scoped run records with filter chips for layer, status, bead,
  harness, and time range; active filters are encoded in the URL
- Given I click a `work` record, then I navigate to
  `/nodes/:nodeId/projects/:projectId/runs/:runId` and see the work detail:
  queue inputs, selected beads, stop condition, and child try attempts
- Given I click a child `try`, then I see bead id, base/result revisions,
  worktree path, merge/preserve result, checks, and child layer-1 runs
- Given I click a layer-1 `run`, then I see prompt/config summary, power
  bounds, selected harness/provider/model, tokens, cost, duration, output, and
  evidence links
- Given I am viewing any run detail, then breadcrumbs show the full
  `work → try → run` path and each crumb is a navigable link
- Given a run produced an artifact, then the detail route links to the artifact
  browser entry and that artifact links back to the producing run

**E2E Test:** `runs.spec.ts` — full workflow: navigate to project runs → apply layer filter → verify URL → click work record → verify work detail fields → click child try → verify try fields → click layer-1 run → verify all fields → follow evidence link → press Back through breadcrumbs to run list → verify filter state restored → follow artifact link from run → verify artifact links back to run

## New API Required

In addition to the endpoints delivered by FEAT-020 and FEAT-010, FEAT-021
requires:

| Endpoint | Description |
|----------|-------------|
| `GET /api/projects/:project/commits` | Git log for the project; supports `?limit=&since=&author=` |

All other data is served by endpoints already defined in FEAT-002, FEAT-004,
FEAT-006, FEAT-008, FEAT-010, and SD-019.

## Implementation Notes

### Router

SvelteKit handles routing automatically based on `src/routes/` file structure.
Routes are nested using the folder hierarchy:

```
src/routes/
├── +layout.svelte              → Node+Project layout with nav
├── +page.ts                    → Root redirect to /nodes/:nodeId
└── nodes/
    └── [$nodeId]/
        ├── +layout.ts          → Load node context
        ├── +page.svelte        → NodeOverview
        ├── beads/              → CombinedBeads
        │   └── +page.svelte
        ├── runs/               → CombinedRuns
        │   └── +page.svelte
        └── projects/
            └── [$projectId]/
                ├── +page.svelte      → ProjectOverview
                ├── beads/            → ProjectBeads
                │   └── +page.svelte
                ├── artifacts/        → ArtifactBrowser
                │   └── +page.svelte
                ├── documents/        → DocumentBrowser
                │   └── +page.svelte
                ├── graph/            → DocGraph
                │   └── +page.svelte
                ├── commits/          → CommitLog
                │   └── +page.svelte
                └── runs/             → ProjectRuns
                    ├── +page.svelte
                    └── [$runId]/
                        └── +page.svelte
```

The `[$nodeId]` and `[$projectId]` dynamic route segments capture URL parameters.
SvelteKit's `+layout.ts` and `+page.ts` files define load functions that
fetch data via graphql-request.

### Data Layer

graphql-request is the GraphQL client for data fetching, with graphql-ws for
subscriptions. Pages define queries inline using the `gql` tagged template
and manually typed response interfaces in `+page.ts` load functions.

Example bead list query:

```typescript
// src/routes/nodes/$nodeId/beads/+page.ts
import { gql } from 'graphql-request';
import { createClient } from '$lib/gql/client';

const BEAD_LIST_QUERY = gql`
  query BeadList($projectID: ID) {
    beads(projectID: $projectID, first: 50) {
      edges {
        node {
          id
          title
          status
          priority
          projectID
        }
      }
    }
  }
`;

export const load = async ({ fetch, params }) => {
  const client = createClient(fetch);
  const data = await client.request(BEAD_LIST_QUERY, { projectID: params.projectId });
  return { beads: data.beads };
};
```

graphql-ws subscription support enables real-time updates without polling.

Example project run query:

```typescript
// src/routes/nodes/$nodeId/projects/$projectId/runs/+page.ts
import { gql } from 'graphql-request';
import { createClient } from '$lib/gql/client';

const RUN_LIST_QUERY = gql`
  query ProjectRuns($projectID: ID!, $layer: RunLayer) {
    runs(projectID: $projectID, layer: $layer, first: 50) {
      edges {
        node {
          id
          layer
          status
          beadID
          artifactID
          parentRunID
          startedAt
          completedAt
        }
      }
    }
  }
`;

export const load = async ({ fetch, params, url }) => {
  const client = createClient(fetch);
  const layer = url.searchParams.get('layer');
  const data = await client.request(RUN_LIST_QUERY, {
    projectID: params.projectId,
    layer
  });
  return { runs: data.runs };
};
```

### Project Context

The `ProjectStore` (using Svelte 5 runes) stores the selected node ID and
project ID. It's populated from URL params by `+layout.ts` load functions:

```typescript
// src/lib/stores/project.ts
import { writable } from 'svelte/store';

export const projectStore = writable<{
  nodeId: string;
  projectId?: string;
} | null>(null);
```

Components read from `projectStore` to construct API URLs and filter queries.

### Testing

Playwright e2e tests verify the complete end-to-end workflow for each user
story. Each spec covers the full flow: entry point → actions → outcome →
navigation back. Tests use real ddx-server with seeded fixture data; no
mocking of the GraphQL or HTTP layer.

| E2E Spec | User Stories | Workflow Covered |
|---|---|---|
| `navigation.spec.ts` | US-091, US-092, US-094 | Root redirect → node overview → project picker → project switch preserving page context → bookmarkable URL round-trip → node identity visible |
| `node-beads.spec.ts` | US-090 (beads) | Combined bead view → cross-project badges → filter by project → click bead → navigate to project-scoped detail → Back |
| `node-runs.spec.ts` | US-090 (runs) | Combined run history → cross-project layer badges → filter → drill work→try→run → breadcrumbs back to list |
| `runs.spec.ts` | US-094b | Project runs → apply layer filter → work detail → try detail → run detail → evidence link → artifact link → Back through full hierarchy |
| `commits.spec.ts` | US-093 | Bead with closing commit → commit log → highlighted commit → bead cross-link → Back |

## Dependencies

- FEAT-008 (web UI foundation — build pipeline, embed pattern, existing views)
- FEAT-020 (node identity and project registry — required for /api/node and /api/projects)
- SD-019 (project-scoped API routing — /api/projects/:project/...)
- FEAT-004 (beads)
- FEAT-006 (agent service metadata and native session references)
- FEAT-007 (doc graph)
- FEAT-010 (unified run substrate and run read APIs)
- FEAT-012 (git awareness — for commit log endpoint)

## Federation Compatibility Hooks (FEAT-026)

When the node runs as a **federation hub** (FEAT-026, ADR-007), the UI gains:

- `/federation` — federation overview route showing the hub, its registered
  spokes, each spoke's liveness (`live` / `stale` / `offline`), and
  compatibility status (`live` / `degraded` / `rejected`). Owned by FEAT-026.
- `/nodes/:nodeId/...` resolves across the federation: when `:nodeId` is a
  registered spoke, the hub serves the route from the spoke's federated read
  surface. Spoke UIs remain directly reachable as a fallback.
- Combined views (`/nodes/:nodeId/beads`, `/nodes/:nodeId/runs`) accept the
  `?scope=federation` query parameter; when set on a hub, the view fans out
  across all spokes and tags each row with `node_id`, `project_id`, and a
  `node` badge in addition to the existing project badge.
- Rows sourced from a `stale`, `offline`, or `degraded` spoke carry the
  matching badge; the page does not fail when a spoke is unreachable.

The naming convention from ADR-007 applies to UI copy: prefer
**federation / hub / spoke** over `coordinator`, `primary`, `replica`.

FEAT-021 contributes only the URL grammar and route stubs above; the
federation registry, hub endpoints, fan-out, version handshake, and
`?scope=federation` data plumbing are owned by FEAT-026.

## Out of Scope

- Federation registry, hub/spoke HTTP endpoints, fan-out client, federated
  GraphQL fields, and version handshake. Owned by FEAT-026 and ADR-007.
  FEAT-021 contributes only the route grammar (`/federation`,
  hub-resolved `/nodes/:nodeId/...`, `?scope=federation`).
- Cross-machine writes (federation is read-only aggregation in v1; see
  ADR-007 write-routing contract for Story 15).
- Authentication beyond ts-net (ADR-006).
- Real-time push (polling is sufficient for v1).
- Mobile layout.
