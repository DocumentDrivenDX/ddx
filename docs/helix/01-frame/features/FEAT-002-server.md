---
ddx:
  id: FEAT-002
  depends_on:
    - helix.prd
    - FEAT-004
    - FEAT-010
    - FEAT-007
    - FEAT-012
---
# Feature: DDx Server

**ID:** FEAT-002
**Status:** Complete
**Priority:** P0
**Owner:** DDx Team

## Overview

`ddx-server` is a lightweight Go web server that exposes DDx platform services over HTTP and MCP endpoints. It serves documents, beads, execution definitions and run history, the document dependency graph, DDx agent invocation activity plus embedded-agent telemetry references, and (via FEAT-008) an embedded web UI — all from a single binary. In the multi-project topology, one binary can serve several project roots on one machine with explicit project scoping.

## Architecture

```
ddx-server binary
├── /projects  → Project registry / project picker
├── /            → Web UI (embedded SPA, FEAT-008)
├── /api/        → HTTP REST API (JSON)
└── /mcp/        → MCP tool endpoints (Streamable HTTP transport)
```

All three surfaces share the same underlying services. The web UI calls the HTTP API. MCP tools call the same service layer.

### Project Registry and Routing

`ddx-server` resolves every request against an explicit project context
before dispatching to feature-specific adapters.

**Registry contract**

- `server.projects` declares the local project roots served by one server process
- each entry carries a stable `id`, an absolute `root`, an optional display `name`, and an optional `default` marker
- if no registry is configured, the server synthesizes a singleton project from the current working directory so today's `ddx server` invocation still works

**Canonical project-scoped surfaces**

- `GET /api/projects` - list configured project roots and health
- `GET /api/projects/:project` - show one project context
- `ddx_list_projects` - enumerate projects over MCP
- `ddx_show_project` - show one project context over MCP

**Selection rules**

1. explicit project in the request path
2. explicit project supplied by the caller
3. configured default project
4. implicit singleton project when only one project exists

Legacy unscoped `/api/...` and `/mcp/...` forms remain only as compatibility aliases when the server can resolve exactly one project context. They are not the canonical multi-project contract.

## Requirements

### Functional

Unless otherwise noted, the canonical resource routes below are project-scoped
at `/api/projects/:project/...`, and project-aware MCP tools resolve against an
explicit or selected project context. Legacy unscoped `/api/...` and `/mcp/...`
forms remain only as singleton compatibility aliases when the server can
resolve exactly one project context.

**Document Library**
1. `GET /api/projects/:project/documents` — list documents by category
2. `GET /api/projects/:project/documents/:path` — read document content
3. `GET /api/projects/:project/search?q=<query>` — full-text search across document contents
4. `GET /api/projects/:project/personas/:role` — resolve persona for role from project config
5. MCP tools: `ddx_list_documents`, `ddx_read_document`, `ddx_search`, `ddx_resolve_persona` (project selector required unless singleton compatibility mode applies)

**Bead Tracker (FEAT-004)**
6. `GET /api/projects/:project/beads` — list beads with optional status/label filters
7. `GET /api/projects/:project/beads/:id` — show one bead with all fields
8. `GET /api/projects/:project/beads/ready` — list ready beads (no unclosed deps)
9. `GET /api/projects/:project/beads/blocked` — list blocked beads
10. `GET /api/projects/:project/beads/status` — summary counts
11. `GET /api/projects/:project/beads/dep/tree/:id` — dependency tree for a bead
12. MCP tools: `ddx_list_beads`, `ddx_show_bead`, `ddx_bead_ready`, `ddx_bead_status` (project selector required unless singleton compatibility mode applies)

**Document Graph (FEAT-007)**
13. `GET /api/projects/:project/docs/graph` — full dependency graph as JSON
14. `GET /api/projects/:project/docs/stale` — list stale documents
15. `GET /api/projects/:project/docs/:id` — document metadata and staleness status
16. `GET /api/projects/:project/docs/:id/deps` — upstream dependencies
17. `GET /api/projects/:project/docs/:id/dependents` — downstream dependents
18. MCP tools: `ddx_doc_graph`, `ddx_doc_stale`, `ddx_doc_show`, `ddx_doc_deps` (project selector required unless singleton compatibility mode applies)

**Agent Activity (FEAT-006)**
19. `GET /api/projects/:project/agent/sessions` — list recent DDx agent invocations
20. `GET /api/projects/:project/agent/sessions/:id` — invocation detail, including native
    session/trace references and any DDx-owned transcript data
