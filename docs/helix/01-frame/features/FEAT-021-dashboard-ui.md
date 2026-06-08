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
from the `/graphql` endpoint defined in SD-022. The shared run-detail surface
uses the FEAT-008/FEAT-010 tab structure, and FEAT-019 drill-down links land
on the same tab strip:

- `work` → `overview`
- `try` → `overview`, `prompt`, `response`, `tools`, `evidence`
- `run` → `overview`, `prompt`, `response`, `session`, `tools`, `evidence`

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
/nodes/:nodeId/workers                → combined worker workbench (all projects)
/nodes/:nodeId/projects/:projectId                   → project overview
/nodes/:nodeId/projects/:projectId/beads             → project beads
/nodes/:nodeId/projects/:projectId/artifacts         → artifact browser
/nodes/:nodeId/projects/:projectId/documents         → document browser
/nodes/:nodeId/projects/:projectId/graph             → doc dependency graph
/nodes/:nodeId/projects/:projectId/commits           → commit log
/nodes/:nodeId/projects/:projectId/runs              → project run history
/nodes/:nodeId/projects/:projectId/runs/:runId       → project run detail
/nodes/:nodeId/projects/:projectId/workers           → project worker workbench
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
  top-level activity tabs and opens the shared run-detail tab strip:
  `layer=work` rows expose `overview` only plus queue inputs and child run
  links; `layer=try` rows expose `overview`, `prompt`, `response`, `tools`,
  and `evidence`; `layer=run` rows expose `overview`, `prompt`, `response`,
  `session`, `tools`, and `evidence`.
- Re-queue affordances per FEAT-008 US-086c (try-layer + run-layer
  re-queue button; work-layer "Start worker from this drain"); every
  successful re-queue emits a `run_requeue` audit event on the
  originating bead per FEAT-010 §"Re-queue audit event schema".
- Same capabilities as FEAT-008 US-086 and US-086b

**GraphQL Query:** `runs` from FEAT-010 with optional `projectID` omitted.

**File:** `src/routes/nodes/[nodeId]/runs/+page.svelte`

### Combined Worker Workbench (`/nodes/:nodeId/workers`)

All DDx work workers across the selected node's registered projects merged into
one operator view. When the node is a federation hub, the view accepts
`?scope=federation` and includes workers reported by registered spokes. Each row
shows the owner node, project, worker id, state, current bead id/title when
available, attempt id, phase, harness/provider/model, elapsed time, last event
time, freshness (`connected`, `stale`, `disconnected`, `offline`), and the
worker command mode (`once`, `drain`, or `watch`).

- Scope toggle: `LOCAL` shows this node only; `FEDERATION` fans out across
  registered spokes when hub-mode is enabled.
- Project and node filters are URL-backed.
- Rows link to the owning project worker detail route.
- A detail drawer shows recent phase transitions, captured output, prompt,
  route/model, current bead, attempt bundle link, and stop/cancel actions.
- `+ Add worker` starts one or more autonomous `ddx work` workers for a selected
  project through the server. The server is a launcher and observer; the bead
  store remains the claim authority per ADR-022.
- Hub-mode worker start/stop commands are owner-targeted. The hub forwards to
  the node that owns the selected project; it never broadcasts a worker command.

**GraphQL Queries:** `reportedWorkers`, `workersByProject`, `queueAndWorkersSummary`.

**GraphQL Mutations:** `startWorker`, `stopWorker`.

**File:** `src/routes/nodes/[nodeId]/workers/+page.svelte`

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
artifact links. The tab strip is layer-aware and shared across FEAT-008,
FEAT-010, and FEAT-019: `work` shows `overview`; `try` shows `overview`,
`prompt`, `response`, `tools`, and `evidence`; `run` shows `overview`,
`prompt`, `response`, `session`, `tools`, and `evidence`.

