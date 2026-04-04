---
ddx:
  id: FEAT-002
  depends_on:
    - helix.prd
---
# Feature: DDx Server

**ID:** FEAT-002
**Status:** Not Started
**Priority:** P0
**Owner:** DDx Team

## Overview

`ddx-server` is a lightweight Go web server that exposes DDx document libraries over HTTP and MCP endpoints. It lets AI agents programmatically browse, search, and read documents without relying on filesystem access.

## Problem Statement

**Current situation:** Agents can only access documents that are on the local filesystem and explicitly provided in their context. There's no way for an agent to discover what documents are available or fetch them on demand.

**Pain points:**
- Agents can't browse a document library — they only see what humans copy into their context
- No MCP interface for document access — agents that support MCP tools can't query DDx libraries
- Remote agents (cloud-hosted, CI environments) have no access to local document libraries
- No search across document contents — finding the right document requires human curation

**Desired outcome:** Agents can discover, search, and read DDx documents through standard MCP tool calls or HTTP requests, enabling self-directed context assembly.

## Requirements

### Functional

1. **Library browsing** — list document categories, list documents within a category, get document metadata
2. **Document reading** — fetch full content of any document by path
3. **Search** — full-text search across document contents, filterable by category
4. **Persona resolution** — given a role name and project config, return the bound persona document
5. **MCP tool endpoints** — expose all above as MCP tools that agents can call
6. **HTTP API** — REST endpoints for the same operations (for web UI, scripts, non-MCP consumers)
7. **Configuration** — specify library path, port, and optional auth via CLI flags or config file
8. **Serve from local library** — reads directly from `.ddx/library/` on disk, no database

### Non-Functional

- **Performance:** <200ms response time for document reads, <500ms for search
- **Stateless:** No database, no persistent state. Reads filesystem on each request.
- **Lightweight:** Single binary, minimal memory footprint, suitable for running alongside development tools
- **Security:** Optional API key auth for non-local access. Default: localhost only.

## User Stories

### US-010: Agent Browses Documents via MCP
**As an** AI agent with MCP tool access
**I want** to list available documents and read their contents
**So that** I can self-assemble the context I need for a task

**Acceptance Criteria:**
- Given ddx-server is running, when an agent calls the `ddx_list_documents` MCP tool, then it receives a list of documents with types and descriptions
- Given an agent calls `ddx_read_document` with a path, then it receives the full document content
- Given an agent calls `ddx_search` with a query, then it receives matching documents ranked by relevance

### US-011: Developer Starts Server Locally
**As a** developer using agents with MCP support
**I want** to start ddx-server with a single command
**So that** my agents can access my document library

**Acceptance Criteria:**
- Given I'm in a DDx project, when I run `ddx-server`, then it starts serving on localhost:PORT
- Given I specify `--library-path /path/to/library`, then it serves that library
- Given I run `ddx-server --port 8080`, then it listens on port 8080

### US-012: Resolve Persona for Role
**As an** AI agent assigned to a role
**I want** to fetch the persona document bound to my role
**So that** I know how to behave for this project

**Acceptance Criteria:**
- Given a project has `persona_bindings: { code-reviewer: strict-code-reviewer }` in config, when an agent calls `ddx_resolve_persona` with role "code-reviewer", then it receives the full strict-code-reviewer persona document

### US-013: HTTP API for Web Consumers
**As a** developer building a document browser UI
**I want** to query the document library over HTTP
**So that** I can build web interfaces on top of DDx

**Acceptance Criteria:**
- Given ddx-server is running, when I GET `/api/documents`, then I receive a JSON list of all documents
- Given I GET `/api/documents/personas/strict-code-reviewer`, then I receive the document content
- Given I GET `/api/search?q=error+handling`, then I receive matching results

## Edge Cases

- Library path doesn't exist — return clear error on startup
- Document requested that doesn't exist — 404 with helpful message
- Empty library — return empty results, not errors
- Very large documents — stream response, don't buffer entire file
- Concurrent requests — safe for concurrent reads (filesystem is the source of truth)

## Dependencies

- Go standard library (net/http)
- MCP SDK or protocol implementation
- DDx document library on disk

## Out of Scope

- Document editing/writing through the server (read-only)
- User authentication beyond API keys (no OAuth, no user accounts)
- Document change notifications (P2 — future WebSocket feature)
- Multi-library aggregation (P2)
- Hosting as a cloud service
