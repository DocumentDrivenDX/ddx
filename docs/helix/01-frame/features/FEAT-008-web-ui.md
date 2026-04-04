---
ddx:
  id: FEAT-008
  depends_on:
    - helix.prd
    - FEAT-002
---
# Feature: DDx Server Web UI

**ID:** FEAT-008
**Status:** Not Started
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
frontend/           → Vite + TypeScript + React/Solid
  npm run build     → frontend/dist/
server/             → Go with embed.FS
  go build          → ddx-server (single binary with embedded UI)
```

During development, Vite's dev server proxies `/api/` to the running Go server.

### Technology Choices

| Layer | Choice | Why |
|-------|--------|-----|
| Frontend framework | **Solid** or **React** | Solid is smaller/faster; React has larger ecosystem. Decide at implementation. |
| Build tool | **Vite** | Industry standard, fast, good Go embed integration |
| Styling | **Tailwind CSS** | Consistent with website (Hextra uses Tailwind) |
| Data fetching | **TanStack Query** | Caching, refetching, works with both React and Solid |
| Graph visualization | **D3.js** or **Cytoscape.js** | For document dependency graph rendering |
| Embedding | **Go embed.FS** | Compile frontend into the binary — no external files needed |
| Go HTTP | **Chi** or **net/http** | Standard library compatible routing |
| Go MCP | **mcp-go** | Leading Go MCP SDK, supports Streamable HTTP transport |

### Alternatives Considered

**Templ + htmx** — simpler (no JS toolchain), but the dependency graph visualization and interactive bead board need more interactivity than htmx provides well. Templ + htmx is great for CRUD; we need a richer graph/dashboard experience.

**Local-first with CRDT sync** — interesting for offline use but overkill for v1. The UI is primarily read-heavy (browsing docs, viewing graphs). Can be added later with PowerSync or ElectricSQL if needed.

## Requirements

### Functional

1. **Document browser** — list documents by category (prompts, personas, patterns, templates), view document content rendered as markdown, show `ddx:` frontmatter metadata
2. **Document dependency graph** — interactive visual graph showing document relationships, highlight stale nodes, click to navigate to document detail
3. **Bead board** — list beads with status, priority, labels; show dependency tree; filter by status/label; visual ready queue
4. **Agent session log** — browse recent agent invocations with harness, tokens, duration; drill into session detail (prompt + response)
5. **Status dashboard** — summary counts (documents by type, beads by status, stale documents, recent agent activity)
6. **Persona viewer** — browse personas, see which roles they're bound to in the project config

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

### US-082: Developer Monitors Bead Status
**As a** developer tracking work items
**I want** to see beads in a visual board with filtering
**So that** I can quickly understand what's ready, blocked, and in progress

**Acceptance Criteria:**
- Given beads exist, when I open the bead view, then I see beads grouped by status (open, in_progress, closed)
- Given I filter by label, then only matching beads are shown
- Given I click a bead, then I see its full details including dependencies and acceptance criteria

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
- Vite, TypeScript, React or Solid, TanStack Query, D3.js or Cytoscape.js
- Go embed.FS, Chi or net/http, mcp-go

## Out of Scope

- Document editing in the browser (use your editor, view in the web UI)
- Bead creation/mutation from the web UI (CLI-only for now; read-only dashboard)
- Real-time sync / collaborative editing
- Mobile-optimized layout
- Authentication (localhost-only for v1; auth deferred to FEAT-002 server security)
- Offline/local-first capabilities (v2 consideration)