**GraphQL Query:** `run` by id with project filter per FEAT-010.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/runs/[runId]/+page.svelte`

### Project Worker Workbench (`/nodes/:nodeId/projects/:projectId/workers`)

Project-scoped variant of the combined worker workbench. It shows only workers
for the selected project and keeps the same row, detail, start, stop, and live
progress affordances. This is the primary page for increasing capacity on one
queue: the operator can see ready depth, running count, worker cap, and start
additional autonomous workers without opening a shell.

**GraphQL Queries:** `workersByProject`, `reportedWorkers`, `queueAndWorkersSummary`.

**GraphQL Mutations:** `startWorker`, `stopWorker`.

**File:** `src/routes/nodes/[nodeId]/projects/[projectId]/workers/+page.svelte`

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
to show, using the shared `work` / `try` / `run` tab strip described above:
`work` shows `overview` only plus queue inputs and child run links; `try`
shows `overview`, `prompt`, `response`, `tools`, and `evidence`; `run`
shows `overview`, `prompt`, `response`, `session`, `tools`, and
`evidence`. Legacy URLs (`/sessions`, `/executions`, `/executions/[id]`)
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
13. Combined worker view lists every worker for the selected node and, in
    hub-mode with `scope=federation`, every reachable spoke with node/project
    badges and freshness state.
14. Worker detail exposes the current bead, attempt phase, route/model, recent
    events, captured output, prompt, and stop action.
15. Starting workers from the UI creates autonomous `ddx work` workers through
    the server for the selected project. The UI can request `count > 1`, but the
    server enforces project and global worker caps before launching.
16. Hub-mode worker start/stop is owner-targeted. Commands for spoke projects
    forward to the owning spoke and are refused when the spoke is offline or
    lacks the advertised write capability.
17. Bead queue mutations (`create`, `update`, `approve`, `block`, `cancel`,
    `reopen`, dependency edits when implemented) are project-scoped and
    owner-targeted in hub-mode.
18. Spec/document edits from the UI are project-scoped, path-confined, and
    owner-targeted in hub-mode. Editing `docs/helix/**` is the primary spec-edit
    workflow for basic functionality.

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
  queue inputs, selected beads, stop condition, child try attempts, and the
  `overview` tab
- Given I click a child `try`, then I see bead id, base/result revisions,
  worktree path, merge/preserve result, checks, child layer-1 runs, and the
  `overview` / `prompt` / `response` / `tools` / `evidence` tabs
- Given I click a layer-1 `run`, then I see prompt/config summary, power
  bounds, selected harness/provider/model, tokens, cost, duration, output,
  evidence links, and the `overview` / `prompt` / `response` / `session` /
  `tools` / `evidence` tabs
- Given I am viewing any run detail, then breadcrumbs show the full
  `work → try → run` path and each crumb is a navigable link
- Given a run produced an artifact, then the detail route links to the artifact
  browser entry and that artifact links back to the producing run

**E2E Test:** `runs.spec.ts` — full workflow: navigate to project runs → apply layer filter → verify URL → click work record → verify work detail fields → click child try → verify try fields → click layer-1 run → verify all fields → follow evidence link → press Back through breadcrumbs to run list → verify filter state restored → follow artifact link from run → verify artifact links back to run

### US-100: Operator Sees Every Worker in One Pane
**As an** operator running DDx workers across projects and nodes
**I want** one Workers workbench that shows every worker with node and project
context
**So that** I can understand what is running without opening shells on each
machine

**Acceptance Criteria:**
- Given two projects on one node each have active or recently-reported workers,
  when I open `/nodes/:nodeId/workers`, then I see all workers with project
  badges, worker state, current bead id/title, phase, model/route, elapsed time,
  and last-event freshness.
- Given the node is a federation hub with one spoke, when I set
  `scope=federation`, then I see hub and spoke workers in the same table with
  node badges and direct fallback URLs for spoke rows.
- Given a worker stops reporting, then its row transitions from `connected` to
  `stale` or `disconnected` without disappearing.
- Given I click a worker row, then a detail view shows recent phase events,
  captured output, prompt, attempt id, current bead link, and stop action.

**E2E Test:** `workers-single-pane.spec.ts` — full workflow: seed two projects
with reported workers, open node workers, verify badges and current bead fields,
open detail, verify prompt/output/events, then run a two-node fixture and verify
`scope=federation` merges hub and spoke workers with freshness badges.

### US-101: Operator Starts Server-Managed Work Capacity
**As an** operator watching a ready queue
**I want** to start more `ddx work` workers from the server UI
**So that** the server handles worker launch, tracking, and stop control

**Acceptance Criteria:**
- Given a project has ready beads, when I click `+ Add worker` and request
  count `2`, mode `watch`, and an optional label filter, then the server starts
  two autonomous workers for that project and the Workers table shows both.
- Given the project has `workers.max_count` configured, then the UI disables or
  rejects starts that would exceed the cap and shows the cap reason.
- Given a newly-started worker claims a bead, then the worker row updates with
  current bead id/title and phase without a page reload.
- Given I click Stop on a running worker, then the server requests stop and the
  row reaches a terminal or stopping state with an audit event.

**E2E Test:** `workers-dispatch.spec.ts` — full workflow: open project workers,
start two workers, verify rows and queue summary, verify cap refusal, stop one
worker, and verify terminal state plus audit event.

### US-102: Operator Controls Workers on Spoke Nodes
**As an** operator using a federation hub
**I want** worker start and stop commands for spoke projects to route to the
owning spoke
**So that** the hub is the single pane of glass but the owning node remains
authoritative

**Acceptance Criteria:**
- Given a hub and spoke are registered, when I start a worker for a spoke
  project from the hub UI, then the hub forwards the command to exactly that
  spoke and the spoke starts the worker locally.
- Given the spoke reports worker events, then the hub Workers view shows the
  new worker with spoke node badge and project badge.
- Given the spoke is offline, then the hub refuses the start/stop command with a
  clear offline reason and does not create a phantom worker row.
- Given a command is retried with the same request id, then the owning node
  returns the original result without launching a duplicate worker.

**E2E Test:** `federation-worker-control.spec.ts` — full workflow: start hub and
spoke, register projects, start a spoke worker from the hub, verify exactly one
spoke-local worker, stop it, simulate offline spoke, and verify command refusal.

### US-103: Operator Mutates Any Project's Bead Queue
**As an** operator triaging work across projects
**I want** to create, edit, approve, block, cancel, reopen, and inspect beads
from the web UI for any registered project
**So that** queue stewardship does not require shell access in each checkout

**Acceptance Criteria:**
- Given I am on a project bead page, when I create a bead with title,
  description, acceptance, priority, type, and labels, then it appears in that
  project only and no sibling project receives it.
- Given I edit title, labels, priority, description, acceptance, or status
  fields, then the mutation is persisted in the owning project and the row
  updates without cross-project leakage.
- Given I approve, block, cancel, or reopen a bead, then the lifecycle event is
  recorded and the queue summary updates.
- Given I perform the same actions from a federation hub against a spoke
  project, then the hub forwards the mutation to the owning spoke and preserves
  origin/forwarding audit metadata.

**E2E Test:** `beads-queue-mutations.spec.ts` — full workflow: create and edit
a bead in project A, verify project B isolation, run approve/block/cancel/reopen
actions, then repeat create/edit through a hub against a spoke project and
verify owner-targeted write routing.

### US-104: Operator Edits Specs for Any Project
**As an** operator aligning implementation work
**I want** to edit project specs and documents through the web UI
**So that** I can update FEAT/ADR/TP docs before launching workers

**Acceptance Criteria:**
- Given I open a `docs/helix/**` document, when I edit and save it, then the
  `documentWrite` mutation writes only inside the selected project and the
  rendered document refreshes.
- Given I attempt a path traversal or absolute-path write, then the server
  rejects it and no file is written.
- Given I edit a spoke project's spec from the hub UI, then the hub forwards the
  write to the owning spoke and preserves origin/forwarding audit metadata.
- Given the document changed after the editor loaded, then save fails with a
  conflict or stale-write message instead of silently overwriting.

**E2E Test:** `spec-editing.spec.ts` — full workflow: edit a FEAT document in
project A, verify project B isolation and graph/staleness refresh, reject path
escape, repeat through hub-to-spoke forwarding, and verify stale-write refusal.

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
| `runs.spec.ts` | US-094b | Project runs → apply layer filter → work detail → try detail → run detail tabs/evidence → artifact link → Back through full hierarchy |
| `commits.spec.ts` | US-093 | Bead with closing commit → commit log → highlighted commit → bead cross-link → Back |

## Dependencies

- FEAT-008 (web UI foundation — build pipeline, embed pattern, existing views)
- FEAT-020 (node identity and project registry — required for /api/node and /api/projects)
- SD-019 (project-scoped API routing — /api/projects/:project/...)
- FEAT-004 (beads)
- FEAT-006 (agent service metadata and native session references)
- FEAT-007 (doc graph)
- FEAT-010 (Task Execution run records and read APIs)
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
