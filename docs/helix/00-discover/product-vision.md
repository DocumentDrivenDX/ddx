# Product Vision: DDx

## Mission Statement

DDx makes documents the unit of software development — providing the shared infrastructure that developers and workflow tools use to maintain, compose, and deliver the documents AI agents consume to build software.

## 3-5 Year Vision

| Timeframe | Market Position | Key Milestones |
|-----------|----------------|----------------|
| Year 1 | Established open-source toolkit used by early adopters building agent-driven workflows | CLI stable, server shipping, 3+ workflow tools building on DDx, active community library |
| Year 3 | Standard infrastructure layer for document-driven development, analogous to what npm is for packages | Ecosystem of workflow tools, enterprise adoption, rich MCP integration, document analytics |
| Year 5 | Foundational platform for the agent-driven development era — documents-as-code is the default practice | Industry-standard document formats, broad IDE integration, self-improving document libraries |

**North Star:** Every developer working with AI agents uses DDx (or tools built on DDx) to manage the documents that drive those agents.

## Target Market

| Segment | Primary: Agent-First Developers | Secondary: Team Leads & Architects |
|---------|-------------------------------|-------------------------------------|
| Size | Millions of developers using AI coding agents daily | Hundreds of thousands managing teams using agents |
| Pain | Documents scattered, no composition, no reuse — agents get bad context and produce bad output | No standardization across teams, no way to share what works, knowledge trapped in individual repos |
| Current Solution | Ad-hoc markdown files, copy-paste between projects, manual context assembly | Internal wikis, tribal knowledge, per-project CLAUDE.md files maintained by hand |

## Key Value Propositions

| Capability | Benefit |
|-----------|---------|
| Structured document library | Agent-facing documents stay organized, discoverable, and current instead of rotting in random files |
| Git-native sync | Proven patterns flow between projects and teams without reinventing sharing infrastructure |
| Persona composition | Consistent agent behavior across projects — bind "strict-code-reviewer" once, get it everywhere |
| Meta-prompt injection | Right baseline context injected into agents automatically, no manual assembly |
| MCP server for document access | Agents can programmatically browse and consume document libraries |
| Template engine | New projects start with proven document structures, not blank files |
| Bead tracker | Portable work items with dependency DAG, ready queue, and import/export — shared across workflow tools |
| Agent service | Unified harness dispatch (codex, claude, gemini, etc.) with quorum, session logging, and prompt envelope format |
| Document dependency graph | Track which docs depend on which, detect staleness via content hashing, cascade invalidation when upstream docs change |
| Workflow-agnostic primitives | Any methodology (HELIX, custom, etc.) can build on DDx without reimplementing infrastructure |

## Success Definition

| Metric | Target | Timeline |
|--------|--------|----------|
| Projects using DDx document libraries | 500+ | Year 1 |
| Workflow tools built on DDx | 3+ | Year 1 |
| Community-contributed personas/patterns | 100+ | Year 1 |
| Document reuse rate across projects | >40% | Year 1 |
| DDx server MCP endpoints adopted by agent tools | 2+ integrations | Year 1 |

## Strategic Fit

**Why us:** DDx grew out of real agent-driven development practice. The document management problems we're solving are ones we hit daily building software with AI agents.

**Why now:** AI agents crossed the capability threshold where document quality — not agent capability — is the bottleneck. Every team is independently discovering they need document infrastructure. No standard exists yet.

**Resources:** Open source, community-driven. Go CLI (proven tech), static website (low maintenance), server component (Go, leveraging CLI internals). Single repo, three focused outputs.

## Principles

1. **Documents are the product** — code is output, documents are what you maintain
2. **Infrastructure, not methodology** — DDx provides primitives, workflow tools provide opinions
3. **Git-native, file-first** — plain files, standard git, no lock-in
4. **Composition over monoliths** — small focused documents combined on demand
5. **Agent-agnostic** — documents work with any capable agent
