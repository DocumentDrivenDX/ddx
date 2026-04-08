---
ddx:
  id: FEAT-008
  depends_on:
    - helix.prd
    - FEAT-002
---
# Feature: DDx Server Web UI

**ID:** FEAT-008
**Status:** Complete
**Priority:** P1
**Owner:** DDx Team

## Overview

The DDx server (`ddx-server`) serves a web UI for browsing documents, beads, the document dependency graph, and agent session logs. The UI is a TypeScript SPA built with Vite, embedded into the Go binary via `embed.FS`, and served alongside the MCP and HTTP API endpoints from a single process.

## Problem Statement

**Current situation:** DDx documents, beads, and the doc graph are only accessible via CLI commands or MCP tool calls. There's no visual way to browse the document library, see the dependency graph, check bead status, or review agent session logs.

**Pain points:**
- CLI output for `ddx doc graph` is text-only — dependency graphs need visual rendering
- Bead status and dependency trees are hard to grok in terminal output
- Agent session logs have no browsable interface
- New team members can't quickly understand what documents exist or how they relate

**Desired outcome:** A web dashboard served from the same `ddx-server` binary that already handles MCP and HTTP API. Developers open a browser, see their document library, dependency graph, bead board, and agent activity — all from one URL.

## Architecture

### Single Binary, Multiple Surfaces

```
ddx-server binary
├── /            → Web UI (embedded SPA)
├── /api/        → HTTP REST API (JSON)
└── /mcp/        → MCP endpoints (SSE transport)
```

All three surfaces share the same underlying services (document library, bead store, doc graph, agent logs). The web UI calls the HTTP API.

### Build Pipeline

```
frontend/           → Vite + TypeScript + React
  npm run build     → frontend/dist/
server/             → Go with embed.FS
  go build          → ddx-server (single binary with embedded UI)
```

During development, Vite's dev server proxies `/api/` to the running Go server.

### Technology Choices

| Layer | Choice | Why |
|-------|--------|-----|
| Frontend framework | **React** | Decided in ADR-002 (Accepted 2026-04-04) |
| Build tool | **Vite** | Industry standard, fast, good Go embed integration |
| Styling | **Tailwind CSS** | Consistent with website (Hextra uses Tailwind) |
| Data fetching | **TanStack Query** | Caching, refetching, works with React |
| Graph visualization | **D3.js** or **Cytoscape.js** | For document dependency graph rendering |
| Embedding | **Go embed.FS** | Compile frontend into the binary — no external files needed |
| Go HTTP | **Chi** or **net/http** | Standard library compatible routing |
| Go MCP | **mcp-go** | Leading Go MCP SDK, supports Streamable HTTP transport |

### Alternatives Considered

**Templ + htmx** — simpler (no JS toolchain), but the dependency graph visualization and interactive bead board need more interactivity than htmx provides well. Templ + htmx is great for CRUD; we need a richer graph/dashboard experience.

**Local-first with CRDT sync** — interesting for offline use but overkill for v1. The UI is primarily read-heavy (browsing docs, viewing graphs). Can be added later with PowerSync or ElectricSQL if needed.

## Requirements

### Functional

1. **Artifact browser, viewer, and editor**
   - List documents by category (prompts, personas, patterns, templates, ADRs, SDs)
   - Full-text search across document content and frontmatter
   - View document content rendered as markdown with syntax highlighting
   - Show `ddx:` frontmatter metadata (id, dependencies, staleness status)
   - Clickable dependency links — navigate upstream/downstream docs inline
   - **Editor**: in-browser markdown editing with save-to-disk (calls API to write file)
   - Sidebar showing document dependency tree for current doc
   - Staleness indicator per document (fresh/stale/missing-deps)

2. **Document dependency graph**
   - Interactive visual graph (D3/Cytoscape) showing all document relationships
   - Color-code nodes by staleness (green=fresh, orange=stale, red=missing-deps)
   - Click node to navigate to document detail
   - Filter by document type, staleness status
   - Zoom/pan for large graphs
   - Layout options (hierarchical top-down, force-directed)

