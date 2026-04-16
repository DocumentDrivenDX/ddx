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
**Status:** Complete
**Priority:** P1
**Owner:** DDx Team

## Overview

Extends the FEAT-008 web UI with a node/project-aware dashboard. The UI can
show a combined view of beads and agent sessions across all registered projects
on a node, or narrow to a specific project for context-dependent views
(document browser, dependency graph, commit log). Node, project, and page are
all embedded in the URL so every view is bookmarkable and shareable.

The dashboard UI is implemented as a SvelteKit application with graphql-request for
GraphQL data fetching. Every page uses `+page.ts` load functions to fetch data
from the `/graphql` endpoint defined in SD-022.

## Problem Statement

**Current situation:** The FEAT-008 UI is implicitly scoped to the single
project the server was started in. There is no node concept, no project picker,
and no combined cross-project view for beads or agent sessions.

**Pain points:**
- Operator cannot see all active work across projects in one dashboard
- Navigating to a different project requires restarting the server in that
  project's directory
- Deep links to a specific bead or document are not stable because the URL
  carries no project context
- Node identity is invisible in the UI — there is no indication of which
  machine or server instance is being viewed

**Desired outcome:** A single `ddx server` UI that surfaces everything
happening on the node. The operator can scan the full bead queue and agent
session log across all projects, drill into a specific project for
document-level work, and bookmark any view with a stable URL.

## URL Structure

All routes embed context so every view is directly addressable:

```
/                                     → redirect to /nodes/:nodeId
/nodes/:nodeId                        → node overview (health, project list)
/nodes/:nodeId/sessions               → combined agent sessions (all projects)
/nodes/:nodeId/beads                  → combined bead queue (all projects)
/nodes/:nodeId/projects/:projectId                   → project overview
/nodes/:nodeId/projects/:projectId/beads             → project beads
/nodes/:nodeId/projects/:projectId/documents         → document browser
/nodes/:nodeId/projects/:projectId/graph             → doc dependency graph
/nodes/:nodeId/projects/:projectId/commits           → commit log
/nodes/:nodeId/projects/:projectId/sessions          → project agent sessions
```

**Combined views** (`/nodes/:nodeId/sessions`, `/nodes/:nodeId/beads`) operate
across all registered projects. They call the existing `/api/agent/sessions`
and `/api/beads` endpoints which are already node-scoped (since one server =
one node). A future multi-node coordinator would fan these out across nodes.

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
- Quick links to combined sessions and combined beads
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

### Combined Agent Sessions (`/nodes/:nodeId/sessions`)

All agent sessions across all registered projects, newest first. The project
each session belongs to is shown inline.

- Project filter to narrow
- Harness filter
- Time range filter
- Click to expand: DDx metadata, native session references, token usage
- Same capabilities as FEAT-008 US-086

**GraphQL Query:** `agentSessions` from FEAT-020.

**File:** `src/routes/nodes/[nodeId]/sessions/+page.svelte`

### Project Overview (`/nodes/:nodeId/projects/:projectId`)

- Project name, path, git remote
- Library configured: yes/no
- Bead summary: counts by status
- Recent agent activity: last 5 sessions
- Quick links to project-scoped views

**GraphQL Query:** `project` by ID, `beads` count aggregation, `agentSessions`
filtered by project.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/+page.svelte`

### Project Beads (`/nodes/:nodeId/projects/:projectId/beads`)

Same as FEAT-008 bead views (list, kanban, ready queue, detail) scoped to one
project. This is the same UI already specified in FEAT-008 US-082 through
US-086, with the URL carrying the project context.

**GraphQL Query:** `beadsByProject` with cursor pagination per SD-019.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/beads/+page.svelte`

### Document Browser (`/nodes/:nodeId/projects/:projectId/documents`)

FEAT-008 artifact browser scoped to the selected project.

**GraphQL Query:** `documents` with project context per SD-019.

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

### Project Agent Sessions (`/nodes/:nodeId/projects/:projectId/sessions`)

Agent sessions filtered to one project. Same UI as combined sessions view.

