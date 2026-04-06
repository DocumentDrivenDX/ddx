---
ddx:
  id: helix.prd
  depends_on:
    - product-vision
---
# Product Requirements Document: DDx

**Version:** 3.5.0
**Date:** 2026-04-04
**Status:** Active

## Summary

DDx is a monorepo producing three artifacts that together form the shared
local-first infrastructure for document-driven development:

1. **`ddx` CLI** — document library management, artifact graph operations, bead
   tracking, agent dispatch, execution definitions and runs, persona
   composition, template application, and git sync
2. **`ddx-server`** — web server + MCP endpoints for browsing documents,
   artifacts, beads, agent session logs, and execution history over the network
3. **`ddx.github.io`** — promotional website explaining DDx to developers and
   linking to docs

DDx is the foundation layer. Workflow tools (HELIX, others) build on top. DDx
provides reusable local services; it does not impose workflow phases or
methodology.

Concrete command, API, and storage contracts belong in the DDx feature
specifications. The PRD stays at the user- and capability-level:

- FEAT-001 defines the CLI surface and operator experience
- FEAT-002 defines the server, HTTP, and MCP surfaces
- FEAT-003 defines the promotional website and documentation
- FEAT-004 defines shared work-item storage
- FEAT-005 defines the artifact convention and frontmatter schema
- FEAT-006 defines agent dispatch and session evidence
- FEAT-007 defines the artifact graph and staleness model
- FEAT-008 defines the embedded web UI for browsing and managing project state
- FEAT-009 defines the online library and plugin registry
- FEAT-010 defines generic execution definitions and immutable run history
- FEAT-011 defines agent-facing skills for DDx CLI operations
- FEAT-012 defines git awareness: auto-commit, document history, and
  write-then-commit for MCP/UI clients
- FEAT-013 defines multi-agent coordination: concurrent bead safety,
  MCP supervisor surface, worktree-aware dispatch
- FEAT-014 defines agent token awareness: usage tracking, budget enforcement,
  and model selection guidance across harnesses

## Problem

AI-assisted development needs more than prompt files. Teams need a shared way
to manage declarative artifacts, reusable runtime evidence, and local
automation infrastructure without hardcoding workflow semantics into each tool.

- **No structure**: Artifacts, prompts, personas, and patterns accumulate as
  ad hoc files with weak identity and no explicit relationships
- **No reusable work-item store**: Workflow tools reimplement issue storage,
  dependency tracking, and coordination instead of sharing a local substrate
- **No reusable agent dispatch**: Each tool grows its own harness registry,
  logging, and output-capture behavior
- **No reusable execution evidence**: Metrics, checks, and similar operations
  fall back to bespoke scripts and logs with no shared history model
- **No composition**: Assembling the right combination of persona + pattern +
  spec into agent context is manual and error-prone
- **No reuse**: Every project reinvents its agent instructions and supporting
  mechanics from scratch; proven patterns stay trapped in individual repos
- **No network access**: Agents and tools can only read state from the local
  filesystem unless projects build their own HTTP or MCP layer
- **No discoverability**: Developers can't easily browse what documents,
  artifacts, or local runtime evidence are available

## Goals

### Primary
1. **Manage artifacts and document libraries** — provide structure,
   conventions, and CLI tooling so declarative project knowledge stays
   organized
2. **Provide reusable local runtime services** — expose beads, agent dispatch,
   and execution history as workflow-agnostic DDx primitives
3. **Enable document composition** — combine personas, patterns, specs, and
   templates into coherent agent context
6. **Provide agent-facing skills for DDx operations** — ship interactive
   skills (slash commands) that guide agents through complex DDx CLI
   operations like bead triage, agent dispatch, and package installation
4. **Serve project state to agents and tools** — expose documents, artifacts,
   beads, and execution evidence via MCP endpoints and HTTP
5. **Support cross-project reuse** — share document libraries and workflow
   plugins through an online registry (`ddx install`)
7. **Integrate with revision control** — auto-commit document changes to
   protect work, expose document history to agents and tools, enable
   write-then-commit workflows for MCP and UI clients
8. **Support multi-agent coordination** — make bead operations, document
   writes, and agent dispatch safe under concurrent multi-agent use, with
   MCP as the remote observation and control surface
9. **Embed essential utilities** — bundle common developer tools (jq, etc.)
   so workflow tools have a consistent, cross-platform base without external
   runtime dependencies

### Secondary
1. **Promote the practice** — website explains document-driven development and
   drives adoption
2. **Keep artifacts honest** — detect drift between governing documents and
   lower-level artifacts or runtime evidence

### Success Metrics

| Metric | Target |
|--------|--------|
| Time to assemble agent context | <30 seconds |
| Document reuse rate across projects | >40% |
| MCP endpoint response time | <200ms |
| Website explains DDx clearly to new visitor | <2 minutes to understand |

### Non-Goals