3. **Bead views (local-first, client-side data)**

   The beads UI runs entirely in the browser. On load, the client fetches the
   full bead set from `GET /api/beads` (JSONL is small — hundreds of beads,
   not millions). All search, filter, sort, and graph traversal happens
   client-side using an in-browser data layer (see ADR-005).

   **Master-detail list view:**
   - Searchable list: full-text across title, description, acceptance, labels
   - Sortable columns: ID, title, status, priority, labels, owner, updated_at
   - Filterable by status, priority, label, owner
   - Click a bead to open detail panel (split pane or slide-over)
   - Detail shows: all fields, dependency tree, execution beads, agent sessions
   - Graph traversal: click a dependency → navigate to that bead's detail
   - Drill-down into execution runs linked to the bead (shows pass/fail,
     logs, duration)

   **Kanban board view:**
   - Columns represent status: `open`, `in_progress`, `closed`
   - Cards within columns ordered by priority (P0 top, P4 bottom)
   - Dependency grouping: beads that block each other cluster visually
     (similar to Trello, Fuzz by 37signals, GitHub Projects)
   - Drag-and-drop to change status (calls `ddx bead update` API)
   - Swimlanes option: group rows by label (e.g., `area:cli`, `area:web`)
   - Color coding: priority → card border, labels → chips, blocked → dimmed
   - Collapsed card shows: title, priority badge, label chips, dep count
   - Expanded card shows: description, acceptance, execution summary

   **Ready queue view:**
   - Filtered view showing only beads with all dependencies satisfied
   - Sorted by priority, then by creation date
   - One-click claim (calls `ddx bead claim` API)

   **Bead detail actions:**

   The detail panel is not just a viewer — it is the primary interaction
   surface for evaluating and driving work items.

   *Navigation:*
   - `spec-id` links to the governing artifact (rendered inline or navigates
     to the document viewer with that artifact's content)
   - Parent bead link navigates to the parent's detail
   - Dependency links navigate to each dep's detail
   - Execution runs link to the execution detail view (logs, results)
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
   - Re-run: dispatch `ddx exec run` or `ddx agent run` against the bead's
     linked execution definition (calls server API which delegates to CLI)
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
   - Click to expand: full prompt text and agent response
   - Filter by harness, time range, exit code
   - Token usage summary (total tokens by harness, by day)
   - Auto-refresh on configurable interval (or WebSocket push in v2)

5. **Status dashboard**
   - Summary cards: document count by type, bead counts by status, stale document count, recent agent activity
   - Quick links to ready beads, stale documents, recent agent sessions
   - Project health indicators (library populated, config valid, beads active)

6. **Persona viewer**
   - Browse personas with descriptions and tags
   - See which roles each persona is bound to in project config
   - View full persona content rendered as markdown

### Non-Functional

- **Performance:** Initial page load <2s. API calls <200ms. Graph renders smoothly for 100+ nodes.
- **Deployment:** Single binary. No external runtime dependencies. No separate frontend server in production.
- **Development:** `npm run dev` proxies to Go server. Hot reload for frontend. `go run` for backend.
- **Responsive:** Usable on tablet/laptop screens. Not optimized for mobile.

## User Stories

### US-080: Developer Browses Document Library
**As a** developer onboarding to a project
**I want** to open a web UI and browse all available documents
**So that** I can quickly understand what documents exist and how they're organized

**Acceptance Criteria:**
- Given ddx-server is running, when I open `http://localhost:PORT`, then I see a dashboard with document counts by category
- Given I click on "Personas", then I see a list of all persona documents with names and descriptions
- Given I click on a persona, then I see its full content rendered as markdown

### US-081: Developer Views Dependency Graph
**As a** developer checking document health
**I want** to see a visual graph of document dependencies
**So that** I can identify which documents are stale and what depends on what

**Acceptance Criteria:**
- Given documents have `ddx:` frontmatter with dependencies, when I open the graph view, then I see nodes and edges representing the dependency relationships
- Given some documents are stale, then stale nodes are visually highlighted (red/orange)
- Given I click a node, then I navigate to that document's detail view

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
- Given I click a card, then a detail panel opens showing description,
  acceptance criteria, execution runs, and the dependency graph

### US-082b: Developer Searches and Traverses Beads
**As a** developer investigating a work item
**I want** to search beads and navigate their dependency graph
**So that** I can trace blocking relationships and inspect execution evidence

**Acceptance Criteria:**
- Given I type in the search bar, then results filter instantly (client-side,
  no server round-trip) across title, description, acceptance, and labels
- Given I'm viewing a bead's detail, when I click a dependency link, then I
  navigate to that bead's detail
- Given a bead has linked execution runs, then I see their pass/fail status,
  duration, and can expand to see logs

### US-082c: Developer Views Bead Execution Evidence
**As a** developer evaluating whether a bead is truly done
**I want** to drill into the execution beads and agent sessions associated
  with a work item
**So that** I can verify tests passed and the implementation matches acceptance

**Acceptance Criteria:**
- Given a bead has execution runs, when I open its detail, then I see a list
  of runs with status, harness, duration, and timestamp
- Given I click a run, then I see structured results and raw log output
- Given a bead has agent sessions, then I see prompt/response summaries

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
  recorded

### US-082e: Supervisor Re-runs Execution from Bead Detail
**As a** supervisor who wants fresh evidence
**I want** to re-run an execution or agent invocation directly from the bead
  detail
**So that** I can verify the implementation still works after spec changes

**Acceptance Criteria:**
- Given a bead has a linked execution definition, when I click "Re-run", then
  the execution is dispatched via the server API
- Given the run completes, then the result appears in the bead's execution
  list and the UI refreshes
- Given I want to run an agent review, when I click "Agent review", then
  an agent session is dispatched with the bead's context as the prompt

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

### US-083: Developer Edits Document in Browser
**As a** developer fixing a stale document
**I want** to edit its markdown content in the browser and save
**So that** I don't need to switch to my editor for quick fixes

**Acceptance Criteria:**
- Given I'm viewing a document, when I click "Edit", then I see a markdown editor with the current content
- Given I make changes and click "Save", then the file is written to disk via the API
- Given the save succeeds, then the rendered view updates and staleness is rechecked

### US-084: Developer Searches Across All Documents
**As a** developer looking for how something is documented
**I want** to search across all document content
**So that** I can find relevant docs without knowing their location

**Acceptance Criteria:**
- Given I type in the search bar, then results appear from all document types
- Given results are shown, then I see matching snippets with the search term highlighted
- Given I click a result, then I navigate to that document's detail view

### US-085: Developer Creates and Manages Beads in the UI
**As a** developer triaging work
**I want** to create beads, set their status, and manage dependencies in the browser
**So that** I can manage the work queue visually

**Acceptance Criteria:**
- Given I click "New Bead", then I see a form with title, type, priority, labels, description, acceptance
- Given I submit the form, then a bead is created via the API and appears in the list
- Given I click a bead's status, then I can transition it (open → in_progress → closed)
- Given I'm viewing a bead, then I can add/remove dependencies by selecting other beads

### US-086: Developer Monitors Agent Activity in Real Time
**As a** developer running agents against my project
**I want** to see agent invocations as they happen with full prompt/response details
**So that** I can diagnose issues and track token usage

**Acceptance Criteria:**
- Given agents have been invoked, when I open the agent log view, then I see recent sessions sorted by time
- Given I click a session, then I see the full prompt that was sent and the agent's response
- Given I filter by harness, then only sessions for that harness are shown
- Given I look at the summary, then I see total tokens consumed by harness and by day

## Implementation Notes

### Embedding Pattern (Pocketbase-style)

```go
//go:embed all:frontend/dist
var frontendFiles embed.FS

func main() {
    distFS, _ := fs.Sub(frontendFiles, "frontend/dist")
    
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
cd server && go run . --library-path ../.ddx/library

# Terminal 2: Vite dev server with proxy
cd frontend && npm run dev
# vite.config.ts proxies /api/ and /mcp/ to localhost:8080
```

### Project Structure

```
ddx/
├── server/
│   ├── main.go            # Server entry point
│   ├── api/               # HTTP API handlers
│   ├── mcp/               # MCP endpoint handlers
│   └── embed.go           # embed.FS declaration
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── App.tsx
│   │   ├── pages/
│   │   │   ├── Dashboard.tsx
│   │   │   ├── Documents.tsx
│   │   │   ├── Graph.tsx
│   │   │   ├── Beads.tsx
│   │   │   └── Agent.tsx
│   │   └── components/
│   └── dist/              # Build output (embedded into Go)
```

## Dependencies

- FEAT-002 (DDx server HTTP/MCP API) — the web UI consumes the same API
- FEAT-004 (Beads) — for bead board
- FEAT-007 (Doc graph) — for dependency graph visualization
- FEAT-006 (Agent service) — for session log viewer
- Vite, TypeScript, React, TanStack Query, D3.js or Cytoscape.js
- Go embed.FS, Chi or net/http, mcp-go

## Out of Scope

- Real-time sync / collaborative editing
- Mobile-optimized layout
- Authentication (localhost-only for v1; auth deferred to FEAT-002 server security)
- Server-side search or pagination for beads (client-side only — see ADR-005)
