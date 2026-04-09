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

`ddx-server` is a lightweight Go web server that exposes DDx platform services over HTTP and MCP endpoints. It serves documents, beads, execution definitions and run history, the document dependency graph, agent session logs, and (via FEAT-008) an embedded web UI — all from a single binary.

## Architecture

```
ddx-server binary
├── /            → Web UI (embedded SPA, FEAT-008)
├── /api/        → HTTP REST API (JSON)
└── /mcp/        → MCP tool endpoints (Streamable HTTP transport)
```

All three surfaces share the same underlying services. The web UI calls the HTTP API. MCP tools call the same service layer.

## Requirements

### Functional

**Document Library**
1. `GET /api/documents` — list documents by category
2. `GET /api/documents/:path` — read document content
3. `GET /api/search?q=<query>` — full-text search across document contents
4. `GET /api/personas/:role` — resolve persona for role from project config
5. MCP tools: `ddx_list_documents`, `ddx_read_document`, `ddx_search`, `ddx_resolve_persona`

**Bead Tracker (FEAT-004)**
6. `GET /api/beads` — list beads with optional status/label filters
7. `GET /api/beads/:id` — show one bead with all fields
8. `GET /api/beads/ready` — list ready beads (no unclosed deps)
9. `GET /api/beads/blocked` — list blocked beads
10. `GET /api/beads/status` — summary counts
11. `GET /api/beads/dep/tree/:id` — dependency tree for a bead
12. MCP tools: `ddx_list_beads`, `ddx_show_bead`, `ddx_bead_ready`, `ddx_bead_status`

**Document Graph (FEAT-007)**
13. `GET /api/docs/graph` — full dependency graph as JSON
14. `GET /api/docs/stale` — list stale documents
15. `GET /api/docs/:id` — document metadata and staleness status
16. `GET /api/docs/:id/deps` — upstream dependencies
17. `GET /api/docs/:id/dependents` — downstream dependents
18. MCP tools: `ddx_doc_graph`, `ddx_doc_stale`, `ddx_doc_show`, `ddx_doc_deps`

**Agent Session Logs (FEAT-006)**
19. `GET /api/agent/sessions` — list recent agent sessions
20. `GET /api/agent/sessions/:id` — full session detail (prompt, response, tokens)
21. MCP tool: `ddx_agent_sessions`

**Executions (FEAT-010)**
22. `GET /api/exec/definitions` — list execution definitions with optional artifact filter
23. `GET /api/exec/definitions/:id` — show one execution definition
24. `GET /api/exec/runs` — list execution runs with optional artifact/definition/status filters
25. `GET /api/exec/runs/:id` — show one execution run with structured result metadata
26. `GET /api/exec/runs/:id/log` — show raw captured logs for one execution run
27. MCP tools: `ddx_exec_definitions`, `ddx_exec_show`, `ddx_exec_history`

**Configuration**
28. Library path, port, optional ts-net hostname via CLI flags or config file (see ADR-006)
29. Default: localhost only, no auth required

### Non-Functional

- **Performance:** Document reads <200ms, search <500ms, graph build <500ms for 100+ documents
- **Stateless:** Reads from filesystem on each request. No database.
- **Single binary:** Embeds web UI (FEAT-008) via `embed.FS`
- **Security:** Localhost-only by default. Optional ts-net (Tailscale) listener for non-local access (ADR-006).

**Bead Mutations (FEAT-008 UI interaction)**
33. `POST /api/beads` — create a bead
34. `PUT /api/beads/:id` — update bead fields (status, labels, description, etc.)
35. `POST /api/beads/:id/claim` — claim a bead for the current session
36. `POST /api/beads/:id/unclaim` — release a claim
37. `POST /api/beads/:id/reopen` — re-open a closed bead with a reason
38. `POST /api/beads/:id/deps` — add/remove dependencies
39. MCP tools: `ddx_bead_create`, `ddx_bead_update`, `ddx_bead_claim`

**Execution Dispatch (UI-initiated, localhost-only)**
40. `POST /api/exec/run/:id` — dispatch an execution run (delegates to
    `ddx exec run` internally). Localhost-only or via ts-net (ADR-006) for
    non-local access.
41. `POST /api/agent/run` — dispatch an agent invocation with harness,
    model, effort, and prompt. Localhost-only or via ts-net (ADR-006) for non-local access.
42. MCP tools: `ddx_exec_dispatch`, `ddx_agent_dispatch` (localhost-only)

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
- FEAT-006 (Agent Service) — session log endpoints read agent logs
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