- A workflow methodology (that's HELIX and others, not DDx)
- Workflow-specific artifact ladders or stage progression (for example,
  `FEAT -> SD -> TD -> TP`) beyond storing IDs and relationships
- Workflow-specific bead validation (phase labels, spec-id enforcement — that's
  the workflow layer via hooks)
- Supervisory loop orchestration — deciding what to do next based on agent or
  execution results is workflow-level. DDx provides single-invocation dispatch,
  immutable evidence, and mechanical quorum, not decision loops.
- An AI agent or agent framework
- A GUI for editing documents (use your editor)
- A cloud/SaaS service
- Real-time collaboration

## Users

### Primary: Developer Using AI Agents

**Role:** Professional developer directing AI agents and local automation
**Goals:** Keep project artifacts organized, compose context quickly, reuse
patterns across projects, inspect local execution evidence
**Pain:** Documents and evidence scattered everywhere, manual context assembly,
reinventing instructions and runtime tooling per project

### Secondary: Workflow Tool Author

**Role:** Developer building a methodology tool (like HELIX) on DDx primitives
**Goals:** Leverage DDx's document management, bead storage, agent dispatch,
execution history, persona system, and sync without reimplementing them
**Pain:** No standard infrastructure to build on; every workflow tool reinvents
local state, execution, and document management

### Tertiary: Agent (Machine Consumer)

**Role:** AI agent consuming documents via MCP or filesystem
**Goals:** Discover available documents and artifacts, read their contents,
understand their relationships, and inspect reusable runtime evidence
**Pain:** No programmatic way to browse document libraries or local execution
history; relies on humans to copy-paste context

## Requirements

### Must Have (P0)

**CLI experience**
The exact CLI contract lives in FEAT-001. At the PRD level, DDx must provide a
local operator surface that lets users:

- initialize and maintain a repo-local DDx workspace
- discover, inspect, and manage document-library content and declarative
  artifacts
- understand artifact relationships, dependency structure, and document
  freshness
- manage shared work items and their dependencies for higher-level tools
- dispatch supported AI agents through one reusable interface and inspect the
  resulting evidence
- validate installation and configuration health
- reuse and update shared DDx library content across projects
- invoke DDx operations through agent-facing skills (slash commands) that
  provide guided, validated workflows for complex CLI commands

**Server experience**
The exact server, HTTP, and MCP contracts live in FEAT-002. At the PRD level,
DDx must provide a local network surface that lets tools and agents:

- browse and read document-library content remotely
- query artifact metadata, relationships, and staleness
- inspect shared work-item state
- inspect recorded agent session evidence
- rely on a stateless, filesystem-backed implementation rather than a hosted
  service

**Website experience**
- Clear explanation of what DDx is and why it exists
- Quick start guide
- Link to CLI installation
- Link to documentation
- Embedded terminal recordings (asciinema) demonstrating core workflows
- README with animated demos that sell the tool at a glance

**Release infrastructure**
- CI pipeline that runs the full test suite (via lefthook) on every push and PR
- E2E smoke tests validating the install-to-use journey
- Automated demo recording regeneration when CLI behavior changes
- GitHub Pages deployment gated on CI passing
- Multi-platform release builds with changelog generation

### Should Have (P1)

**CLI experience**
The CLI feature spec should also define requirements for:

- assembling context from multiple DDx resources
- stronger document quality checks and health diagnostics
- generic execution definitions and immutable run history for evidence-producing
  operations
- higher-level projections over execution history for domains such as metrics
  and acceptance evidence

**Server experience**
The server feature spec should also define requirements for:

- richer search across document-library contents
- persona resolution for remote consumers
- read-oriented access to generic execution definitions and run history

**Website experience**
- Ecosystem page showing workflow tools built on DDx
- Document library browser (interactive)
- "See It In Action" section with recordings of end-to-end workflows
  (init, plugin install, project creation, feature evolution)

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
- **Agent safety:** DDx defaults to safe agent permissions. Permissive modes
  (`unrestricted`) require explicit opt-in via config or CLI flag. Normal
  users should never accidentally run agents with bypassed safety rails.
  See FEAT-006 Agent Permission Model.

## Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| Documents go stale and get ignored | High | High | `ddx doctor` checks freshness; make documents immediately useful to agents |
| MCP spec changes break server | Medium | Medium | Keep MCP integration thin; abstract behind internal API |
| Too much structure discourages adoption | Medium | Medium | Minimal defaults; let teams grow into structure |
| Git subtree complexity confuses users | Medium | Low | Wrap in simple commands; clear error messages |

## Success Criteria

- [ ] Users can set up DDx in a repository and manage project knowledge without
      relying on ad hoc file conventions
- [ ] Workflow tools can rely on DDx for shared work-item state instead of
      reimplementing local tracker storage
- [ ] Workflow tools can rely on DDx for agent dispatch and reusable invocation
      evidence
- [ ] Agents and tools can inspect repository documents and project state over
      local MCP or HTTP surfaces
- [ ] Website: live at ddx.github.io with clear messaging and embedded demos
- [ ] At least one workflow tool (HELIX) successfully building on DDx beads and
      agent dispatch
- [ ] `ddx install helix` bootstraps HELIX from the registry
- [ ] Document library syncing between 2+ projects
- [ ] CI pipeline green on every merge to main; Pages deploy gated on CI
- [ ] README includes animated terminal recordings of core workflows