**GraphQL Query:** `agentSessions` with project filter per SD-019.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/sessions/+page.svelte`

## Navigation

The global navigation bar shows:
- Node name + ID (links to node overview)
- Project picker dropdown (populated from `/api/projects`)
  - Selecting a project navigates to `/nodes/:nodeId/projects/:projectId`
  - "All projects" option navigates to combined views
- Active page tab: Beads | Documents | Graph | Sessions | Commits

The project picker changes the `:projectId` segment in-place while preserving
the current page tab. So switching project while on the Graph tab navigates to
the new project's graph directly.

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
5. Combined session view fetches from `GET /api/agent/sessions` (node-wide).
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

### Non-Functional

- Combined bead view with 500 beads across 5 projects renders in <500ms
  (client-side merge).
- Navigation between views within the same node+project uses client-side
  routing (no full page reload).
- The UI degrades gracefully when a project becomes unavailable: its beads and
  sessions are shown with a stale badge rather than being silently dropped.

## User Stories

### US-090: Operator Views All Active Work in One Dashboard
**As an** operator managing multiple projects on one machine
**I want** to see all open beads and recent agent sessions from every project
**So that** I can understand the full workload without switching context

**Acceptance Criteria:**
- Given multiple projects are registered, when I open the combined bead view,
  then I see beads from all projects with a project badge on each
- Given I filter by project, then only beads from that project are shown and
  the URL updates to reflect the filter
- Given I open the combined sessions view, then I see agent sessions from all
  projects merged and sorted by time

### US-091: Operator Navigates to a Project-Scoped View
**As an** operator investigating one project
**I want** to select a project and see its documents, graph, and commits
**So that** I can do project-specific work without losing the node context

**Acceptance Criteria:**
- Given I am on the node overview, when I click a project, then I navigate to
  that project's overview at `/nodes/:nodeId/projects/:projectId`
- Given I am on the document browser for project A, when I switch projects in
  the picker, then I navigate to the document browser for project B
- Given I copy the current URL and open it in a new tab, then I see the same
  view with the same project and page selected

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

### US-093: Operator Traces a Bead to Its Closing Commit
**As an** operator reviewing completed work
**I want** to see the git commit that closed a bead
**So that** I can inspect the code change that fulfilled the bead's acceptance

**Acceptance Criteria:**
- Given a bead has `closing_commit_sha` set, when I view the commit log, then
  that commit is highlighted and linked to the bead
- Given I click the commit link from the bead detail, then I navigate to the
  commit log with that commit in view

### US-094: Operator Sees Node Identity in the UI
**As an** operator with multiple machines
**I want** to see which node I am connected to
**So that** I don't confuse the eitri server with the bragi server

**Acceptance Criteria:**
- Given I open the web UI, then the node name and ID are visible in the
  navigation bar or node overview
- Given the URL contains `:nodeId`, then it matches the ID returned by
  `GET /api/node`

## New API Required

In addition to the endpoints delivered by FEAT-020, FEAT-021 requires:

| Endpoint | Description |
|----------|-------------|
| `GET /api/projects/:project/commits` | Git log for the project; supports `?limit=&since=&author=` |

All other data is served by endpoints already defined in FEAT-002, FEAT-004,
FEAT-006, FEAT-008, and SD-019.

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
        ├── sessions/           → CombinedSessions
        │   └── +page.svelte
        └── projects/
            └── [$projectId]/
                ├── +page.svelte      → ProjectOverview
                ├── beads/            → ProjectBeads
                │   └── +page.svelte
                ├── documents/        → DocumentBrowser
                │   └── +page.svelte
                ├── graph/            → DocGraph
                │   └── +page.svelte
                ├── commits/          → CommitLog
                │   └── +page.svelte
                └── sessions/         → ProjectSessions
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

Playwright e2e tests verify the complete user flow. Each test file covers a
specific feature area:

| Playwright Spec | TC Identifiers Covered |
|-----------------|------------------------|
| `e2e/navigation.spec.ts` | TC-010, TC-012 |
| `e2e/node-beads.spec.ts` | TC-023 |
| `e2e/node-agents.spec.ts` | TC-022 |
| `e2e/projects.spec.ts` | TC-010, TC-012 |
| `e2e/workers.spec.ts` | TC-009 |
| `cli/internal/server/frontend/e2e/app.spec.ts` | TC-001 through TC-008 |

Each test file documents the TC identifiers it covers in a header comment.
Test names use the pattern `TC-NNN.N — description` to match the test plan.

**Example:** `e2e/node-beads.spec.ts` covers TC-023 through TC-023.9, testing
the cross-project beads view at `/nodes/:nodeId/beads`.

## Dependencies

- FEAT-008 (web UI foundation — build pipeline, embed pattern, existing views)
- FEAT-020 (node identity and project registry — required for /api/node and /api/projects)
- SD-019 (project-scoped API routing — /api/projects/:project/...)
- FEAT-004 (beads)
- FEAT-006 (agent sessions)
- FEAT-007 (doc graph)
- FEAT-012 (git awareness — for commit log endpoint)

## Out of Scope

- Multi-node federation (coordinator aggregating state from multiple servers)
- Cross-machine views (each node's UI shows only what that node knows)
- Authentication
- Real-time push (polling is sufficient for v1)
- Mobile layout
