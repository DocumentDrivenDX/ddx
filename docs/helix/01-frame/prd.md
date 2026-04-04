# Product Requirements Document: DDx

**Version:** 3.1.0
**Date:** 2026-04-03
**Status:** Active

## Summary

DDx is a monorepo producing three artifacts that together form the shared infrastructure for document-driven development:

1. **`ddx` CLI** — local document library management, persona composition, template application, git sync
2. **`ddx-server`** — web server + MCP endpoints for browsing and serving documents to agents over the network
3. **`ddx.github.io`** — promotional website explaining DDx to developers and linking to docs

DDx is the foundation layer. Workflow tools (HELIX, others) build on top. AI agents sit below, consuming the documents DDx manages.

## Problem

AI agents produce code from documents. But there's no tooling for the documents themselves:

- **No structure**: Agent-facing documents (prompts, personas, patterns, specs) accumulate as ad-hoc files with no conventions
- **No composition**: Assembling the right combination of persona + pattern + spec into agent context is manual and error-prone
- **No reuse**: Every project reinvents its agent instructions from scratch; proven patterns stay trapped in individual repos
- **No network access**: Agents can only read documents from the local filesystem; there's no way to serve documents over MCP or HTTP
- **No discoverability**: Developers can't easily browse what documents are available or what they do

## Goals

### Primary
1. **Manage document libraries** — provide structure, conventions, and CLI tooling so document libraries stay organized
2. **Enable document composition** — combine personas, patterns, specs, and templates into coherent agent context
3. **Serve documents to agents** — expose document libraries via MCP endpoints and HTTP so agents can discover and consume them programmatically
4. **Support cross-project reuse** — share document libraries between projects and teams via git subtree

### Secondary
1. **Promote the practice** — website explains document-driven development and drives adoption
2. **Keep documents honest** — detect drift between documents and implementation

### Success Metrics

| Metric | Target |
|--------|--------|
| Time to assemble agent context | <30 seconds |
| Document reuse rate across projects | >40% |
| MCP endpoint response time | <200ms |
| Website explains DDx clearly to new visitor | <2 minutes to understand |

### Non-Goals

- A workflow methodology (that's HELIX and others, not DDx)
- Workflow-specific bead validation (phase labels, spec-id enforcement — that's the workflow layer via hooks)
- Supervisory execution loops or agent orchestration (that's workflow-level)
- An AI agent or agent framework
- A GUI for editing documents (use your editor)
- A cloud/SaaS service
- Real-time collaboration

## Users

### Primary: Developer Using AI Agents

**Role:** Professional developer directing AI agents to produce code
**Goals:** Keep agent-facing documents organized, compose context quickly, reuse patterns across projects
**Pain:** Documents scattered everywhere, manual context assembly, reinventing instructions per project

### Secondary: Workflow Tool Author

**Role:** Developer building a methodology tool (like HELIX) on DDx primitives
**Goals:** Leverage DDx's document management, persona system, and sync without reimplementing them
**Pain:** No standard infrastructure to build on; every workflow tool reinvents document management

### Tertiary: Agent (Machine Consumer)

**Role:** AI agent consuming documents via MCP or filesystem
**Goals:** Discover available documents, read their contents, understand their relationships
**Pain:** No programmatic way to browse document libraries; relies on humans to copy-paste context

## Requirements

### Must Have (P0)

**CLI (`ddx`)**
- Initialize document library in a project (`ddx init`)
- List and browse documents by type (`ddx list`, `ddx prompts list`, etc.)
- Persona management with role bindings (`ddx persona list/show/bind`)
- Template application with variable substitution (`ddx templates apply`)
- Git subtree sync with upstream (`ddx update`, `ddx contribute`)
- Meta-prompt injection into CLAUDE.md
- MCP server installation and configuration (`ddx mcp install`)
- Installation health checks (`ddx doctor`)
- Self-upgrade (`ddx upgrade`)
- Bead tracker for work items (`ddx bead create/show/update/close/list/ready/blocked/status`)
- Bead dependency DAG (`ddx bead dep add/remove/tree`)
- Bead import/export for interchange with `bd`, `br`, and JSONL (`ddx bead import/export`)

**Server (`ddx-server`)**
- HTTP endpoints to browse document library contents
- MCP tool endpoints for agent consumption (list documents, read document, search)
- Serve from a local DDx library path
- Stateless — reads from filesystem, no database
- Bead endpoints (list, show, ready, status) via MCP and HTTP

**Website (`ddx.github.io`)**
- Clear explanation of what DDx is and why it exists
- Quick start guide
- Link to CLI installation
- Link to documentation

### Should Have (P1)

**CLI**
- Document composition commands (assemble context from multiple documents)
- Document freshness tracking and staleness warnings
- Pattern application with variable substitution
- Improved `ddx doctor` with document quality checks
- ADR scaffolding, listing, and validation (`ddx adr create/list/show/validate`)
- Solution design scaffolding, listing, and validation (`ddx sd create/list/show/validate`)

**Server**
- Document relationship/dependency information in MCP responses
- Search across document contents
- Persona resolution (given a role, return the bound persona document)
- ADR and SD endpoints (list, show) via MCP and HTTP

**Website**
- Ecosystem page showing workflow tools built on DDx
- Document library browser (interactive)

### Nice to Have (P2)

**CLI**
- Document testing (validate documents produce expected agent behavior)
- Document analytics (most reused, most effective)
- IDE integration for document browsing

**Server**
- WebSocket notifications when documents change
- Multi-library aggregation (serve documents from multiple projects)

## Constraints

- **Technical:** Git-native. File-based. No external services required. Go for CLI and server.
- **Scope:** DDx manages documents, not agents. No workflow enforcement.
- **Platform:** macOS, Linux, Windows for CLI. Server runs anywhere Go runs.
- **License:** MIT, open source.

## Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| Documents go stale and get ignored | High | High | `ddx doctor` checks freshness; make documents immediately useful to agents |
| MCP spec changes break server | Medium | Medium | Keep MCP integration thin; abstract behind internal API |
| Too much structure discourages adoption | Medium | Medium | Minimal defaults; let teams grow into structure |
| Git subtree complexity confuses users | Medium | Low | Wrap in simple commands; clear error messages |

## Success Criteria

- [ ] CLI: all P0 commands working with tests passing
- [ ] CLI: bead tracker CRUD, deps, import/export working with tests
- [ ] Server: MCP endpoints serving documents from local library
- [ ] Server: bead endpoints serving work items to agents
- [ ] Website: live at ddx.github.io with clear messaging
- [ ] At least one workflow tool (HELIX) successfully building on DDx beads
- [ ] Document library syncing between 2+ projects