21. MCP tool: `ddx_agent_sessions` (project selector required unless singleton compatibility mode applies)

**Executions (FEAT-010)**
22. `GET /api/projects/:project/exec/definitions` — list execution definitions with optional artifact filter
23. `GET /api/projects/:project/exec/definitions/:id` — show one execution definition
24. `GET /api/projects/:project/exec/runs` — list execution runs with optional artifact/definition/status filters
25. `GET /api/projects/:project/exec/runs/:id` — show one execution run with structured result metadata
26. `GET /api/projects/:project/exec/runs/:id/log` — show raw captured logs for one execution run
27. MCP tools: `ddx_exec_definitions`, `ddx_exec_show`, `ddx_exec_history` (project selector required unless singleton compatibility mode applies)

**Configuration**
28. Library path, project registry, port, optional ts-net hostname via CLI flags or config file (see ADR-006 and SD-019)
29. Default: localhost only, no auth required

### Non-Functional

- **Performance:** Document reads <200ms, search <500ms, graph build <500ms for 100+ documents
- **Stateless:** Reads from filesystem on each request. No database.
- **Single binary:** Embeds web UI (FEAT-008) via `embed.FS`
- **Security:** Localhost-only by default. Optional ts-net (Tailscale) listener for non-local access (ADR-006).

**Bead Mutations (FEAT-008 UI interaction)**
33. `POST /api/projects/:project/beads` — create a bead
34. `PUT /api/projects/:project/beads/:id` — update bead fields (status, labels, description, etc.)
35. `POST /api/projects/:project/beads/:id/claim` — claim a bead for the current session
36. `POST /api/projects/:project/beads/:id/unclaim` — release a claim
37. `POST /api/projects/:project/beads/:id/reopen` — re-open a closed bead with a reason
38. `POST /api/projects/:project/beads/:id/deps` — add/remove dependencies
39. MCP tools: `ddx_bead_create`, `ddx_bead_update`, `ddx_bead_claim` (project selector required unless singleton compatibility mode applies)

**Execution Dispatch (UI-initiated, localhost-only)**
40. `POST /api/projects/:project/exec/run/:id` — dispatch an execution run (delegates to
    `ddx exec run` internally). Localhost-only or via ts-net (ADR-006) for
    non-local access.
41. `POST /api/projects/:project/agent/run` — dispatch an agent invocation with harness,
    model, effort, and prompt. Localhost-only or via ts-net (ADR-006) for non-local access.
42. MCP tools: `ddx_exec_dispatch`, `ddx_agent_dispatch` (project selector required unless singleton compatibility mode applies; localhost-only)

## Technology

| Component | Choice | Reference |
|-----------|--------|-----------|
| HTTP routing | Chi or net/http | ADR-001 |
| MCP transport | mcp-go (Streamable HTTP) | |
| Embedded web UI | Go embed.FS | ADR-002 |
| Frontend | Vite + React + Tailwind | ADR-002, FEAT-008 |

## Dependencies

- FEAT-004 (Beads) — bead endpoints read from bead store
- FEAT-010 (Executions) — execution endpoints read definitions and immutable run history
- FEAT-007 (Doc Graph) — graph/stale endpoints use doc graph engine
- FEAT-006 (Agent Service) — agent activity endpoints read DDx invocation
  metadata and embedded telemetry references
- FEAT-008 (Web UI) — embedded SPA served at `/`
- mcp-go SDK for MCP transport

**Document Write + Commit (FEAT-012)**
43. `PUT /api/docs/:id` — write document content and auto-commit
44. MCP tool: `ddx_doc_write` — write document by artifact ID, commit by
    default
45. `GET /api/docs/:id/history` — document commit history
46. `GET /api/docs/:id/diff` — document content diff between refs
47. MCP tools: `ddx_doc_history`, `ddx_doc_diff`, `ddx_doc_changed`

Write endpoints commit by default (configurable via `git.auto_commit` in
`.ddx/config.yaml`). Commit messages follow the structured format defined
in FEAT-012.

## Out of Scope

- Agent/execution invocation from non-localhost without ts-net (security
  boundary — dispatch endpoints are localhost-only or ts-net only, per ADR-006)
- User authentication beyond ts-net identity (no custom auth middleware)
- Multi-library aggregation
- Hosting as a cloud service
- Branch management or merge conflict resolution via API
