---
title: DDx Server
weight: 4
---

`ddx server` is a lightweight Go web server that exposes your document library, bead tracker, document graph, agent session logs, and personas over HTTP and MCP endpoints — with an embedded web UI for visual management.

## Quick Start

```bash
# Start the server (reads from current project)
ddx server

# Custom port
ddx server --port 3000

# Open in browser
open http://127.0.0.1:8080
```

## Demo

<video controls autoplay muted loop width="100%" style="border-radius: 8px; border: 1px solid #e5e7eb;">
  <source src="/demos/ddx-server-ui.webm" type="video/webm">
  Your browser does not support the video tag.
</video>

## Web UI

The embedded web UI provides a browser-based interface for managing your DDx project. Six pages cover the full surface:

| Page | URL | What it does |
|------|-----|-------------|
| **Dashboard** | `/` | Project overview: document count, bead status, stale docs, server health |
| **Documents** | `/documents` | Browse, search, filter by type, view rendered markdown, edit in-place |
| **Beads** | `/beads` | Kanban board with drag-and-drop, full-text search, create/claim/close/reopen |
| **Graph** | `/graph` | Interactive document dependency graph visualization |
| **Agent** | `/agent` | Agent session history with prompt/response/token details |
| **Personas** | `/personas` | Browse personas, view by role, inspect content |

### Beads Kanban Board

The beads page is a full project tracker with:

- **Three-column kanban**: Open, In Progress, Closed
- **Drag-and-drop**: Move beads between columns to update status
- **Full-text search**: Powered by MiniSearch across all bead fields
- **Create beads**: Modal form with title, type, priority, labels, description, acceptance criteria
- **Detail panel**: Click any bead to see full details, dependencies, and execution evidence
- **Bead actions**: Claim, unclaim, close, reopen with reason

### Document Library

- **Browse by type**: Filter documents by category (prompts, personas, patterns, etc.)
- **Search**: Find documents by name
- **Rendered markdown**: View documents with full GitHub-flavored markdown rendering
- **Edit in-place**: Switch to raw editing mode, save changes directly

## HTTP API

All data is available programmatically over REST:

### Documents & Search

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/documents` | List all library documents |
| `GET` | `/api/documents/:path` | Read document content |
| `PUT` | `/api/documents/:path` | Write document (auto-commits) |
| `GET` | `/api/search?q=<query>` | Full-text search across documents |
| `GET` | `/api/personas` | List all personas |
| `GET` | `/api/personas/:role` | Resolve persona for a role |

### Bead Tracker

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/beads` | List all beads |
| `GET` | `/api/beads/:id` | Show a specific bead |
| `GET` | `/api/beads/ready` | List ready beads (no open deps) |
| `GET` | `/api/beads/blocked` | List blocked beads |
| `GET` | `/api/beads/status` | Summary counts by status |
| `GET` | `/api/beads/dep/tree/:id` | Dependency tree |
| `POST` | `/api/beads` | Create a bead |
| `PUT` | `/api/beads/:id` | Update bead fields |
| `POST` | `/api/beads/:id/claim` | Claim a bead |
| `POST` | `/api/beads/:id/unclaim` | Release a claim |
| `POST` | `/api/beads/:id/reopen` | Reopen with reason |
| `POST` | `/api/beads/:id/deps` | Add/remove dependencies |

### Document Graph

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/docs/graph` | Full dependency graph |
| `GET` | `/api/docs/stale` | Stale documents |
| `GET` | `/api/docs/:id` | Document metadata and staleness |
| `GET` | `/api/docs/:id/deps` | Upstream dependencies |
| `GET` | `/api/docs/:id/dependents` | Downstream dependents |
| `GET` | `/api/docs/:id/history` | Document commit history |

### Execution & Agent Sessions

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/exec/runs` | List execution runs |
| `GET` | `/api/exec/runs/:id` | Show execution run detail |
| `GET` | `/api/exec/runs/:id/log` | Execution run logs |
| `POST` | `/api/exec/run/:id` | Dispatch an execution |
| `GET` | `/api/agent/sessions` | List agent sessions |
| `GET` | `/api/agent/sessions/:id` | Session detail |

### System

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Liveness check |
| `GET` | `/api/ready` | Readiness check |

## MCP Endpoints

AI agents can connect to `ddx server` via MCP (Streamable HTTP at `POST /mcp`). Available tools:

**Documents:** `ddx_list_documents`, `ddx_read_document`, `ddx_search`, `ddx_resolve_persona`, `ddx_doc_write`, `ddx_doc_history`, `ddx_doc_diff`, `ddx_doc_changed`

**Beads:** `ddx_list_beads`, `ddx_show_bead`, `ddx_bead_ready`, `ddx_bead_status`, `ddx_bead_create`, `ddx_bead_update`, `ddx_bead_claim`

**Graph:** `ddx_doc_graph`, `ddx_doc_stale`, `ddx_doc_show`, `ddx_doc_deps`

**Execution:** `ddx_exec_definitions`, `ddx_exec_show`, `ddx_exec_history`, `ddx_exec_dispatch`

**Agent:** `ddx_agent_sessions`, `ddx_agent_dispatch`

## Architecture

- **Single binary** — server and web UI are embedded in the `ddx` CLI
- **Stateless** — reads from filesystem on each request, no database
- **Localhost by default** — binds to `127.0.0.1` for security
- **File-backed** — all data comes from git-tracked files (`.ddx/beads.jsonl`, library docs, agent logs)
